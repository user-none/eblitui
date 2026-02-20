//go:build !libretro

// Package types provides shared interfaces used across UI packages.
// This package exists to avoid import cycles between screens and sub-packages.
package types

import (
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/rdb"
)

// Direction constants for navigation
const (
	DirNone  = 0
	DirUp    = 1
	DirDown  = 2
	DirLeft  = 3
	DirRight = 4
)

// Navigation zone types
const (
	NavZoneHorizontal = "horizontal" // Left/Right navigates, Up/Down exits zone
	NavZoneVertical   = "vertical"   // Up/Down navigates, Left/Right exits zone
	NavZoneGrid       = "grid"       // 2D grid navigation
)

// Navigation index constants
const (
	NavIndexPreserve = -1 // Try to preserve column/row position
	NavIndexFirst    = -2 // Go to first item
	NavIndexLast     = -3 // Go to last item
)

// ScreenCallback provides callbacks for screen navigation
type ScreenCallback interface {
	SwitchToLibrary()
	SwitchToDetail(gameCRC string)
	SwitchToSettings()
	SwitchToScanProgress(rescanAll bool)
	LaunchGame(gameCRC string, resume bool)
	Exit()
	GetWindowWidth() int             // For responsive layout calculations
	RequestRebuild()                 // Request UI rebuild after state changes
	GetPlaceholderImageData() []byte // Get raw placeholder image data for missing artwork
	GetRDB() *rdb.RDB                // Get RDB for metadata lookups
	GetExtensions() []string         // Get supported ROM file extensions
}

// FocusRestorer is implemented by screens that support focus restoration after rebuilds
type FocusRestorer interface {
	// GetPendingFocusButton returns the button that should receive focus after rebuild
	GetPendingFocusButton() *widget.Button
	// ClearPendingFocus clears the pending focus state
	ClearPendingFocus()
}

// FocusManager interface for focus restoration and scroll management.
// Implemented by BaseScreen, used by sub-sections that need to register
// focusable buttons and manage scroll position.
type FocusManager interface {
	RegisterFocusButton(key string, btn *widget.Button)
	SetPendingFocus(key string)
	SetScrollWidgets(sc *widget.ScrollContainer, slider *widget.Slider)
	SaveScrollPosition()
	RestoreScrollPosition()
	// Zone-based navigation
	RegisterNavZone(name string, zoneType string, keys []string, columns int)
	SetNavTransition(fromZone string, direction int, toZone string, toIndex int)
}
