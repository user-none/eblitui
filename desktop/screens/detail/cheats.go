// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package detail

import (
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/cheats"
	"github.com/user-none/eblitui/desktop/gamecheats"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// Navigation zones of the Cheats page
const (
	customZone      = "cheats-custom"      // custom cheat rows: Edit, Delete, toggle
	addZone         = "cheats-add"         // the Add Cheat button
	cheatsZone      = "cheats"             // database cheat toggles
	formFieldsZone  = "cheat-form-fields"  // form Name and Code fields
	formButtonsZone = "cheat-form-buttons" // form Save and Cancel
	addKey          = "cheat-add"          // focus key of the Add Cheat button
	formNameKey     = "cheat-form-name"    // focus key of the Name field
	formCodeKey     = "cheat-form-code"    // focus key of the Code field
	formCancelKey   = "cheat-form-cancel"  // focus key of Cancel
	formSaveKey     = "cheat-form-save"    // focus key of Save
	codeFieldRows   = 12                   // visible lines of the Code field
	codeHint        = "Separate codes with spaces, + or new lines"
)

// cheatForm is the open Add or Edit Cheat form. Its fields are kept
// across rebuilds so typed text survives them.
type cheatForm struct {
	edit int // index of the custom cheat being edited, -1 when adding
	name *widget.TextInput
	code *widgets.MultiLineInput
	err  string // why the last Save failed
}

// CheatsSection is the detail Cheats page: the custom cheats with Edit,
// Delete, and an On/Off toggle each, an Add Cheat button, and the
// database cheats with an On/Off toggle each. Add and Edit replace the
// page with a form. The game's cheats and settings are read by SetGame
// and kept until the next SetGame.
type CheatsSection struct {
	callback     types.ScreenCallback
	system       cheats.System
	bigEndian    bool // byte order of the core's memory
	contentWidth func() int
	game         *storage.GameEntry

	set          gamecheats.Set
	gameSettings *storage.GameSettingsFile // which cheats are on, and the custom cheats
	form         *cheatForm
	textInputs   *widgets.TextInputGroup
	zones        []string           // navigation zones from the last Build, in order
	focus        types.FocusManager // from the last Build; set before any row or form handler can run

	// save writes a game's settings file: storage.SaveGameSettings
	save func(gameCRC string, settings *storage.GameSettingsFile) error
}

// NewCheatsSection creates the Cheats page. system is the core's cheat
// system and bigEndian the byte order of the core's memory, which cheats
// are converted to. contentWidth returns the width of the page content
// area.
func NewCheatsSection(callback types.ScreenCallback, system cheats.System, bigEndian bool, contentWidth func() int) *CheatsSection {
	return &CheatsSection{
		callback:     callback,
		system:       system,
		bigEndian:    bigEndian,
		contentWidth: contentWidth,
		textInputs:   widgets.NewTextInputGroup(),
		save:         storage.SaveGameSettings,
	}
}

// SetGame reads the game's settings file and cheats and closes any open
// form. A nil game clears the page.
func (c *CheatsSection) SetGame(game *storage.GameEntry) {
	c.game = game
	c.set = gamecheats.Set{}
	c.gameSettings = nil
	c.form = nil
	if game == nil {
		return
	}

	settings, err := storage.LoadGameSettings(game.CRC32)
	if err != nil {
		log.Printf("Failed to load game settings for %s: %v", game.CRC32, err)
		settings = &storage.GameSettingsFile{}
	}
	c.gameSettings = settings
	c.set = gamecheats.Load(game.CRC32, settings, c.system, c.bigEndian)
}

// EnabledCount returns how many cheats are on. Enabled IDs with no
// listed cheat are not counted.
func (c *CheatsSection) EnabledCount() int {
	if c.gameSettings == nil {
		return 0
	}
	return len(c.set.Enabled(c.gameSettings))
}

// NavZones returns the page's navigation zones from the last Build, in
// order from the top
func (c *CheatsSection) NavZones() []string {
	return c.zones
}

// Update handles the form's clipboard shortcuts
func (c *CheatsSection) Update() {
	if c.form != nil {
		c.textInputs.Update()
	}
}

// HandleBack closes an open form, reporting whether there was one
func (c *CheatsSection) HandleBack() bool {
	if c.form == nil {
		return false
	}
	c.closeForm()
	c.callback.RequestRebuild()
	return true
}

// Build creates the page: the form when one is open, otherwise the
// cheat lists
func (c *CheatsSection) Build(focus types.FocusManager) *widget.Container {
	c.focus = focus
	c.zones = c.zones[:0]
	page := widgets.FillContainer()

	content := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
	)
	if c.form != nil {
		c.buildForm(focus, content)
	} else {
		c.buildLists(focus, content)
	}

	scrollContainer, _, scrollWrapper := widgets.ScrollableContainer(widgets.PageScrollOpts(content))
	focus.SetScrollContainer(scrollContainer)
	focus.RestoreScrollPosition()
	page.AddChild(scrollWrapper)
	return page
}

// buildLists adds the custom cheats, the Add Cheat button, and the
// database cheats to content, and registers their zones
func (c *CheatsSection) buildLists(focus types.FocusManager, content *widget.Container) {
	rowWidth := widgets.PageRowWidth(c.contentWidth())

	content.AddChild(widgets.SectionHeader("Custom Cheats"))
	customWidth := widgets.SettingsRowLabelWidth(rowWidth, widgets.OnOffButtonWidth(),
		widgets.ButtonWidth("Edit"), widgets.ButtonWidth("Delete"))
	var customKeys []string
	for i, ch := range c.set.Custom {
		content.AddChild(c.buildCustomRow(focus, i, ch, customWidth))
		customKeys = append(customKeys, customKey(i, "edit"), customKey(i, "delete"), customKey(i, "toggle"))
	}
	content.AddChild(c.buildAddRow(focus))

	content.AddChild(widgets.SectionHeader("Database Cheats"))
	var dbKeys []string
	switch {
	case c.set.LoadErr != nil:
		content.AddChild(widgets.NoticeText("Unable to read the cheat file"))
	case len(c.set.Database) == 0:
		content.AddChild(widgets.NoticeText("No cheats available for this game"))
	default:
		dbWidth := widgets.SettingsRowLabelWidth(rowWidth, widgets.OnOffButtonWidth())
		for i, ch := range c.set.Database {
			key := fmt.Sprintf("cheat-%d", i)
			content.AddChild(c.buildCheatRow(focus, key, ch.Name, ch.Code, ch.ID, true, dbWidth))
			dbKeys = append(dbKeys, key)
		}
	}

	if len(customKeys) > 0 {
		focus.RegisterNavZone(customZone, types.NavZoneGrid, customKeys, 3)
		c.zones = append(c.zones, customZone)
	}
	focus.RegisterNavZone(addZone, types.NavZoneVertical, []string{addKey}, 0)
	c.zones = append(c.zones, addZone)
	if len(dbKeys) > 0 {
		focus.RegisterNavZone(cheatsZone, types.NavZoneVertical, dbKeys, 0)
		c.zones = append(c.zones, cheatsZone)
	}
	types.ChainZones(focus, c.zones)
}

// customKey returns the focus key of a custom cheat row's control
func customKey(i int, control string) string {
	return fmt.Sprintf("custom-%d-%s", i, control)
}

// buildCustomRow creates a custom cheat row: its name and code, Edit,
// Delete, and an On/Off toggle. A custom cheat whose code no longer parses
// shows why in place of its code, and its toggle stays off.
func (c *CheatsSection) buildCustomRow(focus types.FocusManager, i int, ch gamecheats.Custom, labelWidth int) *widget.Container {
	edit := widgets.TextButton("Edit", style.ButtonPaddingSmall, func(*widget.ButtonClickedEventArgs) {
		c.openForm(i)
	})
	del := widgets.TextButton("Delete", style.ButtonPaddingSmall, func(*widget.ButtonClickedEventArgs) {
		c.deleteCustom(i)
	})
	focus.RegisterFocusButton(customKey(i, "edit"), edit)
	focus.RegisterFocusButton(customKey(i, "delete"), del)

	code := strings.Join(strings.Fields(ch.Code), " ")
	if ch.Err != nil {
		code = "Invalid code: " + ch.Err.Error()
	}
	return c.buildCheatRow(focus, customKey(i, "toggle"), ch.Name, code, ch.Cheat.ID, ch.Err == nil, labelWidth, edit, del)
}

// buildCheatRow creates a cheat row: the name with subtext, each
// truncated to labelWidth, any extra buttons, and an On/Off toggle
// registered under key. Toggling saves the game's settings file; a toggle
// that cannot be turned on does nothing.
func (c *CheatsSection) buildCheatRow(focus types.FocusManager, key, name, subtext, id string, canEnable bool, labelWidth int, buttons ...*widget.Button) *widget.Container {
	if name == "" {
		name = "Unnamed cheat"
	}
	face := *style.FontFace()
	displayName, _ := style.TruncateToWidth(name, face, float64(labelWidth))
	if subtext != "" {
		subtext, _ = style.TruncateToWidth(subtext, face, float64(labelWidth))
	}

	row := widgets.SettingsRow(2 + len(buttons))
	row.AddChild(widgets.LabeledText(displayName, subtext))

	on := canEnable && c.gameSettings.CheatEnabled(id)
	toggle := widgets.OnOffButton(on, func(*widget.ButtonClickedEventArgs) {
		if !canEnable {
			return
		}
		c.gameSettings.SetCheatEnabled(id, !on)
		c.saveSettings()
		focus.SetPendingFocus(key)
		c.callback.RequestRebuild()
	})
	focus.RegisterFocusButton(key, toggle)
	for _, b := range buttons {
		b.GetWidget().LayoutData = widget.GridLayoutData{VerticalPosition: widget.GridLayoutPositionCenter}
		row.AddChild(b)
	}
	// The toggle is last so it lines up with the toggles of rows without
	// extra buttons
	row.AddChild(toggle)
	return row
}

// buildAddRow creates the row holding the Add Cheat button at the right
func (c *CheatsSection) buildAddRow(focus types.FocusManager) *widget.Container {
	row := widgets.SettingsRow(2)
	row.AddChild(widget.NewContainer())
	add := widgets.TextButton("Add Cheat...", style.ButtonPaddingSmall, func(*widget.ButtonClickedEventArgs) {
		c.openForm(-1)
	})
	add.GetWidget().LayoutData = widget.GridLayoutData{VerticalPosition: widget.GridLayoutPositionCenter}
	focus.RegisterFocusButton(addKey, add)
	row.AddChild(add)
	return row
}

// buildForm adds the Add or Edit Cheat form to content and registers its
// zones
func (c *CheatsSection) buildForm(focus types.FocusManager, content *widget.Container) {
	f := c.form
	title := "Add Cheat"
	if f.edit >= 0 {
		title = "Edit Cheat"
	}
	content.AddChild(widgets.SectionHeader(title))

	stretch := widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})
	f.name.GetWidget().LayoutData = widget.RowLayoutData{Stretch: true}
	f.code.GetWidget().LayoutData = widget.RowLayoutData{Stretch: true}

	content.AddChild(fieldLabel("Name", stretch))
	content.AddChild(f.name)
	content.AddChild(fieldLabel("Code", stretch))
	content.AddChild(f.code)
	content.AddChild(widgets.NoticeText(codeHint))
	if f.err != "" {
		content.AddChild(widget.NewText(
			widget.TextOpts.Text(f.err, style.FontFace(), style.Accent),
		))
	}

	save := widgets.PrimaryTextButton("Save", style.ButtonPaddingSmall, func(*widget.ButtonClickedEventArgs) {
		c.saveForm(focus)
	})
	cancel := widgets.TextButton("Cancel", style.ButtonPaddingSmall, func(*widget.ButtonClickedEventArgs) {
		c.closeForm()
		c.callback.RequestRebuild()
	})
	content.AddChild(widgets.ActionRow(save, cancel))

	c.textInputs = widgets.NewTextInputGroup()
	c.textInputs.Add(f.name)
	focus.RegisterFocusWidget(formNameKey, f.name)
	focus.RegisterFocusWidget(formCodeKey, f.code)
	focus.RegisterFocusButton(formSaveKey, save)
	focus.RegisterFocusButton(formCancelKey, cancel)
	focus.RegisterNavZone(formFieldsZone, types.NavZoneVertical, []string{formNameKey, formCodeKey}, 0)
	focus.RegisterNavZone(formButtonsZone, types.NavZoneHorizontal, []string{formSaveKey, formCancelKey}, 0)
	c.zones = append(c.zones, formFieldsZone, formButtonsZone)
	types.ChainZones(focus, c.zones)
}

// fieldLabel creates a form field label
func fieldLabel(text string, opts ...widget.WidgetOpt) *widget.Text {
	return widget.NewText(
		widget.TextOpts.Text(text, style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(opts...),
	)
}

// openForm opens the form to edit custom cheat i, or to add one when i is
// -1, focusing the Name field. An index past the list, from a row built
// before a delete, is ignored.
func (c *CheatsSection) openForm(i int) {
	if i >= len(c.gameSettings.Cheats.Custom) {
		return
	}
	name := widgets.StyledTextInput("Cheat name", false, 0,
		widget.TextInputOpts.Validation(validateCheatName))
	code := widgets.NewMultiLineInput(codeFieldRows, 0,
		storage.MaxCustomCheatCodeLen, storage.MaxCustomCheatCodeRows, nil)
	if i >= 0 {
		e := c.gameSettings.Cheats.Custom[i]
		name.SetText(e.Name)
		code.SetText(e.Code)
	}
	c.form = &cheatForm{edit: i, name: name, code: code}
	c.focus.SetPendingFocus(formNameKey)
	c.callback.RequestRebuild()
}

// validateCheatName keeps a cheat name on one line and within
// storage.MaxCustomCheatNameLen, trimming pasted text to fit
func validateCheatName(text string) (bool, *string) {
	clean := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, text)
	if utf8.RuneCountInString(clean) > storage.MaxCustomCheatNameLen {
		clean = string([]rune(clean)[:storage.MaxCustomCheatNameLen])
	}
	if clean != text {
		return false, &clean
	}
	return true, nil
}

// closeForm closes the form and returns focus to the control that opened
// it
func (c *CheatsSection) closeForm() {
	key := addKey
	if c.form != nil && c.form.edit >= 0 {
		key = customKey(c.form.edit, "edit")
	}
	c.form = nil
	c.focus.SetPendingFocus(key)
}

// saveForm checks the form and stores the cheat, or shows why it cannot.
// A new cheat starts off. Editing the code of a cheat that is on keeps it
// on under its new code.
func (c *CheatsSection) saveForm(focus types.FocusManager) {
	f := c.form
	name := strings.TrimSpace(f.name.GetText())
	code := strings.TrimSpace(f.code.Text())
	ch, err := cheats.ParseCode(code, c.system, c.bigEndian)

	switch {
	case name == "":
		f.err = "Enter a name"
	case code == "":
		f.err = "Enter a code"
	case err != nil:
		f.err = "Invalid code: " + err.Error()
	case f.edit < 0 && len(c.gameSettings.Cheats.Custom) >= storage.MaxCustomCheats:
		f.err = fmt.Sprintf("A game can have at most %d custom cheats", storage.MaxCustomCheats)
	default:
		if other, dup := c.set.NameOfID(ch.ID, f.edit); dup {
			f.err = fmt.Sprintf("This code is already used by %q", other)
		} else {
			f.err = ""
		}
	}
	if f.err != "" {
		focus.SetPendingFocus(formSaveKey)
		c.callback.RequestRebuild()
		return
	}

	entry := storage.CustomCheat{Name: name, Code: code}
	focusKey := ""
	if f.edit >= 0 {
		old := c.set.Custom[f.edit]
		if old.Err == nil && old.Cheat.ID != ch.ID && c.gameSettings.CheatEnabled(old.Cheat.ID) {
			c.gameSettings.SetCheatEnabled(old.Cheat.ID, false)
			c.gameSettings.SetCheatEnabled(ch.ID, true)
		}
		c.gameSettings.Cheats.Custom[f.edit] = entry
		focusKey = customKey(f.edit, "edit")
	} else {
		c.gameSettings.Cheats.Custom = append(c.gameSettings.Cheats.Custom, entry)
		focusKey = customKey(len(c.gameSettings.Cheats.Custom)-1, "toggle")
	}
	c.saveSettings()
	c.set.Custom = gamecheats.ParseCustom(c.gameSettings.Cheats.Custom, c.system, c.bigEndian)
	c.form = nil
	focus.SetPendingFocus(focusKey)
	c.callback.RequestRebuild()
}

// deleteCustom deletes custom cheat i without confirmation, turning its
// code off unless another cheat has the same code, and focuses Add Cheat.
// An index past the list, from a row built before a delete, is ignored.
func (c *CheatsSection) deleteCustom(i int) {
	if i < 0 || i >= len(c.set.Custom) {
		return
	}
	if old := c.set.Custom[i]; old.Err == nil {
		if _, shared := c.set.NameOfID(old.Cheat.ID, i); !shared {
			c.gameSettings.SetCheatEnabled(old.Cheat.ID, false)
		}
	}
	list := c.gameSettings.Cheats.Custom
	c.gameSettings.Cheats.Custom = append(list[:i:i], list[i+1:]...)
	c.saveSettings()
	c.set.Custom = gamecheats.ParseCustom(c.gameSettings.Cheats.Custom, c.system, c.bigEndian)
	c.focus.SetPendingFocus(addKey)
	c.callback.RequestRebuild()
}

// saveSettings writes the game's settings file
func (c *CheatsSection) saveSettings() {
	if err := c.save(c.game.CRC32, c.gameSettings); err != nil {
		log.Printf("Failed to save game settings for %s: %v", c.game.CRC32, err)
	}
}
