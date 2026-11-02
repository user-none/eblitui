// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	goimage "image"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
)

// captionedChildren returns a 50x20 caption and an empty image widget for
// each image, in the order the layout takes them
func captionedChildren(n int) []widget.PreferredSizeLocateableWidget {
	var out []widget.PreferredSizeLocateableWidget
	for range n {
		out = append(out, sizedBox(50, 20), sizedBox(0, 0))
	}
	return out
}

func rectOf(w widget.PreferredSizeLocateableWidget) goimage.Rectangle {
	return w.GetWidget().Rect
}

func TestCaptionedImagesSideBySide(t *testing.T) {
	// Two 3:4 covers on a wide page: side by side at the full height left
	// under the captions, the pair centered across the page
	l := NewCaptionedImagesLayout([]float64{0.75, 0.75}, 10, 4)
	ws := captionedChildren(2)
	l.Layout(ws, goimage.Rect(0, 0, 1000, 500))

	want := []goimage.Rectangle{
		goimage.Rect(138, 0, 188, 20),
		goimage.Rect(138, 24, 495, 500),
		goimage.Rect(505, 0, 555, 20),
		goimage.Rect(505, 24, 862, 500),
	}
	for i, r := range want {
		if got := rectOf(ws[i]); got != r {
			t.Errorf("child %d = %v, want %v", i, got, r)
		}
	}
}

func TestCaptionedImagesSharedHeight(t *testing.T) {
	// A square front and a wide back side by side share a height, set by
	// the page width
	l := NewCaptionedImagesLayout([]float64{1, 2}, 10, 4)
	ws := captionedChildren(2)
	l.Layout(ws, goimage.Rect(0, 0, 1210, 600))

	front, back := rectOf(ws[1]), rectOf(ws[3])
	if front.Dy() != back.Dy() {
		t.Errorf("heights %d and %d, want equal", front.Dy(), back.Dy())
	}
	if front.Dx() != 400 || back.Dx() != 800 || front.Dy() != 400 {
		t.Errorf("front %v back %v, want 400x400 and 800x400", front, back)
	}
	if front.Min.Y != back.Min.Y {
		t.Errorf("tops %d and %d, want aligned", front.Min.Y, back.Min.Y)
	}
}

func TestCaptionedImagesStacked(t *testing.T) {
	// A square front over a wide back on a tall page: stacked shows them
	// larger, at a shared width, the stack centered down the page
	l := NewCaptionedImagesLayout([]float64{1, 2}, 10, 4)
	ws := captionedChildren(2)
	l.Layout(ws, goimage.Rect(0, 0, 600, 1000))

	want := []goimage.Rectangle{
		goimage.Rect(0, 21, 50, 41),
		goimage.Rect(0, 45, 600, 645),
		goimage.Rect(0, 655, 50, 675),
		goimage.Rect(0, 679, 600, 979),
	}
	for i, r := range want {
		if got := rectOf(ws[i]); got != r {
			t.Errorf("child %d = %v, want %v", i, got, r)
		}
	}
}

func TestCaptionedImagesSingle(t *testing.T) {
	l := NewCaptionedImagesLayout([]float64{0.5}, 10, 4)
	ws := captionedChildren(1)
	l.Layout(ws, goimage.Rect(0, 0, 400, 400))

	if got := rectOf(ws[1]); got != goimage.Rect(106, 24, 294, 400) {
		t.Errorf("image = %v, want the full height, centered", got)
	}
	if got := rectOf(ws[0]); got != goimage.Rect(106, 0, 156, 20) {
		t.Errorf("caption = %v, want directly above the image", got)
	}
}

func TestCaptionedImagesTooSmall(t *testing.T) {
	l := NewCaptionedImagesLayout([]float64{0.75, 2}, 10, 4)
	ws := captionedChildren(2)
	l.Layout(ws, goimage.Rect(0, 0, 10, 10))
	for i, w := range ws {
		if r := rectOf(w); r.Dx() < 0 || r.Dy() < 0 {
			t.Errorf("child %d = %v, want no negative size", i, r)
		}
	}
}
