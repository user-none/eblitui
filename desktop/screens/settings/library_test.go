// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"slices"
	"testing"

	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/types"
)

func TestLibraryArtworkZone(t *testing.T) {
	l := NewLibrarySection(nil, storage.DefaultLibrary(), storage.DefaultConfig(), func() int { return 800 })
	nav := newNavRecorder()
	l.setupNavigation(nav, 0)

	want := []string{"lib-artwork-download", "lib-boxart-region"}
	if got := nav.zones["lib-artwork"]; !slices.Equal(got, want) {
		t.Errorf("artwork zone = %v, want %v", got, want)
	}
}

func TestLibraryNavNoFolders(t *testing.T) {
	l := NewLibrarySection(nil, storage.DefaultLibrary(), storage.DefaultConfig(), func() int { return 800 })
	nav := newNavRecorder()
	l.setupNavigation(nav, 0)

	if _, ok := nav.zones["lib-folders"]; ok {
		t.Errorf("folder zone registered with no folders")
	}
	if got := nav.transitions["lib-artwork"][types.DirDown]; got != (transition{"lib-buttons", types.NavIndexFirst}) {
		t.Errorf("down from the artwork rows = %+v, want the buttons", got)
	}
	if got := nav.transitions["lib-buttons"][types.DirUp]; got != (transition{"lib-artwork", types.NavIndexLast}) {
		t.Errorf("up from the buttons = %+v, want the last artwork row", got)
	}
}

func TestLibraryNavWithFolders(t *testing.T) {
	l := NewLibrarySection(nil, storage.DefaultLibrary(), storage.DefaultConfig(), func() int { return 800 })
	nav := newNavRecorder()
	l.setupNavigation(nav, 2)

	if got := nav.zones["lib-folders"]; !slices.Equal(got, []string{"folder-0", "folder-1"}) {
		t.Errorf("folder zone = %v", got)
	}
	cases := []struct {
		from string
		dir  int
		want transition
	}{
		{"lib-artwork", types.DirDown, transition{"lib-folders", types.NavIndexFirst}},
		{"lib-folders", types.DirUp, transition{"lib-artwork", types.NavIndexLast}},
		{"lib-folders", types.DirDown, transition{"lib-buttons", types.NavIndexFirst}},
		{"lib-buttons", types.DirUp, transition{"lib-folders", types.NavIndexLast}},
	}
	for _, c := range cases {
		if got := nav.transitions[c.from][c.dir]; got != c.want {
			t.Errorf("%s dir %d = %+v, want %+v", c.from, c.dir, got, c.want)
		}
	}
	if _, ok := nav.transitions["lib-buttons"][types.DirDown]; ok {
		t.Errorf("buttons have a down transition but are the last zone")
	}
}
