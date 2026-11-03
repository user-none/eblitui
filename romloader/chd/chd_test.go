package chd

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// testHunk is one hunk map entry for testCHD.
type testHunk struct {
	comp    uint8  // map compression type (compCodec0-3, compNone, compSelf)
	data    []byte // stored bytes for codec and none types
	offset  uint64 // referenced hunk for compSelf
	decoded []byte // codec types: the decoded hunk the entry's CRC covers
}

// crc returns the CRC the map stores for the hunk: of the decoded hunk for
// codec types, of the stored bytes for compNone, and 0 for references.
func (h testHunk) crc() uint16 {
	switch h.comp {
	case compCodec0, compCodec1, compCodec2, compCodec3:
		return crc16(h.decoded)
	case compNone:
		return crc16(h.data)
	}
	return 0
}

// testCHD describes a CHD v5 image built in memory by build. Hunk data is
// stored after the header, followed by the map and then the metadata chain.
type testCHD struct {
	headerLen    uint32 // 0 means v5HeaderSize
	codecs       [4]uint32
	hunkBytes    uint32
	unitBytes    uint32
	logicalBytes uint64 // 0 means len(hunks) * hunkBytes
	lengthBits   uint8
	selfBits     uint8
	parentBits   uint8
	hunks        []testHunk
	extraMeta    int      // non-track metadata entries placed before the tracks
	tracks       []string // one CHT2 metadata payload per track
}

// testMetaTag is a metadata tag the reader does not interpret.
const testMetaTag = 0x54455354 // "TEST"

// bitWriter writes values MSB first, the order the map bitstream reads.
type bitWriter struct {
	buf   []byte
	nbits int
}

func (w *bitWriter) write(v uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		if w.nbits%8 == 0 {
			w.buf = append(w.buf, 0)
		}
		if v>>uint(i)&1 != 0 {
			w.buf[len(w.buf)-1] |= 0x80 >> uint(w.nbits%8)
		}
		w.nbits++
	}
}

// build returns the image bytes. The map's Huffman tree gives all 16
// compression types a 4-bit code equal to the type, so each hunk's type is
// written as 4 bits.
func (c testCHD) build() []byte {
	out := make([]byte, v5HeaderSize)

	firstOffs := uint64(len(out))
	for _, h := range c.hunks {
		out = append(out, h.data...)
	}

	var bw bitWriter
	for range 16 {
		bw.write(4, 4)
	}
	for _, h := range c.hunks {
		bw.write(uint64(h.comp), 4)
	}
	// The map CRC covers each decoded entry as 12 bytes: type, 24-bit
	// length, 48-bit offset and 16-bit hunk CRC.
	var rawMap []byte
	dataOffset := firstOffs
	for _, h := range c.hunks {
		var length uint32
		offset := dataOffset
		switch h.comp {
		case compCodec0, compCodec1, compCodec2, compCodec3:
			length = uint32(len(h.data))
			bw.write(uint64(length), int(c.lengthBits))
			bw.write(uint64(h.crc()), 16)
		case compNone:
			length = c.hunkBytes
			bw.write(uint64(h.crc()), 16)
		case compSelf:
			offset = h.offset
			bw.write(h.offset, int(c.selfBits))
		}
		dataOffset += uint64(len(h.data))
		rawMap = append(rawMap, h.comp, byte(length>>16), byte(length>>8), byte(length))
		rawMap = append(rawMap, byte(offset>>40), byte(offset>>32), byte(offset>>24), byte(offset>>16), byte(offset>>8), byte(offset))
		rawMap = binary.BigEndian.AppendUint16(rawMap, h.crc())
	}

	mapOffset := uint64(len(out))
	var mapHdr [16]byte
	binary.BigEndian.PutUint32(mapHdr[0:4], uint32(len(bw.buf)))
	binary.BigEndian.PutUint16(mapHdr[10:12], crc16(rawMap))
	mapHdr[4] = byte(firstOffs >> 40)
	mapHdr[5] = byte(firstOffs >> 32)
	mapHdr[6] = byte(firstOffs >> 24)
	mapHdr[7] = byte(firstOffs >> 16)
	mapHdr[8] = byte(firstOffs >> 8)
	mapHdr[9] = byte(firstOffs)
	mapHdr[12] = c.lengthBits
	mapHdr[13] = c.selfBits
	mapHdr[14] = c.parentBits
	out = append(out, mapHdr[:]...)
	out = append(out, bw.buf...)

	type metaNode struct {
		tag     uint32
		payload string
	}
	var nodes []metaNode
	for range c.extraMeta {
		nodes = append(nodes, metaNode{testMetaTag, "x"})
	}
	for _, tr := range c.tracks {
		nodes = append(nodes, metaNode{metaTagCHT2, tr})
	}

	var metaOffset uint64
	if len(nodes) > 0 {
		metaOffset = uint64(len(out))
	}
	for i, n := range nodes {
		var node [16]byte
		binary.BigEndian.PutUint32(node[0:4], n.tag)
		binary.BigEndian.PutUint32(node[4:8], uint32(len(n.payload)))
		if i+1 < len(nodes) {
			binary.BigEndian.PutUint64(node[8:16], uint64(len(out)+16+len(n.payload)))
		}
		out = append(out, node[:]...)
		out = append(out, n.payload...)
	}

	logical := c.logicalBytes
	if logical == 0 {
		logical = uint64(len(c.hunks)) * uint64(c.hunkBytes)
	}
	headerLen := c.headerLen
	if headerLen == 0 {
		headerLen = v5HeaderSize
	}
	copy(out[0:8], magic)
	binary.BigEndian.PutUint32(out[8:12], headerLen)
	binary.BigEndian.PutUint32(out[12:16], 5)
	for i, codec := range c.codecs {
		binary.BigEndian.PutUint32(out[16+4*i:20+4*i], codec)
	}
	binary.BigEndian.PutUint64(out[32:40], logical)
	binary.BigEndian.PutUint64(out[40:48], mapOffset)
	binary.BigEndian.PutUint64(out[48:56], metaOffset)
	binary.BigEndian.PutUint32(out[56:60], c.hunkBytes)
	binary.BigEndian.PutUint32(out[60:64], c.unitBytes)
	return out
}

// openTestCHD builds c and opens it from memory.
func openTestCHD(t *testing.T, c testCHD) (*Reader, error) {
	t.Helper()
	img := c.build()
	return Open(bytes.NewReader(img), int64(len(img)))
}

// testFramesPerHunk is the hunk size used by the CD codec tests: 8 frames
// of 2448 bytes, small enough that the base length is a 2-byte field.
const testFramesPerHunk = 8

// cdFrames interleaves sector data and subcode into 2448-byte frames, the
// decoded form of a CD hunk.
func cdFrames(sectors, subcode []byte) []byte {
	n := len(sectors) / cdSectorSize
	out := make([]byte, 0, n*cdFrameSize)
	for f := range n {
		out = append(out, sectors[f*cdSectorSize:(f+1)*cdSectorSize]...)
		out = append(out, subcode[f*cdSubcodeSize:(f+1)*cdSubcodeSize]...)
	}
	return out
}

// cdzsHunk builds a cdzs hunk payload: one ECC flag byte (no flags set),
// the 2-byte compressed base length, the base stream and the subcode
// stream.
func cdzsHunk(base, subcode []byte) []byte {
	out := []byte{0}
	out = binary.BigEndian.AppendUint16(out, uint16(len(base)))
	out = append(out, base...)
	return append(out, subcode...)
}

// TestReadSectorCDZS tests that a cdzs hunk whose streams fit the hunk
// decodes, so the zstd memory limit does not reject real data.
func TestReadSectorCDZS(t *testing.T) {
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatalf("zstd.NewWriter: %v", err)
	}
	defer enc.Close()

	base := bytes.Repeat([]byte{0x5A}, testFramesPerHunk*cdSectorSize)
	subcode := make([]byte, testFramesPerHunk*cdSubcodeSize)
	hunk := cdzsHunk(enc.EncodeAll(base, nil), enc.EncodeAll(subcode, nil))

	rd, err := openTestCHD(t, testCHD{
		codecs:     [4]uint32{codecCDZS},
		hunkBytes:  testFramesPerHunk * cdFrameSize,
		unitBytes:  cdFrameSize,
		lengthBits: 24,
		selfBits:   24,
		parentBits: 24,
		hunks:      []testHunk{{comp: compCodec0, data: hunk, decoded: cdFrames(base, subcode)}},
		tracks:     []string{"TRACK:1 TYPE:MODE1_RAW SUBTYPE:NONE FRAMES:8"},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rd.Close()

	sector, err := rd.ReadSector(0)
	if err != nil {
		t.Fatalf("ReadSector: %v", err)
	}
	if !bytes.Equal(sector, base[:cdSectorSize]) {
		t.Fatal("sector does not match the encoded base data")
	}
}

// TestReadSectorCDZSDeclaredSizeTooLarge tests that a zstd frame declaring
// more output than a hunk holds is rejected before its output is allocated.
func TestReadSectorCDZSDeclaredSizeTooLarge(t *testing.T) {
	// Single-segment frame header declaring a 16 MiB content size, then one
	// empty raw last block.
	frame := []byte{0x28, 0xB5, 0x2F, 0xFD, 0xE0}
	frame = binary.LittleEndian.AppendUint64(frame, 16<<20)
	frame = append(frame, 0x01, 0x00, 0x00)

	rd, err := openTestCHD(t, testCHD{
		codecs:     [4]uint32{codecCDZS},
		hunkBytes:  testFramesPerHunk * cdFrameSize,
		unitBytes:  cdFrameSize,
		lengthBits: 24,
		selfBits:   24,
		parentBits: 24,
		hunks:      []testHunk{{comp: compCodec0, data: cdzsHunk(frame, nil)}},
		tracks:     []string{"TRACK:1 TYPE:MODE1_RAW SUBTYPE:NONE FRAMES:8"},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rd.Close()

	_, err = rd.ReadSector(0)
	if !errors.Is(err, zstd.ErrDecoderSizeExceeded) {
		t.Fatalf("expected zstd.ErrDecoderSizeExceeded, got %v", err)
	}
}

// metaTestCHD is a one-hunk image carrying the given track lines and extra
// metadata entries.
func metaTestCHD(extraMeta int, tracks ...string) testCHD {
	return testCHD{
		codecs:     [4]uint32{codecCDZS},
		hunkBytes:  testFramesPerHunk * cdFrameSize,
		unitBytes:  cdFrameSize,
		lengthBits: 24,
		selfBits:   24,
		parentBits: 24,
		hunks:      []testHunk{{comp: compNone, data: make([]byte, testFramesPerHunk*cdFrameSize)}},
		extraMeta:  extraMeta,
		tracks:     tracks,
	}
}

// openImage opens a built image from memory.
func openImage(img []byte) (*Reader, error) {
	return Open(bytes.NewReader(img), int64(len(img)))
}

// trackLines returns n empty CHT2 track lines numbered from 1. They hold no
// frames, so any number of them fits the one-hunk test image.
func trackLines(n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("TRACK:%d TYPE:MODE1_RAW SUBTYPE:NONE FRAMES:0", i+1)
	}
	return lines
}

// TestOpenTrackValues tests the limits on track numbers, frame counts and
// pregaps, and that tracks must fit the frames the image holds. The test
// image holds one hunk of testFramesPerHunk frames.
func TestOpenTrackValues(t *testing.T) {
	line := func(num, frames, pregap string) string {
		return "TRACK:" + num + " TYPE:MODE1_RAW SUBTYPE:NONE FRAMES:" + frames + " PREGAP:" + pregap
	}
	cases := []struct {
		name   string
		tracks []string
		wantOK bool
	}{
		{"two tracks", []string{line("1", "4", "0"), line("2", "4", "2")}, true},
		{"chain out of order", []string{line("2", "4", "0"), line("1", "4", "0")}, true},
		{"zero frames", []string{line("1", "0", "0"), line("2", "8", "0")}, true},
		{"pregap equals frames", []string{line("1", "4", "4")}, true},
		{"padded to the image size", []string{line("1", "5", "0")}, true},
		{"track zero", []string{line("0", "4", "0")}, false},
		{"negative track", []string{line("-1", "4", "0")}, false},
		{"track 100", []string{line("100", "4", "0")}, false},
		{"duplicate track", []string{line("1", "4", "0"), line("1", "4", "0")}, false},
		{"negative frames", []string{line("1", "-100", "0"), line("2", "4", "0")}, false},
		{"negative pregap", []string{line("1", "4", "-1")}, false},
		{"pregap past frames", []string{line("1", "4", "5")}, false},
		{"tracks exceed image", []string{line("1", "4", "0"), line("2", "5", "0")}, false},
		{"huge frames", []string{line("1", "9223372036854775807", "0")}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rd, err := openTestCHD(t, metaTestCHD(0, tc.tracks...))
			if !tc.wantOK {
				if err == nil {
					rd.Close()
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			rd.Close()
		})
	}
}

// TestOpenTrackLayout tests the start LBA and pregap of a valid two-track
// image.
func TestOpenTrackLayout(t *testing.T) {
	rd, err := openTestCHD(t, metaTestCHD(0,
		"TRACK:1 TYPE:MODE1_RAW SUBTYPE:NONE FRAMES:3 PREGAP:0",
		"TRACK:2 TYPE:AUDIO SUBTYPE:NONE FRAMES:4 PREGAP:2"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rd.Close()

	tracks := rd.Tracks()
	if len(tracks) != 2 {
		t.Fatalf("tracks = %d, want 2", len(tracks))
	}
	if tracks[1].StartLBA() != 3 || tracks[1].Pregap != 2 || tracks[1].chdStart != 4 {
		t.Fatalf("track 2 start LBA %d, pregap %d, image frame %d, want 3, 2, 4",
			tracks[1].StartLBA(), tracks[1].Pregap, tracks[1].chdStart)
	}
}

// TestOpenMetadataChain tests the limits on the metadata chain walk.
func TestOpenMetadataChain(t *testing.T) {
	t.Run("forward chain", func(t *testing.T) {
		rd, err := openTestCHD(t, metaTestCHD(3, trackLines(3)...))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer rd.Close()
		if n := len(rd.Tracks()); n != 3 {
			t.Fatalf("tracks = %d, want 3", n)
		}
	})

	t.Run("99 tracks", func(t *testing.T) {
		rd, err := openTestCHD(t, metaTestCHD(0, trackLines(99)...))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		rd.Close()
	})

	t.Run("link to itself", func(t *testing.T) {
		img := metaTestCHD(0, trackLines(1)...).build()
		meta := binary.BigEndian.Uint64(img[48:56])
		binary.BigEndian.PutUint64(img[meta+8:meta+16], meta)
		if _, err := openImage(img); err == nil {
			t.Fatal("expected an error for a metadata entry linking to itself")
		}
	})

	t.Run("link backward", func(t *testing.T) {
		img := metaTestCHD(0, trackLines(2)...).build()
		first := binary.BigEndian.Uint64(img[48:56])
		second := binary.BigEndian.Uint64(img[first+8 : first+16])
		binary.BigEndian.PutUint64(img[second+8:second+16], first)
		if _, err := openImage(img); err == nil {
			t.Fatal("expected an error for a metadata entry linking backward")
		}
	})

	t.Run("too many tracks", func(t *testing.T) {
		if _, err := openTestCHD(t, metaTestCHD(0, trackLines(100)...)); err == nil {
			t.Fatal("expected an error for 100 track entries")
		}
	})

	t.Run("too many entries", func(t *testing.T) {
		if _, err := openTestCHD(t, metaTestCHD(maxMetaEntries, trackLines(1)...)); err == nil {
			t.Fatalf("expected an error for %d metadata entries", maxMetaEntries+1)
		}
	})

	t.Run("track entry too long", func(t *testing.T) {
		line := trackLines(1)[0] + strings.Repeat(" ", maxTrackMetaLen)
		if _, err := openTestCHD(t, metaTestCHD(0, line)); err == nil {
			t.Fatal("expected an error for an oversized track entry")
		}
	})
}

// TestOpenHeaderSizes tests the limits on the hunk size, unit size and
// logical size. The images hold no hunks, so only the header is checked.
func TestOpenHeaderSizes(t *testing.T) {
	cases := []struct {
		name         string
		hunkBytes    uint32
		unitBytes    uint32
		logicalBytes uint64
		wantOK       bool
	}{
		{"one frame per hunk", cdFrameSize, cdFrameSize, 0, true},
		{"largest hunk", maxHunkBytes / cdFrameSize * cdFrameSize, cdFrameSize, 0, true},
		{"zero hunk size", 0, cdFrameSize, 0, false},
		{"hunk smaller than a frame", 1, cdFrameSize, 0, false},
		{"hunk one byte short of a frame", cdFrameSize - 1, cdFrameSize, 0, false},
		{"hunk not whole frames", cdFrameSize + 1, cdFrameSize, 0, false},
		{"hunk over 1 MiB", (maxHunkBytes/cdFrameSize + 1) * cdFrameSize, cdFrameSize, 0, false},
		{"zero unit size", cdFrameSize, 0, 0, false},
		{"unit size not a frame", 2 * cdFrameSize, 2 * cdFrameSize, 0, false},
		{"logical size not whole frames", cdFrameSize, cdFrameSize, cdFrameSize + 1, false},
		{"logical size too large", cdFrameSize, cdFrameSize, (maxImageFrames + 1) * cdFrameSize, false},
		{"logical size wraps the hunk count", cdFrameSize, cdFrameSize, 0xFFFFFFFF00000000, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rd, err := openTestCHD(t, testCHD{
				codecs:       [4]uint32{codecCDZS},
				hunkBytes:    tc.hunkBytes,
				unitBytes:    tc.unitBytes,
				logicalBytes: tc.logicalBytes,
				lengthBits:   24,
				selfBits:     24,
				parentBits:   24,
			})
			if !tc.wantOK {
				if err == nil {
					rd.Close()
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			rd.Close()
		})
	}
}

// mapTestCHD is an image of the given hunks with 8-frame hunks, 24-bit map
// fields and one track covering every hunk.
func mapTestCHD(hunks ...testHunk) testCHD {
	return testCHD{
		codecs:     [4]uint32{codecCDZS},
		hunkBytes:  testFramesPerHunk * cdFrameSize,
		unitBytes:  cdFrameSize,
		lengthBits: 24,
		selfBits:   24,
		parentBits: 24,
		hunks:      hunks,
		tracks:     []string{fmt.Sprintf("TRACK:1 TYPE:MODE1_RAW SUBTYPE:NONE FRAMES:%d", len(hunks)*testFramesPerHunk)},
	}
}

// rawHunk is an uncompressed hunk filled with fill.
func rawHunk(fill byte) testHunk {
	return testHunk{comp: compNone, data: bytes.Repeat([]byte{fill}, testFramesPerHunk*cdFrameSize)}
}

// TestOpenMapFields tests the limits on the map's bit widths, its size and
// the hunk data ranges it describes.
func TestOpenMapFields(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		rd, err := openTestCHD(t, mapTestCHD(rawHunk(1), rawHunk(2)))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		rd.Close()
	})

	for _, field := range []string{"length", "self", "parent"} {
		t.Run(field+" bits over 24", func(t *testing.T) {
			c := mapTestCHD(rawHunk(1))
			switch field {
			case "length":
				c.lengthBits = 25
			case "self":
				c.selfBits = 25
			case "parent":
				c.parentBits = 25
			}
			if _, err := openTestCHD(t, c); err == nil {
				t.Fatal("expected an error")
			}
		})
	}

	t.Run("map offset past the file", func(t *testing.T) {
		img := mapTestCHD(rawHunk(1)).build()
		binary.BigEndian.PutUint64(img[40:48], uint64(len(img)))
		if _, err := openImage(img); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("map length past the file", func(t *testing.T) {
		img := mapTestCHD(rawHunk(1)).build()
		mapOffset := binary.BigEndian.Uint64(img[40:48])
		binary.BigEndian.PutUint32(img[mapOffset:mapOffset+4], 0xFFFFFFFF)
		if _, err := openImage(img); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("hunk data past the file", func(t *testing.T) {
		img := mapTestCHD(rawHunk(1)).build()
		mapOffset := binary.BigEndian.Uint64(img[40:48])
		// Move the first hunk's data offset to the end of the file.
		first := uint64(len(img))
		for i := range 6 {
			img[mapOffset+4+uint64(i)] = byte(first >> uint(40-8*i))
		}
		if _, err := openImage(img); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("compressed length past the file", func(t *testing.T) {
		c := mapTestCHD(testHunk{comp: compCodec0, data: []byte{0}})
		img := c.build()
		// Grow the hunk's 24-bit length field, which follows the 64-bit
		// Huffman tree and the 4-bit type, to its maximum.
		mapOffset := binary.BigEndian.Uint64(img[40:48])
		bits := img[mapOffset+16:]
		var bw bitWriter
		for range 16 {
			bw.write(4, 4)
		}
		bw.write(compCodec0, 4)
		bw.write(0xFFFFFF, 24)
		bw.write(0, 16)
		copy(bits, bw.buf)
		if _, err := openImage(img); err == nil {
			t.Fatal("expected an error")
		}
	})
}

// TestOpenSelfReferences tests that a hunk may only refer to an earlier
// hunk, and that such a reference reads the earlier hunk's data.
func TestOpenSelfReferences(t *testing.T) {
	t.Run("refers to an earlier hunk", func(t *testing.T) {
		rd, err := openTestCHD(t, mapTestCHD(rawHunk(0x33), testHunk{comp: compSelf, offset: 0}))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer rd.Close()
		sector, err := rd.ReadSector(testFramesPerHunk)
		if err != nil {
			t.Fatalf("ReadSector: %v", err)
		}
		if sector[0] != 0x33 {
			t.Fatalf("sector[0] = %#x, want 0x33", sector[0])
		}
	})

	t.Run("chain resolves to the stored hunk", func(t *testing.T) {
		rd, err := openTestCHD(t, mapTestCHD(rawHunk(0x44),
			testHunk{comp: compSelf, offset: 0},
			testHunk{comp: compSelf, offset: 1}))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer rd.Close()
		if rd.entries[2].offset != 0 {
			t.Fatalf("hunk 2 refers to hunk %d, want 0", rd.entries[2].offset)
		}
		sector, err := rd.ReadSector(2 * testFramesPerHunk)
		if err != nil {
			t.Fatalf("ReadSector: %v", err)
		}
		if sector[0] != 0x44 {
			t.Fatalf("sector[0] = %#x, want 0x44", sector[0])
		}
	})

	// The stored bytes of a referenced hunk are shared, but each hunk is
	// returned converted for its own track: audio samples swapped to
	// little-endian, data byte-exact.
	for _, tc := range []struct {
		name      string
		refType   string
		selfType  string
		refFirst  byte // first byte read from the referenced hunk
		selfFirst byte // first byte read from the self reference
	}{
		{"audio hunk referenced from data", "AUDIO", "MODE1_RAW", 0x22, 0x11},
		{"data hunk referenced from audio", "MODE1_RAW", "AUDIO", 0x11, 0x22},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored := bytes.Repeat([]byte{0x11, 0x22}, testFramesPerHunk*cdFrameSize/2)
			c := mapTestCHD(testHunk{comp: compNone, data: stored}, testHunk{comp: compSelf, offset: 0})
			c.tracks = []string{
				"TRACK:1 TYPE:" + tc.refType + " SUBTYPE:NONE FRAMES:8",
				"TRACK:2 TYPE:" + tc.selfType + " SUBTYPE:NONE FRAMES:8",
			}
			rd, err := openTestCHD(t, c)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer rd.Close()

			ref, err := rd.ReadSector(0)
			if err != nil {
				t.Fatalf("ReadSector(0): %v", err)
			}
			if ref[0] != tc.refFirst {
				t.Fatalf("referenced hunk byte 0 = %#x, want %#x", ref[0], tc.refFirst)
			}
			self, err := rd.ReadSector(testFramesPerHunk)
			if err != nil {
				t.Fatalf("ReadSector(%d): %v", testFramesPerHunk, err)
			}
			if self[0] != tc.selfFirst {
				t.Fatalf("self reference byte 0 = %#x, want %#x", self[0], tc.selfFirst)
			}
		})
	}

	t.Run("refers to itself", func(t *testing.T) {
		if _, err := openTestCHD(t, mapTestCHD(testHunk{comp: compSelf, offset: 0})); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("refers to a later hunk", func(t *testing.T) {
		if _, err := openTestCHD(t, mapTestCHD(testHunk{comp: compSelf, offset: 1}, rawHunk(1))); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("cycle", func(t *testing.T) {
		if _, err := openTestCHD(t, mapTestCHD(testHunk{comp: compSelf, offset: 1}, testHunk{comp: compSelf, offset: 0})); err == nil {
			t.Fatal("expected an error")
		}
	})
}

// TestOpenMapCRC tests that a map whose decoded entries do not match its
// CRC is rejected.
func TestOpenMapCRC(t *testing.T) {
	img := mapTestCHD(rawHunk(1), rawHunk(2)).build()
	mapOffset := binary.BigEndian.Uint64(img[40:48])
	img[mapOffset+10] ^= 0xFF
	if _, err := openImage(img); err == nil {
		t.Fatal("expected an error for a wrong map CRC")
	}
}

// TestReadSectorHunkCRC tests that a hunk whose data does not match the
// CRC in its map entry is not returned.
func TestReadSectorHunkCRC(t *testing.T) {
	img := mapTestCHD(rawHunk(1)).build()
	// Change the stored hunk data, which starts right after the header.
	img[v5HeaderSize] ^= 0xFF
	rd, err := openImage(img)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rd.Close()
	if _, err := rd.ReadSector(0); err == nil {
		t.Fatal("expected an error for a hunk not matching its CRC")
	}
}

// TestReadSectorCDFLCRC tests that a cdfl hunk's CRC is checked over the
// sector data as stored, with big-endian samples, and that the audio is then
// returned with little-endian samples.
func TestReadSectorCDFLCRC(t *testing.T) {
	samples := testFramesPerHunk * cdSectorSize / 4
	audio := flacStereoConstant(samples, 0x1234, -2)
	subcode := make([]byte, testFramesPerHunk*cdSubcodeSize)
	hunk := append(append([]byte{}, audio...), deflateRaw(t, subcode)...)

	stored := bytes.Repeat([]byte{0x12, 0x34, 0xFF, 0xFE}, samples)

	rd, err := openTestCHD(t, testCHD{
		codecs:     [4]uint32{codecCDFL},
		hunkBytes:  testFramesPerHunk * cdFrameSize,
		unitBytes:  cdFrameSize,
		lengthBits: 24,
		selfBits:   24,
		parentBits: 24,
		hunks:      []testHunk{{comp: compCodec0, data: hunk, decoded: cdFrames(stored, subcode)}},
		tracks:     []string{"TRACK:1 TYPE:AUDIO SUBTYPE:NONE FRAMES:8"},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rd.Close()

	sector, err := rd.ReadSector(0)
	if err != nil {
		t.Fatalf("ReadSector: %v", err)
	}
	if !bytes.Equal(sector[:4], []byte{0x34, 0x12, 0xFE, 0xFF}) {
		t.Fatalf("first sample pair = % X, want 34 12 FE FF", sector[:4])
	}
}

// TestOpenHeaderLength tests that a v5 header must declare the v5 header
// length.
func TestOpenHeaderLength(t *testing.T) {
	for _, headerLen := range []uint32{1, v5HeaderSize - 1, v5HeaderSize + 1, 0xFFFFFFFF} {
		_, err := openTestCHD(t, testCHD{
			headerLen:  headerLen,
			codecs:     [4]uint32{codecCDZS},
			hunkBytes:  testFramesPerHunk * cdFrameSize,
			unitBytes:  cdFrameSize,
			lengthBits: 24,
			selfBits:   24,
			parentBits: 24,
			hunks:      []testHunk{{comp: compNone, data: make([]byte, testFramesPerHunk*cdFrameSize)}},
			tracks:     []string{"TRACK:1 TYPE:MODE1_RAW SUBTYPE:NONE FRAMES:8"},
		})
		if err == nil {
			t.Errorf("header length %d: expected an error", headerLen)
		}
	}
}
