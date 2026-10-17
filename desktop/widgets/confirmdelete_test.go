// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/event"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/style"
)

// deleteCalls records what a ConfirmDelete button asks of its screen: the
// pending focus key and the rebuilds and confirms it requests
type deleteCalls struct {
	pending            string
	rebuilds, confirms int
}

func (c *deleteCalls) SetPendingFocus(key string) { c.pending = key }

// button builds the Delete button for key reporting to c
func (c *deleteCalls) button(d *ConfirmDelete, key string) *widget.Button {
	return d.Button(key, style.ButtonPaddingSmall, c,
		func() { c.rebuilds++ },
		func() { c.confirms++ })
}

// press clicks btn and runs its handlers
func press(btn *widget.Button) {
	btn.Click()
	event.ExecuteDeferred()
}

// buttonText returns the label btn shows
func buttonText(btn *widget.Button) string {
	btn.Validate()
	return btn.Text().Label
}

func TestConfirmDeleteTwoPresses(t *testing.T) {
	var d ConfirmDelete
	var calls deleteCalls

	btn := calls.button(&d, "a")
	if got := buttonText(btn); got != "Delete" {
		t.Errorf("unarmed label = %q, want Delete", got)
	}

	// The first press arms without confirming, keeping focus on the button
	press(btn)
	if calls.rebuilds != 1 || calls.confirms != 0 {
		t.Fatalf("after first press: rebuilds=%d confirms=%d", calls.rebuilds, calls.confirms)
	}
	if calls.pending != "a" {
		t.Errorf("pending focus = %q, want a", calls.pending)
	}
	if got := d.Label("a"); got != "Confirm Delete" {
		t.Errorf("armed Label = %q, want Confirm Delete", got)
	}

	// The rebuilt button shows the armed state, and pressing it confirms
	btn = calls.button(&d, "a")
	if got := buttonText(btn); got != "Confirm Delete" {
		t.Errorf("armed button label = %q, want Confirm Delete", got)
	}
	press(btn)
	if calls.rebuilds != 1 || calls.confirms != 1 {
		t.Errorf("after second press: rebuilds=%d confirms=%d", calls.rebuilds, calls.confirms)
	}
	if got := d.Label("a"); got != "Delete" {
		t.Errorf("Label after confirm = %q, want Delete", got)
	}
}

func TestConfirmDeleteArmMoves(t *testing.T) {
	var d ConfirmDelete
	var calls deleteCalls

	press(calls.button(&d, "a"))
	press(calls.button(&d, "b"))

	if calls.rebuilds != 2 || calls.confirms != 0 || calls.pending != "b" {
		t.Errorf("rebuilds=%d confirms=%d pending=%q, want 2, 0, b", calls.rebuilds, calls.confirms, calls.pending)
	}
	if d.Label("a") != "Delete" || d.Label("b") != "Confirm Delete" {
		t.Errorf("labels a=%q b=%q, want only b armed", d.Label("a"), d.Label("b"))
	}
}

func TestConfirmDeleteFocusLostDisarms(t *testing.T) {
	var d ConfirmDelete
	var calls deleteCalls

	press(calls.button(&d, "a"))
	btn := calls.button(&d, "a")
	calls = deleteCalls{}

	// Gaining focus keeps it armed
	btn.Focus(true)
	event.ExecuteDeferred()
	if calls.rebuilds != 0 || d.Label("a") != "Confirm Delete" {
		t.Fatalf("focus gained: rebuilds=%d label=%q", calls.rebuilds, d.Label("a"))
	}

	// A press on the button reports a focus event with no widget, which
	// is not the button losing focus
	btn.GetWidget().FireFocusEvent(nil, false, image.Point{})
	event.ExecuteDeferred()
	if calls.rebuilds != 0 || d.Label("a") != "Confirm Delete" {
		t.Fatalf("press event: rebuilds=%d label=%q", calls.rebuilds, d.Label("a"))
	}

	// Losing focus disarms and rebuilds without moving focus
	btn.Focus(false)
	event.ExecuteDeferred()
	if calls.rebuilds != 1 || d.Label("a") != "Delete" {
		t.Errorf("focus lost: rebuilds=%d label=%q", calls.rebuilds, d.Label("a"))
	}
	if calls.pending != "" {
		t.Errorf("pending focus = %q after focus loss, want none", calls.pending)
	}

	// Losing focus again with nothing armed does nothing
	btn.Focus(false)
	event.ExecuteDeferred()
	if calls.rebuilds != 1 {
		t.Errorf("rebuilds = %d after a second focus loss", calls.rebuilds)
	}
}

func TestConfirmDeleteFocusLostOtherKey(t *testing.T) {
	var d ConfirmDelete
	var calls deleteCalls

	other := calls.button(&d, "b")
	press(calls.button(&d, "a"))
	calls = deleteCalls{}

	other.Focus(false)
	event.ExecuteDeferred()
	if calls.rebuilds != 0 || d.Label("a") != "Confirm Delete" {
		t.Errorf("unarmed button losing focus: rebuilds=%d label=%q", calls.rebuilds, d.Label("a"))
	}
}

func TestConfirmDeleteDisarm(t *testing.T) {
	var d ConfirmDelete
	if d.Disarm() {
		t.Error("Disarm reported an armed delete on the zero value")
	}

	var calls deleteCalls
	press(calls.button(&d, "a"))
	calls = deleteCalls{}
	if !d.Disarm() {
		t.Error("Disarm did not report the armed delete")
	}
	if d.Label("a") != "Delete" {
		t.Errorf("Label after Disarm = %q, want Delete", d.Label("a"))
	}
	if d.Disarm() {
		t.Error("second Disarm reported an armed delete")
	}
	if calls.rebuilds != 0 {
		t.Errorf("Disarm requested %d rebuilds", calls.rebuilds)
	}
}
