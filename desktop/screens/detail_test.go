// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"bytes"
	"image"
	"image/png"
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
	s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, 0, nil, true, nil)
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
		s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, sys, nil, true, nil)

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
		s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, sys, nil, true, nil)
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
		s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, sys, nil, true, nil)
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
	lib.AddGame(&storage.GameEntry{GameID: "aaaa", File: filepath.Join(t.TempDir(), "a.cue")})
	upscale := coreif.CoreOption{Key: "upscale", Type: coreif.CoreOptionSelect, Values: []string{"1x", "2x"}, Category: coreif.CoreOptionCategoryVideo, PerGame: true}
	s := NewDetailScreen(&pagesCallback{}, lib, nil, 0, cheats.SystemSaturn, nil, true, []coreif.CoreOption{upscale})
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
		return NewDetailScreen(&widthCallback{width: width}, storage.DefaultLibrary(), nil, 0, 0, nil, true, nil)
	}

	// The art sits over the sidebar at every width: the column is the art
	// width, from the minimum up to the maximum
	s := screen(900)
	if got := s.columnWidth(); got != style.DetailColumnWidth {
		t.Errorf("900 wide: column = %d, want the minimum %d", got, style.DetailColumnWidth)
	}
	s = screen(1100)
	if got, want := s.columnWidth(), 1100/detailColumnFraction; got != want {
		t.Errorf("1100 wide: column = %d, want %d", got, want)
	}
	s = screen(1600)
	if got, want := s.columnWidth(), 1600/detailColumnFraction; got != want {
		t.Errorf("1600 wide: column = %d, want %d", got, want)
	}
	s = screen(4000)
	if got, want := s.columnWidth(), style.Px(detailColumnMaxWidth); got != want {
		t.Errorf("4000 wide: column = %d, want the maximum %d", got, want)
	}
}

func TestDetailArtworkPageAfterOverview(t *testing.T) {
	s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, cheats.SystemSaturn, nil, true, nil)
	if got := s.pages.labels(); len(got) < 2 || got[0] != "Overview" || got[1] != "Artwork" {
		t.Errorf("pages = %v, want Overview then Artwork", got)
	}
}

// writeArtPNG writes a w by h PNG as name in a game's artwork directory
func writeArtPNG(t *testing.T, gameID, name string, w, h int) {
	t.Helper()
	dir, err := storage.GetGameArtworkDir(gameID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetailFrontSize(t *testing.T) {
	useTempDataDir(t)
	lib := storage.DefaultLibrary()
	for _, id := range []string{"T-1", "T-2", "T-3"} {
		lib.AddGame(&storage.GameEntry{GameID: id})
	}
	writeArtPNG(t, "T-1", "boxart.webp", 300, 300)
	dir, err := storage.GetGameArtworkDir("T-3")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "boxart.webp"), []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewDetailScreen(nil, lib, nil, 0, 0, nil, true, nil)

	s.SetGame("T-1")
	if w, h := s.frontSize(); w != 300 || h != 300 {
		t.Fatalf("front size = %dx%d, want 300x300", w, h)
	}
	// Read once per game: a changed file is seen after SetGame
	writeArtPNG(t, "T-1", "boxart.webp", 200, 100)
	if w, h := s.frontSize(); w != 300 || h != 300 {
		t.Errorf("same game: front size = %dx%d, want the first read 300x300", w, h)
	}
	s.SetGame("T-1")
	if w, h := s.frontSize(); w != 200 || h != 100 {
		t.Errorf("after SetGame: front size = %dx%d, want 200x100", w, h)
	}

	s.SetGame("T-2")
	if w, h := s.frontSize(); w != 0 || h != 0 {
		t.Errorf("no front: size = %dx%d, want 0x0", w, h)
	}
	s.SetGame("T-3")
	if w, h := s.frontSize(); w != 0 || h != 0 {
		t.Errorf("unreadable front: size = %dx%d, want 0x0", w, h)
	}
}

func TestDetailArtHeight(t *testing.T) {
	tests := []struct {
		name        string
		width, w, h int
		want        int
	}{
		{"square cover", 300, 1000, 1000, 300},
		{"tall cover capped at 3:4", 300, 1000, 1500, 400},
		{"3:4 cover", 300, 750, 1000, 400},
		{"wide cover", 300, 1000, 500, 150},
		{"no cover", 300, 0, 0, 400},
	}
	for _, tt := range tests {
		if got := artHeight(tt.width, tt.w, tt.h); got != tt.want {
			t.Errorf("%s: artHeight(%d, %d, %d) = %d, want %d", tt.name, tt.width, tt.w, tt.h, got, tt.want)
		}
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
		GameID:       "T-1",
		Discs:        []storage.GameDisc{{Index: 0, File: present}, {Index: 1, File: filepath.Join(roms, "d2.chd")}},
		SelectedDisc: 1,
	})
	cb := &removeCallback{}
	s := NewDetailScreen(cb, lib, nil, 0, 0, nil, true, nil)
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
	lib.AddGame(&storage.GameEntry{GameID: "aaaa", File: filepath.Join(t.TempDir(), "gone.sms"), PlayTimeSeconds: 60})
	cb := &removeCallback{}
	s := NewDetailScreen(cb, lib, nil, 0, 0, nil, true, nil)
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
	lib.AddGame(&storage.GameEntry{GameID: "aaaa", File: path})
	s := NewDetailScreen(nil, lib, nil, 0, 0, nil, true, nil)
	s.SetGame("aaaa")

	if !s.playable() || s.actionKey() != "play" {
		t.Errorf("present cartridge: playable %v action %q, want play", s.playable(), s.actionKey())
	}
}

func TestDetailOnExitWithoutGame(t *testing.T) {
	s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, 0, nil, true, nil)
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
	s := NewDetailScreen(nil, storage.DefaultLibrary(), nil, 0, cheats.SystemSaturn, nil, true, nil)
	idx := -1
	for i, sec := range s.pages.sections {
		if sec.focusKey == cheatsPageKey {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatal("no Cheats page")
	}
	s.pages.sections[idx].label = "Cheats (3)"

	// No game, so no cheats are on
	s.updateCheatsLabel()

	if got := s.pages.sections[idx].label; got != "Cheats" {
		t.Errorf("label with no game = %q, want Cheats", got)
	}
}
