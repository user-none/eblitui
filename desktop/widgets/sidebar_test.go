// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	"math"
	"testing"

	"github.com/ebitenui/ebitenui/event"
	"github.com/user-none/eblitui/desktop/style"
)

func TestSidebarFitWidth(t *testing.T) {
	labels := []string{"Short", "A much longer label"}
	want := int(math.Ceil(style.MeasureWidth(labels[1]))) + style.ButtonPaddingSmall*2 + style.SmallSpacing*2
	if got := SidebarFitWidth(labels); got != want {
		t.Errorf("SidebarFitWidth = %d, want %d", got, want)
	}
}

func TestSidebarFitWidthEmpty(t *testing.T) {
	want := style.ButtonPaddingSmall*2 + style.SmallSpacing*2
	if got := SidebarFitWidth(nil); got != want {
		t.Errorf("SidebarFitWidth(nil) = %d, want %d", got, want)
	}
}

func TestSidebarItemsAndClear(t *testing.T) {
	s := NewSidebar(style.SidebarMinWidth)
	clicked := false
	btn := s.AddButton("One", true, func() { clicked = true })
	s.AddDivider()
	s.AddButton("Two", false, nil)
	s.AddDisabledItem("Three")

	if got := len(s.Container.Children()); got != 4 {
		t.Fatalf("children = %d, want 4", got)
	}

	btn.Click()
	event.ExecuteDeferred()
	if !clicked {
		t.Error("button handler not called on click")
	}

	s.Clear()
	if got := len(s.Container.Children()); got != 0 {
		t.Errorf("children after Clear = %d, want 0", got)
	}
}
