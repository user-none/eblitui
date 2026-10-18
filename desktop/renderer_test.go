// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"bytes"
	"testing"
)

func TestDownscaleFactor(t *testing.T) {
	tests := []struct {
		name                   string
		width, height, maxSize int
		want                   int
	}{
		{"fits", 704, 512, 16384, 1},
		{"at the limit", 16384, 16384, 16384, 1},
		{"width over", 16385, 512, 16384, 2},
		{"height over", 704, 40000, 16384, 3},
		{"larger side decides", 50000, 20000, 16384, 4},
		{"size unknown", 50000, 20000, 0, 1},
	}
	for _, tt := range tests {
		if got := downscaleFactor(tt.width, tt.height, tt.maxSize); got != tt.want {
			t.Errorf("%s: got %d, want %d", tt.name, got, tt.want)
		}
	}
}

// rgba returns width x height RGBA pixels from fn, called per pixel
func rgba(width, height int, fn func(x, y int) [4]byte) []byte {
	out := make([]byte, 0, width*height*4)
	for y := range height {
		for x := range width {
			p := fn(x, y)
			out = append(out, p[:]...)
		}
	}
	return out
}

func TestDownscaleBoxAverages(t *testing.T) {
	// 4x2 frame, k=2: each 2x2 block averages to one pixel
	src := rgba(4, 2, func(x, y int) [4]byte {
		v := byte(x*10 + y*20)
		return [4]byte{v, v + 1, v + 2, 255}
	})

	got, w, h := downscaleBox(nil, src, 4, 2, 2)
	if w != 2 || h != 1 {
		t.Fatalf("size = %dx%d, want 2x1", w, h)
	}
	// Block 0: x 0-1, y 0-1 -> values 0, 10, 20, 30; block 1: 20, 30, 40, 50
	want := []byte{15, 16, 17, 255, 35, 36, 37, 255}
	if !bytes.Equal(got, want) {
		t.Errorf("pixels = %v, want %v", got, want)
	}
}

func TestDownscaleBoxPartialEdgeBlocks(t *testing.T) {
	// 3x3 frame, k=2: the right column and bottom row are partial blocks
	// that average only the pixels the frame has
	src := rgba(3, 3, func(x, y int) [4]byte {
		return [4]byte{byte(x * 10), byte(y * 10), 0, 255}
	})

	got, w, h := downscaleBox(nil, src, 3, 3, 2)
	if w != 2 || h != 2 {
		t.Fatalf("size = %dx%d, want 2x2", w, h)
	}
	want := []byte{
		5, 5, 0, 255, // x 0-1, y 0-1
		20, 5, 0, 255, // x 2, y 0-1
		5, 20, 0, 255, // x 0-1, y 2
		20, 20, 0, 255, // x 2, y 2
	}
	if !bytes.Equal(got, want) {
		t.Errorf("pixels = %v, want %v", got, want)
	}
}

func TestDownscaleBoxReusesBuffer(t *testing.T) {
	src := rgba(4, 4, func(x, y int) [4]byte { return [4]byte{1, 2, 3, 4} })
	buf := make([]byte, 64)

	got, _, _ := downscaleBox(buf, src, 4, 4, 2)
	if &got[0] != &buf[0] {
		t.Error("large enough buffer not reused")
	}
	if len(got) != 16 {
		t.Errorf("len = %d, want 16", len(got))
	}

	small := make([]byte, 4)
	got, _, _ = downscaleBox(small, src, 4, 4, 2)
	if len(got) != 16 {
		t.Errorf("grown len = %d, want 16", len(got))
	}
}
