package libretro

import (
	"testing"
)

// TestConvertRGBAToXRGB8888_Basic verifies R,G,B channels swap correctly
func TestConvertRGBAToXRGB8888_Basic(t *testing.T) {
	// RGBA input: Red=0xFF, Green=0x80, Blue=0x40, Alpha=0x00
	src := []byte{0xFF, 0x80, 0x40, 0x00}
	dst := make([]byte, 4)

	convertRGBAToXRGB8888(src, dst, 1)

	// Expected XRGB8888 (little-endian): B=0x40, G=0x80, R=0xFF, X=0xFF
	expected := []byte{0x40, 0x80, 0xFF, 0xFF}

	for i := 0; i < 4; i++ {
		if dst[i] != expected[i] {
			t.Errorf("dst[%d] = %#02x, want %#02x", i, dst[i], expected[i])
		}
	}
}

// TestConvertRGBAToXRGB8888_Alpha verifies alpha becomes 0xFF
func TestConvertRGBAToXRGB8888_Alpha(t *testing.T) {
	testCases := []struct {
		name     string
		srcAlpha byte
	}{
		{"zero alpha", 0x00},
		{"half alpha", 0x80},
		{"full alpha", 0xFF},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte{0x12, 0x34, 0x56, tc.srcAlpha}
			dst := make([]byte, 4)

			convertRGBAToXRGB8888(src, dst, 1)

			if dst[3] != 0xFF {
				t.Errorf("X channel = %#02x, want 0xFF (input alpha was %#02x)", dst[3], tc.srcAlpha)
			}
		})
	}
}

// TestConvertRGBAToXRGB8888_Empty handles empty input
func TestConvertRGBAToXRGB8888_Empty(t *testing.T) {
	src := []byte{}
	dst := []byte{}

	// Should not panic
	convertRGBAToXRGB8888(src, dst, 0)
}

// TestConvertRGBAToXRGB8888_MultiplePixels tests conversion of multiple pixels
func TestConvertRGBAToXRGB8888_MultiplePixels(t *testing.T) {
	// Two pixels: Red and Blue
	src := []byte{
		0xFF, 0x00, 0x00, 0xFF, // Red pixel (RGBA)
		0x00, 0x00, 0xFF, 0xFF, // Blue pixel (RGBA)
	}
	dst := make([]byte, 8)

	convertRGBAToXRGB8888(src, dst, 2)

	expected := []byte{
		0x00, 0x00, 0xFF, 0xFF, // Red pixel in XRGB8888: B=0, G=0, R=FF, X=FF
		0xFF, 0x00, 0x00, 0xFF, // Blue pixel in XRGB8888: B=FF, G=0, R=0, X=FF
	}

	for i := 0; i < 8; i++ {
		if dst[i] != expected[i] {
			t.Errorf("dst[%d] = %#02x, want %#02x", i, dst[i], expected[i])
		}
	}
}

// TestReorderDefault verifies default value moves to front
func TestReorderDefault(t *testing.T) {
	values := []string{"a", "b", "c"}

	result := reorderDefault(values, "b")
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}
	if result[0] != "b" {
		t.Errorf("result[0] = %q, want \"b\"", result[0])
	}
	if result[1] != "a" {
		t.Errorf("result[1] = %q, want \"a\"", result[1])
	}
	if result[2] != "c" {
		t.Errorf("result[2] = %q, want \"c\"", result[2])
	}
}

// TestReorderDefault_AlreadyFirst verifies no change when default is first
func TestReorderDefault_AlreadyFirst(t *testing.T) {
	values := []string{"x", "y", "z"}

	result := reorderDefault(values, "x")
	if result[0] != "x" || result[1] != "y" || result[2] != "z" {
		t.Errorf("unexpected reorder: %v", result)
	}
}

// TestJoypadConstants verifies libretro button ID constants
func TestJoypadConstants(t *testing.T) {
	if JoypadB != 0 {
		t.Errorf("JoypadB = %d, want 0", JoypadB)
	}
	if JoypadY != 1 {
		t.Errorf("JoypadY = %d, want 1", JoypadY)
	}
	if JoypadSelect != 2 {
		t.Errorf("JoypadSelect = %d, want 2", JoypadSelect)
	}
	if JoypadStart != 3 {
		t.Errorf("JoypadStart = %d, want 3", JoypadStart)
	}
	if JoypadA != 8 {
		t.Errorf("JoypadA = %d, want 8", JoypadA)
	}
	if JoypadX != 9 {
		t.Errorf("JoypadX = %d, want 9", JoypadX)
	}
	if JoypadL != 10 {
		t.Errorf("JoypadL = %d, want 10", JoypadL)
	}
	if JoypadR != 11 {
		t.Errorf("JoypadR = %d, want 11", JoypadR)
	}
}
