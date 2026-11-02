// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	goimage "image"
	"image/color"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// scrollNavScreen is a screen laid out as
//
//	sidebar | rows in a scroll view | scroll bar | action
//
// with a toolbar button above the scroll bar. The rows zone has its own
// Right transition to the action.
type scrollNavScreen struct {
	b                     *BaseScreen
	side, row0, row1, act *widget.Button
	tool                  *widget.Button
	bar                   *widget.Slider
	view                  *widgets.ScrollView
	content               *widget.Container
}

func placed(rect goimage.Rectangle) *widget.Button {
	b := widget.NewButton()
	b.GetWidget().Rect = rect
	return b
}

func newScrollNavScreen(contentH int) *scrollNavScreen {
	s := &scrollNavScreen{b: &BaseScreen{}}
	s.b.InitBase()

	content := widget.NewContainer()
	s.content = content
	s.row0 = placed(goimage.Rect(200, 60, 650, 100))
	s.row1 = placed(goimage.Rect(200, 110, 650, 150))
	content.AddChild(s.row0, s.row1)
	s.view = widgets.NewScrollView(content, color.Black, true)
	s.view.SetLocation(goimage.Rect(200, 50, 700, 600))
	content.SetLocation(goimage.Rect(200, 50, 700, 50+contentH))
	s.bar = widget.NewSlider()
	s.bar.GetWidget().Rect = goimage.Rect(700, 50, 720, 600)

	s.side = placed(goimage.Rect(0, 100, 100, 140))
	s.tool = placed(goimage.Rect(700, 0, 800, 40))
	s.act = placed(goimage.Rect(750, 300, 850, 340))

	s.b.RegisterFocusButton("side", s.side)
	s.b.RegisterFocusButton("tool", s.tool)
	s.b.RegisterFocusButton("row0", s.row0)
	s.b.RegisterFocusButton("row1", s.row1)
	s.b.RegisterFocusButton("act", s.act)
	s.b.RegisterNavZone(sidebarZone, types.NavZoneVertical, []string{"side"}, 0)
	s.b.RegisterNavZone("toolbar", types.NavZoneHorizontal, []string{"tool"}, 0)
	s.b.RegisterNavZone("rows", types.NavZoneVertical, []string{"row0", "row1"}, 0)
	s.b.RegisterNavZone("actions", types.NavZoneHorizontal, []string{"act"}, 0)
	s.b.SetNavTransition("rows", types.DirRight, "actions", types.NavIndexFirst)
	s.b.SetScrollContainer(s.view)
	s.b.RegisterScrollBar("bar", s.view, s.bar)
	return s
}

func TestScrollBarRightFromContentEdge(t *testing.T) {
	s := newScrollNavScreen(1000)
	if got := s.b.FindFocusInDirection(s.row1, types.DirRight); got != widget.Focuser(s.bar) {
		t.Fatalf("Right from a row = %v, want the scroll bar", got)
	}
}

func TestScrollBarLeftToNearestVisible(t *testing.T) {
	s := newScrollNavScreen(1000)
	below := placed(goimage.Rect(200, 700, 650, 740)) // past the bottom of the view
	s.content.AddChild(below)
	s.b.RegisterFocusButton("below", below)
	s.b.FindFocusInDirection(s.row1, types.DirRight)

	// At the top, the row nearest the top of the view, not the row the bar
	// was entered from
	if got := s.b.FindFocusInDirection(s.bar, types.DirLeft); got != widget.Focuser(s.row0) {
		t.Errorf("Left at the top = %v, want the first row", got)
	}
	// At the bottom, the visible row nearest the bottom; the row past the
	// view is not chosen
	s.view.SetScrollTop(1)
	if got := s.b.FindFocusInDirection(s.bar, types.DirLeft); got != widget.Focuser(s.row1) {
		t.Errorf("Left at the bottom = %v, want the last visible row", got)
	}
}

func TestScrollBarLeftWithNothingVisible(t *testing.T) {
	s := newScrollNavScreen(1000)
	s.row0.GetWidget().Rect = goimage.Rect(200, 2000, 650, 2040)
	s.row1.GetWidget().Rect = goimage.Rect(200, 2050, 650, 2090)
	s.b.FindFocusInDirection(s.side, types.DirRight)
	if got := s.b.FindFocusInDirection(s.bar, types.DirLeft); got != widget.Focuser(s.side) {
		t.Errorf("Left with no visible row = %v, want where the bar was entered from", got)
	}
}

func TestScrollBarLeavesSpatially(t *testing.T) {
	s := newScrollNavScreen(1000)
	if got := s.b.FindFocusInDirection(s.bar, types.DirRight); got != widget.Focuser(s.act) {
		t.Errorf("Right from the scroll bar = %v, want the action", got)
	}
	if got := s.b.FindFocusInDirection(s.bar, types.DirUp); got != widget.Focuser(s.tool) {
		t.Errorf("Up from the scroll bar = %v, want the toolbar, not a row in the view", got)
	}
}

func TestScrollBarFromSidebarWithoutPageZone(t *testing.T) {
	s := newScrollNavScreen(1000)
	if got := s.b.FindFocusInDirection(s.side, types.DirRight); got != widget.Focuser(s.bar) {
		t.Fatalf("Right from the sidebar = %v, want the page scroll bar", got)
	}
}

func TestScrollBarScrollBy(t *testing.T) {
	s := newScrollNavScreen(1000)
	if !s.b.ScrollBy(s.bar, types.DirDown) || s.view.ScrollTop == 0 {
		t.Error("Down on the scroll bar did not scroll")
	}
	if !s.b.ScrollBy(s.bar, types.DirUp) || s.view.ScrollTop != 0 {
		t.Error("Up on the scroll bar did not scroll back")
	}
	if s.b.ScrollBy(s.bar, types.DirUp) {
		t.Error("Up at the top scrolled")
	}
	if s.b.ScrollBy(s.bar, types.DirLeft) || s.b.ScrollBy(s.row0, types.DirDown) {
		t.Error("scrolled for Left, or for a target that is not a scroll bar")
	}

	fits := newScrollNavScreen(100)
	if fits.b.ScrollBy(fits.bar, types.DirDown) {
		t.Error("scrolled content that fits")
	}
	if !fits.b.IsFocusTarget(fits.bar) {
		t.Error("scroll bar of content that fits is not a target")
	}
}

func TestClearFocusButtonsClearsScrollBars(t *testing.T) {
	s := newScrollNavScreen(1000)
	s.b.ClearFocusButtons()
	if s.b.ScrollBy(s.bar, types.DirDown) || s.b.IsFocusTarget(s.bar) {
		t.Error("scroll bar still registered after ClearFocusButtons")
	}
}
