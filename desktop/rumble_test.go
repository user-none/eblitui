package desktop

import (
	"testing"

	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/content"
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

// mapRumbleSource is a content.RumbleSource over a map of file names to
// contents
type mapRumbleSource map[string]string

func (m mapRumbleSource) GameRumble(name string) ([]byte, bool) {
	s, ok := m[name]
	return []byte(s), ok
}

func TestRumbleFile(t *testing.T) {
	game := func(selected int) *storage.GameEntry {
		return &storage.GameEntry{
			Discs:        []storage.GameDisc{{Index: 0}, {Index: 1}},
			SelectedDisc: selected,
		}
	}
	both := mapRumbleSource{"T-1219H": "shared", "T-1219H.disc2": "disc2"}
	discOnly := mapRumbleSource{"T-1219H.disc2": "disc2"}
	tests := []struct {
		name     string
		source   content.RumbleSource
		disc     bool
		game     *storage.GameEntry
		wantName string
		wantSrc  string
		wantOK   bool
	}{
		{"cartridge ignores disc files", both, false, game(1), "T-1219H", "shared", true},
		{"disc-specific file", both, true, game(1), "T-1219H.disc2", "disc2", true},
		{"no disc-specific file", both, true, game(0), "T-1219H", "shared", true},
		{"no current game", both, true, nil, "T-1219H", "shared", true},
		{"disc-specific file only, other disc", discOnly, true, game(0), "", "", false},
		{"no file", mapRumbleSource{}, true, game(1), "", "", false},
		{"no source", nil, true, game(1), "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm := &GameplayManager{
				systemInfo:   coreif.SystemInfo{Disc: tt.disc},
				currentGame:  tt.game,
				rumbleSource: tt.source,
			}
			name, src, ok := gm.rumbleFile("T-1219H")
			if ok != tt.wantOK || (ok && (name != tt.wantName || string(src) != tt.wantSrc)) {
				t.Errorf("rumbleFile() = %q, %q, %v, want %q, %q, %v", name, src, ok, tt.wantName, tt.wantSrc, tt.wantOK)
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
