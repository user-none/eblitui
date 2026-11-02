// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	goimage "image"
	"image/color"
	"time"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/style"
)

// Carousel timing and look
const (
	carouselSlide = 200 * time.Millisecond // length of a slide
	carouselDim   = 0.35                   // brightness of the images beside the current one
	carouselFocus = 3                      // logical width of the focus border on a peek
)

// Carousel shows one image of a row at a time, captioned and as large as
// its rect allows, with a slice of the images beside it at the same height,
// dimmed, at the edges of its rect. Each visible slice is a button that moves the
// row one image its way. The row does not wrap.
//
// The carousel holds no position of its own. The owner keeps the index,
// creates a new Carousel after each move, and passes the index moved from
// and when, so a slide in progress continues across rebuilds.
type Carousel struct {
	container *widget.Container
	caption   *widget.Text
	prev      *widget.Button // nil on the first image
	next      *widget.Button // nil on the last image

	aspects []float64 // width / height of each image
	images  []*ebiten.Image
	index   int       // image shown
	from    int       // image the slide started at, index when none
	start   time.Time // when the slide started
	now     func() time.Time
	gap     int // between images
	peek    int // least width of a visible slice

	rect     goimage.Rectangle // from the last layout
	captionH int               // caption height and the gap under it
}

// CarouselOptions configures a Carousel
type CarouselOptions struct {
	Images []*ebiten.Image // the row, all non-nil
	Labels []string        // a caption per image
	Index  int             // image shown
	From   int             // image the last move started at; Index when none
	Start  time.Time       // when the last move started
	Now    func() time.Time
	Gap    int // pixels between images
	Peek   int // least pixel width of a visible slice beside the image shown

	// Move is called with -1 or 1 when the slice on that side is
	// pressed
	Move func(dir int)
}

// NewCarousel creates a Carousel. The index is clamped to the row.
func NewCarousel(o CarouselOptions) *Carousel {
	n := len(o.Images)
	c := &Carousel{
		images:  o.Images,
		aspects: make([]float64, n),
		index:   clampIndex(o.Index, n),
		from:    clampIndex(o.From, n),
		start:   o.Start,
		now:     o.Now,
		gap:     o.Gap,
		peek:    o.Peek,
	}
	for i, img := range o.Images {
		b := img.Bounds()
		c.aspects[i] = float64(b.Dx()) / float64(b.Dy())
	}

	c.container = widget.NewContainer(widget.ContainerOpts.Layout(carouselLayout{c}))
	c.container.AddChild(&carouselCanvas{c: c, widget: widget.NewWidget()})
	label := ""
	if c.index < len(o.Labels) {
		label = o.Labels[c.index]
	}
	c.caption = widget.NewText(widget.TextOpts.Text(label, style.FontFace(), style.TextSecondary))
	c.container.AddChild(c.caption)
	if c.index > 0 {
		c.prev = peekButton(func() { o.Move(-1) })
		c.container.AddChild(c.prev)
	}
	if c.index+1 < n {
		c.next = peekButton(func() { o.Move(1) })
		c.container.AddChild(c.next)
	}
	return c
}

// clampIndex limits i to a row of n images
func clampIndex(i, n int) int {
	return max(0, min(i, n-1))
}

// peekButton creates the button over a slice: clear, with a border when
// focused or hovered
func peekButton(move func()) *widget.Button {
	border := image.NewBorderedNineSliceColor(color.Transparent, style.PrimaryHover, style.Px(carouselFocus))
	return widget.NewButton(
		widget.ButtonOpts.Image(&widget.ButtonImage{
			Idle:    image.NewNineSliceColor(color.Transparent),
			Hover:   border,
			Pressed: border,
		}),
		widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { move() }),
	)
}

// Container returns the widget to add to a page. It fills the cell it is
// placed in.
func (c *Carousel) Container() *widget.Container {
	return c.container
}

// Prev returns the slice button of the image before the current one, nil
// on the first image
func (c *Carousel) Prev() *widget.Button {
	return c.prev
}

// Next returns the slice button of the image after the current one, nil on
// the last image
func (c *Carousel) Next() *widget.Button {
	return c.next
}

// frect is a rectangle in fractional pixels
type frect struct {
	x0, y0, x1, y1 float64
}

// lerp returns the rectangle t of the way from a to b
func (a frect) lerp(b frect, t float64) frect {
	return frect{
		a.x0 + (b.x0-a.x0)*t,
		a.y0 + (b.y0-a.y0)*t,
		a.x1 + (b.x1-a.x1)*t,
		a.y1 + (b.y1-a.y1)*t,
	}
}

// pixels returns the rectangle rounded in to whole pixels
func (a frect) pixels() goimage.Rectangle {
	return goimage.Rect(int(a.x0+0.5), int(a.y0+0.5), int(a.x1+0.5), int(a.y1+0.5))
}

// carouselRects places the images of a row with index shown, in rect under
// a caption captionH high. The image shown is as large as fits, centered,
// with room for a slice peek wide and a gap at each side when there are
// others. The images beside it, at its height, show peek wide at the edges
// of rect, and the rest continue the row outward gap apart, outside rect.
func carouselRects(rect goimage.Rectangle, aspects []float64, index, captionH, gap, peek int) []frect {
	n := len(aspects)
	rects := make([]frect, n)
	if n == 0 {
		return rects
	}
	reserve := 0
	if n > 1 {
		reserve = peek + gap
	}
	availW := float64(rect.Dx() - 2*reserve)
	availH := float64(rect.Dy() - captionH)
	h := max(0, min(availH, availW/aspects[index]))
	w := h * aspects[index]

	x0 := float64(rect.Min.X) + (float64(rect.Dx())-w)/2
	y0 := float64(rect.Min.Y+captionH) + (availH-h)/2
	rects[index] = frect{x0, y0, x0 + w, y0 + h}

	x := max(float64(rect.Max.X-peek), x0+w+float64(gap))
	for i := index + 1; i < n; i++ {
		wi := h * aspects[i]
		rects[i] = frect{x, y0, x + wi, y0 + h}
		x += wi + float64(gap)
	}
	x = min(float64(rect.Min.X+peek), x0-float64(gap))
	for i := index - 1; i >= 0; i-- {
		wi := h * aspects[i]
		rects[i] = frect{x - wi, y0, x, y0 + h}
		x -= wi + float64(gap)
	}
	return rects
}

// slideProgress returns how far a slide elapsed into it has come, eased
// out, from 0 to 1
func slideProgress(elapsed time.Duration) float64 {
	if elapsed >= carouselSlide {
		return 1
	}
	t := max(0, float64(elapsed)/float64(carouselSlide))
	u := 1 - t
	return 1 - u*u*u
}

// carouselLayout places the canvas over the whole rect, the caption above
// the image shown, and each slice button over its visible slice
type carouselLayout struct {
	c *Carousel
}

// PreferredSize reports no size of its own; the carousel fills its cell
func (l carouselLayout) PreferredSize([]widget.PreferredSizeLocateableWidget) (int, int) {
	return 0, 0
}

// Layout places the carousel's widgets in rect
func (l carouselLayout) Layout(children []widget.PreferredSizeLocateableWidget, rect goimage.Rectangle) {
	c := l.c
	_, textH := c.caption.PreferredSize()
	c.rect = rect
	c.captionH = textH + style.SmallSpacing
	for _, ch := range children {
		if canvas, ok := ch.(*carouselCanvas); ok {
			canvas.SetLocation(rect)
		}
	}
	if len(c.images) == 0 {
		return
	}

	rects := carouselRects(rect, c.aspects, c.index, c.captionH, c.gap, c.peek)
	cur := rects[c.index].pixels()
	textW, _ := c.caption.PreferredSize()
	c.caption.SetLocation(goimage.Rect(cur.Min.X, cur.Min.Y-c.captionH, cur.Min.X+textW, cur.Min.Y-style.SmallSpacing))
	if c.prev != nil {
		c.prev.SetLocation(rects[c.index-1].pixels().Intersect(rect))
	}
	if c.next != nil {
		c.next.SetLocation(rects[c.index+1].pixels().Intersect(rect))
	}
}

// carouselCanvas draws the carousel's images, sliding from the last
// position to the current one
type carouselCanvas struct {
	c      *Carousel
	widget *widget.Widget
}

// GetWidget returns the underlying widget
func (v *carouselCanvas) GetWidget() *widget.Widget {
	return v.widget
}

// PreferredSize reports no size of its own; the layout decides
func (v *carouselCanvas) PreferredSize() (int, int) {
	return 0, 0
}

// SetLocation positions the canvas
func (v *carouselCanvas) SetLocation(rect goimage.Rectangle) {
	v.widget.Rect = rect
}

// Validate satisfies the widget interface; nothing to validate
func (v *carouselCanvas) Validate() {}

// Render draws each image that is in the carousel's rect, clipped to it,
// the image shown at full brightness and the others dimmed
func (v *carouselCanvas) Render(screen *ebiten.Image) {
	v.widget.Render(screen)
	c := v.c
	if len(c.images) == 0 || c.rect.Empty() {
		return
	}

	to := carouselRects(c.rect, c.aspects, c.index, c.captionH, c.gap, c.peek)
	from := to
	t := 1.0
	if c.from != c.index {
		t = slideProgress(c.now().Sub(c.start))
		if t < 1 {
			from = carouselRects(c.rect, c.aspects, c.from, c.captionH, c.gap, c.peek)
		}
	}

	clip := screen.SubImage(c.rect).(*ebiten.Image)
	for i, img := range c.images {
		r := from[i].lerp(to[i], t)
		if !r.pixels().Overlaps(c.rect) {
			continue
		}
		bright := carouselDim + (1-carouselDim)*lerpWeight(i == c.from, i == c.index, t)
		ib := img.Bounds()
		opts := &ebiten.DrawImageOptions{}
		opts.Filter = ebiten.FilterLinear
		opts.GeoM.Scale((r.x1-r.x0)/float64(ib.Dx()), (r.y1-r.y0)/float64(ib.Dy()))
		opts.GeoM.Translate(r.x0, r.y0)
		opts.ColorScale.Scale(float32(bright), float32(bright), float32(bright), 1)
		clip.DrawImage(img, opts)
	}
}

// lerpWeight returns a value t of the way from 1 or 0, whether a holds, to
// 1 or 0, whether b holds
func lerpWeight(a, b bool, t float64) float64 {
	var wa, wb float64
	if a {
		wa = 1
	}
	if b {
		wb = 1
	}
	return wa + (wb-wa)*t
}
