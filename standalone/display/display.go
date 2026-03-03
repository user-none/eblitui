package display

import (
	"github.com/hajimehoshi/ebiten/v2"
	emucore "github.com/user-none/eblitui/api"
)

// Size computes the display dimensions for the given aspect ratio mode,
// fitting within screenW x screenH while preserving the chosen ratio.
// sourceW/sourceH are the native pixel dimensions and par is the pixel aspect
// ratio (used only for "dar" mode).
func Size(mode string, screenW, screenH, sourceW, sourceH int, par float64) (float64, float64) {
	switch mode {
	case "stretch":
		return float64(screenW), float64(screenH)
	case "4:3":
		ratio := 4.0 / 3.0
		displayW := float64(screenW)
		displayH := displayW / ratio
		if displayH > float64(screenH) {
			displayH = float64(screenH)
			displayW = displayH * ratio
		}
		return displayW, displayH
	default: // "dar" or unset
		dar := emucore.DisplayAspectRatio(sourceW, sourceH, par)
		displayW := float64(screenW)
		displayH := displayW / dar
		if displayH > float64(screenH) {
			displayH = float64(screenH)
			displayW = displayH * dar
		}
		return displayW, displayH
	}
}

// ScaleAndCenter computes scale factors and centering offsets to fit a
// display-sized image (displayW x displayH) from a source (sourceW x sourceH)
// into the screen (screenW x screenH).
func ScaleAndCenter(displayW, displayH, sourceW, sourceH float64, screenW, screenH int) (scaleX, scaleY, offsetX, offsetY float64) {
	scaleX = displayW / sourceW
	scaleY = displayH / sourceH
	scaledW := sourceW * scaleX
	scaledH := sourceH * scaleY
	offsetX = (float64(screenW) - scaledW) / 2
	offsetY = (float64(screenH) - scaledH) / 2
	return
}

// DPIScale returns the device scale factor for the current monitor.
// Returns 1.0 if the monitor is not available (e.g. in test environments).
func DPIScale() float64 {
	if m := ebiten.Monitor(); m != nil {
		return m.DeviceScaleFactor()
	}
	return 1.0
}
