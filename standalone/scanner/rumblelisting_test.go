package scanner

import (
	"testing"
)

func TestAddRumbleEntry(t *testing.T) {
	rl := newRumbleListing()

	// Valid entry with suffix
	rl.addRumbleEntry("Sonic the Hedgehog (Rumbles).cht")
	if _, ok := rl.Exact["Sonic the Hedgehog"]; !ok {
		t.Error("expected exact key 'Sonic the Hedgehog'")
	}
	if _, ok := rl.ByNorm["sonic the hedgehog"]; !ok {
		t.Error("expected normalized key 'sonic the hedgehog'")
	}

	// Entry with & in name
	rl.addRumbleEntry("Ghouls & Ghosts (Rumbles).cht")
	if _, ok := rl.Exact["Ghouls _ Ghosts"]; !ok {
		t.Error("expected exact key 'Ghouls _ Ghosts' (& replaced)")
	}
	if orig := rl.Exact["Ghouls _ Ghosts"]; orig != "Ghouls & Ghosts" {
		t.Errorf("expected original 'Ghouls & Ghosts', got %q", orig)
	}

	// Entry without rumble suffix is ignored
	rl.addRumbleEntry("Some Other File.cht")
	if len(rl.Exact) != 2 {
		t.Errorf("expected 2 exact entries, got %d", len(rl.Exact))
	}

	// Empty name is ignored
	rl.addRumbleEntry("")
	if len(rl.Exact) != 2 {
		t.Errorf("expected 2 exact entries after empty, got %d", len(rl.Exact))
	}
}

func TestResolveRumbleName(t *testing.T) {
	rl := newRumbleListing()
	rl.addRumbleEntry("Sonic the Hedgehog (Rumbles).cht")
	rl.addRumbleEntry("Ghouls & Ghosts (Rumbles).cht")
	rl.addRumbleEntry("Street Fighter II' - Special Champion Edition (Rumbles).cht")

	tests := []struct {
		name      string
		gameName  string
		wantOrig  string
		wantFound bool
	}{
		{
			name:      "exact match",
			gameName:  "Sonic the Hedgehog",
			wantOrig:  "Sonic the Hedgehog",
			wantFound: true,
		},
		{
			name:      "exact match with & replaced",
			gameName:  "Ghouls & Ghosts",
			wantOrig:  "Ghouls & Ghosts",
			wantFound: true,
		},
		{
			name:      "normalized match - case difference",
			gameName:  "sonic the hedgehog",
			wantOrig:  "Sonic the Hedgehog",
			wantFound: true,
		},
		{
			name:      "normalized match - punctuation stripped",
			gameName:  "Street Fighter II - Special Champion Edition",
			wantOrig:  "Street Fighter II' - Special Champion Edition",
			wantFound: true,
		},
		{
			name:      "no match",
			gameName:  "Nonexistent Game",
			wantOrig:  "",
			wantFound: false,
		},
		{
			name:      "empty game name",
			gameName:  "",
			wantOrig:  "",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig, found := resolveRumbleName(rl, tt.gameName)
			if found != tt.wantFound {
				t.Errorf("found = %v, want %v", found, tt.wantFound)
			}
			if orig != tt.wantOrig {
				t.Errorf("orig = %q, want %q", orig, tt.wantOrig)
			}
		})
	}

	// nil listing
	orig, found := resolveRumbleName(nil, "Sonic the Hedgehog")
	if found {
		t.Error("expected false for nil listing")
	}
	if orig != "" {
		t.Errorf("expected empty for nil listing, got %q", orig)
	}
}
