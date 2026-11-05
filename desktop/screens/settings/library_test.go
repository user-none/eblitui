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
	if got := nav.transitions["lib-buttons"][types.DirDown]; got != (transition{"lib-artwork", types.NavIndexFirst}) {
		t.Errorf("down from the buttons = %+v, want the first artwork row", got)
	}
	if got := nav.transitions["lib-artwork"][types.DirUp]; got != (transition{"lib-buttons", types.NavIndexFirst}) {
		t.Errorf("up from the artwork rows = %+v, want the buttons", got)
	}
}
