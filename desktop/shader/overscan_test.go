package shader

import "testing"

func TestOverscanInsets(t *testing.T) {
	tests := []struct {
		name                     string
		w, h                     int
		left, right, top, bottom int
	}{
		// 256*0.08=20.48 -> 20 (even) -> 10/side; 192*0.06=11.52 -> 11 -> 12 -> 6/side
		{"sms 256x192", 256, 192, 10, 10, 6, 6},
		// 320*0.08=25.6 -> 25 -> 26 -> 13/side; 224*0.06=13.44 -> 13 -> 14 -> 7/side
		{"md 320x224", 320, 224, 13, 13, 7, 7},
		// Tiny dimensions: computed crop would consume the whole frame -> no crop.
		{"degenerate", 4, 2, 0, 0, 0, 0},
		{"zero", 0, 0, 0, 0, 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			left, right, top, bottom := overscanInsets(tc.w, tc.h)
			if left != tc.left || right != tc.right || top != tc.top || bottom != tc.bottom {
				t.Errorf("overscanInsets(%d,%d) = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					tc.w, tc.h, left, right, top, bottom, tc.left, tc.right, tc.top, tc.bottom)
			}
			// Crop must be symmetric on opposing edges.
			if left != right {
				t.Errorf("horizontal crop not symmetric: left=%d right=%d", left, right)
			}
			if top != bottom {
				t.Errorf("vertical crop not symmetric: top=%d bottom=%d", top, bottom)
			}
			// Total removed per axis must stay within the dimension.
			if left+right >= tc.w && tc.w > 0 {
				t.Errorf("horizontal crop %d consumes whole width %d", left+right, tc.w)
			}
			if top+bottom >= tc.h && tc.h > 0 {
				t.Errorf("vertical crop %d consumes whole height %d", top+bottom, tc.h)
			}
		})
	}
}

func TestHasOverscan(t *testing.T) {
	tests := []struct {
		name     string
		ids      []string
		expected bool
	}{
		{"present", []string{"xbr", "overscan", "scanlines"}, true},
		{"only", []string{"overscan"}, true},
		{"absent", []string{"xbr", "ghosting"}, false},
		{"empty", nil, false},
	}

	for _, tc := range tests {
		got := hasOverscan(tc.ids)
		if got != tc.expected {
			t.Errorf("hasOverscan(%v) = %v, want %v", tc.ids, got, tc.expected)
		}
	}
}
