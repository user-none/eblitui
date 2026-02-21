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
		{"ArrowUp", ebiten.KeyArrowUp},
		{"ArrowDown", ebiten.KeyArrowDown},
		{"0", ebiten.Key0},
		{"9", ebiten.Key9},
		{"[", ebiten.KeyLeftBracket},
		{"]", ebiten.KeyRightBracket},
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
		{"DpadUp", ebiten.StandardGamepadButtonLeftTop},
		{"DpadDown", ebiten.StandardGamepadButtonLeftBottom},
		{"DpadLeft", ebiten.StandardGamepadButtonLeftLeft},
		{"DpadRight", ebiten.StandardGamepadButtonLeftRight},
		{"L3", ebiten.StandardGamepadButtonLeftStick},
		{"R3", ebiten.StandardGamepadButtonRightStick},
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
	invalids := []string{"", "a", "start", "Unknown"}
	for _, name := range invalids {
		_, ok := ParsePad(name)
		if ok {
			t.Errorf("ParsePad(%q) returned true, want false", name)
		}
	}
}

func TestKeyToName(t *testing.T) {
	name, ok := KeyToName(ebiten.KeyJ)
	if !ok {
		t.Error("KeyToName(KeyJ) returned false")
	}
	if name != "J" {
		t.Errorf("KeyToName(KeyJ) = %q, want %q", name, "J")
	}

	name, ok = KeyToName(ebiten.KeyArrowUp)
	if !ok {
		t.Error("KeyToName(KeyArrowUp) returned false")
	}
	if name != "ArrowUp" {
		t.Errorf("KeyToName(KeyArrowUp) = %q, want %q", name, "ArrowUp")
	}
}

func TestPadToName(t *testing.T) {
	name, ok := PadToName(ebiten.StandardGamepadButtonRightBottom)
	if !ok {
		t.Error("PadToName(RightBottom) returned false")
	}
	if name != "A" {
		t.Errorf("PadToName(RightBottom) = %q, want %q", name, "A")
	}

	name, ok = PadToName(ebiten.StandardGamepadButtonLeftTop)
	if !ok {
		t.Error("PadToName(LeftTop) returned false")
	}
	if name != "DpadUp" {
		t.Errorf("PadToName(LeftTop) = %q, want %q", name, "DpadUp")
	}
}

func TestIsReservedKey(t *testing.T) {
	if !IsReservedKey(ebiten.KeyEscape) {
		t.Error("Escape should be reserved")
	}
	if !IsReservedKey(ebiten.KeyF1) {
		t.Error("F1 should be reserved")
	}
	if !IsReservedKey(ebiten.KeyControl) {
		t.Error("Control should be reserved")
	}
	if IsReservedKey(ebiten.KeyJ) {
		t.Error("J should not be reserved")
	}
	if IsReservedKey(ebiten.KeyW) {
		t.Error("W should not be reserved (d-pad keys are now mappable)")
	}
}

// D-pad count: 4 keys + 4 gamepad buttons are always included in BuildDefaultMapping
const dpadKeyCount = 4
const dpadPadCount = 4

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

	// Verify adaptor keyboard mappings (d-pad is also included)
	expectedKeys := map[int]ebiten.Key{
		emucore.ButtonUp:    ebiten.KeyW,
		emucore.ButtonDown:  ebiten.KeyS,
		emucore.ButtonLeft:  ebiten.KeyA,
		emucore.ButtonRight: ebiten.KeyD,
		4:                   ebiten.KeyJ,
		5:                   ebiten.KeyK,
		6:                   ebiten.KeyL,
		8:                   ebiten.KeyU,
		9:                   ebiten.KeyI,
		10:                  ebiten.KeyO,
		7:                   ebiten.KeyEnter,
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
		emucore.ButtonUp:    ebiten.StandardGamepadButtonLeftTop,
		emucore.ButtonDown:  ebiten.StandardGamepadButtonLeftBottom,
		emucore.ButtonLeft:  ebiten.StandardGamepadButtonLeftLeft,
		emucore.ButtonRight: ebiten.StandardGamepadButtonLeftRight,
		4:                   ebiten.StandardGamepadButtonRightBottom,
		5:                   ebiten.StandardGamepadButtonRightRight,
		6:                   ebiten.StandardGamepadButtonFrontTopRight,
		8:                   ebiten.StandardGamepadButtonRightLeft,
		9:                   ebiten.StandardGamepadButtonRightTop,
		10:                  ebiten.StandardGamepadButtonFrontTopLeft,
		7:                   ebiten.StandardGamepadButtonCenterRight,
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

	// 4 d-pad + 4 adaptor buttons
	if len(m.Keys) != dpadKeyCount+4 {
		t.Errorf("Keys map has %d entries, want %d", len(m.Keys), dpadKeyCount+4)
	}
	if len(m.Gamepad) != dpadPadCount+4 {
		t.Errorf("Gamepad map has %d entries, want %d", len(m.Gamepad), dpadPadCount+4)
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

	// D-pad defaults are always included
	if len(m.Keys) != dpadKeyCount {
		t.Errorf("Keys map has %d entries, want %d (d-pad only)", len(m.Keys), dpadKeyCount)
	}
	if len(m.Gamepad) != dpadPadCount {
		t.Errorf("Gamepad map has %d entries, want %d (d-pad only)", len(m.Gamepad), dpadPadCount)
	}
}

func TestBuildDefaultMappingReservedKeysSkipped(t *testing.T) {
	// Reserved keys that the standalone UI uses for non-gameplay functions
	reservedNames := []string{
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
			t.Errorf("Reserved key %q was not skipped for adaptor button", name)
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

	// D-pad defaults are still included; only the adaptor button with bad names is skipped
	if len(m.Keys) != dpadKeyCount {
		t.Errorf("Keys map has %d entries, want %d for unknown key", len(m.Keys), dpadKeyCount)
	}
	if len(m.Gamepad) != dpadPadCount {
		t.Errorf("Gamepad map has %d entries, want %d for unknown pad", len(m.Gamepad), dpadPadCount)
	}
}

func TestBuildMappingFromConfigDefaults(t *testing.T) {
	buttons := []emucore.Button{
		{Name: "A", ID: 4, DefaultKey: "J", DefaultPad: "A"},
		{Name: "Start", ID: 7, DefaultKey: "Enter", DefaultPad: "Start"},
	}

	// Empty overrides = all defaults
	m := BuildMappingFromConfig(buttons, nil, nil)

	// D-pad defaults
	if m.Keys[emucore.ButtonUp] != ebiten.KeyW {
		t.Error("expected d-pad up = W")
	}
	if m.Keys[emucore.ButtonDown] != ebiten.KeyS {
		t.Error("expected d-pad down = S")
	}
	if m.Gamepad[emucore.ButtonUp] != ebiten.StandardGamepadButtonLeftTop {
		t.Error("expected d-pad up = DpadUp")
	}

	// Adaptor defaults
	if m.Keys[4] != ebiten.KeyJ {
		t.Error("expected A button = J")
	}
	if m.Gamepad[7] != ebiten.StandardGamepadButtonCenterRight {
		t.Error("expected Start = CenterRight")
	}
}

func TestBuildMappingFromConfigOverrides(t *testing.T) {
	buttons := []emucore.Button{
		{Name: "A", ID: 4, DefaultKey: "J", DefaultPad: "A"},
		{Name: "B", ID: 5, DefaultKey: "K", DefaultPad: "B"},
	}

	kbOverrides := map[string]string{
		"Up": "ArrowUp",
		"A":  "Z",
	}
	padOverrides := map[string]string{
		"A": "Y",
	}

	m := BuildMappingFromConfig(buttons, kbOverrides, padOverrides)

	// Overridden d-pad
	if m.Keys[emucore.ButtonUp] != ebiten.KeyArrowUp {
		t.Error("expected d-pad up override = ArrowUp")
	}
	// Non-overridden d-pad stays default
	if m.Keys[emucore.ButtonDown] != ebiten.KeyS {
		t.Error("expected d-pad down = S (default)")
	}

	// Overridden adaptor button
	if m.Keys[4] != ebiten.KeyZ {
		t.Error("expected A button override = Z")
	}
	if m.Gamepad[4] != ebiten.StandardGamepadButtonRightTop {
		t.Error("expected A button pad override = Y (RightTop)")
	}

	// Non-overridden adaptor button stays default
	if m.Keys[5] != ebiten.KeyK {
		t.Error("expected B button = K (default)")
	}
	if m.Gamepad[5] != ebiten.StandardGamepadButtonRightRight {
		t.Error("expected B button pad = B (default)")
	}
}

func TestBuildMappingFromConfigReservedOverrideSkipped(t *testing.T) {
	buttons := []emucore.Button{
		{Name: "A", ID: 4, DefaultKey: "J", DefaultPad: "A"},
	}

	// Try to override with a reserved key
	kbOverrides := map[string]string{
		"A": "Escape",
	}

	m := BuildMappingFromConfig(buttons, kbOverrides, nil)

	// Reserved key override should be skipped entirely (no fallback to default)
	if _, ok := m.Keys[4]; ok {
		t.Error("reserved key override should result in no mapping for that button")
	}
}

func TestResolveKeyDisplay(t *testing.T) {
	overrides := map[string]string{
		"Up": "ArrowUp",
	}

	// Overridden
	if got := ResolveKeyDisplay("Up", "W", overrides); got != "ArrowUp" {
		t.Errorf("expected ArrowUp, got %q", got)
	}

	// Not overridden - falls back to default
	if got := ResolveKeyDisplay("Down", "S", overrides); got != "S" {
		t.Errorf("expected S, got %q", got)
	}

	// No default
	if got := ResolveKeyDisplay("Missing", "", nil); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestResolvePadDisplay(t *testing.T) {
	overrides := map[string]string{
		"A": "Y",
	}

	if got := ResolvePadDisplay("A", "A", overrides); got != "Y" {
		t.Errorf("expected Y, got %q", got)
	}

	if got := ResolvePadDisplay("B", "B", overrides); got != "B" {
		t.Errorf("expected B, got %q", got)
	}
}
