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
	par             float64
	aspectRatioMode string
	offscreen       *ebiten.Image
	scaled          []byte // pixels of a frame downscaled to fit the maximum image size, reused across frames
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
func NewFramebufferRenderer(par float64) *FramebufferRenderer {
	return &FramebufferRenderer{
		par: par,
	}
}

// DrawFramebuffer renders pixel data to the screen with PAR-corrected
// aspect ratio scaling.
func (r *FramebufferRenderer) DrawFramebuffer(screen *ebiten.Image, pixels []byte, stride, activeHeight int) {
	if img := r.frameImage(pixels, stride, activeHeight); img != nil {
		display.DrawScaled(screen, img, r.aspectRatioMode, r.par)
	}
}

// GetFramebufferImage returns pixel data as an ebiten.Image at native
// resolution. Used for shader processing.
func (r *FramebufferRenderer) GetFramebufferImage(pixels []byte, stride, activeHeight int) *ebiten.Image {
	return r.frameImage(pixels, stride, activeHeight)
}

// frameImage writes the frame to the offscreen image and returns it, or
// nil for an empty frame or one with too few pixels. A frame wider or
// taller than the maximum image size cannot be held in an image, so it is
// downscaled to fit first; the picture keeps its aspect ratio since both
// sides are scaled by the same factor.
func (r *FramebufferRenderer) frameImage(pixels []byte, stride, activeHeight int) *ebiten.Image {
	if activeHeight == 0 || stride == 0 {
		return nil
	}

	requiredLen := stride * activeHeight
	if len(pixels) < requiredLen {
		return nil
	}
	pixels = pixels[:requiredLen]

	width, height := stride/4, activeHeight
	if k := downscaleFactor(width, height, ebiten.MaxImageSize()); k > 1 {
		r.scaled, width, height = downscaleBox(r.scaled, pixels, width, height, k)
		pixels = r.scaled
	}

	if r.offscreen == nil || r.offscreen.Bounds().Dx() != width || r.offscreen.Bounds().Dy() != height {
		if r.offscreen != nil {
			r.offscreen.Deallocate()
		}
		r.offscreen = display.NewUnmanagedImage(width, height)
	}

	r.offscreen.WritePixels(pixels)

	return r.offscreen
}

// downscaleFactor returns the smallest factor that brings a width x height
// frame to at most maxSize on both sides, or 1 when it already fits. A
// maxSize of 0, before the size is known, gives 1.
func downscaleFactor(width, height, maxSize int) int {
	if maxSize <= 0 {
		return 1
	}
	return max(1, ceilDiv(width, maxSize), ceilDiv(height, maxSize))
}

// downscaleBox reduces a width x height RGBA frame by k on both sides,
// each output pixel the average of a k x k block. Blocks at the right and
// bottom edges average the pixels the frame has. dst is reused when large
// enough. It returns the output pixels and their width and height.
func downscaleBox(dst, src []byte, width, height, k int) ([]byte, int, int) {
	outW, outH := ceilDiv(width, k), ceilDiv(height, k)
	n := outW * outH * 4
	if cap(dst) < n {
		dst = make([]byte, n)
	}
	dst = dst[:n]

	sums := make([]uint32, outW*4)
	for oy := range outH {
		clear(sums)
		y0, y1 := oy*k, min((oy+1)*k, height)
		for y := y0; y < y1; y++ {
			row := src[y*width*4 : (y+1)*width*4]
			for x := range width {
				s := sums[x/k*4:]
				p := row[x*4:]
				s[0] += uint32(p[0])
				s[1] += uint32(p[1])
				s[2] += uint32(p[2])
				s[3] += uint32(p[3])
			}
		}
		rows := uint32(y1 - y0)
		out := dst[oy*outW*4:]
		for ox := range outW {
			count := rows * uint32(min((ox+1)*k, width)-ox*k)
			for c := range 4 {
				out[ox*4+c] = byte(sums[ox*4+c] / count)
			}
		}
	}
	return dst, outW, outH
}

// ceilDiv returns a / b rounded up, for positive a and b
func ceilDiv(a, b int) int {
	return (a + b - 1) / b
}
