// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/display"
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
}

// SetAspectRatioMode sets the aspect ratio scaling mode ("dar", "4:3", "1:1", "stretch").
func (r *FramebufferRenderer) SetAspectRatioMode(mode string) {
	r.aspectRatioMode = mode
}

// SetPAR updates the pixel aspect ratio used for "dar" scaling. Called
// per frame with the value delivered alongside the framebuffer so cores
// whose PAR changes with video mode render correctly.
func (r *FramebufferRenderer) SetPAR(par float64) {
	if par > 0 {
		r.par = par
	}
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
		if r.offscreen != nil {
			r.offscreen.Deallocate()
		}
		r.offscreen = display.NewUnmanagedImage(pixelWidth, activeHeight)
	}

	r.offscreen.WritePixels(pixels[:requiredLen])

	display.DrawScaled(screen, r.offscreen, r.aspectRatioMode, r.par)
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
		if r.offscreen != nil {
			r.offscreen.Deallocate()
		}
		r.offscreen = display.NewUnmanagedImage(pixelWidth, activeHeight)
	}

	r.offscreen.WritePixels(pixels[:requiredLen])

	return r.offscreen
}
