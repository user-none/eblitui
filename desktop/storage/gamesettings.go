// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"errors"
	"os"
	"slices"

	"github.com/user-none/eblitui/coreif"
)

// GameSettingsFile holds a game's settings, stored per game in
// gamesettings/<key>.json. Each feature has its own section.
type GameSettingsFile struct {
	SaveStates SaveStateSettings `json:"saveStates"`
	Cheats     CheatSettings     `json:"cheats"`
	Overrides  OverrideSettings  `json:"overrides"`
}

// OverrideSettings holds a game's overrides of global settings. A setting
// with no entry uses the global value.
type OverrideSettings struct {
	CoreOptions map[string]string `json:"coreOptions,omitempty"` // core option key -> value
	Shaders     map[string]bool   `json:"shaders,omitempty"`     // shader ID -> on or off
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

// CoreOption returns the game's value for a core option and whether the
// game overrides it. Only per game Bool and Select options can be
// overridden, and only with a value the option accepts.
func (o *OverrideSettings) CoreOption(opt coreif.CoreOption) (string, bool) {
	if !opt.PerGame {
		return "", false
	}
	v, ok := o.CoreOptions[opt.Key]
	if !ok {
		return "", false
	}
	valid := false
	switch opt.Type {
	case coreif.CoreOptionBool:
		valid = v == "true" || v == "false"
	case coreif.CoreOptionSelect:
		valid = slices.Contains(opt.Values, v)
	}
	if !valid {
		return "", false
	}
	return v, true
}

// SetCoreOption overrides a core option with value
func (o *OverrideSettings) SetCoreOption(key, value string) {
	if o.CoreOptions == nil {
		o.CoreOptions = make(map[string]string)
	}
	o.CoreOptions[key] = value
}

// ClearCoreOption removes the core option's override
func (o *OverrideSettings) ClearCoreOption(key string) {
	delete(o.CoreOptions, key)
}

// Shader returns whether the game turns the shader on and whether the game
// overrides it
func (o *OverrideSettings) Shader(id string) (on bool, ok bool) {
	on, ok = o.Shaders[id]
	return on, ok
}

// SetShader overrides the shader on or off
func (o *OverrideSettings) SetShader(id string, on bool) {
	if o.Shaders == nil {
		o.Shaders = make(map[string]bool)
	}
	o.Shaders[id] = on
}

// ClearShader removes the shader's override
func (o *OverrideSettings) ClearShader(id string) {
	delete(o.Shaders, id)
}

// Reset removes every override
func (o *OverrideSettings) Reset() {
	o.CoreOptions = nil
	o.Shaders = nil
}
