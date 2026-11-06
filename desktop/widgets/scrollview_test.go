// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	goimage "image"
	"image/color"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
)

// testScrollView creates a view 100 high over content contentH high, both
// at the origin
func testScrollView(contentH int) (*ScrollView, *widget.Container) {
	content := widget.NewContainer()
	s := NewScrollView(content, color.Black, true)
	s.SetLocation(goimage.Rect(0, 0, 50, 100))
	content.SetLocation(goimage.Rect(0, 0, 50, contentH))
	return s, content
}

func TestScrollViewScrollStep(t *testing.T) {
	s, _ := testScrollView(500) // 400 px of scroll range, 25 px per step

	if s.ScrollStep(-1) {
		t.Error("scrolled up at the top")
	}
	if !s.ScrollStep(1) || s.ScrollTop != 25.0/400 {
		t.Errorf("step down: ScrollTop %v, want %v", s.ScrollTop, 25.0/400)
	}
	s.SetScrollTop(1)
	if s.ScrollStep(1) {
		t.Error("scrolled down at the bottom")
	}
	if !s.ScrollStep(-1) || s.ScrollTop != 375.0/400 {
		t.Errorf("step up from the bottom: ScrollTop %v, want %v", s.ScrollTop, 375.0/400)
	}
}

func TestScrollViewScrollStepFits(t *testing.T) {
	s, _ := testScrollView(80)
	if s.ScrollStep(1) || s.ScrollStep(-1) {
		t.Error("scrolled content that fits")
	}
}

func TestScrollViewContains(t *testing.T) {
	s, content := testScrollView(500)
	inner := widget.NewContainer()
	btn := widget.NewButton()
	inner.AddChild(btn)
	content.AddChild(inner)

	if !s.Contains(btn.GetWidget()) || !s.Contains(content.GetWidget()) {
		t.Error("content widgets not contained")
	}
	if s.Contains(widget.NewButton().GetWidget()) || s.Contains(nil) {
		t.Error("outside widget contained")
	}
}

func TestScrollViewScrollRectToCenter(t *testing.T) {
	tests := []struct {
		name  string
		top   int
		start float64
		want  float64
	}{
		{"middle", 240, 0, 200.0 / 400},
		{"middle from scrolled", 40, 0.5, 200.0 / 400},
		{"clamped at top", 20, 0, 0},
		{"clamped at bottom", 480, 0, 1},
		{"clamped at bottom from scrolled", 80, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := testScrollView(500) // 400 px of scroll range
			s.SetScrollTop(tt.start)
			s.ScrollRectToCenter(goimage.Rect(0, tt.top, 50, tt.top+20))
			if s.ScrollTop != tt.want {
				t.Errorf("ScrollTop %v, want %v", s.ScrollTop, tt.want)
			}
		})
	}
}

func TestScrollViewScrollRectToCenterFits(t *testing.T) {
	s, _ := testScrollView(80)
	s.ScrollRectToCenter(goimage.Rect(0, 60, 50, 80))
	if s.ScrollTop != 0 {
		t.Errorf("ScrollTop %v, want 0 for content that fits", s.ScrollTop)
	}
}
