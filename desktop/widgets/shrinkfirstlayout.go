// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	goimage "image"

	"github.com/ebitenui/ebitenui/widget"
)

// ShrinkFirstLayout stacks its widgets top to bottom at the container
// width, each at its preferred height, directly under one another. When
// the stack is taller than the container, only the first widget shrinks,
// so the ones below it stay in place and are never pushed out. It lays out
// a column whose head adapts to the height left by its tail, such as box
// art over a sidebar.
type ShrinkFirstLayout struct {
	spacing int
}

// NewShrinkFirstLayout creates a ShrinkFirstLayout with spacing pixels
// between widgets.
func NewShrinkFirstLayout(spacing int) *ShrinkFirstLayout {
	return &ShrinkFirstLayout{spacing: spacing}
}

// PreferredSize reports the widest widget and the height of the stack.
func (l *ShrinkFirstLayout) PreferredSize(widgets []widget.PreferredSizeLocateableWidget) (int, int) {
	width, height := 0, 0
	n := 0
	for _, w := range widgets {
		if w.GetWidget().GetVisibility() == widget.Visibility_Hide {
			continue
		}
		ww, wh := w.PreferredSize()
		width = max(width, ww)
		height += wh
		n++
	}
	if n > 1 {
		height += (n - 1) * l.spacing
	}
	return width, height
}

// Layout places the widgets in rect.
func (l *ShrinkFirstLayout) Layout(widgets []widget.PreferredSizeLocateableWidget, rect goimage.Rectangle) {
	var shown []widget.PreferredSizeLocateableWidget
	for _, w := range widgets {
		if w.GetWidget().GetVisibility() != widget.Visibility_Hide {
			shown = append(shown, w)
		}
	}
	if len(shown) == 0 {
		return
	}

	// The first widget gets what the rest leave, up to its preference
	heights := make([]int, len(shown))
	tail := (len(shown) - 1) * l.spacing
	for i, w := range shown {
		_, heights[i] = w.PreferredSize()
		if i > 0 {
			tail += heights[i]
		}
	}
	heights[0] = max(0, min(heights[0], rect.Dy()-tail))

	y := rect.Min.Y
	for i, w := range shown {
		w.SetLocation(goimage.Rect(rect.Min.X, y, rect.Max.X, y+heights[i]))
		y += heights[i] + l.spacing
	}
}
