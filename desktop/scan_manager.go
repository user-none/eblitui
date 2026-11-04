// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"fmt"

	"github.com/user-none/eblitui/coreif"
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
	extensions       []string // Supported ROM file extensions
	defaultConsoleID int
	disc             bool                   // disc-based system (group-key identity)
	discID           coreif.DiscIdentifier  // reads disc info (may be nil)
	sources          func() scanner.Sources // where and which artwork a scan downloads, read when it starts

	// Callbacks to App
	onProgress func() // Called when progress updates (triggers UI rebuild)
	onComplete func(msg string)
}

// NewScanManager creates a new scan manager
func NewScanManager(
	library *storage.Library,
	scanScreen *screens.ScanProgressScreen,
	extensions []string,
	defaultConsoleID int,
	disc bool,
	discID coreif.DiscIdentifier,
	sources func() scanner.Sources,
	onProgress func(),
	onComplete func(msg string),
) *ScanManager {
	return &ScanManager{
		library:          library,
		scanScreen:       scanScreen,
		extensions:       extensions,
		defaultConsoleID: defaultConsoleID,
		disc:             disc,
		discID:           discID,
		sources:          sources,
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
		sm.library.ScanDirectories(),
		sm.library.ExcludedPaths(),
		sm.library.ScanBase(),
		sm.extensions,
		sm.defaultConsoleID,
		sm.disc,
		sm.discID,
		sm.sources(),
	)

	// Configure scan screen
	sm.scanScreen.SetScanner(sm.activeScanner)

	// Start scanner in background
	go sm.activeScanner.Run()
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

// handleComplete processes scan results
func (sm *ScanManager) handleComplete(result scanner.ScanResult) {
	newGames := sm.library.MergeScanned(sm.activeScanner.Games())

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
