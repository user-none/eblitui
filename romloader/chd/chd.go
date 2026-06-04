package chd

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
)

const (
	magic        = "MComprHD"
	v5HeaderSize = 124

	cdSectorSize  = 2352
	cdSubcodeSize = 96
	cdFrameSize   = 2448 // sector + subcode

	cacheSlots = 8
)

// Reader reads sectors from a CHD V5 disc image.
type Reader struct {
	r             io.ReaderAt
	codecs        [4]uint32
	logicalBytes  uint64
	hunkBytes     uint32
	unitBytes     uint32
	totalHunks    uint32
	framesPerHunk int
	entries       []mapEntry
	tracks        []Track
	cache         [cacheSlots]cacheEntry
	cacheAge      uint64
	zstdDec       *zstd.Decoder
}

// Track describes a single CD track from CHD metadata.
type Track struct {
	Number   int
	Type     string // "MODE1_RAW", "AUDIO", etc.
	Frames   int
	Pregap   int
	Control  uint8 // CTRL/ADR byte (0x41=data, 0x01=audio)
	startLBA int   // first data LBA for this track
	chdStart int   // first CHD frame index (includes pregap)
}

// StartLBA returns the first data LBA for this track.
func (t Track) StartLBA() int {
	return t.startLBA
}

type cacheEntry struct {
	hunkIdx int
	data    []byte
	age     uint64
}

// Open parses a CHD V5 file and returns a Reader.
// The caller must provide an io.ReaderAt and the total file size.
func Open(r io.ReaderAt, size int64) (*Reader, error) {
	if size < v5HeaderSize {
		return nil, errors.New("chd: file too small for header")
	}

	var hdr [v5HeaderSize]byte
	if _, err := r.ReadAt(hdr[:], 0); err != nil {
		return nil, fmt.Errorf("chd: reading header: %w", err)
	}

	if string(hdr[0:8]) != magic {
		return nil, errors.New("chd: invalid magic")
	}

	headerLen := binary.BigEndian.Uint32(hdr[8:12])
	version := binary.BigEndian.Uint32(hdr[12:16])
	if version != 5 {
		return nil, fmt.Errorf("chd: unsupported version %d", version)
	}
	_ = headerLen

	rd := &Reader{r: r}
	rd.codecs[0] = binary.BigEndian.Uint32(hdr[16:20])
	rd.codecs[1] = binary.BigEndian.Uint32(hdr[20:24])
	rd.codecs[2] = binary.BigEndian.Uint32(hdr[24:28])
	rd.codecs[3] = binary.BigEndian.Uint32(hdr[28:32])
	rd.logicalBytes = binary.BigEndian.Uint64(hdr[32:40])
	mapOffset := binary.BigEndian.Uint64(hdr[40:48])
	metaOffset := binary.BigEndian.Uint64(hdr[48:56])
	rd.hunkBytes = binary.BigEndian.Uint32(hdr[56:60])
	rd.unitBytes = binary.BigEndian.Uint32(hdr[60:64])

	if rd.hunkBytes == 0 {
		return nil, errors.New("chd: hunkBytes is zero")
	}
	rd.totalHunks = uint32((rd.logicalBytes + uint64(rd.hunkBytes) - 1) / uint64(rd.hunkBytes))
	rd.framesPerHunk = int(rd.hunkBytes) / cdFrameSize

	if err := rd.parseMap(mapOffset); err != nil {
		return nil, fmt.Errorf("chd: parsing map: %w", err)
	}

	if err := rd.parseMetadata(metaOffset); err != nil {
		return nil, fmt.Errorf("chd: parsing metadata: %w", err)
	}

	dec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return nil, fmt.Errorf("chd: creating zstd decoder: %w", err)
	}
	rd.zstdDec = dec

	for i := range rd.cache {
		rd.cache[i].hunkIdx = -1
	}

	return rd, nil
}

// ReadSector returns the 2352-byte sector data at the given LBA.
func (rd *Reader) ReadSector(lba int) ([]byte, error) {
	frameIdx, err := rd.lbaToFrame(lba)
	if err != nil {
		return nil, err
	}

	hunkIdx := frameIdx / rd.framesPerHunk
	frameInHunk := frameIdx % rd.framesPerHunk

	hunkData, err := rd.getHunk(hunkIdx)
	if err != nil {
		return nil, fmt.Errorf("chd: reading hunk %d: %w", hunkIdx, err)
	}

	off := frameInHunk * cdFrameSize
	if off+cdSectorSize > len(hunkData) {
		return nil, fmt.Errorf("chd: frame %d out of hunk bounds", frameInHunk)
	}
	return hunkData[off : off+cdSectorSize], nil
}

// Tracks returns a copy of the track list.
func (rd *Reader) Tracks() []Track {
	out := make([]Track, len(rd.tracks))
	copy(out, rd.tracks)
	return out
}

// Close releases resources held by the reader.
func (rd *Reader) Close() {
	if rd.zstdDec != nil {
		rd.zstdDec.Close()
		rd.zstdDec = nil
	}
}

func (rd *Reader) lbaToFrame(lba int) (int, error) {
	for i := len(rd.tracks) - 1; i >= 0; i-- {
		t := &rd.tracks[i]
		if lba >= t.startLBA {
			offset := lba - t.startLBA
			if offset >= t.Frames {
				return 0, fmt.Errorf("chd: LBA %d beyond track %d", lba, t.Number)
			}
			return t.chdStart + offset, nil
		}
	}
	return 0, fmt.Errorf("chd: LBA %d before first track", lba)
}

func (rd *Reader) getHunk(idx int) ([]byte, error) {
	if idx < 0 || idx >= int(rd.totalHunks) {
		return nil, fmt.Errorf("hunk index %d out of range", idx)
	}

	// Check cache
	for i := range rd.cache {
		if rd.cache[i].hunkIdx == idx {
			rd.cacheAge++
			rd.cache[i].age = rd.cacheAge
			return rd.cache[i].data, nil
		}
	}

	entry := &rd.entries[idx]
	var data []byte
	var err error

	switch entry.compression {
	case compSelf:
		// Self-reference: decompress the referenced hunk
		refIdx := int(entry.offset)
		data, err = rd.getHunk(refIdx)
		if err != nil {
			return nil, fmt.Errorf("self-ref hunk %d: %w", refIdx, err)
		}
		// Copy so we don't share the cache slice
		cp := make([]byte, len(data))
		copy(cp, data)
		data = cp
	case compNone:
		// Uncompressed: read directly from file.
		data = make([]byte, rd.hunkBytes)
		if _, err := rd.r.ReadAt(data, int64(entry.offset)); err != nil {
			return nil, fmt.Errorf("reading uncompressed hunk %d: %w", idx, err)
		}
		rd.swapFrames(idx, data, true)
	case compParent:
		return nil, fmt.Errorf("parent compression not supported (hunk %d)", idx)
	default:
		// Codec 0-3
		if entry.compression > 3 {
			return nil, fmt.Errorf("unsupported compression type %d", entry.compression)
		}
		compressed := make([]byte, entry.length)
		if _, err := rd.r.ReadAt(compressed, int64(entry.offset)); err != nil {
			return nil, fmt.Errorf("reading compressed data: %w", err)
		}
		codec := rd.codecs[entry.compression]
		data, err = rd.decompress(idx, codec, compressed)
		if err != nil {
			return nil, fmt.Errorf("decompressing hunk %d (codec %08x): %w", idx, codec, err)
		}
	}

	// Store in cache, evict oldest
	rd.cacheAge++
	evictIdx := 0
	minAge := rd.cache[0].age
	for i := 1; i < cacheSlots; i++ {
		if rd.cache[i].age < minAge {
			minAge = rd.cache[i].age
			evictIdx = i
		}
	}
	rd.cache[evictIdx] = cacheEntry{
		hunkIdx: idx,
		data:    data,
		age:     rd.cacheAge,
	}

	return data, nil
}

func (rd *Reader) decompress(idx int, codec uint32, compressed []byte) ([]byte, error) {
	switch codec {
	case codecCDZL, codecCDLZ, codecCDFL, codecCDZS:
		data, err := decompressCD(codec, compressed, rd.framesPerHunk, rd.hunkBytes, rd.zstdDec)
		if err != nil {
			return nil, err
		}
		// CDFL decodes native little-endian (audio already canonical, data must
		// be swapped to its true order); the base codecs reconstruct big-endian
		// frames (data already byte-exact, audio must be swapped). Data hunks can
		// reach either path - codec is by ratio.
		rd.swapFrames(idx, data, codec != codecCDFL)
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported codec %08x", codec)
	}
}
