// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

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

	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/metadata"
	"github.com/user-none/eblitui/desktop/netutil"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/rdb"
	"github.com/user-none/eblitui/romloader"
)

// ScanPhase represents the current scanning phase
type ScanPhase int

const (
	ScanPhaseInit ScanPhase = iota
	ScanPhaseDiscovery
	ScanPhaseArtwork
	ScanPhaseRumble
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
	Phase            ScanPhase
	Progress         float64 // 0.0 to 1.0
	GamesFound       int
	DownloadTotal    int
	DownloadComplete int
	StatusText       string
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

	// Disc-based systems identify and group games by the on-disc
	// product number instead of a whole-file CRC32. discID
	// reads the disc info from an opened disc.
	disc   bool
	discID coreif.DiscIdentifier

	// Metadata
	metadata         *metadata.MetadataManager
	defaultConsoleID int

	// Channels
	progress chan ScanProgress
	done     chan ScanResult

	// Internal state
	mu             sync.Mutex
	games          map[string]*storage.GameEntry
	artworkQueue   []downloadJob // Games that need artwork download
	chtRumbleQueue []downloadJob // Games that need legacy rumble CHT file download
	erumbleQueue   []downloadJob // Games that need .erumble file download
	errors         []error
	cancelled      bool
	downloadSem    chan struct{} // Semaphore for concurrent downloads (size 2)
}

// downloadJob represents a pending asset download: artwork, a legacy
// rumble CHT file, or an .erumble file.
type downloadJob struct {
	gameCRC    string
	gameName   string // No-Intro name from RDB
	variantIdx int    // Index into MetadataVariants for correct repo
}

// resolvedJob represents a download that has been matched against a listing
// and has a fully built download URL and save path.
type resolvedJob struct {
	downloadURL string
	savePath    string
	// validateImage decodes the download as an image before writing it, so
	// corrupt downloads are not persisted. Used for artwork, not text assets.
	validateImage bool
}

// NewScanner creates a new scanner instance
func NewScanner(dirs []storage.ScanDirectory, excluded []string, existing map[string]*storage.GameEntry, rescanAll bool, extensions []string, md *metadata.MetadataManager, defaultConsoleID int, disc bool, discID coreif.DiscIdentifier) *Scanner {
	excludedMap := make(map[string]bool)
	for _, p := range excluded {
		excludedMap[p] = true
	}

	return &Scanner{
		directories:      dirs,
		excludedPaths:    excludedMap,
		existingGames:    existing, // Keep full map to preserve user data
		rescanAll:        rescanAll,
		extensions:       extensions,
		disc:             disc,
		discID:           discID,
		metadata:         md,
		defaultConsoleID: defaultConsoleID,
		progress:         make(chan ScanProgress, 10),
		done:             make(chan ScanResult, 1),
		games:            make(map[string]*storage.GameEntry),
		artworkQueue:     make([]downloadJob, 0),
		chtRumbleQueue:   make([]downloadJob, 0),
		erumbleQueue:     make([]downloadJob, 0),
		downloadSem:      make(chan struct{}, 2), // Limit to 2 concurrent downloads
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

	// Phase 3: Resolve and download artwork
	s.sendProgress(ScanProgress{
		Phase:      ScanPhaseArtwork,
		StatusText: "Resolving artwork...",
	})
	artworkJobs := s.resolveArtwork(s.artworkQueue)
	s.downloadAssets(artworkJobs, ScanPhaseArtwork, "Downloading artwork...")

	// Phase 4: Resolve and download rumble files, legacy CHT then .erumble
	if !s.isCancelled() {
		s.sendProgress(ScanProgress{
			Phase:      ScanPhaseRumble,
			StatusText: "Resolving rumble data...",
		})
		chtRumbleJobs := s.resolveCHTRumble(s.chtRumbleQueue)
		s.downloadAssets(chtRumbleJobs, ScanPhaseRumble, "Downloading rumble data...")
	}

	if !s.isCancelled() {
		s.sendProgress(ScanProgress{
			Phase:      ScanPhaseRumble,
			StatusText: "Resolving rumble files...",
		})
		erumbleJobs := s.resolveERumble(s.erumbleQueue)
		s.downloadAssets(erumbleJobs, ScanPhaseRumble, "Downloading rumble files...")
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
// archives), key by whole-file CRC32, and resolve metadata by CRC32.
func (s *Scanner) processCartridge(path string) {
	romData, fn, err := romloader.Load(path, s.extensions)
	if err != nil {
		// Skip unsupported formats silently
		return
	}
	crcValue := crc32.ChecksumIEEE(romData)
	crcHex := fmt.Sprintf("%08x", crcValue)
	filename := fn
	game, variantIdx := s.metadata.LookupByCRC32(crcValue)

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
			Serial:      existingEntry.Serial,
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

	// Game ID: for cartridges use the RDB serial when a match was found.
	if entry.Serial == "" && game != nil {
		entry.Serial = game.Serial
	}

	s.applyMetadata(entry, game, variantIdx, crcHex, filename)

	s.mu.Lock()
	s.games[crcHex] = entry
	s.mu.Unlock()
}

// processDisc handles a disc-based game. Identity and grouping use the
// on-disc product number (identical across every disc of a game); the
// disc number/total come from the IP device-info field. RDB metadata is
// resolved via scanner-derived serial candidates that compensate for the
// libretro Saturn-RDB publisher-prefix defect. All discs of a game merge
// into one library entry keyed by the product number.
func (s *Scanner) processDisc(path string) {
	if s.discID == nil {
		return
	}
	d, derr := romloader.OpenDisc(path)
	if derr != nil {
		return
	}
	di, ok := s.discID.DiscInfo(d)
	d.Close()
	if !ok || di.ProductNumber == "" {
		return
	}

	productNumber := di.ProductNumber
	filename := filepath.Base(path)
	idx0 := di.DiscNumber - 1
	if idx0 < 0 {
		idx0 = 0
	}

	game, variantIdx := s.lookupDiscMetadata(di)

	// Per-disc name: the filename's trailing metadata (everything from
	// the first " (" - region, disc, and variant tags), else the full
	// extension-stripped filename when it carries no metadata. The game
	// title is shown from RDB metadata on the detail screen; this label
	// only has to keep each disc - including a same-serial variant such
	// as a translation - distinguishable in the disc list.
	perDiscName := discLabelFromFilename(filename)

	// Find-or-merge keyed by product number: an in-progress entry from another
	// disc this scan takes precedence over the library entry.
	s.mu.Lock()
	entry := s.games[productNumber]
	s.mu.Unlock()

	existingEntry := s.existingGames[productNumber]

	if entry == nil {
		if existingEntry != nil {
			entry = &storage.GameEntry{
				CRC32:           productNumber,
				Favorite:        existingEntry.Favorite,
				PlayTimeSeconds: existingEntry.PlayTimeSeconds,
				LastPlayed:      existingEntry.LastPlayed,
				Added:           existingEntry.Added,
				Settings:        existingEntry.Settings,
				Missing:         false,
				Name:            existingEntry.Name,
				DisplayName:     existingEntry.DisplayName,
				Region:          existingEntry.Region,
				Developer:       existingEntry.Developer,
				Publisher:       existingEntry.Publisher,
				Genre:           existingEntry.Genre,
				Franchise:       existingEntry.Franchise,
				ESRBRating:      existingEntry.ESRBRating,
				ReleaseDate:     existingEntry.ReleaseDate,
				System:          existingEntry.System,
				Serial:          existingEntry.Serial,
				Discs:           append([]storage.GameDisc(nil), existingEntry.Discs...),
				SelectedDisc:    existingEntry.SelectedDisc,
			}
		} else {
			entry = &storage.GameEntry{
				CRC32:   productNumber,
				Added:   time.Now().Unix(),
				Missing: false,
			}
		}
	}

	entry.Discs = upsertDisc(entry.Discs, storage.GameDisc{Index: idx0, File: path, Name: perDiscName})
	if entry.SelectedDisc < 0 || entry.SelectedDisc >= len(entry.Discs) {
		entry.SelectedDisc = 0
	}

	// Game ID for disc systems is the on-disc product number.
	entry.Serial = productNumber

	// Entry-level name = matched RDB name with "(Disc N)" (and region)
	// stripped, else on-disc title, else cleaned filename. Seed before
	// applyMetadata so its empty-only fill does not use the full
	// per-disc RDB name.
	if entry.Name == "" {
		if game != nil && game.Name != "" {
			entry.Name = rdb.GetDisplayName(game.Name)
		} else if di.Title != "" {
			entry.Name = di.Title
		}
	}
	if entry.DisplayName == "" {
		if game != nil && game.Name != "" {
			entry.DisplayName = rdb.GetDisplayName(game.Name)
		} else if di.Title != "" {
			entry.DisplayName = di.Title
		}
	}

	s.applyMetadata(entry, game, variantIdx, productNumber, filename)

	s.mu.Lock()
	s.games[productNumber] = entry
	s.mu.Unlock()
}

// lookupDiscMetadata resolves the RDB row for one disc. The libretro
// Saturn RDB inconsistently drops the publisher prefix and appends a
// 0-based disc suffix for multi-disc titles; some entries keep the full
// serial. This derives ordered candidates from the on-disc product
// number and disc number and returns the first match. Used only to
// populate library metadata - never for identity or grouping.
func (s *Scanner) lookupDiscMetadata(di coreif.DiscInfo) (*rdb.Game, int) {
	for _, c := range discSerialCandidates(di) {
		if g, idx := s.metadata.LookupBySerial(c); g != nil {
			return g, idx
		}
	}
	return nil, -1
}

// discSerialCandidates builds the ordered RDB serial candidate list for
// a disc, most-specific first so a bare core serial cannot collide with
// another game's full serial. core = product number with a leading
// "<prefix>-" segment removed (else the product number);
// idx0 = DiscNumber-1.
func discSerialCandidates(di coreif.DiscInfo) []string {
	productNumber := di.ProductNumber
	core := productNumber
	if i := strings.IndexByte(productNumber, '-'); i >= 0 && i+1 < len(productNumber) {
		core = productNumber[i+1:]
	}
	idx0 := di.DiscNumber - 1
	if idx0 < 0 {
		idx0 = 0
	}
	var out []string
	if di.DiscTotal > 1 {
		out = append(out,
			fmt.Sprintf("%s-%d", core, idx0),
			fmt.Sprintf("%s-%d", productNumber, idx0),
			core,
			productNumber,
		)
	} else {
		out = append(out, productNumber, core)
	}
	// De-duplicate while preserving order (core may equal the product number).
	seen := make(map[string]bool, len(out))
	uniq := out[:0]
	for _, c := range out {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		uniq = append(uniq, c)
	}
	return uniq
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

// applyMetadata fills empty entry fields from an RDB match (when any),
// queues artwork/rumble downloads, resolves the achievements console ID,
// and falls back to the filename for Name/DisplayName. Only empty fields
// are filled so user data and caller-seeded values are preserved.
func (s *Scanner) applyMetadata(entry *storage.GameEntry, game *rdb.Game, variantIdx int, crcHex, filename string) {
	// Fill in metadata from the RDB match (if any) - only empty fields
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

		// Resolve per-variant console ID for achievements
		entry.ConsoleID = s.metadata.ResolveConsoleID(variantIdx, s.defaultConsoleID)

		// Queue artwork download only if artwork doesn't exist
		artPath, _ := storage.GetGameArtworkPath(crcHex)
		if _, err := os.Stat(artPath); os.IsNotExist(err) {
			s.mu.Lock()
			s.artworkQueue = append(s.artworkQueue, downloadJob{
				gameCRC:    crcHex,
				gameName:   game.Name,
				variantIdx: variantIdx,
			})
			s.mu.Unlock()
		}

		// Queue rumble file download only if rumble file doesn't exist
		rumblePath, _ := storage.GetGameCHTRumblePath(crcHex)
		if _, err := os.Stat(rumblePath); os.IsNotExist(err) {
			s.mu.Lock()
			s.chtRumbleQueue = append(s.chtRumbleQueue, downloadJob{
				gameCRC:    crcHex,
				gameName:   game.Name,
				variantIdx: variantIdx,
			})
			s.mu.Unlock()
		}
	}

	// Queue .erumble download when the shared <gameid>.erumble is absent.
	// Repo files are named by game ID (CRC32 / disc serial), so no RDB
	// match is needed; a -1 variantIdx tries every variant's repo dir.
	// Per-disc variant files resolve alongside the shared file. A core
	// that declares no RumbleRepoDir does not support rumble downloads,
	// so nothing is queued for it.
	if s.hasERumbleRepoDir(variantIdx) {
		erumblePath, _ := storage.GetGameRumblePath(crcHex)
		if _, err := os.Stat(erumblePath); os.IsNotExist(err) {
			s.mu.Lock()
			s.erumbleQueue = append(s.erumbleQueue, downloadJob{
				gameCRC:    crcHex,
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
			s.artworkQueue = append(s.artworkQueue, downloadJob{
				gameCRC:    crcHex,
				gameName:   artName,
				variantIdx: -1, // Non-RDB: try all variants
			})
			s.mu.Unlock()
		}
	}

	// Set default console ID for non-RDB matches
	if game == nil {
		entry.ConsoleID = s.defaultConsoleID
	}

	// Fallback to filename when no RDB match provided Name/DisplayName
	if entry.Name == "" {
		entry.Name = strings.TrimSuffix(filename, filepath.Ext(filename))
	}
	if entry.DisplayName == "" {
		entry.DisplayName = s.cleanDisplayName(filename)
	}
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
func (s *Scanner) resolveArtwork(queue []downloadJob) []resolvedJob {
	if len(queue) == 0 {
		return nil
	}

	var resolved []resolvedJob

	// Build per-variant sub-queues
	type variantQueue struct {
		jobs []downloadJob
	}
	byVariant := make(map[int]*variantQueue)
	var nonRDB []downloadJob

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
						downloadURL:   dlURL,
						savePath:      savePath,
						validateImage: true,
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
							downloadURL:   dlURL,
							savePath:      savePath,
							validateImage: true,
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

// resolveCHTRumble resolves legacy rumble CHT download URLs for queued
// games by fetching a single listing per variant. Returns early if the
// queue is empty.
func (s *Scanner) resolveCHTRumble(queue []downloadJob) []resolvedJob {
	if len(queue) == 0 {
		return nil
	}

	var resolved []resolvedJob

	// Group by variant
	byVariant := make(map[int][]downloadJob)
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

			savePath, err := storage.GetGameCHTRumblePath(job.gameCRC)
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

// hasERumbleRepoDir reports whether the variant declares a rumble repo
// directory. A -1 variantIdx (no RDB match) checks every variant. A core
// that declares none does not support rumble downloads.
func (s *Scanner) hasERumbleRepoDir(variantIdx int) bool {
	if variantIdx >= 0 {
		return s.metadata.VariantRumbleRepoDir(variantIdx) != ""
	}
	for vi := 0; vi < s.metadata.VariantCount(); vi++ {
		if s.metadata.VariantRumbleRepoDir(vi) != "" {
			return true
		}
	}
	return false
}

// resolveERumble resolves .erumble download URLs for queued games by
// fetching one repo directory listing per variant rumble repo dir.
// Files are matched by game ID, so a job matches at most one variant's
// listing; a -1 variantIdx (no RDB match) tries every variant that
// declares a repo dir. Multi-disc games queue once per disc; duplicates
// are dropped here.
func (s *Scanner) resolveERumble(queue []downloadJob) []resolvedJob {
	if len(queue) == 0 {
		return nil
	}

	var resolved []resolvedJob

	// One listing per distinct repo dir, fetched lazily. Variants may
	// share a dir.
	listings := make(map[string]*ERumbleListing)
	listingFor := func(repoDir string) *ERumbleListing {
		l, ok := listings[repoDir]
		if !ok {
			l = fetchERumbleListing(repoDir)
			listings[repoDir] = l
		}
		return l
	}

	variantCount := s.metadata.VariantCount()
	seen := make(map[string]bool, len(queue))

	for _, job := range queue {
		if s.isCancelled() {
			break
		}
		if seen[job.gameCRC] {
			continue
		}
		seen[job.gameCRC] = true

		var dirs []string
		if job.variantIdx >= 0 {
			if d := s.metadata.VariantRumbleRepoDir(job.variantIdx); d != "" {
				dirs = append(dirs, d)
			}
		} else {
			for vi := 0; vi < variantCount; vi++ {
				if d := s.metadata.VariantRumbleRepoDir(vi); d != "" {
					dirs = append(dirs, d)
				}
			}
		}

		for _, dir := range dirs {
			files := resolveERumbleFiles(listingFor(dir), job.gameCRC)
			if len(files) == 0 {
				continue
			}
			for _, f := range files {
				savePath, err := storage.GetGameRumblePath(strings.TrimSuffix(f.Name, erumbleExt))
				if err != nil {
					continue
				}
				// A per-disc variant may already exist even when the
				// shared file that queued the job does not.
				if _, err := os.Stat(savePath); err == nil {
					continue
				}
				resolved = append(resolved, resolvedJob{
					downloadURL: f.DownloadURL,
					savePath:    savePath,
				})
			}
			break
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

			if j.validateImage {
				netutil.DownloadImgToFile(j.downloadURL, j.savePath)
			} else {
				netutil.DownloadToFile(j.downloadURL, j.savePath)
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
