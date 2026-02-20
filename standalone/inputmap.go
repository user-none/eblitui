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
	"A":         ebiten.KeyA,
	"B":         ebiten.KeyB,
	"C":         ebiten.KeyC,
	"D":         ebiten.KeyD,
	"E":         ebiten.KeyE,
	"F":         ebiten.KeyF,
	"G":         ebiten.KeyG,
	"H":         ebiten.KeyH,
	"I":         ebiten.KeyI,
	"J":         ebiten.KeyJ,
	"K":         ebiten.KeyK,
	"L":         ebiten.KeyL,
	"M":         ebiten.KeyM,
	"N":         ebiten.KeyN,
	"O":         ebiten.KeyO,
	"P":         ebiten.KeyP,
	"Q":         ebiten.KeyQ,
	"R":         ebiten.KeyR,
	"S":         ebiten.KeyS,
	"T":         ebiten.KeyT,
	"U":         ebiten.KeyU,
	"V":         ebiten.KeyV,
	"W":         ebiten.KeyW,
	"X":         ebiten.KeyX,
	"Y":         ebiten.KeyY,
	"Z":         ebiten.KeyZ,
	"Enter":     ebiten.KeyEnter,
	"Backspace": ebiten.KeyBackspace,
	"Space":     ebiten.KeySpace,
	"Semicolon": ebiten.KeySemicolon,
	"Comma":     ebiten.KeyComma,
	"Period":    ebiten.KeyPeriod,
	"Slash":     ebiten.KeySlash,
	"Tab":       ebiten.KeyTab,
	"Escape":    ebiten.KeyEscape,
	"Shift":     ebiten.KeyShift,
	"F1":        ebiten.KeyF1,
	"F2":        ebiten.KeyF2,
	"F3":        ebiten.KeyF3,
	"F4":        ebiten.KeyF4,
	"F5":        ebiten.KeyF5,
	"F6":        ebiten.KeyF6,
	"F7":        ebiten.KeyF7,
	"F8":        ebiten.KeyF8,
	"F9":        ebiten.KeyF9,
	"F10":       ebiten.KeyF10,
	"F11":       ebiten.KeyF11,
	"F12":       ebiten.KeyF12,
}

// padNameMap maps gamepad button name strings to ebiten StandardGamepadButton values.
var padNameMap = map[string]ebiten.StandardGamepadButton{
	"A":      ebiten.StandardGamepadButtonRightBottom,
	"B":      ebiten.StandardGamepadButtonRightRight,
	"X":      ebiten.StandardGamepadButtonRightLeft,
	"Y":      ebiten.StandardGamepadButtonRightTop,
	"L1":     ebiten.StandardGamepadButtonFrontTopLeft,
	"R1":     ebiten.StandardGamepadButtonFrontTopRight,
	"L2":     ebiten.StandardGamepadButtonFrontBottomLeft,
	"R2":     ebiten.StandardGamepadButtonFrontBottomRight,
	"Start":  ebiten.StandardGamepadButtonCenterRight,
	"Select": ebiten.StandardGamepadButtonCenterLeft,
}

// reservedKeys are keyboard keys used by the standalone UI for non-button
// functions (D-pad, menus, save states, etc.). These are skipped when
// building default mappings to avoid conflicts.
var reservedKeys = map[ebiten.Key]bool{
	ebiten.KeyW:      true, // D-pad Up
	ebiten.KeyA:      true, // D-pad Left
	ebiten.KeyS:      true, // D-pad Down
	ebiten.KeyD:      true, // D-pad Right
	ebiten.KeyEscape: true, // Pause menu
	ebiten.KeyTab:    true, // Achievement overlay
	ebiten.KeyR:      true, // Rewind
	ebiten.KeyF1:     true, // Save state
	ebiten.KeyF2:     true, // Cycle slot
	ebiten.KeyF3:     true, // Load state
	ebiten.KeyF4:     true, // Turbo
	ebiten.KeyF11:    true, // Fullscreen
	ebiten.KeyF12:    true, // Screenshot
	ebiten.KeyShift:  true, // Modifier (Shift+F2)
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

// BuildDefaultMapping creates an InputMapping from the given button definitions.
// It parses DefaultKey and DefaultPad strings from each button and builds
// maps keyed by the button's bit ID. Empty or unknown strings are skipped.
// Keys that conflict with reserved standalone UI keys are also skipped.
func BuildDefaultMapping(buttons []emucore.Button) InputMapping {
	m := InputMapping{
		Keys:    make(map[int]ebiten.Key),
		Gamepad: make(map[int]ebiten.StandardGamepadButton),
	}

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

// PollButtons reads P1 input from keyboard (WASD D-pad + mapped keys)
// and gamepad (D-pad + analog stick + mapped buttons). Returns a button bitmask.
func PollButtons(mapping InputMapping, gamepadID ebiten.GamepadID, hasGamepad bool) uint32 {
	var buttons uint32

	// Keyboard D-pad (WASD)
	if ebiten.IsKeyPressed(ebiten.KeyW) {
		buttons |= 1 << emucore.ButtonUp
	}
	if ebiten.IsKeyPressed(ebiten.KeyS) {
		buttons |= 1 << emucore.ButtonDown
	}
	if ebiten.IsKeyPressed(ebiten.KeyA) {
		buttons |= 1 << emucore.ButtonLeft
	}
	if ebiten.IsKeyPressed(ebiten.KeyD) {
		buttons |= 1 << emucore.ButtonRight
	}

	// Keyboard mapped buttons
	for bitID, key := range mapping.Keys {
		if ebiten.IsKeyPressed(key) {
			buttons |= 1 << uint(bitID)
		}
	}

	if !hasGamepad {
		return buttons
	}

	// Gamepad D-pad
	if ebiten.IsStandardGamepadButtonPressed(gamepadID, ebiten.StandardGamepadButtonLeftTop) {
		buttons |= 1 << emucore.ButtonUp
	}
	if ebiten.IsStandardGamepadButtonPressed(gamepadID, ebiten.StandardGamepadButtonLeftBottom) {
		buttons |= 1 << emucore.ButtonDown
	}
	if ebiten.IsStandardGamepadButtonPressed(gamepadID, ebiten.StandardGamepadButtonLeftLeft) {
		buttons |= 1 << emucore.ButtonLeft
	}
	if ebiten.IsStandardGamepadButtonPressed(gamepadID, ebiten.StandardGamepadButtonLeftRight) {
		buttons |= 1 << emucore.ButtonRight
	}

	// Gamepad mapped buttons
	for bitID, padBtn := range mapping.Gamepad {
		if ebiten.IsStandardGamepadButtonPressed(gamepadID, padBtn) {
			buttons |= 1 << uint(bitID)
		}
	}

	// Analog stick
	axisX := ebiten.StandardGamepadAxisValue(gamepadID, ebiten.StandardGamepadAxisLeftStickHorizontal)
	axisY := ebiten.StandardGamepadAxisValue(gamepadID, ebiten.StandardGamepadAxisLeftStickVertical)
	if axisX < -0.25 {
		buttons |= 1 << emucore.ButtonLeft
	}
	if axisX > 0.25 {
		buttons |= 1 << emucore.ButtonRight
	}
	if axisY < -0.25 {
		buttons |= 1 << emucore.ButtonUp
	}
	if axisY > 0.25 {
		buttons |= 1 << emucore.ButtonDown
	}

	return buttons
}

// PollGamepadButtons reads P2 input from gamepad only (D-pad + analog stick
// + mapped buttons). No keyboard D-pad since WASD belongs to P1.
func PollGamepadButtons(mapping InputMapping, gamepadID ebiten.GamepadID) uint32 {
	var buttons uint32

	// Gamepad D-pad
	if ebiten.IsStandardGamepadButtonPressed(gamepadID, ebiten.StandardGamepadButtonLeftTop) {
		buttons |= 1 << emucore.ButtonUp
	}
	if ebiten.IsStandardGamepadButtonPressed(gamepadID, ebiten.StandardGamepadButtonLeftBottom) {
		buttons |= 1 << emucore.ButtonDown
	}
	if ebiten.IsStandardGamepadButtonPressed(gamepadID, ebiten.StandardGamepadButtonLeftLeft) {
		buttons |= 1 << emucore.ButtonLeft
	}
	if ebiten.IsStandardGamepadButtonPressed(gamepadID, ebiten.StandardGamepadButtonLeftRight) {
		buttons |= 1 << emucore.ButtonRight
	}

	// Gamepad mapped buttons
	for bitID, padBtn := range mapping.Gamepad {
		if ebiten.IsStandardGamepadButtonPressed(gamepadID, padBtn) {
			buttons |= 1 << uint(bitID)
		}
	}

	// Analog stick
	axisX := ebiten.StandardGamepadAxisValue(gamepadID, ebiten.StandardGamepadAxisLeftStickHorizontal)
	axisY := ebiten.StandardGamepadAxisValue(gamepadID, ebiten.StandardGamepadAxisLeftStickVertical)
	if axisX < -0.25 {
		buttons |= 1 << emucore.ButtonLeft
	}
	if axisX > 0.25 {
		buttons |= 1 << emucore.ButtonRight
	}
	if axisY < -0.25 {
		buttons |= 1 << emucore.ButtonUp
	}
	if axisY > 0.25 {
		buttons |= 1 << emucore.ButtonDown
	}

	return buttons
}
