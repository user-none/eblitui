package chd

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
)

const (
	codecCDZL = 0x63647A6C // "cdzl"
	codecCDLZ = 0x63646C7A // "cdlz"
	codecCDFL = 0x6364666C // "cdfl"
	codecCDZS = 0x63647A73 // "cdzs"
)

// syncPattern is the CD sector sync field (12 bytes).
var syncPattern = [12]byte{0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00}

func decompressCD(codec uint32, compressed []byte, framesPerHunk int, hunkBytes uint32, zstdDec *zstd.Decoder) ([]byte, error) {
	// CDFL uses a different layout: raw FLAC frames followed by
	// deflate-compressed subcode, with no ECC/length header.
	if codec == codecCDFL {
		return decompressCDFL(compressed, framesPerHunk)
	}

	// Read ECC flags
	eccFlagBytes := (framesPerHunk + 7) / 8
	if len(compressed) < eccFlagBytes {
		return nil, fmt.Errorf("compressed data too short for ECC flags")
	}
	eccFlags := compressed[:eccFlagBytes]
	pos := eccFlagBytes

	// Read compressed base length
	var baseLen int
	if hunkBytes < 65536 {
		if pos+2 > len(compressed) {
			return nil, fmt.Errorf("compressed data too short for base length")
		}
		baseLen = int(binary.BigEndian.Uint16(compressed[pos : pos+2]))
		pos += 2
	} else {
		if pos+3 > len(compressed) {
			return nil, fmt.Errorf("compressed data too short for base length")
		}
		baseLen = int(compressed[pos])<<16 | int(compressed[pos+1])<<8 | int(compressed[pos+2])
		pos += 3
	}

	if pos+baseLen > len(compressed) {
		return nil, fmt.Errorf("compressed base data overflows buffer")
	}
	compBase := compressed[pos : pos+baseLen]
	compSubcode := compressed[pos+baseLen:]

	// Compute expected sizes for decompressors that need them.
	// Base: worst case all raw frames = framesPerHunk * cdSectorSize
	baseExpected := framesPerHunk * cdSectorSize

	// Select inner decompressors.
	type decompFunc func(data []byte, expectedSize int) ([]byte, error)
	var decompBase, decompSubcode decompFunc

	deflateFunc := func(data []byte, expectedSize int) ([]byte, error) {
		return decompressDeflate(data)
	}
	zstdFunc := func(data []byte, expectedSize int) ([]byte, error) {
		return decompressZstd(data, zstdDec)
	}

	switch codec {
	case codecCDZL:
		decompBase = deflateFunc
		decompSubcode = deflateFunc
	case codecCDLZ:
		dictSize := lzmaDictSize(hunkBytes)
		decompBase = func(data []byte, expectedSize int) ([]byte, error) {
			return decompressLZMA(data, expectedSize, dictSize)
		}
		decompSubcode = deflateFunc
	case codecCDZS:
		decompBase = zstdFunc
		decompSubcode = zstdFunc
	default:
		return nil, fmt.Errorf("unknown CD codec %08x", codec)
	}

	base, err := decompBase(compBase, baseExpected)
	if err != nil {
		return nil, fmt.Errorf("decompressing base: %w", err)
	}

	subcode, err := decompSubcode(compSubcode, 0)
	if err != nil {
		return nil, fmt.Errorf("decompressing subcode: %w", err)
	}

	// Reconstruct frames. Base data always contains full 2352-byte
	// sectors regardless of ECC flag. The ECC flag controls whether
	// to restore the sync header and regenerate ECC bytes.
	output := make([]byte, framesPerHunk*cdFrameSize)
	basePos := 0
	outPos := 0

	for i := 0; i < framesPerHunk; i++ {
		if basePos+cdSectorSize > len(base) {
			return nil, fmt.Errorf("base data underflow at frame %d", i)
		}
		copy(output[outPos:], base[basePos:basePos+cdSectorSize])

		byteIdx := i / 8
		bitIdx := uint(i % 8)
		eccSet := (eccFlags[byteIdx]>>bitIdx)&1 != 0
		if eccSet {
			// Restore sync header, then regenerate the EDC and P/Q parity
			// the encoder stripped so the sector is byte-faithful.
			copy(output[outPos:], syncPattern[:])
			eccGenerate(output[outPos : outPos+cdSectorSize])
		}

		basePos += cdSectorSize
		outPos += cdSectorSize

		// Subcode (96 bytes)
		scOff := i * cdSubcodeSize
		if scOff+cdSubcodeSize <= len(subcode) {
			copy(output[outPos:], subcode[scOff:scOff+cdSubcodeSize])
		}
		outPos += cdSubcodeSize
	}

	return output, nil
}

// decompressCDFL handles the CDFL (CD FLAC) codec which stores raw FLAC
// frames followed by deflate-compressed subcode, without the ECC/length
// header used by other CD codecs.
func decompressCDFL(compressed []byte, framesPerHunk int) ([]byte, error) {
	// Total samples to decode: each sector is 2352 bytes treated as
	// 16-bit stereo audio = 588 sample pairs per sector.
	totalSamples := framesPerHunk * cdSectorSize / 4 // 16-bit * 2 channels = 4 bytes per sample pair

	base, consumed, err := decodeFLAC(compressed, 2, 16, totalSamples)
	if err != nil {
		return nil, fmt.Errorf("decompressing CDFL base: %w", err)
	}

	// FLAC output is little-endian 16-bit samples but sector data is
	// raw bytes stored as big-endian. Swap to restore original byte order.
	for i := 0; i+1 < len(base); i += 2 {
		base[i], base[i+1] = base[i+1], base[i]
	}

	// Remaining compressed data is deflate-compressed subcode
	var subcode []byte
	if consumed < len(compressed) {
		subcode, err = decompressDeflate(compressed[consumed:])
		if err != nil {
			return nil, fmt.Errorf("decompressing CDFL subcode: %w", err)
		}
	}

	// Reassemble frames: sector data + subcode per frame
	output := make([]byte, framesPerHunk*cdFrameSize)
	basePos := 0
	outPos := 0

	for i := 0; i < framesPerHunk; i++ {
		if basePos+cdSectorSize <= len(base) {
			copy(output[outPos:], base[basePos:basePos+cdSectorSize])
		}
		basePos += cdSectorSize
		outPos += cdSectorSize

		scOff := i * cdSubcodeSize
		if scOff+cdSubcodeSize <= len(subcode) {
			copy(output[outPos:], subcode[scOff:scOff+cdSubcodeSize])
		}
		outPos += cdSubcodeSize
	}

	return output, nil
}

func decompressDeflate(data []byte) ([]byte, error) {
	fr := flate.NewReader(bytes.NewReader(data))
	defer fr.Close()
	return io.ReadAll(fr)
}

func decompressLZMA(data []byte, expectedSize int, dictSize uint32) ([]byte, error) {
	return decodeLZMA(data, expectedSize, 0x5D, dictSize)
}

// lzmaDictSize computes the LZMA dictionary size for CHD.
// This mirrors libchdr's lzma_compute_aligned_dictionary_size with level=9.
func lzmaDictSize(hunkBytes uint32) uint32 {
	// Level 9 on 64-bit: dictSize = 1 << (sizeof(size_t)/2 + 24) = 1 << 28
	dictSize := uint32(1 << 28)

	// Alignment step from LzmaEnc_WriteProperties
	if dictSize >= (1 << 21) {
		kDictMask := uint32((1 << 20) - 1)
		aligned := (dictSize + kDictMask) &^ kDictMask
		if aligned < dictSize {
			return aligned
		}
	}
	return dictSize
}

func decompressFLAC(data []byte) ([]byte, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("flac: empty data")
	}
	endian := data[0]
	if endian != 'L' && endian != 'B' {
		return nil, fmt.Errorf("flac: invalid endian byte 0x%02X", endian)
	}
	out, _, err := decodeFLAC(data[1:], 2, 16, 0)
	if err != nil {
		return nil, err
	}
	if endian == 'B' {
		for i := 0; i+1 < len(out); i += 2 {
			out[i], out[i+1] = out[i+1], out[i]
		}
	}
	return out, nil
}

func decompressZstd(data []byte, dec *zstd.Decoder) ([]byte, error) {
	return dec.DecodeAll(data, nil)
}
