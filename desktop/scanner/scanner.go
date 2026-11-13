// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package scanner

import (
	"fmt"
	"hash/crc32"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/artwork"
	"github.com/user-none/eblitui/desktop/signing"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/romloader"
)

// ScanPhase represents the current scanning phase
type ScanPhase int

const (
	ScanPhaseDiscovery ScanPhase = iota
	ScanPhaseArtwork
)

// ScanProgress represents progress updates from the scanner
type ScanProgress struct {
	Phase            ScanPhase
	Progress         float64 // 0.0 to 1.0
	GamesFound       int
	DownloadTotal    int
	DownloadComplete int
	StatusText       string
}

// ScanResult represents the final scan result
type ScanResult struct {
	Errors    []error
	Cancelled bool
}

// Sources are where a scan downloads artwork from, and which artwork. An
// empty URL turns artwork downloads off; the app leaves it empty when the
// emulator does not publish artwork.
type Sources struct {
	ArtworkURL      string   // HTTPS base URL of the artwork site
	ArtworkKeys     []string // keys trusted to sign the artwork index, as algorithm:base64
	ArtworkDownload string   // which images to download: a storage.ArtworkDownload* setting
}

// Scanner handles ROM scanning in the background
type Scanner struct {
	// Configuration
	directories   []storage.ScanDirectory
	excludedPaths map[string]bool
	existingGames map[string]*storage.GameEntry // Full existing entries to preserve user data
	extensions    []string                      // Supported ROM file extensions

	// Disc-based systems identify and group games by the on-disc
	// product number instead of a whole-file CRC32. discID
	// reads the disc info from an opened disc.
	disc   bool
	discID coreif.DiscIdentifier

	// Where artwork is downloaded from
	sources Sources

	defaultConsoleID int

	// Channels
	progress chan ScanProgress
	done     chan ScanResult

	// Internal state
	mu          sync.Mutex
	games       map[string]*storage.GameEntry
	errors      []error
	cancelled   bool
	downloadSem chan struct{} // Semaphore for concurrent downloads
}

// resolvedJob is a download whose source and destination are known.
// download fetches and writes it; a failed download writes nothing and is
// retried by a later scan. url is the source, logged when it fails.
type resolvedJob struct {
	url      string
	download func() error
}

// NewScanner creates a new scanner instance. existing holds the entries
// whose user data a scan keeps when it finds their game: the library and
// the removed games.
func NewScanner(dirs []storage.ScanDirectory, excluded []string, existing map[string]*storage.GameEntry, extensions []string, defaultConsoleID int, disc bool, discID coreif.DiscIdentifier, sources Sources) *Scanner {
	excludedMap := make(map[string]bool)
	for _, p := range excluded {
		excludedMap[p] = true
	}

	return &Scanner{
		directories:      dirs,
		excludedPaths:    excludedMap,
		existingGames:    existing, // Keep full map to preserve user data
		extensions:       extensions,
		disc:             disc,
		discID:           discID,
		sources:          sources,
		defaultConsoleID: defaultConsoleID,
		progress:         make(chan ScanProgress, 10),
		done:             make(chan ScanResult, 1),
		games:            make(map[string]*storage.GameEntry),
		downloadSem:      make(chan struct{}, 4), // Limit to 4 concurrent downloads
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
			Errors:    s.getErrors(),
			Cancelled: true,
		}
		return
	}

	// Phase 3: Check the games' artwork against the artwork index and
	// download what is missing or changed
	s.sendProgress(ScanProgress{
		Phase:      ScanPhaseArtwork,
		StatusText: "Checking artwork...",
	})
	artworkJobs := s.resolveArtwork()
	s.downloadAssets(artworkJobs, ScanPhaseArtwork, "Downloading artwork...")

	// Send final result
	s.done <- ScanResult{
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

// processROM loads and processes a single ROM file. Disc-based systems
// and cartridge systems take different identity/grouping paths.
func (s *Scanner) processROM(path string) {
	if s.disc {
		s.processDisc(path)
		return
	}
	s.processCartridge(path)
}

// processCartridge handles a cartridge ROM: load via romloader (handles
// archives) and key by whole-file CRC32.
func (s *Scanner) processCartridge(path string) {
	romData, _, err := romloader.Load(path, s.extensions)
	if err != nil {
		// Skip unsupported formats silently
		return
	}
	gameID := fmt.Sprintf("%08x", crc32.ChecksumIEEE(romData))

	// Library or removed entry whose user data is kept
	existingEntry := s.existingGames[gameID]

	var entry *storage.GameEntry

	if existingEntry != nil {
		// Update existing entry - preserve user data, update the file
		// path (may have moved)
		entry = &storage.GameEntry{
			GameID:          gameID,
			File:            path,
			Favorite:        existingEntry.Favorite,
			PlayTimeSeconds: existingEntry.PlayTimeSeconds,
			LastPlayed:      existingEntry.LastPlayed,
			Added:           addedTime(existingEntry),
		}
	} else {
		entry = &storage.GameEntry{
			GameID: gameID,
			File:   path,
			Added:  time.Now().Unix(),
		}
	}

	entry.ConsoleID = s.defaultConsoleID

	s.mu.Lock()
	s.games[gameID] = entry
	s.mu.Unlock()
}

// processDisc handles a disc-based game. Identity and grouping use the
// on-disc product number (identical across every disc of a game); the
// disc number/total come from the IP device-info field. All discs of a
// game merge into one library entry keyed by the product number, which is
// also the key of the game's metadata.
func (s *Scanner) processDisc(path string) {
	if s.discID == nil {
		return
	}
	// Identifying a disc reads little of it, so it is opened without
	// preparing the whole image.
	d, derr := romloader.OpenDiscLazy(path)
	if derr != nil {
		return
	}
	di, ok := s.discID.DiscInfo(d)
	d.Close()
	// The product number comes from the disc header and becomes the game
	// key used in file paths, so clean it before any use.
	di.ProductNumber = storage.CleanGameKey(di.ProductNumber)
	if !ok || di.ProductNumber == "" {
		return
	}

	productNumber := di.ProductNumber
	filename := filepath.Base(path)
	idx0 := di.DiscNumber - 1
	if idx0 < 0 {
		idx0 = 0
	}

	// Per-disc name: the filename's trailing metadata (everything from
	// the first " (" - region, disc, and variant tags), else the full
	// extension-stripped filename when it carries no metadata. The game
	// title is shown from the game's metadata; this label only has to keep
	// each disc - including a same-serial variant such as a translation -
	// distinguishable in the disc list.
	perDiscName := discLabelFromFilename(filename)

	// Find-or-merge keyed by product number: an in-progress entry from another
	// disc this scan takes precedence over the library or removed entry.
	s.mu.Lock()
	entry := s.games[productNumber]
	s.mu.Unlock()

	existingEntry := s.existingGames[productNumber]

	if entry == nil {
		if existingEntry != nil {
			entry = &storage.GameEntry{
				GameID:          productNumber,
				Favorite:        existingEntry.Favorite,
				PlayTimeSeconds: existingEntry.PlayTimeSeconds,
				LastPlayed:      existingEntry.LastPlayed,
				Added:           addedTime(existingEntry),
				Discs:           append([]storage.GameDisc(nil), existingEntry.Discs...),
				SelectedDisc:    existingEntry.SelectedDisc,
			}
		} else {
			entry = &storage.GameEntry{
				GameID: productNumber,
				Added:  time.Now().Unix(),
			}
		}
	}

	entry.Discs = upsertDisc(entry.Discs, storage.GameDisc{Index: idx0, File: path, Name: perDiscName})
	entry.SelectedDisc = entry.SelectedDiscIndex()
	entry.ConsoleID = s.defaultConsoleID

	s.mu.Lock()
	s.games[productNumber] = entry
	s.mu.Unlock()
}

// addedTime returns when an existing entry was added to the library, or
// now when it has no added time: a removed game being restored is added
// again when it is found.
func addedTime(existing *storage.GameEntry) int64 {
	if existing.Added == 0 {
		return time.Now().Unix()
	}
	return existing.Added
}

// upsertDisc inserts the disc keyed by filename basename, keeping the
// slice sorted ascending by Index. A rescan of the same filename -
// including the same file found in another directory - updates its
// entry in place. A different filename sharing the game's product
// number (another disc, or a same-serial variant such as a translation
// that reports the same disc number) is appended as its own entry
// rather than replacing the existing one. Equal-Index entries keep
// insertion order, so a variant sorts directly after the disc it
// shadows.
func upsertDisc(discs []storage.GameDisc, d storage.GameDisc) []storage.GameDisc {
	for i := range discs {
		if filepath.Base(discs[i].File) == filepath.Base(d.File) {
			discs[i] = d
			return discs
		}
	}
	discs = append(discs, d)
	for i := len(discs) - 1; i > 0 && discs[i-1].Index > discs[i].Index; i-- {
		discs[i-1], discs[i] = discs[i], discs[i-1]
	}
	return discs
}

// discLabelFromFilename derives a disc's list label from its filename:
// the trailing metadata starting at the first " (" (region, disc, and
// variant tags such as "(En. Trans)"), or the full extension-stripped
// name when the filename carries no metadata. Two files sharing a
// product number and disc number - an original and a variant - stay
// distinguishable by their tags.
func discLabelFromFilename(filename string) string {
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	if i := strings.Index(stem, " ("); i >= 0 {
		return strings.TrimSpace(stem[i+1:])
	}
	return stem
}

// resolveArtwork checks the artwork of every game found by this scan against
// the artwork index and returns downloads for the images of the types the
// artwork download setting includes that are missing locally or differ
// from the index. No local file is removed. An index that cannot be
// fetched, does not verify against the artwork keys, or cannot be read is
// logged; no local file is changed then. With the setting at none the
// index is not fetched.
func (s *Scanner) resolveArtwork() []resolvedJob {
	baseURL := s.sources.ArtworkURL
	if baseURL == "" || s.sources.ArtworkDownload == storage.ArtworkDownloadNone || s.isCancelled() {
		return nil
	}
	keys, keyErrs := signing.ParseKeys(s.sources.ArtworkKeys)
	for _, e := range keyErrs {
		log.Printf("Artwork key: %v", e)
	}
	ix, err := artwork.FetchIndex(baseURL, keys)
	if err != nil {
		log.Printf("Artwork index %s: %v", baseURL, err)
		return nil
	}

	types := artwork.DownloadTypes(s.sources.ArtworkDownload)

	s.mu.Lock()
	gameIDs := make([]string, 0, len(s.games))
	for id := range s.games {
		gameIDs = append(gameIDs, id)
	}
	s.mu.Unlock()
	sort.Strings(gameIDs)

	var resolved []resolvedJob
	for _, gameID := range gameIDs {
		if s.isCancelled() {
			return nil
		}
		dir, err := storage.GetGameArtworkDir(gameID)
		if err != nil {
			continue
		}
		for _, f := range ix.Files(gameID, types) {
			if !artwork.NeedsDownload(dir, f) {
				continue
			}
			u, err := artwork.FileURL(baseURL, f.Path)
			if err != nil {
				log.Printf("Artwork %s: %v", gameID, err)
				continue
			}
			f := f
			resolved = append(resolved, resolvedJob{url: u, download: func() error {
				return artwork.Download(baseURL, dir, f)
			}})
		}
	}
	return resolved
}

// downloadAssets downloads resolved asset jobs in parallel with a semaphore.
func (s *Scanner) downloadAssets(jobs []resolvedJob, phase ScanPhase, statusText string) {
	total := len(jobs)
	if total == 0 {
		return
	}

	s.sendProgress(ScanProgress{
		Phase:            phase,
		Progress:         0,
		GamesFound:       s.gamesCount(),
		DownloadTotal:    total,
		DownloadComplete: 0,
		StatusText:       statusText,
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

			if err := j.download(); err != nil {
				log.Printf("Download failed: %s: %v", j.url, err)
			}

			s.mu.Lock()
			complete++
			c := complete
			s.mu.Unlock()

			s.sendProgress(ScanProgress{
				Phase:            phase,
				Progress:         float64(c) / float64(total),
				GamesFound:       s.gamesCount(),
				DownloadTotal:    total,
				DownloadComplete: c,
				StatusText:       statusText,
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
