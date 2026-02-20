//go:build !libretro

package style

import (
	"fmt"
	goimage "image"
	"image/draw"
	"time"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	xdraw "golang.org/x/image/draw"
)

// ScaleImage scales an image to fit within maxWidth x maxHeight while preserving aspect ratio.
// Returns an ebiten.Image suitable for display.
// Scaling is done on CPU to avoid creating large temporary GPU textures.
func ScaleImage(src goimage.Image, maxWidth, maxHeight int) *ebiten.Image {
	bounds := src.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()

	// Calculate scale to fit within max dimensions
	scaleX := float64(maxWidth) / float64(srcWidth)
	scaleY := float64(maxHeight) / float64(srcHeight)
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	// Calculate new dimensions
	newWidth := int(float64(srcWidth) * scale)
	newHeight := int(float64(srcHeight) * scale)

	if newWidth < 1 {
		newWidth = 1
	}
	if newHeight < 1 {
		newHeight = 1
	}

	// Scale on CPU using approximate bilinear interpolation (fast with good quality)
	dstRect := goimage.Rect(0, 0, newWidth, newHeight)
	scaled := goimage.NewRGBA(dstRect)
	xdraw.ApproxBiLinear.Scale(scaled, dstRect, src, bounds, draw.Over, nil)

	// Create Ebiten image from the small scaled image only
	return ebiten.NewImageFromImage(scaled)
}

// TruncateStart truncates a string from the start, keeping the end portion.
// Returns the truncated string and whether truncation occurred.
// Useful for file paths where the end (filename) is most relevant.
func TruncateStart(s string, maxLen int) (string, bool) {
	if len(s) <= maxLen {
		return s, false
	}
	if maxLen <= 3 {
		return s[len(s)-maxLen:], true
	}
	return "..." + s[len(s)-maxLen+3:], true
}

// TruncateEnd truncates a string from the end, keeping the start portion.
// Returns the truncated string and whether truncation occurred.
// Useful for titles where the beginning is most relevant.
func TruncateEnd(s string, maxLen int) (string, bool) {
	if len(s) <= maxLen {
		return s, false
	}
	if maxLen <= 3 {
		return s[:maxLen], true
	}
	return s[:maxLen-3] + "...", true
}

// MeasureWidth returns the pixel width of s rendered at the current font size.
func MeasureWidth(s string) float64 {
	w, _ := text.Measure(s, *FontFace(), 0)
	return w
}

// TruncateToWidth truncates a string to fit within a given pixel width using actual font measurement.
// Returns the truncated string (with "..." suffix if truncated) and whether truncation occurred.
// Uses binary search on rune boundaries for efficiency with proportional fonts.
func TruncateToWidth(s string, face text.Face, maxWidth float64) (string, bool) {
	if s == "" {
		return s, false
	}
	w, _ := text.Measure(s, face, 0)
	if w <= maxWidth {
		return s, false
	}

	ellipsis := "..."
	ellipsisW, _ := text.Measure(ellipsis, face, 0)
	if ellipsisW > maxWidth {
		return ellipsis, true
	}

	// Count runes for binary search bounds
	runeCount := utf8.RuneCountInString(s)

	// Binary search for the largest rune prefix that fits with ellipsis
	lo, hi := 0, runeCount
	best := 0

	for lo <= hi {
		mid := (lo + hi) / 2
		// Extract first mid runes
		prefix := truncateRunes(s, mid)
		candidate := prefix + ellipsis
		cw, _ := text.Measure(candidate, face, 0)
		if cw <= maxWidth {
			best = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	if best == 0 {
		return ellipsis, true
	}
	return truncateRunes(s, best) + ellipsis, true
}

// truncateRunes returns the first n runes of s as a string.
func truncateRunes(s string, n int) string {
	i := 0
	for j := 0; j < n; j++ {
		_, size := utf8.DecodeRuneInString(s[i:])
		if size == 0 {
			break
		}
		i += size
	}
	return s[:i]
}

// FormatPlayTime formats a duration in seconds into a human-readable string.
// Returns "—" for 0 seconds, "< 1m" for under a minute,
// or a formatted string like "2h 30m" or "45m".
func FormatPlayTime(seconds int64) string {
	if seconds == 0 {
		return "-"
	}
	if seconds < 60 {
		return "< 1m"
	}

	hours := seconds / 3600
	minutes := (seconds % 3600) / 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// FormatLastPlayed formats a Unix timestamp into a relative or absolute date string.
// Returns "Never" for 0, "Today"/"Yesterday" for recent dates,
// "Jan 2" for this year, or "Jan 2, 2006" for previous years.
func FormatLastPlayed(timestamp int64) string {
	if timestamp == 0 {
		return "Never"
	}

	t := time.Unix(timestamp, 0)
	now := time.Now()

	// Check if same day
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return "Today"
	}

	// Check if yesterday
	yesterday := now.AddDate(0, 0, -1)
	if t.Year() == yesterday.Year() && t.YearDay() == yesterday.YearDay() {
		return "Yesterday"
	}

	// This year - show month and day
	if t.Year() == now.Year() {
		return t.Format("Jan 2")
	}

	// Previous years - show full date
	return t.Format("Jan 2, 2006")
}

// FormatDate formats a Unix timestamp as a date string.
// Returns "Unknown" for 0, otherwise "Jan 2, 2006".
func FormatDate(timestamp int64) string {
	if timestamp == 0 {
		return "Unknown"
	}
	return time.Unix(timestamp, 0).Format("Jan 2, 2006")
}

// ApplyGrayscale converts an ebiten image to grayscale.
// Returns a new image with grayscale applied.
func ApplyGrayscale(src *ebiten.Image) *ebiten.Image {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Read pixels from source
	pixels := make([]byte, w*h*4)
	src.ReadPixels(pixels)

	// Convert to grayscale using luminance weights (ITU-R BT.601)
	for i := 0; i < len(pixels); i += 4 {
		r := float64(pixels[i])
		g := float64(pixels[i+1])
		b := float64(pixels[i+2])
		gray := uint8(0.299*r + 0.587*g + 0.114*b)
		pixels[i] = gray
		pixels[i+1] = gray
		pixels[i+2] = gray
		// Alpha (pixels[i+3]) stays unchanged
	}

	// Create new image with grayscale pixels
	dst := ebiten.NewImage(w, h)
	dst.WritePixels(pixels)
	return dst
}
