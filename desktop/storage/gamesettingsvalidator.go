// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"strings"
	"unicode/utf8"
)

// SanitizeGameSettings silently corrects invalid game settings fields.
// This runs on load so invalid values never reach the UI or emulator.
func SanitizeGameSettings(s *GameSettingsFile) {
	// Save slot: must be 0-9
	if s.SaveStates.Slot < 0 || s.SaveStates.Slot > 9 {
		s.SaveStates.Slot = 0
	}

	// Enabled cheat IDs: drop empty and repeated IDs
	seen := make(map[string]bool, len(s.Cheats.Enabled))
	enabled := s.Cheats.Enabled[:0]
	for _, id := range s.Cheats.Enabled {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		enabled = append(enabled, id)
	}
	s.Cheats.Enabled = enabled

	// Custom cheats: drop entries with no name or code, or over the
	// limits. They are dropped, not truncated, since a truncated code
	// is a different cheat.
	custom := s.Cheats.Custom[:0]
	for _, c := range s.Cheats.Custom {
		if len(custom) == MaxCustomCheats {
			break
		}
		if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Code) == "" ||
			utf8.RuneCountInString(c.Name) > MaxCustomCheatNameLen ||
			utf8.RuneCountInString(c.Code) > MaxCustomCheatCodeLen ||
			strings.Count(c.Code, "\n") >= MaxCustomCheatCodeRows {
			continue
		}
		custom = append(custom, c)
	}
	s.Cheats.Custom = custom
}
