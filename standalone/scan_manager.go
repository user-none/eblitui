//go:build !libretro

package standalone

import (
	"fmt"
	"log"

	"github.com/user-none/eblitui/standalone/screens"
	"github.com/user-none/eblitui/standalone/storage"
)

// ScanManager handles ROM scanning orchestration.
// This includes creating and running scanners, tracking progress,
// and merging results into the library.
type ScanManager struct {
	// Active scanner instance
	scanner *Scanner

	// External dependencies (not owned by ScanManager)
	library       *storage.Library
	scanScreen    *screens.ScanProgressScreen
	extensions    []string // Supported ROM file extensions
	rdbName       string   // RDB name for metadata downloads
	thumbnailRepo string   // Thumbnail repository name for artwork

	// Callbacks to App
	onProgress func() // Called when progress updates (triggers UI rebuild)
	onComplete func(msg string)
}

// NewScanManager creates a new scan manager
func NewScanManager(
	library *storage.Library,
	scanScreen *screens.ScanProgressScreen,
	extensions []string,
	rdbName string,
	thumbnailRepo string,
	onProgress func(),
	onComplete func(msg string),
) *ScanManager {
	return &ScanManager{
		library:       library,
		scanScreen:    scanScreen,
		extensions:    extensions,
		rdbName:       rdbName,
		thumbnailRepo: thumbnailRepo,
		onProgress:    onProgress,
		onComplete:    onComplete,
	}
}

// SetLibrary updates the library reference
func (sm *ScanManager) SetLibrary(library *storage.Library) {
	sm.library = library
}

// SetScanScreen updates the scan screen reference
func (sm *ScanManager) SetScanScreen(screen *screens.ScanProgressScreen) {
	sm.scanScreen = screen
}

// IsScanning returns true if a scan is in progress
func (sm *ScanManager) IsScanning() bool {
	return sm.scanner != nil
}

// Start begins a new scan operation
func (sm *ScanManager) Start(rescanAll bool) {
	// Create scanner with current library data
	sm.scanner = NewScanner(
		sm.library.ScanDirectories,
		sm.library.ExcludedPaths,
		sm.library.Games,
		rescanAll,
		sm.extensions,
		sm.rdbName,
		sm.thumbnailRepo,
	)

	// Configure scan screen
	sm.scanScreen.SetScanner(sm.scanner)

	// Start scanner in background
	go sm.scanner.Run()
}

// Update polls for scan progress and completion.
// Should be called each frame while scanning.
func (sm *ScanManager) Update() {
	if sm.scanner == nil {
		return
	}

	// Non-blocking read from progress channel
	select {
	case progress := <-sm.scanner.Progress():
		// Convert ui.ScanProgress to screens.ScanProgress
		sm.scanScreen.UpdateProgress(screens.ScanProgress{
			Phase:           int(progress.Phase),
			Progress:        progress.Progress,
			GamesFound:      progress.GamesFound,
			ArtworkTotal:    progress.ArtworkTotal,
			ArtworkComplete: progress.ArtworkComplete,
			StatusText:      progress.StatusText,
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
	case result := <-sm.scanner.Done():
		sm.handleComplete(result)
	default:
		// Still running
	}
}

// Cancel stops the current scan
func (sm *ScanManager) Cancel() {
	if sm.scanner != nil {
		sm.scanner.Cancel()
	}
}

// handleComplete processes scan results
func (sm *ScanManager) handleComplete(result ScanResult) {
	// Merge discovered games into library
	for gameCRC, game := range sm.scanner.Games() {
		sm.library.Games[gameCRC] = game
	}

	// Save updated library
	if err := storage.SaveLibrary(sm.library); err != nil {
		log.Printf("Failed to save library: %v", err)
	}

	// Prepare notification message
	var msg string
	if result.Cancelled {
		msg = "" // No notification on cancel
	} else if len(result.Errors) > 0 {
		msg = result.Errors[0].Error()
	} else if result.NewGames > 0 {
		msg = fmt.Sprintf("Found %d new games", result.NewGames)
	} else {
		msg = "Library up to date"
	}

	// Clear scanner reference
	sm.scanner = nil

	// Notify App of completion
	if sm.onComplete != nil {
		sm.onComplete(msg)
	}
}
