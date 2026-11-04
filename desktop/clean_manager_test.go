package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/user-none/eblitui/desktop/screens"
	"github.com/user-none/eblitui/desktop/storage"
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

// touchROM creates an empty file in dir and returns its path
func touchROM(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestCleanMessage(t *testing.T) {
	tests := []struct {
		n    int
		err  error
		want string
	}{
		{0, nil, "No missing games"},
		{1, nil, "Cleaned 1 game"},
		{2, nil, "Cleaned 2 games"},
		{0, errors.New("disk full"), "Failed to clean library"},
	}
	for _, tt := range tests {
		if got := cleanMessage(tt.n, tt.err); got != tt.want {
			t.Errorf("cleanMessage(%d, %v) = %q, want %q", tt.n, tt.err, got, tt.want)
		}
	}
}

func TestCleanRunCheck(t *testing.T) {
	roms := t.TempDir()
	present := touchROM(t, roms, "a.sms")
	gone := filepath.Join(roms, "gone.sms")
	games := map[string]storage.GameEntry{
		"present": {GameID: "present", File: present},
		"lost":    {GameID: "lost", File: gone},
		"partial": {GameID: "partial", Discs: []storage.GameDisc{{File: present}, {File: gone}}},
	}

	r := newCleanRun()
	r.check(games)
	res := <-r.done
	if res == nil {
		t.Fatal("check sent no result")
	}
	files := res.files

	if len(files) != 2 {
		t.Fatalf("files = %v, want lost and partial", files)
	}
	if !files["lost"].Missing {
		t.Error("lost not missing")
	}
	if p := files["partial"]; p.Missing || !p.MissingAt(1) || p.MissingAt(0) {
		t.Errorf("partial = %+v, want only disc 2 missing", p)
	}
}

func TestCleanRunCancelled(t *testing.T) {
	r := newCleanRun()
	r.Cancel()
	r.check(map[string]storage.GameEntry{"lost": {GameID: "lost", File: filepath.Join(t.TempDir(), "gone")}})

	if res := <-r.done; res != nil {
		t.Errorf("cancelled run sent %+v, want nil", res)
	}
}

// runClean starts a clean and calls Update until it completes, returning
// the completion message
func runClean(t *testing.T, cm *CleanManager, cancel bool) string {
	t.Helper()
	var msg string
	completed := false
	cm.onComplete = func(m string) {
		msg = m
		completed = true
	}
	cm.Start()
	if cancel {
		cm.screen.Cancel()
	}
	deadline := time.Now().Add(5 * time.Second)
	for !completed {
		if time.Now().After(deadline) {
			t.Fatal("clean did not complete")
		}
		cm.Update()
	}
	return msg
}

func TestCleanManagerRemovesMissing(t *testing.T) {
	useTempDataDir(t)
	roms := t.TempDir()
	lib := storage.DefaultLibrary()
	lib.AddGame(storage.GameEntry{GameID: "present", File: touchROM(t, roms, "a.sms")})
	lib.AddGame(storage.GameEntry{GameID: "lost", File: filepath.Join(roms, "gone.sms"), PlayTimeSeconds: 40})
	cm := NewCleanManager(lib, screens.NewScanProgressScreen(), nil, nil)

	if msg := runClean(t, cm, false); msg != "Cleaned 1 game" {
		t.Errorf("message = %q, want Cleaned 1 game", msg)
	}
	if lib.HasGame("lost") || !lib.HasGame("present") {
		t.Errorf("library games = %v, want only present", lib.Games())
	}
	removed, err := storage.LoadRemovedGames()
	if err != nil {
		t.Fatalf("LoadRemovedGames: %v", err)
	}
	if g := removed.Games["lost"]; g == nil || g.PlayTimeSeconds != 40 {
		t.Errorf("removed lost = %+v, want play time kept", g)
	}
}

func TestCleanManagerCancelChangesNothing(t *testing.T) {
	useTempDataDir(t)
	lib := storage.DefaultLibrary()
	lib.AddGame(storage.GameEntry{GameID: "lost", File: filepath.Join(t.TempDir(), "gone.sms")})
	cm := NewCleanManager(lib, screens.NewScanProgressScreen(), nil, nil)

	if msg := runClean(t, cm, true); msg != "" {
		t.Errorf("message = %q, want none", msg)
	}
	if !lib.HasGame("lost") {
		t.Error("cancelled clean removed a game")
	}
}

// artworkLibrary returns a library with one present game, T-1, whose
// artwork directory holds the given files
func artworkLibrary(t *testing.T, names ...string) (*storage.Library, string) {
	t.Helper()
	lib := storage.DefaultLibrary()
	lib.AddGame(storage.GameEntry{GameID: "T-1", File: touchROM(t, t.TempDir(), "a.cue")})
	dir, err := storage.GetGameArtworkDir("T-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return lib, dir
}

func TestCleanManagerRemovesSupersededBoxArt(t *testing.T) {
	useTempDataDir(t)
	lib, dir := artworkLibrary(t, "boxart.webp", "boxart.png", "boxback.webp")
	runClean(t, NewCleanManager(lib, screens.NewScanProgressScreen(), nil, nil), false)

	if _, err := os.Stat(filepath.Join(dir, "boxart.png")); !os.IsNotExist(err) {
		t.Error("boxart.png kept although boxart.webp is present")
	}
	for _, name := range []string{"boxart.webp", "boxback.webp"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s removed: %v", name, err)
		}
	}
}

func TestCleanManagerKeepsOnlyBoxArt(t *testing.T) {
	useTempDataDir(t)
	lib, dir := artworkLibrary(t, "boxart.png")
	runClean(t, NewCleanManager(lib, screens.NewScanProgressScreen(), nil, nil), false)

	if _, err := os.Stat(filepath.Join(dir, "boxart.png")); err != nil {
		t.Errorf("boxart.png removed without a boxart.webp: %v", err)
	}
}

func TestCleanManagerCancelKeepsBoxArt(t *testing.T) {
	useTempDataDir(t)
	lib, dir := artworkLibrary(t, "boxart.webp", "boxart.png")
	runClean(t, NewCleanManager(lib, screens.NewScanProgressScreen(), nil, nil), true)

	if _, err := os.Stat(filepath.Join(dir, "boxart.png")); err != nil {
		t.Errorf("cancelled clean removed boxart.png: %v", err)
	}
}
