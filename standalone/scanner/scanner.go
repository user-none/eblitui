package scanner

import (
	"fmt"
	"hash/crc32"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/user-none/eblitui/rdb"
	"github.com/user-none/eblitui/romloader"
	"github.com/user-none/eblitui/standalone/metadata"
	"github.com/user-none/eblitui/standalone/netutil"
	"github.com/user-none/eblitui/standalone/storage"
)

// ScanPhase represents the current scanning phase
type ScanPhase int

const (
	ScanPhaseInit ScanPhase = iota
	ScanPhaseDiscovery
	ScanPhaseArtwork
)

const (
	// Base URL for libretro-thumbnails repositories
	thumbnailBaseURL = "https://github.com/libretro-thumbnails"

	// Base URL for libretro-database CHT rumble files
	chtBaseURL = "https://raw.githubusercontent.com/libretro/libretro-database/master/cht"
)

// Artwork types in fallback order
var artworkTypes = []string{
	"Named_Boxarts",
	"Named_Titles",
	"Named_Snaps",
	"Named_Logos",
}

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
	metadata *metadata.MetadataManager

	// Channels
	progress chan ScanProgress
	done     chan ScanResult

	// Internal state
	mu           sync.Mutex
	games        map[string]*storage.GameEntry
	artworkQueue []artworkJob // Games that need artwork download
	rumbleQueue  []artworkJob // Games that need rumble file download
	errors       []error
	cancelled    bool
	downloadSem  chan struct{} // Semaphore for concurrent downloads (size 2)
}

// artworkJob represents a pending artwork or rumble download
type artworkJob struct {
	gameCRC    string
	gameName   string // No-Intro name from RDB
	variantIdx int    // Index into MetadataVariants for correct repo
}

// resolvedJob represents a download that has been matched against a listing
// and has a fully built download URL and save path.
type resolvedJob struct {
	downloadURL string
	savePath    string
}

// NewScanner creates a new scanner instance
func NewScanner(dirs []storage.ScanDirectory, excluded []string, existing map[string]*storage.GameEntry, rescanAll bool, extensions []string, md *metadata.MetadataManager) *Scanner {
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
		metadata:      md,
		progress:      make(chan ScanProgress, 10),
		done:          make(chan ScanResult, 1),
		games:         make(map[string]*storage.GameEntry),
		artworkQueue:  make([]artworkJob, 0),
		rumbleQueue:   make([]artworkJob, 0),
		downloadSem:   make(chan struct{}, 2), // Limit to 2 concurrent downloads
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
	s.cancelled = true
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

	// Phase 3: Resolve and download artwork, then rumble
	s.sendProgress(ScanProgress{
		Phase:      ScanPhaseArtwork,
		StatusText: "Resolving artwork...",
	})
	artworkJobs := s.resolveArtwork(s.artworkQueue)
	s.downloadAssets(artworkJobs, "Downloading artwork...")

	if !s.isCancelled() {
		s.sendProgress(ScanProgress{
			Phase:      ScanPhaseArtwork,
			StatusText: "Resolving rumble data...",
		})
		rumbleJobs := s.resolveRumble(s.rumbleQueue)
		s.downloadAssets(rumbleJobs, "Downloading rumble data...")
	}

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
			System:      existingEntry.System,
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
	game, variantIdx := s.metadata.LookupByCRC32(crcValue)
	if game != nil {
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

		if entry.System == "" && s.metadata.VariantCount() > 1 {
			entry.System = s.metadata.VariantName(variantIdx)
		}

		// Queue artwork download only if artwork doesn't exist
		artPath, _ := storage.GetGameArtworkPath(crcHex)
		if _, err := os.Stat(artPath); os.IsNotExist(err) {
			s.mu.Lock()
			s.artworkQueue = append(s.artworkQueue, artworkJob{
				gameCRC:    crcHex,
				gameName:   game.Name,
				variantIdx: variantIdx,
			})
			s.mu.Unlock()
		}

		// Queue rumble file download only if rumble file doesn't exist
		rumblePath, _ := storage.GetGameRumblePath(crcHex)
		if _, err := os.Stat(rumblePath); os.IsNotExist(err) {
			s.mu.Lock()
			s.rumbleQueue = append(s.rumbleQueue, artworkJob{
				gameCRC:    crcHex,
				gameName:   game.Name,
				variantIdx: variantIdx,
			})
			s.mu.Unlock()
		}
	}

	// No RDB match - queue artwork job using filename for fuzzy matching
	if game == nil {
		artPath, _ := storage.GetGameArtworkPath(crcHex)
		if _, err := os.Stat(artPath); os.IsNotExist(err) {
			// Use filename without extension (keep region parenthetical)
			artName := strings.TrimSuffix(filename, filepath.Ext(filename))
			s.mu.Lock()
			s.artworkQueue = append(s.artworkQueue, artworkJob{
				gameCRC:    crcHex,
				gameName:   artName,
				variantIdx: -1, // Non-RDB: try all variants
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

// resolveArtwork resolves artwork download URLs for queued games by fetching
// listings one artwork type at a time per variant. Games are removed from the
// need queue as they are matched. Returns early if the queue is emptied.
func (s *Scanner) resolveArtwork(queue []artworkJob) []resolvedJob {
	if len(queue) == 0 {
		return nil
	}

	var resolved []resolvedJob

	// Build per-variant sub-queues
	type variantQueue struct {
		jobs []artworkJob
	}
	byVariant := make(map[int]*variantQueue)
	var nonRDB []artworkJob

	for _, job := range queue {
		if job.variantIdx == -1 {
			nonRDB = append(nonRDB, job)
			continue
		}
		vq := byVariant[job.variantIdx]
		if vq == nil {
			vq = &variantQueue{}
			byVariant[job.variantIdx] = vq
		}
		vq.jobs = append(vq.jobs, job)
	}

	// Process each artType in priority order
	variantCount := s.metadata.VariantCount()
	for _, artType := range artworkTypes {
		if s.isCancelled() {
			break
		}

		// Per-artType listing cache: one slot per variant
		listings := make([]*ThumbnailListing, variantCount)
		fetched := make([]bool, variantCount)

		// Fetch listing for each variant that still has pending jobs
		for vi, vq := range byVariant {
			if len(vq.jobs) == 0 {
				continue
			}
			if s.isCancelled() {
				break
			}

			if !fetched[vi] {
				listings[vi] = fetchArtworkTypeListing(s.metadata.VariantThumbnailRepo(vi), artType)
				fetched[vi] = true
			}
			listing := listings[vi]
			if listing == nil {
				continue
			}

			repo := s.metadata.VariantThumbnailRepo(vi)
			remaining := vq.jobs[:0]
			for _, job := range vq.jobs {
				fileName, found := resolveArtworkNameForType(listing, artType, job.gameName)
				if found {
					savePath, err := storage.GetGameArtworkPath(job.gameCRC)
					if err != nil {
						continue
					}
					encodedName := url.PathEscape(strings.ReplaceAll(fileName, "&", "_"))
					dlURL := fmt.Sprintf("%s/%s/raw/refs/heads/master/%s/%s.png",
						thumbnailBaseURL, repo, artType, encodedName)
					resolved = append(resolved, resolvedJob{
						downloadURL: dlURL,
						savePath:    savePath,
					})
				} else {
					remaining = append(remaining, job)
				}
			}
			vq.jobs = remaining
		}

		// Non-RDB games: try all variants for this artType
		if len(nonRDB) > 0 && !s.isCancelled() {
			remaining := nonRDB[:0]
			for _, job := range nonRDB {
				matched := false
				for vi := 0; vi < variantCount; vi++ {
					if s.isCancelled() {
						remaining = append(remaining, job)
						matched = true
						break
					}

					if !fetched[vi] {
						listings[vi] = fetchArtworkTypeListing(s.metadata.VariantThumbnailRepo(vi), artType)
						fetched[vi] = true
					}
					listing := listings[vi]
					if listing == nil {
						continue
					}

					fileName, found := resolveArtworkNameForType(listing, artType, job.gameName)
					if found {
						savePath, err := storage.GetGameArtworkPath(job.gameCRC)
						if err != nil {
							continue
						}
						repo := s.metadata.VariantThumbnailRepo(vi)
						encodedName := url.PathEscape(strings.ReplaceAll(fileName, "&", "_"))
						dlURL := fmt.Sprintf("%s/%s/raw/refs/heads/master/%s/%s.png",
							thumbnailBaseURL, repo, artType, encodedName)
						resolved = append(resolved, resolvedJob{
							downloadURL: dlURL,
							savePath:    savePath,
						})
						matched = true
						break
					}
				}
				if !matched {
					remaining = append(remaining, job)
				}
			}
			nonRDB = remaining
		}

		// Check if all queues are empty
		allDone := len(nonRDB) == 0
		if allDone {
			for _, vq := range byVariant {
				if len(vq.jobs) > 0 {
					allDone = false
					break
				}
			}
		}
		if allDone {
			break
		}
	}

	return resolved
}

// resolveRumble resolves rumble download URLs for queued games by fetching
// a single listing per variant. Returns early if the queue is empty.
func (s *Scanner) resolveRumble(queue []artworkJob) []resolvedJob {
	if len(queue) == 0 {
		return nil
	}

	var resolved []resolvedJob

	// Group by variant
	byVariant := make(map[int][]artworkJob)
	for _, job := range queue {
		byVariant[job.variantIdx] = append(byVariant[job.variantIdx], job)
	}

	for vi, jobs := range byVariant {
		if s.isCancelled() {
			break
		}

		listing := fetchRumbleListing(s.metadata.VariantRDBName(vi))
		if listing == nil {
			continue
		}

		rdbName := s.metadata.VariantRDBName(vi)
		for _, job := range jobs {
			displayName := rdb.GetDisplayName(job.gameName)
			resolvedName, found := resolveRumbleName(listing, displayName)
			if !found {
				continue
			}

			savePath, err := storage.GetGameRumblePath(job.gameCRC)
			if err != nil {
				continue
			}

			encodedName := url.PathEscape(strings.ReplaceAll(resolvedName, "&", "_"))
			dlURL := fmt.Sprintf("%s/%s/%s (Rumbles).cht",
				chtBaseURL, url.PathEscape(rdbName), encodedName)
			resolved = append(resolved, resolvedJob{
				downloadURL: dlURL,
				savePath:    savePath,
			})
		}
	}

	return resolved
}

// downloadAssets downloads resolved asset jobs in parallel with a semaphore.
func (s *Scanner) downloadAssets(jobs []resolvedJob, statusText string) {
	total := len(jobs)
	if total == 0 {
		return
	}

	s.sendProgress(ScanProgress{
		Phase:           ScanPhaseArtwork,
		Progress:        0,
		GamesFound:      s.gamesCount(),
		ArtworkTotal:    total,
		ArtworkComplete: 0,
		StatusText:      statusText,
	})

	var wg sync.WaitGroup
	var complete int

	for _, job := range jobs {
		if s.isCancelled() {
			break
		}

		wg.Add(1)
		go func(j resolvedJob) {
			defer wg.Done()

			s.downloadSem <- struct{}{}
			defer func() { <-s.downloadSem }()

			if s.isCancelled() {
				return
			}

			netutil.DownloadToFile(j.downloadURL, j.savePath)

			s.mu.Lock()
			complete++
			c := complete
			s.mu.Unlock()

			s.sendProgress(ScanProgress{
				Phase:           ScanPhaseArtwork,
				Progress:        float64(c) / float64(total),
				GamesFound:      s.gamesCount(),
				ArtworkTotal:    total,
				ArtworkComplete: c,
				StatusText:      statusText,
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
