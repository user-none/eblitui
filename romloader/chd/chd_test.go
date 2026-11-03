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
	comp   uint8  // map compression type (compCodec0-3, compNone, compSelf)
	data   []byte // stored bytes for codec and none types
	offset uint64 // referenced hunk for compSelf
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
	for _, h := range c.hunks {
		switch h.comp {
		case compCodec0, compCodec1, compCodec2, compCodec3:
			bw.write(uint64(len(h.data)), int(c.lengthBits))
			bw.write(0, 16)
		case compNone:
			bw.write(0, 16)
		case compSelf:
			bw.write(h.offset, int(c.selfBits))
		}
	}

	mapOffset := uint64(len(out))
	var mapHdr [16]byte
	binary.BigEndian.PutUint32(mapHdr[0:4], uint32(len(bw.buf)))
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
		hunks:      []testHunk{{comp: compCodec0, data: hunk}},
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
