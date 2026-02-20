//go:build !libretro

package settings

import (
	"fmt"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
	"github.com/user-none/eblitui/standalone/types"
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
	// Use GridLayout so the scrollable list can stretch
	section := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			// Row stretch: font row=no, theme label=no, theme list=YES
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, false, true}),
			widget.GridLayoutOpts.Spacing(0, style.DefaultSpacing),
		)),
	)

	// Font size row: label left, stepper right
	section.AddChild(a.buildFontSizeRow(focus))

	// Theme label
	themeLabel := widget.NewText(
		widget.TextOpts.Text("Theme", style.FontFace(), style.Accent),
	)
	section.AddChild(themeLabel)

	// Theme cards in scrollable list
	themeListContent := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.DefaultSpacing),
		)),
	)

	for _, theme := range style.AvailableThemes {
		themeListContent.AddChild(a.buildThemeCard(theme, focus))
	}

	// Wrap in scrollable container using existing style helper
	scrollContainer, vSlider, scrollWrapper := style.ScrollableContainer(style.ScrollableOpts{
		Content:     themeListContent,
		BgColor:     style.Background,
		BorderColor: style.Border,
		Spacing:     0,
		Padding:     style.SmallSpacing,
	})
	focus.SetScrollWidgets(scrollContainer, vSlider)
	// Restore scroll position after rebuild
	focus.RestoreScrollPosition()
	section.AddChild(scrollWrapper)

	// Set up navigation zones
	a.setupNavigation(focus)

	return section
}

// setupNavigation registers navigation zones for the appearance section
func (a *AppearanceSection) setupNavigation(focus types.FocusManager) {
	// Font size zone (horizontal)
	focus.RegisterNavZone("font-size", types.NavZoneHorizontal, []string{"font-decrease", "font-increase"}, 0)

	// Theme list zone (vertical)
	themeKeys := make([]string, len(style.AvailableThemes))
	for i, theme := range style.AvailableThemes {
		themeKeys[i] = fmt.Sprintf("theme-%s", theme.Name)
	}
	focus.RegisterNavZone("theme-list", types.NavZoneVertical, themeKeys, 0)

	// Vertical transitions between zones
	focus.SetNavTransition("font-size", types.DirDown, "theme-list", 0)
	focus.SetNavTransition("theme-list", types.DirUp, "font-size", 0)
}

// buildFontSizeRow creates the font size row with label left and +/- stepper right
func (a *AppearanceSection) buildFontSizeRow(focus types.FocusManager) *widget.Container {
	presets := storage.FontSizePresets
	currentSize := storage.ValidFontSize(a.config.FontSize)

	// Find current index in presets
	currentIdx := 0
	for i, p := range presets {
		if p == currentSize {
			currentIdx = i
			break
		}
	}

	// Outer container with background color
	row := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Surface)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{true, false}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(style.SmallSpacing)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	// Label on left
	labelText := widget.NewText(
		widget.TextOpts.Text("Font Size", style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		),
	)
	row.AddChild(labelText)

	// Controls group: [-] value [+]
	controls := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
	)

	// Decrease button
	decImage := style.ButtonImage()
	if currentIdx <= 0 {
		decImage = style.DisabledButtonImage()
	}
	decBtn := widget.NewButton(
		widget.ButtonOpts.Image(decImage),
		widget.ButtonOpts.Text("-", style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			idx := 0
			for i, p := range presets {
				if p == storage.ValidFontSize(a.config.FontSize) {
					idx = i
					break
				}
			}
			if idx > 0 {
				a.config.FontSize = presets[idx-1]
				style.ApplyFontSize(a.config.FontSize)
				storage.SaveConfig(a.config)
				focus.SetPendingFocus("font-decrease")
				a.callback.RequestRebuild()
			}
		}),
	)
	focus.RegisterFocusButton("font-decrease", decBtn)
	controls.AddChild(decBtn)

	// Size value display
	sizeLabel := widget.NewText(
		widget.TextOpts.Text(fmt.Sprintf("%dpt", currentSize), style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(50), 0),
		),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	controls.AddChild(sizeLabel)

	// Increase button
	incImage := style.ButtonImage()
	if currentIdx >= len(presets)-1 {
		incImage = style.DisabledButtonImage()
	}
	incBtn := widget.NewButton(
		widget.ButtonOpts.Image(incImage),
		widget.ButtonOpts.Text("+", style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			idx := 0
			for i, p := range presets {
				if p == storage.ValidFontSize(a.config.FontSize) {
					idx = i
					break
				}
			}
			if idx < len(presets)-1 {
				a.config.FontSize = presets[idx+1]
				style.ApplyFontSize(a.config.FontSize)
				storage.SaveConfig(a.config)
				focus.SetPendingFocus("font-increase")
				a.callback.RequestRebuild()
			}
		}),
	)
	focus.RegisterFocusButton("font-increase", incBtn)
	controls.AddChild(incBtn)

	row.AddChild(controls)

	return row
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
