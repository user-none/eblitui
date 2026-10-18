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
	sanitizeGameEntries(lib.Games)
}

// SanitizeRemovedGames silently corrects invalid removed game entries the
// same way as library entries, and clears any files and added time so a
// restored entry only gets the files a scan finds and is added when found.
func SanitizeRemovedGames(removed *RemovedGames) {
	sanitizeGameEntries(removed.Games)
	for _, game := range removed.Games {
		stripRemoved(game)
	}
}

// sanitizeGameEntries corrects invalid entries in a map of game entries
// keyed by game key.
func sanitizeGameEntries(games map[string]*GameEntry) {
	// Game keys are used in file paths. Re-key entries whose key needs
	// cleaning, and drop entries with no usable key or whose cleaned key
	// collides with an existing entry.
	for key, game := range games {
		if game == nil {
			delete(games, key)
			continue
		}
		cleaned := CleanGameKey(key)
		if cleaned == key && game.CRC32 == key {
			continue
		}
		delete(games, key)
		if cleaned == "" {
			continue
		}
		if _, exists := games[cleaned]; exists {
			continue
		}
		game.CRC32 = cleaned
		games[cleaned] = game
	}

	for _, game := range games {
		if game.PlayTimeSeconds < 0 {
			game.PlayTimeSeconds = 0
		}
		if game.LastPlayed < 0 {
			game.LastPlayed = 0
		}
		if game.Added < 0 {
			game.Added = 0
		}
	}
}

// ValidateLibrary checks library-level fields against valid ranges and returns
// human-readable error descriptions. An empty slice means the library is valid.
func ValidateLibrary(lib *Library) []string {
	var errors []string

	if lib.Version != libraryVersion {
		errors = append(errors, fmt.Sprintf("version: %d (valid: %d)", lib.Version, libraryVersion))
	}

	return errors
}

// CorrectLibrary resets any invalid library-level fields to their defaults.
func CorrectLibrary(lib *Library) *Library {
	if lib.Version != libraryVersion {
		lib.Version = libraryVersion
	}
	return lib
}
