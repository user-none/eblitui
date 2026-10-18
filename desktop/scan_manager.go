// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"fmt"
	"log"

	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/metadata"
	"github.com/user-none/eblitui/desktop/scanner"
	"github.com/user-none/eblitui/desktop/screens"
	"github.com/user-none/eblitui/desktop/storage"
)

// ScanManager handles ROM scanning orchestration.
// This includes creating and running scanners, tracking progress,
// and merging results into the library.
type ScanManager struct {
	// Active scanner instance
	activeScanner *scanner.Scanner

	// External dependencies (not owned by ScanManager)
	library          *storage.Library
	scanScreen       *screens.ScanProgressScreen
	extensions       []string                  // Supported ROM file extensions
	metadata         *metadata.MetadataManager // Metadata for RDB/thumbnail lookups
	defaultConsoleID int
	disc             bool                  // disc-based system (group-key identity)
	discID           coreif.DiscIdentifier // reads disc info (may be nil)
	downloads        scanner.Downloads     // optional asset downloads to perform

	// Callbacks to App
	onProgress func() // Called when progress updates (triggers UI rebuild)
	onComplete func(msg string)
}

// NewScanManager creates a new scan manager
func NewScanManager(
	library *storage.Library,
	scanScreen *screens.ScanProgressScreen,
	extensions []string,
	md *metadata.MetadataManager,
	defaultConsoleID int,
	disc bool,
	discID coreif.DiscIdentifier,
	downloads scanner.Downloads,
	onProgress func(),
	onComplete func(msg string),
) *ScanManager {
	return &ScanManager{
		library:          library,
		scanScreen:       scanScreen,
		extensions:       extensions,
		metadata:         md,
		defaultConsoleID: defaultConsoleID,
		disc:             disc,
		discID:           discID,
		downloads:        downloads,
		onProgress:       onProgress,
		onComplete:       onComplete,
	}
}

// IsScanning returns true if a scan is in progress
func (sm *ScanManager) IsScanning() bool {
	return sm.activeScanner != nil
}

// Start begins a new scan operation
func (sm *ScanManager) Start() {
	// Create scanner with current library data
	sm.activeScanner = scanner.NewScanner(
		sm.library.ScanDirectories,
		sm.library.ExcludedPaths,
		sm.existingGames(),
		sm.extensions,
		sm.metadata,
		sm.defaultConsoleID,
		sm.disc,
		sm.discID,
		sm.downloads,
	)

	// Configure scan screen
	sm.scanScreen.SetScanner(sm.activeScanner)

	// Start scanner in background
	go sm.activeScanner.Run()
}

// existingGames returns the entries whose user data a scan keeps: the
// library's games, plus removed games not in the library so a scan that
// finds one again restores it
func (sm *ScanManager) existingGames() map[string]*storage.GameEntry {
	removed, err := storage.LoadRemovedGames()
	if err != nil {
		log.Printf("Failed to load removed games: %v", err)
		return sm.library.Games
	}

	existing := make(map[string]*storage.GameEntry, len(sm.library.Games)+len(removed.Games))
	for key, game := range removed.Games {
		existing[key] = game
	}
	for key, game := range sm.library.Games {
		existing[key] = game
	}
	return existing
}

// Update polls for scan progress and completion.
// Should be called each frame while scanning.
func (sm *ScanManager) Update() {
	if sm.activeScanner == nil {
		return
	}

	// Non-blocking read from progress channel
	select {
	case progress := <-sm.activeScanner.Progress():
		// Convert scanner.ScanProgress to screens.ScanProgress
		sm.scanScreen.UpdateProgress(screens.ScanProgress{
			Phase:            int(progress.Phase),
			Progress:         progress.Progress,
			CountText:        "Found: " + gameCount(progress.GamesFound),
			DownloadTotal:    progress.DownloadTotal,
			DownloadComplete: progress.DownloadComplete,
			StatusText:       progress.StatusText,
		})
		// Notify App to rebuild UI
		if sm.onProgress != nil {
			sm.onProgress()
		}
	default:
		// No update this frame
	}

	// Check for completion
	select {
	case result := <-sm.activeScanner.Done():
		sm.handleComplete(result)
	default:
		// Still running
	}
}

// Cancel stops the current scan
func (sm *ScanManager) Cancel() {
	if sm.activeScanner != nil {
		sm.activeScanner.Cancel()
	}
}

// handleComplete processes scan results
func (sm *ScanManager) handleComplete(result scanner.ScanResult) {
	newGames := mergeScannedGames(sm.library, sm.activeScanner.Games())

	// Save updated library, then drop restored games from the removed
	// games so their user data is never only in removed.json
	if err := storage.SaveLibrary(sm.library); err != nil {
		log.Printf("Failed to save library: %v", err)
	} else if err := storage.DropRestoredGames(sm.library); err != nil {
		log.Printf("Failed to update removed games: %v", err)
	}

	// Prepare notification message
	var msg string
	if result.Cancelled {
		msg = "" // No notification on cancel
	} else if len(result.Errors) > 0 {
		msg = result.Errors[0].Error()
	} else if newGames == 1 {
		msg = "Found 1 new game"
	} else if newGames > 1 {
		msg = fmt.Sprintf("Found %d new games", newGames)
	} else {
		msg = "Library up to date"
	}

	// Clear scanner reference
	sm.activeScanner = nil

	// Notify App of completion
	if sm.onComplete != nil {
		sm.onComplete(msg)
	}
}

// gameCount returns n followed by "game" or "games"
func gameCount(n int) string {
	if n == 1 {
		return "1 game"
	}
	return fmt.Sprintf("%d games", n)
}

// mergeScannedGames puts the games a scan found into the library,
// replacing the entries they update, and returns how many were not in the
// library before. Restored removed games count as new.
func mergeScannedGames(library *storage.Library, found map[string]*storage.GameEntry) int {
	newGames := 0
	for key, game := range found {
		if library.Games[key] == nil {
			newGames++
		}
		library.Games[key] = game
	}
	return newGames
}
