// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/style"
)

func TestRowLabelWidth(t *testing.T) {
	got := RowLabelWidth(1000, 10, 40, 60)
	want := 1000 - 20 - (style.DefaultSpacing + 40) - (style.DefaultSpacing + 60)
	if got != want {
		t.Errorf("RowLabelWidth = %d, want %d", got, want)
	}

	if got := RowLabelWidth(100, 10, 200); got != minRowLabelWidth {
		t.Errorf("narrow row: RowLabelWidth = %d, want %d", got, minRowLabelWidth)
	}
}

func TestEnabledTextButton(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		calls := 0
		press(EnabledTextButton("Reset", style.ButtonPaddingSmall, enabled, func(*widget.ButtonClickedEventArgs) { calls++ }))
		if want := map[bool]int{true: 1, false: 0}[enabled]; calls != want {
			t.Errorf("enabled %v: handler calls = %d, want %d", enabled, calls, want)
		}
	}
}

func TestCycleButtonNext(t *testing.T) {
	tests := []struct {
		current, count int
		want           int
	}{
		{0, 3, 1},
		{2, 3, 0},
		{-1, 3, 0},
		{0, 1, 0},
	}
	for _, tt := range tests {
		got := -1
		press(CycleButton("x", tt.current, tt.count, 0, func(next int) { got = next }))
		if got != tt.want {
			t.Errorf("current %d of %d: next = %d, want %d", tt.current, tt.count, got, tt.want)
		}
	}

	called := false
	press(CycleButton("x", -1, 0, 0, func(int) { called = true }))
	if called {
		t.Error("no values: handler called")
	}
}

func TestSettingsRowLabelWidth(t *testing.T) {
	if got, want := SettingsRowLabelWidth(1000, 50), RowLabelWidth(1000, style.SmallSpacing, 50); got != want {
		t.Errorf("SettingsRowLabelWidth = %d, want %d", got, want)
	}
}

func TestScrollableOptsContentWidth(t *testing.T) {
	bordered := ScrollableOpts{BorderColor: style.Border, Padding: 6}
	if got, want := bordered.ContentWidth(500), 500-12-style.ScrollbarWidth; got != want {
		t.Errorf("bordered: ContentWidth = %d, want %d", got, want)
	}

	// Without a border, zero spacing uses the default of 4
	plain := ScrollableOpts{}
	if got, want := plain.ContentWidth(500), 500-4-style.ScrollbarWidth; got != want {
		t.Errorf("default spacing: ContentWidth = %d, want %d", got, want)
	}

	spaced := ScrollableOpts{Spacing: 7}
	if got, want := spaced.ContentWidth(500), 500-7-style.ScrollbarWidth; got != want {
		t.Errorf("spacing 7: ContentWidth = %d, want %d", got, want)
	}
}

func TestActionRow(t *testing.T) {
	save := PrimaryTextButton("Save", style.ButtonPaddingSmall, nil)
	cancel := TextButton("Cancel", style.ButtonPaddingSmall, nil)
	row := ActionRow(save, cancel)

	// A stretching spacer first pushes the buttons right, in order
	children := row.Children()
	if len(children) != 3 || children[1] != save || children[2] != cancel {
		t.Errorf("children = %v, want spacer, Save, Cancel", children)
	}
}

func TestOnOffButton(t *testing.T) {
	for on, want := range map[bool]string{true: "On", false: "Off"} {
		btn := OnOffButton(on, nil)
		// The text widget is created when the button is validated for layout
		btn.Validate()
		if got := btn.Text().Label; got != want {
			t.Errorf("OnOffButton(%v) label = %q, want %q", on, got, want)
		}
	}

	if w := OnOffButtonWidth(); w < ButtonWidth("Off") || w < onOffMinWidth() {
		t.Errorf("OnOffButtonWidth = %d, want at least %d and %d", w, ButtonWidth("Off"), onOffMinWidth())
	}
}
