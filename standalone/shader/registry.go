//go:build !libretro

package shader

// ShaderInfo describes an available shader effect
type ShaderInfo struct {
	ID          string // Unique identifier used in config
	Name        string // Display name for UI
	Description string // Brief description of the effect
	Weight      int    // Higher weight = applied earlier in chain
}

// AvailableShaders lists all shaders that can be enabled
var AvailableShaders = []ShaderInfo{
	{
		ID:          "xbr",
		Name:        "Pixel Smoothing (xBR)",
		Description: "Smooth edges while preserving pixel art details",
		Weight:      0, // Handled separately in ApplyPreprocessEffects
	},
	{
		ID:          "ghosting",
		Name:        "Phosphor Persistence",
		Description: "Ghost trails from slow CRT phosphor decay",
		Weight:      0, // Handled separately in ApplyPreprocessEffects
	},
	{
		ID:          "crt",
		Name:        "CRT",
		Description: "Curved screen with RGB separation and vignette",
		Weight:      25,
	},
	{
		ID:          "scanlines",
		Name:        "Scanlines",
		Description: "Horizontal scanline effect",
		Weight:      400,
	},
	{
		ID:          "bloom",
		Name:        "Phosphor Glow",
		Description: "Bright pixels glow into neighbors like CRT phosphors",
		Weight:      550,
	},
	{
		ID:          "lcd",
		Name:        "LCD Grid",
		Description: "Visible pixel grid with RGB subpixels like handhelds",
		Weight:      300,
	},
	{
		ID:          "colorbleed",
		Name:        "Color Bleed",
		Description: "Horizontal color bleeding from composite video",
		Weight:      800,
	},
	{
		ID:          "dotmatrix",
		Name:        "Dot Matrix",
		Description: "Circular pixels like CRT phosphor dots",
		Weight:      350,
	},
	{
		ID:          "ntsc",
		Name:        "NTSC Artifacts",
		Description: "Color fringing at edges from NTSC encoding",
		Weight:      850,
	},
	{
		ID:          "rainbow",
		Name:        "NTSC Rainbow",
		Description: "Rainbow artifacts from dithering on composite video",
		Weight:      845,
	},
	{
		ID:          "gamma",
		Name:        "CRT Gamma",
		Description: "Non-linear brightness curve of CRT displays",
		Weight:      900,
	},
	{
		ID:          "halation",
		Name:        "Halation",
		Description: "Light bleeding behind CRT glass",
		Weight:      500,
	},
	{
		ID:          "rfnoise",
		Name:        "RF Noise",
		Description: "Subtle static grain from RF connection",
		Weight:      50,
	},
	{
		ID:          "rollingband",
		Name:        "Rolling Band",
		Description: "Scrolling dark band for bad reception look",
		Weight:      80,
	},
	{
		ID:          "vhs",
		Name:        "VHS Distortion",
		Description: "Wobble and tracking artifacts like VHS tape",
		Weight:      100,
	},
	{
		ID:          "interlace",
		Name:        "Interlace",
		Description: "Alternating scanline fields for 480i look",
		Weight:      380,
	},
	{
		ID:          "hsoft",
		Name:        "Horizontal Softness",
		Description: "Bandwidth-limited horizontal blur like analog video",
		Weight:      770,
	},
	{
		ID:          "vblur",
		Name:        "Vertical Blur",
		Description: "Electron beam softness causing scanline bleed",
		Weight:      760,
	},
	{
		ID:          "monochrome",
		Name:        "Monochrome",
		Description: "Black and white conversion",
		Weight:      700,
	},
	{
		ID:          "sepia",
		Name:        "Sepia",
		Description: "Warm brownish tint like old photographs",
		Weight:      650,
	},
}

// shaderWeights provides O(1) weight lookup by shader ID
var shaderWeights map[string]int

func init() {
	shaderWeights = make(map[string]int)
	for _, s := range AvailableShaders {
		shaderWeights[s.ID] = s.Weight
	}
}

// GetShaderWeight returns the weight for a shader ID (0 if unknown)
func GetShaderWeight(id string) int {
	return shaderWeights[id]
}
