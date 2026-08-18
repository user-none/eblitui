// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import "fmt"

// SanitizeLibraryEntries silently corrects invalid game entry fields.
// This runs on load so invalid values never reach the UI or emulator.
func SanitizeLibraryEntries(lib *Library) {
	for _, game := range lib.Games {
		if game.PlayTimeSeconds < 0 {
			game.PlayTimeSeconds = 0
		}
		if game.LastPlayed < 0 {
			game.LastPlayed = 0
		}
		if game.Added < 0 {
			game.Added = 0
		}

		// saveSlot: must be 0-9
		if game.Settings.SaveSlot < 0 || game.Settings.SaveSlot > 9 {
			game.Settings.SaveSlot = 0
		}
	}
}

// ValidateLibrary checks library-level fields against valid ranges and returns
// human-readable error descriptions. An empty slice means the library is valid.
func ValidateLibrary(lib *Library) []string {
	var errors []string

	if lib.Version != 1 {
		errors = append(errors, fmt.Sprintf("version: %d (valid: 1)", lib.Version))
	}

	return errors
}

// CorrectLibrary resets any invalid library-level fields to their defaults.
func CorrectLibrary(lib *Library) *Library {
	if lib.Version != 1 {
		lib.Version = 1
	}
	return lib
}
