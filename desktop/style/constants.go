// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package style

import "time"

// Window scaling. The layout is designed for the minimum window size and the
// whole UI, font included, grows with the window from there.
const (
	MinWindowWidth  = 900
	MinWindowHeight = 650
	maxWindowScale  = 4.0

	// baseFontSize is the font size in points at scale 1.0
	baseFontSize = 14.0

	// largeFontFactor is the large font size relative to the font size
	largeFontFactor = 2.0
)

// Base constants (unexported) — these are the logical-pixel reference values.
// The corresponding exported vars are recalculated whenever a scale changes.
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
	baseIconUnfocusedScale  = 0.95
	baseIconUnfocusedDim    = 0.95
	baseDetailColumnWidth   = 200
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
)

// Layout vars used across screens, scaled at runtime from the base constants.
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

// List and icon view layout values
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
	IconUnfocusedScale     = baseIconUnfocusedScale
	IconUnfocusedDim       = baseIconUnfocusedDim
)

// Detail screen vars
var DetailColumnWidth = baseDetailColumnWidth

// Sidebar default minimum width
var SidebarMinWidth = baseSidebarMinWidth

// Settings screen vars
var SettingsFolderListMinHeight = baseFolderListMinHeight

// Settings row height
var SettingsRowHeight = baseSettingsRowHeight

// Progress bar vars
var (
	ProgressBarWidth  = baseProgressBarWidth
	ProgressBarHeight = baseProgressBarHeight
)

// Scroll estimation viewport height
var EstimatedViewportHeight = baseEstimatedViewportHeight

// Gamepad navigation timing constants
const (
	NavInitialDelay  = 400 * time.Millisecond // Delay before repeat starts
	NavStartInterval = 200 * time.Millisecond // Initial repeat interval
	NavMinInterval   = 25 * time.Millisecond  // Fastest repeat (cap)
	NavAcceleration  = 20 * time.Millisecond  // Speed increase per repeat
)

// Auto-save timing constant
const (
	AutoSaveInterval = 5 * time.Second
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

// Achievement list and overlay values
var (
	AchievementBadgeSize      = baseAchievementBadgeSize
	AchievementRowHeight      = baseAchievementRowHeight
	AchievementOverlayWidth   = baseAchievementOverlayW
	AchievementOverlayPadding = baseAchievementOverlayPad
)
