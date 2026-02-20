package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidFontSize(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{"exact preset 10", 10, 10},
		{"exact preset 14", 14, 14},
		{"exact preset 32", 32, 32},
		{"between 10 and 12 closer to 10", 10, 10},
		{"between 10 and 12 equidistant picks lower", 11, 10},
		{"between 14 and 16 closer to 14", 14, 14},
		{"between 14 and 16 closer to 16", 15, 14},
		{"between 16 and 18", 17, 16},
		{"between 20 and 24 closer to 20", 21, 20},
		{"between 20 and 24 closer to 24", 23, 24},
		{"below minimum", 1, 10},
		{"above maximum", 100, 32},
		{"zero", 0, 10},
		{"negative", -5, 10},
		{"exact preset 24", 24, 24},
		{"between 28 and 32", 30, 28},
		{"between 28 and 32 closer to 32", 31, 32},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidFontSize(tc.input)
			if got != tc.expected {
				t.Errorf("ValidFontSize(%d) = %d, want %d", tc.input, got, tc.expected)
			}
		})
	}
}

func TestDefaultConfigFontSize(t *testing.T) {
	config := DefaultConfig()
	if config.FontSize != 14 {
		t.Errorf("expected default font size 14, got %d", config.FontSize)
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Version != 1 {
		t.Errorf("expected version 1, got %d", config.Version)
	}
	if config.Window.Width != 900 {
		t.Errorf("expected window width 900, got %d", config.Window.Width)
	}
	if config.Window.Height != 650 {
		t.Errorf("expected window height 650, got %d", config.Window.Height)
	}
	if config.Audio.Volume != 1.0 {
		t.Errorf("expected volume 1.0, got %f", config.Audio.Volume)
	}
	if config.Library.ViewMode != "icon" {
		t.Errorf("expected view mode 'icon', got '%s'", config.Library.ViewMode)
	}
}

func TestDefaultLibrary(t *testing.T) {
	lib := DefaultLibrary()

	if lib.Version != 1 {
		t.Errorf("expected version 1, got %d", lib.Version)
	}
	if len(lib.Games) != 0 {
		t.Errorf("expected empty games map, got %d entries", len(lib.Games))
	}
	if len(lib.ScanDirectories) != 0 {
		t.Errorf("expected empty scan directories, got %d entries", len(lib.ScanDirectories))
	}
}

func TestAtomicWriteJSON(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test.json")

	data := struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}{
		Name:  "test",
		Value: 42,
	}

	// Write file
	if err := AtomicWriteJSON(path, data); err != nil {
		t.Fatalf("AtomicWriteJSON failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// Read back
	var result struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	if err := ReadJSON(path, &result); err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}

	if result.Name != data.Name || result.Value != data.Value {
		t.Errorf("data mismatch: expected %+v, got %+v", data, result)
	}

	// Verify temp file is cleaned up
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file was not cleaned up")
	}
}

func TestLibraryAddGetRemoveGame(t *testing.T) {
	lib := DefaultLibrary()

	game := &GameEntry{
		CRC32:       "12345678",
		File:        "/path/to/game.sms",
		DisplayName: "Test Game",
		Region:      "us",
	}

	// Add game
	lib.AddGame(game)

	if lib.GameCount() != 1 {
		t.Errorf("expected 1 game, got %d", lib.GameCount())
	}

	// Get game
	retrieved := lib.GetGame("12345678")
	if retrieved == nil {
		t.Fatal("game not found")
	}
	if retrieved.DisplayName != "Test Game" {
		t.Errorf("expected 'Test Game', got '%s'", retrieved.DisplayName)
	}

	// Remove game
	lib.RemoveGame("12345678")
	if lib.GameCount() != 0 {
		t.Errorf("expected 0 games after removal, got %d", lib.GameCount())
	}
}

func TestLibrarySorting(t *testing.T) {
	lib := DefaultLibrary()

	lib.AddGame(&GameEntry{CRC32: "1", DisplayName: "Zelda", PlayTimeSeconds: 100, LastPlayed: 1000})
	lib.AddGame(&GameEntry{CRC32: "2", DisplayName: "Alex Kidd", PlayTimeSeconds: 500, LastPlayed: 500})
	lib.AddGame(&GameEntry{CRC32: "3", DisplayName: "Sonic", PlayTimeSeconds: 300, LastPlayed: 2000})

	// Sort by title
	games := lib.GetGamesSorted("title", false)
	if len(games) != 3 {
		t.Fatalf("expected 3 games, got %d", len(games))
	}
	if games[0].DisplayName != "Alex Kidd" {
		t.Errorf("expected first game 'Alex Kidd', got '%s'", games[0].DisplayName)
	}
	if games[2].DisplayName != "Zelda" {
		t.Errorf("expected last game 'Zelda', got '%s'", games[2].DisplayName)
	}

	// Sort by play time
	games = lib.GetGamesSorted("playTime", false)
	if games[0].DisplayName != "Alex Kidd" { // Most played (500s)
		t.Errorf("expected most played 'Alex Kidd', got '%s'", games[0].DisplayName)
	}

	// Sort by last played
	games = lib.GetGamesSorted("lastPlayed", false)
	if games[0].DisplayName != "Sonic" { // Most recent (2000)
		t.Errorf("expected most recent 'Sonic', got '%s'", games[0].DisplayName)
	}
}

func TestLibrarySortingStability(t *testing.T) {
	// Test that sorting is stable when primary sort values are equal.
	// Games with the same display name should be sorted by region, then by
	// No-Intro name (to distinguish revisions), then by CRC32.
	lib := DefaultLibrary()

	// Add games with same display name but different regions and revisions
	lib.AddGame(&GameEntry{
		CRC32:       "C",
		DisplayName: "Zillion",
		Name:        "Zillion (Japan) (Rev 2)",
		Region:      "jp",
	})
	lib.AddGame(&GameEntry{
		CRC32:       "A",
		DisplayName: "Zillion",
		Name:        "Zillion (USA)",
		Region:      "us",
	})
	lib.AddGame(&GameEntry{
		CRC32:       "B",
		DisplayName: "Zillion",
		Name:        "Zillion (Europe)",
		Region:      "eu",
	})
	lib.AddGame(&GameEntry{
		CRC32:       "D",
		DisplayName: "Zillion",
		Name:        "Zillion (Japan) (Rev 1)",
		Region:      "jp",
	})
	lib.AddGame(&GameEntry{
		CRC32:       "E",
		DisplayName: "Alex Kidd",
		Name:        "Alex Kidd (USA)",
		Region:      "us",
	})

	// Sort by title multiple times and verify order is consistent
	for i := 0; i < 5; i++ {
		games := lib.GetGamesSorted("title", false)
		if len(games) != 5 {
			t.Fatalf("expected 5 games, got %d", len(games))
		}
		// Alex Kidd should be first (alphabetically)
		if games[0].DisplayName != "Alex Kidd" {
			t.Errorf("iteration %d: expected first game 'Alex Kidd', got '%s'", i, games[0].DisplayName)
		}
		// Zillion games should be sorted by region (eu, jp, us), then by Name
		// EU version
		if games[1].Region != "eu" {
			t.Errorf("iteration %d: expected second game region 'eu', got '%s'", i, games[1].Region)
		}
		// JP versions (Rev 1 before Rev 2 alphabetically)
		if games[2].Name != "Zillion (Japan) (Rev 1)" {
			t.Errorf("iteration %d: expected third game 'Zillion (Japan) (Rev 1)', got '%s'", i, games[2].Name)
		}
		if games[3].Name != "Zillion (Japan) (Rev 2)" {
			t.Errorf("iteration %d: expected fourth game 'Zillion (Japan) (Rev 2)', got '%s'", i, games[3].Name)
		}
		// US version
		if games[4].Region != "us" {
			t.Errorf("iteration %d: expected fifth game region 'us', got '%s'", i, games[4].Region)
		}
	}

	// Test with lastPlayed - games with same timestamp should have stable order
	lib2 := DefaultLibrary()
	lib2.AddGame(&GameEntry{CRC32: "C", DisplayName: "Game C", Name: "Game C (JP)", Region: "jp", LastPlayed: 1000})
	lib2.AddGame(&GameEntry{CRC32: "A", DisplayName: "Game A", Name: "Game A (US)", Region: "us", LastPlayed: 1000})
	lib2.AddGame(&GameEntry{CRC32: "B", DisplayName: "Game B", Name: "Game B (EU)", Region: "eu", LastPlayed: 1000})

	for i := 0; i < 5; i++ {
		games := lib2.GetGamesSorted("lastPlayed", false)
		// With equal lastPlayed, should fall back to title order (alphabetical)
		if games[0].DisplayName != "Game A" {
			t.Errorf("lastPlayed iteration %d: expected first 'Game A', got '%s'", i, games[0].DisplayName)
		}
		if games[1].DisplayName != "Game B" {
			t.Errorf("lastPlayed iteration %d: expected second 'Game B', got '%s'", i, games[1].DisplayName)
		}
		if games[2].DisplayName != "Game C" {
			t.Errorf("lastPlayed iteration %d: expected third 'Game C', got '%s'", i, games[2].DisplayName)
		}
	}

	// Test with playTime - games with same play time should have stable order
	lib3 := DefaultLibrary()
	lib3.AddGame(&GameEntry{CRC32: "C", DisplayName: "Game C", Name: "Game C (JP)", Region: "jp", PlayTimeSeconds: 100})
	lib3.AddGame(&GameEntry{CRC32: "A", DisplayName: "Game A", Name: "Game A (US)", Region: "us", PlayTimeSeconds: 100})
	lib3.AddGame(&GameEntry{CRC32: "B", DisplayName: "Game B", Name: "Game B (EU)", Region: "eu", PlayTimeSeconds: 100})

	for i := 0; i < 5; i++ {
		games := lib3.GetGamesSorted("playTime", false)
		// With equal playTime, should fall back to title order (alphabetical)
		if games[0].DisplayName != "Game A" {
			t.Errorf("playTime iteration %d: expected first 'Game A', got '%s'", i, games[0].DisplayName)
		}
		if games[1].DisplayName != "Game B" {
			t.Errorf("playTime iteration %d: expected second 'Game B', got '%s'", i, games[1].DisplayName)
		}
		if games[2].DisplayName != "Game C" {
			t.Errorf("playTime iteration %d: expected third 'Game C', got '%s'", i, games[2].DisplayName)
		}
	}
}

func TestLibraryFavoritesFilter(t *testing.T) {
	lib := DefaultLibrary()

	lib.AddGame(&GameEntry{CRC32: "1", DisplayName: "Game1", Favorite: true})
	lib.AddGame(&GameEntry{CRC32: "2", DisplayName: "Game2", Favorite: false})
	lib.AddGame(&GameEntry{CRC32: "3", DisplayName: "Game3", Favorite: true})

	// All games
	all := lib.GetGamesSorted("title", false)
	if len(all) != 3 {
		t.Errorf("expected 3 games, got %d", len(all))
	}

	// Favorites only
	favorites := lib.GetGamesSorted("title", true)
	if len(favorites) != 2 {
		t.Errorf("expected 2 favorites, got %d", len(favorites))
	}
}

func TestLibraryScanDirectories(t *testing.T) {
	lib := DefaultLibrary()

	lib.AddScanDirectory("/path/to/roms", true)
	lib.AddScanDirectory("/path/to/more", false)

	if len(lib.ScanDirectories) != 2 {
		t.Errorf("expected 2 directories, got %d", len(lib.ScanDirectories))
	}

	// Add duplicate (should be ignored)
	lib.AddScanDirectory("/path/to/roms", false)
	if len(lib.ScanDirectories) != 2 {
		t.Errorf("duplicate should be ignored, got %d directories", len(lib.ScanDirectories))
	}

	// Remove directory
	lib.RemoveScanDirectory("/path/to/roms")
	if len(lib.ScanDirectories) != 1 {
		t.Errorf("expected 1 directory after removal, got %d", len(lib.ScanDirectories))
	}
}

func TestLibraryExcludedPaths(t *testing.T) {
	lib := DefaultLibrary()

	lib.AddExcludedPath("/path/to/exclude")
	lib.AddExcludedPath("/path/to/file.sms")

	if len(lib.ExcludedPaths) != 2 {
		t.Errorf("expected 2 excluded paths, got %d", len(lib.ExcludedPaths))
	}

	// Test path exclusion
	if !lib.IsPathExcluded("/path/to/exclude") {
		t.Error("directory should be excluded")
	}
	if !lib.IsPathExcluded("/path/to/exclude/subdir/file.sms") {
		t.Error("subdirectory should be excluded")
	}
	if lib.IsPathExcluded("/path/to/other") {
		t.Error("/path/to/other should not be excluded")
	}

	// Remove excluded path
	lib.RemoveExcludedPath("/path/to/exclude")
	if len(lib.ExcludedPaths) != 1 {
		t.Errorf("expected 1 excluded path after removal, got %d", len(lib.ExcludedPaths))
	}
}

func TestApplyMissingDefaultsMigration(t *testing.T) {
	// Test migration from version 0 when keys are absent
	config := &Config{
		Version: 0,
		Audio:   AudioConfig{Volume: 0}, // Volume=0 is valid (0% volume)
		Window:  WindowConfig{},
		Library: LibraryView{},
	}

	// Simulate: only audio.volume was present in JSON (as 0)
	presentKeys := map[string]bool{
		"version":      true,
		"audio.volume": true,
	}
	ApplyMissingDefaults(config, presentKeys)

	// version was present as 0 — not overwritten by ApplyMissingDefaults
	if config.Version != 0 {
		t.Errorf("expected version 0 (present in JSON), got %d", config.Version)
	}
	if config.Audio.Volume != 0 {
		t.Errorf("expected volume 0 (present in JSON, 0%% is valid), got %f", config.Audio.Volume)
	}
	if config.Window.Width != 900 {
		t.Errorf("expected width 900 after defaulting, got %d", config.Window.Width)
	}
	if config.Library.ViewMode != "icon" {
		t.Errorf("expected view mode 'icon' after defaulting, got '%s'", config.Library.ViewMode)
	}
}

func TestApplyMissingDefaultsPreservesZeroVolume(t *testing.T) {
	// Volume=0 is a valid user setting (0% volume), must not be overwritten
	config := &Config{
		Version: 1,
		Audio:   AudioConfig{Volume: 0.0},
		Window:  WindowConfig{Width: 900, Height: 650},
		Library: LibraryView{ViewMode: "icon", SortBy: "title"},
		Theme:   "Default",
	}

	// All keys present in JSON
	presentKeys := map[string]bool{
		"version": true, "theme": true, "fontSize": true,
		"audio.volume": true, "window.width": true, "window.height": true,
		"library.viewMode": true, "library.sortBy": true,
		"rewind.bufferSizeMB": true, "rewind.frameStep": true,
	}
	ApplyMissingDefaults(config, presentKeys)

	if config.Audio.Volume != 0.0 {
		t.Errorf("expected volume 0.0 to be preserved, got %f", config.Audio.Volume)
	}
}

func TestUpdatePlayTime(t *testing.T) {
	lib := DefaultLibrary()

	lib.AddGame(&GameEntry{
		CRC32:           "12345678",
		DisplayName:     "Test Game",
		PlayTimeSeconds: 100,
	})

	lib.UpdatePlayTime("12345678", 50)

	game := lib.GetGame("12345678")
	if game.PlayTimeSeconds != 150 {
		t.Errorf("expected 150 seconds, got %d", game.PlayTimeSeconds)
	}
	if game.LastPlayed == 0 {
		t.Error("LastPlayed should be updated")
	}
}

func TestGetGamesSortedFiltered(t *testing.T) {
	lib := DefaultLibrary()

	lib.AddGame(&GameEntry{CRC32: "1", DisplayName: "Sonic the Hedgehog", Name: "Sonic the Hedgehog (USA)"})
	lib.AddGame(&GameEntry{CRC32: "2", DisplayName: "Alex Kidd in Miracle World", Name: "Alex Kidd in Miracle World (Europe)"})
	lib.AddGame(&GameEntry{CRC32: "3", DisplayName: "Phantasy Star", Name: "Phantasy Star (Japan)"})
	lib.AddGame(&GameEntry{CRC32: "4", DisplayName: "Wonder Boy", Name: "Wonder Boy (USA)", Favorite: true})

	// Empty search returns all games
	games := lib.GetGamesSortedFiltered("title", false, "")
	if len(games) != 4 {
		t.Errorf("expected 4 games with empty search, got %d", len(games))
	}

	// Case-insensitive search by DisplayName
	games = lib.GetGamesSortedFiltered("title", false, "sonic")
	if len(games) != 1 {
		t.Errorf("expected 1 game matching 'sonic', got %d", len(games))
	}
	if games[0].CRC32 != "1" {
		t.Errorf("expected Sonic game, got %s", games[0].DisplayName)
	}

	// Case-insensitive search - uppercase
	games = lib.GetGamesSortedFiltered("title", false, "SONIC")
	if len(games) != 1 {
		t.Errorf("expected 1 game matching 'SONIC', got %d", len(games))
	}

	// Search matches Name field (not just DisplayName)
	games = lib.GetGamesSortedFiltered("title", false, "europe")
	if len(games) != 1 {
		t.Errorf("expected 1 game matching 'europe' in Name, got %d", len(games))
	}
	if games[0].CRC32 != "2" {
		t.Errorf("expected Alex Kidd (Europe), got %s", games[0].DisplayName)
	}

	// Partial match - "world" only matches "Miracle World"
	games = lib.GetGamesSortedFiltered("title", false, "world")
	if len(games) != 1 {
		t.Errorf("expected 1 game matching 'world', got %d", len(games))
	}
	if games[0].CRC32 != "2" {
		t.Errorf("expected Alex Kidd (Miracle World), got %s", games[0].DisplayName)
	}

	// Combined with favorites filter
	games = lib.GetGamesSortedFiltered("title", true, "")
	if len(games) != 1 {
		t.Errorf("expected 1 favorite game, got %d", len(games))
	}
	if games[0].CRC32 != "4" {
		t.Errorf("expected Wonder Boy (favorite), got %s", games[0].DisplayName)
	}

	// Combined filters: favorites + search
	games = lib.GetGamesSortedFiltered("title", true, "wonder")
	if len(games) != 1 {
		t.Errorf("expected 1 game matching 'wonder' and favorite, got %d", len(games))
	}

	// No matches
	games = lib.GetGamesSortedFiltered("title", false, "nonexistent")
	if len(games) != 0 {
		t.Errorf("expected 0 games matching 'nonexistent', got %d", len(games))
	}

	// No matches with favorites filter
	games = lib.GetGamesSortedFiltered("title", true, "sonic")
	if len(games) != 0 {
		t.Errorf("expected 0 games matching 'sonic' with favorites filter, got %d", len(games))
	}
}

func TestApplyMissingDefaultsAlreadyCurrent(t *testing.T) {
	config := &Config{
		Version: 1,
		Theme:   "Dark",
		Audio:   AudioConfig{Volume: 0.5},
		Window:  WindowConfig{Width: 1024, Height: 768},
		Library: LibraryView{ViewMode: "list", SortBy: "lastPlayed"},
	}

	// All keys present
	presentKeys := map[string]bool{
		"version": true, "theme": true, "fontSize": true,
		"audio.volume": true, "window.width": true, "window.height": true,
		"library.viewMode": true, "library.sortBy": true,
		"rewind.bufferSizeMB": true, "rewind.frameStep": true,
	}
	ApplyMissingDefaults(config, presentKeys)

	// Should preserve existing values, not overwrite with defaults
	if config.Audio.Volume != 0.5 {
		t.Errorf("volume should remain 0.5, got %f", config.Audio.Volume)
	}
	if config.Window.Width != 1024 {
		t.Errorf("width should remain 1024, got %d", config.Window.Width)
	}
	if config.Window.Height != 768 {
		t.Errorf("height should remain 768, got %d", config.Window.Height)
	}
	if config.Library.ViewMode != "list" {
		t.Errorf("view mode should remain 'list', got '%s'", config.Library.ViewMode)
	}
	if config.Library.SortBy != "lastPlayed" {
		t.Errorf("sort by should remain 'lastPlayed', got '%s'", config.Library.SortBy)
	}
	if config.Theme != "Dark" {
		t.Errorf("theme should remain 'Dark', got '%s'", config.Theme)
	}
}

func TestApplyMissingDefaultsPartialFields(t *testing.T) {
	// Some fields present, others absent
	config := &Config{
		Version: 0,
		Audio:   AudioConfig{Volume: 0.8},
		Window:  WindowConfig{},
		Library: LibraryView{ViewMode: "list"},
	}

	// Only some keys present in the JSON file
	presentKeys := map[string]bool{
		"version":          true,
		"audio.volume":     true,
		"library.viewMode": true,
	}
	ApplyMissingDefaults(config, presentKeys)

	// Present keys preserved (even version=0)
	if config.Version != 0 {
		t.Errorf("version should remain 0 (present), got %d", config.Version)
	}
	if config.Audio.Volume != 0.8 {
		t.Errorf("volume should remain 0.8, got %f", config.Audio.Volume)
	}
	if config.Library.ViewMode != "list" {
		t.Errorf("view mode should remain 'list', got '%s'", config.Library.ViewMode)
	}

	// Absent keys defaulted
	if config.Window.Width != 900 {
		t.Errorf("width should default to 900, got %d", config.Window.Width)
	}
	if config.Window.Height != 650 {
		t.Errorf("height should default to 650, got %d", config.Window.Height)
	}
	if config.Library.SortBy != "title" {
		t.Errorf("sort by should default to 'title', got '%s'", config.Library.SortBy)
	}
	if config.Theme != "Default" {
		t.Errorf("theme should default to 'Default', got '%s'", config.Theme)
	}
}

func TestAtomicWriteJSONInvalidDir(t *testing.T) {
	// Writing to a path under a file (not a directory) should fail
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "not_a_dir")
	os.WriteFile(filePath, []byte("file"), 0644)

	err := AtomicWriteJSON(filepath.Join(filePath, "sub", "test.json"), "data")
	if err == nil {
		t.Error("expected error when writing to invalid directory path")
	}
}

func TestReadJSONInvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "bad.json")

	// Write invalid JSON
	os.WriteFile(path, []byte("{invalid json}"), 0644)

	var result map[string]string
	err := ReadJSON(path, &result)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestReadJSONNonexistentFile(t *testing.T) {
	var result map[string]string
	err := ReadJSON("/nonexistent/path/file.json", &result)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLibraryNilGamesMap(t *testing.T) {
	lib := &Library{Games: nil}

	// These should not panic with nil Games map
	if lib.GameCount() != 0 {
		t.Errorf("expected 0, got %d", lib.GameCount())
	}
	if g := lib.GetGame("test"); g != nil {
		t.Errorf("expected nil, got %+v", g)
	}

	lib.RemoveGame("test") // Should not panic

	games := lib.GetGamesSorted("title", false)
	if games != nil {
		t.Errorf("expected nil, got %v", games)
	}

	filtered := lib.GetGamesSortedFiltered("title", false, "test")
	if filtered != nil {
		t.Errorf("expected nil, got %v", filtered)
	}
}

func TestLibraryAddGameInitializesMap(t *testing.T) {
	lib := &Library{Games: nil}

	lib.AddGame(&GameEntry{CRC32: "abc", DisplayName: "Test"})

	if lib.Games == nil {
		t.Fatal("Games map should be initialized")
	}
	if lib.GameCount() != 1 {
		t.Errorf("expected 1, got %d", lib.GameCount())
	}
}

func TestLibraryDefaultSortOrder(t *testing.T) {
	lib := DefaultLibrary()
	lib.AddGame(&GameEntry{CRC32: "1", DisplayName: "Zelda"})
	lib.AddGame(&GameEntry{CRC32: "2", DisplayName: "Alex Kidd"})

	// "unknown_sort" should fall back to title sort
	games := lib.GetGamesSorted("unknown_sort", false)
	if len(games) != 2 {
		t.Fatalf("expected 2 games, got %d", len(games))
	}
	if games[0].DisplayName != "Alex Kidd" {
		t.Errorf("expected Alex Kidd first with default sort, got %s", games[0].DisplayName)
	}
}

func TestMigrateLibrary(t *testing.T) {
	lib := &Library{
		Version: 0,
		Games:   make(map[string]*GameEntry),
	}

	migrated := migrateLibrary(lib)
	if migrated.Version != 1 {
		t.Errorf("expected version 1, got %d", migrated.Version)
	}

	// Already current version should stay the same
	lib2 := &Library{Version: 1}
	migrated2 := migrateLibrary(lib2)
	if migrated2.Version != 1 {
		t.Errorf("expected version 1, got %d", migrated2.Version)
	}
}

func TestUpdatePlayTimeNonexistentGame(t *testing.T) {
	lib := DefaultLibrary()

	// Should not panic for non-existent game
	lib.UpdatePlayTime("nonexistent", 100)
}

func TestAddScanDirectoryRecursiveFlag(t *testing.T) {
	lib := DefaultLibrary()

	lib.AddScanDirectory("/roms", true)
	if len(lib.ScanDirectories) != 1 {
		t.Fatalf("expected 1 directory, got %d", len(lib.ScanDirectories))
	}
	if !lib.ScanDirectories[0].Recursive {
		t.Error("expected recursive to be true")
	}

	// Adding same path again should be ignored regardless of recursive flag
	lib.AddScanDirectory("/roms", false)
	if len(lib.ScanDirectories) != 1 {
		t.Errorf("duplicate should be ignored, got %d", len(lib.ScanDirectories))
	}
}

func TestRemoveScanDirectoryNotFound(t *testing.T) {
	lib := DefaultLibrary()
	lib.AddScanDirectory("/roms", true)

	// Removing non-existent should not affect list
	lib.RemoveScanDirectory("/nonexistent")
	if len(lib.ScanDirectories) != 1 {
		t.Errorf("expected 1 directory, got %d", len(lib.ScanDirectories))
	}
}

func TestRemoveExcludedPathNotFound(t *testing.T) {
	lib := DefaultLibrary()
	lib.AddExcludedPath("/exclude")

	// Removing non-existent should not affect list
	lib.RemoveExcludedPath("/nonexistent")
	if len(lib.ExcludedPaths) != 1 {
		t.Errorf("expected 1 excluded path, got %d", len(lib.ExcludedPaths))
	}
}

func TestAddExcludedPathDuplicate(t *testing.T) {
	lib := DefaultLibrary()
	lib.AddExcludedPath("/exclude")
	lib.AddExcludedPath("/exclude")

	if len(lib.ExcludedPaths) != 1 {
		t.Errorf("duplicate should be ignored, got %d", len(lib.ExcludedPaths))
	}
}
