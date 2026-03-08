package standalone

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/standalone/display"
)

// FramebufferRenderer owns the ebiten offscreen buffer and handles
// pixel rendering with scaling. Replaces the emulator-specific
// DrawCachedFramebuffer/GetCachedFramebufferImage methods that were
// previously on the bridge emulator.
type FramebufferRenderer struct {
	screenWidth     int
	par             float64
	aspectRatioMode string
	offscreen       *ebiten.Image
	drawOpts        ebiten.DrawImageOptions
}

// SetAspectRatioMode sets the aspect ratio scaling mode ("dar", "4:3", "1:1", "stretch").
func (r *FramebufferRenderer) SetAspectRatioMode(mode string) {
	r.aspectRatioMode = mode
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

	displayW, displayH := display.Size(r.aspectRatioMode, screenW, screenH, pixelWidth, activeHeight, r.par)
	scaleX, scaleY, offsetX, offsetY := display.ScaleAndCenter(displayW, displayH, nativeW, nativeH, screenW, screenH)

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
