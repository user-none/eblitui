//go:build !libretro

package standalone

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/user-none/eblitui/standalone/style"
	"github.com/user-none/eblitui/standalone/types"
)

// UINavigation represents the result of UI input polling
type UINavigation struct {
	Direction    int  // 0=none, 1=up, 2=down, 3=left, 4=right
	Activate     bool // A/Cross button just pressed
	Back         bool // B/Circle button just pressed
	OpenSettings bool // Start button just pressed
	FocusChanged bool // True if navigation caused focus change this frame
}

// InputManager handles all input for UI navigation.
// It tracks gamepad state, handles repeat navigation, and provides
// a clean interface for UI code to query input state.
type InputManager struct {
	// Navigation state for repeat handling
	direction    int           // 0=none, 1=up, 2=down, 3=left, 4=right
	startTime    time.Time     // When direction was first pressed
	lastMove     time.Time     // When last move occurred
	repeatDelay  time.Duration // Current repeat interval
	focusChanged bool          // Track if focus changed this frame
}

// NewInputManager creates a new input manager
func NewInputManager() *InputManager {
	return &InputManager{
		repeatDelay: style.NavStartInterval,
	}
}

// Update polls input state. Should be called once per frame.
// Returns global key states: F12 screenshot and F11 fullscreen toggle.
func (im *InputManager) Update() (screenshotRequested, fullscreenToggle bool) {
	// Check for F12 screenshot (global, works everywhere)
	screenshotRequested = inpututil.IsKeyJustPressed(ebiten.KeyF12)
	// Check for F11 fullscreen toggle (global, works everywhere)
	fullscreenToggle = inpututil.IsKeyJustPressed(ebiten.KeyF11)
	return screenshotRequested, fullscreenToggle
}

// GetUINavigation returns the current UI navigation state.
// This handles keyboard arrow keys and gamepad D-pad/analog stick with repeat navigation,
// and A/B/Start button presses.
func (im *InputManager) GetUINavigation() UINavigation {
	result := UINavigation{}

	// Navigation direction flags - keyboard and gamepad both contribute
	navUp := false
	navDown := false
	navLeft := false
	navRight := false

	// Keyboard navigation (arrow keys)
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		navUp = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		navDown = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		navLeft = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		navRight = true
	}

	// Gamepad navigation (if connected)
	gamepadIDs := ebiten.AppendGamepadIDs(nil)
	var gamepadID ebiten.GamepadID
	hasGamepad := len(gamepadIDs) > 0
	if hasGamepad {
		gamepadID = gamepadIDs[0]

		// D-pad
		if ebiten.IsStandardGamepadButtonPressed(gamepadID, ebiten.StandardGamepadButtonLeftTop) {
			navUp = true
		}
		if ebiten.IsStandardGamepadButtonPressed(gamepadID, ebiten.StandardGamepadButtonLeftBottom) {
			navDown = true
		}
		if ebiten.IsStandardGamepadButtonPressed(gamepadID, ebiten.StandardGamepadButtonLeftLeft) {
			navLeft = true
		}
		if ebiten.IsStandardGamepadButtonPressed(gamepadID, ebiten.StandardGamepadButtonLeftRight) {
			navRight = true
		}

		// Analog stick (0.5 threshold for UI)
		axisY := ebiten.StandardGamepadAxisValue(gamepadID, ebiten.StandardGamepadAxisLeftStickVertical)
		axisX := ebiten.StandardGamepadAxisValue(gamepadID, ebiten.StandardGamepadAxisLeftStickHorizontal)
		if axisY < -0.5 {
			navUp = true
		}
		if axisY > 0.5 {
			navDown = true
		}
		if axisX < -0.5 {
			navLeft = true
		}
		if axisX > 0.5 {
			navRight = true
		}
	}

	// Determine desired direction (vertical takes priority for menu-like behavior)
	desiredDir := types.DirNone
	if navUp {
		desiredDir = types.DirUp
	} else if navDown {
		desiredDir = types.DirDown
	} else if navLeft {
		desiredDir = types.DirLeft
	} else if navRight {
		desiredDir = types.DirRight
	}

	now := time.Now()
	im.focusChanged = false

	if desiredDir == types.DirNone {
		// No direction pressed - reset state
		im.direction = types.DirNone
		im.repeatDelay = style.NavStartInterval
	} else if desiredDir != im.direction {
		// Direction changed - move immediately and start tracking
		im.direction = desiredDir
		im.startTime = now
		im.lastMove = now
		im.repeatDelay = style.NavStartInterval
		im.focusChanged = true
		result.Direction = desiredDir
	} else {
		// Same direction held - check for repeat
		holdDuration := now.Sub(im.startTime)
		timeSinceLastMove := now.Sub(im.lastMove)

		if holdDuration >= style.NavInitialDelay && timeSinceLastMove >= im.repeatDelay {
			// Time to repeat
			im.focusChanged = true
			im.lastMove = now
			result.Direction = desiredDir

			// Accelerate (decrease interval)
			im.repeatDelay -= style.NavAcceleration
			if im.repeatDelay < style.NavMinInterval {
				im.repeatDelay = style.NavMinInterval
			}
		}
	}

	result.FocusChanged = im.focusChanged

	// Activate: A button (gamepad only - Enter/Space handled by ebitenui)
	if hasGamepad {
		result.Activate = inpututil.IsStandardGamepadButtonJustPressed(gamepadID, ebiten.StandardGamepadButtonRightBottom)
	}

	// Back: ESC (keyboard) or B button (gamepad)
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		result.Back = true
	}
	if hasGamepad && inpututil.IsStandardGamepadButtonJustPressed(gamepadID, ebiten.StandardGamepadButtonRightRight) {
		result.Back = true
	}

	// Open Settings: Start button only (gamepad)
	if hasGamepad {
		result.OpenSettings = inpututil.IsStandardGamepadButtonJustPressed(gamepadID, ebiten.StandardGamepadButtonCenterRight)
	}

	return result
}
