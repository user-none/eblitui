package chd

import (
	"testing"
)

// twoTrackReader builds a Reader whose track table is a data track followed by
// an audio track, both 4-frame-aligned, with the audio track starting at the
// given CHD frame. Frames per hunk is fixed at 8 for the boundary tests. Values
// are synthetic.
func twoTrackReader(dataFrames, audioFrames, audioStart int) *Reader {
	return &Reader{
		framesPerHunk: 8,
		tracks: []Track{
			{Number: 1, Type: "MODE1_RAW", Frames: dataFrames, Control: 0x41, chdStart: 0, startLBA: 0},
			{Number: 2, Type: "AUDIO", Frames: audioFrames, Control: 0x01, chdStart: audioStart, startLBA: dataFrames},
		},
	}
}

func TestFrameTrackControl(t *testing.T) {
	// Data track has 5 real frames; 4-frame alignment pads it to 8, so the
	// audio track starts at CHD frame 8. Frames 5,6,7 are inter-track padding.
	rd := twoTrackReader(5, 4, 8)
	cases := []struct {
		frame   int
		ctrl    uint8
		inTrack bool
	}{
		{0, 0x41, true},  // data
		{4, 0x41, true},  // last data frame
		{5, 0, false},    // padding
		{7, 0, false},    // padding
		{8, 0x01, true},  // first audio frame
		{11, 0x01, true}, // last audio frame
		{12, 0, false},   // beyond all tracks
	}
	for _, c := range cases {
		ctrl, ok := rd.frameTrackControl(c.frame)
		if ok != c.inTrack || (ok && ctrl != c.ctrl) {
			t.Errorf("frameTrackControl(%d) = (%#x,%v), want (%#x,%v)", c.frame, ctrl, ok, c.ctrl, c.inTrack)
		}
	}
}

func TestFrameTrackControlEmpty(t *testing.T) {
	rd := &Reader{framesPerHunk: 8}
	if _, ok := rd.frameTrackControl(0); ok {
		t.Error("frameTrackControl on empty track table should report not-in-track")
	}
}

// fillFrames lays out a hunk of framesPerHunk frames, each sector filled with a
// per-frame pattern (even byte = frame, odd byte = frame|0x80) so a swap is
// detectable, and the subcode marked 0x5A so it can be checked as untouched.
func fillFrames(rd *Reader) []byte {
	hunk := make([]byte, rd.framesPerHunk*cdFrameSize)
	for f := 0; f < rd.framesPerHunk; f++ {
		base := f * cdFrameSize
		for j := 0; j < cdSectorSize; j += 2 {
			hunk[base+j] = byte(f)
			hunk[base+j+1] = byte(f) | 0x80
		}
		for j := cdSectorSize; j < cdFrameSize; j++ {
			hunk[base+j] = 0x5A
		}
	}
	return hunk
}

// checkFrames asserts each frame's sector is swapped iff swapped[f], and that
// no subcode byte changed.
func checkFrames(t *testing.T, rd *Reader, hunk []byte, swapped []bool) {
	t.Helper()
	for f := 0; f < rd.framesPerHunk; f++ {
		base := f * cdFrameSize
		for j := 0; j < cdSectorSize; j += 2 {
			lo, hi := hunk[base+j], hunk[base+j+1]
			wantLo, wantHi := byte(f), byte(f)|0x80
			if swapped[f] {
				wantLo, wantHi = byte(f)|0x80, byte(f)
			}
			if lo != wantLo || hi != wantHi {
				t.Fatalf("frame %d offset %d: got (%02x,%02x), want (%02x,%02x) swapped=%v",
					f, j, lo, hi, wantLo, wantHi, swapped[f])
			}
		}
		for j := cdSectorSize; j < cdFrameSize; j++ {
			if hunk[base+j] != 0x5A {
				t.Fatalf("frame %d subcode byte %d changed to %02x", f, j, hunk[base+j])
			}
		}
	}
}

// TestSwapFramesAudioBoundary covers the base-codec path on a hunk straddling
// the data/audio boundary: frames 0..3 are data, 4..7 are audio. swapFrames
// with audio=true must swap only the audio sectors (BE->LE), leaving data.
func TestSwapFramesAudioBoundary(t *testing.T) {
	rd := twoTrackReader(4, 4, 4)
	hunk := fillFrames(rd)
	rd.swapFrames(0, hunk, true)
	checkFrames(t, rd, hunk, []bool{false, false, false, false, true, true, true, true})
}

// TestSwapFramesDataBoundary covers the FLAC path on the same boundary hunk:
// FLAC decodes native little-endian, so audio is already canonical and only the
// data sectors must be swapped back. This is the regression case where chdman
// compresses a data hunk with CDFL.
func TestSwapFramesDataBoundary(t *testing.T) {
	rd := twoTrackReader(4, 4, 4)
	hunk := fillFrames(rd)
	rd.swapFrames(0, hunk, false)
	checkFrames(t, rd, hunk, []bool{true, true, true, true, false, false, false, false})
}

// TestSwapFramesAllDataHunk is the direct regression for the bug: an all-data
// hunk reached through the FLAC path (audio=false) must have every data sector
// swapped, matching what the old whole-buffer FLAC swap produced.
func TestSwapFramesAllDataHunk(t *testing.T) {
	// Single data track of 8 frames fills hunk 0.
	rd := &Reader{
		framesPerHunk: 8,
		tracks:        []Track{{Number: 1, Type: "MODE1_RAW", Frames: 8, Control: 0x41, chdStart: 0, startLBA: 0}},
	}
	hunk := fillFrames(rd)
	rd.swapFrames(0, hunk, false)
	checkFrames(t, rd, hunk, []bool{true, true, true, true, true, true, true, true})

	// The same hunk via the base path (audio=true) must leave data untouched.
	hunk2 := fillFrames(rd)
	rd.swapFrames(0, hunk2, true)
	checkFrames(t, rd, hunk2, []bool{false, false, false, false, false, false, false, false})
}

// TestSwapFramesSecondHunk verifies the absolute-frame offset across hunks: a
// data track filling hunk 0, an audio track filling hunk 1.
func TestSwapFramesSecondHunk(t *testing.T) {
	rd := twoTrackReader(8, 8, 8)

	// Hunk 0 is all data: audio=true swaps nothing.
	h0 := fillFrames(rd)
	rd.swapFrames(0, h0, true)
	checkFrames(t, rd, h0, make([]bool, 8))

	// Hunk 1 is all audio: audio=true swaps every sector.
	h1 := fillFrames(rd)
	rd.swapFrames(1, h1, true)
	allTrue := []bool{true, true, true, true, true, true, true, true}
	checkFrames(t, rd, h1, allTrue)
}

// TestSwapFramesPaddingUntouched confirms inter-track padding frames are never
// swapped in either mode.
func TestSwapFramesPaddingUntouched(t *testing.T) {
	// Data 5 frames (padded to 8), audio starts at frame 8 (beyond hunk 0).
	rd := twoTrackReader(5, 4, 8)
	hunk := fillFrames(rd)
	// audio=true: frames 5,6,7 are padding, 0-4 data -> nothing swapped.
	rd.swapFrames(0, hunk, true)
	checkFrames(t, rd, hunk, make([]bool, 8))
	// audio=false: data frames 0-4 swapped, padding 5-7 left alone.
	rd.swapFrames(0, hunk, false)
	checkFrames(t, rd, hunk, []bool{true, true, true, true, true, false, false, false})
}
