//go:build !libretro

package shader

import (
	_ "embed"

	"github.com/hajimehoshi/ebiten/v2"
	emucore "github.com/user-none/eblitui/api"
)

//go:embed shaders/xbr.kage
var xbrShaderSrc []byte

// XBRScaler handles xBR pixel art scaling with cascaded 2x passes.
// Supports 2x (1 pass), 4x (2 passes), and 8x (3 passes) scaling.
// Buffers are pooled and reused to avoid per-frame GPU allocations.
type XBRScaler struct {
	shader *ebiten.Shader // Cached compiled shader
	par    float64        // Pixel aspect ratio

	// Pooled buffers (reused when dimensions match)
	normalizedSrc *ebiten.Image
	passBuffers   [3]*ebiten.Image // Max 3 passes for 8x
	screenBuffer  *ebiten.Image
}

// NewXBRScaler creates a new xBR scaler instance with the given
// pixel aspect ratio.
func NewXBRScaler(par float64) *XBRScaler {
	return &XBRScaler{
		par: par,
	}
}

// Apply runs xBR scaling on the source and returns a screen-sized image.
// Automatically selects 2x, 4x, or 8x scaling based on screen size.
func (x *XBRScaler) Apply(src *ebiten.Image, screenW, screenH int) *ebiten.Image {
	if src == nil {
		return nil
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

	// Scale final xBR output to screen with centering
	x.drawToScreenBuffer(currentInput, screenW, screenH)

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
		x.normalizedSrc = ebiten.NewImage(srcW, srcH)
		w, h := srcW, srcH
		for i := range x.passBuffers {
			w, h = w*2, h*2
			x.passBuffers[i] = ebiten.NewImage(w, h)
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
		x.screenBuffer = ebiten.NewImage(screenW, screenH)
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

// scaleToScreen scales src to fit screen using the dynamically computed
// display aspect ratio, centered. Used as fallback when shader fails.
func (x *XBRScaler) scaleToScreen(src *ebiten.Image, screenW, screenH int) *ebiten.Image {
	srcW := float64(src.Bounds().Dx())
	srcH := float64(src.Bounds().Dy())

	// Compute DAR dynamically from source dimensions and PAR.
	dar := emucore.DisplayAspectRatio(int(srcW), int(srcH), x.par)

	displayW := float64(screenW)
	displayH := displayW / dar
	if displayH > float64(screenH) {
		displayH = float64(screenH)
		displayW = displayH * dar
	}

	scaleX := displayW / srcW
	scaleY := displayH / srcH
	scaledW := srcW * scaleX
	scaledH := srcH * scaleY
	offsetX := (float64(screenW) - scaledW) / 2
	offsetY := (float64(screenH) - scaledH) / 2

	screenBuffer := ebiten.NewImage(screenW, screenH)
	drawOp := &ebiten.DrawImageOptions{}
	drawOp.GeoM.Scale(scaleX, scaleY)
	drawOp.GeoM.Translate(offsetX, offsetY)
	drawOp.Filter = ebiten.FilterNearest
	screenBuffer.DrawImage(src, drawOp)

	return screenBuffer
}

// drawToScreenBuffer scales src to the pooled screen buffer using the
// dynamically computed display aspect ratio, centered in the screen area.
func (x *XBRScaler) drawToScreenBuffer(src *ebiten.Image, screenW, screenH int) {
	srcW := float64(src.Bounds().Dx())
	srcH := float64(src.Bounds().Dy())

	// Compute DAR dynamically from source dimensions and PAR.
	dar := emucore.DisplayAspectRatio(int(srcW), int(srcH), x.par)

	displayW := float64(screenW)
	displayH := displayW / dar
	if displayH > float64(screenH) {
		displayH = float64(screenH)
		displayW = displayH * dar
	}

	scaleX := displayW / srcW
	scaleY := displayH / srcH
	scaledW := srcW * scaleX
	scaledH := srcH * scaleY
	offsetX := (float64(screenW) - scaledW) / 2
	offsetY := (float64(screenH) - scaledH) / 2

	drawOp := &ebiten.DrawImageOptions{}
	drawOp.GeoM.Scale(scaleX, scaleY)
	drawOp.GeoM.Translate(offsetX, offsetY)
	drawOp.Filter = ebiten.FilterNearest
	x.screenBuffer.DrawImage(src, drawOp)
}
