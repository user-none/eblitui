//go:build !libretro

package style

import "time"

// Base constants (unexported) — these are the logical-pixel reference values.
// The corresponding exported vars are recalculated by SetDPIScale.
const (
	baseDefaultPadding      = 16
	baseDefaultSpacing      = 16
	baseSmallSpacing        = 8
	baseTinySpacing         = 4
	baseLargeSpacing        = 24
	baseScrollbarWidth      = 20
	baseButtonPaddingSmall  = 8
	baseButtonPaddingMedium = 12
	baseIconMinCardWidth    = 200
	baseIconDefaultWinWidth = 800
	baseDetailArtSmall      = 150
	baseDetailArtLarge      = 400
	baseSidebarMinWidth     = 180
	baseFolderListMinHeight = 100
	baseProgressBarWidth    = 300
	baseProgressBarHeight   = 20
	baseAchievementRowSpac  = 4

	// Overlay (notification/search shared)
	baseOverlayPadding = 12
	baseOverlayMargin  = 8

	// Pause menu
	basePauseMinWidth = 150
	basePauseMaxWidth = 350
	basePauseMinBtnH  = 40
	basePauseMaxBtnH  = 60

	// Achievement overlay
	baseAchievementPanelMargin = 40
	baseAchievementMinPanelH   = 200

	// Achievement notification
	baseAchNotifyMargin   = 20
	baseAchNotifyBadge    = 64
	baseAchNotifyPaddingH = 20
	baseAchNotifyPaddingV = 16
	baseAchNotifySpacing  = 6
	baseAchNotifyBorder   = 2

	// Library list view
	baseListMinTitleWidth = 150

	// Font-dependent base values (at 14pt, scale = 1.0)
	baseListRowHeight           = 40
	baseListHeaderHeight        = 38
	baseIconCardTextHeight      = 34
	baseListColFavorite         = 24
	baseListColGenre            = 100
	baseListColRegion           = 50
	baseListColPlayTime         = 80
	baseListColLastPlayed       = 100
	baseSettingsRowHeight       = 38
	baseEstimatedViewportHeight = 400
	baseAchievementRowHeight    = 92
	baseAchievementBadgeSize    = 56
	baseAchievementOverlayW     = 500
	baseAchievementOverlayPad   = 16
	baseMaxLargeFontSize        = 48
)

// Layout vars used across screens — DPI-scaled at runtime via SetDPIScale.
var (
	// Standard spacing and padding values
	DefaultPadding = baseDefaultPadding
	DefaultSpacing = baseDefaultSpacing
	SmallSpacing   = baseSmallSpacing
	TinySpacing    = baseTinySpacing
	LargeSpacing   = baseLargeSpacing

	// Scrollbar dimensions
	ScrollbarWidth = baseScrollbarWidth

	// Button padding
	ButtonPaddingSmall  = baseButtonPaddingSmall
	ButtonPaddingMedium = baseButtonPaddingMedium
)

// Font-dependent layout values (updated by ApplyFontSize)
var (
	ListRowHeight    = baseListRowHeight
	ListHeaderHeight = baseListHeaderHeight

	// Column widths for library list view
	ListColFavorite   = baseListColFavorite
	ListColGenre      = baseListColGenre
	ListColRegion     = baseListColRegion
	ListColPlayTime   = baseListColPlayTime
	ListColLastPlayed = baseListColLastPlayed

	// Icon view
	IconCardTextHeight = baseIconCardTextHeight
)

// Icon view vars for grid layouts
var (
	IconMinCardWidth       = baseIconMinCardWidth
	IconDefaultWindowWidth = baseIconDefaultWinWidth
)

// Detail screen vars
var (
	DetailArtWidthSmall = baseDetailArtSmall
	DetailArtWidthLarge = baseDetailArtLarge
)

// Settings screen vars
var (
	SettingsSidebarMinWidth     = baseSidebarMinWidth
	SettingsFolderListMinHeight = baseFolderListMinHeight
)

// Font-dependent settings layout value (updated by ApplyFontSize)
var SettingsRowHeight = baseSettingsRowHeight

// Progress bar vars
var (
	ProgressBarWidth  = baseProgressBarWidth
	ProgressBarHeight = baseProgressBarHeight
)

// Font-dependent scroll estimation (updated by ApplyFontSize)
var EstimatedViewportHeight = baseEstimatedViewportHeight

// Gamepad navigation timing constants
const (
	NavInitialDelay  = 400 * time.Millisecond // Delay before repeat starts
	NavStartInterval = 200 * time.Millisecond // Initial repeat interval
	NavMinInterval   = 25 * time.Millisecond  // Fastest repeat (cap)
	NavAcceleration  = 20 * time.Millisecond  // Speed increase per repeat
)

// Auto-save and timing constants
const (
	AutoSaveInterval = 5 * time.Second
	HTTPTimeout      = 10 * time.Second
)

// Mouse wheel scroll sensitivity
const (
	ScrollWheelSensitivity = 0.05
)

// Overlay vars (shared by notification/search)
var (
	OverlayPadding = baseOverlayPadding
	OverlayMargin  = baseOverlayMargin
)

// Pause menu vars
var (
	PauseMenuMinWidth     = basePauseMinWidth
	PauseMenuMaxWidth     = basePauseMaxWidth
	PauseMenuMinBtnHeight = basePauseMinBtnH
	PauseMenuMaxBtnHeight = basePauseMaxBtnH
)

// Achievement UI vars
var (
	AchievementRowSpacing      = baseAchievementRowSpac
	AchievementPanelMargin     = baseAchievementPanelMargin
	AchievementMinPanelHeight  = baseAchievementMinPanelH
	AchievementNotifyMargin    = baseAchNotifyMargin
	AchievementNotifyBadgeSize = baseAchNotifyBadge
	AchievementNotifyPaddingH  = baseAchNotifyPaddingH
	AchievementNotifyPaddingV  = baseAchNotifyPaddingV
	AchievementNotifySpacing   = baseAchNotifySpacing
	AchievementNotifyBorder    = baseAchNotifyBorder
)

// Library list view vars
var (
	ListMinTitleWidth = baseListMinTitleWidth
)

// Font-dependent achievement values (updated by ApplyFontSize)
var (
	AchievementBadgeSize      = baseAchievementBadgeSize
	AchievementRowHeight      = baseAchievementRowHeight
	AchievementOverlayWidth   = baseAchievementOverlayW
	AchievementOverlayPadding = baseAchievementOverlayPad
)
