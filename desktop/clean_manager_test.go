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
		"present": {CRC32: "present", File: present},
		"lost":    {CRC32: "lost", File: gone},
		"partial": {CRC32: "partial", Discs: []storage.GameDisc{{File: present}, {File: gone}}},
	}

	r := newCleanRun()
	r.check(games)
	files := <-r.done

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
	r.check(map[string]storage.GameEntry{"lost": {CRC32: "lost", File: filepath.Join(t.TempDir(), "gone")}})

	if files := <-r.done; files != nil {
		t.Errorf("cancelled run sent %v, want nil", files)
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
		cm.Cancel()
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
	lib.AddGame(&storage.GameEntry{CRC32: "present", File: touchROM(t, roms, "a.sms")})
	lib.AddGame(&storage.GameEntry{CRC32: "lost", File: filepath.Join(roms, "gone.sms"), PlayTimeSeconds: 40})
	cm := NewCleanManager(lib, screens.NewScanProgressScreen(), nil, nil)

	if msg := runClean(t, cm, false); msg != "Cleaned 1 game" {
		t.Errorf("message = %q, want Cleaned 1 game", msg)
	}
	if lib.GetGame("lost") != nil || lib.GetGame("present") == nil {
		t.Errorf("library games = %v, want only present", lib.Games)
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
	lib.AddGame(&storage.GameEntry{CRC32: "lost", File: filepath.Join(t.TempDir(), "gone.sms")})
	cm := NewCleanManager(lib, screens.NewScanProgressScreen(), nil, nil)

	if msg := runClean(t, cm, true); msg != "" {
		t.Errorf("message = %q, want none", msg)
	}
	if lib.GetGame("lost") == nil {
		t.Error("cancelled clean removed a game")
	}
}
