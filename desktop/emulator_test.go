// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"testing"

	"github.com/user-none/eblitui/desktop/cheats"
	"github.com/user-none/eblitui/desktop/storage"
)

func TestScanSources(t *testing.T) {
	emu := Emulator{
		Artwork: &ArtworkConfig{URL: "https://artwork.example.com/", Keys: []string{"ed25519:KEY"}},
	}

	src := (&App{emu: emu}).scanSources()
	if src.ArtworkURL != "https://artwork.example.com/" || len(src.ArtworkKeys) != 1 || src.ArtworkKeys[0] != "ed25519:KEY" {
		t.Errorf("artwork source = %+v", src)
	}

	if src := (&App{emu: Emulator{}}).scanSources(); src.ArtworkURL != "" || src.ArtworkKeys != nil {
		t.Errorf("no artwork config: %+v", src)
	}

	// The artwork download setting is read from the config each time, so a
	// change applies to the next scan
	a := &App{emu: emu, config: storage.DefaultConfig()}
	if src := a.scanSources(); src.ArtworkDownload != storage.ArtworkDownloadAll {
		t.Errorf("default artwork download = %q, want %q", src.ArtworkDownload, storage.ArtworkDownloadAll)
	}
	a.config.Library.ArtworkDownload = storage.ArtworkDownloadFront
	if src := a.scanSources(); src.ArtworkDownload != storage.ArtworkDownloadFront {
		t.Errorf("changed artwork download = %q, want %q", src.ArtworkDownload, storage.ArtworkDownloadFront)
	}
}

func TestCheatSystem(t *testing.T) {
	tests := []struct {
		name   string
		format string
		memory bool
		want   cheats.System
	}{
		{"saturn", "saturn", true, cheats.SystemSaturn},
		{"no memory", "saturn", false, 0},
		{"no format", "", true, 0},
		{"unknown format", "unknown", true, 0},
	}
	for _, tt := range tests {
		a := &App{emu: Emulator{CheatFormat: tt.format}, hasMemory: tt.memory}
		if got := a.cheatSystem(); got != tt.want {
			t.Errorf("%s: cheatSystem = %v, want %v", tt.name, got, tt.want)
		}
	}
}
