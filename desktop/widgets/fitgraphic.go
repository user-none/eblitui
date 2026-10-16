// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	goimage "image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// FitGraphic draws one texture scaled to fit its rect, centered, keeping
// the aspect ratio. It reports no preferred size, so the layout cell it is
// stretched into decides how large the image is drawn. This lets a large
// texture be shown at whatever size the window allows without rescaling it
// on the CPU.
//
// Image is set directly by the owner, matching the public-field style of
// ebitenui's own Graphic.
type FitGraphic struct {
	Image *ebiten.Image

	widget *widget.Widget
}

// NewFitGraphic creates a FitGraphic for img. opts are applied to the
// underlying widget.
func NewFitGraphic(img *ebiten.Image, opts ...widget.WidgetOpt) *FitGraphic {
	return &FitGraphic{
		Image:  img,
		widget: widget.NewWidget(opts...),
	}
}

// GetWidget returns the underlying widget.
func (g *FitGraphic) GetWidget() *widget.Widget {
	return g.widget
}

// PreferredSize reports no size of its own; the layout decides.
func (g *FitGraphic) PreferredSize() (int, int) {
	return 0, 0
}

// SetLocation positions the widget.
func (g *FitGraphic) SetLocation(rect goimage.Rectangle) {
	g.widget.Rect = rect
}

// Validate satisfies the widget interface; nothing to validate.
func (g *FitGraphic) Validate() {}

// Render draws the texture scaled to fit the widget rect and centered in
// it.
func (g *FitGraphic) Render(screen *ebiten.Image) {
	g.widget.Render(screen)

	if g.Image == nil {
		return
	}
	rectW := float64(g.widget.Rect.Dx())
	rectH := float64(g.widget.Rect.Dy())
	if rectW <= 0 || rectH <= 0 {
		return
	}

	ib := g.Image.Bounds()
	w := float64(ib.Dx())
	h := float64(ib.Dy())
	scale := min(rectW/w, rectH/h)

	opts := &ebiten.DrawImageOptions{}
	opts.Filter = ebiten.FilterLinear
	opts.GeoM.Scale(scale, scale)
	opts.GeoM.Translate((rectW-w*scale)/2, (rectH-h*scale)/2)
	opts.GeoM.Translate(float64(g.widget.Rect.Min.X), float64(g.widget.Rect.Min.Y))
	screen.DrawImage(g.Image, opts)
}
