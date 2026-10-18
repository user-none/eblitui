// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package detail

import (
	"slices"
	"testing"

	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/storage"
)

var testSettingsOptions = []coreif.CoreOption{
	{Key: "upscale", Type: coreif.CoreOptionSelect, Values: []string{"1x", "2x"}, Category: coreif.CoreOptionCategoryVideo, PerGame: true},
	{Key: "renderer", Type: coreif.CoreOptionSelect, Values: []string{"GPU"}, Category: coreif.CoreOptionCategoryVideo},
	{Key: "level", Type: coreif.CoreOptionRange, Max: 9, Category: coreif.CoreOptionCategoryAudio, PerGame: true},
	{Key: "flag", Type: coreif.CoreOptionBool, Category: coreif.CoreOptionCategoryCore, PerGame: true},
}

// newTestSettingsSection creates a section for a game with the given
// settings, counting saves
func newTestSettingsSection(t *testing.T, settings *storage.GameSettingsFile) (*SettingsSection, *fakeFocus, *int) {
	t.Helper()
	s := NewSettingsSection(&fakeCallback{}, testSettingsOptions, func() int { return 800 })
	saves := 0
	s.save = func(string, *storage.GameSettingsFile) error {
		saves++
		return nil
	}
	s.SetGame(&storage.GameEntry{CRC32: "12345678"}, settings)
	focus := newFakeFocus()
	s.Build(focus)
	return s, focus, &saves
}

func TestSettingsListsPerGameRows(t *testing.T) {
	_, focus, _ := newTestSettingsSection(t, &storage.GameSettingsFile{})

	want := []string{
		"game-settings-opt-upscale",
		"game-settings-shader-xbr",
		"game-settings-shader-ghosting",
		"game-settings-shader-overscan",
		"game-settings-opt-flag",
		resetKey,
	}
	if got := focus.zones[gameSettingsZone]; !slices.Equal(got, want) {
		t.Errorf("zone keys = %v, want %v", got, want)
	}
}

func TestSettingsCycleSelect(t *testing.T) {
	s, focus, saves := newTestSettingsSection(t, &storage.GameSettingsFile{})
	key := "game-settings-opt-upscale"

	for _, want := range []struct{ label, value string }{{"1x", "1x"}, {"2x", "2x"}, {inheritLabel, ""}} {
		pressButton(t, focus.targets[key])
		focus = newFakeFocus()
		s.Build(focus)
		if got := buttonLabel(t, focus.targets[key]); got != want.label {
			t.Errorf("label = %q, want %q", got, want.label)
		}
		got, ok := s.gameSettings.Overrides.CoreOptions["upscale"]
		if ok != (want.value != "") || got != want.value {
			t.Errorf("stored = %q %v, want %q", got, ok, want.value)
		}
	}
	if *saves != 3 {
		t.Errorf("saves = %d, want 3", *saves)
	}
}

func TestSettingsCycleBool(t *testing.T) {
	s, focus, _ := newTestSettingsSection(t, &storage.GameSettingsFile{})
	key := "game-settings-opt-flag"

	for _, want := range []struct{ label, value string }{{"On", "true"}, {"Off", "false"}, {inheritLabel, ""}} {
		pressButton(t, focus.targets[key])
		focus = newFakeFocus()
		s.Build(focus)
		if got := buttonLabel(t, focus.targets[key]); got != want.label {
			t.Errorf("label = %q, want %q", got, want.label)
		}
		got, ok := s.gameSettings.Overrides.CoreOptions["flag"]
		if ok != (want.value != "") || got != want.value {
			t.Errorf("stored = %q %v, want %q", got, ok, want.value)
		}
	}
}

func TestSettingsCycleShader(t *testing.T) {
	s, focus, _ := newTestSettingsSection(t, &storage.GameSettingsFile{})
	key := "game-settings-shader-overscan"

	for _, want := range []struct {
		label string
		on    bool
		set   bool
	}{{"On", true, true}, {"Off", false, true}, {inheritLabel, false, false}} {
		pressButton(t, focus.targets[key])
		focus = newFakeFocus()
		s.Build(focus)
		if got := buttonLabel(t, focus.targets[key]); got != want.label {
			t.Errorf("label = %q, want %q", got, want.label)
		}
		if on, ok := s.gameSettings.Overrides.Shader("overscan"); on != want.on || ok != want.set {
			t.Errorf("stored = %v %v, want %v %v", on, ok, want.on, want.set)
		}
	}
}

func TestSettingsInvalidValueShowsInherit(t *testing.T) {
	settings := &storage.GameSettingsFile{}
	settings.Overrides.SetCoreOption("upscale", "3x")
	_, focus, _ := newTestSettingsSection(t, settings)

	if got := buttonLabel(t, focus.targets["game-settings-opt-upscale"]); got != inheritLabel {
		t.Errorf("label = %q, want %q", got, inheritLabel)
	}
}

func TestSettingsResetDisabledWithoutOverrides(t *testing.T) {
	// An entry the page does not list is not an override it shows
	settings := &storage.GameSettingsFile{}
	settings.Overrides.SetCoreOption("renderer", "GPU")
	s, focus, saves := newTestSettingsSection(t, settings)

	pressButton(t, focus.targets[resetKey])
	if *saves != 0 {
		t.Errorf("saves = %d, want 0", *saves)
	}
	if s.gameSettings.Overrides.CoreOptions["renderer"] != "GPU" {
		t.Error("disabled reset removed an entry")
	}
}

func TestSettingsResetRemovesOverrides(t *testing.T) {
	settings := &storage.GameSettingsFile{}
	settings.Overrides.SetCoreOption("upscale", "2x")
	settings.Overrides.SetShader("xbr", false)
	s, focus, saves := newTestSettingsSection(t, settings)

	pressButton(t, focus.targets[resetKey])
	if *saves != 1 {
		t.Errorf("saves = %d, want 1", *saves)
	}
	if o := s.gameSettings.Overrides; o.CoreOptions != nil || o.Shaders != nil {
		t.Errorf("overrides = %+v, want none", o)
	}
}
