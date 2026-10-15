// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"path/filepath"
	"slices"
	"time"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/user-none/eblitui/desktop/filebrowser"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// placesPollInterval is how often the sidebar places are re-read so drives
// plugged in while the picker is open show up.
const placesPollInterval = 2 * time.Second

// maxEntryNameChars is the length list entry names are truncated to.
const maxEntryNameChars = 80

// Focus keys for the fixed picker buttons.
const (
	pickerUpKey     = "picker-up"
	pickerAcceptKey = "picker-accept"
	pickerCancelKey = "picker-cancel"
)

// FilePickerScreen is the full-screen file and folder picker. The path field
// is the source of truth: in folder mode Accept takes the folder the field
// names, in file mode the file selected from the list or typed and submitted.
type FilePickerScreen struct {
	BaseScreen

	callback ScreenCallback
	close    func()

	req      types.FilePickerRequest
	dir      string              // directory the list shows
	entries  []filebrowser.Entry // unfiltered listing of dir
	filter   string              // typed text after the last separator
	dirError string              // shown in place of the list when the typed folder is invalid
	selected string              // file mode: path of the selected file
	places   []filebrowser.Place
	lastPoll time.Time

	// pathTextValue mirrors the field text. It is updated on every edit and
	// programmatic change so it survives rebuilds and exists before the widget.
	pathTextValue      string
	settingText        bool // suppresses onPathChanged during programmatic SetText
	pathFocusRequested bool
	selectAllOnFocus   bool

	pathInput    *widget.TextInput
	textInputs   *widgets.TextInputGroup
	listContent  *widget.Container
	sidebar      *widgets.Sidebar
	acceptBtn    *widget.Button
	cancelBtn    *widget.Button
	placeButtons []*widget.Button
	placeKeys    []string
	listKeys     []string
	listButtons  map[string]*widget.Button
}

// NewFilePickerScreen creates the picker. close is called on Accept or Cancel
// to return to the screen that opened it.
func NewFilePickerScreen(callback ScreenCallback, close func()) *FilePickerScreen {
	s := &FilePickerScreen{
		callback:    callback,
		close:       close,
		listButtons: make(map[string]*widget.Button),
	}
	s.InitBase()
	return s
}

// Open resets the picker for a new request, starting in the home folder.
func (s *FilePickerScreen) Open(req types.FilePickerRequest) {
	s.req = req
	s.filter = ""
	s.selected = ""
	s.dirError = ""
	s.pathFocusRequested = false
	s.selectAllOnFocus = false
	s.places = filebrowser.Places()
	s.lastPoll = time.Now()
	s.scrollTop = 0
	s.ClearPendingFocus()
	home := filebrowser.HomeDir()
	if err := s.loadDir(home); err != nil {
		s.dir = filepath.Clean(home)
		s.entries = nil
		s.dirError = "Cannot open folder"
	}
	s.setPathText(filebrowser.DirText(s.dir))
	s.SetPendingFocus(s.firstListKey())
}

// loadDir lists dir and makes it current. On error nothing changes.
func (s *FilePickerScreen) loadDir(dir string) error {
	entries, err := filebrowser.ReadDir(dir, s.req.Mode)
	if err != nil {
		return err
	}
	s.dir = filepath.Clean(dir)
	s.entries = entries
	s.dirError = ""
	return nil
}

// enterDir navigates into dir from the list or sidebar, clearing the filter
// and selection. Unreadable directories show a notification and leave the
// picker where it is.
func (s *FilePickerScreen) enterDir(dir string) {
	if err := s.loadDir(dir); err != nil {
		s.callback.ShowNotification("Cannot open folder")
		return
	}
	s.filter = ""
	s.selected = ""
	s.setPathText(filebrowser.DirText(s.dir))
	s.refreshList(true, "")
}

// selectFile marks a file as the selection and puts its path in the field
// without changing the list filter.
func (s *FilePickerScreen) selectFile(path string) {
	s.selected = path
	s.setPathText(path)
	s.refreshList(false, entryKey(path))
}

// onPathChanged handles typing in the path field. The text up to the last
// separator names the listed folder; the rest filters it. Typing clears any
// file selection.
func (s *FilePickerScreen) onPathChanged(text string) {
	// ebitenui queues change events and runs them after the frame's update,
	// so the event from setPathText arrives after settingText is cleared.
	// Text equal to what was last set is that event, not an edit.
	if s.settingText || text == s.pathTextValue {
		return
	}
	s.pathTextValue = text
	s.selected = ""
	dirPart, filter := filebrowser.SplitInput(text)
	s.filter = filter
	switch {
	case dirPart == "":
		s.dirError = ""
	case !filepath.IsAbs(dirPart):
		s.dirError = "Folder not found"
	case filepath.Clean(dirPart) == s.dir:
		s.dirError = ""
	default:
		if err := s.loadDir(dirPart); err != nil {
			s.dirError = "Cannot open folder"
		}
	}
	s.refreshList(true, "")
}

// onPathSubmit handles Enter in the path field: open a typed folder, or select
// a typed file in file mode. It never accepts.
func (s *FilePickerScreen) onPathSubmit(text string) {
	if p, err := filebrowser.Resolve(text, filebrowser.ModeFolder); err == nil {
		s.enterDir(p)
		return
	}
	if s.req.Mode != filebrowser.ModeFile {
		return
	}
	p, err := filebrowser.Resolve(text, filebrowser.ModeFile)
	if err != nil {
		return
	}
	if err := s.loadDir(filepath.Dir(p)); err != nil {
		s.callback.ShowNotification("Cannot open folder")
		return
	}
	s.filter = ""
	s.selectFile(p)
}

// canAccept reports whether Accept has a complete choice: a selected file in
// file mode, or in folder mode a field naming exactly the listed folder with
// no filter text.
func (s *FilePickerScreen) canAccept() bool {
	if s.req.Mode == filebrowser.ModeFile {
		return s.selected != ""
	}
	if s.dirError != "" || s.dir == "" {
		return false
	}
	dirPart, filter := filebrowser.SplitInput(s.pathTextValue)
	return filter == "" && dirPart != "" && filepath.IsAbs(dirPart) && filepath.Clean(dirPart) == s.dir
}

// accept closes the picker and passes the choice to the request's OnSelect.
// The choice is checked again in case it was removed while the picker was open.
func (s *FilePickerScreen) accept() {
	if !s.canAccept() {
		return
	}
	choice := s.dir
	if s.req.Mode == filebrowser.ModeFile {
		choice = s.selected
	}
	p, err := filebrowser.Resolve(choice, s.req.Mode)
	if err != nil {
		s.callback.ShowNotification("Selection no longer exists")
		return
	}
	onSelect := s.req.OnSelect
	s.close()
	if onSelect != nil {
		onSelect(p)
	}
}

// Cancel closes the picker without a result.
func (s *FilePickerScreen) Cancel() {
	s.close()
}

// setPathText sets the field without triggering onPathChanged.
func (s *FilePickerScreen) setPathText(text string) {
	s.pathTextValue = text
	if s.pathInput == nil {
		return
	}
	s.settingText = true
	s.pathInput.SetText(text)
	s.settingText = false
}

// firstListKey returns the focus key of the first list row, or the Cancel
// button when the list is empty.
func (s *FilePickerScreen) firstListKey() string {
	if s.dirError == "" {
		if _, ok := filebrowser.Parent(s.dir); ok {
			return pickerUpKey
		}
		if shown := filebrowser.Filter(s.entries, s.filter); len(shown) > 0 {
			return entryKey(shown[0].Path)
		}
	}
	return pickerCancelKey
}

// entryKey returns the focus key for a list entry.
func entryKey(path string) string {
	return "picker-entry:" + path
}

// Build creates the picker UI. The sidebar and list are filled by
// refreshPlaces and refreshList so later changes update them in place without
// recreating the path field.
func (s *FilePickerScreen) Build() *widget.Container {
	root := widgets.ScreenContainer()
	content := widgets.ScreenContentContainer([]bool{false, true})

	content.AddChild(widget.NewText(
		widget.TextOpts.Text(s.req.Title, style.FontFace(), style.Text),
	))

	main := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			// Col 0 (sidebar) = fixed, Col 1 (path, list, buttons) = stretch
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
		)),
	)

	s.sidebar = widgets.NewSidebar(style.SidebarMinWidth)
	main.AddChild(s.sidebar.Container)

	right := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			// Row stretch: path=no, list=YES, buttons=no
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true, false}),
			widget.GridLayoutOpts.Spacing(0, style.DefaultSpacing),
		)),
	)

	s.textInputs = widgets.NewTextInputGroup()
	s.pathInput = widgets.StyledTextInput("", false, 0,
		widget.TextInputOpts.ChangedHandler(func(args *widget.TextInputChangedEventArgs) {
			s.onPathChanged(args.InputText)
		}),
		widget.TextInputOpts.SubmitHandler(func(args *widget.TextInputChangedEventArgs) {
			s.onPathSubmit(args.InputText)
		}),
		widget.TextInputOpts.AllowDuplicateSubmit(true),
	)
	s.setPathText(s.pathTextValue)
	s.textInputs.Add(s.pathInput)
	right.AddChild(s.pathInput)

	s.listContent = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(0),
		)),
	)
	scroll, _, wrapper := widgets.ScrollableContainer(widgets.ScrollableOpts{
		Content:     s.listContent,
		BgColor:     style.Surface,
		BorderColor: style.Border,
		Spacing:     0,
		Padding:     2,
	})
	s.SetScrollContainer(scroll)
	right.AddChild(wrapper)

	right.AddChild(s.buildButtons())
	main.AddChild(right)
	content.AddChild(main)
	root.AddChild(content)

	s.refreshPlaces()
	s.refreshList(false, "")
	s.RestoreScrollPosition()
	return root
}

// buildButtons creates the right-aligned Accept | Cancel row.
func (s *FilePickerScreen) buildButtons() *widget.Container {
	s.acceptBtn = widgets.PrimaryTextButton("Accept", style.ButtonPaddingSmall, func(args *widget.ButtonClickedEventArgs) {
		s.accept()
	})
	s.cancelBtn = widgets.TextButton("Cancel", style.ButtonPaddingSmall, func(args *widget.ButtonClickedEventArgs) {
		s.Cancel()
	})
	return widgets.ActionRow(s.acceptBtn, s.cancelBtn)
}

// refreshPlaces rebuilds the sidebar buttons in place. Volumes are separated
// from the standard folders by a divider.
func (s *FilePickerScreen) refreshPlaces() {
	if s.sidebar == nil {
		return
	}
	focusKey := s.focusedPlaceKey()
	s.sidebar.Clear()
	s.placeButtons = s.placeButtons[:0]
	s.placeKeys = s.placeKeys[:0]

	prevKind := filebrowser.PlaceFolder
	for i, p := range s.places {
		if i > 0 && p.Kind != prevKind {
			s.sidebar.AddDivider()
		}
		prevKind = p.Kind
		path := p.Path
		btn := s.sidebar.AddButton(p.Label, false, func() {
			s.enterDir(path)
		})
		s.placeButtons = append(s.placeButtons, btn)
		s.placeKeys = append(s.placeKeys, placeKey(path))
	}

	s.registerNavigation()
	if focusKey != "" {
		if slices.Contains(s.placeKeys, focusKey) {
			s.SetPendingFocus(focusKey)
		} else if len(s.placeKeys) > 0 {
			s.SetPendingFocus(s.placeKeys[0])
		}
	}
}

// placeKey returns the focus key for a sidebar place.
func placeKey(path string) string {
	return "picker-place:" + path
}

// refreshList rebuilds the list rows in place, updates Accept and re-registers
// navigation. resetScroll returns the list to the top. When a row had focus,
// focus moves to focusKey if it is still listed, else the first row.
func (s *FilePickerScreen) refreshList(resetScroll bool, focusKey string) {
	if s.listContent == nil {
		return
	}
	hadFocus := false
	for _, btn := range s.listButtons {
		if btn.IsFocused() {
			hadFocus = true
			break
		}
	}
	s.listContent.RemoveChildren()
	s.listKeys = s.listKeys[:0]
	s.listButtons = make(map[string]*widget.Button)

	if s.dirError != "" {
		s.listContent.AddChild(widgets.EmptyState(s.dirError, "", nil))
	} else {
		row := 0
		if parent, ok := filebrowser.Parent(s.dir); ok {
			s.addRow(pickerUpKey, "Up one level", row, false, func() { s.enterDir(parent) })
			row++
		}
		shown := filebrowser.Filter(s.entries, s.filter)
		for _, e := range shown {
			path := e.Path
			label := e.Name
			if e.IsDir {
				label += string(filepath.Separator)
				s.addRow(entryKey(path), label, row, false, func() { s.enterDir(path) })
			} else {
				s.addRow(entryKey(path), label, row, path == s.selected, func() { s.selectFile(path) })
			}
			row++
		}
		if len(shown) == 0 {
			msg := "No folders"
			if s.req.Mode == filebrowser.ModeFile {
				msg = "No items"
			}
			if s.filter != "" {
				msg = "No matches"
			}
			s.listContent.AddChild(widgets.EmptyState(msg, "", nil))
		}
	}

	if s.acceptBtn != nil {
		s.acceptBtn.GetWidget().Disabled = !s.canAccept()
	}
	s.registerNavigation()
	if resetScroll && s.scrollContainer != nil {
		s.scrollContainer.SetScrollTop(0)
	}
	if hadFocus {
		if _, ok := s.listButtons[focusKey]; ok {
			s.SetPendingFocus(focusKey)
		} else {
			s.SetPendingFocus(s.firstListKey())
		}
	}
}

// addRow appends a list row: a full-width button with the label drawn over
// it. Selected rows use the primary color; others alternate background colors.
func (s *FilePickerScreen) addRow(key, label string, index int, selected bool, onClick func()) {
	rowBg := widgets.AlternatingRowColor(index)
	if selected {
		rowBg = style.Primary
	}

	display, truncated := style.TruncateEnd(label, maxEntryNameChars)
	labelOpts := []widget.WidgetOpt{
		widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
		}),
	}
	if truncated {
		labelOpts = append(labelOpts, widget.WidgetOpts.ToolTip(
			widget.NewToolTip(widget.ToolTipOpts.Content(widgets.TooltipContent(label))),
		))
	}

	rowContent := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout(
			widget.AnchorLayoutOpts.Padding(&widget.Insets{Left: style.ButtonPaddingMedium, Right: style.ButtonPaddingMedium}),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, style.SettingsRowHeight),
		),
	)
	rowContent.AddChild(widget.NewText(
		widget.TextOpts.Text(display, style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(labelOpts...),
	))

	btn := widget.NewButton(
		widget.ButtonOpts.Image(&widget.ButtonImage{
			Idle:    image.NewNineSliceColor(rowBg),
			Hover:   image.NewNineSliceColor(style.PrimaryHover),
			Pressed: image.NewNineSliceColor(style.Primary),
		}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			widget.WidgetOpts.MinSize(0, style.SettingsRowHeight),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			onClick()
		}),
	)

	wrapper := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewStackedLayout()),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			widget.WidgetOpts.MinSize(0, style.SettingsRowHeight),
		),
	)
	wrapper.AddChild(btn)
	wrapper.AddChild(rowContent)
	s.listContent.AddChild(wrapper)

	s.listKeys = append(s.listKeys, key)
	s.listButtons[key] = btn
}

// focusedPlaceKey returns the key of the focused sidebar button, or "".
func (s *FilePickerScreen) focusedPlaceKey() string {
	for i, btn := range s.placeButtons {
		if btn.IsFocused() {
			return s.placeKeys[i]
		}
	}
	return ""
}

// registerNavigation re-registers every focus button and the three navigation
// zones. Called after any in-place change to the sidebar or list.
func (s *FilePickerScreen) registerNavigation() {
	s.ClearFocusButtons()
	for i, btn := range s.placeButtons {
		s.RegisterFocusButton(s.placeKeys[i], btn)
	}
	for _, k := range s.listKeys {
		s.RegisterFocusButton(k, s.listButtons[k])
	}
	buttonKeys := []string{}
	// A disabled Accept is left out: ebitenui moves focus off a disabled
	// widget using its own order, which cannot see the list.
	if s.acceptBtn != nil && !s.acceptBtn.GetWidget().Disabled {
		s.RegisterFocusButton(pickerAcceptKey, s.acceptBtn)
		buttonKeys = append(buttonKeys, pickerAcceptKey)
	}
	if s.cancelBtn != nil {
		s.RegisterFocusButton(pickerCancelKey, s.cancelBtn)
		buttonKeys = append(buttonKeys, pickerCancelKey)
	}

	s.RegisterNavZone("picker-places", types.NavZoneVertical, slices.Clone(s.placeKeys), 0)
	s.RegisterNavZone("picker-list", types.NavZoneVertical, slices.Clone(s.listKeys), 0)
	s.RegisterNavZone("picker-buttons", types.NavZoneHorizontal, buttonKeys, 0)

	s.SetNavTransition("picker-places", types.DirRight, "picker-list", types.NavIndexFirst)
	s.SetNavTransition("picker-list", types.DirLeft, "picker-places", types.NavIndexFirst)
	s.SetNavTransition("picker-list", types.DirRight, "picker-buttons", types.NavIndexFirst)
	s.SetNavTransition("picker-buttons", types.DirLeft, "picker-list", types.NavIndexFirst)
	s.SetNavTransition("picker-buttons", types.DirUp, "picker-list", types.NavIndexFirst)
}

// NavigatePath handles navigation into and out of the path field, which is
// not a button and so is outside the zone navigation. Down from the field
// moves to the first list row; other directions stay in the field so left and
// right move the caret. Up from the first list row moves to the field.
// Returns the widget to focus (nil to stay) and whether it handled the move.
func (s *FilePickerScreen) NavigatePath(focused widget.Focuser, direction int) (widget.Focuser, bool) {
	if s.pathInput == nil || focused == nil {
		return nil, false
	}
	if focused == widget.Focuser(s.pathInput) {
		if direction == types.DirDown {
			if btn := s.target(s.firstListKey()); btn != nil {
				return btn, true
			}
		}
		return nil, true
	}
	if direction == types.DirUp && len(s.listKeys) > 0 && focused == widget.Focuser(s.listButtons[s.listKeys[0]]) {
		return s.pathInput, true
	}
	return nil, false
}

// Update runs each frame while the picker is open: Ctrl/Cmd+L focuses the path
// field, clipboard shortcuts apply to it, and the sidebar places are re-polled.
func (s *FilePickerScreen) Update() {
	if s.textInputs != nil {
		s.textInputs.Update()
	}
	if widgets.ShortcutModifierPressed() && inpututil.IsKeyJustPressed(ebiten.KeyL) {
		s.pathFocusRequested = true
		s.selectAllOnFocus = true
	}
	if time.Since(s.lastPoll) >= placesPollInterval {
		s.lastPoll = time.Now()
		if p := filebrowser.Places(); !slices.Equal(p, s.places) {
			s.places = p
			s.refreshPlaces()
		}
	}
}

// SaveFocusState records focus before a rebuild. The path field is not a
// button, so its focus is kept as a request the App applies after the rebuild.
func (s *FilePickerScreen) SaveFocusState(focused widget.Focuser) {
	if s.pathInput != nil && s.pathInput.IsFocused() {
		s.pathFocusRequested = true
		return
	}
	s.BaseScreen.SaveFocusState(focused)
}

// TakePathFocusRequest returns the path field when it should receive focus,
// clearing the request. Text is selected first when the request came from the
// focus shortcut so typing replaces it.
func (s *FilePickerScreen) TakePathFocusRequest() (*widget.TextInput, bool) {
	if !s.pathFocusRequested || s.pathInput == nil {
		return nil, false
	}
	s.pathFocusRequested = false
	if s.selectAllOnFocus {
		s.selectAllOnFocus = false
		s.pathInput.SelectAll()
	}
	return s.pathInput, true
}

// EnsureFocusedVisible scrolls the list to keep a focused row visible. Sidebar
// and button focus does not scroll the list.
func (s *FilePickerScreen) EnsureFocusedVisible(focused widget.Focuser) {
	s.BaseScreen.EnsureFocusedVisible(focused, func(btn *widget.Button) bool {
		for _, b := range s.listButtons {
			if b == btn {
				return true
			}
		}
		return false
	})
}
