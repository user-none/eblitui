// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	goimage "image"
	"math"
	"testing"
	"time"

	"github.com/ebitenui/ebitenui/event"
	"github.com/hajimehoshi/ebiten/v2"
)

func near(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func TestCarouselRectsSingleFillsRect(t *testing.T) {
	rect := goimage.Rect(0, 0, 1000, 500)
	r := carouselRects(rect, []float64{0.5}, 0, 20, 10, 40)[0]
	// Height-limited: 480 high, 240 wide, centered, under the caption
	if !near(r.y1-r.y0, 480) || !near(r.x1-r.x0, 240) {
		t.Errorf("size %vx%v, want 240x480", r.x1-r.x0, r.y1-r.y0)
	}
	if !near(r.x0, 380) || !near(r.y0, 20) {
		t.Errorf("origin %v,%v, want 380,20", r.x0, r.y0)
	}
}

func TestCarouselRectsKeepsPeekRoom(t *testing.T) {
	// Width-limited image: the slices at each side take peek+gap
	rect := goimage.Rect(0, 0, 1000, 1000)
	rects := carouselRects(rect, []float64{1, 1}, 0, 0, 10, 40)
	cur, next := rects[0], rects[1]
	if !near(cur.x1-cur.x0, 900) {
		t.Errorf("current width %v, want 900", cur.x1-cur.x0)
	}
	if !near(next.x0, cur.x1+10) || !near(next.y0, cur.y0) || !near(next.y1, cur.y1) {
		t.Errorf("next %+v does not follow current %+v at its height", next, cur)
	}
	if visible := float64(rect.Max.X) - next.x0; visible < 40 {
		t.Errorf("visible slice %v, want at least 40", visible)
	}
}

func TestCarouselRectsSlicesAtEdges(t *testing.T) {
	// Height-limited image with room to spare: the images beside it show
	// exactly peek wide at the edges, at its height
	rect := goimage.Rect(100, 0, 1100, 400)
	rects := carouselRects(rect, []float64{1, 0.5, 2}, 1, 0, 10, 40)
	cur := rects[1]
	if !near(cur.x0, 500) || !near(cur.x1-cur.x0, 200) {
		t.Errorf("current %+v, want 200 wide at 500", cur)
	}
	if !near(rects[0].x1, 140) || !near(rects[0].x1-rects[0].x0, 400) {
		t.Errorf("before %+v, want 400 wide ending at 140", rects[0])
	}
	if !near(rects[2].x0, 1060) || !near(rects[2].x1-rects[2].x0, 800) {
		t.Errorf("after %+v, want 800 wide starting at 1060", rects[2])
	}
}

func TestCarouselRectsRowContinues(t *testing.T) {
	rect := goimage.Rect(0, 0, 1000, 400)
	rects := carouselRects(rect, []float64{1, 1, 0.5, 1, 1}, 2, 0, 10, 40)
	if !near(rects[4].x0, rects[3].x1+10) || !near(rects[0].x1, rects[1].x0-10) {
		t.Errorf("row not continued gap apart: %+v", rects)
	}
}

func TestCarouselRectsTooSmall(t *testing.T) {
	r := carouselRects(goimage.Rect(0, 0, 50, 10), []float64{1, 1}, 0, 20, 10, 40)[0]
	if r.x1-r.x0 < 0 || r.y1-r.y0 < 0 {
		t.Errorf("negative size %+v", r)
	}
}

func TestSlideProgress(t *testing.T) {
	if p := slideProgress(0); p != 0 {
		t.Errorf("start %v, want 0", p)
	}
	if p := slideProgress(carouselSlide); p != 1 {
		t.Errorf("end %v, want 1", p)
	}
	if p := slideProgress(-time.Second); p != 0 {
		t.Errorf("before start %v, want 0", p)
	}
	half := slideProgress(carouselSlide / 2)
	if half <= 0.5 || half >= 1 {
		t.Errorf("half way %v, want eased out past 0.5", half)
	}
}

func testCarousel(t *testing.T, index, n int) (*Carousel, *[]int) {
	t.Helper()
	imgs := make([]*ebiten.Image, n)
	labels := make([]string, n)
	for i := range imgs {
		imgs[i] = ebiten.NewImage(10, 20)
		labels[i] = "L"
	}
	var moves []int
	c := NewCarousel(CarouselOptions{
		Images: imgs, Labels: labels, Index: index, From: index,
		Now: time.Now, Gap: 10, Peek: 40,
		Move: func(dir int) { moves = append(moves, dir) },
	})
	return c, &moves
}

func TestCarouselButtonsAtEnds(t *testing.T) {
	tests := []struct {
		index, n   int
		prev, next bool
	}{
		{0, 1, false, false},
		{0, 2, false, true},
		{1, 2, true, false},
		{1, 3, true, true},
	}
	for _, tt := range tests {
		c, _ := testCarousel(t, tt.index, tt.n)
		if (c.Prev() != nil) != tt.prev || (c.Next() != nil) != tt.next {
			t.Errorf("index %d of %d: prev %v next %v, want %v %v",
				tt.index, tt.n, c.Prev() != nil, c.Next() != nil, tt.prev, tt.next)
		}
	}
}

func TestCarouselButtonsCoverSlices(t *testing.T) {
	c, moves := testCarousel(t, 1, 3)
	rect := goimage.Rect(100, 50, 1100, 650)
	c.Container().SetLocation(rect)
	c.Container().Validate()
	carouselLayout{c}.Layout(nil, rect)

	rects := carouselRects(rect, c.aspects, 1, c.captionH, 10, 40)
	if got, want := c.Prev().GetWidget().Rect, rects[0].pixels().Intersect(rect); got != want || got.Empty() {
		t.Errorf("prev at %v, want %v", got, want)
	}
	if got, want := c.Next().GetWidget().Rect, rects[2].pixels().Intersect(rect); got != want || got.Empty() {
		t.Errorf("next at %v, want %v", got, want)
	}

	c.Prev().Click()
	c.Next().Click()
	event.ExecuteDeferred()
	if len(*moves) != 2 || (*moves)[0] != -1 || (*moves)[1] != 1 {
		t.Errorf("moves %v, want [-1 1]", *moves)
	}
}
