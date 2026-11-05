// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"sync"
)

// Config is the application configuration, safe for use from any goroutine.
// Its contents are only reachable through its methods: Values returns a copy
// and every change is a single locked operation. A configuration loaded from
// or created in config.json saves itself in the background after each
// change; one from DefaultConfig is never written.
type Config struct {
	mu    sync.RWMutex
	data  ConfigData
	saver *saver // nil when the configuration is not persisted
}

// DefaultConfig returns the default configuration, never written to disk.
// It is used where a configuration is needed but must not replace
// config.json, such as when config.json failed to load.
func DefaultConfig() *Config {
	return &Config{data: defaultConfigData()}
}

// newPersistedConfig returns a configuration holding data that saves itself
// to path after each change. config.json holds the RetroAchievements token,
// so it is readable only by its owner.
func newPersistedConfig(data ConfigData, path string) *Config {
	c := &Config{data: data}
	c.saver = newSaver(path, 0600, c.encode)
	return c
}

// encode returns the configuration as JSON.
func (c *Config) encode() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return json.MarshalIndent(&c.data, "", "  ")
}

// update applies fn under the lock and schedules a save. fn must not call
// any other Config method.
func (c *Config) update(fn func(*ConfigData)) {
	c.mu.Lock()
	fn(&c.data)
	c.mu.Unlock()
	if c.saver != nil {
		c.saver.request()
	}
}

// Flush waits until every change has been written. Call before exit.
func (c *Config) Flush() {
	if c.saver != nil {
		c.saver.flush()
	}
}

// LoadConfig loads the configuration from config.json.
// If the file doesn't exist, it returns the default configuration, saved on
// its first change. If the file is corrupted, it returns an error.
// Missing fields (absent from JSON) are silently defaulted.
func LoadConfig() (*Config, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return newPersistedConfig(defaultConfigData(), path), nil
	}

	jsonBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var data ConfigData
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Apply defaults only for fields that are absent from the file
	applyMissingDefaults(&data, detectPresentKeys(jsonBytes))

	// Apply any migration for older config versions, saving the result so
	// the migration runs once
	migrated := migrateConfig(&data)

	c := newPersistedConfig(data, path)
	if migrated {
		c.saver.request()
	}
	return c, nil
}

// ResetConfig replaces config.json with the defaults and returns a
// configuration saved to it.
func ResetConfig() (*Config, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}
	c := newPersistedConfig(defaultConfigData(), path)
	c.saver.request()
	return c, nil
}

// configVersion is the current config.json format version. Version 2 added
// the automatic update check setting.
const configVersion = 2

// migrateConfig updates a config from an older version to the current one
// and reports whether it changed anything. Version 0 (absent from the file)
// is treated as version 1. A version newer than the current one is left for
// validation to report.
func migrateConfig(config *ConfigData) bool {
	migrated := false
	if config.Version == 0 {
		config.Version = 1
		migrated = true
	}
	if config.Version == 1 {
		config.Updates.AutoCheck = true
		config.Version = 2
		migrated = true
	}
	return migrated
}

// CreateConfigIfMissing creates a default config.json if it doesn't exist
func CreateConfigIfMissing() error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		data := defaultConfigData()
		return atomicWriteJSON(path, &data, 0600)
	}

	return nil
}

// Validate checks all config fields, input included, against valid ranges
// and rules, and returns human-readable error descriptions. An empty slice
// means the config is valid.
func (c *Config) Validate(rules ConfigRules) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return validateConfig(&c.data, rules)
}

// Correct resets any invalid fields to their defaults and removes invalid
// input entries, keeping valid ones.
func (c *Config) Correct(rules ConfigRules) {
	c.update(func(d *ConfigData) { correctConfig(d, rules) })
}

// Values returns a copy of the configuration that shares nothing with it.
func (c *Config) Values() ConfigData {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data.clone()
}

// RetroAchievements returns a copy of the RetroAchievements settings. It
// copies only that section, for code that reads it every frame.
func (c *Config) RetroAchievements() RetroAchievementsConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data.RetroAchievements
}

// Shaders returns a copy of the shader lists. It copies only that section,
// for code that reads it every frame.
func (c *Config) Shaders() ShaderConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ShaderConfig{
		UIShaders:   slices.Clone(c.data.Shaders.UIShaders),
		GameShaders: slices.Clone(c.data.Shaders.GameShaders),
	}
}

// Input returns a copy of the input settings. It copies only that section,
// for code that reads it every frame.
func (c *Config) Input() InputConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data.Input.clone()
}

// clone returns a deep copy of d.
func (d *ConfigData) clone() ConfigData {
	c := *d
	if d.Window.X != nil {
		x := *d.Window.X
		c.Window.X = &x
	}
	if d.Window.Y != nil {
		y := *d.Window.Y
		c.Window.Y = &y
	}
	c.Shaders.UIShaders = slices.Clone(d.Shaders.UIShaders)
	c.Shaders.GameShaders = slices.Clone(d.Shaders.GameShaders)
	c.Input = d.Input.clone()
	c.CoreOptions = maps.Clone(d.CoreOptions)
	if d.BIOS != nil {
		c.BIOS = make(map[string]BIOSConfig, len(d.BIOS))
		for key, bc := range d.BIOS {
			bc.Files = maps.Clone(bc.Files)
			c.BIOS[key] = bc
		}
	}
	return c
}

// clone returns a deep copy of the input configuration.
func (in *InputConfig) clone() InputConfig {
	c := *in
	c.P1Keyboard = maps.Clone(in.P1Keyboard)
	c.Players = slices.Clone(in.Players)
	c.PadMappings = maps.Clone(in.PadMappings)
	if in.Profiles != nil {
		c.Profiles = make([]ControllerProfile, len(in.Profiles))
		for i, p := range in.Profiles {
			p.Bindings = maps.Clone(p.Bindings)
			c.Profiles[i] = p
		}
	}
	return c
}

// Appearance

// SetTheme sets the UI theme name.
func (c *Config) SetTheme(name string) {
	c.update(func(d *ConfigData) { d.Theme = name })
}

// Video

// SetAspectRatio sets the game aspect ratio mode.
func (c *Config) SetAspectRatio(mode string) {
	c.update(func(d *ConfigData) { d.Video.AspectRatio = mode })
}

// ToggleUIShader adds the shader to the UI shader list, or removes it if
// present, and reports whether it is now enabled.
func (c *Config) ToggleUIShader(id string) bool {
	var on bool
	c.update(func(d *ConfigData) { d.Shaders.UIShaders, on = toggleID(d.Shaders.UIShaders, id) })
	return on
}

// ToggleGameShader adds the shader to the game shader list, or removes it
// if present, and reports whether it is now enabled.
func (c *Config) ToggleGameShader(id string) bool {
	var on bool
	c.update(func(d *ConfigData) { d.Shaders.GameShaders, on = toggleID(d.Shaders.GameShaders, id) })
	return on
}

// toggleID removes every occurrence of id from list if present, otherwise
// appends it, and reports whether it is present afterwards.
func toggleID(list []string, id string) ([]string, bool) {
	if slices.Contains(list, id) {
		return slices.DeleteFunc(list, func(s string) bool { return s == id }), false
	}
	return append(list, id), true
}

// Audio

// SetVolume sets the playback volume.
func (c *Config) SetVolume(volume float64) {
	c.update(func(d *ConfigData) { d.Audio.Volume = volume })
}

// ToggleMuted flips audio muting and returns the new value.
func (c *Config) ToggleMuted() bool {
	var v bool
	c.update(func(d *ConfigData) { d.Audio.Muted = !d.Audio.Muted; v = d.Audio.Muted })
	return v
}

// ToggleFastForwardMute flips muting during fast-forward and returns the
// new value.
func (c *Config) ToggleFastForwardMute() bool {
	var v bool
	c.update(func(d *ConfigData) { d.Audio.FastForwardMute = !d.Audio.FastForwardMute; v = d.Audio.FastForwardMute })
	return v
}

// Window

// SetWindow records the window size, position and fullscreen state.
func (c *Config) SetWindow(w WindowConfig) {
	if w.X != nil {
		x := *w.X
		w.X = &x
	}
	if w.Y != nil {
		y := *w.Y
		w.Y = &y
	}
	c.update(func(d *ConfigData) { d.Window = w })
}

// SetFullscreen records whether the window is fullscreen.
func (c *Config) SetFullscreen(fullscreen bool) {
	c.update(func(d *ConfigData) { d.Window.Fullscreen = fullscreen })
}

// Library view

// SetViewMode sets the library view mode, "icon" or "list".
func (c *Config) SetViewMode(mode string) {
	c.update(func(d *ConfigData) { d.Library.ViewMode = mode })
}

// SetSortBy sets the library sort order.
func (c *Config) SetSortBy(sortBy string) {
	c.update(func(d *ConfigData) { d.Library.SortBy = sortBy })
}

// ToggleFavoritesFilter flips showing only favorites and returns the new
// value.
func (c *Config) ToggleFavoritesFilter() bool {
	var v bool
	c.update(func(d *ConfigData) {
		d.Library.FavoritesFilter = !d.Library.FavoritesFilter
		v = d.Library.FavoritesFilter
	})
	return v
}

// SetArtworkDownload sets which artwork a scan downloads.
func (c *Config) SetArtworkDownload(setting string) {
	c.update(func(d *ConfigData) { d.Library.ArtworkDownload = setting })
}

// SetBoxArtRegion sets the region whose artwork is shown first.
func (c *Config) SetBoxArtRegion(region string) {
	c.update(func(d *ConfigData) { d.Library.BoxArtRegion = region })
}

// Rewind

// ToggleRewind flips rewind and returns the new value.
func (c *Config) ToggleRewind() bool {
	var v bool
	c.update(func(d *ConfigData) { d.Rewind.Enabled = !d.Rewind.Enabled; v = d.Rewind.Enabled })
	return v
}

// SetRewindBufferSizeMB sets the rewind buffer size in MB.
func (c *Config) SetRewindBufferSizeMB(mb int) {
	c.update(func(d *ConfigData) { d.Rewind.BufferSizeMB = mb })
}

// SetRewindFrameStep sets how many frames pass between rewind captures.
func (c *Config) SetRewindFrameStep(step int) {
	c.update(func(d *ConfigData) { d.Rewind.FrameStep = step })
}

// Updates

// ToggleAutoCheck flips the automatic update check and returns the new
// value.
func (c *Config) ToggleAutoCheck() bool {
	var v bool
	c.update(func(d *ConfigData) { d.Updates.AutoCheck = !d.Updates.AutoCheck; v = d.Updates.AutoCheck })
	return v
}

// Core options and BIOS

// SetCoreOption sets a core option value.
func (c *Config) SetCoreOption(key, value string) {
	c.update(func(d *ConfigData) {
		if d.CoreOptions == nil {
			d.CoreOptions = make(map[string]string)
		}
		d.CoreOptions[key] = value
	})
}

// SetBIOSActive selects the BIOS variant used for a BIOS option; empty
// selects none.
func (c *Config) SetBIOSActive(key, label string) {
	c.update(func(d *ConfigData) {
		bc := d.BIOS[key]
		bc.Active = label
		if d.BIOS == nil {
			d.BIOS = make(map[string]BIOSConfig)
		}
		d.BIOS[key] = bc
	})
}

// SetBIOSFile records the file chosen for a BIOS option's variant.
func (c *Config) SetBIOSFile(key, label, path string) {
	c.update(func(d *ConfigData) {
		if d.BIOS == nil {
			d.BIOS = make(map[string]BIOSConfig)
		}
		bc := d.BIOS[key]
		bc.Files = maps.Clone(bc.Files)
		if bc.Files == nil {
			bc.Files = make(map[string]string)
		}
		bc.Files[label] = path
		d.BIOS[key] = bc
	})
}

// RemoveBIOSFile forgets the file chosen for a BIOS option's variant and,
// when that variant was active, selects none.
func (c *Config) RemoveBIOSFile(key, label string) {
	c.update(func(d *ConfigData) {
		if d.BIOS == nil {
			d.BIOS = make(map[string]BIOSConfig)
		}
		bc := d.BIOS[key]
		bc.Files = maps.Clone(bc.Files)
		delete(bc.Files, label)
		if bc.Active == label {
			bc.Active = ""
		}
		d.BIOS[key] = bc
	})
}

// RetroAchievements

// ToggleRetroAchievements flips RetroAchievements and returns the new value.
func (c *Config) ToggleRetroAchievements() bool {
	var v bool
	c.update(func(d *ConfigData) {
		d.RetroAchievements.Enabled = !d.RetroAchievements.Enabled
		v = d.RetroAchievements.Enabled
	})
	return v
}

// ToggleAchievementNotification flips the unlock popup and returns the new
// value.
func (c *Config) ToggleAchievementNotification() bool {
	var v bool
	c.update(func(d *ConfigData) {
		d.RetroAchievements.ShowNotification = !d.RetroAchievements.ShowNotification
		v = d.RetroAchievements.ShowNotification
	})
	return v
}

// ToggleAchievementSound flips the unlock sound and returns the new value.
func (c *Config) ToggleAchievementSound() bool {
	var v bool
	c.update(func(d *ConfigData) {
		d.RetroAchievements.UnlockSound = !d.RetroAchievements.UnlockSound
		v = d.RetroAchievements.UnlockSound
	})
	return v
}

// ToggleAchievementScreenshot flips the unlock screenshot and returns the
// new value.
func (c *Config) ToggleAchievementScreenshot() bool {
	var v bool
	c.update(func(d *ConfigData) {
		d.RetroAchievements.AutoScreenshot = !d.RetroAchievements.AutoScreenshot
		v = d.RetroAchievements.AutoScreenshot
	})
	return v
}

// ToggleEncoreMode flips encore mode and returns the new value.
func (c *Config) ToggleEncoreMode() bool {
	var v bool
	c.update(func(d *ConfigData) {
		d.RetroAchievements.EncoreMode = !d.RetroAchievements.EncoreMode
		v = d.RetroAchievements.EncoreMode
	})
	return v
}

// ToggleSpectatorMode flips spectator mode and returns the new value.
func (c *Config) ToggleSpectatorMode() bool {
	var v bool
	c.update(func(d *ConfigData) {
		d.RetroAchievements.SpectatorMode = !d.RetroAchievements.SpectatorMode
		v = d.RetroAchievements.SpectatorMode
	})
	return v
}

// SetRetroAchievementsLogin stores a successful login and turns
// RetroAchievements on.
func (c *Config) SetRetroAchievementsLogin(username, token string) {
	c.update(func(d *ConfigData) {
		d.RetroAchievements.Username = username
		d.RetroAchievements.Token = token
		d.RetroAchievements.Enabled = true
	})
}

// ClearRetroAchievementsToken forgets the stored token, keeping the
// username.
func (c *Config) ClearRetroAchievementsToken() {
	c.update(func(d *ConfigData) { d.RetroAchievements.Token = "" })
}

// ClearRetroAchievementsLogin forgets the stored username and token.
func (c *Config) ClearRetroAchievementsLogin() {
	c.update(func(d *ConfigData) {
		d.RetroAchievements.Username = ""
		d.RetroAchievements.Token = ""
	})
}

// Input

// SetKeyboardBinding sets player 1's keyboard key for a button; an empty
// key removes the override.
func (c *Config) SetKeyboardBinding(button, key string) {
	c.update(func(d *ConfigData) {
		if key == "" {
			delete(d.Input.P1Keyboard, button)
			if len(d.Input.P1Keyboard) == 0 {
				d.Input.P1Keyboard = nil
			}
			return
		}
		if d.Input.P1Keyboard == nil {
			d.Input.P1Keyboard = make(map[string]string)
		}
		d.Input.P1Keyboard[button] = key
	})
}

// ResetKeyboardBindings removes every player 1 keyboard override.
func (c *Config) ResetKeyboardBindings() {
	c.update(func(d *ConfigData) { d.Input.P1Keyboard = nil })
}

// SetPadMapping stores a generated controller mapping line for a SDL GUID.
func (c *Config) SetPadMapping(guid, line string) {
	c.update(func(d *ConfigData) {
		if d.Input.PadMappings == nil {
			d.Input.PadMappings = make(map[string]string)
		}
		d.Input.PadMappings[guid] = line
	})
}

// AddProfile adds a controller profile with a new unique ID and returns
// the ID. The profile's ID field is ignored.
func (c *Config) AddProfile(p ControllerProfile) string {
	var id string
	c.update(func(d *ConfigData) {
		id = d.Input.NewProfileID()
		p.ID = id
		p.Bindings = maps.Clone(p.Bindings)
		d.Input.Profiles = append(d.Input.Profiles, p)
	})
	return id
}

// DeleteProfile removes a controller profile and unassigns it from every
// player.
func (c *Config) DeleteProfile(id string) {
	c.update(func(d *ConfigData) {
		d.Input.Profiles = slices.DeleteFunc(d.Input.Profiles, func(p ControllerProfile) bool { return p.ID == id })
		if len(d.Input.Profiles) == 0 {
			d.Input.Profiles = nil
		}
		for i := range d.Input.Players {
			if d.Input.Players[i].Profile == id {
				d.Input.Players[i].Profile = ""
			}
		}
	})
}

// updateProfile applies fn to the profile with the given ID, if any.
func (c *Config) updateProfile(id string, fn func(*ControllerProfile)) {
	c.update(func(d *ConfigData) {
		if p := d.Input.ProfileByID(id); p != nil {
			fn(p)
		}
	})
}

// SetProfileName renames a controller profile.
func (c *Config) SetProfileName(id, name string) {
	c.updateProfile(id, func(p *ControllerProfile) { p.Name = name })
}

// SetProfileRumbleScale sets a controller profile's rumble intensity.
func (c *Config) SetProfileRumbleScale(id string, scale float64) {
	c.updateProfile(id, func(p *ControllerProfile) { p.RumbleScale = scale })
}

// SetProfileBinding sets a controller profile's pad button for a button;
// an empty pad button removes the override.
func (c *Config) SetProfileBinding(id, button, padButton string) {
	c.updateProfile(id, func(p *ControllerProfile) {
		if padButton == "" {
			delete(p.Bindings, button)
			if len(p.Bindings) == 0 {
				p.Bindings = nil
			}
			return
		}
		if p.Bindings == nil {
			p.Bindings = make(map[string]string)
		}
		p.Bindings[button] = padButton
	})
}

// ResetProfileBindings removes every binding override from a profile.
func (c *Config) ResetProfileBindings(id string) {
	c.updateProfile(id, func(p *ControllerProfile) { p.Bindings = nil })
}

// SetPlayerProfile assigns a controller profile to a player; an empty ID
// unassigns it.
func (c *Config) SetPlayerProfile(player int, id string) {
	if player < 0 || player >= MaxInputPlayers {
		return
	}
	c.update(func(d *ConfigData) {
		for len(d.Input.Players) <= player {
			d.Input.Players = append(d.Input.Players, PlayerConfig{})
		}
		d.Input.Players[player].Profile = id
	})
}
