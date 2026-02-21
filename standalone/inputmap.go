//go:build !ios && !libretro

package standalone

import (
	"github.com/hajimehoshi/ebiten/v2"
	emucore "github.com/user-none/eblitui/api"
)

// InputMapping maps button bit IDs to ebiten input types.
// Keyed by the Button.ID (bit position in the uint32 bitmask).
type InputMapping struct {
	Keys    map[int]ebiten.Key                   // bit ID -> keyboard key
	Gamepad map[int]ebiten.StandardGamepadButton // bit ID -> gamepad button
}

// keyNameMap maps short key name strings to ebiten.Key values.
var keyNameMap = map[string]ebiten.Key{
	"A":          ebiten.KeyA,
	"B":          ebiten.KeyB,
	"C":          ebiten.KeyC,
	"D":          ebiten.KeyD,
	"E":          ebiten.KeyE,
	"F":          ebiten.KeyF,
	"G":          ebiten.KeyG,
	"H":          ebiten.KeyH,
	"I":          ebiten.KeyI,
	"J":          ebiten.KeyJ,
	"K":          ebiten.KeyK,
	"L":          ebiten.KeyL,
	"M":          ebiten.KeyM,
	"N":          ebiten.KeyN,
	"O":          ebiten.KeyO,
	"P":          ebiten.KeyP,
	"Q":          ebiten.KeyQ,
	"R":          ebiten.KeyR,
	"S":          ebiten.KeyS,
	"T":          ebiten.KeyT,
	"U":          ebiten.KeyU,
	"V":          ebiten.KeyV,
	"W":          ebiten.KeyW,
	"X":          ebiten.KeyX,
	"Y":          ebiten.KeyY,
	"Z":          ebiten.KeyZ,
	"0":          ebiten.Key0,
	"1":          ebiten.Key1,
	"2":          ebiten.Key2,
	"3":          ebiten.Key3,
	"4":          ebiten.Key4,
	"5":          ebiten.Key5,
	"6":          ebiten.Key6,
	"7":          ebiten.Key7,
	"8":          ebiten.Key8,
	"9":          ebiten.Key9,
	"Enter":      ebiten.KeyEnter,
	"Backspace":  ebiten.KeyBackspace,
	"Space":      ebiten.KeySpace,
	"Semicolon":  ebiten.KeySemicolon,
	"Comma":      ebiten.KeyComma,
	"Period":     ebiten.KeyPeriod,
	"Slash":      ebiten.KeySlash,
	"Tab":        ebiten.KeyTab,
	"Escape":     ebiten.KeyEscape,
	"Shift":      ebiten.KeyShift,
	"ArrowUp":    ebiten.KeyArrowUp,
	"ArrowDown":  ebiten.KeyArrowDown,
	"ArrowLeft":  ebiten.KeyArrowLeft,
	"ArrowRight": ebiten.KeyArrowRight,
	"[":          ebiten.KeyLeftBracket,
	"]":          ebiten.KeyRightBracket,
	"-":          ebiten.KeyMinus,
	"=":          ebiten.KeyEqual,
	"'":          ebiten.KeyApostrophe,
	"F1":         ebiten.KeyF1,
	"F2":         ebiten.KeyF2,
	"F3":         ebiten.KeyF3,
	"F4":         ebiten.KeyF4,
	"F5":         ebiten.KeyF5,
	"F6":         ebiten.KeyF6,
	"F7":         ebiten.KeyF7,
	"F8":         ebiten.KeyF8,
	"F9":         ebiten.KeyF9,
	"F10":        ebiten.KeyF10,
	"F11":        ebiten.KeyF11,
	"F12":        ebiten.KeyF12,
}

// padNameMap maps gamepad button name strings to ebiten StandardGamepadButton values.
var padNameMap = map[string]ebiten.StandardGamepadButton{
	"A":         ebiten.StandardGamepadButtonRightBottom,
	"B":         ebiten.StandardGamepadButtonRightRight,
	"X":         ebiten.StandardGamepadButtonRightLeft,
	"Y":         ebiten.StandardGamepadButtonRightTop,
	"L1":        ebiten.StandardGamepadButtonFrontTopLeft,
	"R1":        ebiten.StandardGamepadButtonFrontTopRight,
	"L2":        ebiten.StandardGamepadButtonFrontBottomLeft,
	"R2":        ebiten.StandardGamepadButtonFrontBottomRight,
	"Start":     ebiten.StandardGamepadButtonCenterRight,
	"Select":    ebiten.StandardGamepadButtonCenterLeft,
	"DpadUp":    ebiten.StandardGamepadButtonLeftTop,
	"DpadDown":  ebiten.StandardGamepadButtonLeftBottom,
	"DpadLeft":  ebiten.StandardGamepadButtonLeftLeft,
	"DpadRight": ebiten.StandardGamepadButtonLeftRight,
	"L3":        ebiten.StandardGamepadButtonLeftStick,
	"R3":        ebiten.StandardGamepadButtonRightStick,
}

// reservedKeys are keyboard keys used by the standalone UI for non-gameplay
// functions (menus, save states, etc.). These cannot be assigned as button bindings.
var reservedKeys = map[ebiten.Key]bool{
	ebiten.KeyEscape:      true, // Pause menu
	ebiten.KeyTab:         true, // Achievement overlay
	ebiten.KeyR:           true, // Rewind
	ebiten.KeyF1:          true, // Save state
	ebiten.KeyF2:          true, // Cycle slot
	ebiten.KeyF3:          true, // Load state
	ebiten.KeyF4:          true, // Turbo
	ebiten.KeyF5:          true,
	ebiten.KeyF6:          true,
	ebiten.KeyF7:          true,
	ebiten.KeyF8:          true,
	ebiten.KeyF9:          true,
	ebiten.KeyF10:         true,
	ebiten.KeyF11:         true, // Fullscreen
	ebiten.KeyF12:         true, // Screenshot
	ebiten.KeyShift:       true, // Modifier (Shift+F2)
	ebiten.KeyControl:     true,
	ebiten.KeyAlt:         true,
	ebiten.KeyMeta:        true,
	ebiten.KeyGraveAccent: true, // ~ key
}

// Reverse lookup maps (built from keyNameMap/padNameMap at init).
var keyToName map[ebiten.Key]string
var padToName map[ebiten.StandardGamepadButton]string

func init() {
	keyToName = make(map[ebiten.Key]string, len(keyNameMap))
	for name, key := range keyNameMap {
		keyToName[key] = name
	}
	padToName = make(map[ebiten.StandardGamepadButton]string, len(padNameMap))
	for name, btn := range padNameMap {
		padToName[btn] = name
	}
}

// KeyToName converts an ebiten.Key to its name string.
// Returns the name and true if the key has a name, or "" and false otherwise.
func KeyToName(k ebiten.Key) (string, bool) {
	name, ok := keyToName[k]
	return name, ok
}

// PadToName converts an ebiten.StandardGamepadButton to its name string.
// Returns the name and true if the button has a name, or "" and false otherwise.
func PadToName(b ebiten.StandardGamepadButton) (string, bool) {
	name, ok := padToName[b]
	return name, ok
}

// IsReservedKey returns true if the key is reserved for UI functions.
func IsReservedKey(k ebiten.Key) bool {
	return reservedKeys[k]
}

// ParseKey converts a key name string to an ebiten.Key.
// Returns the key and true if the name is valid, or 0 and false otherwise.
func ParseKey(name string) (ebiten.Key, bool) {
	k, ok := keyNameMap[name]
	return k, ok
}

// ParsePad converts a gamepad button name string to an ebiten.StandardGamepadButton.
// Returns the button and true if the name is valid, or 0 and false otherwise.
func ParsePad(name string) (ebiten.StandardGamepadButton, bool) {
	b, ok := padNameMap[name]
	return b, ok
}

// D-pad button names used in config overrides and display.
var dpadButtons = []struct {
	Name       string
	BitID      int
	DefaultKey string
	DefaultPad string
}{
	{"Up", emucore.ButtonUp, "W", "DpadUp"},
	{"Down", emucore.ButtonDown, "S", "DpadDown"},
	{"Left", emucore.ButtonLeft, "A", "DpadLeft"},
	{"Right", emucore.ButtonRight, "D", "DpadRight"},
}

// BuildDefaultMapping creates an InputMapping from the given button definitions.
// It includes D-pad defaults (WASD keyboard, D-pad controller) plus adaptor buttons.
// Keys that conflict with reserved standalone UI keys are skipped.
func BuildDefaultMapping(buttons []emucore.Button) InputMapping {
	m := InputMapping{
		Keys:    make(map[int]ebiten.Key),
		Gamepad: make(map[int]ebiten.StandardGamepadButton),
	}

	// D-pad defaults
	for _, dp := range dpadButtons {
		if k, ok := ParseKey(dp.DefaultKey); ok {
			m.Keys[dp.BitID] = k
		}
		if b, ok := ParsePad(dp.DefaultPad); ok {
			m.Gamepad[dp.BitID] = b
		}
	}

	// Adaptor buttons
	for _, btn := range buttons {
		if btn.DefaultKey != "" {
			if k, ok := ParseKey(btn.DefaultKey); ok {
				if !reservedKeys[k] {
					m.Keys[btn.ID] = k
				}
			}
		}
		if btn.DefaultPad != "" {
			if b, ok := ParsePad(btn.DefaultPad); ok {
				m.Gamepad[btn.ID] = b
			}
		}
	}

	return m
}

// BuildMappingFromConfig creates an InputMapping using config overrides with
// adaptor defaults as fallback. D-pad defaults are WASD (keyboard) and
// DpadUp/Down/Left/Right (controller). For each button, the override map
// is checked first; if absent or invalid, the adaptor default is used.
func BuildMappingFromConfig(buttons []emucore.Button, kbOverrides, padOverrides map[string]string) InputMapping {
	m := InputMapping{
		Keys:    make(map[int]ebiten.Key),
		Gamepad: make(map[int]ebiten.StandardGamepadButton),
	}

	// D-pad
	for _, dp := range dpadButtons {
		// Keyboard
		if override, ok := kbOverrides[dp.Name]; ok {
			if k, ok := ParseKey(override); ok && !reservedKeys[k] {
				m.Keys[dp.BitID] = k
			}
		} else {
			if k, ok := ParseKey(dp.DefaultKey); ok {
				m.Keys[dp.BitID] = k
			}
		}
		// Controller
		if override, ok := padOverrides[dp.Name]; ok {
			if b, ok := ParsePad(override); ok {
				m.Gamepad[dp.BitID] = b
			}
		} else {
			if b, ok := ParsePad(dp.DefaultPad); ok {
				m.Gamepad[dp.BitID] = b
			}
		}
	}

	// Adaptor buttons
	for _, btn := range buttons {
		// Keyboard
		if override, ok := kbOverrides[btn.Name]; ok {
			if k, ok := ParseKey(override); ok && !reservedKeys[k] {
				m.Keys[btn.ID] = k
			}
		} else if btn.DefaultKey != "" {
			if k, ok := ParseKey(btn.DefaultKey); ok && !reservedKeys[k] {
				m.Keys[btn.ID] = k
			}
		}
		// Controller
		if override, ok := padOverrides[btn.Name]; ok {
			if b, ok := ParsePad(override); ok {
				m.Gamepad[btn.ID] = b
			}
		} else if btn.DefaultPad != "" {
			if b, ok := ParsePad(btn.DefaultPad); ok {
				m.Gamepad[btn.ID] = b
			}
		}
	}

	return m
}

// ResolveKeyDisplay returns the display string for a button's current keyboard
// binding, checking overrides first then falling back to the provided default.
func ResolveKeyDisplay(buttonName string, defaultKey string, overrides map[string]string) string {
	if override, ok := overrides[buttonName]; ok {
		return override
	}
	if defaultKey != "" {
		return defaultKey
	}
	return ""
}

// ResolvePadDisplay returns the display string for a button's current controller
// binding, checking overrides first then falling back to the provided default.
func ResolvePadDisplay(buttonName string, defaultPad string, overrides map[string]string) string {
	if override, ok := overrides[buttonName]; ok {
		return override
	}
	if defaultPad != "" {
		return defaultPad
	}
	return ""
}

// PollButtons reads P1 input from keyboard and gamepad (including D-pad
// and analog stick). All buttons including D-pad are in the mapping.
// When disableAnalog is true, the analog stick is not polled.
// Returns a button bitmask.
func PollButtons(mapping InputMapping, gamepadID ebiten.GamepadID, hasGamepad, disableAnalog bool) uint32 {
	var buttons uint32

	// All keyboard-mapped buttons (including D-pad)
	for bitID, key := range mapping.Keys {
		if ebiten.IsKeyPressed(key) {
			buttons |= 1 << uint(bitID)
		}
	}

	if !hasGamepad {
		return buttons
	}

	// All gamepad-mapped buttons (including D-pad)
	for bitID, padBtn := range mapping.Gamepad {
		if ebiten.IsStandardGamepadButtonPressed(gamepadID, padBtn) {
			buttons |= 1 << uint(bitID)
		}
	}

	// Analog stick follows d-pad mappings so remapped d-pad buttons
	// are also triggered by the stick.
	if !disableAnalog {
		pollAnalogStick(&buttons, mapping, gamepadID)
	}

	return buttons
}

// PollGamepadButtons reads P2 input from gamepad only (D-pad + analog stick
// + mapped buttons). No keyboard since that belongs to P1.
// When disableAnalog is true, the analog stick is not polled.
// All D-pad buttons are expected in the mapping.
func PollGamepadButtons(mapping InputMapping, gamepadID ebiten.GamepadID, disableAnalog bool) uint32 {
	var buttons uint32

	// All gamepad-mapped buttons (including D-pad)
	for bitID, padBtn := range mapping.Gamepad {
		if ebiten.IsStandardGamepadButtonPressed(gamepadID, padBtn) {
			buttons |= 1 << uint(bitID)
		}
	}

	// Analog stick follows d-pad mappings so remapped d-pad buttons
	// are also triggered by the stick.
	if !disableAnalog {
		pollAnalogStick(&buttons, mapping, gamepadID)
	}

	return buttons
}

// pollAnalogStick reads the left analog stick and sets the same bit IDs
// that the d-pad buttons are mapped to. This ensures the stick follows
// any d-pad remapping in the controller bindings.
func pollAnalogStick(buttons *uint32, mapping InputMapping, gamepadID ebiten.GamepadID) {
	axisX := ebiten.StandardGamepadAxisValue(gamepadID, ebiten.StandardGamepadAxisLeftStickHorizontal)
	axisY := ebiten.StandardGamepadAxisValue(gamepadID, ebiten.StandardGamepadAxisLeftStickVertical)

	for bitID, padBtn := range mapping.Gamepad {
		switch padBtn {
		case ebiten.StandardGamepadButtonLeftLeft:
			if axisX < -0.25 {
				*buttons |= 1 << uint(bitID)
			}
		case ebiten.StandardGamepadButtonLeftRight:
			if axisX > 0.25 {
				*buttons |= 1 << uint(bitID)
			}
		case ebiten.StandardGamepadButtonLeftTop:
			if axisY < -0.25 {
				*buttons |= 1 << uint(bitID)
			}
		case ebiten.StandardGamepadButtonLeftBottom:
			if axisY > 0.25 {
				*buttons |= 1 << uint(bitID)
			}
		}
	}
}
