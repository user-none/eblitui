// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"slices"
	"strings"
	"testing"
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
