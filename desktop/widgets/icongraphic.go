package widgets

import (
	goimage "image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// IconGraphic draws a single artwork texture centered in its rect for the
// library icon view. When unfocused it renders the texture scaled down and
// dimmed; when focused it renders at full size and full brightness. This
// produces the zoom effect from one texture per game instead of caching a
// second pre-scaled, pre-dimmed copy.
//
// Image and Focused are set directly by the owner to swap artwork and toggle
// the zoom state, matching the public-field style of ebitenui's own Graphic.
type IconGraphic struct {
	Image   *ebiten.Image
	Focused bool

	scale float64 // Unfocused scale factor (0..1)
	dim   float32 // Unfocused brightness multiplier (0..1)

	widget *widget.Widget
}

// NewIconGraphic creates an IconGraphic for img. scale and dim control the
// unfocused appearance. opts are applied to the underlying widget.
func NewIconGraphic(img *ebiten.Image, scale float64, dim float32, opts ...widget.WidgetOpt) *IconGraphic {
	return &IconGraphic{
		Image:  img,
		scale:  scale,
		dim:    dim,
		widget: widget.NewWidget(opts...),
	}
}

// GetWidget returns the underlying widget.
func (g *IconGraphic) GetWidget() *widget.Widget {
	return g.widget
}

// PreferredSize reports the texture size, matching the stock graphic widget.
func (g *IconGraphic) PreferredSize() (int, int) {
	if g.Image == nil {
		return 50, 50
	}
	s := g.Image.Bounds().Size()
	return s.X, s.Y
}

// SetLocation positions the widget.
func (g *IconGraphic) SetLocation(rect goimage.Rectangle) {
	g.widget.Rect = rect
}

// Validate satisfies the widget interface; nothing to validate.
func (g *IconGraphic) Validate() {}

// Render draws the texture centered in the widget rect. Unfocused cards are
// scaled down and dimmed; focused cards draw at native size and brightness.
func (g *IconGraphic) Render(screen *ebiten.Image) {
	g.widget.Render(screen)

	if g.Image == nil {
		return
	}

	ib := g.Image.Bounds()
	w := float64(ib.Dx())
	h := float64(ib.Dy())
	rectW := float64(g.widget.Rect.Dx())
	rectH := float64(g.widget.Rect.Dy())

	opts := &ebiten.DrawImageOptions{}
	if g.Focused {
		opts.GeoM.Translate((rectW-w)/2, (rectH-h)/2)
	} else {
		opts.Filter = ebiten.FilterLinear
		opts.GeoM.Scale(g.scale, g.scale)
		opts.GeoM.Translate((rectW-w*g.scale)/2, (rectH-h*g.scale)/2)
		opts.ColorScale.Scale(g.dim, g.dim, g.dim, 1)
	}
	opts.GeoM.Translate(float64(g.widget.Rect.Min.X), float64(g.widget.Rect.Min.Y))
	screen.DrawImage(g.Image, opts)
}
