package storage

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/user-none/eblitui/desktop/region"
)

// testInfo returns a GameInfo that reads each game's title and region from
// a map keyed by game ID. Games not in the map have an empty title and
// region.
func testInfo(m map[string][2]string) GameInfo {
	return func(g *GameEntry) (string, string) {
		v := m[g.GameID]
		return v[0], v[1]
	}
}

// TestConfigFileOwnerOnly tests that config.json, which holds the
// RetroAchievements token, is readable only by its owner, including when a
// temp file left by an earlier failed write had wider permissions.
func TestConfigFileOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits do not apply on Windows")
	}
	useTempDataDir(t)
	if err := EnsureDirectories(); err != nil {
		t.Fatal(err)
	}
	path, err := GetConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".tmp", []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := ResetConfig()
	if err != nil {
		t.Fatalf("ResetConfig: %v", err)
	}
	config.Flush()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("config.json mode = %o, want 600", perm)
	}
}

// TestAtomicWriteFileReplacesStaleTemp tests that a temp file left by an
// earlier failed write is replaced and not left behind.
func TestAtomicWriteFileReplacesStaleTemp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(path+".tmp", []byte("stale contents that are longer"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWriteFile(path, []byte("new")); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new" {
		t.Fatalf("file = %q, %v; want %q", got, err, "new")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file left behind")
	}
}

// TestAtomicWriteFileConcurrent tests that writers to the same file at the
// same time each write it whole: the result is exactly one writer's data,
// never a mix, and every write succeeds.
func TestAtomicWriteFileConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.bin")
	const writers = 16
	const size = 4 * 1024 * 1024

	// Release every writer at once so their writes overlap.
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := range writers {
		data := bytes.Repeat([]byte{byte(i + 1)}, size)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = AtomicWriteFile(path, data)
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("writer %d: %v", i, err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != size {
		t.Fatalf("file is %d bytes, want %d", len(got), size)
	}
	for _, b := range got {
		if b != got[0] {
			t.Fatal("file mixes data from more than one writer")
		}
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file left behind")
	}
}

func TestGameEntryDiscsRoundTrip(t *testing.T) {
	in := &GameEntry{
		GameID: "MK-81307",
		Discs: []GameDisc{
			{Index: 0, File: "/roms/pds1.chd", Name: "Panzer Dragoon Saga (USA) (Disc 1)"},
			{Index: 1, File: "/roms/pds2.chd", Name: "Panzer Dragoon Saga (USA) (Disc 2)"},
		},
		SelectedDisc: 1,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out GameEntry
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(out.Discs) != 2 || out.Discs[1].File != "/roms/pds2.chd" || out.Discs[1].Index != 1 {
		t.Errorf("Discs round-trip failed: %+v", out.Discs)
	}
	if out.SelectedDisc != 1 {
		t.Errorf("SelectedDisc = %d, want 1", out.SelectedDisc)
	}
}

func TestGameEntryBackCompatNoDiscs(t *testing.T) {
	// Old library.json entries have neither discs nor selectedDisc.
	old := `{"crc32":"abc","file":"/roms/sonic.chd","name":"Sonic","displayName":"Sonic"}`
	var e GameEntry
	if err := json.Unmarshal([]byte(old), &e); err != nil {
		t.Fatalf("Unmarshal old format: %v", err)
	}
	if e.Discs != nil {
		t.Errorf("Discs = %v, want nil for old format", e.Discs)
	}
	if e.SelectedDisc != 0 {
		t.Errorf("SelectedDisc = %d, want 0 for old format", e.SelectedDisc)
	}
	// Single-disc/cartridge entries must omit the new fields entirely.
	b, err := json.Marshal(&GameEntry{GameID: "abc", File: "/roms/sonic.chd"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(b)
	for _, sub := range []string{"\"discs\"", "\"selectedDisc\""} {
		if containsSub(s, sub) {
			t.Errorf("marshaled single-disc entry contains %s: %s", sub, s)
		}
	}
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig().Values()

	if config.Version != configVersion {
		t.Errorf("expected version %d, got %d", configVersion, config.Version)
	}
	if !config.Updates.AutoCheck {
		t.Error("expected automatic update checks on")
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
	if config.Library.ArtworkDownload != ArtworkDownloadAll {
		t.Errorf("expected artwork download %q, got %q", ArtworkDownloadAll, config.Library.ArtworkDownload)
	}
	if config.Library.BoxArtRegion != region.US {
		t.Errorf("expected box art region %q, got %q", region.US, config.Library.BoxArtRegion)
	}
}

// TestConfigRemoveBIOSFile tests that removing a variant's file keeps the
// other variants and clears the active selection only when it was that
// variant.
func TestConfigRemoveBIOSFile(t *testing.T) {
	config := DefaultConfig()
	config.SetBIOSFile("bios", "US", "/us.bin")
	config.SetBIOSFile("bios", "JP", "/jp.bin")
	config.SetBIOSActive("bios", "JP")

	config.RemoveBIOSFile("bios", "US")
	bc := config.Values().BIOS["bios"]
	if _, ok := bc.Files["US"]; ok || bc.Files["JP"] != "/jp.bin" || bc.Active != "JP" {
		t.Errorf("after removing US: %+v", bc)
	}

	config.RemoveBIOSFile("bios", "JP")
	bc = config.Values().BIOS["bios"]
	if len(bc.Files) != 0 || bc.Active != "" {
		t.Errorf("after removing the active JP: %+v", bc)
	}
}

// TestConfigToggleShaderDuplicates tests that turning off a shader listed
// more than once removes every occurrence.
func TestConfigToggleShaderDuplicates(t *testing.T) {
	data := defaultConfigData()
	data.Shaders.UIShaders = []string{"crt", "scan", "crt"}
	config := &Config{data: data}

	if config.ToggleUIShader("crt") {
		t.Error("toggle reported the shader on")
	}
	if got := config.Values().Shaders.UIShaders; len(got) != 1 || got[0] != "scan" {
		t.Errorf("UI shaders = %v, want [scan]", got)
	}
}

// TestConfigChangesPersist tests that a change made through a method is
// written to config.json once flushed.
func TestConfigChangesPersist(t *testing.T) {
	useTempDataDir(t)
	if err := EnsureDirectories(); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	config.SetTheme("Dark")
	config.Flush()

	reloaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := reloaded.Values().Theme; got != "Dark" {
		t.Errorf("reloaded theme = %q, want Dark", got)
	}
}

func TestDefaultLibrary(t *testing.T) {
	lib := DefaultLibrary()

	if lib.data.Version != libraryVersion {
		t.Errorf("expected version %d, got %d", libraryVersion, lib.data.Version)
	}
	if len(lib.data.Games) != 0 {
		t.Errorf("expected empty games map, got %d entries", len(lib.data.Games))
	}
	if len(lib.data.ScanDirectories) != 0 {
		t.Errorf("expected empty scan directories, got %d entries", len(lib.data.ScanDirectories))
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

func TestLibraryAddAndGame(t *testing.T) {
	lib := DefaultLibrary()

	game := GameEntry{
		GameID: "12345678",
		File:   "/path/to/game.sms",
	}

	// Add game
	lib.AddGame(game)

	if lib.GameCount() != 1 {
		t.Errorf("expected 1 game, got %d", lib.GameCount())
	}

	// Get game
	retrieved, ok := lib.Game("12345678")
	if !ok {
		t.Fatal("game not found")
	}
	if retrieved.File != "/path/to/game.sms" {
		t.Errorf("expected '/path/to/game.sms', got '%s'", retrieved.File)
	}
}

func TestLibrarySorting(t *testing.T) {
	lib := DefaultLibrary()

	lib.AddGame(GameEntry{GameID: "1", PlayTimeSeconds: 100, LastPlayed: 1000})
	lib.AddGame(GameEntry{GameID: "2", PlayTimeSeconds: 500, LastPlayed: 500})
	lib.AddGame(GameEntry{GameID: "3", PlayTimeSeconds: 300, LastPlayed: 2000})
	info := testInfo(map[string][2]string{"1": {"Zelda"}, "2": {"Alex Kidd"}, "3": {"Sonic"}})

	// Sort by title
	games := lib.GamesSorted("title", false, info)
	if len(games) != 3 {
		t.Fatalf("expected 3 games, got %d", len(games))
	}
	if games[0].GameID != "2" {
		t.Errorf("expected first game Alex Kidd (2), got %s", games[0].GameID)
	}
	if games[2].GameID != "1" {
		t.Errorf("expected last game Zelda (1), got %s", games[2].GameID)
	}

	// Sort by play time
	games = lib.GamesSorted("playTime", false, info)
	if games[0].GameID != "2" { // Most played (500s)
		t.Errorf("expected most played Alex Kidd (2), got %s", games[0].GameID)
	}

	// Sort by last played
	games = lib.GamesSorted("lastPlayed", false, info)
	if games[0].GameID != "3" { // Most recent (2000)
		t.Errorf("expected most recent Sonic (3), got %s", games[0].GameID)
	}
}

func TestLibrarySortingStability(t *testing.T) {
	// Games with the same title sort by region, then by game ID, so the
	// order is the same on every sort.
	lib := DefaultLibrary()
	for _, id := range []string{"C", "A", "B", "D", "E"} {
		lib.AddGame(GameEntry{GameID: id})
	}
	info := testInfo(map[string][2]string{
		"C": {"Zillion", "jp"},
		"A": {"Zillion", "us"},
		"B": {"zillion", "eu"},
		"D": {"Zillion", "jp"},
		"E": {"Alex Kidd", "us"},
	})

	// Title is compared case-insensitively, then region (eu, jp, us), then
	// game ID breaks the jp tie.
	want := []string{"E", "B", "C", "D", "A"}
	for i := 0; i < 5; i++ {
		games := lib.GamesSorted("title", false, info)
		if len(games) != len(want) {
			t.Fatalf("expected %d games, got %d", len(want), len(games))
		}
		for j, id := range want {
			if games[j].GameID != id {
				t.Errorf("iteration %d: position %d is %s, want %s", i, j, games[j].GameID, id)
			}
		}
	}

	// Equal lastPlayed and playTime fall back to title order
	lib2 := DefaultLibrary()
	lib2.AddGame(GameEntry{GameID: "C", LastPlayed: 1000, PlayTimeSeconds: 100})
	lib2.AddGame(GameEntry{GameID: "A", LastPlayed: 1000, PlayTimeSeconds: 100})
	lib2.AddGame(GameEntry{GameID: "B", LastPlayed: 1000, PlayTimeSeconds: 100})
	info2 := testInfo(map[string][2]string{"C": {"Game C"}, "A": {"Game A"}, "B": {"Game B"}})
	for _, sortBy := range []string{"lastPlayed", "playTime"} {
		for i := 0; i < 5; i++ {
			games := lib2.GamesSorted(sortBy, false, info2)
			for j, id := range []string{"A", "B", "C"} {
				if games[j].GameID != id {
					t.Errorf("%s iteration %d: position %d is %s, want %s", sortBy, i, j, games[j].GameID, id)
				}
			}
		}
	}
}

func TestLibraryFavoritesFilter(t *testing.T) {
	lib := DefaultLibrary()

	lib.AddGame(GameEntry{GameID: "1", Favorite: true})
	lib.AddGame(GameEntry{GameID: "2", Favorite: false})
	lib.AddGame(GameEntry{GameID: "3", Favorite: true})
	info := testInfo(nil)

	// All games
	all := lib.GamesSorted("title", false, info)
	if len(all) != 3 {
		t.Errorf("expected 3 games, got %d", len(all))
	}

	// Favorites only
	favorites := lib.GamesSorted("title", true, info)
	if len(favorites) != 2 {
		t.Errorf("expected 2 favorites, got %d", len(favorites))
	}
}

func TestLibraryScanDirectories(t *testing.T) {
	lib := DefaultLibrary()

	lib.AddScanDirectory("/path/to/roms", true)
	lib.AddScanDirectory("/path/to/more", false)

	if len(lib.ScanDirectories()) != 2 {
		t.Errorf("expected 2 directories, got %d", len(lib.ScanDirectories()))
	}

	// Add duplicate (should be ignored)
	lib.AddScanDirectory("/path/to/roms", false)
	if len(lib.ScanDirectories()) != 2 {
		t.Errorf("duplicate should be ignored, got %d directories", len(lib.ScanDirectories()))
	}

	// Remove directory
	lib.RemoveScanDirectory("/path/to/roms")
	if len(lib.ScanDirectories()) != 1 {
		t.Errorf("expected 1 directory after removal, got %d", len(lib.ScanDirectories()))
	}
}

func TestLibraryExcludedPaths(t *testing.T) {
	lib := DefaultLibrary()

	lib.AddExcludedPath("/path/to/exclude")
	lib.AddExcludedPath("/path/to/file.sms")

	if len(lib.ExcludedPaths()) != 2 {
		t.Errorf("expected 2 excluded paths, got %d", len(lib.ExcludedPaths()))
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
	if len(lib.ExcludedPaths()) != 1 {
		t.Errorf("expected 1 excluded path after removal, got %d", len(lib.ExcludedPaths()))
	}
}

func TestApplyMissingDefaultsMigration(t *testing.T) {
	// Test migration from version 0 when keys are absent
	config := &ConfigData{
		Version: 0,
		Audio:   AudioConfig{Volume: 0}, // Volume=0 is valid (0% volume)
		Window:  WindowConfig{},
		Library: LibraryView{},
	}

	// Simulate: only audio.volume was present in JSON (as 0)
	presentKeys := map[string]bool{
		"audio.volume": true,
	}
	applyMissingDefaults(config, presentKeys)

	// version is never defaulted; migrateConfig handles version 0
	if config.Version != 0 {
		t.Errorf("expected version 0, got %d", config.Version)
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
	config := &ConfigData{
		Version: 1,
		Audio:   AudioConfig{Volume: 0.0},
		Window:  WindowConfig{Width: 900, Height: 650},
		Library: LibraryView{ViewMode: "icon", SortBy: "title"},
		Theme:   "Default",
	}

	// All keys present in JSON
	presentKeys := map[string]bool{
		"theme":        true,
		"audio.volume": true, "window.width": true, "window.height": true,
		"library.viewMode": true, "library.sortBy": true,
		"rewind.bufferSizeMB": true, "rewind.frameStep": true,
	}
	applyMissingDefaults(config, presentKeys)

	if config.Audio.Volume != 0.0 {
		t.Errorf("expected volume 0.0 to be preserved, got %f", config.Audio.Volume)
	}
}

func TestAddPlayTimeSetLastPlayed(t *testing.T) {
	lib := DefaultLibrary()

	lib.AddGame(GameEntry{
		GameID:          "12345678",
		PlayTimeSeconds: 100,
	})

	lib.AddPlayTime("12345678", 50)
	lib.SetLastPlayed("12345678", 1700000000)

	game, _ := lib.Game("12345678")
	if game.PlayTimeSeconds != 150 {
		t.Errorf("expected 150 seconds, got %d", game.PlayTimeSeconds)
	}
	if game.LastPlayed != 1700000000 {
		t.Errorf("expected LastPlayed 1700000000, got %d", game.LastPlayed)
	}
}

func TestGamesSortedFiltered(t *testing.T) {
	lib := DefaultLibrary()

	lib.AddGame(GameEntry{GameID: "1"})
	lib.AddGame(GameEntry{GameID: "2"})
	lib.AddGame(GameEntry{GameID: "3"})
	lib.AddGame(GameEntry{GameID: "4", Favorite: true})
	info := testInfo(map[string][2]string{
		"1": {"Sonic the Hedgehog", "us"},
		"2": {"Alex Kidd in Miracle World", "eu"},
		"3": {"Phantasy Star", "jp"},
		"4": {"Wonder Boy", "us"},
	})

	// Empty search returns all games
	games := lib.GamesSortedFiltered("title", false, "", info)
	if len(games) != 4 {
		t.Errorf("expected 4 games with empty search, got %d", len(games))
	}

	// Case-insensitive search by title
	games = lib.GamesSortedFiltered("title", false, "sonic", info)
	if len(games) != 1 {
		t.Errorf("expected 1 game matching 'sonic', got %d", len(games))
	}
	if games[0].GameID != "1" {
		t.Errorf("expected Sonic game, got %s", games[0].GameID)
	}

	// Case-insensitive search - uppercase
	games = lib.GamesSortedFiltered("title", false, "SONIC", info)
	if len(games) != 1 {
		t.Errorf("expected 1 game matching 'SONIC', got %d", len(games))
	}

	// Search matches the title only, not the region
	games = lib.GamesSortedFiltered("title", false, "eu", info)
	if len(games) != 0 {
		t.Errorf("expected 0 games matching 'eu', got %d", len(games))
	}

	// Partial match - "world" only matches "Miracle World"
	games = lib.GamesSortedFiltered("title", false, "world", info)
	if len(games) != 1 {
		t.Errorf("expected 1 game matching 'world', got %d", len(games))
	}
	if games[0].GameID != "2" {
		t.Errorf("expected Alex Kidd (Miracle World), got %s", games[0].GameID)
	}

	// Combined with favorites filter
	games = lib.GamesSortedFiltered("title", true, "", info)
	if len(games) != 1 {
		t.Errorf("expected 1 favorite game, got %d", len(games))
	}
	if games[0].GameID != "4" {
		t.Errorf("expected Wonder Boy (favorite), got %s", games[0].GameID)
	}

	// Combined filters: favorites + search
	games = lib.GamesSortedFiltered("title", true, "wonder", info)
	if len(games) != 1 {
		t.Errorf("expected 1 game matching 'wonder' and favorite, got %d", len(games))
	}

	// No matches
	games = lib.GamesSortedFiltered("title", false, "nonexistent", info)
	if len(games) != 0 {
		t.Errorf("expected 0 games matching 'nonexistent', got %d", len(games))
	}

	// No matches with favorites filter
	games = lib.GamesSortedFiltered("title", true, "sonic", info)
	if len(games) != 0 {
		t.Errorf("expected 0 games matching 'sonic' with favorites filter, got %d", len(games))
	}
}

func TestApplyMissingDefaultsAlreadyCurrent(t *testing.T) {
	config := &ConfigData{
		Version: 1,
		Theme:   "Dark",
		Audio:   AudioConfig{Volume: 0.5},
		Window:  WindowConfig{Width: 1024, Height: 768},
		Library: LibraryView{ViewMode: "list", SortBy: "lastPlayed"},
	}

	// All keys present
	presentKeys := map[string]bool{
		"theme":        true,
		"audio.volume": true, "window.width": true, "window.height": true,
		"library.viewMode": true, "library.sortBy": true,
		"rewind.bufferSizeMB": true, "rewind.frameStep": true,
	}
	applyMissingDefaults(config, presentKeys)

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
	config := &ConfigData{
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
	applyMissingDefaults(config, presentKeys)

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
	lib := &Library{data: libraryData{Games: nil}}

	// These should not panic with nil Games map
	if lib.GameCount() != 0 {
		t.Errorf("expected 0, got %d", lib.GameCount())
	}
	if g, ok := lib.Game("test"); ok {
		t.Errorf("expected no game, got %+v", g)
	}

	// No matching game, so nothing is read or written
	if err := lib.RemoveGames("test"); err != nil {
		t.Errorf("RemoveGames: %v", err)
	}

	games := lib.GamesSorted("title", false, testInfo(nil))
	if len(games) != 0 {
		t.Errorf("expected no games, got %v", games)
	}

	filtered := lib.GamesSortedFiltered("title", false, "test", testInfo(nil))
	if len(filtered) != 0 {
		t.Errorf("expected no games, got %v", filtered)
	}
}

func TestLibraryDefaultSortOrder(t *testing.T) {
	lib := DefaultLibrary()
	lib.AddGame(GameEntry{GameID: "1"})
	lib.AddGame(GameEntry{GameID: "2"})
	info := testInfo(map[string][2]string{"1": {"Zelda"}, "2": {"Alex Kidd"}})

	// "unknown_sort" should fall back to title sort
	games := lib.GamesSorted("unknown_sort", false, info)
	if len(games) != 2 {
		t.Fatalf("expected 2 games, got %d", len(games))
	}
	if games[0].GameID != "2" {
		t.Errorf("expected Alex Kidd (2) first with default sort, got %s", games[0].GameID)
	}
}

func TestMigrateLibraryVersion2(t *testing.T) {
	// Version 2 entries keep their user data, take GameID from the map key,
	// and lose the dropped metadata fields when the library is saved.
	useTempDataDir(t)
	path, err := GetLibraryPath()
	if err != nil {
		t.Fatalf("GetLibraryPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	old := `{"version": 2, "games": {"T-14310G": {"crc32": "T-14310G", "name": "Bulk Slash", "displayName": "Bulk Slash", "region": "jp", "developer": "HUDSON", "favorite": true, "playTimeSeconds": 60, "discs": [{"index": 0, "file": "/roms/bs.chd", "name": "(Japan)"}]}}}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	lib, err := LoadLibrary()
	if err != nil {
		t.Fatalf("LoadLibrary: %v", err)
	}
	if lib.data.Version != libraryVersion {
		t.Errorf("version = %d, want %d", lib.data.Version, libraryVersion)
	}
	g, ok := lib.Game("T-14310G")
	if !ok || g.GameID != "T-14310G" || !g.Favorite || g.PlayTimeSeconds != 60 || len(g.Discs) != 1 {
		t.Fatalf("migrated entry = %+v", g)
	}

	lib.Flush()
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, field := range []string{`"crc32"`, `"displayName"`, `"region"`, `"developer"`} {
		if containsSub(string(saved), field) {
			t.Errorf("saved library still has %s: %s", field, saved)
		}
	}
	if !containsSub(string(saved), `"gameId": "T-14310G"`) {
		t.Errorf("saved library lacks gameId: %s", saved)
	}
}

func TestMigrateLibraryCurrentVersion(t *testing.T) {
	// A current library is left alone and no library file is read. Older
	// versions read the library file for save slots, so they are not
	// exercised here.
	data := &libraryData{Version: libraryVersion}
	if migrateLibrary(data, "") {
		t.Error("current library reported as migrated")
	}
	if data.Version != libraryVersion {
		t.Errorf("expected version %d, got %d", libraryVersion, data.Version)
	}
}

func TestMigrateConfig(t *testing.T) {
	tests := []struct {
		name          string
		version       int
		autoCheck     bool
		wantVersion   int
		wantAutoCheck bool
		wantMigrated  bool
	}{
		{"absent version is version 1", 0, false, configVersion, true, true},
		{"version 1 turns on automatic checks", 1, false, configVersion, true, true},
		{"current version keeps automatic checks off", configVersion, false, configVersion, false, false},
		{"current version keeps automatic checks on", configVersion, true, configVersion, true, false},
		{"newer version left for validation", configVersion + 1, false, configVersion + 1, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := &ConfigData{Version: tc.version, Updates: UpdatesConfig{AutoCheck: tc.autoCheck}}
			if got := migrateConfig(config); got != tc.wantMigrated {
				t.Errorf("migrated = %v, want %v", got, tc.wantMigrated)
			}
			if config.Version != tc.wantVersion {
				t.Errorf("version = %d, want %d", config.Version, tc.wantVersion)
			}
			if config.Updates.AutoCheck != tc.wantAutoCheck {
				t.Errorf("autoCheck = %v, want %v", config.Updates.AutoCheck, tc.wantAutoCheck)
			}
		})
	}
}

// writeConfigFile writes raw config.json contents to the data directory
func writeConfigFile(t *testing.T, data string) string {
	t.Helper()
	path, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestLoadConfigMigrates(t *testing.T) {
	for _, data := range []string{
		`{"version": 1, "theme": "Dark"}`,
		`{"theme": "Dark"}`,
	} {
		t.Run(data, func(t *testing.T) {
			useTempDataDir(t)
			path := writeConfigFile(t, data)

			loaded, err := LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			config := loaded.Values()
			if config.Version != configVersion || !config.Updates.AutoCheck || config.Theme != "Dark" {
				t.Errorf("loaded version %d, autoCheck %v, theme %q", config.Version, config.Updates.AutoCheck, config.Theme)
			}

			// The migration is saved so it runs once
			loaded.Flush()
			var saved ConfigData
			if err := ReadJSON(path, &saved); err != nil {
				t.Fatalf("ReadJSON: %v", err)
			}
			if saved.Version != configVersion || !saved.Updates.AutoCheck || saved.Theme != "Dark" {
				t.Errorf("saved version %d, autoCheck %v, theme %q", saved.Version, saved.Updates.AutoCheck, saved.Theme)
			}
		})
	}
}

func TestLoadConfigCurrentVersionNotRewritten(t *testing.T) {
	useTempDataDir(t)
	data := `{"version": 2, "updates": {"autoCheck": false}}`
	path := writeConfigFile(t, data)

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if config.Values().Updates.AutoCheck {
		t.Error("automatic checks turned off by the user were turned on")
	}
	config.Flush()

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(after) != data {
		t.Errorf("current config was rewritten: %s", after)
	}
}

func TestLegacySaveSlots(t *testing.T) {
	data := []byte(`{
		"version": 1,
		"games": {
			"T-14310G": {"crc32": "T-14310G", "settings": {"saveSlot": 2}},
			"MK-81019": {"crc32": "MK-81019", "settings": {}},
			"ZERO": {"settings": {"saveSlot": 0}},
			"HIGH": {"settings": {"saveSlot": 10}},
			"NEG": {"settings": {"saveSlot": -1}},
			"NONE": {"crc32": "NONE"},
			"NINE": {"settings": {"saveSlot": 9}}
		}
	}`)

	slots, err := legacySaveSlots(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]int{"T-14310G": 2, "NINE": 9}
	if len(slots) != len(want) {
		t.Errorf("got %v, want %v", slots, want)
	}
	for key, slot := range want {
		if slots[key] != slot {
			t.Errorf("%s: got %d, want %d", key, slots[key], slot)
		}
	}

	if _, err := legacySaveSlots([]byte("not json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// TestMigrateSaveSlotsSkipsInvalidKeys tests that a version 1 library's save
// slots move to the settings files of valid game keys only: an empty or
// unclean key writes no settings file.
func TestMigrateSaveSlotsSkipsInvalidKeys(t *testing.T) {
	useTempDataDir(t)
	path, err := GetLibraryPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	old := `{"version": 1, "games": {
		"T-1": {"settings": {"saveSlot": 2}},
		"": {"settings": {"saveSlot": 3}},
		"..": {"settings": {"saveSlot": 4}}
	}}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadLibrary(); err != nil {
		t.Fatalf("LoadLibrary: %v", err)
	}

	settings, err := LoadGameSettings("T-1")
	if err != nil || settings.SaveStates.Slot != 2 {
		t.Fatalf("T-1 settings = %+v, %v; want slot 2", settings, err)
	}
	dir, err := GetGameSettingsDir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "T-1.json" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("settings files = %v, want only T-1.json", names)
	}
}

// TestGamePathsRejectInvalidKeys tests that every per-game path refuses an
// empty or unclean game ID, so none can resolve to a shared directory or
// leave it, and accepts a valid one.
func TestGamePathsRejectInvalidKeys(t *testing.T) {
	useTempDataDir(t)
	funcs := map[string]func(string) (string, error){
		"GetGameSettingsPath":  GetGameSettingsPath,
		"GetGameSaveDir":       GetGameSaveDir,
		"GetGameScreenshotDir": GetGameScreenshotDir,
		"GetGameArtworkDir":    GetGameArtworkDir,
	}
	for name, fn := range funcs {
		for _, key := range []string{"", ".", "..", "a/b", `a\b`, "a\nb"} {
			if p, err := fn(key); err == nil {
				t.Errorf("%s(%q) = %q, want error", name, key, p)
			}
		}
		if _, err := fn("T-1"); err != nil {
			t.Errorf("%s(T-1): %v", name, err)
		}
	}
}

func TestAddPlayTimeNonexistentGame(t *testing.T) {
	lib := DefaultLibrary()

	// Should not panic for non-existent game
	lib.AddPlayTime("nonexistent", 100)
	lib.SetLastPlayed("nonexistent", 100)
	if lib.GameCount() != 0 {
		t.Errorf("expected 0 games, got %d", lib.GameCount())
	}
}

func TestAddScanDirectoryRecursiveFlag(t *testing.T) {
	lib := DefaultLibrary()

	lib.AddScanDirectory("/roms", true)
	if len(lib.ScanDirectories()) != 1 {
		t.Fatalf("expected 1 directory, got %d", len(lib.ScanDirectories()))
	}
	if !lib.ScanDirectories()[0].Recursive {
		t.Error("expected recursive to be true")
	}

	// Adding same path again should be ignored regardless of recursive flag
	lib.AddScanDirectory("/roms", false)
	if len(lib.ScanDirectories()) != 1 {
		t.Errorf("duplicate should be ignored, got %d", len(lib.ScanDirectories()))
	}
}

func TestRemoveScanDirectoryNotFound(t *testing.T) {
	lib := DefaultLibrary()
	lib.AddScanDirectory("/roms", true)

	// Removing non-existent should not affect list
	lib.RemoveScanDirectory("/nonexistent")
	if len(lib.ScanDirectories()) != 1 {
		t.Errorf("expected 1 directory, got %d", len(lib.ScanDirectories()))
	}
}

func TestRemoveExcludedPathNotFound(t *testing.T) {
	lib := DefaultLibrary()
	lib.AddExcludedPath("/exclude")

	// Removing non-existent should not affect list
	lib.RemoveExcludedPath("/nonexistent")
	if len(lib.ExcludedPaths()) != 1 {
		t.Errorf("expected 1 excluded path, got %d", len(lib.ExcludedPaths()))
	}
}

func TestAddExcludedPathDuplicate(t *testing.T) {
	lib := DefaultLibrary()
	lib.AddExcludedPath("/exclude")
	lib.AddExcludedPath("/exclude")

	if len(lib.ExcludedPaths()) != 1 {
		t.Errorf("duplicate should be ignored, got %d", len(lib.ExcludedPaths()))
	}
}
