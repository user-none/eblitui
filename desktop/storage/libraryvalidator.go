// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"fmt"
	"strings"
)

// CleanGameKey removes path separators and control characters from a game
// key so it is safe to use as a file or directory name. Returns "" when
// nothing usable remains or the result is a traversal name.
func CleanGameKey(key string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, key)
	if cleaned == "." || cleaned == ".." {
		return ""
	}
	return cleaned
}

// SanitizeLibraryEntries silently corrects invalid game entry fields.
// This runs on load so invalid values never reach the UI or emulator.
func SanitizeLibraryEntries(lib *Library) {
	// Game keys are used in file paths. Re-key entries whose key needs
	// cleaning, and drop entries with no usable key or whose cleaned key
	// collides with an existing entry.
	for key, game := range lib.Games {
		if game == nil {
			delete(lib.Games, key)
			continue
		}
		cleaned := CleanGameKey(key)
		if cleaned == key && game.CRC32 == key {
			continue
		}
		delete(lib.Games, key)
		if cleaned == "" {
			continue
		}
		if _, exists := lib.Games[cleaned]; exists {
			continue
		}
		game.CRC32 = cleaned
		lib.Games[cleaned] = game
	}

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
