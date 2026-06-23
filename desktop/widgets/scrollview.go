package widgets

import (
	img "image"
	"image/color"
	"math"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// ScrollView is a vertical scroll container that clips its content with an
// ebiten SubImage instead of an allocated mask buffer. ebitenui's
// ScrollContainer renders its content through a screen-sized
// MaskedRenderBuffer; ebiten promotes those buffers onto large texture-atlas
// pages that stay mostly empty and accumulate across UI rebuilds. A SubImage is
// only a bounded view of the destination, so clipping allocates nothing.
//
// Only vertical scrolling is supported; the screens that scroll never scroll
// horizontally. ScrollTop, ViewRect and ContentRect mirror the subset of the
// ebitenui ScrollContainer API the screens rely on.
type ScrollView struct {
	// ScrollTop is the vertical scroll position as a fraction in [0, 1].
	ScrollTop float64

	widget              *widget.Widget
	content             widget.PreferredSizeLocateableWidget
	bg                  *image.NineSlice
	stretchContentWidth bool

	// Optional paired slider, kept in sync when ScrollTop is changed
	// programmatically (e.g. by the ScrollRect* helpers).
	slider *widget.Slider
}

// NewScrollView creates a ScrollView wrapping content. bg fills the viewport
// behind the content. When stretchContentWidth is true, content narrower than
// the viewport is widened to fill it.
func NewScrollView(content widget.PreferredSizeLocateableWidget, bg color.Color, stretchContentWidth bool, opts ...widget.WidgetOpt) *ScrollView {
	s := &ScrollView{
		content:             content,
		bg:                  image.NewNineSliceColor(bg),
		stretchContentWidth: stretchContentWidth,
		widget:              widget.NewWidget(opts...),
	}

	// Forward the content's bubbling events up to this widget, the way
	// Container.addChildInit does for a normal child. The content is held
	// directly rather than via AddChild, so without this the events never
	// reach the root: focus changes inside the content would not update the
	// UI's focused widget, breaking keyboard and gamepad navigation.
	cw := content.GetWidget()
	cw.FocusEvent.AddHandler(func(args interface{}) {
		if a, ok := args.(*widget.WidgetFocusEventArgs); ok {
			s.widget.FireFocusEvent(a.Widget, a.Focused, a.Location)
		}
	})
	cw.ContextMenuEvent.AddHandler(func(args interface{}) {
		if a, ok := args.(*widget.WidgetContextMenuEventArgs); ok {
			s.widget.FireContextMenuEvent(a.Widget, a.Location)
		}
	})
	cw.ToolTipEvent.AddHandler(func(args interface{}) {
		if a, ok := args.(*widget.WidgetToolTipEventArgs); ok {
			s.widget.FireToolTipEvent(a.Window, a.Show)
		}
	})
	cw.DragAndDropEvent.AddHandler(func(args interface{}) {
		if a, ok := args.(*widget.WidgetDragAndDropEventArgs); ok {
			s.widget.FireDragAndDropEvent(a.Window, a.Show, a.DnD)
		}
	})

	return s
}

// GetWidget returns the underlying widget.
func (s *ScrollView) GetWidget() *widget.Widget {
	return s.widget
}

// SetLocation positions the viewport.
func (s *ScrollView) SetLocation(rect img.Rectangle) {
	s.widget.Rect = rect
}

// PreferredSize reports the content's preferred size.
func (s *ScrollView) PreferredSize() (int, int) {
	if p, ok := s.content.(widget.PreferredSizer); ok {
		return p.PreferredSize()
	}
	return 50, 50
}

// Validate validates the content subtree.
func (s *ScrollView) Validate() {
	s.content.Validate()
}

// ViewRect returns the visible viewport in screen coordinates.
func (s *ScrollView) ViewRect() img.Rectangle {
	return s.widget.Rect
}

// ContentRect returns the content's full rect at its current scrolled position.
func (s *ScrollView) ContentRect() img.Rectangle {
	return s.content.GetWidget().Rect
}

func (s *ScrollView) clampScroll() {
	if s.ScrollTop < 0 {
		s.ScrollTop = 0
	} else if s.ScrollTop > 1 {
		s.ScrollTop = 1
	}
}

// SetSlider pairs a slider with the view so programmatic scroll changes keep it
// in sync.
func (s *ScrollView) SetSlider(slider *widget.Slider) {
	s.slider = slider
}

// SetScrollTop clamps the fraction, applies it, and syncs the paired slider.
func (s *ScrollView) SetScrollTop(frac float64) {
	if frac < 0 {
		frac = 0
	} else if frac > 1 {
		frac = 1
	}
	s.ScrollTop = frac
	if s.slider != nil {
		s.slider.Current = int(frac * 1000)
	}
}

// scrollRange returns the scrollable pixel range, or 0 when the content fits.
func (s *ScrollView) scrollRange() int {
	r := s.ContentRect().Dy() - s.ViewRect().Dy()
	if r < 0 {
		return 0
	}
	return r
}

// ScrollRectIntoView scrolls the minimum amount so rect (in screen coordinates)
// is visible: aligning its top to the viewport top when above, or its bottom to
// the viewport bottom when below. Does nothing when the content fits.
func (s *ScrollView) ScrollRectIntoView(rect img.Rectangle) {
	maxScroll := s.scrollRange()
	if maxScroll == 0 {
		return
	}
	viewRect := s.ViewRect()
	offset := int(s.ScrollTop * float64(maxScroll))
	topInView := rect.Min.Y - viewRect.Min.Y
	bottomInView := rect.Max.Y - viewRect.Min.Y

	if topInView < 0 {
		s.SetScrollTop(float64(offset+topInView) / float64(maxScroll))
	} else if bottomInView > viewRect.Dy() {
		s.SetScrollTop(float64(offset+(bottomInView-viewRect.Dy())) / float64(maxScroll))
	}
}

// ScrollRectToTop scrolls so the top of rect (in screen coordinates) aligns with
// the top of the viewport, clamped to the scroll range. Does nothing when the
// content fits.
func (s *ScrollView) ScrollRectToTop(rect img.Rectangle) {
	maxScroll := s.scrollRange()
	if maxScroll == 0 {
		return
	}
	offset := int(s.ScrollTop * float64(maxScroll))
	topInView := rect.Min.Y - s.ViewRect().Min.Y
	s.SetScrollTop(float64(offset+topInView) / float64(maxScroll))
}

// positionContent places the content according to the scroll fraction, so the
// scrolled-away portion sits above the viewport and gets clipped on render.
func (s *ScrollView) positionContent() {
	cw, ch := s.PreferredSize()
	vrect := s.ViewRect()
	if s.stretchContentWidth && cw < vrect.Dx() {
		cw = vrect.Dx()
	}

	rect := img.Rect(0, 0, cw, ch).Add(s.widget.Rect.Min)
	if ch > vrect.Dy() {
		rect = rect.Sub(img.Point{Y: int(math.Round(float64(ch-vrect.Dy()) * s.ScrollTop))})
	}

	if rect != s.content.GetWidget().Rect {
		s.content.SetLocation(rect)
		if r, ok := s.content.(widget.Relayoutable); ok {
			r.RequestRelayout()
		}
	}
}

// Render draws the background and the content, clipping the content to the
// viewport with a SubImage.
func (s *ScrollView) Render(screen *ebiten.Image) {
	s.clampScroll()
	s.content.GetWidget().Disabled = s.widget.Disabled
	s.widget.Render(screen)

	vrect := s.ViewRect()
	if vrect.Empty() {
		return
	}

	if s.bg != nil {
		s.bg.Draw(screen, vrect.Dx(), vrect.Dy(), func(opts *ebiten.DrawImageOptions) {
			opts.GeoM.Translate(float64(vrect.Min.X), float64(vrect.Min.Y))
		})
	}

	s.positionContent()

	// SubImage is a bounded view of screen; drawing the content onto it clips
	// to the viewport without allocating a buffer.
	sub := screen.SubImage(vrect).(*ebiten.Image)
	if r, ok := s.content.(widget.Renderer); ok {
		r.Render(sub)
	}
}

// Update updates the content subtree.
func (s *ScrollView) Update(updObj *widget.UpdateObject) {
	s.widget.Update(updObj)
	if u, ok := s.content.(widget.Updater); ok {
		u.Update(updObj)
	}
}

// SetupInputLayer elevates the content onto an input layer clipped to the
// viewport, so content scrolled out of view does not receive pointer events.
// Wheel events are excluded so they fall through to the ScrollView's own
// widget, which fires ScrolledEvent.
func (s *ScrollView) SetupInputLayer(def input.DeferredSetupInputLayerFunc) {
	if !s.widget.IsVisible() {
		return
	}
	s.content.GetWidget().ElevateToNewInputLayer(&input.Layer{
		DebugLabel: "scroll view content",
		EventTypes: input.LayerEventTypeAll ^ input.LayerEventTypeWheel,
		BlockLower: true,
		FullScreen: false,
		RectFunc:   s.ViewRect,
	})
	if il, ok := s.content.(input.Layerer); ok {
		il.SetupInputLayer(def)
	}
}
