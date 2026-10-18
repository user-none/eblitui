// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/user-none/eblitui/coreif"
)

func TestSetCheatEnabled(t *testing.T) {
	s := &GameSettingsFile{}

	s.SetCheatEnabled("A", true)
	s.SetCheatEnabled("B", true)
	s.SetCheatEnabled("A", true)
	if !slices.Equal(s.Cheats.Enabled, []string{"A", "B"}) {
		t.Errorf("enabled = %v, want [A B]", s.Cheats.Enabled)
	}
	if !s.CheatEnabled("A") || !s.CheatEnabled("B") || s.CheatEnabled("C") {
		t.Errorf("CheatEnabled wrong for %v", s.Cheats.Enabled)
	}

	s.SetCheatEnabled("A", false)
	s.SetCheatEnabled("C", false)
	if !slices.Equal(s.Cheats.Enabled, []string{"B"}) {
		t.Errorf("enabled = %v, want [B]", s.Cheats.Enabled)
	}
}

func TestSanitizeGameSettings(t *testing.T) {
	s := &GameSettingsFile{Cheats: CheatSettings{Enabled: []string{"A", "", "B", "A", "B", "C"}}}

	SanitizeGameSettings(s)

	if !slices.Equal(s.Cheats.Enabled, []string{"A", "B", "C"}) {
		t.Errorf("enabled = %v, want [A B C]", s.Cheats.Enabled)
	}
}

func TestSanitizeGameSettingsCustomCheats(t *testing.T) {
	ok := CustomCheat{Name: "Lives", Code: "160EF100 0000\n160C5238 0000"}
	s := &GameSettingsFile{Cheats: CheatSettings{Custom: []CustomCheat{
		ok,
		{Name: " ", Code: "160EF100 0000"},
		{Name: "No code", Code: "\n"},
		{Name: strings.Repeat("n", MaxCustomCheatNameLen+1), Code: "160EF100 0000"},
		{Name: "Long", Code: strings.Repeat("1", MaxCustomCheatCodeLen+1)},
		{Name: "Rows", Code: strings.Repeat("\n", MaxCustomCheatCodeRows) + "1"},
		{Name: strings.Repeat("n", MaxCustomCheatNameLen), Code: "160EF100 0001"},
	}}}

	SanitizeGameSettings(s)

	if len(s.Cheats.Custom) != 2 || s.Cheats.Custom[0] != ok || s.Cheats.Custom[1].Code != "160EF100 0001" {
		t.Errorf("custom = %+v, want the valid entries only", s.Cheats.Custom)
	}
}

func TestSanitizeGameSettingsCustomCheatCap(t *testing.T) {
	s := &GameSettingsFile{}
	for range MaxCustomCheats + 5 {
		s.Cheats.Custom = append(s.Cheats.Custom, CustomCheat{Name: "x", Code: "1"})
	}
	SanitizeGameSettings(s)
	if len(s.Cheats.Custom) != MaxCustomCheats {
		t.Errorf("kept %d custom cheats, want %d", len(s.Cheats.Custom), MaxCustomCheats)
	}
}

func TestSanitizeGameSettingsSaveSlot(t *testing.T) {
	for slot, want := range map[int]int{-1: 0, 0: 0, 5: 5, 9: 9, 10: 0, 99: 0} {
		s := &GameSettingsFile{SaveStates: SaveStateSettings{Slot: slot}}
		SanitizeGameSettings(s)
		if s.SaveStates.Slot != want {
			t.Errorf("slot %d: got %d, want %d", slot, s.SaveStates.Slot, want)
		}
	}
}

func TestOverrideCoreOption(t *testing.T) {
	sel := coreif.CoreOption{Key: "upscale", Type: coreif.CoreOptionSelect, Values: []string{"1x", "2x"}, PerGame: true}
	boolOpt := coreif.CoreOption{Key: "flag", Type: coreif.CoreOptionBool, PerGame: true}
	rng := coreif.CoreOption{Key: "level", Type: coreif.CoreOptionRange, Min: 0, Max: 9, PerGame: true}
	global := coreif.CoreOption{Key: "renderer", Type: coreif.CoreOptionSelect, Values: []string{"GPU"}}

	tests := []struct {
		name   string
		opt    coreif.CoreOption
		value  string
		want   string
		wantOK bool
	}{
		{"select value", sel, "2x", "2x", true},
		{"select invalid", sel, "3x", "", false},
		{"bool true", boolOpt, "true", "true", true},
		{"bool false", boolOpt, "false", "false", true},
		{"bool invalid", boolOpt, "on", "", false},
		{"range", rng, "5", "", false},
		{"not per game", global, "GPU", "", false},
	}
	for _, tt := range tests {
		o := OverrideSettings{CoreOptions: map[string]string{tt.opt.Key: tt.value}}
		got, ok := o.CoreOption(tt.opt)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("%s: got %q %v, want %q %v", tt.name, got, ok, tt.want, tt.wantOK)
		}
	}

	var empty OverrideSettings
	if _, ok := empty.CoreOption(sel); ok {
		t.Error("no entry reported as an override")
	}
}

func TestOverrideSetAndClear(t *testing.T) {
	var o OverrideSettings

	o.SetCoreOption("upscale", "4x")
	o.SetShader("xbr", false)
	o.SetShader("overscan", true)
	if o.CoreOptions["upscale"] != "4x" {
		t.Errorf("core options = %v, want upscale 4x", o.CoreOptions)
	}
	if on, ok := o.Shader("xbr"); !ok || on {
		t.Errorf("xbr = %v %v, want off override", on, ok)
	}
	if on, ok := o.Shader("overscan"); !ok || !on {
		t.Errorf("overscan = %v %v, want on override", on, ok)
	}
	if _, ok := o.Shader("ghosting"); ok {
		t.Error("ghosting reported as an override")
	}

	o.ClearCoreOption("upscale")
	o.ClearShader("xbr")
	if _, ok := o.CoreOptions["upscale"]; ok {
		t.Error("cleared upscale still overridden")
	}
	if _, ok := o.Shader("xbr"); ok {
		t.Error("cleared xbr still overridden")
	}

	o.SetCoreOption("upscale", "2x")
	o.Reset()
	if o.CoreOptions != nil || o.Shaders != nil {
		t.Errorf("after reset = %+v, want no overrides", o)
	}
}

func TestOverridesJSONOmitsEmpty(t *testing.T) {
	data, err := json.Marshal(&GameSettingsFile{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "coreOptions") || strings.Contains(string(data), "shaders") {
		t.Errorf("empty overrides written: %s", data)
	}

	s := &GameSettingsFile{}
	s.Overrides.SetCoreOption("upscale", "8x")
	s.Overrides.SetShader("ghosting", true)
	data, err = json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var back GameSettingsFile
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if back.Overrides.CoreOptions["upscale"] != "8x" || !back.Overrides.Shaders["ghosting"] {
		t.Errorf("round trip = %+v, want upscale 8x and ghosting on", back.Overrides)
	}
}
