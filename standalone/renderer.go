//go:build !ios && !libretro

package standalone

import (
	"github.com/hajimehoshi/ebiten/v2"
)

// FramebufferRenderer owns the ebiten offscreen buffer and handles
// pixel rendering with scaling. Replaces the emulator-specific
// DrawCachedFramebuffer/GetCachedFramebufferImage methods that were
// previously on the bridge emulator.
type FramebufferRenderer struct {
	screenWidth int
	offscreen   *ebiten.Image
	drawOpts    ebiten.DrawImageOptions
}

// NewFramebufferRenderer creates a renderer for the given native screen width.
func NewFramebufferRenderer(screenWidth int) *FramebufferRenderer {
	return &FramebufferRenderer{
		screenWidth: screenWidth,
	}
}

// DrawFramebuffer renders pixel data to the screen with aspect-ratio-preserving
// scaling.
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

	scaleX := float64(screenW) / nativeW
	scaleY := float64(screenH) / nativeH
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	scaledW := nativeW * scale
	scaledH := nativeH * scale
	offsetX := (float64(screenW) - scaledW) / 2
	offsetY := (float64(screenH) - scaledH) / 2

	r.drawOpts = ebiten.DrawImageOptions{}
	r.drawOpts.GeoM.Scale(scale, scale)
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
