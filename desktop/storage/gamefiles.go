// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"errors"
	"os"
	"slices"
)

// FileStatus reports which of a game's files are missing on disk
type FileStatus struct {
	Missing     bool   // No file of the game is available
	DiscMissing []bool // Per Discs row; nil for cartridge games
}

// MissingAt reports whether the disc at the given Discs index is missing.
// For a cartridge game it reports whether the game is missing.
func (s FileStatus) MissingAt(idx int) bool {
	if idx < 0 || idx >= len(s.DiscMissing) {
		return s.Missing
	}
	return s.DiscMissing[idx]
}

// CheckFiles stats the game's files. A file is missing when it does not
// exist; other stat errors count as present and are reported by launch.
func (g *GameEntry) CheckFiles() FileStatus {
	if len(g.Discs) == 0 {
		return FileStatus{Missing: !fileExists(g.File)}
	}

	status := FileStatus{Missing: true, DiscMissing: make([]bool, len(g.Discs))}
	for i, d := range g.Discs {
		status.DiscMissing[i] = !fileExists(d.File)
		if !status.DiscMissing[i] {
			status.Missing = false
		}
	}
	return status
}

// SelectedDiscIndex returns the Discs index to launch: SelectedDisc, or
// the first disc when SelectedDisc is out of range. Returns 0 for a
// cartridge game.
func (g *GameEntry) SelectedDiscIndex() int {
	if g.SelectedDisc < 0 || g.SelectedDisc >= len(g.Discs) {
		return 0
	}
	return g.SelectedDisc
}

// RemoveDisc removes the Discs row at idx. The selection stays on the same
// disc when another row is removed and moves to the first row when the
// selected row is removed.
func (g *GameEntry) RemoveDisc(idx int) {
	if idx < 0 || idx >= len(g.Discs) {
		return
	}
	drop := make([]bool, len(g.Discs))
	drop[idx] = true
	g.removeDiscs(drop)
}

// removeDiscs removes every Discs row whose drop entry is true, keeping
// the selection on the same disc when it stays and moving it to the first
// row otherwise
func (g *GameEntry) removeDiscs(drop []bool) {
	sel := g.SelectedDiscIndex()
	kept := make([]GameDisc, 0, len(g.Discs))
	newSel := 0
	for i, d := range g.Discs {
		if drop[i] {
			continue
		}
		if i == sel {
			newSel = len(kept)
		}
		kept = append(kept, d)
	}
	g.Discs = kept
	g.SelectedDisc = newSel
}

// HasMissing reports whether any of the game's files is missing
func (s FileStatus) HasMissing() bool {
	return s.Missing || slices.Contains(s.DiscMissing, true)
}

// RemoveMissing removes the missing files found by CheckFiles, keyed by
// game key: disc rows whose file is gone are dropped, and games with no
// file left move to removed.json. A status whose disc count no longer
// matches its game is skipped. The library is saved when anything
// changed. Returns the number of games changed or removed.
func (lib *Library) RemoveMissing(files map[string]FileStatus) (int, error) {
	var gone []string
	pruned := 0
	for key, status := range files {
		game := lib.GetGame(key)
		if game == nil {
			continue
		}
		if status.Missing {
			gone = append(gone, key)
			continue
		}
		if len(status.DiscMissing) == len(game.Discs) && slices.Contains(status.DiscMissing, true) {
			game.removeDiscs(status.DiscMissing)
			pruned++
		}
	}

	switch {
	case len(gone) > 0:
		// Saves the library, including the pruned discs
		if err := lib.RemoveGames(gone...); err != nil {
			return 0, err
		}
	case pruned > 0:
		if err := SaveLibrary(lib); err != nil {
			return 0, err
		}
	}
	return len(gone) + pruned, nil
}

// fileExists reports whether path exists. An empty path does not exist.
func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}
