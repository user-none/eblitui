// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"testing"

	"github.com/ebitenui/ebitenui/event"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// inputFocus records the buttons and pending focus a section registers
type inputFocus struct {
	targets map[string]widget.Focuser
	pending string
}

func newInputFocus() *inputFocus {
	return &inputFocus{targets: make(map[string]widget.Focuser)}
}

func (f *inputFocus) RegisterFocusButton(key string, btn *widget.Button) { f.targets[key] = btn }
func (f *inputFocus) RegisterFocusWidget(key string, w widget.Focuser)   { f.targets[key] = w }
func (f *inputFocus) SetPendingFocus(key string)                         { f.pending = key }
func (f *inputFocus) SetScrollContainer(*widgets.ScrollView)             {}
func (f *inputFocus) SaveScrollPosition()                                {}
func (f *inputFocus) RestoreScrollPosition()                             {}
func (f *inputFocus) RegisterNavZone(string, string, []string, int)      {}
func (f *inputFocus) SetNavTransition(string, int, string, int)          {}

// inputCallback counts rebuild requests
type inputCallback struct {
	types.ScreenCallback
	rebuilds int
}

func (c *inputCallback) RequestRebuild() { c.rebuilds++ }

// pressInputButton clicks the button f and runs its handlers
func pressInputButton(t *testing.T, f widget.Focuser) {
	t.Helper()
	btn, ok := f.(*widget.Button)
	if !ok {
		t.Fatalf("%T is not a button", f)
	}
	btn.Click()
	event.ExecuteDeferred()
}

// inputButtonLabel returns the label the button f shows
func inputButtonLabel(t *testing.T, f widget.Focuser) string {
	t.Helper()
	btn, ok := f.(*widget.Button)
	if !ok {
		t.Fatalf("%T is not a button", f)
	}
	btn.Validate()
	return btn.Text().Label
}

func TestProfileDeleteTwoPresses(t *testing.T) {
	// Deleting saves the config; keep it in a temp data directory
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("APPDATA", dir)

	config := &storage.Config{Input: storage.InputConfig{
		Profiles: []storage.ControllerProfile{{ID: "p1", Name: "Pad"}, {ID: "p2", Name: "Other"}},
		Players:  []storage.PlayerConfig{{Profile: "p1"}},
	}}
	s := NewInputSection(&inputCallback{}, config, coreif.SystemInfo{})
	focus := newInputFocus()
	s.focus = focus
	delKey := "input-profile-p1-delete"
	build := func() { s.buildProfileRow(focus, &config.Input.Profiles[0]) }

	// The first press arms the delete and keeps focus on it
	build()
	pressInputButton(t, focus.targets[delKey])
	if len(config.Input.Profiles) != 2 {
		t.Fatal("profile deleted on the first press")
	}
	if focus.pending != delKey {
		t.Errorf("pending focus = %q, want %q", focus.pending, delKey)
	}
	build()
	if got := inputButtonLabel(t, focus.targets[delKey]); got != "Confirm Delete" {
		t.Fatalf("armed label = %q, want Confirm Delete", got)
	}

	// Back cancels the armed delete
	if !s.HandleBack() {
		t.Error("HandleBack did not report the cancel")
	}
	if s.HandleBack() {
		t.Error("HandleBack handled with nothing armed")
	}
	build()
	if got := inputButtonLabel(t, focus.targets[delKey]); got != "Delete" {
		t.Errorf("label after Back = %q, want Delete", got)
	}

	// Two presses delete the profile and unassign its player
	pressInputButton(t, focus.targets[delKey])
	build()
	pressInputButton(t, focus.targets[delKey])
	if len(config.Input.Profiles) != 1 || config.Input.Profiles[0].ID != "p2" {
		t.Errorf("profiles = %+v, want only p2", config.Input.Profiles)
	}
	if config.Input.Players[0].Profile != "" {
		t.Errorf("player 1 profile = %q, want none", config.Input.Players[0].Profile)
	}
	if focus.pending != "input-new-profile" {
		t.Errorf("pending focus = %q, want New Profile", focus.pending)
	}
}
