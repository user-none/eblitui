// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
)

// libraryData is the contents of library.json.
type libraryData struct {
	Version         int                   `json:"version"`
	ScanDirectories []ScanDirectory       `json:"scanDirectories"`
	ExcludedPaths   []string              `json:"excludedPaths"`
	Games           map[string]*GameEntry `json:"games"` // Game ID -> entry
}

// Library is the game library, safe for use from any goroutine. Its
// contents are only reachable through its methods: reads return copies and
// every change is a single locked operation. A library loaded from or
// created in library.json saves itself in the background after each
// change; one from DefaultLibrary is never written.
type Library struct {
	mu    sync.RWMutex
	data  libraryData
	saver *saver // nil when the library is not persisted
}

// DefaultLibrary returns an empty library that is never written to disk.
// It is used where a library is needed but must not replace library.json,
// such as while the error screen is shown.
func DefaultLibrary() *Library {
	return &Library{data: defaultLibraryData()}
}

// defaultLibraryData returns empty library contents at the current version.
func defaultLibraryData() libraryData {
	return libraryData{
		Version:         libraryVersion,
		ScanDirectories: []ScanDirectory{},
		ExcludedPaths:   []string{},
		Games:           make(map[string]*GameEntry),
	}
}

// newPersistedLibrary returns a library holding data that saves itself to
// path after each change.
func newPersistedLibrary(data libraryData, path string) *Library {
	lib := &Library{data: data}
	lib.saver = newSaver(path, 0644, lib.encode)
	return lib
}

// encode returns the library's contents as JSON.
func (lib *Library) encode() ([]byte, error) {
	lib.mu.RLock()
	defer lib.mu.RUnlock()
	return json.MarshalIndent(&lib.data, "", "  ")
}

// changed schedules a save after a change. The caller must not hold mu.
func (lib *Library) changed() {
	if lib.saver != nil {
		lib.saver.request()
	}
}

// Flush waits until every change has been written. Call before exit.
func (lib *Library) Flush() {
	if lib.saver != nil {
		lib.saver.flush()
	}
}

// LoadLibrary loads the library from library.json. If the file doesn't
// exist, it returns an empty library that is saved on its first change.
// If the file is corrupted, it returns an error.
func LoadLibrary() (*Library, error) {
	path, err := GetLibraryPath()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return newPersistedLibrary(defaultLibraryData(), path), nil
	}

	var data libraryData
	if err := ReadJSON(path, &data); err != nil {
		return nil, err
	}
	if data.Games == nil {
		data.Games = make(map[string]*GameEntry)
	}

	// Apply any migration for older library versions, saving the result so
	// the migration runs once
	migrated := migrateLibrary(&data, path)

	// Silently fix invalid game entry fields
	sanitizeGameEntries(data.Games)

	lib := newPersistedLibrary(data, path)
	if migrated {
		lib.changed()
	}
	return lib, nil
}

// ResetLibrary deletes library.json and returns a new empty library that
// is saved to it.
func ResetLibrary() (*Library, error) {
	path, err := GetLibraryPath()
	if err != nil {
		return nil, err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	lib := newPersistedLibrary(defaultLibraryData(), path)
	lib.changed()
	return lib, nil
}

// CreateLibraryIfMissing creates a default library.json if it doesn't exist
func CreateLibraryIfMissing() error {
	path, err := GetLibraryPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		data := defaultLibraryData()
		return AtomicWriteJSON(path, &data)
	}

	return nil
}

// libraryVersion is the current library.json format version. Version 2
// moved per-game settings out of the library into per-game settings files.
// Version 3 renamed each entry's key field from crc32 to gameId and dropped
// the metadata fields, which now come from the core's metadata provider.
const libraryVersion = 3

// migrateLibrary updates library contents loaded from path from an older
// version to the current one and reports whether it changed anything.
// Version 0 is treated as version 1. Version 1 kept each game's save slot
// in the library under "settings"; it moves to the game's settings file.
// Version 2 entries need no conversion: their dropped fields are not
// decoded, and sanitizing sets each GameID from its map key; saving the
// migrated library removes the old fields from the file.
func migrateLibrary(data *libraryData, path string) bool {
	migrated := false
	if data.Version == 0 {
		data.Version = 1
		migrated = true
	}
	if data.Version == 1 {
		migrateSaveSlots(path)
		data.Version = 2
		migrated = true
	}
	if data.Version == 2 {
		data.Version = 3
		migrated = true
	}
	return migrated
}

// migrateSaveSlots copies each game's version 1 save slot from the library
// file at path into the game's settings file, unless that file already has
// a slot. Game keys that are not safe file names are skipped. Failures are
// logged and that game is skipped.
func migrateSaveSlots(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("Failed to read library for save slot migration: %v", err)
		return
	}
	slots, err := legacySaveSlots(data)
	if err != nil {
		log.Printf("Failed to parse library for save slot migration: %v", err)
		return
	}

	for key, slot := range slots {
		if CleanGameKey(key) != key {
			continue
		}
		settings, err := LoadGameSettings(key)
		if err != nil {
			log.Printf("Failed to load game settings for %s: %v", key, err)
			continue
		}
		if settings.SaveStates.Slot != 0 {
			continue
		}
		settings.SaveStates.Slot = slot
		if err := SaveGameSettings(key, settings); err != nil {
			log.Printf("Failed to save game settings for %s: %v", key, err)
		}
	}
}

// legacySaveSlots returns the version 1 per-game save slots from a raw
// library file, keyed by game key. Only slots 1-9 are returned; slot 0 is
// the default and needs no migration.
func legacySaveSlots(data []byte) (map[string]int, error) {
	var raw struct {
		Games map[string]struct {
			Settings struct {
				SaveSlot int `json:"saveSlot"`
			} `json:"settings"`
		} `json:"games"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	slots := make(map[string]int)
	for key, game := range raw.Games {
		if slot := game.Settings.SaveSlot; slot >= 1 && slot <= 9 {
			slots[key] = slot
		}
	}
	return slots, nil
}

// GameInfo returns the title and region code a game is sorted and
// searched by.
type GameInfo func(game *GameEntry) (title, region string)

// clone returns a copy of g that shares nothing with it.
func (g *GameEntry) clone() GameEntry {
	c := *g
	c.Discs = append([]GameDisc(nil), g.Discs...)
	return c
}

// Game returns a copy of the game with the given ID.
func (lib *Library) Game(gameID string) (GameEntry, bool) {
	lib.mu.RLock()
	defer lib.mu.RUnlock()
	game, ok := lib.data.Games[gameID]
	if !ok {
		return GameEntry{}, false
	}
	return game.clone(), true
}

// HasGame reports whether the library holds the game with the given ID.
func (lib *Library) HasGame(gameID string) bool {
	lib.mu.RLock()
	defer lib.mu.RUnlock()
	_, ok := lib.data.Games[gameID]
	return ok
}

// Games returns copies of every game, keyed by game ID.
func (lib *Library) Games() map[string]GameEntry {
	lib.mu.RLock()
	defer lib.mu.RUnlock()
	games := make(map[string]GameEntry, len(lib.data.Games))
	for key, game := range lib.data.Games {
		games[key] = game.clone()
	}
	return games
}

// GameCount returns the number of games in the library
func (lib *Library) GameCount() int {
	lib.mu.RLock()
	defer lib.mu.RUnlock()
	return len(lib.data.Games)
}

// AddGame adds or replaces a game entry, keyed by its GameID.
func (lib *Library) AddGame(entry GameEntry) {
	lib.mu.Lock()
	c := entry.clone()
	lib.data.Games[c.GameID] = &c
	lib.mu.Unlock()
	lib.changed()
}

// update applies fn to the game with the given ID and saves, reporting
// whether the game exists.
func (lib *Library) update(gameID string, fn func(*GameEntry)) bool {
	lib.mu.Lock()
	game, ok := lib.data.Games[gameID]
	if ok {
		fn(game)
	}
	lib.mu.Unlock()
	if ok {
		lib.changed()
	}
	return ok
}

// ToggleFavorite flips the game's favorite mark and returns the new value.
func (lib *Library) ToggleFavorite(gameID string) bool {
	var favorite bool
	lib.update(gameID, func(g *GameEntry) {
		g.Favorite = !g.Favorite
		favorite = g.Favorite
	})
	return favorite
}

// SetSelectedDisc sets the disc the game launches with.
func (lib *Library) SetSelectedDisc(gameID string, idx int) {
	lib.update(gameID, func(g *GameEntry) { g.SelectedDisc = idx })
}

// SetLastPlayed records when the game was last played, as a Unix time.
func (lib *Library) SetLastPlayed(gameID string, unix int64) {
	lib.update(gameID, func(g *GameEntry) { g.LastPlayed = unix })
}

// AddPlayTime adds seconds to the game's total play time.
func (lib *Library) AddPlayTime(gameID string, seconds int64) {
	lib.update(gameID, func(g *GameEntry) { g.PlayTimeSeconds += seconds })
}

// RemoveDisc removes the game's Discs row at idx; see GameEntry.RemoveDisc.
func (lib *Library) RemoveDisc(gameID string, idx int) {
	lib.update(gameID, func(g *GameEntry) { g.RemoveDisc(idx) })
}

// sortKey is the title and region a game is sorted by, lowercased once
// per sort.
type sortKey struct {
	title  string
	region string
}

// sortGames orders games by sortBy, using info for the title and region.
// Unknown sortBy values sort by title.
func sortGames(games []GameEntry, sortBy string, info GameInfo) {
	keys := make(map[string]sortKey, len(games))
	for i := range games {
		title, region := info(&games[i])
		keys[games[i].GameID] = sortKey{title: strings.ToLower(title), region: region}
	}
	byTitle := func(a, b *GameEntry) bool {
		return compareGamesForSort(a, b, keys[a.GameID], keys[b.GameID])
	}

	switch sortBy {
	case "lastPlayed":
		sort.Slice(games, func(i, j int) bool {
			// Primary: most recent first
			if games[i].LastPlayed != games[j].LastPlayed {
				return games[i].LastPlayed > games[j].LastPlayed
			}
			// Secondary: fall back to title ordering
			return byTitle(&games[i], &games[j])
		})
	case "playTime":
		sort.Slice(games, func(i, j int) bool {
			// Primary: most played first
			if games[i].PlayTimeSeconds != games[j].PlayTimeSeconds {
				return games[i].PlayTimeSeconds > games[j].PlayTimeSeconds
			}
			// Secondary: fall back to title ordering
			return byTitle(&games[i], &games[j])
		})
	default:
		sort.Slice(games, func(i, j int) bool {
			return byTitle(&games[i], &games[j])
		})
	}
}

// compareGamesForSort compares two games for sorting purposes. It compares
// by title (A-Z), then region, then game ID, which is unique.
func compareGamesForSort(a, b *GameEntry, ak, bk sortKey) bool {
	if ak.title != bk.title {
		return ak.title < bk.title
	}
	if ak.region != bk.region {
		return ak.region < bk.region
	}
	return a.GameID < b.GameID
}

// GamesSorted returns copies of the games sorted by sortBy, using info for
// each game's title and region.
func (lib *Library) GamesSorted(sortBy string, favoritesOnly bool, info GameInfo) []GameEntry {
	return lib.GamesSortedFiltered(sortBy, favoritesOnly, "", info)
}

// GamesSortedFiltered returns copies of the games filtered by search text
// and sorted by sortBy, using info for each game's title and region.
// Search is case-insensitive and matches against the title. Empty
// searchText returns all games. info is called without the library locked.
func (lib *Library) GamesSortedFiltered(sortBy string, favoritesOnly bool, searchText string, info GameInfo) []GameEntry {
	lib.mu.RLock()
	all := make([]GameEntry, 0, len(lib.data.Games))
	for _, game := range lib.data.Games {
		if favoritesOnly && !game.Favorite {
			continue
		}
		all = append(all, game.clone())
	}
	lib.mu.RUnlock()

	games := all[:0]
	searchLower := strings.ToLower(searchText)
	for i := range all {
		if searchText != "" {
			title, _ := info(&all[i])
			if !strings.Contains(strings.ToLower(title), searchLower) {
				continue
			}
		}
		games = append(games, all[i])
	}

	sortGames(games, sortBy, info)
	return games
}

// ScanDirectories returns a copy of the directories scanned for games.
func (lib *Library) ScanDirectories() []ScanDirectory {
	lib.mu.RLock()
	defer lib.mu.RUnlock()
	return append([]ScanDirectory(nil), lib.data.ScanDirectories...)
}

// ExcludedPaths returns a copy of the paths excluded from scans.
func (lib *Library) ExcludedPaths() []string {
	lib.mu.RLock()
	defer lib.mu.RUnlock()
	return append([]string(nil), lib.data.ExcludedPaths...)
}

// AddScanDirectory adds a directory to scan for ROMs
func (lib *Library) AddScanDirectory(path string, recursive bool) {
	lib.mu.Lock()
	for _, dir := range lib.data.ScanDirectories {
		if dir.Path == path {
			lib.mu.Unlock()
			return
		}
	}
	lib.data.ScanDirectories = append(lib.data.ScanDirectories, ScanDirectory{
		Path:      path,
		Recursive: recursive,
	})
	lib.mu.Unlock()
	lib.changed()
}

// RemoveScanDirectory removes a directory from the scan list
func (lib *Library) RemoveScanDirectory(path string) {
	lib.mu.Lock()
	removed := false
	for i, dir := range lib.data.ScanDirectories {
		if dir.Path == path {
			lib.data.ScanDirectories = append(lib.data.ScanDirectories[:i], lib.data.ScanDirectories[i+1:]...)
			removed = true
			break
		}
	}
	lib.mu.Unlock()
	if removed {
		lib.changed()
	}
}

// AddExcludedPath adds a path to the exclusion list
func (lib *Library) AddExcludedPath(path string) {
	lib.mu.Lock()
	for _, p := range lib.data.ExcludedPaths {
		if p == path {
			lib.mu.Unlock()
			return
		}
	}
	lib.data.ExcludedPaths = append(lib.data.ExcludedPaths, path)
	lib.mu.Unlock()
	lib.changed()
}

// RemoveExcludedPath removes a path from the exclusion list
func (lib *Library) RemoveExcludedPath(path string) {
	lib.mu.Lock()
	removed := false
	for i, p := range lib.data.ExcludedPaths {
		if p == path {
			lib.data.ExcludedPaths = append(lib.data.ExcludedPaths[:i], lib.data.ExcludedPaths[i+1:]...)
			removed = true
			break
		}
	}
	lib.mu.Unlock()
	if removed {
		lib.changed()
	}
}

// IsPathExcluded checks if a path is in the exclusion list
func (lib *Library) IsPathExcluded(path string) bool {
	lib.mu.RLock()
	defer lib.mu.RUnlock()
	for _, excluded := range lib.data.ExcludedPaths {
		if path == excluded || strings.HasPrefix(path, excluded+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
