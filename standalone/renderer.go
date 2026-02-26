//go:build !ios && !libretro

package standalone

import (
	"github.com/hajimehoshi/ebiten/v2"
	emucore "github.com/user-none/eblitui/api"
)

// FramebufferRenderer owns the ebiten offscreen buffer and handles
// pixel rendering with scaling. Replaces the emulator-specific
// DrawCachedFramebuffer/GetCachedFramebufferImage methods that were
// previously on the bridge emulator.
type FramebufferRenderer struct {
	screenWidth int
	par         float64
	offscreen   *ebiten.Image
	drawOpts    ebiten.DrawImageOptions
}

// NewFramebufferRenderer creates a renderer for the given native screen width
// and pixel aspect ratio.
func NewFramebufferRenderer(screenWidth int, par float64) *FramebufferRenderer {
	return &FramebufferRenderer{
		screenWidth: screenWidth,
		par:         par,
	}
}

// DrawFramebuffer renders pixel data to the screen with PAR-corrected
// aspect ratio scaling.
func (r *FramebufferRenderer) DrawFramebuffer(screen *ebiten.Image, pixels []byte, stride, activeHeight int) {
	if activeHeight == 0 || stride == 0 {
		return
	}

	requiredLen := stride * activeHeight
	if len(pixels) < requiredLen {
		return
	}

	pixelWidth := stride / 4
	if r.offscreen == nil || r.offscreen.Bounds().Dx() != pixelWidth || r.offscreen.Bounds().Dy() != activeHeight {
		r.offscreen = ebiten.NewImage(pixelWidth, activeHeight)
	}

	r.offscreen.WritePixels(pixels[:requiredLen])

	screenW, screenH := screen.Bounds().Dx(), screen.Bounds().Dy()
	nativeW := float64(pixelWidth)
	nativeH := float64(activeHeight)

	// Compute DAR dynamically from frame dimensions and PAR.
	dar := emucore.DisplayAspectRatio(pixelWidth, activeHeight, r.par)

	// Fit to screen while preserving the display aspect ratio.
	displayW := float64(screenW)
	displayH := displayW / dar
	if displayH > float64(screenH) {
		displayH = float64(screenH)
		displayW = displayH * dar
	}

	scaleX := displayW / nativeW
	scaleY := displayH / nativeH
	scaledW := nativeW * scaleX
	scaledH := nativeH * scaleY
	offsetX := (float64(screenW) - scaledW) / 2
	offsetY := (float64(screenH) - scaledH) / 2

	r.drawOpts = ebiten.DrawImageOptions{}
	r.drawOpts.GeoM.Scale(scaleX, scaleY)
	r.drawOpts.GeoM.Translate(offsetX, offsetY)
	r.drawOpts.Filter = ebiten.FilterNearest
	screen.DrawImage(r.offscreen, &r.drawOpts)
}

// GetFramebufferImage returns pixel data as an ebiten.Image at native
// resolution. Used for shader processing.
func (r *FramebufferRenderer) GetFramebufferImage(pixels []byte, stride, activeHeight int) *ebiten.Image {
	if activeHeight == 0 || stride == 0 {
		return nil
	}

	requiredLen := stride * activeHeight
	if len(pixels) < requiredLen {
		return nil
	}

	pixelWidth := stride / 4
	if r.offscreen == nil || r.offscreen.Bounds().Dx() != pixelWidth || r.offscreen.Bounds().Dy() != activeHeight {
		r.offscreen = ebiten.NewImage(pixelWidth, activeHeight)
	}

	r.offscreen.WritePixels(pixels[:requiredLen])

	return r.offscreen
}
