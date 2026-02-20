package storage

import (
	"errors"
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

	// Apply any migration for older library versions
	library = migrateLibrary(library)

	// Silently fix invalid game entry fields
	SanitizeLibraryEntries(library)

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

// migrateLibrary handles any necessary migrations from older library versions
func migrateLibrary(library *Library) *Library {
	// Currently at version 1, no migrations needed
	if library.Version == 0 {
		library.Version = 1
	}

	return library
}

// AddGame adds or updates a game entry in the library
func (lib *Library) AddGame(entry *GameEntry) {
	if lib.Games == nil {
		lib.Games = make(map[string]*GameEntry)
	}
	lib.Games[entry.CRC32] = entry
}

// GetGame retrieves a game by CRC32
func (lib *Library) GetGame(gameCRC string) *GameEntry {
	if lib.Games == nil {
		return nil
	}
	return lib.Games[gameCRC]
}

// RemoveGame removes a game from the library
func (lib *Library) RemoveGame(gameCRC string) {
	if lib.Games != nil {
		delete(lib.Games, gameCRC)
	}
}

// GameCount returns the number of games in the library
func (lib *Library) GameCount() int {
	if lib.Games == nil {
		return 0
	}
	return len(lib.Games)
}

// GetGamesSorted returns a sorted slice of game entries
func (lib *Library) GetGamesSorted(sortBy string, favoritesOnly bool) []*GameEntry {
	if lib.Games == nil {
		return nil
	}

	games := make([]*GameEntry, 0, len(lib.Games))
	for _, game := range lib.Games {
		if favoritesOnly && !game.Favorite {
			continue
		}
		games = append(games, game)
	}

	switch sortBy {
	case "title":
		sort.Slice(games, func(i, j int) bool {
			return compareGamesForSort(games[i], games[j])
		})
	case "lastPlayed":
		sort.Slice(games, func(i, j int) bool {
			// Primary: most recent first
			if games[i].LastPlayed != games[j].LastPlayed {
				return games[i].LastPlayed > games[j].LastPlayed
			}
			// Secondary: fall back to title ordering
			return compareGamesForSort(games[i], games[j])
		})
	case "playTime":
		sort.Slice(games, func(i, j int) bool {
			// Primary: most played first
			if games[i].PlayTimeSeconds != games[j].PlayTimeSeconds {
				return games[i].PlayTimeSeconds > games[j].PlayTimeSeconds
			}
			// Secondary: fall back to title ordering
			return compareGamesForSort(games[i], games[j])
		})
	default:
		// Default to title sort
		sort.Slice(games, func(i, j int) bool {
			return compareGamesForSort(games[i], games[j])
		})
	}

	return games
}

// compareGamesForSort compares two games for sorting purposes.
// It compares by DisplayName (A-Z), then Region, then Name, then CRC32.
func compareGamesForSort(a, b *GameEntry) bool {
	// Compare by DisplayName (case-insensitive, A-Z)
	aName := strings.ToLower(a.DisplayName)
	bName := strings.ToLower(b.DisplayName)
	if aName != bName {
		return aName < bName
	}

	// Compare by Region (alphabetical: eu, jp, us)
	if a.Region != b.Region {
		return a.Region < b.Region
	}

	// Compare by full Name (No-Intro name, for revisions)
	aFullName := strings.ToLower(a.Name)
	bFullName := strings.ToLower(b.Name)
	if aFullName != bFullName {
		return aFullName < bFullName
	}

	// Final tiebreaker: CRC32 (guaranteed unique)
	return a.CRC32 < b.CRC32
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
func (lib *Library) UpdatePlayTime(gameCRC string, secondsPlayed int64) {
	if game := lib.GetGame(gameCRC); game != nil {
		game.PlayTimeSeconds += secondsPlayed
		game.LastPlayed = time.Now().Unix()
	}
}

// GetGamesSortedFiltered returns a sorted slice of game entries filtered by search text.
// Search is case-insensitive and matches against DisplayName and Name fields.
// Empty searchText returns all games (same as GetGamesSorted).
func (lib *Library) GetGamesSortedFiltered(sortBy string, favoritesOnly bool, searchText string) []*GameEntry {
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
		// Apply search filter if search text is provided
		if searchText != "" {
			displayLower := strings.ToLower(game.DisplayName)
			nameLower := strings.ToLower(game.Name)
			if !strings.Contains(displayLower, searchLower) && !strings.Contains(nameLower, searchLower) {
				continue
			}
		}
		games = append(games, game)
	}

	switch sortBy {
	case "title":
		sort.Slice(games, func(i, j int) bool {
			return compareGamesForSort(games[i], games[j])
		})
	case "lastPlayed":
		sort.Slice(games, func(i, j int) bool {
			// Primary: most recent first
			if games[i].LastPlayed != games[j].LastPlayed {
				return games[i].LastPlayed > games[j].LastPlayed
			}
			// Secondary: fall back to title ordering
			return compareGamesForSort(games[i], games[j])
		})
	case "playTime":
		sort.Slice(games, func(i, j int) bool {
			// Primary: most played first
			if games[i].PlayTimeSeconds != games[j].PlayTimeSeconds {
				return games[i].PlayTimeSeconds > games[j].PlayTimeSeconds
			}
			// Secondary: fall back to title ordering
			return compareGamesForSort(games[i], games[j])
		})
	default:
		// Default to title sort
		sort.Slice(games, func(i, j int) bool {
			return compareGamesForSort(games[i], games[j])
		})
	}

	return games
}
