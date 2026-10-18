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

	var d boxDownscaler
	got, w, h := d.downscale(src, 4, 2, 2)
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

	var d boxDownscaler
	got, w, h := d.downscale(src, 3, 3, 2)
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

func TestDownscaleBoxReusesBuffers(t *testing.T) {
	var d boxDownscaler
	large := rgba(8, 8, func(x, y int) [4]byte { return [4]byte{200, 200, 200, 255} })
	d.downscale(large, 8, 8, 2)
	out, sums := &d.out[0], &d.sums[0]

	// A smaller frame reuses both buffers, and the sums left by the larger
	// frame do not carry into its averages
	small := rgba(4, 4, func(x, y int) [4]byte { return [4]byte{10, 20, 30, 255} })
	got, w, h := d.downscale(small, 4, 4, 2)
	if &d.out[0] != out || &d.sums[0] != sums {
		t.Error("buffers not reused")
	}
	if w != 2 || h != 2 || len(got) != 16 {
		t.Fatalf("size = %dx%d len %d, want 2x2 len 16", w, h, len(got))
	}
	for i := 0; i < len(got); i += 4 {
		if p := got[i : i+4]; !bytes.Equal(p, []byte{10, 20, 30, 255}) {
			t.Errorf("pixel %d = %v, want [10 20 30 255]", i/4, p)
		}
	}

	// A larger frame grows them
	d.downscale(rgba(16, 16, func(x, y int) [4]byte { return [4]byte{} }), 16, 16, 2)
	if len(d.out) < 8*8*4 || len(d.sums) < 8*4 {
		t.Errorf("buffers not grown: out %d sums %d", len(d.out), len(d.sums))
	}
}
