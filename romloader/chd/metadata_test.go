package chd

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// buildMetaBlob lays out chained metadata nodes (16-byte header + payload) all
// tagged CHGD, starting at offset base, and returns the blob. Node headers are
// tag(4) | flagsAndLen(4) | nextOffset(8), all big-endian.
func buildMetaBlob(base int, payloads []string) []byte {
	offs := make([]int, len(payloads))
	pos := base
	for i, p := range payloads {
		offs[i] = pos
		pos += 16 + len(p)
	}
	blob := make([]byte, pos)
	for i, p := range payloads {
		o := offs[i]
		binary.BigEndian.PutUint32(blob[o:], metaTagCHGD)
		binary.BigEndian.PutUint32(blob[o+4:], uint32(len(p)))
		var next uint64
		if i+1 < len(payloads) {
			next = uint64(offs[i+1])
		}
		binary.BigEndian.PutUint64(blob[o+8:], next)
		copy(blob[o+16:], p)
	}
	return blob
}

// TestParseMetadataCHGD verifies GD-ROM CHGD records are parsed (the tag is
// recognized), the track types/frames come through, the PAD field is ignored,
// and startLBA is the cumulative frame count. Values are synthetic.
func TestParseMetadataCHGD(t *testing.T) {
	payloads := []string{
		"TRACK:1 TYPE:MODE1_RAW SUBTYPE:NONE FRAMES:300 PAD:5 PREGAP:0 PGTYPE:MODE1 PGSUB:NONE POSTGAP:0",
		"TRACK:2 TYPE:AUDIO SUBTYPE:NONE FRAMES:1000 PAD:777 PREGAP:0 PGTYPE:MODE1 PGSUB:NONE POSTGAP:0",
		"TRACK:3 TYPE:MODE1_RAW SUBTYPE:NONE FRAMES:2000 PAD:0 PREGAP:0 PGTYPE:MODE1 PGSUB:NONE POSTGAP:0",
	}
	blob := buildMetaBlob(16, payloads)
	rd := &Reader{r: bytes.NewReader(blob)}
	if err := rd.parseMetadata(16); err != nil {
		t.Fatal(err)
	}

	if len(rd.tracks) != 3 {
		t.Fatalf("tracks = %d, want 3", len(rd.tracks))
	}
	// PAD does not affect logical addressing: startLBA is cumulative frames.
	wantLBA := []int{0, 300, 1300}
	for i, tr := range rd.tracks {
		if tr.StartLBA() != wantLBA[i] {
			t.Errorf("track %d startLBA = %d, want %d", tr.Number, tr.StartLBA(), wantLBA[i])
		}
	}
	if rd.tracks[2].Type != "MODE1_RAW" || rd.tracks[2].Frames != 2000 {
		t.Errorf("track 3 = %s/%d, want MODE1_RAW/2000", rd.tracks[2].Type, rd.tracks[2].Frames)
	}
}
