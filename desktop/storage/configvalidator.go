// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"encoding/json"
	"fmt"
)

// detectPresentKeys unmarshals JSON bytes to determine which config keys
// are explicitly present in the file. Returns a flat set of dotted-path keys
// (e.g., "audio.volume", "window.width"). Only checks non-omitempty fields
// that have validation rules.
func detectPresentKeys(jsonBytes []byte) map[string]bool {
	present := make(map[string]bool)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(jsonBytes, &raw); err != nil {
		return present
	}

	// Top-level keys
	topKeys := []string{"version", "theme", "fontSize"}
	for _, k := range topKeys {
		if _, ok := raw[k]; ok {
			present[k] = true
		}
	}

	// Nested: audio
	if audioRaw, ok := raw["audio"]; ok {
		var audio map[string]json.RawMessage
		if json.Unmarshal(audioRaw, &audio) == nil {
			if _, ok := audio["volume"]; ok {
				present["audio.volume"] = true
			}
			if _, ok := audio["fastForwardMute"]; ok {
				present["audio.fastForwardMute"] = true
			}
		}
	}

	// Nested: window
	if windowRaw, ok := raw["window"]; ok {
		var window map[string]json.RawMessage
		if json.Unmarshal(windowRaw, &window) == nil {
			if _, ok := window["width"]; ok {
				present["window.width"] = true
			}
			if _, ok := window["height"]; ok {
				present["window.height"] = true
			}
		}
	}

	// Nested: library
	if libraryRaw, ok := raw["library"]; ok {
		var library map[string]json.RawMessage
		if json.Unmarshal(libraryRaw, &library) == nil {
			if _, ok := library["viewMode"]; ok {
				present["library.viewMode"] = true
			}
			if _, ok := library["sortBy"]; ok {
				present["library.sortBy"] = true
			}
		}
	}

	// Nested: video
	if videoRaw, ok := raw["video"]; ok {
		var video map[string]json.RawMessage
		if json.Unmarshal(videoRaw, &video) == nil {
			if _, ok := video["aspectRatio"]; ok {
				present["video.aspectRatio"] = true
			}
		}
	}

	// Nested: rewind
	if rewindRaw, ok := raw["rewind"]; ok {
		var rewind map[string]json.RawMessage
		if json.Unmarshal(rewindRaw, &rewind) == nil {
			if _, ok := rewind["bufferSizeMB"]; ok {
				present["rewind.bufferSizeMB"] = true
			}
			if _, ok := rewind["frameStep"]; ok {
				present["rewind.frameStep"] = true
			}
		}
	}

	return present
}

// ApplyMissingDefaults sets default values for config fields that are absent
// from the JSON file. This replaces ensureConfigDefaults with key-presence
// awareness: only truly missing fields get defaults, preserving intentional
// zero values (e.g., volume=0).
func ApplyMissingDefaults(config *Config, presentKeys map[string]bool) {
	defaults := DefaultConfig()

	if !presentKeys["version"] {
		config.Version = defaults.Version
	}
	if !presentKeys["theme"] {
		config.Theme = defaults.Theme
	}
	if !presentKeys["fontSize"] {
		config.FontSize = defaults.FontSize
	}
	if !presentKeys["video.aspectRatio"] {
		config.Video.AspectRatio = defaults.Video.AspectRatio
	}
	if !presentKeys["audio.volume"] {
		config.Audio.Volume = defaults.Audio.Volume
	}
	if !presentKeys["audio.fastForwardMute"] {
		config.Audio.FastForwardMute = defaults.Audio.FastForwardMute
	}
	if !presentKeys["window.width"] {
		config.Window.Width = defaults.Window.Width
	}
	if !presentKeys["window.height"] {
		config.Window.Height = defaults.Window.Height
	}
	if !presentKeys["library.viewMode"] {
		config.Library.ViewMode = defaults.Library.ViewMode
	}
	if !presentKeys["library.sortBy"] {
		config.Library.SortBy = defaults.Library.SortBy
	}
	if !presentKeys["rewind.bufferSizeMB"] {
		config.Rewind.BufferSizeMB = defaults.Rewind.BufferSizeMB
	}
	if !presentKeys["rewind.frameStep"] {
		config.Rewind.FrameStep = defaults.Rewind.FrameStep
	}
}

// ValidateConfig checks all config fields against valid ranges and returns
// human-readable error descriptions. An empty slice means the config is valid.
// validThemes should be the list of known theme names.
func ValidateConfig(config *Config, validThemes []string) []string {
	var errors []string

	// version
	if config.Version != 1 {
		errors = append(errors, fmt.Sprintf("version: %d (valid: 1)", config.Version))
	}

	// theme
	themeValid := false
	for _, t := range validThemes {
		if config.Theme == t {
			themeValid = true
			break
		}
	}
	if !themeValid {
		errors = append(errors, fmt.Sprintf("theme: %q (valid: %v)", config.Theme, validThemes))
	}

	// fontSize
	fontSizeValid := false
	for _, p := range FontSizePresets {
		if config.FontSize == p {
			fontSizeValid = true
			break
		}
	}
	if !fontSizeValid {
		errors = append(errors, fmt.Sprintf("fontSize: %d (valid: %v)", config.FontSize, FontSizePresets))
	}

	// video.aspectRatio
	aspectRatioValid := false
	for _, ar := range ValidAspectRatios {
		if config.Video.AspectRatio == ar {
			aspectRatioValid = true
			break
		}
	}
	if !aspectRatioValid {
		errors = append(errors, fmt.Sprintf("video.aspectRatio: %q (valid: %v)", config.Video.AspectRatio, ValidAspectRatios))
	}

	// audio.volume
	if config.Audio.Volume < 0 || config.Audio.Volume > 2.0 {
		errors = append(errors, fmt.Sprintf("audio.volume: %.2f (valid: 0.0-2.0)", config.Audio.Volume))
	}

	// window.width
	if config.Window.Width < 900 {
		errors = append(errors, fmt.Sprintf("window.width: %d (valid: >= 900)", config.Window.Width))
	}

	// window.height
	if config.Window.Height < 650 {
		errors = append(errors, fmt.Sprintf("window.height: %d (valid: >= 650)", config.Window.Height))
	}

	// library.viewMode
	if config.Library.ViewMode != "icon" && config.Library.ViewMode != "list" {
		errors = append(errors, fmt.Sprintf("library.viewMode: %q (valid: \"icon\", \"list\")", config.Library.ViewMode))
	}

	// library.sortBy
	if config.Library.SortBy != "title" && config.Library.SortBy != "lastPlayed" && config.Library.SortBy != "playTime" {
		errors = append(errors, fmt.Sprintf("library.sortBy: %q (valid: \"title\", \"lastPlayed\", \"playTime\")", config.Library.SortBy))
	}

	// rewind.bufferSizeMB
	if config.Rewind.BufferSizeMB < 10 || config.Rewind.BufferSizeMB > 400 {
		errors = append(errors, fmt.Sprintf("rewind.bufferSizeMB: %d (valid: 10-400)", config.Rewind.BufferSizeMB))
	}

	// rewind.frameStep
	if config.Rewind.FrameStep < 1 || config.Rewind.FrameStep > 30 {
		errors = append(errors, fmt.Sprintf("rewind.frameStep: %d (valid: 1-30)", config.Rewind.FrameStep))
	}

	return errors
}

// MaxInputPlayers is the maximum number of player slots stored in config.
const MaxInputPlayers = 2

// ValidateInputConfig checks input config overrides using the provided
// validators. Returns error descriptions for invalid entries.
// isValidKey checks if a key name is valid, isValidPad checks if a pad name is valid.
func ValidateInputConfig(config *Config, isValidKey, isValidPad func(string) bool) []string {
	var errors []string
	for btn, keyName := range config.Input.P1Keyboard {
		if !isValidKey(keyName) {
			errors = append(errors, fmt.Sprintf("input.p1Keyboard[%q]: invalid key %q", btn, keyName))
		}
	}

	seenIDs := make(map[string]bool)
	seenNames := make(map[string]bool)
	for i := range config.Input.Profiles {
		p := &config.Input.Profiles[i]
		if p.ID == "" || p.SDLID == "" || p.Controller == "" || p.Name == "" {
			errors = append(errors, fmt.Sprintf("input.profiles[%d]: missing id, sdlId, controller, or name", i))
			continue
		}
		if seenIDs[p.ID] {
			errors = append(errors, fmt.Sprintf("input.profiles[%d]: duplicate id %q", i, p.ID))
		}
		seenIDs[p.ID] = true
		nameKey := p.SDLID + "\x00" + p.Controller + "\x00" + p.Name
		if seenNames[nameKey] {
			errors = append(errors, fmt.Sprintf("input.profiles[%d]: duplicate name %q", i, p.Name))
		}
		seenNames[nameKey] = true
		for btn, padName := range p.Bindings {
			if !isValidPad(padName) {
				errors = append(errors, fmt.Sprintf("input.profiles[%d].bindings[%q]: invalid pad %q", i, btn, padName))
			}
		}
	}

	if len(config.Input.Players) > MaxInputPlayers {
		errors = append(errors, fmt.Sprintf("input.players: %d entries (valid: <= %d)", len(config.Input.Players), MaxInputPlayers))
	}
	for i, pl := range config.Input.Players {
		if i >= MaxInputPlayers {
			break
		}
		if pl.Profile != "" && !seenIDs[pl.Profile] {
			errors = append(errors, fmt.Sprintf("input.players[%d]: unknown profile %q", i, pl.Profile))
		}
	}

	for guid, line := range config.Input.PadMappings {
		if !ValidPadMapping(guid, line) {
			errors = append(errors, fmt.Sprintf("input.padMappings[%q]: malformed mapping", guid))
		}
	}
	return errors
}

// CorrectInputConfig removes invalid entries from input override maps and
// profile/player configuration. Profiles missing identity fields are dropped,
// duplicate IDs and duplicate names per controller model are dropped, invalid
// bindings are removed, the players list is clamped, and dangling player
// profile references are cleared.
// isValidKey checks if a key name is valid, isValidPad checks if a pad name is valid.
func CorrectInputConfig(config *Config, isValidKey, isValidPad func(string) bool) {
	for btn, keyName := range config.Input.P1Keyboard {
		if !isValidKey(keyName) {
			delete(config.Input.P1Keyboard, btn)
		}
	}

	seenIDs := make(map[string]bool)
	seenNames := make(map[string]bool)
	kept := config.Input.Profiles[:0]
	for i := range config.Input.Profiles {
		p := config.Input.Profiles[i]
		if p.ID == "" || p.SDLID == "" || p.Controller == "" || p.Name == "" {
			continue
		}
		if seenIDs[p.ID] {
			continue
		}
		nameKey := p.SDLID + "\x00" + p.Controller + "\x00" + p.Name
		if seenNames[nameKey] {
			continue
		}
		seenIDs[p.ID] = true
		seenNames[nameKey] = true
		for btn, padName := range p.Bindings {
			if !isValidPad(padName) {
				delete(p.Bindings, btn)
			}
		}
		if len(p.Bindings) == 0 {
			p.Bindings = nil
		}
		kept = append(kept, p)
	}
	config.Input.Profiles = kept
	if len(config.Input.Profiles) == 0 {
		config.Input.Profiles = nil
	}

	if len(config.Input.Players) > MaxInputPlayers {
		config.Input.Players = config.Input.Players[:MaxInputPlayers]
	}
	for i := range config.Input.Players {
		if config.Input.Players[i].Profile != "" && !seenIDs[config.Input.Players[i].Profile] {
			config.Input.Players[i].Profile = ""
		}
	}

	for guid, line := range config.Input.PadMappings {
		if !ValidPadMapping(guid, line) {
			delete(config.Input.PadMappings, guid)
		}
	}
	if len(config.Input.PadMappings) == 0 {
		config.Input.PadMappings = nil
	}
}

// CorrectConfig resets any invalid fields to their defaults from DefaultConfig().
// Valid fields are preserved. validThemes should be the list of known theme names.
func CorrectConfig(config *Config, validThemes []string) *Config {
	defaults := DefaultConfig()

	// version
	if config.Version != 1 {
		config.Version = defaults.Version
	}

	// theme
	themeValid := false
	for _, t := range validThemes {
		if config.Theme == t {
			themeValid = true
			break
		}
	}
	if !themeValid {
		config.Theme = defaults.Theme
	}

	// fontSize
	fontSizeValid := false
	for _, p := range FontSizePresets {
		if config.FontSize == p {
			fontSizeValid = true
			break
		}
	}
	if !fontSizeValid {
		config.FontSize = defaults.FontSize
	}

	// video.aspectRatio
	aspectRatioValid := false
	for _, ar := range ValidAspectRatios {
		if config.Video.AspectRatio == ar {
			aspectRatioValid = true
			break
		}
	}
	if !aspectRatioValid {
		config.Video.AspectRatio = defaults.Video.AspectRatio
	}

	// audio.volume
	if config.Audio.Volume < 0 || config.Audio.Volume > 2.0 {
		config.Audio.Volume = defaults.Audio.Volume
	}

	// window.width
	if config.Window.Width < 900 {
		config.Window.Width = defaults.Window.Width
	}

	// window.height
	if config.Window.Height < 650 {
		config.Window.Height = defaults.Window.Height
	}

	// library.viewMode
	if config.Library.ViewMode != "icon" && config.Library.ViewMode != "list" {
		config.Library.ViewMode = defaults.Library.ViewMode
	}

	// library.sortBy
	if config.Library.SortBy != "title" && config.Library.SortBy != "lastPlayed" && config.Library.SortBy != "playTime" {
		config.Library.SortBy = defaults.Library.SortBy
	}

	// rewind.bufferSizeMB
	if config.Rewind.BufferSizeMB < 10 || config.Rewind.BufferSizeMB > 400 {
		config.Rewind.BufferSizeMB = defaults.Rewind.BufferSizeMB
	}

	// rewind.frameStep
	if config.Rewind.FrameStep < 1 || config.Rewind.FrameStep > 30 {
		config.Rewind.FrameStep = defaults.Rewind.FrameStep
	}

	return config
}
