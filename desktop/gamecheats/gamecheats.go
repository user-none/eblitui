// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package gamecheats loads a game's cheats: the database cheats from the
// emulator's cheats source and the custom cheats from its settings file.
// Both the Cheats page and the cheat engine use it, so they agree on
// which cheats exist and which are on.
package gamecheats

import (
	"github.com/user-none/eblitui/desktop/cheats"
	"github.com/user-none/eblitui/desktop/content"
	"github.com/user-none/eblitui/desktop/storage"
)

// Custom is a custom cheat with its parse result. Cheat is set, with the
// entry's name, when Err is nil.
type Custom struct {
	storage.CustomCheat
	Cheat cheats.Cheat
	Err   error
}

// Set is a game's cheats
type Set struct {
	Database []cheats.Cheat // cheats from the emulator's source that can be applied
	Custom   []Custom       // custom cheats in settings order
}

// Load gets a game's database cheats from provider and parses the custom
// cheats in settings. sys and bigEndian are the core's cheat system and
// memory byte order. A nil provider, or one with no entry for the game,
// means the game has no database cheats.
func Load(gameID string, provider content.CheatsSource, settings *storage.GameSettingsFile, sys cheats.System, bigEndian bool) Set {
	var s Set
	if provider != nil {
		if gc, ok := provider.GameCheats(gameID); ok {
			s.Database = ParseDatabase(gc.Cheats, sys, bigEndian)
		}
	}
	s.Custom = ParseCustom(settings.Cheats.Custom, sys, bigEndian)
	return s
}

// ParseDatabase parses the provider's cheats in order, keeping those that
// can be applied. Entries whose code does not parse are left out.
func ParseDatabase(list []content.GameCheat, sys cheats.System, bigEndian bool) []cheats.Cheat {
	var out []cheats.Cheat
	for _, e := range list {
		c, err := cheats.ParseCode(e.Code, sys, bigEndian)
		if err != nil {
			continue
		}
		c.Name = e.Name
		out = append(out, c)
	}
	return out
}

// ParseCustom parses custom cheats
func ParseCustom(list []storage.CustomCheat, sys cheats.System, bigEndian bool) []Custom {
	out := make([]Custom, len(list))
	for i, e := range list {
		c, err := cheats.ParseCode(e.Code, sys, bigEndian)
		c.Name = e.Name
		out[i] = Custom{CustomCheat: e, Cheat: c, Err: err}
	}
	return out
}

// Enabled returns the cheats that are on, custom cheats first, each ID
// once
func (s Set) Enabled(settings *storage.GameSettingsFile) []cheats.Cheat {
	var out []cheats.Cheat
	seen := make(map[string]bool)
	add := func(c cheats.Cheat) {
		if !seen[c.ID] && settings.CheatEnabled(c.ID) {
			seen[c.ID] = true
			out = append(out, c)
		}
	}
	for _, c := range s.Custom {
		if c.Err == nil {
			add(c.Cheat)
		}
	}
	for _, c := range s.Database {
		add(c)
	}
	return out
}

// NameOfID returns the name of the cheat with the given ID. The custom
// cheat at index skip is not considered; pass -1 to consider them all. ok
// is false when no cheat has the ID.
func (s Set) NameOfID(id string, skip int) (name string, ok bool) {
	for i, c := range s.Custom {
		if i != skip && c.Err == nil && c.Cheat.ID == id {
			return c.Name, true
		}
	}
	for _, c := range s.Database {
		if c.ID == id {
			return c.Name, true
		}
	}
	return "", false
}
