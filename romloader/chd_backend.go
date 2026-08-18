// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package romloader

import (
	"fmt"
	"os"

	"github.com/user-none/eblitui/romloader/chd"
)

// chdBackend adapts the streaming CHD reader to discBackend. The CHD metadata
// carries no INDEX map, so the index accessors synthesize a single INDEX 01 per
// track from the pregap.
type chdBackend struct {
	f      *os.File
	rd     *chd.Reader
	tracks []chd.Track
}

func openCHD(path string) (discBackend, error) {
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
	return &chdBackend{f: f, rd: rd, tracks: rd.Tracks()}, nil
}

func (b *chdBackend) ReadSector(lba int) ([]byte, error) { return b.rd.ReadSector(lba) }

func (b *chdBackend) NumTracks() int { return len(b.tracks) }

func (b *chdBackend) Track(i int) (number int, typ string, frames int, pregap int, startLBA int, control uint8) {
	t := b.tracks[i]
	return t.Number, t.Type, t.Frames, t.Pregap, t.StartLBA(), t.Control
}

func (b *chdBackend) NumTrackIndexes(i int) int {
	_ = b.tracks[i] // bounds check, parity with Track(i)
	return 1
}

func (b *chdBackend) TrackIndex(i, n int) (indexNumber int, lba int) {
	t := b.tracks[i]
	if n != 0 {
		panic(fmt.Sprintf("romloader: track index %d out of range", n))
	}
	return synthIndex01(t.StartLBA(), t.Pregap)
}

func (b *chdBackend) Close() error {
	b.rd.Close()
	return b.f.Close()
}
