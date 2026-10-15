// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	"math"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/style"
)

// Sidebar is a vertical list of navigation items on a surface panel.
// Focus registration is left to the owning screen, which knows its keys.
type Sidebar struct {
	Container *widget.Container
}

// NewSidebar creates an empty sidebar. minWidth is the minimum width; the
// sidebar still grows when a label needs more room.
func NewSidebar(minWidth int) *Sidebar {
	return &Sidebar{
		Container: widget.NewContainer(
			widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Surface)),
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionVertical),
				widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(style.SmallSpacing)),
				widget.RowLayoutOpts.Spacing(style.TinySpacing),
			)),
			widget.ContainerOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(minWidth, 0),
			),
		),
	}
}

// SidebarFitWidth returns the width a sidebar needs to fit the longest of
// labels, including button and sidebar padding, at the current font size.
func SidebarFitWidth(labels []string) int {
	var labelWidth float64
	for _, label := range labels {
		labelWidth = math.Max(labelWidth, style.MeasureWidth(label))
	}
	return int(math.Ceil(labelWidth)) + style.ButtonPaddingSmall*2 + style.SmallSpacing*2
}

// AddButton appends a button stretched to the sidebar width. active selects
// the highlighted style used for the current selection.
func (s *Sidebar) AddButton(label string, active bool, handler func()) *widget.Button {
	btn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(active)),
		widget.ButtonOpts.Text(label, style.FontFace(), &widget.ButtonTextColor{
			Idle:     style.Text,
			Disabled: style.TextSecondary,
		}),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			if handler != nil {
				handler()
			}
		}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	s.Container.AddChild(btn)
	return btn
}

// AddDisabledItem appends a non-focusable item for future or unavailable
// entries. It uses the button padding so it matches the button height.
func (s *Sidebar) AddDisabledItem(label string) {
	item := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Border)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout(
			widget.AnchorLayoutOpts.Padding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	item.AddChild(widget.NewText(
		widget.TextOpts.Text(label, style.FontFace(), style.TextSecondary),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionCenter,
				VerticalPosition:   widget.AnchorLayoutPositionCenter,
			}),
		),
	))
	s.Container.AddChild(item)
}

// AddDivider appends a 1px separator line between groups of items.
func (s *Sidebar) AddDivider() {
	s.Container.AddChild(widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Border)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			widget.WidgetOpts.MinSize(0, style.Px(1)),
		),
	))
}

// Clear removes all items so the sidebar can be refilled in place.
func (s *Sidebar) Clear() {
	s.Container.RemoveChildren()
}
