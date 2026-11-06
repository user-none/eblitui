// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package region

import (
	"slices"
	"testing"
)

func TestCode(t *testing.T) {
	for name, want := range map[string]string{
		"USA": US, "Germany": EU, "Australia": EU, "Japan": JP,
		"Korea": "", "Taiwan": "", "Brazil": "", "Nowhere": "", "": "",
	} {
		if got := Code(name); got != want {
			t.Errorf("Code(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestCodes(t *testing.T) {
	for _, tc := range []struct {
		names []string
		want  []string
	}{
		{[]string{"Japan"}, []string{JP}},
		{[]string{"Germany", "USA", "Europe"}, []string{EU, US}},
		{[]string{"Japan", "France"}, []string{JP, EU}},
		{[]string{"Japan", "USA", "Europe"}, []string{JP, US, EU}},
		{[]string{"USA", "Brazil", "Europe"}, []string{US, EU}},
		{[]string{"USA", "Brazil", "Korea"}, []string{US}},
		{[]string{"Korea", "Taiwan"}, nil},
		{nil, nil},
	} {
		if got := Codes(tc.names); !slices.Equal(got, tc.want) {
			t.Errorf("Codes(%v) = %v, want %v", tc.names, got, tc.want)
		}
	}
}

func TestAbbreviationsAndNames(t *testing.T) {
	for _, tc := range []struct {
		codes       []string
		abbr, names string
	}{
		{[]string{US, EU}, "US, EU", "USA, Europe"},
		{[]string{JP}, "JP", "Japan"},
		{[]string{JP, US, EU}, "JP, US, EU", "Japan, USA, Europe"},
		{nil, "", ""},
	} {
		if got := Abbreviations(tc.codes); got != tc.abbr {
			t.Errorf("Abbreviations(%v) = %q, want %q", tc.codes, got, tc.abbr)
		}
		if got := Names(tc.codes); got != tc.names {
			t.Errorf("Names(%v) = %q, want %q", tc.codes, got, tc.names)
		}
	}
}

func TestSupported(t *testing.T) {
	for _, c := range All {
		if !Supported(c) {
			t.Errorf("Supported(%q) = false, want true", c)
		}
	}
	for _, c := range []string{"kr", "tw", "br", "", "US"} {
		if Supported(c) {
			t.Errorf("Supported(%q) = true, want false", c)
		}
	}
}

func TestDisplayName(t *testing.T) {
	for code, want := range map[string]string{US: "USA", EU: "Europe", JP: "Japan", "kr": "", "": ""} {
		if got := DisplayName(code); got != want {
			t.Errorf("DisplayName(%q) = %q, want %q", code, got, want)
		}
	}
}
