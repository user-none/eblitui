// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	goimage "image"

	"github.com/ebitenui/ebitenui/widget"
)

// CaptionedImagesLayout places images with a caption directly above each,
// as large as the container allows. The images share a scale: side by side
// they share a height, stacked they share a width, so their edges line up
// while each keeps its own aspect ratio. It picks whichever of the two
// arrangements shows the images larger, and centers the group in the
// container.
//
// The container's children are taken in pairs: a caption, then its image.
// Captions are placed at their preferred size; images are placed at their
// computed size, so an image widget should draw to fill its rect.
type CaptionedImagesLayout struct {
	aspects    []float64 // width / height of each image
	spacing    int       // between one captioned image and the next
	captionGap int       // between a caption and its image
}

// NewCaptionedImagesLayout creates a layout for images with the given
// aspect ratios (width / height), spacing pixels apart, each captionGap
// pixels under its caption.
func NewCaptionedImagesLayout(aspects []float64, spacing, captionGap int) *CaptionedImagesLayout {
	return &CaptionedImagesLayout{aspects: aspects, spacing: spacing, captionGap: captionGap}
}

// PreferredSize reports no size of its own; the container is sized by its
// parent and the images fill it.
func (l *CaptionedImagesLayout) PreferredSize(widgets []widget.PreferredSizeLocateableWidget) (int, int) {
	return 0, 0
}

// Layout places the captions and images in rect.
func (l *CaptionedImagesLayout) Layout(widgets []widget.PreferredSizeLocateableWidget, rect goimage.Rectangle) {
	n := min(len(l.aspects), len(widgets)/2)
	if n == 0 {
		return
	}
	captionH := 0
	for i := range n {
		_, h := widgets[2*i].PreferredSize()
		captionH = max(captionH, h)
	}
	head := captionH + l.captionGap
	gaps := (n - 1) * l.spacing

	var sumAspect, sumInverse float64
	for _, a := range l.aspects[:n] {
		sumAspect += a
		sumInverse += 1 / a
	}

	// Side by side: one shared height, limited by the height under the
	// captions and by the width the images and spacing take
	rowH := min(float64(rect.Dy()-head), float64(rect.Dx()-gaps)/sumAspect)
	// Stacked: one shared width, limited by the container width and by the
	// height the captions, images, and spacing take
	colW := min(float64(rect.Dx()), float64(rect.Dy()-n*head-gaps)/sumInverse)
	rowH, colW = max(rowH, 0), max(colW, 0)

	if rowH*rowH*sumAspect >= colW*colW*sumInverse {
		l.layoutRow(widgets[:2*n], rect, int(rowH), head, gaps)
	} else {
		l.layoutColumn(widgets[:2*n], rect, int(colW), head, gaps)
	}
}

// layoutRow places the images side by side at height h, centered in rect
func (l *CaptionedImagesLayout) layoutRow(widgets []widget.PreferredSizeLocateableWidget, rect goimage.Rectangle, h, head, gaps int) {
	widths := make([]int, len(l.aspects))
	total := gaps
	for i := range len(widgets) / 2 {
		widths[i] = int(l.aspects[i] * float64(h))
		total += widths[i]
	}
	x := rect.Min.X + max(0, rect.Dx()-total)/2
	y := rect.Min.Y + max(0, rect.Dy()-head-h)/2
	for i := range len(widgets) / 2 {
		placeCaption(widgets[2*i], x, y)
		widgets[2*i+1].SetLocation(goimage.Rect(x, y+head, x+widths[i], y+head+h))
		x += widths[i] + l.spacing
	}
}

// layoutColumn places the images stacked at width w, centered in rect
func (l *CaptionedImagesLayout) layoutColumn(widgets []widget.PreferredSizeLocateableWidget, rect goimage.Rectangle, w, head, gaps int) {
	heights := make([]int, len(l.aspects))
	total := gaps
	for i := range len(widgets) / 2 {
		heights[i] = int(float64(w) / l.aspects[i])
		total += head + heights[i]
	}
	x := rect.Min.X + max(0, rect.Dx()-w)/2
	y := rect.Min.Y + max(0, rect.Dy()-total)/2
	for i := range len(widgets) / 2 {
		placeCaption(widgets[2*i], x, y)
		widgets[2*i+1].SetLocation(goimage.Rect(x, y+head, x+w, y+head+heights[i]))
		y += head + heights[i] + l.spacing
	}
}

// placeCaption places a caption at its preferred size with its top left at
// x, y
func placeCaption(c widget.PreferredSizeLocateableWidget, x, y int) {
	w, h := c.PreferredSize()
	c.SetLocation(goimage.Rect(x, y, x+w, y+h))
}
