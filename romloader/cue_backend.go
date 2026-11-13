// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package romloader

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// indexEntry is one exposed index (number >= 1) at an absolute disc LBA.
type indexEntry struct {
	number int
	lba    int
}

// binFile is one open bin: random access to its bytes and its size. closer
// releases it and is nil when the bin holds nothing of its own to release.
type binFile struct {
	r      io.ReaderAt
	size   int64
	closer io.Closer
}

// track is one laid-out disc track. Its LBA span is, in order: leadSilence
// generated sectors (from a PREGAP command), fileFrames sectors read from bin at
// fileOffset, then trailSilence generated sectors (from a POSTGAP command).
// frames is the sum of the three. bin is borrowed from binCueBackend.bins; the track
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
	bin          *binFile

	indexes []indexEntry // index numbers >= 1, absolute LBA, ascending
}

// binCueBackend is a disc backend over a cue sheet's raw bin files. bins owns every
// open bin (one per distinct cleaned bin filename); it is the single source of truth
// for closing. Each track borrows a bin pointer from it.
type binCueBackend struct {
	bins   map[string]*binFile
	tracks []track
}

// buildTrack is parser-local scratch: the per-track data gathered while reading
// the cue, before file sizes and neighbours are known. It is never stored on the
// finished track.
type buildTrack struct {
	number  int
	typ     string
	control uint8
	bin     *binFile
	indexes []cueIndex // file-relative, as parsed
	pregap  int        // PREGAP command frames
	postgap int        // POSTGAP command frames
}

// maxCueSize is the largest cue sheet accepted. Real sheets are a few KB,
// and the whole sheet is held in memory while it is parsed.
const maxCueSize = 1024 * 1024

// maxCueFrames is the most frames a cue sheet's disc may span: a CD's time
// code ends at 99:59:74.
const maxCueFrames = 100 * 60 * 75

// openBinCue opens a cue sheet on disk, with its bin files read from the
// sheet's directory.
func openBinCue(path string, _ bool) (discBackend, error) {
	cf, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer cf.Close()

	sheet, err := readCueSheet(cf)
	if err != nil {
		return nil, err
	}

	// Bin files are opened through a root on the cue's directory so a FILE
	// directive can only reach files inside it.
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("cue: opening sheet directory: %w", err)
	}
	defer root.Close()

	bc, err := buildBinCue(sheet, func(name string) (*binFile, error) {
		fd, err := root.Open(name)
		if err != nil {
			return nil, err
		}
		fi, err := fd.Stat()
		if err != nil {
			fd.Close()
			return nil, err
		}
		return &binFile{r: fd, size: fi.Size(), closer: fd}, nil
	})
	if err != nil {
		return nil, err
	}
	return bc, nil
}

// readCueSheet reads a whole cue sheet, rejecting one over maxCueSize.
func readCueSheet(r io.Reader) ([]byte, error) {
	// Read one byte past the limit so an oversized sheet is detected from
	// the bytes read rather than a separate size check.
	sheet, err := io.ReadAll(io.LimitReader(r, maxCueSize+1))
	if err != nil {
		return nil, fmt.Errorf("cue: reading sheet: %w", err)
	}
	if len(sheet) > maxCueSize {
		return nil, fmt.Errorf("cue: sheet is larger than %d bytes: %w", maxCueSize, ErrFileTooLarge)
	}
	return sheet, nil
}

// buildBinCue parses a cue sheet and builds the backend, opening each bin a
// track uses once and computing the disc layout. open is given the cleaned
// name from a FILE directive and returns that bin. On any failure every
// opened bin is closed before returning.
func buildBinCue(sheet []byte, open func(name string) (*binFile, error)) (*binCueBackend, error) {
	bc := &binCueBackend{bins: make(map[string]*binFile)}
	if err := bc.parse(sheet, open); err != nil {
		bc.Close()
		return nil, err
	}
	return bc, nil
}

// parse reads the cue sheet into the backend: the bins its tracks use and
// the track layout. Bins opened before a failure are left for the caller to
// close.
func (bc *binCueBackend) parse(sheet []byte, open func(name string) (*binFile, error)) error {
	// curFile is the name from the latest FILE directive. Its bin file is
	// opened by the first TRACK that uses it, so a FILE line no track refers
	// to never opens anything.
	var curFile string
	var built []buildTrack

	sc := bufio.NewScanner(bytes.NewReader(sheet))
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
				return fmt.Errorf("cue: malformed FILE directive (line %d)", lineNo)
			}
			if !strings.EqualFold(ftype, "BINARY") {
				return fmt.Errorf("cue: unsupported FILE type %q (line %d)", ftype, lineNo)
			}
			resolved, err := resolveTrackPath(filename)
			if err != nil {
				return err
			}
			curFile = resolved

		case "TRACK":
			if curFile == "" {
				return fmt.Errorf("cue: TRACK before any FILE (line %d)", lineNo)
			}
			fields := strings.Fields(rest)
			if len(fields) < 2 {
				return fmt.Errorf("cue: malformed TRACK directive (line %d)", lineNo)
			}
			num, ok := parseCueNumber(fields[0], 1)
			if !ok {
				return fmt.Errorf("cue: bad TRACK number %q, want 01-99 (line %d)", fields[0], lineNo)
			}
			if len(built) > 0 && num <= built[len(built)-1].number {
				return fmt.Errorf("cue: TRACK %02d does not follow TRACK %02d (line %d)", num, built[len(built)-1].number, lineNo)
			}
			typ, control, err := normalizeTrackType(fields[1])
			if err != nil {
				return fmt.Errorf("cue: %w (line %d)", err, lineNo)
			}
			// Track numbers are two digits and increasing, so at most 99
			// tracks, and so at most 99 bin files, can be opened.
			bin, ok := bc.bins[curFile]
			if !ok {
				bin, err = open(curFile)
				if err != nil {
					return fmt.Errorf("cue: opening track file %q: %w", curFile, err)
				}
				bc.bins[curFile] = bin
			}
			built = append(built, buildTrack{number: num, typ: typ, control: control, bin: bin})

		case "INDEX":
			if len(built) == 0 {
				return fmt.Errorf("cue: INDEX before any TRACK (line %d)", lineNo)
			}
			fields := strings.Fields(rest)
			if len(fields) < 2 {
				return fmt.Errorf("cue: malformed INDEX directive (line %d)", lineNo)
			}
			num, ok := parseCueNumber(fields[0], 0)
			if !ok {
				return fmt.Errorf("cue: bad INDEX number %q, want 00-99 (line %d)", fields[0], lineNo)
			}
			frame, err := parseMSF(fields[1])
			if err != nil {
				return fmt.Errorf("cue: %w (line %d)", err, lineNo)
			}
			bt := &built[len(built)-1]
			if n := len(bt.indexes); n > 0 {
				prev := bt.indexes[n-1]
				if num <= prev.number {
					return fmt.Errorf("cue: INDEX %02d does not follow INDEX %02d (line %d)", num, prev.number, lineNo)
				}
				if frame < prev.frame {
					return fmt.Errorf("cue: INDEX %02d is positioned before INDEX %02d (line %d)", num, prev.number, lineNo)
				}
			}
			bt.indexes = append(bt.indexes, cueIndex{number: num, frame: frame})

		case "PREGAP", "POSTGAP":
			if len(built) == 0 {
				return fmt.Errorf("cue: %s before any TRACK (line %d)", keyword, lineNo)
			}
			frame, err := parseMSF(strings.TrimSpace(rest))
			if err != nil {
				return fmt.Errorf("cue: %w (line %d)", err, lineNo)
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
		return fmt.Errorf("cue: reading sheet: %w", err)
	}
	if len(built) == 0 {
		return fmt.Errorf("cue: no tracks")
	}

	tracks, err := layoutTracks(built)
	if err != nil {
		return err
	}
	bc.tracks = tracks
	return nil
}

// layoutTracks turns the parsed scratch tracks into laid-out tracks: absolute
// LBA, file offsets, pregap accounting, and exposed index entries. A track's
// in-file length runs to the next track in the same file, or to end-of-file.
func layoutTracks(built []buildTrack) ([]track, error) {
	// Validate up front that every track has at least one INDEX, so the
	// same-file look-ahead below (the next track's first index) and the
	// firstFrame access are always safe. Parsing already guarantees each
	// track's indexes are in increasing number order with positions that
	// never decrease.
	for i := range built {
		if len(built[i].indexes) == 0 {
			return nil, fmt.Errorf("cue: track %d has no INDEX", built[i].number)
		}
	}

	tracks := make([]track, len(built))
	runningLBA := 0
	for i := range built {
		bt := &built[i]
		firstFrame := bt.indexes[0].frame

		if bt.bin.size%cueSectorBytes != 0 {
			return nil, fmt.Errorf("cue: track %d file size %d is not a multiple of %d", bt.number, bt.bin.size, cueSectorBytes)
		}
		fileSectors := int(bt.bin.size / cueSectorBytes)
		if firstFrame > fileSectors {
			return nil, fmt.Errorf("cue: track %d INDEX position %d beyond file end %d", bt.number, firstFrame, fileSectors)
		}

		var fileFrames int
		if i+1 < len(built) && built[i+1].bin == bt.bin {
			fileFrames = built[i+1].indexes[0].frame - firstFrame
		} else {
			fileFrames = fileSectors - firstFrame
		}
		if fileFrames <= 0 {
			return nil, fmt.Errorf("cue: track %d has no data (check INDEX positions and file size)", bt.number)
		}
		if last := bt.indexes[len(bt.indexes)-1]; last.frame-firstFrame >= fileFrames {
			return nil, fmt.Errorf("cue: track %02d INDEX %02d is beyond the track's data", bt.number, last.number)
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
			bin:          bt.bin,
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
		if runningLBA > maxCueFrames {
			return nil, fmt.Errorf("cue: disc is longer than 99:59:74 at track %02d", bt.number)
		}
	}
	return tracks, nil
}

// resolveTrackPath turns a track's bin filename (from a FILE directive) into a
// name to open within the cue's directory root. Cue sheets may use
// backslashes, so they are treated as separators. The name is cleaned so
// different spellings of the same file share one name. Keeping the file inside
// the directory is left to the root, which rejects absolute names, parent
// traversal, symlinks that lead outside, and Windows device names when the
// file is opened.
func resolveTrackPath(filename string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("cue: empty FILE name")
	}
	return filepath.Clean(filepath.FromSlash(strings.ReplaceAll(filename, "\\", "/"))), nil
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
			if _, err := t.bin.r.ReadAt(buf, byteOff); err != nil {
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

// Close closes every open bin. bins holds one bin per distinct cleaned filename,
// so iterating it closes each exactly once.
func (b *binCueBackend) Close() error {
	var first error
	for _, bin := range b.bins {
		if bin.closer == nil {
			continue
		}
		if err := bin.closer.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
