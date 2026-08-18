// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package romloader

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// indexEntry is one exposed index (number >= 1) at an absolute disc LBA.
type indexEntry struct {
	number int
	lba    int
}

// track is one laid-out disc track. Its LBA span is, in order: leadSilence
// generated sectors (from a PREGAP command), fileFrames sectors read from fd at
// fileOffset, then trailSilence generated sectors (from a POSTGAP command).
// frames is the sum of the three. fd is borrowed from binCueBackend.fds; the track
// never owns or names the file.
type track struct {
	number  int
	typ     string
	control uint8

	startLBA int
	frames   int
	pregap   int // total pregap (generated + in-file), == body LBA - startLBA

	leadSilence  int
	fileFrames   int
	fileOffset   int64
	trailSilence int
	fd           *os.File

	indexes []indexEntry // index numbers >= 1, absolute LBA, ascending
}

// binCueBackend is a disc backend over a cue sheet's raw bin files. fds owns every
// open handle (one per distinct bin filename); it is the single source of truth
// for closing. Each track borrows an fd pointer from it.
type binCueBackend struct {
	filename string // the .cue path
	fds      map[string]*os.File
	tracks   []track
}

// buildTrack is parser-local scratch: the per-track data gathered while reading
// the cue, before file sizes and neighbours are known. It is never stored on the
// finished track.
type buildTrack struct {
	number  int
	typ     string
	control uint8
	fd      *os.File
	indexes []cueIndex // file-relative, as parsed
	pregap  int        // PREGAP command frames
	postgap int        // POSTGAP command frames
}

// openBinCue parses a cue sheet and builds the backend, opening each referenced
// bin file once and computing the disc layout. On any failure every opened
// handle is closed before returning.
func openBinCue(path string) (discBackend, error) {
	bc := &binCueBackend{filename: path, fds: make(map[string]*os.File)}
	closeAll := func() {
		for _, fd := range bc.fds {
			fd.Close()
		}
	}

	cf, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer cf.Close()

	cueDir := filepath.Dir(path)
	var curFd *os.File
	var built []buildTrack

	sc := bufio.NewScanner(cf)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		keyword, rest := splitKeyword(line)
		switch strings.ToUpper(keyword) {
		case "FILE":
			filename, ftype := parseFileLine(rest)
			if filename == "" {
				closeAll()
				return nil, fmt.Errorf("cue: malformed FILE directive (line %d)", lineNo)
			}
			if !strings.EqualFold(ftype, "BINARY") {
				closeAll()
				return nil, fmt.Errorf("cue: unsupported FILE type %q (line %d)", ftype, lineNo)
			}
			fd, ok := bc.fds[filename]
			if !ok {
				resolved, err := resolveTrackPath(cueDir, filename)
				if err != nil {
					closeAll()
					return nil, err
				}
				fd, err = os.Open(resolved)
				if err != nil {
					closeAll()
					return nil, fmt.Errorf("cue: opening track file %q: %w", filename, err)
				}
				bc.fds[filename] = fd
			}
			curFd = fd

		case "TRACK":
			if curFd == nil {
				closeAll()
				return nil, fmt.Errorf("cue: TRACK before any FILE (line %d)", lineNo)
			}
			fields := strings.Fields(rest)
			if len(fields) < 2 {
				closeAll()
				return nil, fmt.Errorf("cue: malformed TRACK directive (line %d)", lineNo)
			}
			num, err := strconv.Atoi(fields[0])
			if err != nil {
				closeAll()
				return nil, fmt.Errorf("cue: bad TRACK number %q (line %d)", fields[0], lineNo)
			}
			typ, control, err := normalizeTrackType(fields[1])
			if err != nil {
				closeAll()
				return nil, fmt.Errorf("cue: %w (line %d)", err, lineNo)
			}
			built = append(built, buildTrack{number: num, typ: typ, control: control, fd: curFd})

		case "INDEX":
			if len(built) == 0 {
				closeAll()
				return nil, fmt.Errorf("cue: INDEX before any TRACK (line %d)", lineNo)
			}
			fields := strings.Fields(rest)
			if len(fields) < 2 {
				closeAll()
				return nil, fmt.Errorf("cue: malformed INDEX directive (line %d)", lineNo)
			}
			num, err := strconv.Atoi(fields[0])
			if err != nil {
				closeAll()
				return nil, fmt.Errorf("cue: bad INDEX number %q (line %d)", fields[0], lineNo)
			}
			frame, err := parseMSF(fields[1])
			if err != nil {
				closeAll()
				return nil, fmt.Errorf("cue: %w (line %d)", err, lineNo)
			}
			bt := &built[len(built)-1]
			bt.indexes = append(bt.indexes, cueIndex{number: num, frame: frame})

		case "PREGAP", "POSTGAP":
			if len(built) == 0 {
				closeAll()
				return nil, fmt.Errorf("cue: %s before any TRACK (line %d)", keyword, lineNo)
			}
			frame, err := parseMSF(strings.TrimSpace(rest))
			if err != nil {
				closeAll()
				return nil, fmt.Errorf("cue: %w (line %d)", err, lineNo)
			}
			bt := &built[len(built)-1]
			if strings.EqualFold(keyword, "PREGAP") {
				bt.pregap = frame
			} else {
				bt.postgap = frame
			}
		}
	}
	if err := sc.Err(); err != nil {
		closeAll()
		return nil, fmt.Errorf("cue: reading sheet: %w", err)
	}
	if len(built) == 0 {
		closeAll()
		return nil, fmt.Errorf("cue: no tracks")
	}

	tracks, err := layoutTracks(built)
	if err != nil {
		closeAll()
		return nil, err
	}
	bc.tracks = tracks
	return bc, nil
}

// layoutTracks turns the parsed scratch tracks into laid-out tracks: absolute
// LBA, file offsets, pregap accounting, and exposed index entries. A track's
// in-file length runs to the next track in the same file, or to end-of-file.
func layoutTracks(built []buildTrack) ([]track, error) {
	// Validate up front that every track has at least one INDEX, so the
	// same-file look-ahead below (minIndexFrame on the next track) and the
	// firstFrame access are always safe.
	for i := range built {
		if len(built[i].indexes) == 0 {
			return nil, fmt.Errorf("cue: track %d has no INDEX", built[i].number)
		}
	}

	tracks := make([]track, len(built))
	runningLBA := 0
	for i := range built {
		bt := &built[i]
		sort.Slice(bt.indexes, func(a, b int) bool { return bt.indexes[a].number < bt.indexes[b].number })
		firstFrame := bt.indexes[0].frame

		fi, err := bt.fd.Stat()
		if err != nil {
			return nil, err
		}
		if fi.Size()%cueSectorBytes != 0 {
			return nil, fmt.Errorf("cue: track %d file size %d is not a multiple of %d", bt.number, fi.Size(), cueSectorBytes)
		}
		fileSectors := int(fi.Size() / cueSectorBytes)
		if firstFrame > fileSectors {
			return nil, fmt.Errorf("cue: track %d INDEX position %d beyond file end %d", bt.number, firstFrame, fileSectors)
		}

		var fileFrames int
		if i+1 < len(built) && built[i+1].fd == bt.fd {
			fileFrames = minIndexFrame(built[i+1].indexes) - firstFrame
		} else {
			fileFrames = fileSectors - firstFrame
		}
		if fileFrames <= 0 {
			return nil, fmt.Errorf("cue: track %d has no data (check INDEX positions and file size)", bt.number)
		}

		inFilePregap := 0
		if bt.indexes[0].number == 0 {
			if i01, ok := indexFrame(bt.indexes, 1); ok {
				inFilePregap = i01 - bt.indexes[0].frame
			}
		}

		t := track{
			number:       bt.number,
			typ:          bt.typ,
			control:      bt.control,
			startLBA:     runningLBA,
			pregap:       bt.pregap + inFilePregap,
			leadSilence:  bt.pregap,
			fileFrames:   fileFrames,
			fileOffset:   int64(firstFrame) * cueSectorBytes,
			trailSilence: bt.postgap,
			fd:           bt.fd,
		}
		t.frames = t.leadSilence + t.fileFrames + t.trailSilence

		for _, idx := range bt.indexes {
			if idx.number < 1 {
				continue
			}
			abs := t.startLBA + t.leadSilence + (idx.frame - firstFrame)
			t.indexes = append(t.indexes, indexEntry{number: idx.number, lba: abs})
		}
		if len(t.indexes) == 0 {
			n, lba := synthIndex01(t.startLBA, t.pregap)
			t.indexes = append(t.indexes, indexEntry{number: n, lba: lba})
		}

		tracks[i] = t
		runningLBA += t.frames
	}
	return tracks, nil
}

// resolveTrackPath turns a track's bin filename (from a FILE directive) into a
// full path under the cue's directory. The bin filename must be relative:
// absolute paths and parent-directory traversal are rejected so a cue can only
// reference files within its own directory tree.
func resolveTrackPath(cueDir, filename string) (string, error) {
	// Cue sheets may use backslashes; treat them as separators.
	if filename == "" {
		return "", fmt.Errorf("cue: empty FILE name")
	}
	norm := strings.ReplaceAll(filename, "\\", "/")
	if filepath.IsAbs(norm) || strings.HasPrefix(norm, "/") {
		return "", fmt.Errorf("cue: FILE %q must be a relative path", filename)
	}
	for _, seg := range strings.Split(norm, "/") {
		if seg == ".." {
			return "", fmt.Errorf("cue: FILE %q must not traverse parent directories", filename)
		}
	}
	return filepath.Join(cueDir, filepath.FromSlash(norm)), nil
}

// minIndexFrame returns the lowest-numbered index's file-relative frame.
func minIndexFrame(idx []cueIndex) int {
	min := idx[0].frame
	minNum := idx[0].number
	for _, e := range idx {
		if e.number < minNum {
			minNum = e.number
			min = e.frame
		}
	}
	return min
}

// indexFrame returns the file-relative frame of the index with the given number.
func indexFrame(idx []cueIndex, number int) (int, bool) {
	for _, e := range idx {
		if e.number == number {
			return e.frame, true
		}
	}
	return 0, false
}

func (b *binCueBackend) ReadSector(lba int) ([]byte, error) {
	for i := len(b.tracks) - 1; i >= 0; i-- {
		t := &b.tracks[i]
		if lba < t.startLBA {
			continue
		}
		off := lba - t.startLBA
		if off >= t.frames {
			return nil, fmt.Errorf("cue: LBA %d beyond track %d", lba, t.number)
		}
		buf := make([]byte, cueSectorBytes)
		if off < t.leadSilence {
			return buf, nil // generated leading silence
		}
		off -= t.leadSilence
		if off < t.fileFrames {
			byteOff := t.fileOffset + int64(off)*cueSectorBytes
			if _, err := t.fd.ReadAt(buf, byteOff); err != nil {
				return nil, fmt.Errorf("cue: reading LBA %d (track %d): %w", lba, t.number, err)
			}
			return buf, nil
		}
		return buf, nil // generated trailing silence
	}
	return nil, fmt.Errorf("cue: LBA %d before first track", lba)
}

func (b *binCueBackend) NumTracks() int { return len(b.tracks) }

func (b *binCueBackend) Track(i int) (number int, typ string, frames int, pregap int, startLBA int, control uint8) {
	t := b.tracks[i]
	return t.number, t.typ, t.frames, t.pregap, t.startLBA, t.control
}

func (b *binCueBackend) NumTrackIndexes(i int) int {
	return len(b.tracks[i].indexes)
}

func (b *binCueBackend) TrackIndex(i, n int) (indexNumber int, lba int) {
	e := b.tracks[i].indexes[n]
	return e.number, e.lba
}

// Close closes every open bin file. fds holds one handle per distinct filename,
// so iterating it closes each exactly once.
func (b *binCueBackend) Close() error {
	var first error
	for _, fd := range b.fds {
		if err := fd.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
