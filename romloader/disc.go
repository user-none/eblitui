package romloader

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrUnsupportedDisc is returned by OpenDisc when the path's extension is
// neither .chd nor .cue.
var ErrUnsupportedDisc = errors.New("romloader: unsupported disc format (expected .chd or .cue)")

// discBackend is the internal contract a concrete disc format implements. The
// public Disc delegates to it. Every signature uses only stdlib types and no
// named aggregate, matching the structural disc-reader interface consumers
// expect.
type discBackend interface {
	ReadSector(lba int) ([]byte, error)
	NumTracks() int
	Track(i int) (number int, typ string, frames int, pregap int, startLBA int, control uint8)
	NumTrackIndexes(i int) int
	TrackIndex(i, n int) (indexNumber int, lba int)
	Close() error
}

// Disc is a streaming reader over a disc image. It owns the open file handles
// and decodes sectors on demand, so the whole image is never read into memory.
// Internally it is a facade over a format-specific backend (CHD or bin/cue);
// OpenDisc selects the backend. Its method set uses only stdlib types so it
// satisfies a disc-reader interface structurally without a shared type.
type Disc struct {
	backend discBackend
}

// OpenDisc opens a disc image for streaming, selected by extension: ".chd" is
// opened as a CHD image, ".cue" as a cue sheet whose referenced bin track files
// are opened and held for the disc's lifetime. Any other extension returns
// ErrUnsupportedDisc. File handles are held open for on-demand sector reads
// until Close is called.
func OpenDisc(path string) (*Disc, error) {
	var b discBackend
	var err error
	switch strings.ToLower(filepath.Ext(path)) {
	case ".chd":
		b, err = openCHD(path)
	case ".cue":
		b, err = openBinCue(path)
	default:
		return nil, ErrUnsupportedDisc
	}
	if err != nil {
		return nil, err
	}
	return &Disc{backend: b}, nil
}

// ReadSector returns the raw 2352-byte sector at the given LBA.
func (d *Disc) ReadSector(lba int) ([]byte, error) {
	return d.backend.ReadSector(lba)
}

// NumTracks returns the number of tracks on the disc.
func (d *Disc) NumTracks() int {
	return d.backend.NumTracks()
}

// Track returns the TOC fields for track index i in [0, NumTracks).
func (d *Disc) Track(i int) (number int, typ string, frames int, pregap int, startLBA int, control uint8) {
	return d.backend.Track(i)
}

// NumTrackIndexes returns the count of index entries (index numbers >= 1)
// exposed for track index i in [0, NumTracks). Index 0 (the pregap) is never
// reported here; it is implied for any FAD below the first entry.
func (d *Disc) NumTrackIndexes(i int) int {
	return d.backend.NumTrackIndexes(i)
}

// TrackIndex returns the nth index entry of track index i. n is a 0-based
// ordinal into the exposed list in [0, NumTrackIndexes(i)), not the index
// number; entry 0 is the lowest-numbered exposed index (normally INDEX 01). The
// returned lba is the absolute disc LBA of the index.
func (d *Disc) TrackIndex(i, n int) (indexNumber int, lba int) {
	return d.backend.TrackIndex(i, n)
}

// Close releases the decoder and all underlying file handles.
func (d *Disc) Close() error {
	return d.backend.Close()
}

// synthIndex01 returns the single implicit-floor index entry for a track with no
// explicit index map: INDEX 01 at the body start (startLBA + pregap). Shared by
// the CHD backend (always) and the bin/cue backend (for a track whose cue block
// carries no INDEX 01).
func synthIndex01(startLBA, pregap int) (indexNumber int, lba int) {
	return 1, startLBA + pregap
}
