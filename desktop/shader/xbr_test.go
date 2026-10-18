package shader

import "testing"

func TestSelectOptimalScale(t *testing.T) {
	tests := []struct {
		name                         string
		srcW, srcH, screenW, screenH int
		expected                     int
	}{
		// SMS at 256x192
		{"SMS to 512x384 (exact 2x)", 256, 192, 512, 384, 2},
		{"SMS to 640x480", 256, 192, 640, 480, 4}, // scaleToFit=2.5, needs 4x
		{"SMS to 800x600", 256, 192, 800, 600, 4},
		{"SMS to 1024x768 (exact 4x)", 256, 192, 1024, 768, 4},
		{"SMS to 1280x720 (720p)", 256, 192, 1280, 720, 4},
		{"SMS to 1920x1080 (1080p)", 256, 192, 1920, 1080, 8},
		{"SMS to 2048x1536 (exact 8x)", 256, 192, 2048, 1536, 8},
		{"SMS to 2560x1440 (1440p)", 256, 192, 2560, 1440, 8},
		{"SMS to 3840x2160 (4K)", 256, 192, 3840, 2160, 8},

		// Edge cases - screen smaller than 2x
		{"SMS to 400x300", 256, 192, 400, 300, 2},
		{"SMS to 256x192 (1:1)", 256, 192, 256, 192, 2},

		// Non-standard aspect ratios
		{"SMS to ultrawide 2560x1080", 256, 192, 2560, 1080, 8},
		{"SMS to tall 1080x1920", 256, 192, 1080, 1920, 8}, // scaleToFit=4.22, needs 8x
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectOptimalScale(tt.srcW, tt.srcH, tt.screenW, tt.screenH)
			if got != tt.expected {
				t.Errorf("selectOptimalScale(%d, %d, %d, %d) = %d, want %d",
					tt.srcW, tt.srcH, tt.screenW, tt.screenH, got, tt.expected)
			}
		})
	}
}

func TestScaleFactorToPasses(t *testing.T) {
	tests := []struct {
		factor   int
		expected int
	}{
		{2, 1},
		{4, 2},
		{8, 3},
		{1, 1},  // Default case
		{16, 1}, // Default case
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := scaleFactorToPasses(tt.factor)
			if got != tt.expected {
				t.Errorf("scaleFactorToPasses(%d) = %d, want %d", tt.factor, got, tt.expected)
			}
		})
	}
}

func TestFitPasses(t *testing.T) {
	tests := []struct {
		name                        string
		srcW, srcH, passes, maxSize int
		expected                    int
	}{
		{"all passes fit", 352, 224, 3, 16384, 3},
		{"output at the limit", 8192, 224, 1, 16384, 1},
		{"one over the limit", 8193, 224, 1, 16384, 0},
		{"16x low-res keeps one pass", 5632, 3584, 1, 16384, 1},
		{"16x hi-res drops every pass", 11264, 3584, 1, 16384, 0},
		{"three reduced to two", 4096, 224, 3, 16384, 2},
		{"three reduced to one", 4097, 224, 3, 16384, 1},
		{"height over", 352, 5000, 2, 16384, 1},
		{"8192 limit", 5632, 3584, 1, 8192, 0},
		{"no passes requested", 352, 224, 0, 16384, 0},
		{"size unknown", 352, 224, 3, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fitPasses(tt.srcW, tt.srcH, tt.passes, tt.maxSize)
			if got != tt.expected {
				t.Errorf("fitPasses(%d, %d, %d, %d) = %d, want %d",
					tt.srcW, tt.srcH, tt.passes, tt.maxSize, got, tt.expected)
			}
		})
	}
}
