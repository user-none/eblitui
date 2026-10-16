// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"fmt"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// AppearanceSection manages theme settings
type AppearanceSection struct {
	callback types.ScreenCallback
	config   *storage.Config
}

// NewAppearanceSection creates a new appearance section
func NewAppearanceSection(callback types.ScreenCallback, config *storage.Config) *AppearanceSection {
	return &AppearanceSection{
		callback: callback,
		config:   config,
	}
}

// SetConfig updates the config reference
func (a *AppearanceSection) SetConfig(config *storage.Config) {
	a.config = config
}

// Build creates the appearance section UI
func (a *AppearanceSection) Build(focus types.FocusManager) *widget.Container {
	outer := widgets.FillContainer()

	section := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
	)

	// Theme label
	section.AddChild(widgets.SectionHeader("Theme"))

	// Theme cards
	for _, theme := range style.AvailableThemes {
		section.AddChild(a.buildThemeCard(theme, focus))
	}

	// Set up navigation zones
	a.setupNavigation(focus)

	scrollContainer, _, scrollWrapper := widgets.ScrollableContainer(widgets.PageScrollOpts(section))
	focus.SetScrollContainer(scrollContainer)
	focus.RestoreScrollPosition()
	outer.AddChild(scrollWrapper)

	return outer
}

// setupNavigation registers navigation zones for the appearance section
func (a *AppearanceSection) setupNavigation(focus types.FocusManager) {
	// Theme list zone (vertical)
	themeKeys := make([]string, len(style.AvailableThemes))
	for i, theme := range style.AvailableThemes {
		themeKeys[i] = fmt.Sprintf("theme-%s", theme.Name)
	}
	focus.RegisterNavZone("theme-list", types.NavZoneVertical, themeKeys, 0)
}

// buildThemeCard creates a theme selection card with button and color preview
func (a *AppearanceSection) buildThemeCard(theme style.Theme, focus types.FocusManager) *widget.Container {
	themeName := theme.Name
	isActive := a.config.Theme == themeName
	focusKey := fmt.Sprintf("theme-%s", themeName)

	// Use grid layout so preview can stretch
	card := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	// Theme button
	themeBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(isActive)),
		widget.ButtonOpts.Text(themeName, style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingMedium)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(style.Px(120), 0),
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			a.config.Theme = themeName
			style.ApplyThemeByName(themeName)
			storage.SaveConfig(a.config)
			focus.SetPendingFocus(fmt.Sprintf("theme-%s", themeName))
			a.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton(focusKey, themeBtn)
	card.AddChild(themeBtn)

	// Theme preview mockup
	card.AddChild(a.buildThemePreview(theme))

	return card
}

// buildThemePreview creates a mini UI mockup showing the theme applied
func (a *AppearanceSection) buildThemePreview(theme style.Theme) *widget.Container {
	previewHeight := style.Px(100)
	sidebarWidth := style.Px(70)
	btnPadding := style.Px(4)
	itemHeight := style.Px(22)

	// Outer container with theme's background color
	preview := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(theme.Background)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{true}),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(style.Px(6))),
			widget.GridLayoutOpts.Spacing(style.Px(6), 0),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, previewHeight),
		),
	)

	// Mini sidebar with surface color
	sidebar := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(theme.Surface)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(style.Px(4))),
			widget.RowLayoutOpts.Spacing(style.Px(2)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(sidebarWidth, 0),
		),
	)

	// Selected sidebar item (primary color)
	selectedItem := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(theme.Primary)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout(
			widget.AnchorLayoutOpts.Padding(widget.NewInsetsSimple(style.Px(2))),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, itemHeight),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	selectedItemText := widget.NewText(
		widget.TextOpts.Text("Library", style.FontFace(), theme.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				VerticalPosition: widget.AnchorLayoutPositionCenter,
			}),
		),
	)
	selectedItem.AddChild(selectedItemText)
	sidebar.AddChild(selectedItem)

	// Unselected sidebar items
	for _, label := range []string{"Settings", "Help"} {
		item := widget.NewText(
			widget.TextOpts.Text(label, style.FontFace(), theme.TextSecondary),
			widget.TextOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, itemHeight),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
		)
		sidebar.AddChild(item)
	}

	preview.AddChild(sidebar)

	// Content area - surface panel
	contentPanel := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(theme.Surface)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(style.Px(6))),
			widget.RowLayoutOpts.Spacing(style.Px(6)),
		)),
	)

	// Header row with title
	title := widget.NewText(
		widget.TextOpts.Text("Game Title", style.FontFace(), theme.Text),
	)
	contentPanel.AddChild(title)

	// Info text
	info := widget.NewText(
		widget.TextOpts.Text("Developer: Studio Name", style.FontFace(), theme.TextSecondary),
	)
	contentPanel.AddChild(info)

	// Button row
	buttonRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(style.Px(6)),
		)),
	)

	// Primary button (Play)
	primaryBtn := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(theme.Primary)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout(
			widget.AnchorLayoutOpts.Padding(widget.NewInsetsSimple(btnPadding)),
		)),
	)
	primaryBtnText := widget.NewText(
		widget.TextOpts.Text("Play", style.FontFace(), theme.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionCenter,
				VerticalPosition:   widget.AnchorLayoutPositionCenter,
			}),
		),
	)
	primaryBtn.AddChild(primaryBtnText)
	buttonRow.AddChild(primaryBtn)

	// Secondary button (Options)
	secondaryBtn := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(theme.Background)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout(
			widget.AnchorLayoutOpts.Padding(widget.NewInsetsSimple(btnPadding)),
		)),
	)
	secondaryBtnText := widget.NewText(
		widget.TextOpts.Text("Options", style.FontFace(), theme.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionCenter,
				VerticalPosition:   widget.AnchorLayoutPositionCenter,
			}),
		),
	)
	secondaryBtn.AddChild(secondaryBtnText)
	buttonRow.AddChild(secondaryBtn)

	// Accent indicator (favorite star like in the UI)
	accentText := widget.NewText(
		widget.TextOpts.Text("*", style.FontFace(), theme.Accent),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
		),
	)
	buttonRow.AddChild(accentText)

	contentPanel.AddChild(buttonRow)
	preview.AddChild(contentPanel)

	return preview
}
