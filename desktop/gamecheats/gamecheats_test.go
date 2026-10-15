// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package gamecheats

import (
	"testing"

	"github.com/user-none/eblitui/desktop/cheats"
	"github.com/user-none/eblitui/desktop/storage"
)

func testSet() Set {
	return Set{
		Database: []cheats.Cheat{
			{Name: "DB Lives", ID: "160EF1000000"},
			{Name: "DB Time", ID: "160C52380000"},
		},
		Custom: ParseCustom([]storage.CustomCheat{
			{Name: "My Lives", Code: "160EF100 0000"},
			{Name: "Broken", Code: "not a code"},
			{Name: "Mine", Code: "160EF102+0063"},
		}, cheats.SystemSaturn, true),
	}
}

func TestParseCustom(t *testing.T) {
	s := testSet()
	if s.Custom[0].Err != nil || s.Custom[0].Cheat.Name != "My Lives" || s.Custom[0].Cheat.ID != "160EF1000000" {
		t.Errorf("custom 0 = %+v", s.Custom[0])
	}
	if s.Custom[1].Err == nil {
		t.Error("invalid custom code parsed without an error")
	}
	if s.Custom[2].Code != "160EF102+0063" {
		t.Errorf("custom 2 code = %q, want it as entered", s.Custom[2].Code)
	}
}

func TestEnabled(t *testing.T) {
	s := testSet()
	settings := &storage.GameSettingsFile{}
	settings.SetCheatEnabled("160EF1000000", true) // custom and database share it
	settings.SetCheatEnabled("160C52380000", true)
	settings.SetCheatEnabled("GONE", true)

	got := s.Enabled(settings)
	if len(got) != 2 || got[0].Name != "My Lives" || got[1].Name != "DB Time" {
		t.Errorf("enabled = %+v, want My Lives then DB Time", got)
	}
}

func TestNameOfID(t *testing.T) {
	s := testSet()
	if name, ok := s.NameOfID("160EF1000000", -1); !ok || name != "My Lives" {
		t.Errorf("custom first: %q %v", name, ok)
	}
	if name, ok := s.NameOfID("160EF1000000", 0); !ok || name != "DB Lives" {
		t.Errorf("skipping the custom cheat: %q %v", name, ok)
	}
	if _, ok := s.NameOfID("160EF1020063", 2); ok {
		t.Error("found the skipped cheat")
	}
	if _, ok := s.NameOfID("NONE", -1); ok {
		t.Error("found a missing ID")
	}
}
