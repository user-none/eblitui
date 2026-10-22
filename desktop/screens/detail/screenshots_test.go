// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package detail

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ebitenui/ebitenui/event"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/types"
)

// shotCallback counts rebuild requests, which the preview decode makes
// from its goroutine, and records notifications
type shotCallback struct {
	types.ScreenCallback
	mu       sync.Mutex
	rebuilds int
	notices  []string
}

func (c *shotCallback) RequestRebuild() {
	c.mu.Lock()
	c.rebuilds++
	c.mu.Unlock()
}

func (c *shotCallback) ShowNotification(msg string) {
	c.mu.Lock()
	c.notices = append(c.notices, msg)
	c.mu.Unlock()
}

func (c *shotCallback) rebuildCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rebuilds
}

// errTestDecode is what the test decoder fails with
var errTestDecode = errors.New("test decode")

// newTestScreenshotsSection creates a page over a temp screenshot directory
// holding files with the given names. Decodes send the path they were
// asked for on the returned channel and fail.
func newTestScreenshotsSection(t *testing.T, names ...string) (*ScreenshotsSection, *shotCallback, chan string, string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	cb := &shotCallback{}
	decoded := make(chan string, 16)
	s := NewScreenshotsSection(cb, func() int { return 800 })
	s.gameDir = func(string) (string, error) { return dir, nil }
	s.decode = func(path string) (*ebiten.Image, error) {
		decoded <- path
		return nil, errTestDecode
	}
	s.showInFileManager = func(string) error { t.Fatal("showInFileManager called without being replaced"); return nil }
	s.SetGame(&storage.GameEntry{CRC32: "ABCD1234"})
	return s, cb, decoded, dir
}

// waitDecode returns the next decoded path, failing the test when none
// arrives
func waitDecode(t *testing.T, decoded chan string) string {
	t.Helper()
	select {
	case p := <-decoded:
		return p
	case <-time.After(2 * time.Second):
		t.Fatal("no decode started")
		return ""
	}
}

// waitRebuilds waits until the callback has seen at least n rebuilds
func waitRebuilds(t *testing.T, cb *shotCallback, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for cb.rebuildCount() < n {
		if time.Now().After(deadline) {
			t.Fatalf("rebuilds = %d, want at least %d", cb.rebuildCount(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestScreenshotsEmpty(t *testing.T) {
	s, _, decoded, _ := newTestScreenshotsSection(t)
	if s.selected != -1 || len(s.shots) != 0 {
		t.Fatalf("selected = %d, shots = %d", s.selected, len(s.shots))
	}
	focus := newFakeFocus()
	s.Build(focus)
	if len(s.NavZones()) != 0 || len(focus.zones) != 0 {
		t.Errorf("zones registered for an empty page: %v %v", s.NavZones(), focus.zones)
	}
	if len(decoded) != 0 {
		t.Error("decode started with nothing selected")
	}
}

func TestScreenshotsListsNewestFirstAndPreviewsIt(t *testing.T) {
	s, cb, decoded, dir := newTestScreenshotsSection(t,
		"1760540000100.png",
		"1760540000300_achievement_Boss.png",
		"1760540000200.png",
	)
	if len(s.shots) != 3 {
		t.Fatalf("shots = %d, want 3", len(s.shots))
	}
	if filepath.Base(s.shots[0].Path) != "1760540000300_achievement_Boss.png" {
		t.Errorf("shots[0] = %q", filepath.Base(s.shots[0].Path))
	}
	if s.selected != 0 {
		t.Errorf("selected = %d, want 0", s.selected)
	}
	if got := waitDecode(t, decoded); got != filepath.Join(dir, "1760540000300_achievement_Boss.png") {
		t.Errorf("decoded %q", got)
	}
	// The failed decode is stored and shown through a rebuild
	waitRebuilds(t, cb, 1)
	s.mu.Lock()
	loading, loadErr := s.loading, s.loadErr
	s.mu.Unlock()
	if loading || !errors.Is(loadErr, errTestDecode) {
		t.Errorf("loading = %v, loadErr = %v", loading, loadErr)
	}

	focus := newFakeFocus()
	s.Build(focus)
	if got := focus.zones[screenshotsZone]; len(got) != 3 || got[0] != "screenshot-0" || got[2] != "screenshot-2" {
		t.Errorf("list zone keys = %v", got)
	}
	if got := focus.zones[screenshotActionsZone]; len(got) != 2 || got[0] != screenshotFolderKey || got[1] != screenshotDeleteKey {
		t.Errorf("actions zone keys = %v", got)
	}
	if zones := s.NavZones(); len(zones) != 2 || zones[0] != screenshotsZone || zones[1] != screenshotActionsZone {
		t.Errorf("NavZones = %v", zones)
	}
	for _, key := range []string{"screenshot-0", "screenshot-1", "screenshot-2", screenshotFolderKey, screenshotDeleteKey} {
		if focus.targets[key] == nil {
			t.Errorf("%s not registered for focus", key)
		}
	}
}

func TestScreenshotsSelect(t *testing.T) {
	s, _, decoded, dir := newTestScreenshotsSection(t,
		"1760540000100.png",
		"1760540000200.png",
		"1760540000300.png",
	)
	waitDecode(t, decoded)

	focus := newFakeFocus()
	s.selectShot(2, focus)
	if s.selected != 2 {
		t.Errorf("selected = %d, want 2", s.selected)
	}
	if focus.pending != "screenshot-2" {
		t.Errorf("pending focus = %q", focus.pending)
	}
	if got := waitDecode(t, decoded); got != filepath.Join(dir, "1760540000100.png") {
		t.Errorf("decoded %q", got)
	}

	// Left from the actions returns to the selected row, and Right from
	// the list enters the first action
	focus = newFakeFocus()
	s.Build(focus)
	if got := focus.transitions[screenshotActionsZone][types.DirLeft]; got != (navTarget{zone: screenshotsZone, index: 2}) {
		t.Errorf("actions Left = %+v, want the selected row", got)
	}
	if got := focus.transitions[screenshotsZone][types.DirRight]; got != (navTarget{zone: screenshotActionsZone, index: types.NavIndexFirst}) {
		t.Errorf("list Right = %+v, want the first action", got)
	}
	if s.SelectedRow() != 2 {
		t.Errorf("SelectedRow = %d, want 2", s.SelectedRow())
	}

	// Selecting the current screenshot again decodes nothing
	s.selectShot(2, focus)
	select {
	case p := <-decoded:
		t.Errorf("unexpected decode of %q", p)
	case <-time.After(50 * time.Millisecond):
	}

	// Out of range is ignored
	s.selectShot(3, focus)
	if s.selected != 2 {
		t.Errorf("selected = %d after out of range select", s.selected)
	}
}

func TestScreenshotsDeleteTwoStep(t *testing.T) {
	s, _, decoded, dir := newTestScreenshotsSection(t,
		"1760540000100.png",
		"1760540000200.png",
		"1760540000300.png",
	)
	waitDecode(t, decoded)
	focus := newFakeFocus()
	newest := filepath.Join(dir, "1760540000300.png")

	// First press arms the delete, keeps focus on it, and keeps the file
	s.Build(focus)
	pressButton(t, focus.targets[screenshotDeleteKey])
	if focus.pending != screenshotDeleteKey {
		t.Errorf("pending focus = %q", focus.pending)
	}
	if _, err := os.Stat(newest); err != nil {
		t.Fatalf("file removed on first press: %v", err)
	}
	s.Build(focus)
	if got := buttonLabel(t, focus.targets[screenshotDeleteKey]); got != "Confirm Delete" {
		t.Fatalf("armed label = %q, want Confirm Delete", got)
	}

	// Back cancels the armed delete
	if !s.HandleBack() {
		t.Error("HandleBack did not report the cancel")
	}
	if s.HandleBack() {
		t.Error("HandleBack handled with nothing armed")
	}
	s.Build(focus)
	if got := buttonLabel(t, focus.targets[screenshotDeleteKey]); got != "Delete" {
		t.Errorf("label after Back = %q, want Delete", got)
	}

	// Focus leaving the armed delete also cancels it
	pressButton(t, focus.targets[screenshotDeleteKey])
	s.Build(focus)
	focus.targets[screenshotDeleteKey].Focus(false)
	event.ExecuteDeferred()
	s.Build(focus)
	if got := buttonLabel(t, focus.targets[screenshotDeleteKey]); got != "Delete" {
		t.Errorf("label after focus loss = %q, want Delete", got)
	}

	// Two presses delete the selected screenshot, and the selection moves
	// to the next older one at the same position
	s.selectShot(1, focus)
	waitDecode(t, decoded)
	s.Build(focus)
	pressButton(t, focus.targets[screenshotDeleteKey])
	s.Build(focus)
	pressButton(t, focus.targets[screenshotDeleteKey])
	if _, err := os.Stat(filepath.Join(dir, "1760540000200.png")); !os.IsNotExist(err) {
		t.Errorf("selected file not removed: %v", err)
	}
	if len(s.shots) != 2 {
		t.Fatalf("shots = %d after delete, want 2", len(s.shots))
	}
	if s.selected != 1 || filepath.Base(s.shots[1].Path) != "1760540000100.png" {
		t.Errorf("selected = %d (%q), want 1 (1760540000100.png)", s.selected, filepath.Base(s.shots[s.selected].Path))
	}
	if focus.pending != "screenshot-1" {
		t.Errorf("pending focus = %q", focus.pending)
	}
	if got := waitDecode(t, decoded); got != filepath.Join(dir, "1760540000100.png") {
		t.Errorf("decoded %q after delete", got)
	}
}

func TestScreenshotsDeleteAlreadyRemoved(t *testing.T) {
	s, cb, decoded, dir := newTestScreenshotsSection(t,
		"1760540000100.png",
		"1760540000200.png",
	)
	waitDecode(t, decoded)
	focus := newFakeFocus()

	// The selected file disappears outside the app; deleting it succeeds
	// and drops the entry without a notification
	if err := os.Remove(filepath.Join(dir, "1760540000200.png")); err != nil {
		t.Fatal(err)
	}
	s.deleteSelected(focus)
	if len(cb.notices) != 0 {
		t.Errorf("notices = %q, want none", cb.notices)
	}
	if len(s.shots) != 1 || filepath.Base(s.shots[0].Path) != "1760540000100.png" {
		t.Errorf("shots after delete = %+v", s.shots)
	}
	if s.selected != 0 {
		t.Errorf("selected = %d, want 0", s.selected)
	}
	waitDecode(t, decoded)
}

func TestScreenshotsReleasePreview(t *testing.T) {
	s, cb, decoded, _ := newTestScreenshotsSection(t, "1760540000100.png")
	waitDecode(t, decoded)
	waitRebuilds(t, cb, 1)

	s.ReleasePreview()
	s.mu.Lock()
	loading, loadErr, preview := s.loading, s.loadErr, s.preview
	s.mu.Unlock()
	if loading || loadErr != nil || preview != nil {
		t.Errorf("after release: loading=%v loadErr=%v preview=%v", loading, loadErr, preview)
	}

	// The list and selection are kept for the return to the page
	if len(s.shots) != 1 || s.selected != 0 {
		t.Errorf("shots=%d selected=%d after release", len(s.shots), s.selected)
	}
}

func TestScreenshotsDeleteLast(t *testing.T) {
	s, _, decoded, _ := newTestScreenshotsSection(t, "1760540000100.png")
	waitDecode(t, decoded)
	focus := newFakeFocus()

	s.deleteSelected(focus)
	if len(s.shots) != 0 || s.selected != -1 {
		t.Fatalf("shots = %d, selected = %d", len(s.shots), s.selected)
	}
	focus = newFakeFocus()
	s.Build(focus)
	if len(s.NavZones()) != 0 {
		t.Errorf("zones = %v for an empty page", s.NavZones())
	}
	if len(decoded) != 0 {
		t.Error("decode started with nothing left")
	}
}

func TestScreenshotsStaleDecodeDropped(t *testing.T) {
	s, cb, decoded, _ := newTestScreenshotsSection(t, "1760540000100.png")
	waitDecode(t, decoded)
	waitRebuilds(t, cb, 1)

	// A result from before the current generation is ignored
	s.mu.Lock()
	stale := s.loadGen - 1
	s.mu.Unlock()
	s.finishPreview(stale, nil, errors.New("stale"))

	s.mu.Lock()
	loadErr := s.loadErr
	s.mu.Unlock()
	if !errors.Is(loadErr, errTestDecode) {
		t.Errorf("loadErr = %v, want the current decode's error", loadErr)
	}
	if cb.rebuildCount() != 1 {
		t.Errorf("rebuilds = %d, want 1", cb.rebuildCount())
	}
}

func TestScreenshotsShowSelectedInFileManager(t *testing.T) {
	s, cb, decoded, dir := newTestScreenshotsSection(t,
		"1760540000100.png",
		"1760540000200.png",
	)
	waitDecode(t, decoded)

	var shown []string
	s.showInFileManager = func(path string) error {
		shown = append(shown, path)
		return nil
	}
	s.showSelectedInFileManager()
	if len(shown) != 1 || shown[0] != filepath.Join(dir, "1760540000200.png") {
		t.Errorf("shown %q", shown)
	}
	if len(cb.notices) != 0 {
		t.Errorf("notices on success: %q", cb.notices)
	}

	// A failure is reported to the user
	s.showInFileManager = func(string) error { return errors.New("no file manager") }
	s.showSelectedInFileManager()
	if len(cb.notices) != 1 {
		t.Errorf("notices = %q, want one", cb.notices)
	}

	// Nothing selected, nothing shown
	s.SetGame(nil)
	s.showInFileManager = func(string) error { t.Error("showInFileManager with nothing selected"); return nil }
	s.showSelectedInFileManager()
}

func TestScreenshotsSetGameNilClears(t *testing.T) {
	s, _, decoded, _ := newTestScreenshotsSection(t, "1760540000100.png")
	waitDecode(t, decoded)

	s.SetGame(nil)
	if len(s.shots) != 0 || s.selected != -1 || s.dir != "" {
		t.Errorf("page not cleared: shots=%d selected=%d dir=%q", len(s.shots), s.selected, s.dir)
	}
	focus := newFakeFocus()
	s.Build(focus)
	if len(s.NavZones()) != 0 {
		t.Errorf("zones = %v after clearing", s.NavZones())
	}
}
