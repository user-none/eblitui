package shader

import (
	"image"
	"testing"
)

func TestPictureRect(t *testing.T) {
	tests := []struct {
		name string
		p    Picture
		want image.Rectangle
	}{
		{"pillarbox", Picture{Left: 480, Top: 0, Width: 2880, Height: 2160}, image.Rect(480, 0, 3360, 2160)},
		{"letterbox", Picture{Left: 0, Top: 75, Width: 1000, Height: 750}, image.Rect(0, 75, 1000, 825)},
		{"fractional edges round to nearest", Picture{Left: 6.5, Top: 2.4, Width: 10.25, Height: 7.2}, image.Rect(7, 2, 17, 10)},
		{"empty", Picture{}, image.Rectangle{}},
	}
	for _, tt := range tests {
		if got := tt.p.Rect(); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestFullPicture(t *testing.T) {
	if got := FullPicture(640, 480).Rect(); got != image.Rect(0, 0, 640, 480) {
		t.Errorf("got %v", got)
	}
}
