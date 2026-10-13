// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package filebrowser provides the filesystem side of the in-app file and
// folder picker: listing, filtering, path input parsing and the sidebar places.
package filebrowser

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Mode selects what the picker returns.
type Mode int

const (
	// ModeFolder picks a directory. Only directories are listed.
	ModeFolder Mode = iota
	// ModeFile picks a regular file. Directories and files are listed.
	ModeFile
)

// Entry is a single item in a directory listing.
type Entry struct {
	Name  string
	Path  string
	IsDir bool
}

// ErrNotFound is returned by Resolve when the path does not name an item of
// the requested kind.
var ErrNotFound = errors.New("not found")

// ReadDir lists dir for the given mode. Dot entries are skipped. Symlinks are
// followed so a link to a directory lists as a directory; broken links are
// skipped. Directories sort before files, each case-insensitively.
func ReadDir(dir string, mode Mode) ([]Entry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(des))
	for _, de := range des {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		p := filepath.Join(dir, name)
		isDir := de.IsDir()
		if de.Type()&os.ModeSymlink != 0 {
			fi, err := os.Stat(p)
			if err != nil {
				continue
			}
			isDir = fi.IsDir()
		}
		if mode == ModeFolder && !isDir {
			continue
		}
		entries = append(entries, Entry{Name: name, Path: p, IsDir: isDir})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		a, b := strings.ToLower(entries[i].Name), strings.ToLower(entries[j].Name)
		if a != b {
			return a < b
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

// Filter returns the entries whose name contains text, case-insensitively.
// An empty text returns entries unchanged.
func Filter(entries []Entry, text string) []Entry {
	if text == "" {
		return entries
	}
	needle := strings.ToLower(text)
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Name), needle) {
			out = append(out, e)
		}
	}
	return out
}

// SplitInput splits path field text at the last separator into the directory
// part (including the separator) and the filter text after it. Text with no
// separator is all filter.
func SplitInput(text string) (dir, filter string) {
	for i := len(text) - 1; i >= 0; i-- {
		if os.IsPathSeparator(text[i]) {
			return text[:i+1], text[i+1:]
		}
	}
	return "", text
}

// Parent returns the parent of dir, or false when dir is a filesystem or
// drive root.
func Parent(dir string) (string, bool) {
	clean := filepath.Clean(dir)
	parent := filepath.Dir(clean)
	if parent == clean {
		return "", false
	}
	return parent, true
}

// DirText returns dir formatted for the path field: cleaned, with a single
// trailing separator so typing continues inside it.
func DirText(dir string) string {
	clean := filepath.Clean(dir)
	if os.IsPathSeparator(clean[len(clean)-1]) {
		return clean
	}
	return clean + string(filepath.Separator)
}

// Resolve checks that text is an absolute path naming an existing item of the
// kind the mode picks and returns it cleaned.
func Resolve(text string, mode Mode) (string, error) {
	if text == "" || !filepath.IsAbs(text) {
		return "", ErrNotFound
	}
	p := filepath.Clean(text)
	fi, err := os.Stat(p)
	if err != nil {
		return "", ErrNotFound
	}
	switch mode {
	case ModeFolder:
		if !fi.IsDir() {
			return "", ErrNotFound
		}
	case ModeFile:
		if !fi.Mode().IsRegular() {
			return "", ErrNotFound
		}
	}
	return p, nil
}
