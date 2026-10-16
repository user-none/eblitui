// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	goimage "image"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
)

// sizedBox creates an empty container that prefers the given size
func sizedBox(w, h int) *widget.Container {
	return widget.NewContainer(widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(w, h)))
}

func TestShrinkFirstLayoutPreferredSize(t *testing.T) {
	l := NewShrinkFirstLayout(10)
	head, tail := sizedBox(200, 300), sizedBox(180, 100)
	w, h := l.PreferredSize([]widget.PreferredSizeLocateableWidget{head, tail})
	if w != 200 || h != 410 {
		t.Errorf("PreferredSize = %dx%d, want 200x410", w, h)
	}
}

func TestShrinkFirstLayoutStacksWhenRoomy(t *testing.T) {
	l := NewShrinkFirstLayout(10)
	head, tail := sizedBox(200, 300), sizedBox(180, 100)
	l.Layout([]widget.PreferredSizeLocateableWidget{head, tail}, goimage.Rect(0, 0, 200, 600))

	if got := head.GetWidget().Rect; got != goimage.Rect(0, 0, 200, 300) {
		t.Errorf("head = %v, want its preferred height at the top", got)
	}
	// The tail sits directly under the head, not at the bottom
	if got := tail.GetWidget().Rect; got != goimage.Rect(0, 310, 200, 410) {
		t.Errorf("tail = %v, want directly under the head", got)
	}
}

func TestShrinkFirstLayoutShrinksHeadOnly(t *testing.T) {
	l := NewShrinkFirstLayout(10)
	head, tail := sizedBox(200, 300), sizedBox(180, 100)
	l.Layout([]widget.PreferredSizeLocateableWidget{head, tail}, goimage.Rect(0, 0, 200, 250))

	if got := head.GetWidget().Rect; got != goimage.Rect(0, 0, 200, 140) {
		t.Errorf("head = %v, want shrunk to what the tail leaves", got)
	}
	if got := tail.GetWidget().Rect; got != goimage.Rect(0, 150, 200, 250) {
		t.Errorf("tail = %v, want its full height at the bottom", got)
	}

	// A container shorter than the tail leaves the head no height
	l.Layout([]widget.PreferredSizeLocateableWidget{head, tail}, goimage.Rect(0, 0, 200, 50))
	if got := head.GetWidget().Rect.Dy(); got != 0 {
		t.Errorf("head height = %d, want 0", got)
	}
}

func TestShrinkFirstLayoutSkipsHidden(t *testing.T) {
	l := NewShrinkFirstLayout(10)
	head, hidden, tail := sizedBox(200, 300), sizedBox(200, 50), sizedBox(180, 100)
	hidden.GetWidget().SetVisibility(widget.Visibility_Hide)
	all := []widget.PreferredSizeLocateableWidget{head, hidden, tail}

	if _, h := l.PreferredSize(all); h != 410 {
		t.Errorf("PreferredSize height = %d, want 410 without the hidden widget", h)
	}
	l.Layout(all, goimage.Rect(0, 0, 200, 600))
	if got := tail.GetWidget().Rect; got != goimage.Rect(0, 310, 200, 410) {
		t.Errorf("tail = %v, want directly under the head", got)
	}
}
