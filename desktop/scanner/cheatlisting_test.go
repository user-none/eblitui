// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package scanner

import (
	"slices"
	"testing"
)

func cheatURL(name string) string {
	return "https://example.invalid/cht/" + name
}

func newTestCheatListing(names ...string) *CheatListing {
	cl := newCheatListing()
	for _, n := range names {
		cl.addCheatEntry(n, cheatURL(n))
	}
	return cl
}

func TestAddCheatEntrySkips(t *testing.T) {
	cl := newTestCheatListing(
		"Astal (USA).cht",
		"Sonic the Hedgehog (Rumbles).cht",
		"readme.txt",
		"",
	)
	cl.addCheatEntry("Missing URL (USA).cht", "")

	if len(cl.urls) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(cl.urls), cl.urls)
	}
	if _, ok := cl.urls["Astal (USA)"]; !ok {
		t.Error("expected entry for 'Astal (USA)'")
	}
}

func TestResolveCheatName(t *testing.T) {
	cl := newTestCheatListing(
		"Astal (USA).cht",
		"Road & Track Presents - The Need for Speed (USA) (En,De).cht",
		"Panzer Dragoon (Japan) (RetroArch).cht",
		"Sega Rally Championship (USA) (RetroArch).cht",
		"Sega Rally Championship (USA) (RetroArch) (Rumble).cht",
		"Street Fighter Collection (USA) (Disc 1).cht",
		"Bug Too! (USA).cht",
	)

	tests := []struct {
		game string
		want string // file base name, or "" for no match
	}{
		// Exact
		{"Astal (USA)", "Astal (USA)"},
		{"Street Fighter Collection (USA) (Disc 1)", "Street Fighter Collection (USA) (Disc 1)"},
		// Normalized: case differs
		{"road & track presents - the need for speed (usa) (en,de)", "Road & Track Presents - The Need for Speed (USA) (En,De)"},
		// Extra tags on the file name
		{"Panzer Dragoon (Japan)", "Panzer Dragoon (Japan) (RetroArch)"},
		// Shortest file with extra tags wins
		{"Sega Rally Championship (USA)", "Sega Rally Championship (USA) (RetroArch)"},
		// No match: only a disc the game is not, a different title, or a
		// different region
		{"Street Fighter Collection (USA) (Disc 2)", ""},
		{"Bug! (USA)", ""},
		{"Astal (Japan)", ""},
		{"", ""},
	}
	for _, tc := range tests {
		got, found := resolveCheatName(cl, tc.game)
		if tc.want == "" {
			if found {
				t.Errorf("%q: expected no match, got %q", tc.game, got)
			}
			continue
		}
		if !found || got != tc.want {
			t.Errorf("%q: got %q (found %v), want %q", tc.game, got, found, tc.want)
		}
		if u := cl.urls[got]; u != cheatURL(tc.want+".cht") {
			t.Errorf("%q: URL %q, want %q", tc.game, u, cheatURL(tc.want+".cht"))
		}
	}
}

func TestCheatNameCandidates(t *testing.T) {
	tests := []struct {
		game string
		want []string
	}{
		{"Astal (USA)", []string{"Astal (USA)"}},
		{"Astal (USA) (2S)", []string{"Astal (USA) (2S)", "Astal (USA)"}},
		{"Sonic R (USA, Brazil)", []string{"Sonic R (USA, Brazil)", "Sonic R (USA)", "Sonic R (Brazil)"}},
		{
			"Magic Knight Rayearth (Japan) (Genteiban) (2M, 3M)",
			[]string{"Magic Knight Rayearth (Japan) (Genteiban) (2M, 3M)", "Magic Knight Rayearth (Japan) (Genteiban)"},
		},
		{
			"Example (USA, Europe) (5S)",
			[]string{"Example (USA, Europe) (5S)", "Example (USA, Europe)", "Example (USA) (5S)", "Example (Europe) (5S)", "Example (USA)", "Example (Europe)"},
		},
		// Revision and language tags are kept
		{"Vatlva (Japan) (Rev A)", []string{"Vatlva (Japan) (Rev A)"}},
		{"Panzer Dragoon (Europe) (En,Fr,De,Es,It)", []string{"Panzer Dragoon (Europe) (En,Fr,De,Es,It)"}},
		// Not split into groups: returned alone
		{"No Groups", []string{"No Groups"}},
		{"Broken (USA", []string{"Broken (USA"}},
		{"Text (USA) after", []string{"Text (USA) after"}},
	}
	for _, tc := range tests {
		if got := cheatNameCandidates(tc.game); !slices.Equal(got, tc.want) {
			t.Errorf("%q:\n got  %q\n want %q", tc.game, got, tc.want)
		}
	}
}

func TestResolveCheatNamePressingAndRegionList(t *testing.T) {
	cl := newTestCheatListing(
		"Astal (USA).cht",
		"Panzer Dragoon (USA).cht",
		"Sonic R (USA).cht",
		"Nights into Dreams... (USA).cht",
		"Vatlva (Japan).cht",
		"Bubble Bobble also featuring Rainbow Islands (USA).cht",
	)

	tests := []struct {
		game string
		want string // file base name, or "" for no match
	}{
		{"Astal (USA) (2S)", "Astal (USA)"},
		{"Panzer Dragoon (USA) (5S)", "Panzer Dragoon (USA)"},
		{"Sonic R (USA, Brazil)", "Sonic R (USA)"},
		{"NiGHTS into Dreams... (USA, Brazil)", "Nights into Dreams... (USA)"},
		// Revision tags are not dropped
		{"Vatlva (Japan) (Rev A)", ""},
		// Region still has to match
		{"Bubble Bobble also featuring Rainbow Islands (Europe)", ""},
		{"Sonic R (Europe, Brazil)", ""},
	}
	for _, tc := range tests {
		got, found := resolveCheatName(cl, tc.game)
		if tc.want == "" {
			if found {
				t.Errorf("%q: expected no match, got %q", tc.game, got)
			}
			continue
		}
		if !found || got != tc.want {
			t.Errorf("%q: got %q (found %v), want %q", tc.game, got, found, tc.want)
		}
	}
}

func TestResolveCheatNameNilListing(t *testing.T) {
	if _, found := resolveCheatName(nil, "Astal (USA)"); found {
		t.Error("expected no match for nil listing")
	}
}
