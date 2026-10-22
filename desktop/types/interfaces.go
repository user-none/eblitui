// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package types provides shared interfaces used across UI packages.
// This package exists to avoid import cycles between screens and sub-packages.
package types

import (
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/filebrowser"
	"github.com/user-none/eblitui/desktop/widgets"
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

// FilePickerRequest describes a file or folder pick. OnSelect runs on the UI
// thread with the chosen absolute path; it is not called on cancel.
type FilePickerRequest struct {
	Title    string
	Mode     filebrowser.Mode
	OnSelect func(path string)
}

// ScreenCallback provides callbacks for screen navigation
type ScreenCallback interface {
	SwitchToLibrary()
	SwitchToDetail(gameCRC string)
	SwitchToSettings()
	// SwitchToSettingsSection opens settings with the section identified
	// by focusKey selected.
	SwitchToSettingsSection(focusKey string)
	SwitchToScanProgress()
	// SwitchToCleanProgress shows the progress screen and runs Clean
	// Library, which removes the library's missing discs and games.
	SwitchToCleanProgress()
	// OpenFilePicker shows the full-screen file/folder picker. The current
	// screen is restored when the picker closes.
	OpenFilePicker(req FilePickerRequest)
	// LaunchGame starts a game. discIndex selects which disc of a
	// multi-disc game to launch; -1 uses the entry's remembered
	// SelectedDisc. Ignored for single-disc and cartridge games.
	LaunchGame(gameCRC string, resume bool, discIndex int)
	Exit()
	GetWindowWidth() int               // For responsive layout calculations
	RequestRebuild()                   // Request UI rebuild after state changes
	GetPlaceholderImageData() []byte   // Get raw placeholder image data (loading indicator)
	GetMissingArtImageData() []byte    // Get raw missing-art image data (no artwork found)
	GetMD5ByCRC32(crc32 uint32) string // Get MD5 hash from RDB by CRC32
	GetExtensions() []string           // Content file extensions (disc formats for disc cores, else ROM extensions)
	ShowNotification(msg string)       // Show a brief notification message
}

// UpdateStatus is the update state shown in Settings > About.
type UpdateStatus struct {
	Checking  bool   // A check is running
	Available string // Version of an available update, "" when none
	Failed    bool   // The last check this session failed
	LastCheck int64  // Unix time of the last check, 0 = never
}

// Updates is the update system as seen by the library update bar and
// Settings > About. Screens are given nil when the update system is off.
type Updates interface {
	AppName() string
	Status() UpdateStatus
	// BarVersion returns the version the update bar offers, or "" when the
	// bar is not shown.
	BarVersion() string
	CheckNow()
	IgnoreVersion()
	Dismiss()
	// Download opens the available version's release page and ignores
	// that version for the update bar.
	Download()
}

// FocusRestorer is implemented by screens that support focus restoration after rebuilds
type FocusRestorer interface {
	// GetPendingFocus returns the target that should receive focus after rebuild
	GetPendingFocus() widget.Focuser
	// ClearPendingFocus clears the pending focus state
	ClearPendingFocus()
}

// ChainZones links zones top to bottom: Down from each enters the first
// target of the next, and Up from each enters the last target of the one
// above.
func ChainZones(focus FocusManager, zones []string) {
	for i := 0; i+1 < len(zones); i++ {
		focus.SetNavTransition(zones[i], DirDown, zones[i+1], NavIndexFirst)
		focus.SetNavTransition(zones[i+1], DirUp, zones[i], NavIndexLast)
	}
}

// FocusManager interface for focus restoration and scroll management.
// Implemented by BaseScreen, used by sub-sections that need to register
// focusable buttons and text fields and manage scroll position.
type FocusManager interface {
	RegisterFocusButton(key string, btn *widget.Button)
	RegisterFocusWidget(key string, f widget.Focuser)
	SetPendingFocus(key string)
	SetScrollContainer(sc *widgets.ScrollView)
	SaveScrollPosition()
	RestoreScrollPosition()
	// Zone-based navigation
	RegisterNavZone(name string, zoneType string, keys []string, columns int)
	SetNavTransition(fromZone string, direction int, toZone string, toIndex int)
}
