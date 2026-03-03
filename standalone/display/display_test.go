package display

import (
	"math"
	"testing"
)

const tolerance = 0.001

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) < tol
}

func TestSizeStretch(t *testing.T) {
	w, h := Size("stretch", 800, 600, 256, 224, 1.0)
	if w != 800 || h != 600 {
		t.Errorf("stretch: got (%v, %v), want (800, 600)", w, h)
	}
}

func TestSize43Landscape(t *testing.T) {
	// 800x600 screen, 4:3 mode: 800 / (4/3) = 600, fits exactly
	w, h := Size("4:3", 800, 600, 256, 224, 1.0)
	if !almostEqual(w, 800, tolerance) || !almostEqual(h, 600, tolerance) {
		t.Errorf("4:3 landscape: got (%v, %v), want (800, 600)", w, h)
	}
}

func TestSize43Portrait(t *testing.T) {
	// 600x800 screen, 4:3 mode: 600 / (4/3) = 450 < 800, fits by width
	w, h := Size("4:3", 600, 800, 256, 224, 1.0)
	if !almostEqual(w, 600, tolerance) || !almostEqual(h, 450, tolerance) {
		t.Errorf("4:3 portrait: got (%v, %v), want (600, 450)", w, h)
	}
}

func TestSize43HeightConstrained(t *testing.T) {
	// 1200x400 screen: 1200 / (4/3) = 900 > 400, so constrain by height
	w, h := Size("4:3", 1200, 400, 256, 224, 1.0)
	expectedH := 400.0
	expectedW := 400.0 * (4.0 / 3.0)
	if !almostEqual(w, expectedW, tolerance) || !almostEqual(h, expectedH, tolerance) {
		t.Errorf("4:3 height-constrained: got (%v, %v), want (%v, %v)", w, h, expectedW, expectedH)
	}
}

func TestSizeDARLandscape(t *testing.T) {
	// 256x224 source with PAR 1.1458 -> DAR = (256/224)*1.1458 = 1.3095
	// 800x600 screen: 800/1.3095 = 610.92 > 600 -> constrain by height
	w, h := Size("dar", 800, 600, 256, 224, 1.1458)
	dar := (256.0 / 224.0) * 1.1458
	expectedH := 600.0
	expectedW := 600.0 * dar
	if !almostEqual(w, expectedW, tolerance) || !almostEqual(h, expectedH, tolerance) {
		t.Errorf("dar landscape: got (%v, %v), want (%v, %v)", w, h, expectedW, expectedH)
	}
}

func TestSizeDARWidthFit(t *testing.T) {
	// 640x800 screen, 256x224 with PAR 1.0 -> DAR = 256/224 = 1.1428
	// 640/1.1428 = 560.01 < 800 -> fits by width
	w, h := Size("dar", 640, 800, 256, 224, 1.0)
	dar := 256.0 / 224.0
	expectedW := 640.0
	expectedH := 640.0 / dar
	if !almostEqual(w, expectedW, tolerance) || !almostEqual(h, expectedH, tolerance) {
		t.Errorf("dar width fit: got (%v, %v), want (%v, %v)", w, h, expectedW, expectedH)
	}
}

func TestSizeDefaultMode(t *testing.T) {
	// Empty string defaults to DAR
	w1, h1 := Size("", 800, 600, 256, 224, 1.0)
	w2, h2 := Size("dar", 800, 600, 256, 224, 1.0)
	if w1 != w2 || h1 != h2 {
		t.Errorf("default vs dar: (%v,%v) != (%v,%v)", w1, h1, w2, h2)
	}
}

func TestScaleAndCenter(t *testing.T) {
	// 512x448 display from 256x224 source on 800x600 screen
	scaleX, scaleY, offsetX, offsetY := ScaleAndCenter(512, 448, 256, 224, 800, 600)

	if !almostEqual(scaleX, 2.0, tolerance) {
		t.Errorf("scaleX: got %v, want 2.0", scaleX)
	}
	if !almostEqual(scaleY, 2.0, tolerance) {
		t.Errorf("scaleY: got %v, want 2.0", scaleY)
	}
	// Center: (800-512)/2 = 144, (600-448)/2 = 76
	if !almostEqual(offsetX, 144, tolerance) {
		t.Errorf("offsetX: got %v, want 144", offsetX)
	}
	if !almostEqual(offsetY, 76, tolerance) {
		t.Errorf("offsetY: got %v, want 76", offsetY)
	}
}

func TestScaleAndCenterFullFit(t *testing.T) {
	// Display fills screen exactly
	scaleX, scaleY, offsetX, offsetY := ScaleAndCenter(800, 600, 400, 300, 800, 600)

	if !almostEqual(scaleX, 2.0, tolerance) {
		t.Errorf("scaleX: got %v, want 2.0", scaleX)
	}
	if !almostEqual(scaleY, 2.0, tolerance) {
		t.Errorf("scaleY: got %v, want 2.0", scaleY)
	}
	if !almostEqual(offsetX, 0, tolerance) {
		t.Errorf("offsetX: got %v, want 0", offsetX)
	}
	if !almostEqual(offsetY, 0, tolerance) {
		t.Errorf("offsetY: got %v, want 0", offsetY)
	}
}

func TestDPIScale(t *testing.T) {
	// In test environments ebiten.Monitor() returns nil, so DPIScale
	// should return the fallback value of 1.0.
	s := DPIScale()
	if s < 1.0 {
		t.Errorf("DPIScale: got %v, want >= 1.0", s)
	}
}
