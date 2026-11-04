package desktop

import (
	"testing"

	"github.com/user-none/eblitui/desktop/storage"
)

func TestScanManagerIsScanning(t *testing.T) {
	sm := &ScanManager{}

	if sm.IsScanning() {
		t.Error("should not be scanning initially")
	}
}

func TestGameCount(t *testing.T) {
	for n, want := range map[int]string{0: "0 games", 1: "1 game", 2: "2 games"} {
		if got := gameCount(n); got != want {
			t.Errorf("gameCount(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestMergeScanned(t *testing.T) {
	library := storage.DefaultLibrary()
	library.AddGame(storage.GameEntry{GameID: "old", File: "/old.sms"})
	library.AddGame(storage.GameEntry{GameID: "kept", File: "/kept.sms"})

	found := map[string]*storage.GameEntry{
		"old": {GameID: "old", File: "/moved.sms"},
		"new": {GameID: "new", File: "/new.sms"},
	}

	if got := library.MergeScanned(found); got != 1 {
		t.Errorf("new games = %d, want 1", got)
	}
	if library.GameCount() != 3 {
		t.Errorf("library games = %d, want 3", library.GameCount())
	}
	if g, _ := library.Game("old"); g.File != "/moved.sms" {
		t.Errorf("old entry = %+v, want replaced by the scanned entry", g)
	}
	if !library.HasGame("new") || !library.HasGame("kept") {
		t.Error("new or kept entry missing")
	}
}

func TestScanManagerUpdateNilScanner(t *testing.T) {
	sm := &ScanManager{}
	// Should not panic when scanner is nil
	sm.Update()
}
