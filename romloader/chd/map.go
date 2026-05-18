package chd

import (
	"encoding/binary"
	"fmt"
)

const (
	compCodec0 = 0
	compCodec1 = 1
	compCodec2 = 2
	compCodec3 = 3
	compNone   = 4
	compSelf   = 5
	compParent = 6

	compRLESmall  = 7
	compRLELarge  = 8
	compSelf0     = 9
	compSelf1     = 10
	compParentSel = 11
	compParent0   = 12
	compParent1   = 13
)

type mapEntry struct {
	compression uint8
	length      uint32
	offset      uint64
	crc16       uint16
}

func (rd *Reader) parseMap(mapOffset uint64) error {
	var hdr [16]byte
	if _, err := rd.r.ReadAt(hdr[:], int64(mapOffset)); err != nil {
		return fmt.Errorf("reading map header: %w", err)
	}

	compressedLen := binary.BigEndian.Uint32(hdr[0:4])
	firstOffs := getBE48(hdr[4:10])
	// mapCRC at hdr[10:12] - verified after decode
	lengthBits := int(hdr[12])
	selfBits := int(hdr[13])
	parentBits := int(hdr[14])

	// Read compressed map data
	compData := make([]byte, compressedLen)
	if _, err := rd.r.ReadAt(compData, int64(mapOffset)+16); err != nil {
		return fmt.Errorf("reading compressed map: %w", err)
	}

	bs := newBitstream(compData)

	// Decode Huffman tree for compression types (16 symbols, 8-bit max)
	huff, err := newHuffmanDecoder(16, 8)
	if err != nil {
		return fmt.Errorf("creating huffman decoder: %w", err)
	}
	if err := huff.importTreeRLE(bs); err != nil {
		return fmt.Errorf("importing huffman tree: %w", err)
	}

	rd.entries = make([]mapEntry, rd.totalHunks)

	// First pass: decode compression types using Huffman
	repCount := 0
	var lastComp uint8
	for i := uint32(0); i < rd.totalHunks; i++ {
		if repCount > 0 {
			rd.entries[i].compression = lastComp
			repCount--
		} else {
			if bs.overflow() {
				return fmt.Errorf("bitstream overflow at hunk %d", i)
			}
			val := uint8(huff.decodeOne(bs))
			switch val {
			case compRLESmall:
				rd.entries[i].compression = lastComp
				repCount = 2 + int(huff.decodeOne(bs))
			case compRLELarge:
				rd.entries[i].compression = lastComp
				repCount = 2 + 16 + int(huff.decodeOne(bs))<<4
				repCount += int(huff.decodeOne(bs))
			default:
				rd.entries[i].compression = val
				lastComp = val
			}
		}
	}

	// Second pass: decode offsets, lengths, CRCs
	curOffset := firstOffs
	var lastSelf uint64
	var lastParent uint64
	for i := uint32(0); i < rd.totalHunks; i++ {
		e := &rd.entries[i]
		offset := curOffset
		var length uint32
		var crc uint16

		switch e.compression {
		case compCodec0, compCodec1, compCodec2, compCodec3:
			length = uint32(bs.read(lengthBits))
			curOffset += uint64(length)
			crc = uint16(bs.read(16))
		case compNone:
			length = rd.hunkBytes
			curOffset += uint64(length)
			crc = uint16(bs.read(16))
		case compSelf:
			offset = uint64(bs.read(selfBits))
			lastSelf = offset
		case compParent:
			offset = uint64(bs.read(parentBits))
			lastParent = offset

		// Pseudo-types: resolve to base types
		case compSelf1:
			lastSelf++
			e.compression = compSelf
			offset = lastSelf
		case compSelf0:
			e.compression = compSelf
			offset = lastSelf
		case compParentSel:
			e.compression = compParent
			offset = uint64(i) * uint64(rd.hunkBytes) / uint64(rd.unitBytes)
			lastParent = offset
		case compParent1:
			lastParent += uint64(rd.hunkBytes) / uint64(rd.unitBytes)
			e.compression = compParent
			offset = lastParent
		case compParent0:
			e.compression = compParent
			offset = lastParent
		}

		e.offset = offset
		e.length = length
		e.crc16 = crc
	}

	return nil
}

func getBE48(b []byte) uint64 {
	return uint64(b[0])<<40 | uint64(b[1])<<32 | uint64(b[2])<<24 |
		uint64(b[3])<<16 | uint64(b[4])<<8 | uint64(b[5])
}

// bitstream implements the same buffered bit reader as libchdr.
// It reads bytes into a 32-bit buffer and extracts bits MSB-first.
type bitstream struct {
	data    []byte
	doffset int
	buffer  uint32
	bits    int
}

func newBitstream(data []byte) *bitstream {
	return &bitstream{data: data}
}

func (bs *bitstream) peek(n int) uint32 {
	if n == 0 {
		return 0
	}
	for bs.bits <= 24 {
		if bs.doffset < len(bs.data) {
			bs.buffer |= uint32(bs.data[bs.doffset]) << (24 - bs.bits)
		}
		bs.doffset++
		bs.bits += 8
	}
	return bs.buffer >> (32 - n)
}

func (bs *bitstream) remove(n int) {
	bs.buffer <<= n
	bs.bits -= n
}

func (bs *bitstream) read(n int) uint32 {
	val := bs.peek(n)
	bs.remove(n)
	return val
}

func (bs *bitstream) overflow() bool {
	return (bs.doffset - bs.bits/8) > len(bs.data)
}

// huffmanDecoder decodes Huffman-encoded symbols from a bitstream.
type huffmanDecoder struct {
	numCodes int
	maxBits  int
	lookup   []uint32 // (value << 5) | numbits
	nodeBits []uint8  // bit lengths per code
}

func newHuffmanDecoder(numCodes, maxBits int) (*huffmanDecoder, error) {
	if maxBits > 24 {
		return nil, fmt.Errorf("maxBits %d exceeds 24", maxBits)
	}
	return &huffmanDecoder{
		numCodes: numCodes,
		maxBits:  maxBits,
		lookup:   make([]uint32, 1<<maxBits),
		nodeBits: make([]uint8, numCodes),
	}, nil
}

func (h *huffmanDecoder) decodeOne(bs *bitstream) uint32 {
	bits := bs.peek(h.maxBits)
	entry := h.lookup[bits]
	bs.remove(int(entry & 0x1f))
	return entry >> 5
}

func (h *huffmanDecoder) importTreeRLE(bs *bitstream) error {
	// Bits per entry depends on maxBits
	var numBits int
	if h.maxBits >= 16 {
		numBits = 5
	} else if h.maxBits >= 8 {
		numBits = 4
	} else {
		numBits = 3
	}

	curNode := 0
	for curNode < h.numCodes {
		nodeBits := int(bs.read(numBits))
		if nodeBits != 1 {
			h.nodeBits[curNode] = uint8(nodeBits)
			curNode++
		} else {
			// Escape: read next value
			nodeBits = int(bs.read(numBits))
			if nodeBits == 1 {
				// Double-1 = literal 1
				h.nodeBits[curNode] = 1
				curNode++
			} else {
				// RLE: repeat count + 3
				repCount := int(bs.read(numBits)) + 3
				if repCount+curNode > h.numCodes {
					return fmt.Errorf("RLE overflow: %d + %d > %d", repCount, curNode, h.numCodes)
				}
				for repCount > 0 {
					h.nodeBits[curNode] = uint8(nodeBits)
					curNode++
					repCount--
				}
			}
		}
	}

	if err := h.assignCanonicalCodes(); err != nil {
		return err
	}
	return h.buildLookupTable()
}

func (h *huffmanDecoder) assignCanonicalCodes() error {
	// Build histogram of bit lengths
	var bitHisto [33]uint32
	for i := 0; i < h.numCodes; i++ {
		if int(h.nodeBits[i]) > h.maxBits {
			return fmt.Errorf("node %d has %d bits, exceeds max %d", i, h.nodeBits[i], h.maxBits)
		}
		if h.nodeBits[i] <= 32 {
			bitHisto[h.nodeBits[i]]++
		}
	}

	// Determine starting code for each length
	var curStart uint32
	for codeLen := 32; codeLen > 0; codeLen-- {
		nextStart := (curStart + bitHisto[codeLen]) >> 1
		bitHisto[codeLen] = curStart
		curStart = nextStart
	}

	return nil
}

func (h *huffmanDecoder) buildLookupTable() error {
	// Build histogram again for code assignment
	var bitHisto [33]uint32
	for i := 0; i < h.numCodes; i++ {
		if h.nodeBits[i] <= 32 {
			bitHisto[h.nodeBits[i]]++
		}
	}

	var curStart uint32
	for codeLen := 32; codeLen > 0; codeLen-- {
		nextStart := (curStart + bitHisto[codeLen]) >> 1
		bitHisto[codeLen] = curStart
		curStart = nextStart
	}

	// Assign codes and fill lookup table
	tableSize := uint32(1) << h.maxBits
	for i := 0; i < h.numCodes; i++ {
		nBits := h.nodeBits[i]
		if nBits == 0 {
			continue
		}
		code := bitHisto[nBits]
		bitHisto[nBits]++

		value := (uint32(i) << 5) | uint32(nBits)
		shift := h.maxBits - int(nBits)
		start := code << shift
		end := ((code + 1) << shift) - 1
		if start >= tableSize || end >= tableSize {
			return fmt.Errorf("lookup overflow: code %d, bits %d", i, nBits)
		}
		for j := start; j <= end; j++ {
			h.lookup[j] = value
		}
	}

	return nil
}
