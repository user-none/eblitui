// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"testing"

	"github.com/user-none/eblitui/desktop/cheats"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

func TestDetailSetGameResetsPageAndScroll(t *testing.T) {
	s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, 0, true)
	s.pages.selected = 2
	s.scrollContainer = &widgets.ScrollView{}
	s.scrollTop = 120

	s.SetGame("00000000")

	if s.pages.selected != 0 {
		t.Errorf("selected page = %d, want 0", s.pages.selected)
	}
	if s.scrollContainer != nil {
		t.Error("scrollContainer not cleared")
	}
	if s.scrollTop != 0 {
		t.Errorf("scrollTop = %v, want 0", s.scrollTop)
	}
}

func TestDetailCheatsPageFollowsSupport(t *testing.T) {
	for _, sys := range []cheats.System{0, cheats.SystemSaturn} {
		s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, sys, true)

		found := false
		for _, sec := range s.pages.sections {
			if sec.focusKey == cheatsPageKey {
				found = true
			}
		}
		if want := sys != 0; found != want {
			t.Errorf("system %v: Cheats page listed = %v, want %v", sys, found, want)
		}
		if want := sys != 0; (s.cheatsPage != nil) != want {
			t.Errorf("system %v: cheats section created = %v, want %v", sys, s.cheatsPage != nil, want)
		}
	}
}

func TestDetailScreenshotsPageAlwaysListed(t *testing.T) {
	for _, sys := range []cheats.System{0, cheats.SystemSaturn} {
		s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, sys, true)
		if s.screenshotsPage == nil {
			t.Fatalf("system %v: screenshots section not created", sys)
		}
		last := s.pages.sections[len(s.pages.sections)-1]
		if last.focusKey != screenshotsPageKey || last.label != "Screenshots" {
			t.Errorf("system %v: last page = %q (%s), want Screenshots", sys, last.label, last.focusKey)
		}
	}
}

// widthCallback reports a window width; other callback methods are not
// used by the column sizing
type widthCallback struct {
	types.ScreenCallback
	width int
}

func (c *widthCallback) GetWindowWidth() int { return c.width }

func TestDetailColumnWidthFollowsWindow(t *testing.T) {
	screen := func(width int) *DetailScreen {
		return NewDetailScreen(&widthCallback{width: width}, storage.DefaultLibrary(), nil, 0, 0, true)
	}

	// Stacked below the side by side width: the column is the art width
	s := screen(900)
	if s.sideBySide() {
		t.Error("900 wide: side by side")
	}
	if got := s.columnWidth(); got != style.DetailColumnWidth {
		t.Errorf("900 wide: column = %d, want the minimum %d", got, style.DetailColumnWidth)
	}
	s = screen(1100)
	if got, want := s.columnWidth(), 1100/detailColumnFraction; got != want {
		t.Errorf("1100 wide: column = %d, want %d", got, want)
	}

	// Side by side from the threshold: sidebar, spacing, and art
	s = screen(style.Px(detailSideBySideWidth))
	if !s.sideBySide() {
		t.Error("at the threshold: not side by side")
	}
	sidebar := s.pages.sidebarWidth(style.SidebarMinWidth)
	if got, want := s.columnWidth(), sidebar+style.DefaultSpacing+s.artWidth(); got != want {
		t.Errorf("at the threshold: column = %d, want %d", got, want)
	}
	s = screen(4000)
	if got, want := s.artWidth(), style.Px(detailColumnMaxWidth); got != want {
		t.Errorf("4000 wide: art = %d, want the maximum %d", got, want)
	}
	if got, want := s.columnWidth(), sidebar+style.DefaultSpacing+style.Px(detailColumnMaxWidth); got != want {
		t.Errorf("4000 wide: column = %d, want %d", got, want)
	}
}

func TestDetailOnExitWithoutGame(t *testing.T) {
	s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, 0, true)
	s.OnExit() // nothing to release, must not panic
}

func TestDetailCheatsLabel(t *testing.T) {
	for n, want := range map[int]string{0: "Cheats", 1: "Cheats (1)", 2: "Cheats (2)"} {
		if got := cheatsLabel(n); got != want {
			t.Errorf("cheatsLabel(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestDetailUpdateCheatsLabel(t *testing.T) {
	s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, cheats.SystemSaturn, true)
	s.pages.sections[1].label = "Cheats (3)"

	// No game, so no cheats are on
	s.updateCheatsLabel()

	if got := s.pages.sections[1].label; got != "Cheats" {
		t.Errorf("label with no game = %q, want Cheats", got)
	}
}
