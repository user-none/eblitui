//go:build !libretro

package settings

import (
	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	emucore "github.com/user-none/eblitui/api"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
	"github.com/user-none/eblitui/standalone/types"
)

// keyToNameFunc and related are injected from the standalone package to
// avoid import cycles. They must be set before InputSection is used.
var (
	KeyToNameFunc  func(ebiten.Key) (string, bool)
	PadToNameFunc  func(ebiten.StandardGamepadButton) (string, bool)
	IsReservedFunc func(ebiten.Key) bool
	ResolveKeyFunc func(string, string, map[string]string) string
	ResolvePadFunc func(string, string, map[string]string) string
)

// dpadEntry describes a d-pad button for the input settings UI.
type dpadEntry struct {
	Name       string
	DefaultKey string
	DefaultPad string
}

var dpadEntries = []dpadEntry{
	{"Up", "W", "DpadUp"},
	{"Down", "S", "DpadDown"},
	{"Left", "A", "DpadLeft"},
	{"Right", "D", "DpadRight"},
}

// InputSection manages input binding settings
type InputSection struct {
	callback   types.ScreenCallback
	config     *storage.Config
	systemInfo emucore.SystemInfo
	focus      types.FocusManager

	// Capture state
	capturing   bool
	captureType string // "keyboard" or "controller"
	captureBtn  string // button name being captured (e.g. "Up", "A")
}

// NewInputSection creates a new input section
func NewInputSection(callback types.ScreenCallback, config *storage.Config, systemInfo emucore.SystemInfo) *InputSection {
	return &InputSection{
		callback:   callback,
		config:     config,
		systemInfo: systemInfo,
	}
}

// SetConfig updates the config reference
func (s *InputSection) SetConfig(config *storage.Config) {
	s.config = config
}

// SystemInfo returns the system info for navigation setup
func (s *InputSection) SystemInfo() emucore.SystemInfo {
	return s.systemInfo
}

// IsCapturing returns true when the section is waiting for a key/button press
func (s *InputSection) IsCapturing() bool {
	return s.capturing
}

// Update handles per-frame input capture logic
func (s *InputSection) Update() {
	if !s.capturing {
		return
	}

	switch s.captureType {
	case "keyboard":
		// ESC cancels
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			s.cancelCapture()
			return
		}
		keys := inpututil.AppendJustPressedKeys(nil)
		for _, k := range keys {
			if IsReservedFunc != nil && IsReservedFunc(k) {
				continue
			}
			if KeyToNameFunc == nil {
				continue
			}
			name, ok := KeyToNameFunc(k)
			if !ok {
				continue
			}
			s.applyKeyboardBinding(s.captureBtn, name)
			return
		}

	case "controller":
		// ESC cancels
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			s.cancelCapture()
			return
		}
		gamepadIDs := ebiten.AppendGamepadIDs(nil)
		if len(gamepadIDs) == 0 {
			return
		}
		for btn := ebiten.StandardGamepadButton(0); btn <= ebiten.StandardGamepadButtonMax; btn++ {
			if inpututil.IsStandardGamepadButtonJustPressed(gamepadIDs[0], btn) {
				if PadToNameFunc == nil {
					continue
				}
				name, ok := PadToNameFunc(btn)
				if !ok {
					continue
				}
				s.applyControllerBinding(s.captureBtn, name)
				return
			}
		}
	}
}

// cancelCapture exits capture mode without changing bindings
func (s *InputSection) cancelCapture() {
	s.capturing = false
	switch s.captureType {
	case "keyboard":
		s.focus.SetPendingFocus("input-kb-" + s.captureBtn)
	case "controller":
		s.focus.SetPendingFocus("input-pad-" + s.captureBtn)
	}
	s.callback.RequestRebuild()
}

// applyKeyboardBinding saves a keyboard binding and exits capture mode
func (s *InputSection) applyKeyboardBinding(buttonName, keyName string) {
	if s.config.Input.P1Keyboard == nil {
		s.config.Input.P1Keyboard = make(map[string]string)
	}

	// Check if this is the default - if so, remove the override
	defaultKey := s.defaultKeyForButton(buttonName)
	if keyName == defaultKey {
		delete(s.config.Input.P1Keyboard, buttonName)
		if len(s.config.Input.P1Keyboard) == 0 {
			s.config.Input.P1Keyboard = nil
		}
	} else {
		s.config.Input.P1Keyboard[buttonName] = keyName
	}

	storage.SaveConfig(s.config)
	s.capturing = false
	s.focus.SetPendingFocus("input-kb-" + buttonName)
	s.callback.RequestRebuild()
}

// applyControllerBinding saves a controller binding and exits capture mode
func (s *InputSection) applyControllerBinding(buttonName, padName string) {
	if s.config.Input.P1Controller == nil {
		s.config.Input.P1Controller = make(map[string]string)
	}

	// Check if this is the default - if so, remove the override
	defaultPad := s.defaultPadForButton(buttonName)
	if padName == defaultPad {
		delete(s.config.Input.P1Controller, buttonName)
		if len(s.config.Input.P1Controller) == 0 {
			s.config.Input.P1Controller = nil
		}
	} else {
		s.config.Input.P1Controller[buttonName] = padName
	}

	storage.SaveConfig(s.config)
	s.capturing = false
	s.focus.SetPendingFocus("input-pad-" + buttonName)
	s.callback.RequestRebuild()
}

// defaultKeyForButton returns the default keyboard key name for a button
func (s *InputSection) defaultKeyForButton(buttonName string) string {
	for _, dp := range dpadEntries {
		if dp.Name == buttonName {
			return dp.DefaultKey
		}
	}
	for _, btn := range s.systemInfo.Buttons {
		if btn.Name == buttonName {
			return btn.DefaultKey
		}
	}
	return ""
}

// defaultPadForButton returns the default controller button name for a button
func (s *InputSection) defaultPadForButton(buttonName string) string {
	for _, dp := range dpadEntries {
		if dp.Name == buttonName {
			return dp.DefaultPad
		}
	}
	for _, btn := range s.systemInfo.Buttons {
		if btn.Name == buttonName {
			return btn.DefaultPad
		}
	}
	return ""
}

// Build creates the input section UI
func (s *InputSection) Build(focus types.FocusManager) *widget.Container {
	s.focus = focus

	outer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
		)),
	)

	section := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
	)

	// Core options section (options with Category == "Input")
	hasInputOptions := false
	for _, opt := range s.systemInfo.CoreOptions {
		if opt.Category == emucore.CoreOptionCategoryInput {
			hasInputOptions = true
			section.AddChild(buildCoreOptionRow(focus, s.callback, s.config, opt, "input"))
		}
	}

	if hasInputOptions {
		// Spacer between core options and bindings
		section.AddChild(widget.NewText(
			widget.TextOpts.Text("", style.FontFace(), style.TextSecondary),
		))
	}

	// Analog stick toggle
	section.AddChild(s.buildAnalogStickRow(focus))

	// Button bindings header
	headerLabel := widget.NewText(
		widget.TextOpts.Text("Button Bindings", style.FontFace(), style.Accent),
	)
	section.AddChild(headerLabel)

	// Column headers row
	section.AddChild(s.buildHeaderRow())

	// D-pad rows
	for _, dp := range dpadEntries {
		section.AddChild(s.buildBindingRow(focus, dp.Name, dp.DefaultKey, dp.DefaultPad))
	}

	// Adaptor button rows
	for _, btn := range s.systemInfo.Buttons {
		section.AddChild(s.buildBindingRow(focus, btn.Name, btn.DefaultKey, btn.DefaultPad))
	}

	// Reset buttons row
	section.AddChild(s.buildResetRow(focus))

	s.setupNavigation(focus)

	// Wrap in scrollable container
	scrollContainer, vSlider, scrollWrapper := style.ScrollableContainer(style.ScrollableOpts{
		Content:     section,
		BgColor:     style.Background,
		BorderColor: style.Border,
		Spacing:     0,
		Padding:     style.SmallSpacing,
	})
	focus.SetScrollWidgets(scrollContainer, vSlider)
	focus.RestoreScrollPosition()
	outer.AddChild(scrollWrapper)
	return outer
}

// setupNavigation registers navigation zones for the input section
func (s *InputSection) setupNavigation(focus types.FocusManager) {
	// Core options zone
	coreOptKeys := make([]string, 0)
	for _, opt := range s.systemInfo.CoreOptions {
		if opt.Category == emucore.CoreOptionCategoryInput {
			coreOptKeys = append(coreOptKeys, "input-opt-"+opt.Key)
		}
	}
	if len(coreOptKeys) > 0 {
		focus.RegisterNavZone("input-core-opts", types.NavZoneVertical, coreOptKeys, 0)
	}

	// Analog stick toggle zone
	focus.RegisterNavZone("input-analog-stick", types.NavZoneVertical, []string{"input-analog-stick"}, 0)

	// Binding buttons zone: 2-column grid (keyboard col, controller col)
	bindingKeys := make([]string, 0)
	for _, dp := range dpadEntries {
		bindingKeys = append(bindingKeys, "input-kb-"+dp.Name)
		bindingKeys = append(bindingKeys, "input-pad-"+dp.Name)
	}
	for _, btn := range s.systemInfo.Buttons {
		bindingKeys = append(bindingKeys, "input-kb-"+btn.Name)
		bindingKeys = append(bindingKeys, "input-pad-"+btn.Name)
	}
	if len(bindingKeys) > 0 {
		focus.RegisterNavZone("input-bindings", types.NavZoneGrid, bindingKeys, 2)
	}

	// Reset zone
	focus.RegisterNavZone("input-reset", types.NavZoneHorizontal, []string{"input-reset-kb", "input-reset-pad"}, 0)

	// Transitions
	if len(coreOptKeys) > 0 {
		focus.SetNavTransition("input-core-opts", types.DirDown, "input-analog-stick", types.NavIndexFirst)
		focus.SetNavTransition("input-analog-stick", types.DirUp, "input-core-opts", types.NavIndexFirst)
	}
	focus.SetNavTransition("input-analog-stick", types.DirDown, "input-bindings", types.NavIndexFirst)
	focus.SetNavTransition("input-bindings", types.DirUp, "input-analog-stick", types.NavIndexFirst)
	focus.SetNavTransition("input-bindings", types.DirDown, "input-reset", types.NavIndexFirst)
	focus.SetNavTransition("input-reset", types.DirUp, "input-bindings", types.NavIndexLast)
}

// buildHeaderRow creates the column headers for the binding table
func (s *InputSection) buildHeaderRow() *widget.Container {
	row := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			widget.GridLayoutOpts.Stretch([]bool{true, false, false}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
			widget.GridLayoutOpts.Padding(&widget.Insets{
				Left:  style.SmallSpacing,
				Right: style.SmallSpacing,
			}),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	row.AddChild(widget.NewText(
		widget.TextOpts.Text("Button", style.FontFace(), style.TextSecondary),
	))
	row.AddChild(widget.NewText(
		widget.TextOpts.Text("Keyboard", style.FontFace(), style.TextSecondary),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(style.Px(90), 0)),
	))
	row.AddChild(widget.NewText(
		widget.TextOpts.Text("Controller", style.FontFace(), style.TextSecondary),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(style.Px(90), 0)),
	))

	return row
}

// buildBindingRow creates a row for a single button binding
func (s *InputSection) buildBindingRow(focus types.FocusManager, buttonName, defaultKey, defaultPad string) *widget.Container {
	row := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Surface)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			widget.GridLayoutOpts.Stretch([]bool{true, false, false}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(style.SmallSpacing)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	// Button name label
	row.AddChild(widget.NewText(
		widget.TextOpts.Text(buttonName, style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		),
	))

	// Keyboard binding button
	kbDisplay := s.resolveKeyDisplay(buttonName, defaultKey)
	kbFocusKey := "input-kb-" + buttonName

	if s.capturing && s.captureType == "keyboard" && s.captureBtn == buttonName {
		kbDisplay = "Press a key..."
	}

	kbBtn := widget.NewButton(
		widget.ButtonOpts.Image(s.bindingButtonImage(buttonName, defaultKey, s.config.Input.P1Keyboard)),
		widget.ButtonOpts.Text(kbDisplay, style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(90), 0),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			s.capturing = true
			s.captureType = "keyboard"
			s.captureBtn = buttonName
			focus.SetPendingFocus(kbFocusKey)
			s.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton(kbFocusKey, kbBtn)
	row.AddChild(kbBtn)

	// Controller binding button
	padDisplay := s.resolvePadDisplay(buttonName, defaultPad)
	padFocusKey := "input-pad-" + buttonName

	if s.capturing && s.captureType == "controller" && s.captureBtn == buttonName {
		padDisplay = "Press a button..."
	}

	padBtn := widget.NewButton(
		widget.ButtonOpts.Image(s.bindingButtonImage(buttonName, defaultPad, s.config.Input.P1Controller)),
		widget.ButtonOpts.Text(padDisplay, style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(90), 0),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			s.capturing = true
			s.captureType = "controller"
			s.captureBtn = buttonName
			focus.SetPendingFocus(padFocusKey)
			s.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton(padFocusKey, padBtn)
	row.AddChild(padBtn)

	return row
}

// resolveKeyDisplay returns the display string for a keyboard binding
func (s *InputSection) resolveKeyDisplay(buttonName, defaultKey string) string {
	if ResolveKeyFunc != nil {
		return ResolveKeyFunc(buttonName, defaultKey, s.config.Input.P1Keyboard)
	}
	if override, ok := s.config.Input.P1Keyboard[buttonName]; ok {
		return override
	}
	return defaultKey
}

// resolvePadDisplay returns the display string for a controller binding
func (s *InputSection) resolvePadDisplay(buttonName, defaultPad string) string {
	if ResolvePadFunc != nil {
		return ResolvePadFunc(buttonName, defaultPad, s.config.Input.P1Controller)
	}
	if override, ok := s.config.Input.P1Controller[buttonName]; ok {
		return override
	}
	return defaultPad
}

// bindingButtonImage returns the button image based on whether the binding is overridden
func (s *InputSection) bindingButtonImage(buttonName, defaultVal string, overrides map[string]string) *widget.ButtonImage {
	if override, ok := overrides[buttonName]; ok && override != defaultVal {
		return style.ActiveButtonImage(true)
	}
	return style.ButtonImage()
}

// buildResetRow creates the reset buttons row (right-justified)
func (s *InputSection) buildResetRow(focus types.FocusManager) *widget.Container {
	row := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			// First column stretches to push buttons right
			widget.GridLayoutOpts.Stretch([]bool{true, false, false}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	// Spacer
	row.AddChild(widget.NewContainer())

	resetKBBtn := style.TextButton("Reset Keyboard", style.ButtonPaddingSmall, func(args *widget.ButtonClickedEventArgs) {
		s.config.Input.P1Keyboard = nil
		storage.SaveConfig(s.config)
		focus.SetPendingFocus("input-reset-kb")
		s.callback.RequestRebuild()
	})
	focus.RegisterFocusButton("input-reset-kb", resetKBBtn)
	row.AddChild(resetKBBtn)

	resetPadBtn := style.TextButton("Reset Controller", style.ButtonPaddingSmall, func(args *widget.ButtonClickedEventArgs) {
		s.config.Input.P1Controller = nil
		storage.SaveConfig(s.config)
		focus.SetPendingFocus("input-reset-pad")
		s.callback.RequestRebuild()
	})
	focus.RegisterFocusButton("input-reset-pad", resetPadBtn)
	row.AddChild(resetPadBtn)

	return row
}

// buildAnalogStickRow creates the "Disable Analog Stick" toggle row
func (s *InputSection) buildAnalogStickRow(focus types.FocusManager) *widget.Container {
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

	label := widget.NewText(
		widget.TextOpts.Text("Disable Analog Stick", style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		),
	)
	row.AddChild(label)

	isDisabled := s.config.Input.DisableAnalogStick
	toggleBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(isDisabled)),
		widget.ButtonOpts.Text(boolToOnOff(isDisabled), style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(50), 0),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			s.config.Input.DisableAnalogStick = !s.config.Input.DisableAnalogStick
			storage.SaveConfig(s.config)
			focus.SetPendingFocus("input-analog-stick")
			s.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton("input-analog-stick", toggleBtn)
	row.AddChild(toggleBtn)

	return row
}
