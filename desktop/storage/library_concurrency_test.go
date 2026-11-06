package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestLibraryConcurrentUse tests that changes, reads and background saves
// from many goroutines at once are safe and none is lost. Run with -race.
func TestLibraryConcurrentUse(t *testing.T) {
	useTempDataDir(t)
	lib, err := LoadLibrary()
	if err != nil {
		t.Fatal(err)
	}

	const writers = 8
	const games = 20
	const plays = 25

	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for g := range games {
				id := fmt.Sprintf("w%d-g%d", w, g)
				lib.AddGame(GameEntry{GameID: id, Discs: []GameDisc{{File: id + ".chd"}}})
				for range plays {
					lib.AddPlayTime(id, 1)
				}
				lib.ToggleFavorite(id)
				lib.SetSelectedDisc(id, 0)
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			title := func(g *GameEntry) GameKeys { return GameKeys{Title: g.GameID} }
			for range games {
				lib.GamesSortedFiltered("playTime", false, "g", title)
				lib.Games()
				lib.GameCount()
				lib.IsPathExcluded("/x")
			}
		}()
	}
	wg.Wait()
	lib.Flush()

	reloaded, err := LoadLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if n := reloaded.GameCount(); n != writers*games {
		t.Fatalf("reloaded %d games, want %d", n, writers*games)
	}
	for id, game := range reloaded.Games() {
		if game.PlayTimeSeconds != plays {
			t.Fatalf("%s play time %d, want %d", id, game.PlayTimeSeconds, plays)
		}
		if !game.Favorite {
			t.Fatalf("%s not marked favorite", id)
		}
	}
}

// TestLibraryReturnsCopies tests that changing a returned entry does not
// change the library.
func TestLibraryReturnsCopies(t *testing.T) {
	lib := DefaultLibrary()
	lib.AddGame(GameEntry{GameID: "A", Discs: []GameDisc{{File: "a.chd", Name: "Disc 1"}}})

	game, _ := lib.Game("A")
	game.Favorite = true
	game.Discs[0].Name = "changed"

	for _, g := range []GameEntry{lib.Games()["A"], lib.GamesSorted("", false, func(g *GameEntry) GameKeys { return GameKeys{} })[0]} {
		g.Discs[0].Name = "changed"
	}

	stored, _ := lib.Game("A")
	if stored.Favorite || stored.Discs[0].Name != "Disc 1" {
		t.Fatalf("library entry changed through a copy: %+v", stored)
	}
}

// TestDefaultLibraryNeverWrites tests that the in-memory library used by
// the error screen never replaces library.json.
func TestDefaultLibraryNeverWrites(t *testing.T) {
	useTempDataDir(t)
	lib := DefaultLibrary()
	lib.AddGame(GameEntry{GameID: "A"})
	lib.Flush()

	path, err := GetLibraryPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("DefaultLibrary wrote library.json")
	}
}

// TestSaverWritesNewest tests that a burst of changes ends with the newest
// contents on disk once flushed.
func TestSaverWritesNewest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	var mu sync.Mutex
	value := 0
	s := newSaver(path, 0644, func() ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		return []byte(fmt.Sprint(value)), nil
	})

	for i := 1; i <= 100; i++ {
		mu.Lock()
		value = i
		mu.Unlock()
		s.request()
	}
	s.flush()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "100" {
		t.Fatalf("file = %q, want %q", got, "100")
	}
}
