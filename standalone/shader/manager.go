//go:build !libretro

package shader

import (
	_ "embed"
	"fmt"
	"log"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed shaders/crt.kage
var crtShaderSrc []byte

//go:embed shaders/scanlines.kage
var scanlinesShaderSrc []byte

//go:embed shaders/bloom.kage
var bloomShaderSrc []byte

//go:embed shaders/lcd.kage
var lcdShaderSrc []byte

//go:embed shaders/colorbleed.kage
var colorbleedShaderSrc []byte

//go:embed shaders/dotmatrix.kage
var dotmatrixShaderSrc []byte

//go:embed shaders/ntsc.kage
var ntscShaderSrc []byte

//go:embed shaders/gamma.kage
var gammaShaderSrc []byte

//go:embed shaders/halation.kage
var halationShaderSrc []byte

//go:embed shaders/rfnoise.kage
var rfnoiseShaderSrc []byte

//go:embed shaders/rollingband.kage
var rollingbandShaderSrc []byte

//go:embed shaders/vhs.kage
var vhsShaderSrc []byte

//go:embed shaders/interlace.kage
var interlaceShaderSrc []byte

//go:embed shaders/monochrome.kage
var monochromeShaderSrc []byte

//go:embed shaders/sepia.kage
var sepiaShaderSrc []byte

//go:embed shaders/vblur.kage
var vblurShaderSrc []byte

//go:embed shaders/hsoft.kage
var hsoftShaderSrc []byte

//go:embed shaders/rainbow.kage
var rainbowShaderSrc []byte

// shaderSources maps shader IDs to their Kage source code
var shaderSources = map[string][]byte{
	"crt":         crtShaderSrc,
	"scanlines":   scanlinesShaderSrc,
	"bloom":       bloomShaderSrc,
	"lcd":         lcdShaderSrc,
	"colorbleed":  colorbleedShaderSrc,
	"dotmatrix":   dotmatrixShaderSrc,
	"ntsc":        ntscShaderSrc,
	"gamma":       gammaShaderSrc,
	"halation":    halationShaderSrc,
	"rfnoise":     rfnoiseShaderSrc,
	"rollingband": rollingbandShaderSrc,
	"vhs":         vhsShaderSrc,
	"interlace":   interlaceShaderSrc,
	"monochrome":  monochromeShaderSrc,
	"sepia":       sepiaShaderSrc,
	"vblur":       vblurShaderSrc,
	"hsoft":       hsoftShaderSrc,
	"rainbow":     rainbowShaderSrc,
}

// Manager handles shader compilation, caching, and application
type Manager struct {
	// Compiled shader cache
	shaders map[string]*ebiten.Shader

	// Intermediate buffers for shader chaining (ping-pong)
	bufferA *ebiten.Image
	bufferB *ebiten.Image

	// Ghosting buffer for phosphor persistence (persistent across frames)
	ghostingBuffer *ebiten.Image

	// xBR scaler for pixel art scaling
	xbrScaler *XBRScaler

	// Frame counter for animated shaders
	frame int

	// Cached shader pipeline (rebuilt only when config changes)
	cachedShaderIDs     []string
	cachedSortedShaders []*ebiten.Shader
}

// NewManager creates a new shader manager with the given pixel aspect ratio
// (used by xBR scaling to compute display aspect ratio per frame).
func NewManager(par float64) *Manager {
	return &Manager{
		shaders:   make(map[string]*ebiten.Shader),
		xbrScaler: NewXBRScaler(par),
	}
}

// ResetBuffers clears all effect buffers. Call when switching games.
func (m *Manager) ResetBuffers() {
	if m.ghostingBuffer != nil {
		m.ghostingBuffer.Deallocate()
		m.ghostingBuffer = nil
	}
	if m.bufferA != nil {
		m.bufferA.Deallocate()
		m.bufferA = nil
	}
	if m.bufferB != nil {
		m.bufferB.Deallocate()
		m.bufferB = nil
	}
}

// IncrementFrame advances the frame counter for animated shaders
func (m *Manager) IncrementFrame() {
	m.frame++
}

// Frame returns the current frame count
func (m *Manager) Frame() int {
	return m.frame
}

// LoadShader compiles and caches a shader by ID
func (m *Manager) LoadShader(id string) error {
	// Already loaded?
	if _, ok := m.shaders[id]; ok {
		return nil
	}

	// Get source
	src, ok := shaderSources[id]
	if !ok {
		return fmt.Errorf("unknown shader: %s", id)
	}

	// Compile
	shader, err := ebiten.NewShader(src)
	if err != nil {
		return fmt.Errorf("failed to compile shader %s: %w", id, err)
	}

	m.shaders[id] = shader
	return nil
}

// PreloadShaders loads all shaders in the given list
func (m *Manager) PreloadShaders(ids []string) {
	for _, id := range ids {
		if IsPreprocess(id) {
			continue
		}
		if err := m.LoadShader(id); err != nil {
			log.Printf("Warning: failed to load shader %s: %v", id, err)
		}
	}
}

// ensureGhostingBuffer creates or resizes the ghosting buffer to match dimensions
func (m *Manager) ensureGhostingBuffer(width, height int) {
	if m.ghostingBuffer != nil {
		bw, bh := m.ghostingBuffer.Bounds().Dx(), m.ghostingBuffer.Bounds().Dy()
		if bw != width || bh != height {
			m.ghostingBuffer.Deallocate()
			m.ghostingBuffer = nil
		}
	}
	if m.ghostingBuffer == nil {
		m.ghostingBuffer = ebiten.NewImage(width, height)
	}
}

// ensureBuffers creates or resizes the ping-pong buffers to match dimensions
func (m *Manager) ensureBuffers(width, height int) {
	// Check if bufferA needs (re)creation
	if m.bufferA != nil {
		bw, bh := m.bufferA.Bounds().Dx(), m.bufferA.Bounds().Dy()
		if bw != width || bh != height {
			m.bufferA.Deallocate()
			m.bufferA = nil
		}
	}
	if m.bufferA == nil {
		m.bufferA = ebiten.NewImage(width, height)
	}

	// Check if bufferB needs (re)creation
	if m.bufferB != nil {
		bw, bh := m.bufferB.Bounds().Dx(), m.bufferB.Bounds().Dy()
		if bw != width || bh != height {
			m.bufferB.Deallocate()
			m.bufferB = nil
		}
	}
	if m.bufferB == nil {
		m.bufferB = ebiten.NewImage(width, height)
	}
}

// shaderListMatches checks if the given shader IDs match the cached list
func (m *Manager) shaderListMatches(shaderIDs []string) bool {
	if len(shaderIDs) != len(m.cachedShaderIDs) {
		return false
	}
	for i, id := range shaderIDs {
		if m.cachedShaderIDs[i] != id {
			return false
		}
	}
	return true
}

// rebuildShaderCache loads, sorts, and filters shaders, caching the result.
// Preprocessing effects are skipped since they are handled by ApplyPreprocessEffects.
func (m *Manager) rebuildShaderCache(shaderIDs []string) {
	// Load any missing non-preprocess shaders
	for _, id := range shaderIDs {
		if IsPreprocess(id) {
			continue
		}
		if _, ok := m.shaders[id]; !ok {
			if err := m.LoadShader(id); err != nil {
				log.Printf("Warning: shader %s not available: %v", id, err)
			}
		}
	}

	// Cache the full input list for change detection
	m.cachedShaderIDs = make([]string, len(shaderIDs))
	copy(m.cachedShaderIDs, shaderIDs)

	// Filter out preprocessing effects, then sort by weight (descending), ID (ascending)
	filtered := make([]string, 0, len(shaderIDs))
	for _, id := range shaderIDs {
		if !IsPreprocess(id) {
			filtered = append(filtered, id)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		wi, wj := GetShaderWeight(filtered[i]), GetShaderWeight(filtered[j])
		if wi != wj {
			return wi > wj
		}
		return filtered[i] < filtered[j]
	})

	// Keep only shaders that compiled successfully
	m.cachedSortedShaders = make([]*ebiten.Shader, 0, len(filtered))
	for _, id := range filtered {
		if s, ok := m.shaders[id]; ok {
			m.cachedSortedShaders = append(m.cachedSortedShaders, s)
		}
	}
}

// applyGhosting applies the ghosting pre-processing step.
// It updates the ghosting buffer and returns a ghosted image.
func (m *Manager) applyGhosting(src *ebiten.Image) *ebiten.Image {
	srcW, srcH := src.Bounds().Dx(), src.Bounds().Dy()
	m.ensureGhostingBuffer(srcW, srcH)
	m.ensureBuffers(srcW, srcH)

	// Update ghosting buffer: buffer = buffer * 0.6 + src * 0.4
	// Step 1: Copy ghostingBuffer at 60% to bufferA
	m.bufferA.Clear()
	decayOp := &ebiten.DrawImageOptions{}
	decayOp.ColorScale.Scale(0.6, 0.6, 0.6, 1.0)
	m.bufferA.DrawImage(m.ghostingBuffer, decayOp)

	// Step 2: Add current at 40% to bufferA (additive blend)
	addOp := &ebiten.DrawImageOptions{}
	addOp.ColorScale.Scale(0.4, 0.4, 0.4, 1.0)
	addOp.Blend = ebiten.Blend{
		BlendFactorSourceRGB:        ebiten.BlendFactorOne,
		BlendFactorSourceAlpha:      ebiten.BlendFactorOne,
		BlendFactorDestinationRGB:   ebiten.BlendFactorOne,
		BlendFactorDestinationAlpha: ebiten.BlendFactorOne,
		BlendOperationRGB:           ebiten.BlendOperationAdd,
		BlendOperationAlpha:         ebiten.BlendOperationAdd,
	}
	m.bufferA.DrawImage(src, addOp)

	// Step 3: Copy bufferA to ghostingBuffer for next frame
	m.ghostingBuffer.Clear()
	m.ghostingBuffer.DrawImage(m.bufferA, nil)

	// Return the blended result (bufferA already contains it)
	return m.bufferA
}

// hasGhosting returns true if "ghosting" is in the shader list
func hasGhosting(shaderIDs []string) bool {
	for _, id := range shaderIDs {
		if id == "ghosting" {
			return true
		}
	}
	return false
}

// HasXBR returns true if "xbr" is in the shader list (exported for app.go)
func HasXBR(shaderIDs []string) bool {
	for _, id := range shaderIDs {
		if id == "xbr" {
			return true
		}
	}
	return false
}

// ApplyPreprocessEffects applies xBR and ghosting effects (not Kage shaders).
// If xBR is in shaderIDs, src should be native resolution and will be scaled to screen size.
// Otherwise, src should already be screen resolution.
// Returns the processed image (screen-sized).
func (m *Manager) ApplyPreprocessEffects(src *ebiten.Image, shaderIDs []string, screenW, screenH int) *ebiten.Image {
	if src == nil {
		return nil
	}

	effectiveInput := src

	// Handle xBR first (scales native -> screen size with smoothing)
	if HasXBR(shaderIDs) {
		effectiveInput = m.xbrScaler.Apply(src, screenW, screenH)
	}

	// Handle ghosting second (operates at screen size)
	if hasGhosting(shaderIDs) {
		effectiveInput = m.applyGhosting(effectiveInput)
	}

	return effectiveInput
}

// ApplyShaders draws src to dst with the specified shader chain applied.
// If shaderIDs is empty, src is drawn directly to dst.
// sourceHeight is the native vertical resolution of the emulated system
// (used by scanline shader to align with original pixel rows).
// Note: Preprocessing effects (xBR, ghosting) should be applied via
// ApplyPreprocessEffects before calling this function.
// Returns true if shaders were applied, false if direct draw was used.
func (m *Manager) ApplyShaders(dst, src *ebiten.Image, shaderIDs []string, sourceHeight int) bool {
	if src == nil {
		return false
	}
	if len(shaderIDs) == 0 {
		dst.DrawImage(src, nil)
		return false
	}

	// Rebuild cache only when shader list changes
	if !m.shaderListMatches(shaderIDs) {
		m.rebuildShaderCache(shaderIDs)
	}

	validShaders := m.cachedSortedShaders
	if len(validShaders) == 0 {
		dst.DrawImage(src, nil)
		return false
	}

	srcW, srcH := src.Bounds().Dx(), src.Bounds().Dy()

	// Uniforms for shaders
	uniforms := map[string]interface{}{
		"Time":         float32(m.frame),
		"SourceHeight": float32(sourceHeight),
	}

	// Single shader case - draw directly to destination
	if len(validShaders) == 1 {
		op := &ebiten.DrawRectShaderOptions{}
		op.Images[0] = src
		op.Uniforms = uniforms
		dst.DrawRectShader(srcW, srcH, validShaders[0], op)
		return true
	}

	// Multiple shaders - chain through ping-pong buffers
	m.ensureBuffers(srcW, srcH)

	// Track current input for each pass
	currentInput := src
	buffers := [2]*ebiten.Image{m.bufferA, m.bufferB}
	bufferIndex := 1

	for i, shader := range validShaders {
		op := &ebiten.DrawRectShaderOptions{}
		op.Images[0] = currentInput
		op.Uniforms = uniforms

		if i == len(validShaders)-1 {
			// Last shader writes to destination
			dst.DrawRectShader(srcW, srcH, shader, op)
		} else {
			// Intermediate shaders write to ping-pong buffer
			outputBuffer := buffers[bufferIndex%2]
			outputBuffer.Clear()
			outputBuffer.DrawRectShader(srcW, srcH, shader, op)
			currentInput = outputBuffer
			bufferIndex++
		}
	}

	return true
}
