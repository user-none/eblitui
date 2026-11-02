// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"errors"
	"log"
	"os"
	"sync/atomic"

	"github.com/user-none/eblitui/desktop/artwork"
	"github.com/user-none/eblitui/desktop/screens"
	"github.com/user-none/eblitui/desktop/storage"
)

// CleanManager runs Clean Library. The game files are checked in the
// background while the progress screen shows how far the check is, then
// the missing discs and games are removed on the UI thread together with
// the library games' superseded boxart.png files. Clean Library works on
// local files only. A cancelled clean changes nothing.
type CleanManager struct {
	// Active clean, nil when none is running
	active *cleanRun

	// External dependencies (not owned by CleanManager)
	library *storage.Library
	screen  *screens.ScanProgressScreen

	// Callbacks to App
	onProgress func() // Called when progress updates (triggers UI rebuild)
	onComplete func(msg string)
}

// NewCleanManager creates a new clean manager
func NewCleanManager(library *storage.Library, screen *screens.ScanProgressScreen, onProgress func(), onComplete func(msg string)) *CleanManager {
	return &CleanManager{
		library:    library,
		screen:     screen,
		onProgress: onProgress,
		onComplete: onComplete,
	}
}

// Start begins checking the library's files in the background
func (cm *CleanManager) Start() {
	// The check works on copies so it never reads the library entries
	// while the UI thread may change them
	games := make(map[string]storage.GameEntry, len(cm.library.Games))
	for key, game := range cm.library.Games {
		entry := *game
		entry.Discs = append([]storage.GameDisc(nil), game.Discs...)
		games[key] = entry
	}

	cm.active = newCleanRun()
	cm.screen.SetScanner(cm.active)
	go cm.active.check(games)
}

// Update polls for clean progress and completion.
// Should be called each frame while cleaning.
func (cm *CleanManager) Update() {
	if cm.active == nil {
		return
	}

	select {
	case p := <-cm.active.progress:
		cm.screen.UpdateProgress(screens.ScanProgress{
			Progress:   p.progress,
			CountText:  "Missing: " + gameCount(p.missing),
			StatusText: "Checking game files...",
		})
		if cm.onProgress != nil {
			cm.onProgress()
		}
	default:
	}

	select {
	case res := <-cm.active.done:
		cm.handleComplete(res)
	default:
	}
}

// Cancel stops the current clean
func (cm *CleanManager) Cancel() {
	if cm.active != nil {
		cm.active.Cancel()
	}
}

// handleComplete removes the missing files and the superseded box art the
// check found, or nothing when it was cancelled (res is nil)
func (cm *CleanManager) handleComplete(res *cleanResult) {
	cm.active = nil

	var msg string
	if res != nil {
		msg = cleanMessage(cm.library.RemoveMissing(res.files))
		removeArtwork(res.supersededArt)
	}
	if cm.onComplete != nil {
		cm.onComplete(msg)
	}
}

// removeArtwork deletes superseded box art files, logging any that cannot
// be removed
func removeArtwork(paths []string) {
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("Failed to remove superseded box art %s: %v", p, err)
		}
	}
}

// cleanMessage returns the notification for a finished clean that
// changed n games
func cleanMessage(n int, err error) string {
	switch {
	case err != nil:
		log.Printf("Failed to clean library: %v", err)
		return "Failed to clean library"
	case n == 0:
		return "No missing games"
	default:
		return "Cleaned " + gameCount(n)
	}
}

// cleanProgress is one progress update from a running clean
type cleanProgress struct {
	progress float64 // Fraction of games checked
	missing  int     // Games with a missing file so far
}

// cleanResult is what a finished, uncancelled check found
type cleanResult struct {
	files         map[string]storage.FileStatus // games with a missing file
	supersededArt []string                      // boxart.png files to remove
}

// cleanRun is one background check of the library's files
type cleanRun struct {
	cancelled atomic.Bool
	progress  chan cleanProgress
	done      chan *cleanResult
}

func newCleanRun() *cleanRun {
	return &cleanRun{
		progress: make(chan cleanProgress, 10),
		done:     make(chan *cleanResult, 1),
	}
}

// Cancel stops the check; the run then reports no files
func (r *cleanRun) Cancel() {
	r.cancelled.Store(true)
}

// check stats every game's files and finds each game's boxart.png that a
// boxart.webp supersedes (see artwork.SupersededBoxArt), and sends both on
// done, or nil when cancelled
func (r *cleanRun) check(games map[string]storage.GameEntry) {
	files := make(map[string]storage.FileStatus)
	var superseded []string
	checked := 0
	for key, game := range games {
		if r.cancelled.Load() {
			r.done <- nil
			return
		}
		if status := game.CheckFiles(); status.HasMissing() {
			files[key] = status
		}
		if dir, err := storage.GetGameArtworkDir(game.GameID); err == nil {
			if p, ok := artwork.SupersededBoxArt(dir); ok {
				superseded = append(superseded, p)
			}
		}
		checked++

		// Skip the update when the UI has not read the previous ones
		select {
		case r.progress <- cleanProgress{progress: float64(checked) / float64(len(games)), missing: len(files)}:
		default:
		}
	}
	if r.cancelled.Load() {
		r.done <- nil
		return
	}
	r.done <- &cleanResult{files: files, supersededArt: superseded}
}
