// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	"github.com/ebitenui/ebitenui/widget"
)

// ConfirmDelete builds Delete buttons that take two presses. The first
// press arms the button, which the next build shows as Confirm Delete in
// the primary style; the second press deletes. Focus leaving the armed
// button disarms it. At most one delete is armed, named by the key it was
// built with. The zero value has nothing armed.
type ConfirmDelete struct {
	armed string
}

// pendingFocuser sets the focus target restored after a rebuild
type pendingFocuser interface {
	SetPendingFocus(key string)
}

// Button creates the Delete button for key, which must not be empty and
// is unique among the buttons sharing d. key is also the button's focus
// key. Pressing it unarmed arms key, sets focus to it, and calls rebuild;
// a mouse click does not focus a button, and focus leaving the button is
// what disarms it. Pressing it armed disarms and calls onConfirm. Focus
// leaving it while armed disarms and calls rebuild.
func (d *ConfirmDelete) Button(key string, padding int, focus pendingFocuser, rebuild, onConfirm func()) *widget.Button {
	onClick := func(*widget.ButtonClickedEventArgs) {
		if d.armed != key {
			d.armed = key
			focus.SetPendingFocus(key)
			rebuild()
			return
		}
		d.Disarm()
		onConfirm()
	}
	var btn *widget.Button
	if d.armed == key {
		btn = PrimaryTextButton(d.Label(key), padding, onClick)
	} else {
		btn = TextButton(d.Label(key), padding, onClick)
	}

	btn.GetWidget().FocusEvent.AddHandler(func(args interface{}) {
		// A press on a button fires a focus event with no widget, meaning
		// the press was outside any focusable widget; only this button
		// losing focus disarms
		if a, ok := args.(*widget.WidgetFocusEventArgs); !ok || a.Focused || a.Widget != btn || d.armed != key {
			return
		}
		d.Disarm()
		rebuild()
	})
	return btn
}

// Label returns the text of key's button: Confirm Delete when armed,
// otherwise Delete
func (d *ConfirmDelete) Label(key string) string {
	if d.armed == key {
		return "Confirm Delete"
	}
	return "Delete"
}

// Disarm cancels an armed delete, reporting whether there was one
func (d *ConfirmDelete) Disarm() bool {
	if d.armed == "" {
		return false
	}
	d.armed = ""
	return true
}
