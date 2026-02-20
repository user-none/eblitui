//go:build !libretro

package standalone

import (
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/user-none/eblitui/rdb"
	"github.com/user-none/eblitui/romloader"
	"github.com/user-none/eblitui/standalone/storage"
)

// ScanPhase represents the current scanning phase
type ScanPhase int

const (
	ScanPhaseInit ScanPhase = iota
	ScanPhaseDiscovery
	ScanPhaseArtwork
	ScanPhaseComplete
)

// ScanProgress represents progress updates from the scanner
type ScanProgress struct {
	Phase           ScanPhase
	Progress        float64 // 0.0 to 1.0
	GamesFound      int
	ArtworkTotal    int
	ArtworkComplete int
	StatusText      string
}

// ScanResult represents the final scan result
type ScanResult struct {
	NewGames  int
	Errors    []error
	Cancelled bool
}

// Scanner handles ROM scanning in the background
type Scanner struct {
	// Configuration
	directories   []storage.ScanDirectory
	excludedPaths map[string]bool
	existingGames map[string]*storage.GameEntry // Full existing entries to preserve user data
	rescanAll     bool
	extensions    []string // Supported ROM file extensions

	// Metadata
	metadata *MetadataManager

	// Channels
	cancel   chan struct{}
	progress chan ScanProgress
	done     chan ScanResult

	// Internal state
	mu              sync.Mutex
	games           map[string]*storage.GameEntry
	artworkQueue    []artworkJob // Games that need artwork download
	errors          []error
	cancelled       bool
	artworkSem      chan struct{} // Semaphore for concurrent downloads (size 2)
	artworkComplete int
}

// artworkJob represents a pending artwork download
type artworkJob struct {
	gameCRC  string
	gameName string // No-Intro name from RDB
}

// NewScanner creates a new scanner instance
func NewScanner(dirs []storage.ScanDirectory, excluded []string, existing map[string]*storage.GameEntry, rescanAll bool, extensions []string, rdbName, thumbnailRepo string) *Scanner {
	excludedMap := make(map[string]bool)
	for _, p := range excluded {
		excludedMap[p] = true
	}

	return &Scanner{
		directories:   dirs,
		excludedPaths: excludedMap,
		existingGames: existing, // Keep full map to preserve user data
		rescanAll:     rescanAll,
		extensions:    extensions,
		metadata:      NewMetadataManager(rdbName, thumbnailRepo),
		cancel:        make(chan struct{}),
		progress:      make(chan ScanProgress, 10),
		done:          make(chan ScanResult, 1),
		games:         make(map[string]*storage.GameEntry),
		artworkQueue:  make([]artworkJob, 0),
		artworkSem:    make(chan struct{}, 2), // Limit to 2 concurrent downloads
	}
}

// Progress returns the progress channel
func (s *Scanner) Progress() <-chan ScanProgress {
	return s.progress
}

// Done returns the done channel
func (s *Scanner) Done() <-chan ScanResult {
	return s.done
}

// Cancel signals the scanner to stop
func (s *Scanner) Cancel() {
	s.mu.Lock()
	if !s.cancelled {
		s.cancelled = true
		close(s.cancel)
	}
	s.mu.Unlock()
}

// Games returns the discovered games
func (s *Scanner) Games() map[string]*storage.GameEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.games
}

// Run starts the scanning process
func (s *Scanner) Run() {
	defer close(s.done)
	defer close(s.progress)

	// Phase 0: Load RDB metadata
	s.sendProgress(ScanProgress{
		Phase:      ScanPhaseInit,
		StatusText: "Loading metadata...",
	})

	if err := s.metadata.LoadRDB(); err != nil {
		// Non-fatal: continue without metadata
		s.mu.Lock()
		s.errors = append(s.errors, fmt.Errorf("failed to load RDB: %w", err))
		s.mu.Unlock()
	}

	// Phase 1: Discovery - find ROM files
	s.sendProgress(ScanProgress{
		Phase:      ScanPhaseDiscovery,
		StatusText: "Scanning for games...",
	})

	var romFiles []string
	for _, dir := range s.directories {
		if s.isCancelled() {
			s.done <- ScanResult{Cancelled: true}
			return
		}

		files, err := s.scanDirectory(dir)
		if err != nil {
			s.mu.Lock()
			s.errors = append(s.errors, err)
			s.mu.Unlock()
			continue
		}
		romFiles = append(romFiles, files...)
	}

	// Phase 2: Process ROMs
	totalFiles := len(romFiles)
	for i, path := range romFiles {
		if s.isCancelled() {
			break
		}

		s.processROM(path)

		s.sendProgress(ScanProgress{
			Phase:      ScanPhaseDiscovery,
			Progress:   float64(i+1) / float64(totalFiles),
			GamesFound: s.gamesCount(),
			StatusText: "Scanning for games...",
		})
	}

	if s.isCancelled() {
		s.done <- ScanResult{
			NewGames:  s.gamesCount(),
			Errors:    s.getErrors(),
			Cancelled: true,
		}
		return
	}

	// Phase 3: Download artwork
	s.downloadArtwork()

	// Send final result
	s.done <- ScanResult{
		NewGames:  s.gamesCount(),
		Errors:    s.getErrors(),
		Cancelled: s.isCancelled(),
	}
}

// scanDirectory walks a directory looking for ROM files
func (s *Scanner) scanDirectory(dir storage.ScanDirectory) ([]string, error) {
	var files []string

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		// Check if excluded
		if s.isPathExcluded(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip directories (except for non-recursive case)
		if info.IsDir() {
			if path != dir.Path && !dir.Recursive {
				return filepath.SkipDir
			}
			return nil
		}

		// Check extension
		ext := strings.ToLower(filepath.Ext(path))
		if s.isSupportedExtension(ext) {
			files = append(files, path)
		}

		return nil
	}

	err := filepath.Walk(dir.Path, walkFn)
	if err != nil {
		return nil, fmt.Errorf("error scanning %s: %w", dir.Path, err)
	}

	return files, nil
}

// processROM loads and processes a single ROM file
func (s *Scanner) processROM(path string) {
	// Load ROM using romloader (handles archives)
	romData, filename, err := romloader.Load(path, s.extensions)
	if err != nil {
		// Skip unsupported formats silently
		return
	}

	// Calculate CRC32
	crcValue := crc32.ChecksumIEEE(romData)
	crcHex := fmt.Sprintf("%08x", crcValue)

	// Check if game already exists in library
	existingEntry := s.existingGames[crcHex]

	// Skip if already in library and not rescanning all
	if !s.rescanAll && existingEntry != nil {
		return
	}

	var entry *storage.GameEntry

	if existingEntry != nil {
		// Update existing entry - preserve user data
		entry = &storage.GameEntry{
			// Preserve user data
			CRC32:           crcHex,
			Favorite:        existingEntry.Favorite,
			PlayTimeSeconds: existingEntry.PlayTimeSeconds,
			LastPlayed:      existingEntry.LastPlayed,
			Added:           existingEntry.Added,
			Settings:        existingEntry.Settings,

			// Update file path (may have moved)
			File:    path,
			Missing: false,

			// Will be updated with metadata below or from existing
			Name:        existingEntry.Name,
			DisplayName: existingEntry.DisplayName,
			Region:      existingEntry.Region,
			Developer:   existingEntry.Developer,
			Publisher:   existingEntry.Publisher,
			Genre:       existingEntry.Genre,
			Franchise:   existingEntry.Franchise,
			ESRBRating:  existingEntry.ESRBRating,
			ReleaseDate: existingEntry.ReleaseDate,
		}
	} else {
		// Create new entry - Name/DisplayName left empty so RDB lookup can fill them
		entry = &storage.GameEntry{
			CRC32:   crcHex,
			File:    path,
			Added:   time.Now().Unix(),
			Missing: false,
		}
	}

	// Look up in RDB for metadata - only fill in empty fields
	if game := s.metadata.LookupByCRC32(crcValue); game != nil {
		if entry.Name == "" {
			entry.Name = game.Name
		}
		if entry.DisplayName == "" {
			entry.DisplayName = rdb.GetDisplayName(game.Name)
		}
		if entry.Region == "" {
			entry.Region = rdb.GetRegionFromName(game.Name)
		}
		if entry.Developer == "" {
			entry.Developer = game.Developer
		}
		if entry.Publisher == "" {
			entry.Publisher = game.Publisher
		}
		if entry.Genre == "" {
			entry.Genre = game.Genre
		}
		if entry.Franchise == "" {
			entry.Franchise = game.Franchise
		}
		if entry.ESRBRating == "" {
			entry.ESRBRating = game.ESRBRating
		}

		// Combine release month and year into "Month / Year" format
		if entry.ReleaseDate == "" && game.ReleaseYear > 0 {
			if game.ReleaseMonth > 0 && game.ReleaseMonth <= 12 {
				months := []string{"", "January", "February", "March", "April", "May", "June",
					"July", "August", "September", "October", "November", "December"}
				entry.ReleaseDate = fmt.Sprintf("%s %d", months[game.ReleaseMonth], game.ReleaseYear)
			} else {
				entry.ReleaseDate = fmt.Sprintf("%d", game.ReleaseYear)
			}
		}

		// Queue artwork download only if artwork doesn't exist
		artPath, _ := storage.GetGameArtworkPath(crcHex)
		if _, err := os.Stat(artPath); os.IsNotExist(err) {
			s.mu.Lock()
			s.artworkQueue = append(s.artworkQueue, artworkJob{
				gameCRC:  crcHex,
				gameName: game.Name,
			})
			s.mu.Unlock()
		}
	}

	// Fallback to filename when no RDB match provided Name/DisplayName
	if entry.Name == "" {
		entry.Name = strings.TrimSuffix(filename, filepath.Ext(filename))
	}
	if entry.DisplayName == "" {
		entry.DisplayName = s.cleanDisplayName(filename)
	}

	s.mu.Lock()
	s.games[crcHex] = entry
	s.mu.Unlock()
}

// cleanDisplayName removes file extension and parenthesized metadata
func (s *Scanner) cleanDisplayName(filename string) string {
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	if idx := strings.Index(name, " ("); idx > 0 {
		name = strings.TrimSpace(name[:idx])
	}

	return name
}

// downloadArtwork downloads artwork for all queued games
func (s *Scanner) downloadArtwork() {
	s.mu.Lock()
	queue := s.artworkQueue
	total := len(queue)
	s.mu.Unlock()

	if total == 0 {
		return
	}

	s.sendProgress(ScanProgress{
		Phase:           ScanPhaseArtwork,
		Progress:        0,
		GamesFound:      s.gamesCount(),
		ArtworkTotal:    total,
		ArtworkComplete: 0,
		StatusText:      "Downloading artwork...",
	})

	var wg sync.WaitGroup

	for _, job := range queue {
		if s.isCancelled() {
			break
		}

		wg.Add(1)
		go func(j artworkJob) {
			defer wg.Done()

			// Acquire semaphore (limit concurrent downloads)
			s.artworkSem <- struct{}{}
			defer func() { <-s.artworkSem }()

			if s.isCancelled() {
				return
			}

			// Download artwork (silent on failure)
			s.metadata.DownloadArtwork(j.gameCRC, j.gameName)

			// Update progress
			s.mu.Lock()
			s.artworkComplete++
			complete := s.artworkComplete
			s.mu.Unlock()

			s.sendProgress(ScanProgress{
				Phase:           ScanPhaseArtwork,
				Progress:        float64(complete) / float64(total),
				GamesFound:      s.gamesCount(),
				ArtworkTotal:    total,
				ArtworkComplete: complete,
				StatusText:      "Downloading artwork...",
			})
		}(job)
	}

	wg.Wait()
}

// isPathExcluded checks if a path is excluded
func (s *Scanner) isPathExcluded(path string) bool {
	if s.excludedPaths[path] {
		return true
	}
	// Check parent paths
	for excluded := range s.excludedPaths {
		if strings.HasPrefix(path, excluded+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// isCancelled checks if the scanner was cancelled
func (s *Scanner) isCancelled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelled
}

// gamesCount returns the number of discovered games (thread-safe)
func (s *Scanner) gamesCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.games)
}

// getErrors returns a copy of the errors slice (thread-safe)
func (s *Scanner) getErrors() []error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Return a copy to avoid race on the slice
	errs := make([]error, len(s.errors))
	copy(errs, s.errors)
	return errs
}

// sendProgress sends a progress update (non-blocking)
func (s *Scanner) sendProgress(p ScanProgress) {
	select {
	case s.progress <- p:
	default:
		// Progress channel full, skip this update
	}
}

// archiveExtensions are always supported for scanning regardless of system
var archiveExtensions = []string{".zip", ".7z", ".gz", ".tar.gz", ".rar"}

// isSupportedExtension checks if a file extension is supported
func (s *Scanner) isSupportedExtension(ext string) bool {
	for _, a := range archiveExtensions {
		if ext == a {
			return true
		}
	}
	for _, e := range s.extensions {
		if ext == strings.ToLower(e) {
			return true
		}
	}
	return false
}
