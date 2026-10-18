package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// useTempDataDir points the data directory at a temporary directory
func useTempDataDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("APPDATA", dir)
}

// loadSavedLibrary reads library.json back from the data directory
func loadSavedLibrary(t *testing.T) *Library {
	t.Helper()
	path, err := GetLibraryPath()
	if err != nil {
		t.Fatalf("GetLibraryPath: %v", err)
	}
	lib := &Library{}
	if err := ReadJSON(path, lib); err != nil {
		t.Fatalf("ReadJSON library: %v", err)
	}
	return lib
}

func TestLoadRemovedGamesNoFile(t *testing.T) {
	useTempDataDir(t)

	removed, err := LoadRemovedGames()
	if err != nil {
		t.Fatalf("LoadRemovedGames: %v", err)
	}
	if removed.Version != removedVersion {
		t.Errorf("Version = %d, want %d", removed.Version, removedVersion)
	}
	if removed.Games == nil || len(removed.Games) != 0 {
		t.Errorf("Games = %v, want empty map", removed.Games)
	}
}

// writeCorruptRemoved writes an unparsable removed.json
func writeCorruptRemoved(t *testing.T) {
	t.Helper()
	path, err := GetRemovedGamesPath()
	if err != nil {
		t.Fatalf("GetRemovedGamesPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestLoadRemovedGamesCorrupt(t *testing.T) {
	useTempDataDir(t)
	writeCorruptRemoved(t)

	removed, err := LoadRemovedGames()
	if err != nil {
		t.Fatalf("LoadRemovedGames: %v", err)
	}
	if removed.Version != removedVersion || removed.Games == nil || len(removed.Games) != 0 {
		t.Errorf("corrupt file loaded as %+v, want empty", removed)
	}
}

func TestLoadRemovedGamesSanitizes(t *testing.T) {
	useTempDataDir(t)
	if err := SaveRemovedGames(&RemovedGames{Games: map[string]*GameEntry{
		"a/b": {CRC32: "a/b", PlayTimeSeconds: 10},
		"c":   {CRC32: "c", File: "/roms/c.sms", Discs: []GameDisc{{File: "/roms/c.chd"}}, SelectedDisc: 2, PlayTimeSeconds: -5, Added: 100},
	}}); err != nil {
		t.Fatalf("SaveRemovedGames: %v", err)
	}

	removed, err := LoadRemovedGames()
	if err != nil {
		t.Fatalf("LoadRemovedGames: %v", err)
	}
	if g := removed.Games["ab"]; g == nil || g.CRC32 != "ab" || g.PlayTimeSeconds != 10 {
		t.Errorf("re-keyed entry = %+v, want key ab with play time 10", g)
	}
	g := removed.Games["c"]
	if g == nil {
		t.Fatal("entry c missing")
	}
	if g.File != "" || g.Discs != nil || g.SelectedDisc != 0 || g.Added != 0 {
		t.Errorf("files and added time not stripped: %+v", g)
	}
	if g.PlayTimeSeconds != 0 {
		t.Errorf("PlayTimeSeconds = %d, want 0", g.PlayTimeSeconds)
	}
}

func TestRemoveGamesMovesUserData(t *testing.T) {
	useTempDataDir(t)
	lib := DefaultLibrary()
	lib.AddGame(&GameEntry{CRC32: "aaaa", File: "/roms/a.sms", DisplayName: "A", Favorite: true, PlayTimeSeconds: 120, LastPlayed: 50, Added: 10})
	lib.AddGame(&GameEntry{
		CRC32:        "T-1",
		DisplayName:  "Disc Game",
		Discs:        []GameDisc{{Index: 0, File: "/roms/d1.chd"}, {Index: 1, File: "/roms/d2.chd"}},
		SelectedDisc: 1,
	})
	lib.AddGame(&GameEntry{CRC32: "keep", File: "/roms/k.sms"})

	if err := lib.RemoveGames("aaaa", "T-1", "unknown"); err != nil {
		t.Fatalf("RemoveGames: %v", err)
	}

	if lib.GameCount() != 1 || lib.GetGame("keep") == nil {
		t.Errorf("library games = %v, want only keep", lib.Games)
	}
	saved := loadSavedLibrary(t)
	if len(saved.Games) != 1 || saved.Games["keep"] == nil {
		t.Errorf("saved library games = %v, want only keep", saved.Games)
	}

	removed, err := LoadRemovedGames()
	if err != nil {
		t.Fatalf("LoadRemovedGames: %v", err)
	}
	if len(removed.Games) != 2 {
		t.Fatalf("removed games = %v, want 2", removed.Games)
	}
	a := removed.Games["aaaa"]
	if a == nil || !a.Favorite || a.PlayTimeSeconds != 120 || a.LastPlayed != 50 || a.DisplayName != "A" {
		t.Errorf("removed aaaa = %+v, want user data kept", a)
	}
	if a != nil && (a.File != "" || a.Added != 0) {
		t.Errorf("removed aaaa File %q Added %d, want both cleared", a.File, a.Added)
	}
	d := removed.Games["T-1"]
	if d == nil || d.Discs != nil || d.SelectedDisc != 0 {
		t.Errorf("removed T-1 = %+v, want discs stripped", d)
	}
}

func TestRemoveGamesKeepsEarlierRemovals(t *testing.T) {
	useTempDataDir(t)
	lib := DefaultLibrary()
	lib.AddGame(&GameEntry{CRC32: "a", File: "/roms/a.sms"})
	lib.AddGame(&GameEntry{CRC32: "b", File: "/roms/b.sms"})

	if err := lib.RemoveGames("a"); err != nil {
		t.Fatalf("RemoveGames a: %v", err)
	}
	if err := lib.RemoveGames("b"); err != nil {
		t.Fatalf("RemoveGames b: %v", err)
	}

	removed, err := LoadRemovedGames()
	if err != nil {
		t.Fatalf("LoadRemovedGames: %v", err)
	}
	if removed.Games["a"] == nil || removed.Games["b"] == nil {
		t.Errorf("removed games = %v, want a and b", removed.Games)
	}
}

func TestRemoveGamesOverwritesCorruptRemovedFile(t *testing.T) {
	useTempDataDir(t)
	writeCorruptRemoved(t)
	lib := DefaultLibrary()
	lib.AddGame(&GameEntry{CRC32: "a", File: "/roms/a.sms"})

	if err := lib.RemoveGames("a"); err != nil {
		t.Fatalf("RemoveGames: %v", err)
	}
	if lib.GetGame("a") != nil {
		t.Error("game still in library")
	}
	path, err := GetRemovedGamesPath()
	if err != nil {
		t.Fatalf("GetRemovedGamesPath: %v", err)
	}
	removed := &RemovedGames{}
	if err := ReadJSON(path, removed); err != nil {
		t.Fatalf("removed.json not rewritten: %v", err)
	}
	if len(removed.Games) != 1 || removed.Games["a"] == nil {
		t.Errorf("removed games = %v, want only a", removed.Games)
	}
}

func TestDropRestoredGames(t *testing.T) {
	useTempDataDir(t)
	if err := SaveRemovedGames(&RemovedGames{Games: map[string]*GameEntry{
		"back":  {CRC32: "back"},
		"still": {CRC32: "still"},
	}}); err != nil {
		t.Fatalf("SaveRemovedGames: %v", err)
	}
	lib := DefaultLibrary()
	lib.AddGame(&GameEntry{CRC32: "back", File: "/roms/back.sms"})

	if err := DropRestoredGames(lib); err != nil {
		t.Fatalf("DropRestoredGames: %v", err)
	}

	removed, err := LoadRemovedGames()
	if err != nil {
		t.Fatalf("LoadRemovedGames: %v", err)
	}
	if len(removed.Games) != 1 || removed.Games["still"] == nil {
		t.Errorf("removed games = %v, want only still", removed.Games)
	}
}
