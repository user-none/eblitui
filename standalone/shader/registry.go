//go:build !libretro

package shader

// EffectContext describes where an effect can be applied
type EffectContext int

const (
	ContextGame EffectContext = 1 << iota
	ContextUI
	ContextAll = ContextGame | ContextUI
)

// ShaderInfo describes an available shader effect
type ShaderInfo struct {
	ID          string        // Unique identifier used in config
	Name        string        // Display name for UI
	Description string        // Brief description of the effect
	Weight      int           // Higher weight = applied earlier in chain
	Preprocess  bool          // True for preprocessing effects (xBR, ghosting)
	Context     EffectContext // Where this effect can be applied
}

// AvailableShaders lists all shaders that can be enabled
var AvailableShaders = []ShaderInfo{
	{
		ID:          "xbr",
		Name:        "Pixel Smoothing (xBR)",
		Description: "Smooth edges while preserving pixel art details",
		Preprocess:  true,
		Context:     ContextGame,
	},
	{
		ID:          "ghosting",
		Name:        "Phosphor Persistence",
		Description: "Ghost trails from slow CRT phosphor decay",
		Preprocess:  true,
		Context:     ContextGame,
	},
	{
		ID:          "crt",
		Name:        "CRT",
		Description: "Curved screen with RGB separation and vignette",
		Weight:      25,
		Context:     ContextAll,
	},
	{
		ID:          "scanlines",
		Name:        "Scanlines",
		Description: "Horizontal scanline effect",
		Weight:      400,
		Context:     ContextAll,
	},
	{
		ID:          "bloom",
		Name:        "Phosphor Glow",
		Description: "Bright pixels glow into neighbors like CRT phosphors",
		Weight:      550,
		Context:     ContextAll,
	},
	{
		ID:          "lcd",
		Name:        "LCD Grid",
		Description: "Visible pixel grid with RGB subpixels like handhelds",
		Weight:      300,
		Context:     ContextAll,
	},
	{
		ID:          "colorbleed",
		Name:        "Color Bleed",
		Description: "Horizontal color bleeding from composite video",
		Weight:      800,
		Context:     ContextAll,
	},
	{
		ID:          "dotmatrix",
		Name:        "Dot Matrix",
		Description: "Circular pixels like CRT phosphor dots",
		Weight:      350,
		Context:     ContextAll,
	},
	{
		ID:          "ntsc",
		Name:        "NTSC Artifacts",
		Description: "Color fringing at edges from NTSC encoding",
		Weight:      850,
		Context:     ContextAll,
	},
	{
		ID:          "rainbow",
		Name:        "NTSC Rainbow",
		Description: "Rainbow artifacts from dithering on composite video",
		Weight:      845,
		Context:     ContextAll,
	},
	{
		ID:          "gamma",
		Name:        "CRT Gamma",
		Description: "Non-linear brightness curve of CRT displays",
		Weight:      900,
		Context:     ContextAll,
	},
	{
		ID:          "halation",
		Name:        "Halation",
		Description: "Light bleeding behind CRT glass",
		Weight:      500,
		Context:     ContextAll,
	},
	{
		ID:          "rfnoise",
		Name:        "RF Noise",
		Description: "Subtle static grain from RF connection",
		Weight:      50,
		Context:     ContextAll,
	},
	{
		ID:          "rollingband",
		Name:        "Rolling Band",
		Description: "Scrolling dark band for bad reception look",
		Weight:      80,
		Context:     ContextAll,
	},
	{
		ID:          "vhs",
		Name:        "VHS Distortion",
		Description: "Wobble and tracking artifacts like VHS tape",
		Weight:      100,
		Context:     ContextAll,
	},
	{
		ID:          "interlace",
		Name:        "Interlace",
		Description: "Alternating scanline fields for 480i look",
		Weight:      380,
		Context:     ContextAll,
	},
	{
		ID:          "hsoft",
		Name:        "Horizontal Softness",
		Description: "Bandwidth-limited horizontal blur like analog video",
		Weight:      770,
		Context:     ContextAll,
	},
	{
		ID:          "vblur",
		Name:        "Vertical Blur",
		Description: "Electron beam softness causing scanline bleed",
		Weight:      760,
		Context:     ContextAll,
	},
	{
		ID:          "monochrome",
		Name:        "Monochrome",
		Description: "Black and white conversion",
		Weight:      700,
		Context:     ContextAll,
	},
	{
		ID:          "sepia",
		Name:        "Sepia",
		Description: "Warm brownish tint like old photographs",
		Weight:      650,
		Context:     ContextAll,
	},
}

// shaderWeights provides O(1) weight lookup by shader ID
var shaderWeights map[string]int

// shaderPreprocess provides O(1) preprocess lookup by shader ID
var shaderPreprocess map[string]bool

// shaderContexts provides O(1) context lookup by shader ID
var shaderContexts map[string]EffectContext

func init() {
	shaderWeights = make(map[string]int)
	shaderPreprocess = make(map[string]bool)
	shaderContexts = make(map[string]EffectContext)
	for _, s := range AvailableShaders {
		shaderWeights[s.ID] = s.Weight
		shaderPreprocess[s.ID] = s.Preprocess
		shaderContexts[s.ID] = s.Context
	}
}

// GetShaderWeight returns the weight for a shader ID (0 if unknown)
func GetShaderWeight(id string) int {
	return shaderWeights[id]
}

// IsPreprocess returns true if the shader ID is a preprocessing effect
func IsPreprocess(id string) bool {
	return shaderPreprocess[id]
}

// GetShaderContext returns the context for a shader ID (0 if unknown)
func GetShaderContext(id string) EffectContext {
	return shaderContexts[id]
}
