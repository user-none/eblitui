package emucore

import (
	"math"
	"testing"
)

func TestDisplayAspectRatio(t *testing.T) {
	tests := []struct {
		name     string
		width    int
		height   int
		par      float64
		expected float64
	}{
		{
			name:     "Genesis H40 224 lines",
			width:    320,
			height:   224,
			par:      32.0 / 35.0,
			expected: (320.0 / 224.0) * (32.0 / 35.0),
		},
		{
			name:     "SMS 192 lines",
			width:    256,
			height:   192,
			par:      8.0 / 7.0,
			expected: (256.0 / 192.0) * (8.0 / 7.0),
		},
		{
			name:     "SMS 224 lines",
			width:    256,
			height:   224,
			par:      8.0 / 7.0,
			expected: (256.0 / 224.0) * (8.0 / 7.0),
		},
		{
			name:     "Square pixels",
			width:    320,
			height:   240,
			par:      1.0,
			expected: 320.0 / 240.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DisplayAspectRatio(tt.width, tt.height, tt.par)
			if math.Abs(got-tt.expected) > 1e-9 {
				t.Errorf("DisplayAspectRatio(%d, %d, %f) = %f, want %f",
					tt.width, tt.height, tt.par, got, tt.expected)
			}
		})
	}
}
