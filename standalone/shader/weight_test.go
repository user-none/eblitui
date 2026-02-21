//go:build !libretro

package shader

import (
	"sort"
	"testing"
)

func TestGetShaderWeight(t *testing.T) {
	tests := []struct {
		id       string
		expected int
	}{
		{"gamma", 900},
		{"ntsc", 850},
		{"rainbow", 845},
		{"colorbleed", 800},
		{"hsoft", 770},
		{"vblur", 760},
		{"monochrome", 700},
		{"sepia", 650},
		{"bloom", 550},
		{"halation", 500},
		{"scanlines", 400},
		{"interlace", 380},
		{"dotmatrix", 350},
		{"lcd", 300},
		{"vhs", 100},
		{"rollingband", 80},
		{"rfnoise", 50},
		{"crt", 25},
		{"xbr", 0},
		{"ghosting", 0},
		{"unknown", 0},
	}

	for _, tc := range tests {
		got := GetShaderWeight(tc.id)
		if got != tc.expected {
			t.Errorf("GetShaderWeight(%q) = %d, want %d", tc.id, got, tc.expected)
		}
	}
}

func TestShaderSortingByWeight(t *testing.T) {
	// Test that shaders with different weights sort by weight descending
	input := []string{"crt", "gamma", "scanlines", "bloom"}
	expected := []string{"gamma", "bloom", "scanlines", "crt"}

	sorted := make([]string, len(input))
	copy(sorted, input)
	sort.Slice(sorted, func(i, j int) bool {
		wi, wj := GetShaderWeight(sorted[i]), GetShaderWeight(sorted[j])
		if wi != wj {
			return wi > wj
		}
		return sorted[i] < sorted[j]
	})

	for i, id := range sorted {
		if id != expected[i] {
			t.Errorf("Position %d: got %q, want %q", i, id, expected[i])
		}
	}
}

func TestShaderSortingAlphabeticalTiebreaker(t *testing.T) {
	// Test that shaders with same weight sort alphabetically
	// xbr and ghosting both have weight 0
	input := []string{"ghosting", "xbr"}
	expected := []string{"ghosting", "xbr"} // alphabetical order

	sorted := make([]string, len(input))
	copy(sorted, input)
	sort.Slice(sorted, func(i, j int) bool {
		wi, wj := GetShaderWeight(sorted[i]), GetShaderWeight(sorted[j])
		if wi != wj {
			return wi > wj
		}
		return sorted[i] < sorted[j]
	})

	for i, id := range sorted {
		if id != expected[i] {
			t.Errorf("Position %d: got %q, want %q", i, id, expected[i])
		}
	}
}

func TestIsPreprocess(t *testing.T) {
	tests := []struct {
		id       string
		expected bool
	}{
		{"xbr", true},
		{"ghosting", true},
		{"crt", false},
		{"scanlines", false},
		{"bloom", false},
		{"unknown", false},
	}

	for _, tc := range tests {
		got := IsPreprocess(tc.id)
		if got != tc.expected {
			t.Errorf("IsPreprocess(%q) = %v, want %v", tc.id, got, tc.expected)
		}
	}
}

func TestGetShaderContext(t *testing.T) {
	tests := []struct {
		id       string
		expected EffectContext
	}{
		{"xbr", ContextGame},
		{"ghosting", ContextGame},
		{"crt", ContextAll},
		{"scanlines", ContextAll},
		{"bloom", ContextAll},
		{"unknown", 0},
	}

	for _, tc := range tests {
		got := GetShaderContext(tc.id)
		if got != tc.expected {
			t.Errorf("GetShaderContext(%q) = %d, want %d", tc.id, got, tc.expected)
		}
	}
}

func TestAvailableShadersFields(t *testing.T) {
	for _, s := range AvailableShaders {
		if s.Context == 0 {
			t.Errorf("AvailableShaders entry %q has zero Context", s.ID)
		}
		if s.Preprocess && s.Context != ContextGame {
			t.Errorf("AvailableShaders entry %q is Preprocess but Context is %d, want ContextGame", s.ID, s.Context)
		}
	}
}

func TestShaderSortingMixedWeights(t *testing.T) {
	// Test a more complex scenario with all shaders
	input := []string{"crt", "scanlines", "gamma", "ntsc", "colorbleed", "bloom", "halation", "vhs"}
	expected := []string{"gamma", "ntsc", "colorbleed", "bloom", "halation", "scanlines", "vhs", "crt"}

	sorted := make([]string, len(input))
	copy(sorted, input)
	sort.Slice(sorted, func(i, j int) bool {
		wi, wj := GetShaderWeight(sorted[i]), GetShaderWeight(sorted[j])
		if wi != wj {
			return wi > wj
		}
		return sorted[i] < sorted[j]
	})

	for i, id := range sorted {
		if id != expected[i] {
			t.Errorf("Position %d: got %q, want %q", i, id, expected[i])
		}
	}
}
