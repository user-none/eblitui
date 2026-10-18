package storage

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// touch creates an empty file in dir and returns its path
func touch(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestCheckFilesCartridge(t *testing.T) {
	dir := t.TempDir()
	present := touch(t, dir, "a.sms")

	tests := []struct {
		name string
		file string
		want bool
	}{
		{"present", present, false},
		{"missing", filepath.Join(dir, "gone.sms"), true},
		{"empty path", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := (&GameEntry{File: tt.file}).CheckFiles()
			if status.Missing != tt.want {
				t.Errorf("Missing = %v, want %v", status.Missing, tt.want)
			}
			if status.DiscMissing != nil {
				t.Errorf("DiscMissing = %v, want nil", status.DiscMissing)
			}
		})
	}
}

func TestCheckFilesDiscs(t *testing.T) {
	dir := t.TempDir()
	present := touch(t, dir, "d1.chd")
	gone := filepath.Join(dir, "d2.chd")

	t.Run("some missing", func(t *testing.T) {
		g := &GameEntry{Discs: []GameDisc{{File: present}, {File: gone}}}
		status := g.CheckFiles()
		if status.Missing {
			t.Error("Missing = true, want false")
		}
		if !slices.Equal(status.DiscMissing, []bool{false, true}) {
			t.Errorf("DiscMissing = %v, want [false true]", status.DiscMissing)
		}
	})

	t.Run("all missing", func(t *testing.T) {
		g := &GameEntry{Discs: []GameDisc{{File: gone}, {File: ""}}}
		status := g.CheckFiles()
		if !status.Missing {
			t.Error("Missing = false, want true")
		}
		if !slices.Equal(status.DiscMissing, []bool{true, true}) {
			t.Errorf("DiscMissing = %v, want [true true]", status.DiscMissing)
		}
	})
}

func TestFileStatusMissingAt(t *testing.T) {
	discs := FileStatus{DiscMissing: []bool{false, true}}
	if discs.MissingAt(0) {
		t.Error("MissingAt(0) = true, want false")
	}
	if !discs.MissingAt(1) {
		t.Error("MissingAt(1) = false, want true")
	}

	cart := FileStatus{Missing: true}
	if !cart.MissingAt(0) {
		t.Error("cartridge MissingAt(0) = false, want true")
	}
}

func TestSelectedDiscIndex(t *testing.T) {
	tests := []struct {
		name     string
		discs    int
		selected int
		want     int
	}{
		{"cartridge", 0, 0, 0},
		{"in range", 3, 2, 2},
		{"negative", 3, -1, 0},
		{"past end", 3, 3, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GameEntry{Discs: make([]GameDisc, tt.discs), SelectedDisc: tt.selected}
			if got := g.SelectedDiscIndex(); got != tt.want {
				t.Errorf("SelectedDiscIndex() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRemoveDisc(t *testing.T) {
	discs := func() []GameDisc {
		return []GameDisc{{Index: 0, File: "a"}, {Index: 0, File: "a2"}, {Index: 1, File: "b"}}
	}
	tests := []struct {
		name      string
		selected  int
		remove    int
		wantFiles []string
		wantSel   int
	}{
		{"earlier row keeps selected disc", 2, 0, []string{"a2", "b"}, 1},
		{"later row keeps selection", 0, 2, []string{"a", "a2"}, 0},
		{"selected row moves to first", 1, 1, []string{"a", "b"}, 0},
		{"out of range ignored", 1, 3, []string{"a", "a2", "b"}, 1},
		{"negative ignored", 1, -1, []string{"a", "a2", "b"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GameEntry{Discs: discs(), SelectedDisc: tt.selected}
			g.RemoveDisc(tt.remove)
			var files []string
			for _, d := range g.Discs {
				files = append(files, d.File)
			}
			if !slices.Equal(files, tt.wantFiles) {
				t.Errorf("files = %v, want %v", files, tt.wantFiles)
			}
			if g.SelectedDisc != tt.wantSel {
				t.Errorf("SelectedDisc = %d, want %d", g.SelectedDisc, tt.wantSel)
			}
		})
	}
}

// missingFiles checks every game in lib and returns the statuses of the
// games with a missing file
func missingFiles(lib *Library) map[string]FileStatus {
	files := make(map[string]FileStatus)
	for key, game := range lib.Games {
		if status := game.CheckFiles(); status.HasMissing() {
			files[key] = status
		}
	}
	return files
}

func TestFileStatusHasMissing(t *testing.T) {
	tests := []struct {
		name   string
		status FileStatus
		want   bool
	}{
		{"cartridge present", FileStatus{}, false},
		{"cartridge missing", FileStatus{Missing: true}, true},
		{"discs present", FileStatus{DiscMissing: []bool{false, false}}, false},
		{"one disc missing", FileStatus{DiscMissing: []bool{false, true}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.HasMissing(); got != tt.want {
				t.Errorf("HasMissing() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemoveMissing(t *testing.T) {
	useTempDataDir(t)
	roms := t.TempDir()
	cart := touch(t, roms, "cart.sms")
	d1 := touch(t, roms, "d1.chd")
	gone := filepath.Join(roms, "gone")

	lib := DefaultLibrary()
	lib.AddGame(&GameEntry{CRC32: "cart", File: cart})
	lib.AddGame(&GameEntry{CRC32: "lostcart", File: gone, PlayTimeSeconds: 30})
	lib.AddGame(&GameEntry{
		CRC32:        "partial",
		Discs:        []GameDisc{{Index: 0, File: gone}, {Index: 1, File: d1}},
		SelectedDisc: 1,
	})
	lib.AddGame(&GameEntry{CRC32: "lostdisc", Discs: []GameDisc{{Index: 0, File: gone}}})

	n, err := lib.RemoveMissing(missingFiles(lib))
	if err != nil {
		t.Fatalf("RemoveMissing: %v", err)
	}
	if n != 3 {
		t.Errorf("RemoveMissing() = %d, want 3", n)
	}

	saved := loadSavedLibrary(t)
	for _, l := range []*Library{lib, saved} {
		if len(l.Games) != 2 || l.Games["cart"] == nil || l.Games["partial"] == nil {
			t.Fatalf("games = %v, want cart and partial", l.Games)
		}
		p := l.Games["partial"]
		if len(p.Discs) != 1 || p.Discs[0].File != d1 || p.SelectedDisc != 0 {
			t.Errorf("partial = %+v, want only d1 selected", p)
		}
	}

	removed, err := LoadRemovedGames()
	if err != nil {
		t.Fatalf("LoadRemovedGames: %v", err)
	}
	if len(removed.Games) != 2 || removed.Games["lostdisc"] == nil {
		t.Errorf("removed = %v, want lostcart and lostdisc", removed.Games)
	}
	if g := removed.Games["lostcart"]; g == nil || g.PlayTimeSeconds != 30 {
		t.Errorf("removed lostcart = %+v, want play time kept", g)
	}
}

func TestRemoveMissingOnlyPrunedDiscs(t *testing.T) {
	useTempDataDir(t)
	roms := t.TempDir()
	d1 := touch(t, roms, "d1.chd")

	lib := DefaultLibrary()
	lib.AddGame(&GameEntry{CRC32: "partial", Discs: []GameDisc{{File: d1}, {File: filepath.Join(roms, "gone")}}})

	n, err := lib.RemoveMissing(missingFiles(lib))
	if err != nil {
		t.Fatalf("RemoveMissing: %v", err)
	}
	if n != 1 {
		t.Errorf("RemoveMissing() = %d, want 1", n)
	}
	if saved := loadSavedLibrary(t); len(saved.Games["partial"].Discs) != 1 {
		t.Errorf("saved discs = %v, want 1", saved.Games["partial"].Discs)
	}
	path, _ := GetRemovedGamesPath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("removed.json written when no game was removed")
	}
}

func TestRemoveMissingNothingMissing(t *testing.T) {
	useTempDataDir(t)
	lib := DefaultLibrary()
	lib.AddGame(&GameEntry{CRC32: "cart", File: touch(t, t.TempDir(), "cart.sms")})

	n, err := lib.RemoveMissing(missingFiles(lib))
	if err != nil {
		t.Fatalf("RemoveMissing: %v", err)
	}
	if n != 0 {
		t.Errorf("RemoveMissing() = %d, want 0", n)
	}
	path, _ := GetLibraryPath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("library.json written when nothing changed")
	}
}

func TestRemoveMissingSkipsStaleStatus(t *testing.T) {
	useTempDataDir(t)
	lib := DefaultLibrary()
	lib.AddGame(&GameEntry{CRC32: "discs", Discs: []GameDisc{{File: "a"}, {File: "b"}, {File: "c"}}})

	// The statuses no longer match the library: an unknown game, and a
	// disc count that changed since the check
	n, err := lib.RemoveMissing(map[string]FileStatus{
		"unknown": {Missing: true},
		"discs":   {DiscMissing: []bool{true, false}},
	})
	if err != nil {
		t.Fatalf("RemoveMissing: %v", err)
	}
	if n != 0 {
		t.Errorf("RemoveMissing() = %d, want 0", n)
	}
	if got := len(lib.GetGame("discs").Discs); got != 3 {
		t.Errorf("discs = %d, want 3 untouched", got)
	}
}
