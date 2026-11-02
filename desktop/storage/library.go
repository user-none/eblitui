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
	"time"
)

// LoadLibrary loads the library from library.json.
// If the file doesn't exist, it returns an empty library.
// If the file is corrupted, it returns an error.
func LoadLibrary() (*Library, error) {
	path, err := GetLibraryPath()
	if err != nil {
		return nil, err
	}

	// Check if file exists
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		// File doesn't exist, return empty library
		return DefaultLibrary(), nil
	}

	// Load and parse the file
	library := &Library{}
	if err := ReadJSON(path, library); err != nil {
		return nil, err
	}

	// Ensure Games map is initialized
	if library.Games == nil {
		library.Games = make(map[string]*GameEntry)
	}

	// Apply any migration for older library versions, saving the result so
	// the migration runs once
	migrated := migrateLibrary(library, path)

	// Silently fix invalid game entry fields
	SanitizeLibraryEntries(library)

	if migrated {
		if err := SaveLibrary(library); err != nil {
			log.Printf("Failed to save migrated library: %v", err)
		}
	}

	return library, nil
}

// SaveLibrary saves the library to library.json atomically
func SaveLibrary(library *Library) error {
	path, err := GetLibraryPath()
	if err != nil {
		return err
	}

	return AtomicWriteJSON(path, library)
}

// CreateLibraryIfMissing creates a default library.json if it doesn't exist
func CreateLibraryIfMissing() error {
	path, err := GetLibraryPath()
	if err != nil {
		return err
	}

	// Check if file exists
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		// Create default library
		return SaveLibrary(DefaultLibrary())
	}

	return nil
}

// DeleteLibrary removes the library.json file
func DeleteLibrary() error {
	path, err := GetLibraryPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// libraryVersion is the current library.json format version. Version 2
// moved per-game settings out of the library into per-game settings files.
// Version 3 renamed each entry's key field from crc32 to gameId and dropped
// the metadata fields, which now come from the core's metadata provider.
const libraryVersion = 3

// migrateLibrary updates a library loaded from path from an older version
// to the current one and reports whether it changed anything. Version 0
// is treated as version 1. Version 1 kept each game's save slot in the
// library under "settings"; it moves to the game's settings file. Version
// 2 entries need no conversion: their dropped fields are not decoded, and
// SanitizeLibraryEntries sets each GameID from its map key; saving the
// migrated library removes the old fields from the file.
func migrateLibrary(library *Library, path string) bool {
	migrated := false
	if library.Version == 0 {
		library.Version = 1
		migrated = true
	}
	if library.Version == 1 {
		migrateSaveSlots(path)
		library.Version = 2
		migrated = true
	}
	if library.Version == 2 {
		library.Version = 3
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

// AddGame adds or updates a game entry in the library
func (lib *Library) AddGame(entry *GameEntry) {
	if lib.Games == nil {
		lib.Games = make(map[string]*GameEntry)
	}
	lib.Games[entry.GameID] = entry
}

// GetGame retrieves a game by game ID
func (lib *Library) GetGame(gameID string) *GameEntry {
	if lib.Games == nil {
		return nil
	}
	return lib.Games[gameID]
}

// GameCount returns the number of games in the library
func (lib *Library) GameCount() int {
	if lib.Games == nil {
		return 0
	}
	return len(lib.Games)
}

// sortKey is the title and region a game is sorted by, lowercased once
// per sort.
type sortKey struct {
	title  string
	region string
}

// sortGames orders games by sortBy, using info for the title and region.
// Unknown sortBy values sort by title.
func sortGames(games []*GameEntry, sortBy string, info GameInfo) {
	keys := make(map[*GameEntry]sortKey, len(games))
	for _, g := range games {
		title, region := info(g)
		keys[g] = sortKey{title: strings.ToLower(title), region: region}
	}
	byTitle := func(a, b *GameEntry) bool {
		return compareGamesForSort(a, b, keys[a], keys[b])
	}

	switch sortBy {
	case "lastPlayed":
		sort.Slice(games, func(i, j int) bool {
			// Primary: most recent first
			if games[i].LastPlayed != games[j].LastPlayed {
				return games[i].LastPlayed > games[j].LastPlayed
			}
			// Secondary: fall back to title ordering
			return byTitle(games[i], games[j])
		})
	case "playTime":
		sort.Slice(games, func(i, j int) bool {
			// Primary: most played first
			if games[i].PlayTimeSeconds != games[j].PlayTimeSeconds {
				return games[i].PlayTimeSeconds > games[j].PlayTimeSeconds
			}
			// Secondary: fall back to title ordering
			return byTitle(games[i], games[j])
		})
	default:
		sort.Slice(games, func(i, j int) bool {
			return byTitle(games[i], games[j])
		})
	}
}

// GetGamesSorted returns a sorted slice of game entries, using info for
// each game's title and region.
func (lib *Library) GetGamesSorted(sortBy string, favoritesOnly bool, info GameInfo) []*GameEntry {
	return lib.GetGamesSortedFiltered(sortBy, favoritesOnly, "", info)
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

// AddScanDirectory adds a directory to scan for ROMs
func (lib *Library) AddScanDirectory(path string, recursive bool) {
	// Check if already exists
	for _, dir := range lib.ScanDirectories {
		if dir.Path == path {
			return // Already exists
		}
	}
	lib.ScanDirectories = append(lib.ScanDirectories, ScanDirectory{
		Path:      path,
		Recursive: recursive,
	})
}

// RemoveScanDirectory removes a directory from the scan list
func (lib *Library) RemoveScanDirectory(path string) {
	for i, dir := range lib.ScanDirectories {
		if dir.Path == path {
			lib.ScanDirectories = append(lib.ScanDirectories[:i], lib.ScanDirectories[i+1:]...)
			return
		}
	}
}

// AddExcludedPath adds a path to the exclusion list
func (lib *Library) AddExcludedPath(path string) {
	// Check if already exists
	for _, p := range lib.ExcludedPaths {
		if p == path {
			return
		}
	}
	lib.ExcludedPaths = append(lib.ExcludedPaths, path)
}

// RemoveExcludedPath removes a path from the exclusion list
func (lib *Library) RemoveExcludedPath(path string) {
	for i, p := range lib.ExcludedPaths {
		if p == path {
			lib.ExcludedPaths = append(lib.ExcludedPaths[:i], lib.ExcludedPaths[i+1:]...)
			return
		}
	}
}

// IsPathExcluded checks if a path is in the exclusion list
func (lib *Library) IsPathExcluded(path string) bool {
	for _, excluded := range lib.ExcludedPaths {
		if path == excluded || strings.HasPrefix(path, excluded+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// UpdatePlayTime adds play time to a game and updates last played
func (lib *Library) UpdatePlayTime(gameID string, secondsPlayed int64) {
	if game := lib.GetGame(gameID); game != nil {
		game.PlayTimeSeconds += secondsPlayed
		game.LastPlayed = time.Now().Unix()
	}
}

// GetGamesSortedFiltered returns a sorted slice of game entries filtered by
// search text, using info for each game's title and region. Search is
// case-insensitive and matches against the title. Empty searchText returns
// all games.
func (lib *Library) GetGamesSortedFiltered(sortBy string, favoritesOnly bool, searchText string, info GameInfo) []*GameEntry {
	if lib.Games == nil {
		return nil
	}

	// Normalize search text for case-insensitive matching
	searchLower := strings.ToLower(searchText)

	games := make([]*GameEntry, 0, len(lib.Games))
	for _, game := range lib.Games {
		if favoritesOnly && !game.Favorite {
			continue
		}
		if searchText != "" {
			title, _ := info(game)
			if !strings.Contains(strings.ToLower(title), searchLower) {
				continue
			}
		}
		games = append(games, game)
	}

	sortGames(games, sortBy, info)
	return games
}
