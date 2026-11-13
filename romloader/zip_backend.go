// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package romloader

import (
	"archive/zip"
	"cmp"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

// maxZipDiscBytes is the most uncompressed bin data one zip disc may hold:
// the raw sectors of a disc running to 99:59:74.
const maxZipDiscBytes = maxCueFrames * cueSectorBytes

// zipEncrypted is the general purpose flag bit marking an encrypted entry.
const zipEncrypted = 0x1

// zipMacOSXDir is the top-level directory the macOS archiver adds to a zip.
// It holds a metadata file named after each real entry.
const zipMacOSXDir = "__MACOSX/"

// zipAppleDouble is the file name prefix of a macOS metadata file stored
// beside the file it describes.
const zipAppleDouble = "._"

// zipEntryName returns the path a zip entry is known by, and false for an
// entry that is never a disc file: a directory, anything under zipMacOSXDir,
// or a zipAppleDouble file. Names are meant to use forward slashes. Some
// tools write backslashes, which are read as separators too, as they are in
// a cue sheet's FILE names. The path is cleaned and made relative to the top
// of the zip, the form a bin lookup uses.
func zipEntryName(zf *zip.File) (string, bool) {
	name := strings.ReplaceAll(zf.Name, `\`, "/")
	if zf.FileInfo().IsDir() || strings.HasSuffix(name, "/") {
		return "", false
	}
	name = strings.TrimPrefix(path.Clean("/"+name), "/")
	if strings.HasPrefix(name, zipMacOSXDir) || strings.HasPrefix(path.Base(name), zipAppleDouble) {
		return "", false
	}
	return name, true
}

// zipDiscEntries returns the zip's disc files by name and the name of its
// cue sheet. A zip must hold exactly one cue sheet. When the name of a bin
// repeats, the first entry with it is used.
func zipDiscEntries(zr *zip.Reader) (map[string]*zip.File, string, error) {
	entries := make(map[string]*zip.File, len(zr.File))
	cueName := ""
	for _, zf := range zr.File {
		name, ok := zipEntryName(zf)
		if !ok {
			continue
		}
		if _, dup := entries[name]; !dup {
			entries[name] = zf
		}
		if strings.EqualFold(path.Ext(name), ".cue") {
			if cueName != "" {
				return nil, "", fmt.Errorf("zip: more than one cue sheet (%q and %q)", cueName, name)
			}
			cueName = name
		}
	}
	if cueName == "" {
		return nil, "", fmt.Errorf("zip: no cue sheet")
	}
	return entries, cueName, nil
}

// openZipBin returns the bin held in entry zf of the zip file f, which is
// fileSize bytes. A stored entry is read straight from the zip. A deflated
// one is returned as a zipEntry too, with nothing indexed yet.
func openZipBin(f *os.File, fileSize int64, zf *zip.File) (*binFile, *zipEntry, error) {
	if zf.Flags&zipEncrypted != 0 {
		return nil, nil, fmt.Errorf("zip: entry is encrypted")
	}
	// Bound each size before it is used as a signed value.
	if zf.UncompressedSize64 > maxZipDiscBytes || zf.CompressedSize64 > uint64(fileSize) {
		return nil, nil, fmt.Errorf("zip: entry size out of range: %w", ErrFileTooLarge)
	}
	size := int64(zf.UncompressedSize64)
	csize := int64(zf.CompressedSize64)
	off, err := zf.DataOffset()
	if err != nil {
		return nil, nil, fmt.Errorf("zip: locating entry data: %w", err)
	}
	if off > fileSize-csize {
		return nil, nil, fmt.Errorf("zip: entry data runs past the end of the file")
	}

	switch zf.Method {
	case zip.Store:
		if csize != size {
			return nil, nil, fmt.Errorf("zip: stored entry is %d bytes, the zip directory gives %d", csize, size)
		}
		return &binFile{r: io.NewSectionReader(f, off, size), size: size}, nil, nil
	case zip.Deflate:
		e := &zipEntry{r: f, off: off, csize: csize, size: size, crc: zf.CRC32, ckpts: []zipCheckpoint{{}}}
		return &binFile{r: e, size: size}, e, nil
	default:
		return nil, nil, fmt.Errorf("zip: unsupported compression method %d", zf.Method)
	}
}

// zipCueBackend is a cue/bin disc held in a zip: the cue backend reading its
// bins from zip entries, plus the zip file those entries read from.
type zipCueBackend struct {
	*binCueBackend
	f *os.File
}

// openZipCue opens a zip holding one cue sheet and the bin files it names.
// Bin names resolve against the cue sheet's directory inside the zip.
// Deflated bins are indexed before it returns, unless lazy is set, in which
// case each is indexed as far as reads reach.
func openZipCue(file string, lazy bool) (_ discBackend, err error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(f, fi.Size())
	if err != nil {
		return nil, fmt.Errorf("zip: %w", err)
	}
	entries, cueName, err := zipDiscEntries(zr)
	if err != nil {
		return nil, err
	}

	rc, err := entries[cueName].Open()
	if err != nil {
		return nil, fmt.Errorf("zip: opening cue sheet %q: %w", cueName, err)
	}
	sheet, err := readCueSheet(rc)
	rc.Close()
	if err != nil {
		return nil, err
	}

	dec := &zipDecoder{}
	var deflated []*zipEntry
	var total int64
	dir := path.Dir(cueName)
	bc, err := buildBinCue(sheet, func(name string) (*binFile, error) {
		name = path.Join(dir, filepath.ToSlash(name))
		zf := entries[name]
		if zf == nil {
			return nil, os.ErrNotExist
		}
		bin, e, err := openZipBin(f, fi.Size(), zf)
		if err != nil {
			return nil, err
		}
		total += bin.size
		if total > maxZipDiscBytes {
			return nil, fmt.Errorf("zip: bin files hold more than a disc of %d bytes: %w", int64(maxZipDiscBytes), ErrFileTooLarge)
		}
		if e != nil {
			e.name, e.dec = name, dec
			deflated = append(deflated, e)
		}
		return bin, nil
	})
	if err != nil {
		return nil, err
	}

	// The sheet and the bin sizes are valid.
	if !lazy {
		if err := indexZipEntries(deflated); err != nil {
			bc.Close()
			return nil, err
		}
	}
	return &zipCueBackend{binCueBackend: bc, f: f}, nil
}

// zipIndexWorkers returns how many entries to index at a time: the total
// size over the largest size, rounded up, plus one, up to the entry count
// and cpus. order is the entries largest first. More workers than let the
// rest finish alongside the largest entry gain nothing.
func zipIndexWorkers(order []*zipEntry, cpus int) int {
	if len(order) == 0 {
		return 0
	}
	largest := order[0].size
	if largest == 0 {
		return 1
	}
	var total int64
	for _, e := range order {
		total += e.size
	}
	want := (total+largest-1)/largest + 1
	return int(min(want, int64(len(order)), int64(cpus)))
}

// indexZipEntries builds the index of every entry, several at a time. Each
// entry is a separate deflate stream read from the zip at explicit offsets,
// and each worker decodes with its own inflater, so the workers share no
// state and need no locking. The largest entries start first because the
// longest one bounds the total time. All workers have finished when it
// returns.
func indexZipEntries(entries []*zipEntry) error {
	order := slices.Clone(entries)
	slices.SortStableFunc(order, func(a, b *zipEntry) int { return cmp.Compare(b.size, a.size) })

	errs := make([]error, len(order))
	var next atomic.Int64
	var failed atomic.Bool
	var wg sync.WaitGroup
	for range zipIndexWorkers(order, runtime.GOMAXPROCS(0)) {
		wg.Go(func() {
			var inf inflater
			for !failed.Load() {
				i := int(next.Add(1)) - 1
				if i >= len(order) {
					return
				}
				errs[i] = order[i].buildIndex(&inf)
				if errs[i] != nil {
					failed.Store(true)
				}
			}
		})
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("zip: track file %q: %w", order[i].name, err)
		}
	}
	return nil
}

// Close releases the bins and the zip file.
func (b *zipCueBackend) Close() error {
	err := b.binCueBackend.Close()
	if ferr := b.f.Close(); err == nil {
		err = ferr
	}
	return err
}
