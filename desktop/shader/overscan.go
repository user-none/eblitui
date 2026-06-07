package shader

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Overscan crop amounts are the TOTAL fraction removed across both opposing
// edges of the native frame: 8% of width (4% per side) and 6% of height
// (3% per side).
const (
	overscanWidthPct  = 0.08
	overscanHeightPct = 0.06
)

// hasOverscan returns true if "overscan" is in the shader list.
func hasOverscan(shaderIDs []string) bool {
	for _, id := range shaderIDs {
		if id == "overscan" {
			return true
		}
	}
	return false
}

// overscanInset computes the per-side crop in pixels for a single dimension.
// The total crop is floored to an integer, forced even, then halved so the
// crop is symmetric and integer on both edges. Returns 0 when the computed
// crop is non-positive or would consume the whole dimension.
func overscanInset(dim int, pct float64) int {
	total := int(float64(dim) * pct)
	if total%2 != 0 {
		total++
	}
	if total <= 0 || total >= dim {
		return 0
	}
	return total / 2
}

// overscanInsets returns the per-side crop in pixels for the given native
// frame dimensions.
func overscanInsets(w, h int) (left, right, top, bottom int) {
	ph := overscanInset(w, overscanWidthPct)
	pv := overscanInset(h, overscanHeightPct)
	return ph, ph, pv, pv
}

// cropOverscan returns a view of src with the overscan border removed. The
// result is a SubImage (no pixel copy); src is returned unchanged when no crop
// applies.
func cropOverscan(src *ebiten.Image) *ebiten.Image {
	if src == nil {
		return nil
	}
	b := src.Bounds()
	left, right, top, bottom := overscanInsets(b.Dx(), b.Dy())
	if left == 0 && right == 0 && top == 0 && bottom == 0 {
		return src
	}
	rect := image.Rect(b.Min.X+left, b.Min.Y+top, b.Max.X-right, b.Max.Y-bottom)
	return src.SubImage(rect).(*ebiten.Image)
}
