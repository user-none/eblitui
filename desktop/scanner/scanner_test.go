package scanner

import (
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/user-none/eblitui/desktop/storage"
)

func TestUpsertDisc(t *testing.T) {
	var discs []storage.GameDisc
	discs = upsertDisc(discs, storage.GameDisc{Index: 2, File: "/x/c.chd", Name: "C"})
	discs = upsertDisc(discs, storage.GameDisc{Index: 0, File: "/x/a.chd", Name: "A"})
	discs = upsertDisc(discs, storage.GameDisc{Index: 1, File: "/x/b.chd", Name: "B"})
	if len(discs) != 3 || discs[0].Index != 0 || discs[1].Index != 1 || discs[2].Index != 2 {
		t.Fatalf("not sorted by index: %+v", discs)
	}

	// Same basename rescanned from a different directory: update in
	// place, no growth, stored path refreshed.
	discs = upsertDisc(discs, storage.GameDisc{Index: 1, File: "/y/b.chd", Name: "B"})
	if len(discs) != 3 || discs[1].File != "/y/b.chd" {
		t.Fatalf("same-basename rescan should update in place: %+v", discs)
	}

	// A variant (same product number and disc number, different
	// filename) is appended, not replaced, and sorts directly after the
	// disc it shares an Index with.
	discs = upsertDisc(discs, storage.GameDisc{Index: 0, File: "/x/a (En. Trans).chd", Name: "(En. Trans)"})
	if len(discs) != 4 {
		t.Fatalf("variant should be appended: %+v", discs)
	}
	if discs[0].File != "/x/a.chd" || discs[1].File != "/x/a (En. Trans).chd" {
		t.Fatalf("variant should sort adjacent to its same-index sibling: %+v", discs)
	}
	if discs[2].Index != 1 || discs[3].Index != 2 {
		t.Fatalf("remaining discs should stay ordered: %+v", discs)
	}
}

func TestDiscLabelFromFilename(t *testing.T) {
	cases := map[string]string{
		"Panzer Dragoon Saga (USA) (Disc 1).chd": "(USA) (Disc 1)",
		"Bulk Slash (Japan).chd":                 "(Japan)",
		"Bulk Slash (En. Trans).chd":             "(En. Trans)",
		"Sonic Jam.chd":                          "Sonic Jam",
		"NoExtNoMeta":                            "NoExtNoMeta",
	}
	for in, want := range cases {
		if got := discLabelFromFilename(in); got != want {
			t.Errorf("discLabelFromFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsSupportedExtension(t *testing.T) {
	s := &Scanner{extensions: []string{".sms"}}

	supported := []struct {
		ext string
	}{
		{".sms"},
		{".zip"},
		{".7z"},
		{".gz"},
		{".tar.gz"},
		{".rar"},
	}

	for _, tc := range supported {
		t.Run("supported_"+tc.ext, func(t *testing.T) {
			if !s.isSupportedExtension(tc.ext) {
				t.Errorf("isSupportedExtension(%q) = false, want true", tc.ext)
			}
		})
	}

	unsupported := []struct {
		ext string
	}{
		{".txt"},
		{".rom"},
		{".bin"},
		{".gg"},
		{".md"},
		{".iso"},
		{""},
		{".SMS"},  // case sensitive
		{".Zip"},  // case sensitive
		{".sms "}, // trailing space
	}

	for _, tc := range unsupported {
		t.Run("unsupported_"+tc.ext, func(t *testing.T) {
			if s.isSupportedExtension(tc.ext) {
				t.Errorf("isSupportedExtension(%q) = true, want false", tc.ext)
			}
		})
	}
}

func TestScannerIsPathExcluded(t *testing.T) {
	s := &Scanner{
		excludedPaths: map[string]bool{
			"/path/to/exclude":  true,
			"/path/to/file.sms": true,
		},
	}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"exact directory match", "/path/to/exclude", true},
		{"exact file match", "/path/to/file.sms", true},
		{"subdirectory of excluded", "/path/to/exclude" + string(os.PathSeparator) + "subdir", true},
		{"deeper subdirectory", "/path/to/exclude" + string(os.PathSeparator) + "sub" + string(os.PathSeparator) + "deep", true},
		{"not excluded", "/path/to/other", false},
		{"partial prefix no separator", "/path/to/excludemore", false},
		{"parent of excluded", "/path/to", false},
		{"empty path", "", false},
		{"root", "/", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := s.isPathExcluded(tc.path)
			if got != tc.expected {
				t.Errorf("isPathExcluded(%q) = %v, want %v", tc.path, got, tc.expected)
			}
		})
	}
}

func TestScannerIsPathExcludedEmpty(t *testing.T) {
	s := &Scanner{
		excludedPaths: map[string]bool{},
	}

	if s.isPathExcluded("/any/path") {
		t.Error("nothing should be excluded with empty exclusion list")
	}
}

func TestNewScanner(t *testing.T) {
	dirs := []storage.ScanDirectory{
		{Path: "/roms", Recursive: true},
		{Path: "/more", Recursive: false},
	}
	excluded := []string{"/skip/this", "/skip/that"}
	existing := map[string]*storage.GameEntry{
		"aabbccdd": {GameID: "aabbccdd", Favorite: true},
	}

	s := NewScanner(dirs, excluded, existing, []string{".sms"}, 0, false, nil, Sources{})

	if len(s.directories) != 2 {
		t.Errorf("expected 2 directories, got %d", len(s.directories))
	}
	if len(s.excludedPaths) != 2 {
		t.Errorf("expected 2 excluded paths, got %d", len(s.excludedPaths))
	}
	if !s.excludedPaths["/skip/this"] {
		t.Error("excluded path /skip/this not set")
	}
	if !s.excludedPaths["/skip/that"] {
		t.Error("excluded path /skip/that not set")
	}
	if s.existingGames == nil {
		t.Fatal("existingGames should not be nil")
	}
	if !s.existingGames["aabbccdd"].Favorite {
		t.Error("existing game not preserved")
	}
}

func TestScanDirectoryNonRecursive(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test files
	os.WriteFile(filepath.Join(tmpDir, "game1.sms"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "game2.zip"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(subDir, "game3.sms"), []byte("test"), 0644)

	s := &Scanner{
		excludedPaths: map[string]bool{},
		extensions:    []string{".sms"},
	}

	// Non-recursive should not include subdir files
	files, err := s.scanDirectory(storage.ScanDirectory{Path: tmpDir, Recursive: false})
	if err != nil {
		t.Fatal(err)
	}

	// Should find game1.sms and game2.zip but not readme.txt or subdir/game3.sms
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(files), files)
	}
}

func TestScanDirectoryRecursive(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	os.WriteFile(filepath.Join(tmpDir, "game1.sms"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(subDir, "game2.sms"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(subDir, "readme.txt"), []byte("test"), 0644)

	s := &Scanner{
		excludedPaths: map[string]bool{},
		extensions:    []string{".sms"},
	}

	// Recursive should include subdir files
	files, err := s.scanDirectory(storage.ScanDirectory{Path: tmpDir, Recursive: true})
	if err != nil {
		t.Fatal(err)
	}

	// Should find game1.sms and subdir/game2.sms but not readme.txt
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(files), files)
	}
}

func TestScanDirectoryWithExclusions(t *testing.T) {
	tmpDir := t.TempDir()
	excludedDir := filepath.Join(tmpDir, "excluded")
	includedDir := filepath.Join(tmpDir, "included")
	if err := os.MkdirAll(excludedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(includedDir, 0755); err != nil {
		t.Fatal(err)
	}

	os.WriteFile(filepath.Join(excludedDir, "game1.sms"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(includedDir, "game2.sms"), []byte("test"), 0644)

	s := &Scanner{
		excludedPaths: map[string]bool{
			excludedDir: true,
		},
		extensions: []string{".sms"},
	}

	files, err := s.scanDirectory(storage.ScanDirectory{Path: tmpDir, Recursive: true})
	if err != nil {
		t.Fatal(err)
	}

	// Should find only game2.sms (excluded dir skipped)
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d: %v", len(files), files)
	}
}

func TestScannerCancellation(t *testing.T) {
	s := NewScanner(nil, nil, nil, nil, 0, false, nil, Sources{})

	if s.isCancelled() {
		t.Error("new scanner should not be cancelled")
	}

	s.Cancel()
	if !s.isCancelled() {
		t.Error("scanner should be cancelled after Cancel()")
	}

	// Calling Cancel twice should not panic
	s.Cancel()
}

func TestScannerGamesCount(t *testing.T) {
	s := NewScanner(nil, nil, nil, nil, 0, false, nil, Sources{})

	if s.gamesCount() != 0 {
		t.Errorf("expected 0 games, got %d", s.gamesCount())
	}

	s.games["abc"] = &storage.GameEntry{GameID: "abc"}
	if s.gamesCount() != 1 {
		t.Errorf("expected 1 game, got %d", s.gamesCount())
	}
}

func TestNewScannerDefaultConsoleID(t *testing.T) {
	s := NewScanner(nil, nil, nil, nil, 42, false, nil, Sources{})
	if s.defaultConsoleID != 42 {
		t.Errorf("expected defaultConsoleID 42, got %d", s.defaultConsoleID)
	}
}

func TestProcessCartridgeUsesExistingEntry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	rom := []byte("cartridge data")
	path := filepath.Join(dir, "Game (USA).sms")
	if err := os.WriteFile(path, rom, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	key := fmt.Sprintf("%08x", crc32.ChecksumIEEE(rom))

	// A removed entry has no added time, so it is added again when found
	start := time.Now().Unix()
	tests := []struct {
		name      string
		existing  *storage.GameEntry
		keepAdded bool
	}{
		{"moved library entry", &storage.GameEntry{GameID: key, File: "/old/Game (USA).sms", Favorite: true, PlayTimeSeconds: 90, LastPlayed: 7, Added: 3}, true},
		{"removed entry", &storage.GameEntry{GameID: key, Favorite: true, PlayTimeSeconds: 90, LastPlayed: 7}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existing := map[string]*storage.GameEntry{key: tt.existing}
			s := NewScanner(nil, nil, existing, []string{".sms"}, 0, false, nil, Sources{})
			s.processCartridge(path)

			got := s.Games()[key]
			if got == nil {
				t.Fatal("game not found by scan")
			}
			if got.File != path {
				t.Errorf("File = %q, want %q", got.File, path)
			}
			if !got.Favorite || got.PlayTimeSeconds != 90 || got.LastPlayed != 7 || got.GameID != key {
				t.Errorf("user data not kept: %+v", got)
			}
			if tt.keepAdded && got.Added != 3 {
				t.Errorf("Added = %d, want 3", got.Added)
			}
			if !tt.keepAdded && got.Added < start {
				t.Errorf("Added = %d, want the scan time (>= %d)", got.Added, start)
			}
		})
	}
}
