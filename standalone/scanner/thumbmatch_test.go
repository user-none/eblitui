package scanner

import (
	"testing"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "Sonic the Hedgehog", "sonic the hedgehog"},
		{"ampersand", "Knuckles & Sonic", "knuckles sonic"},
		{"punctuation", "R-Type III: The Third Lightning", "rtype iii the third lightning"},
		{"multiple spaces", "Game  Name   Here", "game name here"},
		{"leading trailing spaces", "  Sonic  ", "sonic"},
		{"parenthetical", "Game (USA)", "game usa"},
		{"empty", "", ""},
		{"numbers", "Street Fighter 2", "street fighter 2"},
		{"special chars", "Game!@#$%Name", "gamename"},
		{"mixed", "Gley-Lancer (Japan).png", "gleylancer japanpng"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeName(tc.input)
			if got != tc.expected {
				t.Errorf("normalizeName(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected int
	}{
		{"identical", "abc", "abc", 0},
		{"empty both", "", "", 0},
		{"empty a", "", "abc", 3},
		{"empty b", "abc", "", 3},
		{"one insert", "abc", "abcd", 1},
		{"one delete", "abcd", "abc", 1},
		{"one substitute", "abc", "axc", 1},
		{"completely different", "abc", "xyz", 3},
		{"kitten sitting", "kitten", "sitting", 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := levenshtein(tc.a, tc.b)
			if got != tc.expected {
				t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.expected)
			}
		})
	}
}

func TestSimilarityScore(t *testing.T) {
	tests := []struct {
		name   string
		a      string
		b      string
		minVal float64
		maxVal float64
	}{
		{"identical", "sonic", "sonic", 1.0, 1.0},
		{"unnormalized case not equal", "sonic", "SONIC", 0.0, 0.01},
		{"both empty", "", "", 1.0, 1.0},
		{"completely different", "abc", "xyz", 0.0, 0.01},
		{"similar", "gley lancer", "gleylancer", 0.85, 1.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := similarityScore(tc.a, tc.b)
			if got < tc.minVal || got > tc.maxVal {
				t.Errorf("similarityScore(%q, %q) = %f, want [%f, %f]",
					tc.a, tc.b, got, tc.minVal, tc.maxVal)
			}
		})
	}
}

func TestParseName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantBase   string
		wantGroups []string
	}{
		{
			"no parens",
			"Sonic the Hedgehog",
			"Sonic the Hedgehog",
			nil,
		},
		{
			"single group",
			"Game (USA)",
			"Game",
			[]string{"usa"},
		},
		{
			"multiple groups",
			"High Seas Havoc (USA) (En,Ja)",
			"High Seas Havoc",
			[]string{"usa", "enja"},
		},
		{
			"empty string",
			"",
			"",
			nil,
		},
		{
			"unclosed paren",
			"Game (USA",
			"Game",
			nil,
		},
		{
			"brackets stripped",
			"Game (USA) [!] [b1]",
			"Game",
			[]string{"usa"},
		},
		{
			"mixed brackets and parens",
			"Game [!] (USA) [b1] (En,Ja)",
			"Game",
			[]string{"usa", "enja"},
		},
		{
			"paren at start",
			"(USA) Extra",
			"",
			[]string{"usa"},
		},
		{
			"three groups",
			"Game (Japan) (En,Ja) (Beta 2)",
			"Game",
			[]string{"japan", "enja", "beta 2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotBase, gotGroups := parseName(tc.input)
			if gotBase != tc.wantBase {
				t.Errorf("parseName(%q) base = %q, want %q", tc.input, gotBase, tc.wantBase)
			}
			if len(gotGroups) != len(tc.wantGroups) {
				t.Fatalf("parseName(%q) groups len = %d, want %d: got %v",
					tc.input, len(gotGroups), len(tc.wantGroups), gotGroups)
			}
			for i, g := range gotGroups {
				if g != tc.wantGroups[i] {
					t.Errorf("parseName(%q) group[%d] = %q, want %q",
						tc.input, i, g, tc.wantGroups[i])
				}
			}
		})
	}
}

func TestScoreMatch(t *testing.T) {
	tests := []struct {
		name       string
		gameGroups []string
		candGroups []string
		wantScore  int
	}{
		{
			"exact single",
			[]string{"usa"},
			[]string{"usa"},
			2001,
		},
		{
			"exact multiple",
			[]string{"usa", "enja"},
			[]string{"usa", "enja"},
			4002,
		},
		{
			"prefix match",
			[]string{"beta 2"},
			[]string{"beta"},
			1001,
		},
		{
			"no overlap",
			[]string{"japan"},
			[]string{"usa"},
			1,
		},
		{
			"bare candidate",
			[]string{"usa"},
			nil,
			0,
		},
		{
			"mixed exact and prefix",
			[]string{"japan", "enja", "beta 2"},
			[]string{"japan", "beta"},
			3002,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := scoreMatch(tc.gameGroups, tc.candGroups)
			if got != tc.wantScore {
				t.Errorf("scoreMatch(%v, %v) = %d, want %d",
					tc.gameGroups, tc.candGroups, got, tc.wantScore)
			}
		})
	}
}

func TestResolveArtworkNameForType(t *testing.T) {
	listing := newThumbnailListing()
	listing.addEntry("Named_Boxarts", "Sonic The Hedgehog (USA, Europe)")
	listing.addEntry("Named_Boxarts", "Alex Kidd in Miracle World (USA, Europe)")
	listing.addEntry("Named_Titles", "Sonic The Hedgehog (USA, Europe)")

	t.Run("exact match single type", func(t *testing.T) {
		fileName, found := resolveArtworkNameForType(listing, "Named_Boxarts", "Sonic The Hedgehog (USA, Europe)")
		if !found {
			t.Fatal("expected match")
		}
		if fileName != "Sonic The Hedgehog (USA, Europe)" {
			t.Errorf("fileName = %q, want original name", fileName)
		}
	})

	t.Run("no match wrong type", func(t *testing.T) {
		_, found := resolveArtworkNameForType(listing, "Named_Titles", "Alex Kidd in Miracle World (USA, Europe)")
		if found {
			t.Error("expected no match for type without entry")
		}
	})

	t.Run("fuzzy match single type", func(t *testing.T) {
		listing2 := newThumbnailListing()
		listing2.addEntry("Named_Boxarts", "Advanced Busterhawk Gleylancer (Japan)")

		fileName, found := resolveArtworkNameForType(listing2, "Named_Boxarts", "Advanced Busterhawk Gley Lancer (Japan)")
		if !found {
			t.Fatal("expected fuzzy match")
		}
		if fileName != "Advanced Busterhawk Gleylancer (Japan)" {
			t.Errorf("fileName = %q, want original", fileName)
		}
	})

	t.Run("base plus group single type", func(t *testing.T) {
		listing3 := newThumbnailListing()
		listing3.addEntry("Named_Snaps", "High Seas Havoc (USA)")

		fileName, found := resolveArtworkNameForType(listing3, "Named_Snaps", "High Seas Havoc (USA) (En,Ja)")
		if !found {
			t.Fatal("expected base+group match")
		}
		if fileName != "High Seas Havoc (USA)" {
			t.Errorf("fileName = %q, want High Seas Havoc (USA)", fileName)
		}
	})

	t.Run("nil listing", func(t *testing.T) {
		_, found := resolveArtworkNameForType(nil, "Named_Boxarts", "Sonic")
		if found {
			t.Error("expected no match for nil listing")
		}
	})

	t.Run("empty name", func(t *testing.T) {
		_, found := resolveArtworkNameForType(listing, "Named_Boxarts", "")
		if found {
			t.Error("expected no match for empty name")
		}
	})

	t.Run("missing artType", func(t *testing.T) {
		_, found := resolveArtworkNameForType(listing, "Named_Logos", "Sonic The Hedgehog (USA, Europe)")
		if found {
			t.Error("expected no match for missing artType")
		}
	})
}
