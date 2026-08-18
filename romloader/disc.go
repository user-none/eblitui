// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package romloader

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
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

// discOpeners maps a lowercase file extension (with leading dot) to the backend
// opener for that disc format. It is the single place the set of supported disc
// image formats is defined: OpenDisc dispatches through it and DiscExtensions
// reports its keys, so the two cannot drift.
var discOpeners = map[string]func(path string) (discBackend, error){
	".chd": openCHD,
	".cue": openBinCue,
}

// DiscExtensions returns the lowercase file extensions OpenDisc accepts, each
// with a leading dot (".chd", ".cue"), sorted ascending. Callers that scan a
// directory or filter a file list use it to discover which files are disc images
// without hardcoding the set. Unlike ROM images, where the core declares the
// extensions it understands, the disc formats are owned by romloader: a core
// reads a disc through a format-agnostic interface and never sees the file, so
// romloader is the only component that knows which container formats it can open.
func DiscExtensions() []string {
	exts := make([]string, 0, len(discOpeners))
	for ext := range discOpeners {
		exts = append(exts, ext)
	}
	sort.Strings(exts)
	return exts
}

// OpenDisc opens a disc image for streaming, selected by extension: ".chd" is
// opened as a CHD image, ".cue" as a cue sheet whose referenced bin track files
// are opened and held for the disc's lifetime. Any other extension returns
// ErrUnsupportedDisc. File handles are held open for on-demand sector reads
// until Close is called.
func OpenDisc(path string) (*Disc, error) {
	open, ok := discOpeners[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return nil, ErrUnsupportedDisc
	}
	b, err := open(path)
	if err != nil {
		return nil, err
	}
	return &Disc{backend: b}, nil
}

// ReadSector returns the raw 2352-byte sector at the given LBA.
func (d *Disc) ReadSector(lba int) ([]byte, error) {
	return d.backend.ReadSector(lba)
}

// Sector geometry used to cook a raw 2352-byte sector into user data. A Mode 1
// sector carries 2048 user bytes after a 16-byte header (12 sync + 4 header); a
// Mode 2 (Form 1) sector after a 24-byte header (the extra 8 bytes are the
// subheader); an audio sector has no header and is data throughout.
const (
	cookedUserDataSize = 2048
	mode1HeaderSize    = 16
	mode2HeaderSize    = 24
)

// ReadSectorData returns the cooked user data of the sector at the given
// absolute LBA: the 2048-byte user-data region of a Mode 1 or Mode 2 (Form 1)
// data sector with the sync/header (and Mode 2 subheader) stripped. Audio
// sectors have no header and are returned whole (the full 2352 bytes). Use it
// when a consumer identifies a disc by its volume/boot headers rather than raw
// sector bytes; ReadSector remains the raw view.
func (d *Disc) ReadSectorData(lba int) ([]byte, error) {
	raw, err := d.ReadSector(lba)
	if err != nil {
		return nil, err
	}
	header, audio := d.sectorHeader(lba)
	if audio {
		return raw, nil
	}
	if len(raw) < header+cookedUserDataSize {
		return nil, fmt.Errorf("romloader: LBA %d: sector too small for user data", lba)
	}
	return raw[header : header+cookedUserDataSize], nil
}

// sectorHeader returns the header byte count to strip for the track that
// contains the absolute lba, and whether that track is audio (no header; the
// whole sector is data). The header size follows the track's mode: Mode 1 uses
// 16 bytes, Mode 2 uses 24. An lba below the first track defaults to Mode 1.
func (d *Disc) sectorHeader(lba int) (header int, audio bool) {
	for i := d.NumTracks() - 1; i >= 0; i-- {
		_, typ, _, _, startLBA, _ := d.Track(i)
		if lba < startLBA {
			continue
		}
		switch {
		case strings.HasPrefix(typ, "AUDIO"):
			return 0, true
		case strings.Contains(typ, "MODE2"):
			return mode2HeaderSize, false
		default:
			return mode1HeaderSize, false
		}
	}
	return mode1HeaderSize, false
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
