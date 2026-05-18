package romloader

import (
	"os"

	"github.com/user-none/eblitui/romloader/chd"
)

// Disc is a streaming reader over a CHD disc image. It owns the open file
// handle and decodes hunks on demand, so the whole image is never read
// into memory. Its method set uses only stdlib types so it satisfies a
// disc-reader interface structurally without a shared type.
type Disc struct {
	f      *os.File
	rd     *chd.Reader
	tracks []chd.Track
}

// OpenDisc opens a CHD disc image for streaming. The file handle is held
// open for on-demand hunk reads until Close is called.
func OpenDisc(path string) (*Disc, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	rd, err := chd.Open(f, fi.Size())
	if err != nil {
		f.Close()
		return nil, err
	}
	return &Disc{f: f, rd: rd, tracks: rd.Tracks()}, nil
}

// ReadSector returns the raw 2352-byte sector at the given LBA.
func (d *Disc) ReadSector(lba int) ([]byte, error) {
	return d.rd.ReadSector(lba)
}

// NumTracks returns the number of tracks on the disc.
func (d *Disc) NumTracks() int {
	return len(d.tracks)
}

// Track returns the TOC fields for track index i in [0, NumTracks).
func (d *Disc) Track(i int) (number int, typ string, frames int, pregap int, startLBA int, control uint8) {
	t := d.tracks[i]
	return t.Number, t.Type, t.Frames, t.Pregap, t.StartLBA(), t.Control
}

// Close releases the CHD decoder and the underlying file handle.
func (d *Disc) Close() error {
	d.rd.Close()
	return d.f.Close()
}
