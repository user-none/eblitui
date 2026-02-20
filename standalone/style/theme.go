//go:build !libretro

package style

import (
	"bytes"
	"image/color"
	"log"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"
)

// Theme colors (package-level variables updated by ApplyTheme)
var (
	Background        = color.NRGBA{0x1a, 0x1a, 0x2e, 0xff} // Dark blue-gray
	Surface           = color.NRGBA{0x25, 0x25, 0x3a, 0xff} // Slightly lighter
	Primary           = color.NRGBA{0x4a, 0x4a, 0x8a, 0xff} // Muted purple
	PrimaryHover      = color.NRGBA{0x5a, 0x5a, 0x9a, 0xff}
	Text              = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	TextSecondary     = color.NRGBA{0xaa, 0xaa, 0xaa, 0xff}
	Accent            = color.NRGBA{0xff, 0xd7, 0x00, 0xff} // Gold for favorites
	Border            = color.NRGBA{0x3a, 0x3a, 0x5a, 0xff}
	Black             = color.NRGBA{0x00, 0x00, 0x00, 0xff}
	DimOverlay        = color.NRGBA{0x00, 0x00, 0x00, 0xff} // Base color for screen-dimming overlays (alpha applied per use)
	OverlayBackground = color.NRGBA{0x1a, 0x1a, 0x2e, 0xff} // Base color for floating element backgrounds (alpha applied per use)
)

// Theme holds all color values for a UI theme
type Theme struct {
	Name              string
	Background        color.NRGBA
	Surface           color.NRGBA
	Primary           color.NRGBA
	PrimaryHover      color.NRGBA
	Text              color.NRGBA
	TextSecondary     color.NRGBA
	Accent            color.NRGBA
	Border            color.NRGBA
	Black             color.NRGBA
	DimOverlay        color.NRGBA
	OverlayBackground color.NRGBA
}

// Predefined themes
var (
	ThemeDefault = Theme{
		Name:              "Default",
		Background:        color.NRGBA{0x1a, 0x1a, 0x2e, 0xff}, // Dark blue-gray
		Surface:           color.NRGBA{0x25, 0x25, 0x3a, 0xff},
		Primary:           color.NRGBA{0x4a, 0x4a, 0x8a, 0xff}, // Muted purple
		PrimaryHover:      color.NRGBA{0x5a, 0x5a, 0x9a, 0xff},
		Text:              color.NRGBA{0xff, 0xff, 0xff, 0xff},
		TextSecondary:     color.NRGBA{0xaa, 0xaa, 0xaa, 0xff},
		Accent:            color.NRGBA{0xff, 0xd7, 0x00, 0xff}, // Gold
		Border:            color.NRGBA{0x3a, 0x3a, 0x5a, 0xff},
		Black:             color.NRGBA{0x00, 0x00, 0x00, 0xff},
		DimOverlay:        color.NRGBA{0x00, 0x00, 0x00, 0xff},
		OverlayBackground: color.NRGBA{0x1a, 0x1a, 0x2e, 0xff},
	}

	ThemeDark = Theme{
		Name:              "Dark",
		Background:        color.NRGBA{0x0a, 0x0a, 0x0a, 0xff}, // Pure black
		Surface:           color.NRGBA{0x1a, 0x1a, 0x1a, 0xff},
		Primary:           color.NRGBA{0x1e, 0x40, 0x7a, 0xff}, // Blue
		PrimaryHover:      color.NRGBA{0x2a, 0x50, 0x8a, 0xff},
		Text:              color.NRGBA{0xff, 0xff, 0xff, 0xff},
		TextSecondary:     color.NRGBA{0x88, 0x88, 0x88, 0xff},
		Accent:            color.NRGBA{0x00, 0xc8, 0x53, 0xff}, // Green
		Border:            color.NRGBA{0x2a, 0x2a, 0x2a, 0xff},
		Black:             color.NRGBA{0x00, 0x00, 0x00, 0xff},
		DimOverlay:        color.NRGBA{0x00, 0x00, 0x00, 0xff},
		OverlayBackground: color.NRGBA{0x0a, 0x0a, 0x0a, 0xff},
	}

	ThemeLight = Theme{
		Name:              "Light",
		Background:        color.NRGBA{0xe8, 0xe8, 0xe8, 0xff}, // Light gray
		Surface:           color.NRGBA{0xf5, 0xf5, 0xf5, 0xff},
		Primary:           color.NRGBA{0x1a, 0x56, 0xdb, 0xff}, // Blue
		PrimaryHover:      color.NRGBA{0x2a, 0x66, 0xeb, 0xff},
		Text:              color.NRGBA{0x1a, 0x1a, 0x1a, 0xff}, // Dark text
		TextSecondary:     color.NRGBA{0x66, 0x66, 0x66, 0xff},
		Accent:            color.NRGBA{0xe6, 0x5c, 0x00, 0xff}, // Orange
		Border:            color.NRGBA{0xcc, 0xcc, 0xcc, 0xff},
		Black:             color.NRGBA{0x00, 0x00, 0x00, 0xff},
		DimOverlay:        color.NRGBA{0x00, 0x00, 0x00, 0xff},
		OverlayBackground: color.NRGBA{0xe8, 0xe8, 0xe8, 0xff},
	}

	ThemeRetro = Theme{
		Name:              "Retro",
		Background:        color.NRGBA{0x1c, 0x1c, 0x1c, 0xff}, // Charcoal
		Surface:           color.NRGBA{0x28, 0x28, 0x28, 0xff},
		Primary:           color.NRGBA{0x8b, 0x00, 0x00, 0xff}, // Dark red
		PrimaryHover:      color.NRGBA{0xab, 0x20, 0x20, 0xff},
		Text:              color.NRGBA{0xd0, 0xd0, 0xd0, 0xff},
		TextSecondary:     color.NRGBA{0x80, 0x80, 0x80, 0xff},
		Accent:            color.NRGBA{0x00, 0xaa, 0x00, 0xff}, // Green
		Border:            color.NRGBA{0x3c, 0x3c, 0x3c, 0xff},
		Black:             color.NRGBA{0x00, 0x00, 0x00, 0xff},
		DimOverlay:        color.NRGBA{0x00, 0x00, 0x00, 0xff},
		OverlayBackground: color.NRGBA{0x1c, 0x1c, 0x1c, 0xff},
	}

	ThemePink = Theme{
		Name:              "Pink",
		Background:        color.NRGBA{0x1a, 0x0a, 0x1a, 0xff}, // Very dark magenta
		Surface:           color.NRGBA{0x3a, 0x1a, 0x3a, 0xff},
		Primary:           color.NRGBA{0xc0, 0x10, 0x70, 0xff}, // Darker pink
		PrimaryHover:      color.NRGBA{0xff, 0x14, 0x93, 0xff},
		Text:              color.NRGBA{0xff, 0xfa, 0xfc, 0xff}, // Bright white-pink
		TextSecondary:     color.NRGBA{0xff, 0x99, 0xcc, 0xff}, // Light pink
		Accent:            color.NRGBA{0xff, 0x00, 0xff, 0xff}, // Magenta
		Border:            color.NRGBA{0x5a, 0x2a, 0x5a, 0xff},
		Black:             color.NRGBA{0x00, 0x00, 0x00, 0xff},
		DimOverlay:        color.NRGBA{0x00, 0x00, 0x00, 0xff},
		OverlayBackground: color.NRGBA{0x1a, 0x0a, 0x1a, 0xff},
	}

	ThemeHotPink = Theme{
		Name:              "Hot Pink",
		Background:        color.NRGBA{0x4d, 0x15, 0x3d, 0xff}, // Brighter dark pink
		Surface:           color.NRGBA{0x70, 0x25, 0x58, 0xff}, // Brighter medium pink
		Primary:           color.NRGBA{0xff, 0x33, 0x99, 0xff}, // Brighter neon pink
		PrimaryHover:      color.NRGBA{0xff, 0x66, 0xb2, 0xff},
		Text:              color.NRGBA{0xff, 0xff, 0xff, 0xff}, // White
		TextSecondary:     color.NRGBA{0xff, 0xb3, 0xda, 0xff}, // Brighter light pink
		Accent:            color.NRGBA{0xff, 0x44, 0xff, 0xff}, // Brighter magenta
		Border:            color.NRGBA{0x99, 0x33, 0x77, 0xff},
		Black:             color.NRGBA{0x00, 0x00, 0x00, 0xff},
		DimOverlay:        color.NRGBA{0x00, 0x00, 0x00, 0xff},
		OverlayBackground: color.NRGBA{0x4d, 0x15, 0x3d, 0xff},
	}

	ThemeGreenLCD = Theme{
		Name:              "Green LCD",
		Background:        color.NRGBA{0x07, 0x20, 0x07, 0xff}, // Very dark green
		Surface:           color.NRGBA{0x0f, 0x38, 0x0f, 0xff}, // Dark green
		Primary:           color.NRGBA{0x30, 0x62, 0x30, 0xff}, // Mid green
		PrimaryHover:      color.NRGBA{0x4a, 0x7c, 0x4a, 0xff},
		Text:              color.NRGBA{0x9b, 0xbc, 0x0f, 0xff}, // Bright yellow-green
		TextSecondary:     color.NRGBA{0x5a, 0x7a, 0x0f, 0xff}, // Dimmer green
		Accent:            color.NRGBA{0xc0, 0xe0, 0x30, 0xff}, // Bright lime
		Border:            color.NRGBA{0x20, 0x50, 0x20, 0xff},
		Black:             color.NRGBA{0x07, 0x20, 0x07, 0xff},
		DimOverlay:        color.NRGBA{0x07, 0x20, 0x07, 0xff},
		OverlayBackground: color.NRGBA{0x07, 0x20, 0x07, 0xff},
	}

	ThemeHighContrast = Theme{
		Name:              "High Contrast",
		Background:        color.NRGBA{0x00, 0x00, 0x00, 0xff}, // Pure black
		Surface:           color.NRGBA{0x40, 0x40, 0x40, 0xff}, // Medium gray - button bg
		Primary:           color.NRGBA{0x00, 0x80, 0xff, 0xff}, // Bright blue
		PrimaryHover:      color.NRGBA{0x40, 0xa0, 0xff, 0xff}, // Lighter blue
		Text:              color.NRGBA{0xff, 0xff, 0xff, 0xff}, // Pure white
		TextSecondary:     color.NRGBA{0xcc, 0xcc, 0xcc, 0xff}, // Light gray
		Accent:            color.NRGBA{0xff, 0xff, 0x00, 0xff}, // Yellow for favorites
		Border:            color.NRGBA{0x66, 0x66, 0x66, 0xff}, // Medium gray (disabled bg)
		Black:             color.NRGBA{0x00, 0x00, 0x00, 0xff},
		DimOverlay:        color.NRGBA{0x00, 0x00, 0x00, 0xff},
		OverlayBackground: color.NRGBA{0x00, 0x00, 0x00, 0xff},
	}

	// AvailableThemes lists all themes for UI selection
	AvailableThemes = []Theme{ThemeDefault, ThemeDark, ThemeLight, ThemeRetro, ThemePink, ThemeHotPink, ThemeGreenLCD, ThemeHighContrast}

	// CurrentThemeName tracks the active theme name
	CurrentThemeName = "Default"
)

// ThemeNames returns the list of valid theme name strings.
func ThemeNames() []string {
	names := make([]string, len(AvailableThemes))
	for i, t := range AvailableThemes {
		names[i] = t.Name
	}
	return names
}

// GetThemeByName returns theme by name, or ThemeDefault if not found
func GetThemeByName(name string) Theme {
	for _, t := range AvailableThemes {
		if t.Name == name {
			return t
		}
	}
	return ThemeDefault
}

// IsValidThemeName returns true if the name matches a known theme
func IsValidThemeName(name string) bool {
	for _, t := range AvailableThemes {
		if t.Name == name {
			return true
		}
	}
	return false
}

// ApplyTheme updates package-level color variables from a theme
func ApplyTheme(theme Theme) {
	Background = theme.Background
	Surface = theme.Surface
	Primary = theme.Primary
	PrimaryHover = theme.PrimaryHover
	Text = theme.Text
	TextSecondary = theme.TextSecondary
	Accent = theme.Accent
	Border = theme.Border
	Black = theme.Black
	DimOverlay = theme.DimOverlay
	OverlayBackground = theme.OverlayBackground
	CurrentThemeName = theme.Name
}

// ApplyThemeByName applies theme by name with fallback to Default
func ApplyThemeByName(name string) {
	ApplyTheme(GetThemeByName(name))
}

// currentFontSize is the current font size in points (default 14)
var currentFontSize float64 = 14

// dpiScale is the device pixel ratio (1.0 on non-retina, 2.0 on retina)
var dpiScale float64 = 1.0

// DPIScale returns the current device scale factor.
func DPIScale() float64 {
	return dpiScale
}

// Px converts a logical pixel value to physical pixels using the current DPI scale.
func Px(logical int) int {
	return int(float64(logical) * dpiScale)
}

// PxFont converts a logical pixel value to physical pixels scaled by both DPI and font size.
func PxFont(logical int) int {
	return int(float64(logical) * FontScale() * dpiScale)
}

// SetDPIScale sets the DPI scale factor and recalculates all spatial vars.
func SetDPIScale(scale float64) {
	if scale < 1.0 {
		scale = 1.0
	}
	dpiScale = scale

	// Recalculate all non-font-dependent spatial vars from base constants
	DefaultPadding = Px(baseDefaultPadding)
	DefaultSpacing = Px(baseDefaultSpacing)
	SmallSpacing = Px(baseSmallSpacing)
	TinySpacing = Px(baseTinySpacing)
	LargeSpacing = Px(baseLargeSpacing)
	ScrollbarWidth = Px(baseScrollbarWidth)
	ButtonPaddingSmall = Px(baseButtonPaddingSmall)
	ButtonPaddingMedium = Px(baseButtonPaddingMedium)
	IconMinCardWidth = Px(baseIconMinCardWidth)
	IconDefaultWindowWidth = Px(baseIconDefaultWinWidth)
	DetailArtWidthSmall = Px(baseDetailArtSmall)
	DetailArtWidthLarge = Px(baseDetailArtLarge)
	SettingsSidebarMinWidth = Px(baseSidebarMinWidth)
	SettingsFolderListMinHeight = Px(baseFolderListMinHeight)
	ProgressBarWidth = Px(baseProgressBarWidth)
	ProgressBarHeight = Px(baseProgressBarHeight)
	AchievementRowSpacing = Px(baseAchievementRowSpac)
	AchievementPanelMargin = Px(baseAchievementPanelMargin)
	AchievementMinPanelHeight = Px(baseAchievementMinPanelH)
	AchievementNotifyMargin = Px(baseAchNotifyMargin)
	AchievementNotifyBadgeSize = Px(baseAchNotifyBadge)
	AchievementNotifyPaddingH = Px(baseAchNotifyPaddingH)
	AchievementNotifyPaddingV = Px(baseAchNotifyPaddingV)
	AchievementNotifySpacing = Px(baseAchNotifySpacing)
	AchievementNotifyBorder = Px(baseAchNotifyBorder)
	OverlayPadding = Px(baseOverlayPadding)
	OverlayMargin = Px(baseOverlayMargin)
	PauseMenuMinWidth = Px(basePauseMinWidth)
	PauseMenuMaxWidth = Px(basePauseMaxWidth)
	PauseMenuMinBtnHeight = Px(basePauseMinBtnH)
	PauseMenuMaxBtnHeight = Px(basePauseMaxBtnH)
	ListMinTitleWidth = Px(baseListMinTitleWidth)

	// Recalculate font-dependent vars (they also incorporate DPI scale)
	ApplyFontSize(int(currentFontSize))
}

// sharedFontSource is the cached TrueType font source shared by all font faces
var sharedFontSource *text.GoTextFaceSource

// fontFace is the cached font face
var fontFace text.Face

// largeFontFace is the cached large font face for achievements
var largeFontFace *text.GoTextFace

// loadFontSource loads the shared GoTextFaceSource from goregular.TTF (once)
func loadFontSource() *text.GoTextFaceSource {
	if sharedFontSource == nil {
		source, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
		if err != nil {
			log.Printf("Failed to load font source: %v", err)
			return nil
		}
		sharedFontSource = source
	}
	return sharedFontSource
}

// FontFace returns the font face to use for UI text
func FontFace() *text.Face {
	if fontFace == nil {
		source := loadFontSource()
		if source == nil {
			return &fontFace
		}
		fontFace = &text.GoTextFace{
			Source: source,
			Size:   currentFontSize,
		}
	}
	return &fontFace
}

// LargeFontFace returns a larger font face for prominent displays like achievements
func LargeFontFace() *text.GoTextFace {
	if largeFontFace == nil {
		source := loadFontSource()
		if source == nil {
			return nil
		}
		largeSize := currentFontSize * 2
		if largeSize > 48 {
			largeSize = 48
		}
		largeFontFace = &text.GoTextFace{
			Source: source,
			Size:   largeSize,
		}
	}
	return largeFontFace
}

// FontScale returns the current font scale factor relative to the base size (14pt).
func FontScale() float64 {
	return currentFontSize / 14.0
}

// ApplyFontSize sets the font size and recalculates all font-dependent layout values.
func ApplyFontSize(size int) {
	s := float64(size)
	currentFontSize = s

	// Replace font faces in-place rather than nil-ing them. Existing widgets hold
	// &fontFace (a pointer to the package var), so setting fontFace = nil would cause
	// widgets to see a nil face and crash before the UI rebuild completes.
	source := loadFontSource()
	if source != nil {
		fontFace = &text.GoTextFace{
			Source: source,
			Size:   s * dpiScale,
		}
		largeSize := s * 2
		if largeSize > baseMaxLargeFontSize {
			largeSize = baseMaxLargeFontSize
		}
		largeFontFace = &text.GoTextFace{
			Source: source,
			Size:   largeSize * dpiScale,
		}
	}

	// Scale font-dependent layout constants (font scale * DPI scale)
	scale := s / 14.0
	d := dpiScale
	ListRowHeight = int(baseListRowHeight * scale * d)
	ListHeaderHeight = int(baseListHeaderHeight * scale * d)
	IconCardTextHeight = int(baseIconCardTextHeight * scale * d)
	// Badge and row use dampened scaling so they don't grow disproportionately.
	// (1 + fontScale) / 2 grows slower than text: at 2x font it's only 1.5x.
	badgeScale := (1 + scale) / 2
	AchievementRowHeight = int(baseAchievementRowHeight * badgeScale * d)
	AchievementBadgeSize = int(baseAchievementBadgeSize * badgeScale * d)
	AchievementOverlayWidth = int(baseAchievementOverlayW * scale * d)
	AchievementOverlayPadding = int(baseAchievementOverlayPad * scale * d)
	SettingsRowHeight = int(baseSettingsRowHeight * scale * d)
	EstimatedViewportHeight = int(baseEstimatedViewportHeight * scale * d)
	ListColGenre = int(baseListColGenre * scale * d)
	ListColRegion = int(baseListColRegion * scale * d)
	ListColPlayTime = int(baseListColPlayTime * scale * d)
	ListColLastPlayed = int(baseListColLastPlayed * scale * d)
	ListColFavorite = int(baseListColFavorite * scale * d)
}

// ButtonImage creates a standard button image set
func ButtonImage() *widget.ButtonImage {
	return &widget.ButtonImage{
		Idle:     image.NewNineSliceColor(Surface),
		Hover:    image.NewNineSliceColor(PrimaryHover),
		Pressed:  image.NewNineSliceColor(Primary),
		Disabled: image.NewNineSliceColor(Border),
	}
}

// PrimaryButtonImage creates a prominent button image set
func PrimaryButtonImage() *widget.ButtonImage {
	return &widget.ButtonImage{
		Idle:     image.NewNineSliceColor(Primary),
		Hover:    image.NewNineSliceColor(PrimaryHover),
		Pressed:  image.NewNineSliceColor(Surface),
		Disabled: image.NewNineSliceColor(Border),
	}
}

// DisabledButtonImage creates a disabled-looking button image set
func DisabledButtonImage() *widget.ButtonImage {
	return &widget.ButtonImage{
		Idle:     image.NewNineSliceColor(Border),
		Hover:    image.NewNineSliceColor(Border),
		Pressed:  image.NewNineSliceColor(Border),
		Disabled: image.NewNineSliceColor(Border),
	}
}

// ActiveButtonImage returns a button image based on active state.
// Used for toggle buttons like view mode selectors and sidebar items.
func ActiveButtonImage(active bool) *widget.ButtonImage {
	if active {
		return PrimaryButtonImage()
	}
	return ButtonImage()
}

// SliderButtonImage creates a slider handle button image
func SliderButtonImage() *widget.ButtonImage {
	return &widget.ButtonImage{
		Idle:     image.NewNineSliceColor(Primary),
		Hover:    image.NewNineSliceColor(PrimaryHover),
		Pressed:  image.NewNineSliceColor(Primary),
		Disabled: image.NewNineSliceColor(Border),
	}
}

// SliderTrackImage creates a slider track image
func SliderTrackImage() *widget.SliderTrackImage {
	return &widget.SliderTrackImage{
		Idle:  image.NewNineSliceColor(Border),
		Hover: image.NewNineSliceColor(Border),
	}
}

// ScrollContainerImage creates a scroll container image
func ScrollContainerImage() *widget.ScrollContainerImage {
	return &widget.ScrollContainerImage{
		Idle: image.NewNineSliceColor(Background),
		Mask: image.NewNineSliceColor(Background),
	}
}

// ButtonTextColor returns the standard button text colors
func ButtonTextColor() *widget.ButtonTextColor {
	return &widget.ButtonTextColor{
		Idle:     Text,
		Disabled: TextSecondary,
	}
}
