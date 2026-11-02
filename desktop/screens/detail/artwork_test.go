// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package detail

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/storage"
)

// artworkRecorder records the paths an Artwork page decodes, from the
// loading goroutine
type artworkRecorder struct {
	mu      sync.Mutex
	decoded []string
}

func (r *artworkRecorder) paths() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.decoded...)
}

// newTestArtworkSection creates a page over a temp artwork directory
// holding files with the given names. Decodes record the path they were
// asked for and fail.
func newTestArtworkSection(t *testing.T, names ...string) (*ArtworkSection, *shotCallback, *artworkRecorder, string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cb := &shotCallback{}
	rec := &artworkRecorder{}
	s := NewArtworkSection(cb)
	s.gameDir = func(string) (string, error) { return dir, nil }
	s.images.decode = func(path string) (*ebiten.Image, error) {
		rec.mu.Lock()
		rec.decoded = append(rec.decoded, path)
		rec.mu.Unlock()
		return nil, errTestDecode
	}
	s.SetGame(&storage.GameEntry{GameID: "T-1"})
	return s, cb, rec, dir
}

func TestArtworkLoadsPresentSides(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{"front and back", []string{"boxart.webp", "boxback.webp"}, []string{"boxart.webp", "boxback.webp"}},
		{"front only", []string{"boxart.webp"}, []string{"boxart.webp"}},
		{"back only", []string{"boxback.webp"}, []string{"boxback.webp"}},
		{"legacy front", []string{"boxart.png"}, []string{"boxart.png"}},
		{"none", nil, nil},
	}
	for _, tt := range tests {
		s, cb, rec, dir := newTestArtworkSection(t, tt.files...)
		s.load()
		waitRebuilds(t, cb, 1)
		got := rec.paths()
		if len(got) != len(tt.want) {
			t.Errorf("%s: decoded %v, want %v", tt.name, got, tt.want)
			continue
		}
		for i, name := range tt.want {
			if got[i] != filepath.Join(dir, name) {
				t.Errorf("%s: decoded[%d] = %s, want %s", tt.name, i, got[i], name)
			}
		}
		if _, loading := s.images.Images(); loading {
			t.Errorf("%s: still loading after the rebuild", tt.name)
		}
	}
}

func TestArtworkLoadsOncePerGame(t *testing.T) {
	s, cb, rec, _ := newTestArtworkSection(t, "boxart.webp", "boxback.webp")
	s.load()
	waitRebuilds(t, cb, 1)
	s.load()
	time.Sleep(10 * time.Millisecond)
	if got := len(rec.paths()); got != 2 {
		t.Fatalf("two loads decoded %d files, want 2", got)
	}

	s.SetGame(&storage.GameEntry{GameID: "T-2"})
	s.load()
	waitRebuilds(t, cb, 2)
	if got := len(rec.paths()); got != 4 {
		t.Errorf("after SetGame decoded %d files in all, want 4", got)
	}

	s.Release()
	s.load()
	waitRebuilds(t, cb, 3)
	if got := len(rec.paths()); got != 6 {
		t.Errorf("after Release decoded %d files in all, want 6", got)
	}
}

// newLoadedArtworkSection creates a page whose game has the given artwork
// files, decoded to small textures and loaded
func newLoadedArtworkSection(t *testing.T, names ...string) (*ArtworkSection, *shotCallback) {
	t.Helper()
	s, cb, _, _ := newTestArtworkSection(t, names...)
	s.images.decode = func(string) (*ebiten.Image, error) {
		return ebiten.NewImage(10, 20), nil
	}
	s.load()
	waitRebuilds(t, cb, 1)
	return s, cb
}

func TestArtworkCarouselMoves(t *testing.T) {
	s, cb := newLoadedArtworkSection(t, "boxart.webp", "boxback.webp")
	focus := newFakeFocus()
	s.Build(focus)
	if got := focus.zones[artworkZone]; len(got) != 1 || got[0] != artworkNextKey {
		t.Fatalf("first image zone = %v, want [%s]", got, artworkNextKey)
	}

	pressButton(t, focus.targets[artworkNextKey])
	if s.index != 1 || s.from != 0 {
		t.Errorf("after next: index %d from %d, want 1 from 0", s.index, s.from)
	}
	if focus.pending != artworkPrevKey {
		t.Errorf("pending focus %q, want %q at the last image", focus.pending, artworkPrevKey)
	}
	if cb.rebuildCount() != 2 {
		t.Errorf("rebuilds %d, want 2", cb.rebuildCount())
	}

	focus = newFakeFocus()
	s.Build(focus)
	if got := focus.zones[artworkZone]; len(got) != 1 || got[0] != artworkPrevKey {
		t.Errorf("last image zone = %v, want [%s]", got, artworkPrevKey)
	}
	s.move(1, 2)
	if s.index != 1 || cb.rebuildCount() != 2 {
		t.Errorf("move past the end changed index to %d with %d rebuilds", s.index, cb.rebuildCount())
	}

	pressButton(t, focus.targets[artworkPrevKey])
	if s.index != 0 || s.from != 1 || focus.pending != artworkNextKey {
		t.Errorf("after prev: index %d from %d focus %q", s.index, s.from, focus.pending)
	}

	s.move(1, 2)
	s.SetGame(&storage.GameEntry{GameID: "T-2"})
	if s.index != 0 || s.from != 0 {
		t.Errorf("SetGame left index %d from %d", s.index, s.from)
	}
}

func TestArtworkSingleImageNoZone(t *testing.T) {
	s, _ := newLoadedArtworkSection(t, "boxback.webp")
	focus := newFakeFocus()
	s.Build(focus)
	if len(s.NavZones()) != 0 || len(focus.targets) != 0 {
		t.Errorf("single image: zones %v, targets %d", s.NavZones(), len(focus.targets))
	}
}

func TestArtworkNilGame(t *testing.T) {
	s, cb, rec, _ := newTestArtworkSection(t, "boxart.webp")
	s.SetGame(nil)
	s.load()
	time.Sleep(10 * time.Millisecond)
	if got := rec.paths(); len(got) != 0 || cb.rebuildCount() != 0 {
		t.Errorf("nil game decoded %v with %d rebuilds", got, cb.rebuildCount())
	}
}
