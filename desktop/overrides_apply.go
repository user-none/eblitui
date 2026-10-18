// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"log"
	"slices"

	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/shader"
	"github.com/user-none/eblitui/desktop/storage"
)

// loadOverrides reads a game's overrides of global settings. A settings
// file that cannot be read gives no overrides.
func loadOverrides(gameCRC string) *storage.OverrideSettings {
	settings, err := storage.LoadGameSettings(gameCRC)
	if err != nil {
		log.Printf("Failed to load game settings for %s: %v", gameCRC, err)
		return &storage.OverrideSettings{}
	}
	return &settings.Overrides
}

// launchCoreOption returns the value a core option launches with: the
// game's override when it has one, otherwise the config value if set,
// otherwise the declared default
func launchCoreOption(opt coreif.CoreOption, global map[string]string, overrides *storage.OverrideSettings) string {
	if v, ok := overrides.CoreOption(opt); ok {
		return v
	}
	if v, ok := global[opt.Key]; ok {
		return v
	}
	return opt.Default
}

// launchGameShaders returns the game shaders a game launches with: the
// global game shaders with the game's preprocess shader overrides
// applied. Shaders the game turns on are added in registry order.
func launchGameShaders(global []string, overrides *storage.OverrideSettings) []string {
	out := make([]string, 0, len(global))
	for _, id := range global {
		if on, ok := overrides.Shader(id); ok && !on && shader.IsPreprocess(id) {
			continue
		}
		out = append(out, id)
	}
	for _, info := range shader.AvailableShaders {
		if !info.Preprocess || slices.Contains(out, info.ID) {
			continue
		}
		if on, ok := overrides.Shader(info.ID); ok && on {
			out = append(out, info.ID)
		}
	}
	return out
}
