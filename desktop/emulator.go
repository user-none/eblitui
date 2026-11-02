// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import "github.com/user-none/eblitui/desktop/content"

// Emulator describes the application built around a core: its identity,
// where it keeps its data, the content it supplies or downloads, and the
// keys that verify what it downloads. The core itself is described by
// coreif.SystemInfo. A nil source or config turns that feature off.
type Emulator struct {
	Name        string // window title and RetroAchievements client name
	Version     string // running version
	DataDirName string // data directory name under the platform data folder
	Icon        []byte // PNG, JPEG, or WebP application icon; nil for none

	// RetroAchievementsConsoleID identifies the emulated console to
	// RetroAchievements; 0 turns achievements off.
	RetroAchievementsConsoleID int

	// CheatFormat names the cheats package system whose code formats the
	// emulator's cheats use, such as "saturn"; "" turns cheats off.
	CheatFormat string

	Update  *UpdateConfig  // where releases are published; nil or no keys turns update checks off
	Artwork *ArtworkConfig // where artwork is published; nil turns artwork downloads off

	Metadata content.MetadataSource // game metadata; nil shows file names only
	Cheats   content.CheatsSource   // database cheats; nil offers custom cheats only
	Rumble   content.RumbleSource   // rumble files; nil turns rumble off
}

// ArtworkConfig is where an emulator's artwork is published.
type ArtworkConfig struct {
	// URL is the HTTPS base URL of the artwork site, which serves
	// index.json, index.sig, and the images the index lists.
	URL string

	// Keys are the public keys, as algorithm:base64, trusted to sign the
	// artwork index. Without them no artwork is downloaded.
	Keys []string
}
