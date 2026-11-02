// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package gamecheats

import (
	"testing"

	"github.com/user-none/eblitui/desktop/cheats"
	"github.com/user-none/eblitui/desktop/content"
	"github.com/user-none/eblitui/desktop/storage"
)

// fakeProvider serves cheats for the serials in its map.
type fakeProvider map[string]content.GameCheats

func (f fakeProvider) GameCheats(serial string) (content.GameCheats, bool) {
	gc, ok := f[serial]
	return gc, ok
}

func TestLoadFromProvider(t *testing.T) {
	p := fakeProvider{"T-1234G": {Serial: "T-1234G", MasterCode: "F6000914 C305", Cheats: []content.GameCheat{
		{Name: "Lives", Code: "160EF100 0009"},
		{Name: "Broken", Code: "not a code"},
		{Name: "Time", Code: "160C5238 0000\n360C523A 0063"},
	}}}
	settings := &storage.GameSettingsFile{}

	s := Load("T-1234G", p, settings, cheats.SystemSaturn, true)
	if len(s.Database) != 2 {
		t.Fatalf("database = %+v, want the 2 cheats that parse", s.Database)
	}
	if s.Database[0].Name != "Lives" || s.Database[0].ID != "160EF1000009" || s.Database[0].Code != "160EF100 0009" {
		t.Errorf("database 0 = %+v", s.Database[0])
	}
	if s.Database[1].Name != "Time" || s.Database[1].ID != "160C52380000360C523A0063" || len(s.Database[1].Ops) != 2 {
		t.Errorf("database 1 = %+v", s.Database[1])
	}

	if s := Load("T-9999G", p, settings, cheats.SystemSaturn, true); s.Database != nil {
		t.Errorf("game without an entry: database = %+v, want none", s.Database)
	}
	if s := Load("T-1234G", nil, settings, cheats.SystemSaturn, true); s.Database != nil {
		t.Errorf("nil provider: database = %+v, want none", s.Database)
	}
}

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
