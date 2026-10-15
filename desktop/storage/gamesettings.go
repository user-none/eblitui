// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"errors"
	"os"
	"slices"
)

// GameSettingsFile holds a game's settings, stored per game in
// gamesettings/<key>.json. Each feature has its own section.
type GameSettingsFile struct {
	SaveStates SaveStateSettings `json:"saveStates"`
	Cheats     CheatSettings     `json:"cheats"`
}

// SaveStateSettings holds a game's save state choices.
type SaveStateSettings struct {
	Slot int `json:"slot,omitempty"` // Last-used save state slot (0-9)
}

// CheatSettings holds a game's cheat choices.
type CheatSettings struct {
	Enabled []string      `json:"enabled,omitempty"` // IDs of the cheats turned on
	Custom  []CustomCheat `json:"custom,omitempty"`  // cheats added by the user
}

// Limits on custom cheats. Entries over them are dropped on load.
const (
	MaxCustomCheats        = 1000
	MaxCustomCheatNameLen  = 64   // runes
	MaxCustomCheatCodeLen  = 1024 // runes, line breaks included
	MaxCustomCheatCodeRows = 128  // lines
)

// CustomCheat is a cheat added by the user: a name and a code string as
// entered.
type CustomCheat struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

// LoadGameSettings loads a game's settings file. A missing file returns
// empty settings. Invalid entries are removed on load.
func LoadGameSettings(gameCRC string) (*GameSettingsFile, error) {
	path, err := GetGameSettingsPath(gameCRC)
	if err != nil {
		return nil, err
	}

	settings := &GameSettingsFile{}
	if err := ReadJSON(path, settings); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &GameSettingsFile{}, nil
		}
		return nil, err
	}

	SanitizeGameSettings(settings)
	return settings, nil
}

// SaveGameSettings writes a game's settings file.
func SaveGameSettings(gameCRC string, settings *GameSettingsFile) error {
	path, err := GetGameSettingsPath(gameCRC)
	if err != nil {
		return err
	}
	return AtomicWriteJSON(path, settings)
}

// CheatEnabled reports whether the cheat with the given ID is turned on.
func (s *GameSettingsFile) CheatEnabled(id string) bool {
	return slices.Contains(s.Cheats.Enabled, id)
}

// SetCheatEnabled turns the cheat with the given ID on or off.
func (s *GameSettingsFile) SetCheatEnabled(id string, on bool) {
	idx := slices.Index(s.Cheats.Enabled, id)
	switch {
	case on && idx < 0:
		s.Cheats.Enabled = append(s.Cheats.Enabled, id)
	case !on && idx >= 0:
		s.Cheats.Enabled = slices.Delete(s.Cheats.Enabled, idx, idx+1)
	}
}
