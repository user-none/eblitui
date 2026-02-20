//go:build !ios && !libretro

package standalone

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	emucore "github.com/user-none/eblitui/api"
)

func TestParseKeyValid(t *testing.T) {
	tests := []struct {
		name string
		want ebiten.Key
	}{
		{"J", ebiten.KeyJ},
		{"K", ebiten.KeyK},
		{"Enter", ebiten.KeyEnter},
		{"Semicolon", ebiten.KeySemicolon},
		{"Space", ebiten.KeySpace},
		{"F5", ebiten.KeyF5},
	}
	for _, tt := range tests {
		k, ok := ParseKey(tt.name)
		if !ok {
			t.Errorf("ParseKey(%q) returned false, want true", tt.name)
		}
		if k != tt.want {
			t.Errorf("ParseKey(%q) = %v, want %v", tt.name, k, tt.want)
		}
	}
}

func TestParseKeyInvalid(t *testing.T) {
	invalids := []string{"", "jj", "enter", "ENTER", "F13", "Unknown"}
	for _, name := range invalids {
		_, ok := ParseKey(name)
		if ok {
			t.Errorf("ParseKey(%q) returned true, want false", name)
		}
	}
}

func TestParsePadValid(t *testing.T) {
	tests := []struct {
		name string
		want ebiten.StandardGamepadButton
	}{
		{"A", ebiten.StandardGamepadButtonRightBottom},
		{"B", ebiten.StandardGamepadButtonRightRight},
		{"X", ebiten.StandardGamepadButtonRightLeft},
		{"Y", ebiten.StandardGamepadButtonRightTop},
		{"L1", ebiten.StandardGamepadButtonFrontTopLeft},
		{"R1", ebiten.StandardGamepadButtonFrontTopRight},
		{"Start", ebiten.StandardGamepadButtonCenterRight},
		{"Select", ebiten.StandardGamepadButtonCenterLeft},
	}
	for _, tt := range tests {
		b, ok := ParsePad(tt.name)
		if !ok {
			t.Errorf("ParsePad(%q) returned false, want true", tt.name)
		}
		if b != tt.want {
			t.Errorf("ParsePad(%q) = %v, want %v", tt.name, b, tt.want)
		}
	}
}

func TestParsePadInvalid(t *testing.T) {
	invalids := []string{"", "a", "start", "L3", "Unknown"}
	for _, name := range invalids {
		_, ok := ParsePad(name)
		if ok {
			t.Errorf("ParsePad(%q) returned true, want false", name)
		}
	}
}

func TestBuildDefaultMappingGenesis6Button(t *testing.T) {
	buttons := []emucore.Button{
		{Name: "A", ID: 4, DefaultKey: "J", DefaultPad: "A"},
		{Name: "B", ID: 5, DefaultKey: "K", DefaultPad: "B"},
		{Name: "C", ID: 6, DefaultKey: "L", DefaultPad: "R1"},
		{Name: "X", ID: 8, DefaultKey: "U", DefaultPad: "X"},
		{Name: "Y", ID: 9, DefaultKey: "I", DefaultPad: "Y"},
		{Name: "Z", ID: 10, DefaultKey: "O", DefaultPad: "L1"},
		{Name: "Start", ID: 7, DefaultKey: "Enter", DefaultPad: "Start"},
	}

	m := BuildDefaultMapping(buttons)

	// Verify all keyboard mappings
	expectedKeys := map[int]ebiten.Key{
		4:  ebiten.KeyJ,
		5:  ebiten.KeyK,
		6:  ebiten.KeyL,
		8:  ebiten.KeyU,
		9:  ebiten.KeyI,
		10: ebiten.KeyO,
		7:  ebiten.KeyEnter,
	}
	if len(m.Keys) != len(expectedKeys) {
		t.Errorf("Keys map has %d entries, want %d", len(m.Keys), len(expectedKeys))
	}
	for id, want := range expectedKeys {
		got, ok := m.Keys[id]
		if !ok {
			t.Errorf("Keys[%d] missing", id)
			continue
		}
		if got != want {
			t.Errorf("Keys[%d] = %v, want %v", id, got, want)
		}
	}

	// Verify all gamepad mappings
	expectedPad := map[int]ebiten.StandardGamepadButton{
		4:  ebiten.StandardGamepadButtonRightBottom,
		5:  ebiten.StandardGamepadButtonRightRight,
		6:  ebiten.StandardGamepadButtonFrontTopRight,
		8:  ebiten.StandardGamepadButtonRightLeft,
		9:  ebiten.StandardGamepadButtonRightTop,
		10: ebiten.StandardGamepadButtonFrontTopLeft,
		7:  ebiten.StandardGamepadButtonCenterRight,
	}
	if len(m.Gamepad) != len(expectedPad) {
		t.Errorf("Gamepad map has %d entries, want %d", len(m.Gamepad), len(expectedPad))
	}
	for id, want := range expectedPad {
		got, ok := m.Gamepad[id]
		if !ok {
			t.Errorf("Gamepad[%d] missing", id)
			continue
		}
		if got != want {
			t.Errorf("Gamepad[%d] = %v, want %v", id, got, want)
		}
	}
}

func TestBuildDefaultMapping3Button(t *testing.T) {
	buttons := []emucore.Button{
		{Name: "A", ID: 4, DefaultKey: "J", DefaultPad: "A"},
		{Name: "B", ID: 5, DefaultKey: "K", DefaultPad: "B"},
		{Name: "C", ID: 6, DefaultKey: "L", DefaultPad: "R1"},
		{Name: "Start", ID: 7, DefaultKey: "Enter", DefaultPad: "Start"},
	}

	m := BuildDefaultMapping(buttons)

	if len(m.Keys) != 4 {
		t.Errorf("Keys map has %d entries, want 4", len(m.Keys))
	}
	if len(m.Gamepad) != 4 {
		t.Errorf("Gamepad map has %d entries, want 4", len(m.Gamepad))
	}

	// Verify no entries for X/Y/Z bit IDs
	for _, id := range []int{8, 9, 10} {
		if _, ok := m.Keys[id]; ok {
			t.Errorf("Keys[%d] should not exist for 3-button layout", id)
		}
		if _, ok := m.Gamepad[id]; ok {
			t.Errorf("Gamepad[%d] should not exist for 3-button layout", id)
		}
	}
}

func TestBuildDefaultMappingEmptyDefaults(t *testing.T) {
	buttons := []emucore.Button{
		{Name: "A", ID: 4},
		{Name: "B", ID: 5},
	}

	m := BuildDefaultMapping(buttons)

	if len(m.Keys) != 0 {
		t.Errorf("Keys map has %d entries, want 0", len(m.Keys))
	}
	if len(m.Gamepad) != 0 {
		t.Errorf("Gamepad map has %d entries, want 0", len(m.Gamepad))
	}
}

func TestBuildDefaultMappingReservedKeysSkipped(t *testing.T) {
	// All of these are reserved keys that the standalone UI uses
	reservedNames := []string{
		"W", "A", "S", "D", // D-pad
		"Escape", "Tab", "R", // Menu, overlay, rewind
		"F1", "F2", "F3", "F4", // Save state, turbo
		"F11", "F12", // Fullscreen, screenshot
		"Shift", // Modifier
	}

	for _, name := range reservedNames {
		buttons := []emucore.Button{
			{Name: "Test", ID: 4, DefaultKey: name, DefaultPad: "A"},
		}
		m := BuildDefaultMapping(buttons)
		if _, ok := m.Keys[4]; ok {
			t.Errorf("Reserved key %q was not skipped", name)
		}
		// Gamepad should still be mapped
		if _, ok := m.Gamepad[4]; !ok {
			t.Errorf("Gamepad mapping missing when key %q was reserved", name)
		}
	}
}

func TestBuildDefaultMappingUnknownStringsSkipped(t *testing.T) {
	buttons := []emucore.Button{
		{Name: "A", ID: 4, DefaultKey: "BadKey", DefaultPad: "BadPad"},
	}

	m := BuildDefaultMapping(buttons)

	if len(m.Keys) != 0 {
		t.Errorf("Keys map has %d entries, want 0 for unknown key", len(m.Keys))
	}
	if len(m.Gamepad) != 0 {
		t.Errorf("Gamepad map has %d entries, want 0 for unknown pad", len(m.Gamepad))
	}
}
