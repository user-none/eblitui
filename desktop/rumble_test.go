package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/storage"
)

func TestLaunchedDiscNumber(t *testing.T) {
	discs := []storage.GameDisc{{Index: 0}, {Index: 1}, {Index: 1}, {Index: 3}}
	tests := []struct {
		name     string
		disc     bool
		game     *storage.GameEntry
		selected int
		want     int
	}{
		{"cartridge system", false, &storage.GameEntry{Discs: discs}, 0, 0},
		{"no current game", true, nil, 0, 0},
		{"first disc", true, &storage.GameEntry{Discs: discs}, 0, 1},
		{"second disc", true, &storage.GameEntry{Discs: discs}, 1, 2},
		{"same-number variant", true, &storage.GameEntry{Discs: discs}, 2, 2},
		{"gap in owned discs", true, &storage.GameEntry{Discs: discs}, 3, 4},
		{"selection out of range", true, &storage.GameEntry{Discs: discs}, 4, 0},
		{"negative selection", true, &storage.GameEntry{Discs: discs}, -1, 0},
		{"no discs", true, &storage.GameEntry{}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm := &GameplayManager{
				systemInfo:  coreif.SystemInfo{Disc: tt.disc},
				currentGame: tt.game,
			}
			if gm.currentGame != nil {
				gm.currentGame.SelectedDisc = tt.selected
			}
			if got := gm.launchedDiscNumber(); got != tt.want {
				t.Errorf("launchedDiscNumber() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRumbleGameID(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("test redirects storage via XDG_DATA_HOME")
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir, err := storage.GetRumbleDir()
	if err != nil {
		t.Fatalf("GetRumbleDir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "T-1219H.disc2.erumble"), []byte("---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	game := func(selected int) *storage.GameEntry {
		return &storage.GameEntry{
			Discs:        []storage.GameDisc{{Index: 0}, {Index: 1}},
			SelectedDisc: selected,
		}
	}
	tests := []struct {
		name string
		disc bool
		game *storage.GameEntry
		want string
	}{
		{"cartridge ignores disc files", false, game(1), "T-1219H"},
		{"disc-specific file exists", true, game(1), "T-1219H.disc2"},
		{"no disc-specific file", true, game(0), "T-1219H"},
		{"no current game", true, nil, "T-1219H"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm := &GameplayManager{
				systemInfo:  coreif.SystemInfo{Disc: tt.disc},
				currentGame: tt.game,
			}
			if got := gm.rumbleGameID("T-1219H"); got != tt.want {
				t.Errorf("rumbleGameID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScaleMotor(t *testing.T) {
	tests := []struct {
		name  string
		v     float64
		scale float64
		want  float64
	}{
		{"zero value", 0, 1.0, 0},
		{"negative value", -0.5, 1.0, 0},
		{"zero scale", 0.6, 0, 0},
		{"negative scale", 0.6, -1.0, 0},
		{"identity", 0.6, 1.0, 0.6},
		{"fractional scale", 0.6, 1.25, 0.75},
		{"reduced scale", 0.6, 0.5, 0.3},
		{"clamped to max", 0.6, 3.0, 1.0},
		{"exactly at max", 0.5, 2.0, 1.0},
		{"small step scale", 1.0, 0.05, 0.05},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scaleMotor(tt.v, tt.scale)
			if diff := got - tt.want; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("scaleMotor(%v, %v) = %v, want %v", tt.v, tt.scale, got, tt.want)
			}
		})
	}
}
