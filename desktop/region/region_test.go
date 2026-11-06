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
		{[]string{"Germany", "USA", "Europe"}, []string{US, EU}},
		{[]string{"Japan", "France"}, []string{EU, JP}},
		{[]string{"Japan", "USA", "Europe"}, []string{US, EU, JP}},
		{[]string{"USA", "Brazil", "Korea"}, []string{US}},
		{[]string{"Korea", "Taiwan"}, nil},
		{nil, nil},
	} {
		if got := Codes(tc.names); !slices.Equal(got, tc.want) {
			t.Errorf("Codes(%v) = %v, want %v", tc.names, got, tc.want)
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
