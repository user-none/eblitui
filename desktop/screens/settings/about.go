package settings

import (
	"bytes"
	goimage "image"
	_ "image/png"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
)

// aboutIconSize is the logical width/height the app icon is scaled to fit.
const aboutIconSize = 128

// AboutSection displays the application icon, name, and version.
type AboutSection struct {
	name    string
	version string
	iconPNG []byte // PNG-encoded icon (may be nil)

	iconImg *ebiten.Image // decoded/scaled icon, cached after first Build
}

// NewAboutSection creates the About section. iconPNG may be nil, in which case
// only the name and version are shown.
func NewAboutSection(name, version string, iconPNG []byte) *AboutSection {
	return &AboutSection{
		name:    name,
		version: version,
		iconPNG: iconPNG,
	}
}

// Build creates the About section UI. The focus manager is unused because the
// section has no interactive widgets.
func (a *AboutSection) Build(focus types.FocusManager) *widget.Container {
	// Outer container fills the settings content-area grid cell; the centered
	// content is anchored to the middle.
	outer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)

	center := style.CenteredContainer(style.DefaultSpacing)

	if img := a.icon(); img != nil {
		center.AddChild(widget.NewGraphic(
			widget.GraphicOpts.Image(img),
			widget.GraphicOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{
					Position: widget.RowLayoutPositionCenter,
				}),
			),
		))
	}

	center.AddChild(widget.NewText(
		widget.TextOpts.Text(a.name, style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
		),
	))

	center.AddChild(widget.NewText(
		widget.TextOpts.Text("Version "+a.version, style.FontFace(), style.TextSecondary),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
		),
	))

	outer.AddChild(center)
	return outer
}

// icon decodes and scales the icon once, caching the result. Returns nil when
// no icon is set or the bytes cannot be decoded.
func (a *AboutSection) icon() *ebiten.Image {
	if a.iconImg != nil {
		return a.iconImg
	}
	if len(a.iconPNG) == 0 {
		return nil
	}
	img, _, err := goimage.Decode(bytes.NewReader(a.iconPNG))
	if err != nil {
		return nil
	}
	a.iconImg = style.ScaleImage(img, style.Px(aboutIconSize), style.Px(aboutIconSize))
	return a.iconImg
}
