package display

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/coreif"
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
	case "1:1":
		dar := coreif.DisplayAspectRatio(sourceW, sourceH, 1.0)
		displayW := float64(screenW)
		displayH := displayW / dar
		if displayH > float64(screenH) {
			displayH = float64(screenH)
			displayW = displayH * dar
		}
		return displayW, displayH
	default: // "dar" or unset
		dar := coreif.DisplayAspectRatio(sourceW, sourceH, par)
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

// DrawScaled draws src into dst, scaled and centered to fit dst using the
// given aspect ratio mode and pixel aspect ratio. Nearest filtering preserves
// pixel-art crispness. src's bounds determine the source size, so a cropped
// sub-image scales by its cropped dimensions.
func DrawScaled(dst, src *ebiten.Image, mode string, par float64) {
	if src == nil {
		return
	}
	b := src.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	if srcW == 0 || srcH == 0 {
		return
	}

	screenW, screenH := dst.Bounds().Dx(), dst.Bounds().Dy()
	displayW, displayH := Size(mode, screenW, screenH, srcW, srcH, par)
	scaleX, scaleY, offsetX, offsetY := ScaleAndCenter(displayW, displayH, float64(srcW), float64(srcH), screenW, screenH)

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scaleX, scaleY)
	op.GeoM.Translate(offsetX, offsetY)
	op.Filter = ebiten.FilterNearest
	dst.DrawImage(src, op)
}

// DPIScale returns the device scale factor for the current monitor.
// Returns 1.0 if the monitor is not available (e.g. in test environments).
func DPIScale() float64 {
	if m := ebiten.Monitor(); m != nil {
		return m.DeviceScaleFactor()
	}
	return 1.0
}
