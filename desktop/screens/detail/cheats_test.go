// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package detail

import (
	"errors"
	"testing"

	"github.com/ebitenui/ebitenui/event"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/cheats"
	"github.com/user-none/eblitui/desktop/gamecheats"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// fakeFocus records the targets, zones, and zone transitions a section
// registers
type fakeFocus struct {
	targets     map[string]widget.Focuser
	zones       map[string][]string
	transitions map[string]map[int]navTarget // from zone -> direction -> target
	pending     string
}

// navTarget is where a zone transition lands
type navTarget struct {
	zone  string
	index int
}

func newFakeFocus() *fakeFocus {
	return &fakeFocus{
		targets:     make(map[string]widget.Focuser),
		zones:       make(map[string][]string),
		transitions: make(map[string]map[int]navTarget),
	}
}

func (f *fakeFocus) RegisterFocusButton(key string, btn *widget.Button) { f.targets[key] = btn }
func (f *fakeFocus) RegisterFocusWidget(key string, w widget.Focuser)   { f.targets[key] = w }
func (f *fakeFocus) SetPendingFocus(key string)                         { f.pending = key }
func (f *fakeFocus) SetScrollContainer(*widgets.ScrollView)             {}
func (f *fakeFocus) SaveScrollPosition()                                {}
func (f *fakeFocus) RestoreScrollPosition()                             {}
func (f *fakeFocus) RegisterNavZone(name string, _ string, keys []string, _ int) {
	f.zones[name] = append([]string(nil), keys...)
}
func (f *fakeFocus) SetNavTransition(from string, dir int, to string, index int) {
	if f.transitions[from] == nil {
		f.transitions[from] = make(map[int]navTarget)
	}
	f.transitions[from][dir] = navTarget{zone: to, index: index}
}

// fakeCallback counts rebuild requests; other callback methods are not
// used by the section
type fakeCallback struct {
	types.ScreenCallback
	rebuilds int
}

func (f *fakeCallback) RequestRebuild() { f.rebuilds++ }

// newTestCheatsSection creates a section for a game with the given
// database cheats and custom cheats. Saving the settings does nothing;
// tests that check it replace c.save.
func newTestCheatsSection(t *testing.T, db []cheats.Cheat, custom []storage.CustomCheat) (*CheatsSection, *fakeFocus) {
	t.Helper()
	c := NewCheatsSection(&fakeCallback{}, cheats.SystemSaturn, true, func() int { return 800 })
	c.save = func(string, *storage.GameSettingsFile) error { return nil }
	c.game = &storage.GameEntry{CRC32: "12345678"}
	c.gameSettings = &storage.GameSettingsFile{Cheats: storage.CheatSettings{Custom: custom}}
	c.set = gamecheats.Set{
		Database: db,
		Custom:   gamecheats.ParseCustom(custom, cheats.SystemSaturn, true),
	}
	focus := newFakeFocus()
	c.Build(focus)
	return c, focus
}

func buttonLabel(t *testing.T, f widget.Focuser) string {
	t.Helper()
	btn, ok := f.(*widget.Button)
	if !ok {
		t.Fatalf("%T is not a button", f)
	}
	// The text widget is created when the button is validated for layout
	btn.Validate()
	return btn.Text().Label
}

// pressButton clicks the button f and runs its handlers
func pressButton(t *testing.T, f widget.Focuser) {
	t.Helper()
	btn, ok := f.(*widget.Button)
	if !ok {
		t.Fatalf("%T is not a button", f)
	}
	btn.Click()
	event.ExecuteDeferred()
}

func TestCheatRowShowsStoredState(t *testing.T) {
	c, _ := newTestCheatsSection(t, nil, nil)
	c.gameSettings.SetCheatEnabled("ON", true)
	focus := newFakeFocus()

	c.buildCheatRow(focus, "cheat-0", "On cheat", "", "ON", true, 200)
	c.buildCheatRow(focus, "cheat-1", "Off cheat", "", "OFF", true, 200)
	c.buildCheatRow(focus, "cheat-2", "Invalid", "", "ON", false, 200)

	for key, want := range map[string]string{"cheat-0": "On", "cheat-1": "Off", "cheat-2": "Off"} {
		if got := buttonLabel(t, focus.targets[key]); got != want {
			t.Errorf("%s label = %q, want %q", key, got, want)
		}
	}
}

func TestCheatsEnabledCount(t *testing.T) {
	c, _ := newTestCheatsSection(t,
		[]cheats.Cheat{{Name: "A", ID: "A"}, {Name: "B", ID: "B"}},
		[]storage.CustomCheat{{Name: "Mine", Code: "160EF100 0000"}})
	if n := c.EnabledCount(); n != 0 {
		t.Errorf("none on: count %d, want 0", n)
	}

	c.gameSettings.SetCheatEnabled("A", true)
	c.gameSettings.SetCheatEnabled("160EF1000000", true)
	// Enabled IDs with no listed cheat are not counted
	c.gameSettings.SetCheatEnabled("GONE", true)
	if n := c.EnabledCount(); n != 2 {
		t.Errorf("count %d, want 2", n)
	}

	c.gameSettings = nil
	if n := c.EnabledCount(); n != 0 {
		t.Errorf("no settings: count %d, want 0", n)
	}
}

func TestCheatsSetGameNilClears(t *testing.T) {
	c, _ := newTestCheatsSection(t, []cheats.Cheat{{Name: "x", ID: "x"}}, nil)
	c.set.LoadErr = errors.New("read failed")
	c.form = &cheatForm{edit: -1}

	c.SetGame(nil, nil)

	if c.set.Database != nil || c.set.LoadErr != nil || c.gameSettings != nil || c.form != nil {
		t.Error("cheat state not cleared by SetGame(nil)")
	}
}

func TestCheatsBuildListZones(t *testing.T) {
	c, focus := newTestCheatsSection(t,
		[]cheats.Cheat{{Name: "A", ID: "A"}, {Name: "B", ID: "B"}},
		[]storage.CustomCheat{{Name: "Mine", Code: "160EF100 0000"}})

	want := []string{customZone, addZone, cheatsZone}
	if got := c.NavZones(); len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("NavZones = %v, want %v", got, want)
	}
	if got := focus.zones[customZone]; len(got) != 3 || got[0] != "custom-0-edit" || got[1] != "custom-0-delete" || got[2] != "custom-0-toggle" {
		t.Errorf("custom zone = %v", got)
	}
	if got := focus.zones[cheatsZone]; len(got) != 2 || got[0] != "cheat-0" {
		t.Errorf("database zone = %v", got)
	}

	// No cheats at all: only the Add button
	c, focus = newTestCheatsSection(t, nil, nil)
	if got := c.NavZones(); len(got) != 1 || got[0] != addZone {
		t.Errorf("empty NavZones = %v, want [%s]", got, addZone)
	}
	if focus.targets[addKey] == nil {
		t.Error("Add Cheat not registered")
	}
}

func TestCheatsInvalidCustomRow(t *testing.T) {
	c, focus := newTestCheatsSection(t, nil, []storage.CustomCheat{{Name: "Broken", Code: "not a code"}})
	c.gameSettings.SetCheatEnabled("NOTACODE", true)
	c.Build(focus)
	if got := buttonLabel(t, focus.targets["custom-0-toggle"]); got != "Off" {
		t.Errorf("invalid custom toggle = %q, want Off", got)
	}
}

func openTestForm(t *testing.T, c *CheatsSection, i int, name, code string) {
	t.Helper()
	c.openForm(i)
	if c.form == nil {
		t.Fatal("form not opened")
	}
	c.form.name.SetText(name)
	c.form.code.SetText(code)
	c.Build(c.focus)
}

func TestCheatFormZones(t *testing.T) {
	c, focus := newTestCheatsSection(t, nil, nil)
	c.openForm(-1)
	c.Build(focus)

	if focus.pending != formNameKey {
		t.Errorf("pending focus = %q, want the Name field", focus.pending)
	}
	if got := c.NavZones(); len(got) != 2 || got[0] != formFieldsZone || got[1] != formButtonsZone {
		t.Errorf("form NavZones = %v", got)
	}
	if _, ok := focus.targets[formCodeKey].(*widgets.MultiLineInput); !ok {
		t.Errorf("Code field = %T, want a MultiLineInput", focus.targets[formCodeKey])
	}
}

func TestCheatFormErrors(t *testing.T) {
	db := []cheats.Cheat{{Name: "DB Lives", ID: "160EF1000000"}}
	tests := []struct {
		name, code, want string
	}{
		{"", "160EF100 0001", "Enter a name"},
		{"Lives", "  \n", "Enter a code"},
		{"Lives", "F6000924 FFFF", "Invalid code: code contains only master codes"},
		{"Lives", "160EF100\n0000", `This code is already used by "DB Lives"`},
	}
	for _, tc := range tests {
		c, _ := newTestCheatsSection(t, db, nil)
		openTestForm(t, c, -1, tc.name, tc.code)
		c.saveForm(c.focus)
		if c.form == nil || c.form.err != tc.want {
			var got string
			if c.form != nil {
				got = c.form.err
			}
			t.Errorf("%q / %q: error %q, want %q", tc.name, tc.code, got, tc.want)
		}
		if len(c.gameSettings.Cheats.Custom) != 0 {
			t.Errorf("%q / %q: saved despite the error", tc.name, tc.code)
		}
	}
}

func TestCheatFormAdd(t *testing.T) {
	c, _ := newTestCheatsSection(t, nil, nil)
	var savedCRC string
	var saved []storage.CustomCheat
	c.save = func(crc string, s *storage.GameSettingsFile) error {
		savedCRC = crc
		saved = append([]storage.CustomCheat(nil), s.Cheats.Custom...)
		return nil
	}
	openTestForm(t, c, -1, "  Infinite Lives  ", "160EF100 0000\n160C5238 0000\n")
	c.saveForm(c.focus)

	if c.form != nil {
		t.Fatalf("form still open: %q", c.form.err)
	}
	want := storage.CustomCheat{Name: "Infinite Lives", Code: "160EF100 0000\n160C5238 0000"}
	if len(c.gameSettings.Cheats.Custom) != 1 || c.gameSettings.Cheats.Custom[0] != want {
		t.Errorf("custom = %+v, want [%+v]", c.gameSettings.Cheats.Custom, want)
	}
	if len(c.gameSettings.Cheats.Enabled) != 0 {
		t.Error("a new cheat started on")
	}
	if c.focus.(*fakeFocus).pending != "custom-0-toggle" {
		t.Errorf("pending focus = %q", c.focus.(*fakeFocus).pending)
	}

	// The game's settings were saved with the new cheat
	if savedCRC != "12345678" || len(saved) != 1 || saved[0] != want {
		t.Errorf("saved %q %+v, want 12345678 [%+v]", savedCRC, saved, want)
	}
}

func TestCheatFormEditMovesEnabledID(t *testing.T) {
	c, _ := newTestCheatsSection(t, nil, []storage.CustomCheat{{Name: "Lives", Code: "160EF100 0000"}})
	c.gameSettings.SetCheatEnabled("160EF1000000", true)

	openTestForm(t, c, 0, "Lives", "160EF100 0063")
	if c.form.name.GetText() != "Lives" {
		t.Errorf("edit form name = %q", c.form.name.GetText())
	}
	c.saveForm(c.focus)

	if c.form != nil {
		t.Fatalf("form still open: %q", c.form.err)
	}
	if c.gameSettings.CheatEnabled("160EF1000000") || !c.gameSettings.CheatEnabled("160EF1000063") {
		t.Errorf("enabled = %v, want the new code on and the old off", c.gameSettings.Cheats.Enabled)
	}

	// Saving an edit unchanged is not a duplicate of itself
	openTestForm(t, c, 0, "Lives", "160EF100 0063")
	c.saveForm(c.focus)
	if c.form != nil {
		t.Errorf("unchanged edit rejected: %q", c.form.err)
	}
}

func TestCheatFormCancelKeepsSettings(t *testing.T) {
	c, _ := newTestCheatsSection(t, nil, []storage.CustomCheat{{Name: "Lives", Code: "160EF100 0000"}})
	openTestForm(t, c, 0, "Other", "160EF100 0001")
	if !c.HandleBack() {
		t.Fatal("Back did not close the form")
	}
	if c.form != nil || c.gameSettings.Cheats.Custom[0].Name != "Lives" {
		t.Errorf("cancel changed the cheat: %+v", c.gameSettings.Cheats.Custom)
	}
	if c.focus.(*fakeFocus).pending != "custom-0-edit" {
		t.Errorf("pending focus = %q, want the Edit button", c.focus.(*fakeFocus).pending)
	}
	if c.HandleBack() {
		t.Error("Back reported handled with no form open")
	}
}

func TestCheatDelete(t *testing.T) {
	c, _ := newTestCheatsSection(t, nil, []storage.CustomCheat{
		{Name: "A", Code: "160EF100 0000"},
		{Name: "B", Code: "160EF100 0001"},
	})
	c.gameSettings.SetCheatEnabled("160EF1000000", true)

	c.deleteCustom(0)

	if len(c.gameSettings.Cheats.Custom) != 1 || c.gameSettings.Cheats.Custom[0].Name != "B" {
		t.Errorf("custom = %+v, want only B", c.gameSettings.Cheats.Custom)
	}
	if c.gameSettings.CheatEnabled("160EF1000000") {
		t.Error("deleted cheat still enabled")
	}
	if c.focus.(*fakeFocus).pending != addKey {
		t.Errorf("pending focus = %q, want Add Cheat", c.focus.(*fakeFocus).pending)
	}
}

func TestCheatDeleteTwoPresses(t *testing.T) {
	c, focus := newTestCheatsSection(t, nil, []storage.CustomCheat{{Name: "A", Code: "160EF100 0000"}})
	delKey := customKey(0, "delete")

	// The first press arms the delete and keeps focus on it
	pressButton(t, focus.targets[delKey])
	if len(c.gameSettings.Cheats.Custom) != 1 {
		t.Fatal("cheat deleted on the first press")
	}
	if focus.pending != delKey {
		t.Errorf("pending focus = %q, want %q", focus.pending, delKey)
	}
	c.Build(focus)
	if got := buttonLabel(t, focus.targets[delKey]); got != "Confirm Delete" {
		t.Fatalf("armed label = %q, want Confirm Delete", got)
	}

	// Back cancels the armed delete
	if !c.HandleBack() {
		t.Error("HandleBack did not report the cancel")
	}
	if c.HandleBack() {
		t.Error("HandleBack handled with nothing armed")
	}
	c.Build(focus)
	if got := buttonLabel(t, focus.targets[delKey]); got != "Delete" {
		t.Errorf("label after Back = %q, want Delete", got)
	}

	// Two presses delete the cheat
	pressButton(t, focus.targets[delKey])
	c.Build(focus)
	pressButton(t, focus.targets[delKey])
	if len(c.gameSettings.Cheats.Custom) != 0 {
		t.Errorf("custom = %+v after confirming, want none", c.gameSettings.Cheats.Custom)
	}
}

func TestCheatStaleIndexIgnored(t *testing.T) {
	c, _ := newTestCheatsSection(t, nil, []storage.CustomCheat{{Name: "A", Code: "160EF100 0000"}})
	c.deleteCustom(0)

	// Handlers from a build before the delete carry indexes past the list
	c.deleteCustom(0)
	c.deleteCustom(-1)
	c.openForm(0)

	if len(c.gameSettings.Cheats.Custom) != 0 || len(c.set.Custom) != 0 {
		t.Errorf("custom = %+v, want none", c.gameSettings.Cheats.Custom)
	}
	if c.form != nil {
		t.Error("form opened for a cheat that no longer exists")
	}
}

func TestValidateCheatName(t *testing.T) {
	if ok, _ := validateCheatName("Infinite Lives"); !ok {
		t.Error("plain name rejected")
	}
	ok, repl := validateCheatName("two\nlines")
	if ok || repl == nil || *repl != "two lines" {
		t.Errorf("line break: ok %v replacement %v", ok, repl)
	}
	long := make([]rune, storage.MaxCustomCheatNameLen+5)
	for i := range long {
		long[i] = 'a'
	}
	ok, repl = validateCheatName(string(long))
	if ok || repl == nil || len([]rune(*repl)) != storage.MaxCustomCheatNameLen {
		t.Errorf("long name not trimmed to the limit")
	}
}
