// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ebitenui/ebitenui/event"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/cheats"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

func TestDetailSetGameResetsPageAndScroll(t *testing.T) {
	s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, 0, true, nil)
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
		s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, sys, true, nil)

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
		s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, sys, true, nil)
		if s.screenshotsPage == nil {
			t.Fatalf("system %v: screenshots section not created", sys)
		}
		last := s.pages.sections[len(s.pages.sections)-1]
		if last.focusKey != screenshotsPageKey || last.label != "Screenshots" {
			t.Errorf("system %v: last page = %q (%s), want Screenshots", sys, last.label, last.focusKey)
		}
	}
}

func TestDetailSettingsPageBeforeScreenshots(t *testing.T) {
	for _, sys := range []cheats.System{0, cheats.SystemSaturn} {
		s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, sys, true, nil)
		n := len(s.pages.sections)
		if got := s.pages.sections[n-2]; got.focusKey != settingsPageKey || got.label != "Settings" {
			t.Errorf("system %v: page before Screenshots = %q (%s), want Settings", sys, got.label, got.focusKey)
		}
	}
}

// pagesCallback reports a window width and counts rebuild requests; other
// callback methods are not used by the pages
type pagesCallback struct {
	types.ScreenCallback
	rebuilds int
}

func (c *pagesCallback) GetWindowWidth() int { return 1200 }
func (c *pagesCallback) RequestRebuild()     { c.rebuilds++ }

// pressFocusTarget clicks the button registered under key and runs its
// handlers
func pressFocusTarget(t *testing.T, s *DetailScreen, key string) {
	t.Helper()
	btn, ok := s.focusTargets[key].(*widget.Button)
	if !ok {
		t.Fatalf("%s is not a registered button", key)
	}
	btn.Click()
	event.ExecuteDeferred()
}

func TestDetailPagesShareGameSettings(t *testing.T) {
	useTempDataDir(t)
	if err := storage.SaveGameSettings("aaaa", &storage.GameSettingsFile{
		Cheats: storage.CheatSettings{Custom: []storage.CustomCheat{{Name: "Lives", Code: "160EF100 0000"}}},
	}); err != nil {
		t.Fatalf("SaveGameSettings: %v", err)
	}
	lib := storage.DefaultLibrary()
	lib.AddGame(&storage.GameEntry{CRC32: "aaaa", File: filepath.Join(t.TempDir(), "a.cue")})
	upscale := coreif.CoreOption{Key: "upscale", Type: coreif.CoreOptionSelect, Values: []string{"1x", "2x"}, Category: coreif.CoreOptionCategoryVideo, PerGame: true}
	s := NewDetailScreen(&pagesCallback{}, lib, nil, 0, cheats.SystemSaturn, true, []coreif.CoreOption{upscale})
	s.SetGame("aaaa")

	// A Settings page change followed by a Cheats page change keeps both
	s.settingsPage.Build(&s.BaseScreen)
	pressFocusTarget(t, s, "game-settings-opt-upscale")
	s.cheatsPage.Build(&s.BaseScreen)
	pressFocusTarget(t, s, "custom-0-toggle")

	saved, err := storage.LoadGameSettings("aaaa")
	if err != nil {
		t.Fatalf("LoadGameSettings: %v", err)
	}
	if saved.Overrides.CoreOptions["upscale"] != "1x" {
		t.Errorf("overrides = %+v, want upscale 1x", saved.Overrides)
	}
	if len(saved.Cheats.Enabled) != 1 {
		t.Errorf("enabled cheats = %v, want the custom cheat", saved.Cheats.Enabled)
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
		return NewDetailScreen(&widthCallback{width: width}, storage.DefaultLibrary(), nil, 0, 0, true, nil)
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

// removeCallback records the callbacks the remove actions use; other
// callback methods are not used by them
type removeCallback struct {
	types.ScreenCallback
	rebuilds      int
	toLibrary     int
	notifications []string
}

func (c *removeCallback) RequestRebuild()             { c.rebuilds++ }
func (c *removeCallback) SwitchToLibrary()            { c.toLibrary++ }
func (c *removeCallback) ShowNotification(msg string) { c.notifications = append(c.notifications, msg) }

// useTempDataDir points the data directory at a temporary directory
func useTempDataDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("APPDATA", dir)
}

func TestDetailRemoveMissingDisc(t *testing.T) {
	useTempDataDir(t)
	roms := t.TempDir()
	present := filepath.Join(roms, "d1.chd")
	if err := os.WriteFile(present, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lib := storage.DefaultLibrary()
	lib.AddGame(&storage.GameEntry{
		CRC32:        "T-1",
		Discs:        []storage.GameDisc{{Index: 0, File: present}, {Index: 1, File: filepath.Join(roms, "d2.chd")}},
		SelectedDisc: 1,
	})
	cb := &removeCallback{}
	s := NewDetailScreen(cb, lib, nil, 0, 0, true, nil)
	s.SetGame("T-1")

	if s.playable() || s.actionKey() != "remove" {
		t.Fatalf("missing selected disc: playable %v action %q, want remove", s.playable(), s.actionKey())
	}

	s.removeSelectedDisc()

	g := lib.GetGame("T-1")
	if len(g.Discs) != 1 || g.Discs[0].File != present || g.SelectedDisc != 0 {
		t.Errorf("game after remove = %+v, want only the present disc selected", g)
	}
	if !s.playable() || s.actionKey() != "play" {
		t.Errorf("after remove: playable %v action %q, want play", s.playable(), s.actionKey())
	}
	if cb.rebuilds != 1 {
		t.Errorf("rebuilds = %d, want 1", cb.rebuilds)
	}
}

func TestDetailRemoveGameMovesToRemoved(t *testing.T) {
	useTempDataDir(t)
	lib := storage.DefaultLibrary()
	lib.AddGame(&storage.GameEntry{CRC32: "aaaa", File: filepath.Join(t.TempDir(), "gone.sms"), PlayTimeSeconds: 60})
	cb := &removeCallback{}
	s := NewDetailScreen(cb, lib, nil, 0, 0, true, nil)
	s.SetGame("aaaa")

	if !s.files.Missing || s.actionKey() != "remove" {
		t.Fatalf("missing game: files %+v action %q, want missing and remove", s.files, s.actionKey())
	}

	s.removeGame()

	if lib.GetGame("aaaa") != nil {
		t.Error("game still in library")
	}
	if cb.toLibrary != 1 {
		t.Errorf("SwitchToLibrary calls = %d, want 1", cb.toLibrary)
	}
	removed, err := storage.LoadRemovedGames()
	if err != nil {
		t.Fatalf("LoadRemovedGames: %v", err)
	}
	if g := removed.Games["aaaa"]; g == nil || g.PlayTimeSeconds != 60 {
		t.Errorf("removed entry = %+v, want play time kept", g)
	}
}

func TestDetailPlayableCartridge(t *testing.T) {
	useTempDataDir(t)
	path := filepath.Join(t.TempDir(), "a.sms")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lib := storage.DefaultLibrary()
	lib.AddGame(&storage.GameEntry{CRC32: "aaaa", File: path})
	s := NewDetailScreen(nil, lib, nil, 0, 0, true, nil)
	s.SetGame("aaaa")

	if !s.playable() || s.actionKey() != "play" {
		t.Errorf("present cartridge: playable %v action %q, want play", s.playable(), s.actionKey())
	}
}

func TestDetailOnExitWithoutGame(t *testing.T) {
	s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, 0, true, nil)
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
	s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, cheats.SystemSaturn, true, nil)
	s.pages.sections[1].label = "Cheats (3)"

	// No game, so no cheats are on
	s.updateCheatsLabel()

	if got := s.pages.sections[1].label; got != "Cheats" {
		t.Errorf("label with no game = %q, want Cheats", got)
	}
}
