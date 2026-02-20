//go:build !libretro

package standalone

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/user-none/eblitui/standalone/storage"
)

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

func TestCleanDisplayName(t *testing.T) {
	s := &Scanner{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"sms extension", "Sonic.sms", "Sonic"},
		{"zip extension", "Game.zip", "Game"},
		{"no extension", "GameName", "GameName"},
		{"multiple dots", "My.Game.Name.sms", "My.Game.Name"},
		{"with spaces", "Alex Kidd in Miracle World.sms", "Alex Kidd in Miracle World"},
		{"empty string", "", ""},
		{"region stripped", "Sonic the Hedgehog (USA).sms", "Sonic the Hedgehog"},
		{"multi region stripped", "Sonic the Hedgehog (USA, Europe).sms", "Sonic the Hedgehog"},
		{"multiple parens stripped", "Zillion (Japan) (Rev 2).sms", "Zillion"},
		{"no extension with parens", "Game (USA)", "Game"},
		{"no space before paren", "Game(USA).sms", "Game(USA)"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := s.cleanDisplayName(tc.input)
			if got != tc.expected {
				t.Errorf("cleanDisplayName(%q) = %q, want %q", tc.input, got, tc.expected)
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
		"aabbccdd": {CRC32: "aabbccdd", DisplayName: "Existing Game"},
	}

	s := NewScanner(dirs, excluded, existing, false, []string{".sms"}, "", "")

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
	if s.rescanAll {
		t.Error("rescanAll should be false")
	}
	if s.existingGames == nil {
		t.Fatal("existingGames should not be nil")
	}
	if s.existingGames["aabbccdd"].DisplayName != "Existing Game" {
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
	s := NewScanner(nil, nil, nil, false, nil, "", "")

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
	s := NewScanner(nil, nil, nil, false, nil, "", "")

	if s.gamesCount() != 0 {
		t.Errorf("expected 0 games, got %d", s.gamesCount())
	}

	s.games["abc"] = &storage.GameEntry{CRC32: "abc"}
	if s.gamesCount() != 1 {
		t.Errorf("expected 1 game, got %d", s.gamesCount())
	}
}
