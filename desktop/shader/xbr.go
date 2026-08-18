// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package shader

import (
	_ "embed"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/display"
)

//go:embed shaders/xbr.kage
var xbrShaderSrc []byte

// XBRScaler handles xBR pixel art scaling with cascaded 2x passes.
// Supports 2x (1 pass), 4x (2 passes), and 8x (3 passes) scaling.
// Buffers are pooled and reused to avoid per-frame GPU allocations.
type XBRScaler struct {
	shader          *ebiten.Shader // Cached compiled shader
	par             float64        // Pixel aspect ratio
	aspectRatioMode string         // "dar", "4:3", "1:1", "stretch"

	// Pooled buffers (reused when dimensions match)
	normalizedSrc *ebiten.Image
	passBuffers   [3]*ebiten.Image // Max 3 passes for 8x
	screenBuffer  *ebiten.Image
}

// SetAspectRatioMode sets the aspect ratio scaling mode ("dar", "4:3", "1:1", "stretch").
func (x *XBRScaler) SetAspectRatioMode(mode string) {
	x.aspectRatioMode = mode
}

// NewXBRScaler creates a new xBR scaler instance with the given
// pixel aspect ratio.
func NewXBRScaler(par float64) *XBRScaler {
	return &XBRScaler{
		par: par,
	}
}

// Apply runs xBR scaling on the source and returns a screen-sized image.
// Automatically selects 2x, 4x, or 8x scaling based on screen size. par is the
// pixel aspect ratio delivered with this frame, used for the final scale.
func (x *XBRScaler) Apply(src *ebiten.Image, par float64, screenW, screenH int) *ebiten.Image {
	if src == nil {
		return nil
	}
	if par > 0 {
		x.par = par
	}

	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	// Ensure shader is compiled
	if err := x.ensureShader(); err != nil {
		return x.scaleToScreen(src, screenW, screenH)
	}

	// Determine number of passes needed
	scaleFactor := selectOptimalScale(srcW, srcH, screenW, screenH)
	passes := scaleFactorToPasses(scaleFactor)

	// Ensure all buffers are ready (creates or clears as needed)
	x.ensureBufferPool(srcW, srcH, screenW, screenH)

	// Copy SubImage to regular image at (0,0) to fix coordinate issues
	// SubImages have non-zero bounds that break DrawTrianglesShader srcPos interpolation
	x.normalizedSrc.DrawImage(src, nil)

	// Execute cascade passes
	currentInput := x.normalizedSrc
	for pass := 0; pass < passes; pass++ {
		x.runShaderPass(currentInput, x.passBuffers[pass])
		currentInput = x.passBuffers[pass]
	}

	// Scale final xBR output to the pooled screen buffer with centering
	display.DrawScaled(x.screenBuffer, currentInput, x.aspectRatioMode, x.par)

	return x.screenBuffer
}

// ensureBufferPool ensures all pooled buffers are ready for the given dimensions.
// Creates new buffers if dimensions changed, otherwise clears existing ones.
func (x *XBRScaler) ensureBufferPool(srcW, srcH, screenW, screenH int) {
	// Check if source dimensions changed
	srcChanged := x.normalizedSrc == nil ||
		x.normalizedSrc.Bounds().Dx() != srcW ||
		x.normalizedSrc.Bounds().Dy() != srcH

	if srcChanged {
		// Deallocate old source-derived buffers
		if x.normalizedSrc != nil {
			x.normalizedSrc.Deallocate()
		}
		for i := range x.passBuffers {
			if x.passBuffers[i] != nil {
				x.passBuffers[i].Deallocate()
				x.passBuffers[i] = nil
			}
		}

		// Create all pass buffers
		x.normalizedSrc = display.NewUnmanagedImage(srcW, srcH)
		w, h := srcW, srcH
		for i := range x.passBuffers {
			w, h = w*2, h*2
			x.passBuffers[i] = display.NewUnmanagedImage(w, h)
		}
	} else {
		// Clear existing buffers for reuse
		x.normalizedSrc.Clear()
		for i := range x.passBuffers {
			x.passBuffers[i].Clear()
		}
	}

	// Handle screen buffer separately (depends on window size, not source)
	screenChanged := x.screenBuffer == nil ||
		x.screenBuffer.Bounds().Dx() != screenW ||
		x.screenBuffer.Bounds().Dy() != screenH

	if screenChanged {
		if x.screenBuffer != nil {
			x.screenBuffer.Deallocate()
		}
		x.screenBuffer = display.NewUnmanagedImage(screenW, screenH)
	} else {
		x.screenBuffer.Clear()
	}
}

// ensureShader compiles and caches the xBR shader
func (x *XBRScaler) ensureShader() error {
	if x.shader != nil {
		return nil
	}
	shader, err := ebiten.NewShader(xbrShaderSrc)
	if err != nil {
		return err
	}
	x.shader = shader
	return nil
}

// selectOptimalScale chooses 2, 4, or 8 based on how much scaling is needed to fit screen
func selectOptimalScale(srcW, srcH, screenW, screenH int) int {
	// Calculate aspect-ratio-preserving scale factor to fit screen
	scaleX := float64(screenW) / float64(srcW)
	scaleY := float64(screenH) / float64(srcH)
	scaleToFit := scaleX
	if scaleY < scaleX {
		scaleToFit = scaleY
	}

	// Choose smallest xBR scale that covers the target (prefer downscaling xBR output)
	if scaleToFit <= 2.0 {
		return 2
	} else if scaleToFit <= 4.0 {
		return 4
	}
	return 8
}

// scaleFactorToPasses converts scale factor to number of 2x passes
func scaleFactorToPasses(factor int) int {
	switch factor {
	case 4:
		return 2
	case 8:
		return 3
	default:
		return 1
	}
}

// runShaderPass executes one 2x xBR pass from input to output
func (x *XBRScaler) runShaderPass(input, output *ebiten.Image) {
	inW := input.Bounds().Dx()
	inH := input.Bounds().Dy()
	outW := output.Bounds().Dx()
	outH := output.Bounds().Dy()

	vertices := []ebiten.Vertex{
		{DstX: 0, DstY: 0, SrcX: 0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: float32(outW), DstY: 0, SrcX: float32(inW), SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 0, DstY: float32(outH), SrcX: 0, SrcY: float32(inH), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: float32(outW), DstY: float32(outH), SrcX: float32(inW), SrcY: float32(inH), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	indices := []uint16{0, 1, 2, 1, 3, 2}

	op := &ebiten.DrawTrianglesShaderOptions{}
	op.Images[0] = input

	output.DrawTrianglesShader(vertices, indices, x.shader, op)
}

// scaleToScreen scales src into a new screen-sized image using the configured
// display aspect ratio, centered. Used as fallback when the shader fails.
func (x *XBRScaler) scaleToScreen(src *ebiten.Image, screenW, screenH int) *ebiten.Image {
	screenBuffer := display.NewUnmanagedImage(screenW, screenH)
	display.DrawScaled(screenBuffer, src, x.aspectRatioMode, x.par)
	return screenBuffer
}
