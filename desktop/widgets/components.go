// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	"image/color"
	"runtime"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/user-none/eblitui/desktop/style"
	"golang.design/x/clipboard"
)

// SettingsRow creates a standard settings row container with style.Surface background,
// N-column grid layout, and RowLayoutData stretch.
func SettingsRow(columns int) *widget.Container {
	stretch := make([]bool, columns)
	stretch[0] = true
	return widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Surface)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(columns),
			widget.GridLayoutOpts.Stretch(stretch, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(style.SmallSpacing)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
}

// ScrollSlider creates a vertical scroll slider bound to a scroll container.
// The needsScroll function should return true when content exceeds view height.
// Returns the slider widget.
func ScrollSlider(scrollContainer *ScrollView, needsScroll func() bool) *widget.Slider {
	return widget.NewSlider(
		widget.SliderOpts.TabOrder(-1), // Non-focusable for gamepad navigation
		widget.SliderOpts.Direction(widget.DirectionVertical),
		widget.SliderOpts.MinMax(0, 1000),
		widget.SliderOpts.Images(
			&widget.SliderTrackImage{
				Idle:  image.NewNineSliceColor(style.Border),
				Hover: image.NewNineSliceColor(style.Border),
			},
			style.SliderButtonImage(),
		),
		widget.SliderOpts.FixedHandleSize(style.Px(40)),
		widget.SliderOpts.PageSizeFunc(func() int {
			if !needsScroll() {
				return 1000 // Handle fills track - no scrolling needed
			}
			viewHeight := scrollContainer.ViewRect().Dy()
			contentHeight := scrollContainer.ContentRect().Dy()
			return int(float64(viewHeight) / float64(contentHeight) * 1000)
		}),
		widget.SliderOpts.ChangedHandler(func(args *widget.SliderChangedEventArgs) {
			if !needsScroll() {
				scrollContainer.ScrollTop = 0
				return
			}
			scrollContainer.ScrollTop = float64(args.Current) / 1000
		}),
	)
}

// SetupScrollHandler adds mouse wheel scroll support to a scroll container.
// The slider's Current value is kept in sync with scroll position.
func SetupScrollHandler(scrollContainer *ScrollView, vSlider *widget.Slider, needsScroll func() bool) {
	scrollContainer.GetWidget().ScrolledEvent.AddHandler(func(args interface{}) {
		if !needsScroll() {
			scrollContainer.ScrollTop = 0
			return
		}
		a := args.(*widget.WidgetScrolledEventArgs)
		p := scrollContainer.ScrollTop + (a.Y * 0.05)
		if p < 0 {
			p = 0
		}
		if p > 1 {
			p = 1
		}
		scrollContainer.ScrollTop = p
		vSlider.Current = int(p * 1000)
	})
}

// DisabledSidebarItem creates a non-focusable sidebar item with the given label.
// Used for future/coming-soon menu items.
func DisabledSidebarItem(label string) *widget.Container {
	item := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Border)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout(
			widget.AnchorLayoutOpts.Padding(widget.NewInsetsSimple(style.SmallSpacing)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	item.AddChild(widget.NewText(
		widget.TextOpts.Text(label, style.FontFace(), style.TextSecondary),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionStart,
				VerticalPosition:   widget.AnchorLayoutPositionCenter,
			}),
		),
	))
	return item
}

// TextButton creates a standard text button with consistent styling.
// Use for regular actions like "Back", "Cancel", "Settings".
func TextButton(text string, padding int, handler func(*widget.ButtonClickedEventArgs)) *widget.Button {
	return widget.NewButton(
		widget.ButtonOpts.Image(style.ButtonImage()),
		widget.ButtonOpts.Text(text, style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(padding)),
		widget.ButtonOpts.ClickedHandler(handler),
	)
}

// PrimaryTextButton creates a prominent text button with primary styling.
// Use for main actions like "Play", "Save", "Scan Library".
func PrimaryTextButton(text string, padding int, handler func(*widget.ButtonClickedEventArgs)) *widget.Button {
	return widget.NewButton(
		widget.ButtonOpts.Image(style.PrimaryButtonImage()),
		widget.ButtonOpts.Text(text, style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(padding)),
		widget.ButtonOpts.ClickedHandler(handler),
	)
}

// ToggleButton creates a button that visually indicates an active/inactive state.
// Use for view mode toggles, filters, and other binary state buttons.
func ToggleButton(text string, active bool, handler func(*widget.ButtonClickedEventArgs)) *widget.Button {
	return widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(active)),
		widget.ButtonOpts.Text(text, style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.ClickedHandler(handler),
	)
}

// TooltipContent creates a tooltip container with consistent styling.
// Use for showing full text when content is truncated.
func TooltipContent(text string) *widget.Container {
	container := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Border)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(style.SmallSpacing)),
		)),
	)
	label := widget.NewText(
		widget.TextOpts.Text(text, style.FontFace(), style.Text),
	)
	container.AddChild(label)
	return container
}

// TableCell creates a table cell with text content.
// Use for data cells in list/table views.
func TableCell(text string, width, height int, textColor color.Color) *widget.Container {
	cell := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(width, height),
		),
	)
	label := widget.NewText(
		widget.TextOpts.Text(text, style.FontFace(), textColor),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				VerticalPosition: widget.AnchorLayoutPositionCenter,
			}),
		),
	)
	cell.AddChild(label)
	return cell
}

// TableHeaderCell creates a table header cell with secondary text color.
// Use for column headers in list/table views.
func TableHeaderCell(text string, width, height int) *widget.Container {
	return TableCell(text, width, height, style.TextSecondary)
}

// ScrollableOpts configures a scrollable container.
type ScrollableOpts struct {
	Content     *widget.Container // Required: content to scroll
	BgColor     color.Color       // style.Background color for scroll area (default: style.Background)
	BorderColor color.Color       // style.Border color for wrapper (nil = no border)
	Spacing     int               // Spacing between scroll area and slider (default: 4)
	Padding     int               // Padding inside wrapper, used with BorderColor (default: 0)
}

// ScrollableContainer creates a scrollable container with a vertical slider.
// Returns the scroll container, slider, and wrapper widget for embedding in layouts.
// The scroll container and slider references can be used for scroll position preservation.
func ScrollableContainer(opts ScrollableOpts) (*ScrollView, *widget.Slider, widget.PreferredSizeLocateableWidget) {
	// Apply defaults
	bgColor := opts.BgColor
	if bgColor == nil {
		bgColor = style.Background
	}
	spacing := opts.Spacing
	if spacing == 0 && opts.BorderColor == nil {
		spacing = 4 // Default spacing when no border
	}

	// Create scroll container (clips with a SubImage rather than an allocated
	// mask buffer; see ScrollView).
	scrollContainer := NewScrollView(opts.Content, bgColor, true)

	// Helper to check if scrolling is needed
	needsScroll := func() bool {
		contentHeight := scrollContainer.ContentRect().Dy()
		viewHeight := scrollContainer.ViewRect().Dy()
		return contentHeight > 0 && viewHeight > 0 && contentHeight > viewHeight
	}

	// Create vertical slider and pair it with the view so programmatic scroll
	// changes keep it in sync.
	vSlider := ScrollSlider(scrollContainer, needsScroll)
	scrollContainer.SetSlider(vSlider)

	// Setup mouse wheel scroll support
	SetupScrollHandler(scrollContainer, vSlider, needsScroll)

	// Create wrapper container
	var wrapperOpts []widget.ContainerOpt

	// Add border background if specified
	if opts.BorderColor != nil {
		wrapperOpts = append(wrapperOpts,
			widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(opts.BorderColor)),
		)
	}

	// Grid layout: stretching scroll area + fixed slider
	wrapperOpts = append(wrapperOpts,
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{true, false}, []bool{true}),
			widget.GridLayoutOpts.Spacing(spacing, 0),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(opts.Padding)),
		)),
	)

	wrapper := widget.NewContainer(wrapperOpts...)
	wrapper.AddChild(scrollContainer)
	wrapper.AddChild(vSlider)

	return scrollContainer, vSlider, wrapper
}

// CenteredContainer creates a container with vertical layout, centered in its parent.
// Use for modal dialogs, status screens, and centered content.
// The spacing parameter controls vertical spacing between children.
func CenteredContainer(spacing int) *widget.Container {
	return widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(spacing),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionCenter,
				VerticalPosition:   widget.AnchorLayoutPositionCenter,
			}),
		),
	)
}

// EmptyState creates a centered empty state display with title, optional subtitle, and optional button.
// The returned container has RowLayoutData{Stretch: true} for use in row layouts.
// Pass empty string for subtitle to omit it. Pass nil for button to omit it.
func EmptyState(title, subtitle string, button *widget.Button) *widget.Container {
	container := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Stretch: true,
			}),
		),
	)

	centerContent := CenteredContainer(style.DefaultSpacing)

	titleLabel := widget.NewText(
		widget.TextOpts.Text(title, style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	centerContent.AddChild(titleLabel)

	if subtitle != "" {
		subtitleLabel := widget.NewText(
			widget.TextOpts.Text(subtitle, style.FontFace(), style.TextSecondary),
			widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		)
		centerContent.AddChild(subtitleLabel)
	}

	if button != nil {
		centerContent.AddChild(button)
	}

	container.AddChild(centerContent)
	return container
}

// ScreenContainer creates a full-screen root container with background.
// The container uses AnchorLayout so children can stretch to fill.
func ScreenContainer() *widget.Container {
	return widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Background)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
}

// ScreenContentContainer creates an inner container for screen content.
// Uses a single-column GridLayout with default padding and spacing.
// The stretch parameter controls which rows stretch vertically.
func ScreenContentContainer(rowStretch []bool) *widget.Container {
	return widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(style.DefaultPadding)),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, style.DefaultSpacing),
			widget.GridLayoutOpts.Stretch([]bool{true}, rowStretch),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				StretchHorizontal: true,
				StretchVertical:   true,
			}),
		),
	)
}

// ButtonRow creates a horizontal container for buttons with standard spacing.
func ButtonRow() *widget.Container {
	return widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
	)
}

// AlternatingRowColor returns the appropriate background color for alternating rows.
// Even indices (0, 2, 4...) return style.Background, odd indices return style.Surface.
func AlternatingRowColor(index int) color.Color {
	if index%2 == 0 {
		return style.Background
	}
	return style.Surface
}

// TextInputGroup manages a group of text inputs with clipboard support.
// Call Update() each frame to handle Ctrl/Cmd+A/C/V/X shortcuts.
type TextInputGroup struct {
	inputs          []*widget.TextInput
	clipboardInited bool
}

// NewTextInputGroup creates a new text input group for clipboard handling.
func NewTextInputGroup() *TextInputGroup {
	return &TextInputGroup{}
}

// Add registers a text input with the group for clipboard handling.
func (g *TextInputGroup) Add(input *widget.TextInput) {
	g.inputs = append(g.inputs, input)
}

// Update handles clipboard shortcuts (Ctrl/Cmd+A/C/V/X) for the focused input.
// Call this each frame from your screen's Update method.
func (g *TextInputGroup) Update() {
	// Initialize clipboard on first use
	if !g.clipboardInited {
		if err := clipboard.Init(); err == nil {
			g.clipboardInited = true
		}
	}

	// Check for modifier key (Ctrl on Windows/Linux, Cmd on macOS)
	var modPressed bool
	if runtime.GOOS == "darwin" {
		modPressed = ebiten.IsKeyPressed(ebiten.KeyMeta) ||
			ebiten.IsKeyPressed(ebiten.KeyMetaLeft) ||
			ebiten.IsKeyPressed(ebiten.KeyMetaRight)
	} else {
		modPressed = ebiten.IsKeyPressed(ebiten.KeyControl) ||
			ebiten.IsKeyPressed(ebiten.KeyControlLeft) ||
			ebiten.IsKeyPressed(ebiten.KeyControlRight)
	}

	if !modPressed {
		return
	}

	// Find the focused text input
	var focused *widget.TextInput
	for _, input := range g.inputs {
		if input != nil && input.IsFocused() {
			focused = input
			break
		}
	}

	if focused == nil {
		return
	}

	// Ctrl/Cmd+A: Select all
	if inpututil.IsKeyJustPressed(ebiten.KeyA) {
		focused.SelectAll()
	}

	// Ctrl/Cmd+V: Paste
	if inpututil.IsKeyJustPressed(ebiten.KeyV) {
		if text := clipboard.Read(clipboard.FmtText); text != nil {
			focused.DeleteSelectedText()
			focused.Insert(string(text))
		}
	}

	// Ctrl/Cmd+C: Copy
	if inpututil.IsKeyJustPressed(ebiten.KeyC) {
		if selected := focused.SelectedText(); selected != "" {
			clipboard.Write(clipboard.FmtText, []byte(selected))
		}
	}

	// Ctrl/Cmd+X: Cut
	if inpututil.IsKeyJustPressed(ebiten.KeyX) {
		if selected := focused.SelectedText(); selected != "" {
			clipboard.Write(clipboard.FmtText, []byte(selected))
			focused.DeleteSelectedText()
		}
	}
}

// LabeledText creates a vertical container with a primary label and optional
// secondary subtext. style.Text is vertically centered within the grid cell.
func LabeledText(label, subtext string) *widget.Container {
	if subtext == "" {
		c := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewGridLayout(
				widget.GridLayoutOpts.Columns(1),
				widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
			)),
		)
		c.AddChild(widget.NewText(
			widget.TextOpts.Text(label, style.FontFace(), style.Text),
			widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
		))
		return c
	}

	c := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.TinySpacing),
		)),
	)
	c.AddChild(widget.NewText(
		widget.TextOpts.Text(label, style.FontFace(), style.Text),
	))
	c.AddChild(widget.NewText(
		widget.TextOpts.Text(subtext, style.FontFace(), style.TextSecondary),
	))
	return c
}

// StyledTextInput creates a text input with consistent styling.
func StyledTextInput(placeholder string, secure bool, minWidth int) *widget.TextInput {
	return widget.NewTextInput(
		widget.TextInputOpts.Image(&widget.TextInputImage{
			Idle:     image.NewNineSliceColor(style.Surface),
			Disabled: image.NewNineSliceColor(style.Border),
		}),
		widget.TextInputOpts.Face(style.FontFace()),
		widget.TextInputOpts.Color(&widget.TextInputColor{
			Idle:          style.Text,
			Disabled:      style.TextSecondary,
			Caret:         style.Text,
			DisabledCaret: style.TextSecondary,
		}),
		widget.TextInputOpts.Padding(widget.NewInsetsSimple(style.SmallSpacing)),
		widget.TextInputOpts.Placeholder(placeholder),
		widget.TextInputOpts.Secure(secure),
		widget.TextInputOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(minWidth, 0),
		),
	)
}
