package settings

import (
	"fmt"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
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

// maxProfileNameLen bounds user-entered profile names.
const maxProfileNameLen = 32

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

// inputView identifies which view the input section is showing.
type inputView int

const (
	inputViewMain inputView = iota
	inputViewKeyboard
	inputViewProfile
	inputViewNewProfile
)

// padModel is a connected controller model (deduped by identity key).
type padModel struct {
	sdlID string
	name  string
	count int
}

// InputSection manages input binding settings
type InputSection struct {
	callback   types.ScreenCallback
	config     *storage.Config
	systemInfo coreif.SystemInfo
	focus      types.FocusManager

	view      inputView
	profileID string // profile being edited in inputViewProfile

	// Capture state
	capturing  bool
	captureBtn string // button name being captured (e.g. "Up", "A")

	// Name entry state (profile edit and new profile views)
	textInputs *widgets.TextInputGroup
	nameInput  *widget.TextInput
	nameError  string

	// New-profile controller selection
	newSDLID      string
	newController string
}

// NewInputSection creates a new input section
func NewInputSection(callback types.ScreenCallback, config *storage.Config, systemInfo coreif.SystemInfo) *InputSection {
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
func (s *InputSection) SystemInfo() coreif.SystemInfo {
	return s.systemInfo
}

// IsCapturing returns true when the section is waiting for a key/button press
func (s *InputSection) IsCapturing() bool {
	return s.capturing
}

// playerCount returns the number of player slots to show, bounded by the
// core's player support and the app's player limit.
func (s *InputSection) playerCount() int {
	n := s.systemInfo.Players
	if n < 1 {
		n = 1
	}
	if n > storage.MaxInputPlayers {
		n = storage.MaxInputPlayers
	}
	return n
}

// connectedModels returns the connected controller models, deduped by
// identity key, in gamepad ID order.
func connectedModels() []padModel {
	var models []padModel
	for _, id := range ebiten.AppendGamepadIDs(nil) {
		sdlID := ebiten.GamepadSDLID(id)
		name := ebiten.GamepadName(id)
		found := false
		for i := range models {
			if models[i].sdlID == sdlID && models[i].name == name {
				models[i].count++
				found = true
				break
			}
		}
		if !found {
			models = append(models, padModel{sdlID: sdlID, name: name, count: 1})
		}
	}
	return models
}

// matchingPadIDs returns the connected gamepads matching the profile's model.
func matchingPadIDs(p *storage.ControllerProfile) []ebiten.GamepadID {
	var out []ebiten.GamepadID
	for _, id := range ebiten.AppendGamepadIDs(nil) {
		if p.MatchesPad(ebiten.GamepadSDLID(id), ebiten.GamepadName(id)) {
			out = append(out, id)
		}
	}
	return out
}

// profileDisplay returns the display label for a profile.
func profileDisplay(p *storage.ControllerProfile) string {
	return p.Controller + " - " + p.Name
}

// Update handles per-frame input capture and text input shortcuts
func (s *InputSection) Update() {
	if s.textInputs != nil {
		s.textInputs.Update()
	}

	if !s.capturing {
		return
	}

	// ESC cancels
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		s.cancelCapture()
		return
	}

	switch s.view {
	case inputViewKeyboard:
		s.updateKeyboardCapture()
	case inputViewProfile:
		s.updateProfileCapture()
	}
}

// cancelCapture exits capture mode without changing bindings
func (s *InputSection) cancelCapture() {
	s.capturing = false
	switch s.view {
	case inputViewKeyboard:
		s.focus.SetPendingFocus("input-kb-" + s.captureBtn)
	case inputViewProfile:
		s.focus.SetPendingFocus("input-pad-" + s.captureBtn)
	}
	s.callback.RequestRebuild()
}

// HandleBack handles back navigation within the section. Returns true when
// the back action was consumed (capture cancelled or sub-view exited).
func (s *InputSection) HandleBack() bool {
	if s.capturing {
		s.cancelCapture()
		return true
	}
	if s.view == inputViewMain {
		return false
	}

	var focusKey string
	switch s.view {
	case inputViewKeyboard:
		focusKey = "input-kb-edit"
	case inputViewProfile:
		focusKey = "input-profile-" + s.profileID + "-edit"
	case inputViewNewProfile:
		focusKey = "input-new-profile"
	}
	s.switchView(inputViewMain, focusKey)
	return true
}

// switchView changes the visible view and resets transient view state.
func (s *InputSection) switchView(view inputView, focusKey string) {
	s.view = view
	s.capturing = false
	s.nameError = ""
	s.textInputs = nil
	s.nameInput = nil
	if view != inputViewProfile {
		s.profileID = ""
	}
	if view != inputViewNewProfile {
		s.newSDLID = ""
		s.newController = ""
	}
	if focusKey != "" {
		s.focus.SetPendingFocus(focusKey)
	}
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

// validateProfileName checks a profile name for the given controller model.
// excludeID skips a profile (for rename). Returns an error message or "".
func (s *InputSection) validateProfileName(name, sdlID, controller, excludeID string) string {
	if name == "" {
		return "Name required"
	}
	if len(name) > maxProfileNameLen {
		return fmt.Sprintf("Name too long (max %d)", maxProfileNameLen)
	}
	for i := range s.config.Input.Profiles {
		p := &s.config.Input.Profiles[i]
		if p.ID == excludeID {
			continue
		}
		if p.SDLID == sdlID && p.Controller == controller && p.Name == name {
			return fmt.Sprintf("A %s profile named %q already exists", controller, name)
		}
	}
	return ""
}

// Build creates the input section UI for the current view
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

	switch s.view {
	case inputViewKeyboard:
		s.buildKeyboardView(focus, section)
	case inputViewProfile:
		s.buildProfileView(focus, section)
	case inputViewNewProfile:
		s.buildNewProfileView(focus, section)
	default:
		s.buildMainView(focus, section)
	}

	// Wrap in scrollable container
	scrollContainer, _, scrollWrapper := widgets.ScrollableContainer(widgets.ScrollableOpts{
		Content:     section,
		BgColor:     style.Background,
		BorderColor: style.Border,
		Spacing:     0,
		Padding:     style.SmallSpacing,
	})
	focus.SetScrollContainer(scrollContainer)
	focus.RestoreScrollPosition()
	outer.AddChild(scrollWrapper)
	return outer
}

// sectionHeader creates an accent header text row.
func sectionHeader(text string) *widget.Text {
	return widget.NewText(
		widget.TextOpts.Text(text, style.FontFace(), style.Accent),
	)
}

// noticeText creates a secondary-colored informational text row.
func noticeText(text string) *widget.Text {
	return widget.NewText(
		widget.TextOpts.Text(text, style.FontFace(), style.TextSecondary),
	)
}

// chainZones links zones vertically in order with up/down transitions.
func chainZones(focus types.FocusManager, zones []string) {
	for i := 0; i < len(zones)-1; i++ {
		focus.SetNavTransition(zones[i], types.DirDown, zones[i+1], types.NavIndexFirst)
		focus.SetNavTransition(zones[i+1], types.DirUp, zones[i], types.NavIndexLast)
	}
}

// buildHeaderRow creates the column headers for a binding table
func (s *InputSection) buildHeaderRow(bindingColumn string) *widget.Container {
	row := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{true, false}, []bool{true}),
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
		widget.TextOpts.Text(bindingColumn, style.FontFace(), style.TextSecondary),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(style.Px(90), 0)),
	))

	return row
}

// bindingFocusKeys returns the focus keys for all binding rows with the
// given prefix, in display order.
func (s *InputSection) bindingFocusKeys(prefix string) []string {
	keys := make([]string, 0, len(dpadEntries)+len(s.systemInfo.Buttons))
	for _, dp := range dpadEntries {
		keys = append(keys, prefix+dp.Name)
	}
	for _, btn := range s.systemInfo.Buttons {
		keys = append(keys, prefix+btn.Name)
	}
	return keys
}

// actionButton describes a right-aligned action row button.
type actionButton struct {
	label    string
	focusKey string
	handler  func()
}

// buildActionRow creates a right-justified row of action buttons.
func (s *InputSection) buildActionRow(focus types.FocusManager, actions ...actionButton) *widget.Container {
	row := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(len(actions)+1),
			// First column stretches to push buttons right
			widget.GridLayoutOpts.Stretch(append([]bool{true}, make([]bool, len(actions))...), []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	// Spacer
	row.AddChild(widget.NewContainer())

	for _, a := range actions {
		handler := a.handler
		btn := widgets.TextButton(a.label, style.ButtonPaddingSmall, func(args *widget.ButtonClickedEventArgs) {
			handler()
		})
		focus.RegisterFocusButton(a.focusKey, btn)
		row.AddChild(btn)
	}

	return row
}

// buildNameRow creates the profile name entry row.
func (s *InputSection) buildNameRow(current string) *widget.Container {
	row := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	row.AddChild(widget.NewText(
		widget.TextOpts.Text("Name", style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(style.Px(80), 0),
		),
	))

	s.textInputs = widgets.NewTextInputGroup()
	s.nameInput = widgets.StyledTextInput("Profile name", false, style.Px(200))
	s.textInputs.Add(s.nameInput)
	if current != "" {
		s.nameInput.SetText(current)
	}
	row.AddChild(s.nameInput)

	return row
}

// resolvePadDisplay returns the display string for a controller binding
func resolvePadDisplay(buttonName, defaultPad string, overrides map[string]string) string {
	if ResolvePadFunc != nil {
		return ResolvePadFunc(buttonName, defaultPad, overrides)
	}
	if override, ok := overrides[buttonName]; ok {
		return override
	}
	return defaultPad
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

// bindingButtonImage returns the button image based on whether the binding is overridden
func (s *InputSection) bindingButtonImage(buttonName, defaultVal string, overrides map[string]string) *widget.ButtonImage {
	if override, ok := overrides[buttonName]; ok && override != defaultVal {
		return style.ActiveButtonImage(true)
	}
	return style.ButtonImage()
}

// FirstNavZone returns the first content zone for sidebar navigation in the
// current view.
func (s *InputSection) FirstNavZone() string {
	switch s.view {
	case inputViewKeyboard:
		return "input-kb-bindings"
	case inputViewProfile:
		return "input-pad-bindings"
	case inputViewNewProfile:
		if len(connectedModels()) > 0 {
			return "input-models"
		}
		return "input-newp-actions"
	}
	for _, opt := range s.systemInfo.CoreOptions {
		if opt.Category == coreif.CoreOptionCategoryInput {
			return "input-core-opts"
		}
	}
	return "input-analog-stick"
}

// NavZones returns all content zones for the current view, for wiring
// left-exit transitions to the sidebar.
func (s *InputSection) NavZones() []string {
	switch s.view {
	case inputViewKeyboard:
		return []string{"input-kb-bindings", "input-kb-actions"}
	case inputViewProfile:
		return []string{"input-pad-bindings", "input-profile-actions"}
	case inputViewNewProfile:
		return []string{"input-models", "input-newp-actions"}
	}
	return []string{
		"input-core-opts", "input-analog-stick", "input-rumble",
		"input-players", "input-kb-edit", "input-profiles", "input-new-profile",
	}
}
