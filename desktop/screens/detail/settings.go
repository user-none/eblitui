// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package detail

import (
	"log"
	"slices"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/screens/settings"
	"github.com/user-none/eblitui/desktop/shader"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// Navigation zone, focus key, and labels of the Settings page
const (
	gameSettingsZone = "game-settings"       // every row's control, then Reset All to Global
	resetKey         = "game-settings-reset" // focus key of Reset All to Global
	inheritLabel     = "Inherit"             // the value that uses the global setting
)

// settingsPages lists the core option categories in the order of the
// settings pages they are on, with each page's name
var settingsPages = []struct {
	category coreif.CoreOptionCategory
	name     string
}{
	{coreif.CoreOptionCategoryVideo, "Video"},
	{coreif.CoreOptionCategoryAudio, "Audio"},
	{coreif.CoreOptionCategoryInput, "Input"},
	{coreif.CoreOptionCategoryCore, "Core Options"},
}

// SettingsSection is the detail Settings page: the game's overrides of
// global settings. The per game core options are listed under the name of
// the settings page each is on, and the preprocess shaders under Video,
// as on the Video settings page. Each control cycles through Inherit,
// which uses the global setting, then the setting's values. Reset All to
// Global removes every override.
type SettingsSection struct {
	callback     types.ScreenCallback
	options      []coreif.CoreOption // per game Bool and Select core options
	shaders      []shader.ShaderInfo // preprocess shaders
	contentWidth func() int
	game         *storage.GameEntry
	gameSettings *storage.GameSettingsFile // shared with the Cheats page
	zones        []string                  // navigation zones from the last Build, in order

	// save writes a game's settings file: storage.SaveGameSettings
	save func(gameCRC string, settings *storage.GameSettingsFile) error
}

// NewSettingsSection creates the Settings page for the core's options.
// contentWidth returns the width of the page content area.
func NewSettingsSection(callback types.ScreenCallback, coreOptions []coreif.CoreOption, contentWidth func() int) *SettingsSection {
	s := &SettingsSection{
		callback:     callback,
		contentWidth: contentWidth,
		save:         storage.SaveGameSettings,
	}
	for _, opt := range coreOptions {
		if opt.PerGame && (opt.Type == coreif.CoreOptionBool || opt.Type == coreif.CoreOptionSelect) {
			s.options = append(s.options, opt)
		}
	}
	for _, info := range shader.AvailableShaders {
		if info.Preprocess {
			s.shaders = append(s.shaders, info)
		}
	}
	return s
}

// SetGame sets the game and its settings file, which the page edits and
// saves. A nil game clears the page.
func (s *SettingsSection) SetGame(game *storage.GameEntry, gameSettings *storage.GameSettingsFile) {
	s.game = game
	s.gameSettings = gameSettings
}

// NavZones returns the page's navigation zones from the last Build, in
// order from the top
func (s *SettingsSection) NavZones() []string {
	return s.zones
}

// Build creates the page: each settings page's rows under its name, then
// Reset All to Global
func (s *SettingsSection) Build(focus types.FocusManager) *widget.Container {
	s.zones = s.zones[:0]
	page := widgets.FillContainer()

	content := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
	)

	rowWidth := widgets.PageRowWidth(s.contentWidth())
	var keys []string
	overridden := false
	for _, p := range settingsPages {
		opts := settings.CategoryOptions(s.options, p.category)
		hasShaders := p.category == coreif.CoreOptionCategoryVideo && len(s.shaders) > 0
		if len(opts) == 0 && !hasShaders {
			continue
		}
		content.AddChild(widgets.SectionHeader(p.name))
		group := ""
		for _, opt := range opts {
			if opt.Group != "" && opt.Group != group {
				content.AddChild(widgets.SectionHeader(opt.Group))
			}
			group = opt.Group
			key := "game-settings-opt-" + opt.Key
			row, overrides := s.buildCoreOptionRow(focus, key, opt, rowWidth)
			content.AddChild(row)
			keys = append(keys, key)
			overridden = overridden || overrides
		}
		if hasShaders {
			content.AddChild(widgets.SectionHeader("Shader Effects"))
			for _, info := range s.shaders {
				key := "game-settings-shader-" + info.ID
				row, overrides := s.buildShaderRow(focus, key, info, rowWidth)
				content.AddChild(row)
				keys = append(keys, key)
				overridden = overridden || overrides
			}
		}
	}
	content.AddChild(s.buildResetRow(focus, overridden))
	keys = append(keys, resetKey)

	focus.RegisterNavZone(gameSettingsZone, types.NavZoneVertical, keys, 0)
	s.zones = append(s.zones, gameSettingsZone)

	scrollContainer, _, scrollWrapper := widgets.ScrollableContainer(widgets.PageScrollOpts(content))
	focus.SetScrollContainer(scrollContainer)
	focus.RestoreScrollPosition()
	page.AddChild(scrollWrapper)
	return page
}

// buildCoreOptionRow creates the row of a core option, reporting whether
// the game overrides it. Its values are On and Off for a Bool option and
// the listed values for a Select option.
func (s *SettingsSection) buildCoreOptionRow(focus types.FocusManager, key string, opt coreif.CoreOption, rowWidth int) (*widget.Container, bool) {
	// values[0] is unused: index 0 is Inherit
	labels := []string{inheritLabel}
	values := []string{""}
	if opt.Type == coreif.CoreOptionBool {
		labels = append(labels, "On", "Off")
		values = append(values, "true", "false")
	} else {
		labels = append(labels, opt.Values...)
		values = append(values, opt.Values...)
	}

	current := 0
	if v, ok := s.gameSettings.Overrides.CoreOption(opt); ok {
		current = slices.Index(values, v)
	}
	row := s.buildChoiceRow(focus, key, opt.Label, opt.Description, labels, current, rowWidth, func(i int) {
		if i == 0 {
			s.gameSettings.Overrides.ClearCoreOption(opt.Key)
		} else {
			s.gameSettings.Overrides.SetCoreOption(opt.Key, values[i])
		}
	})
	return row, current != 0
}

// buildShaderRow creates the row of a shader with the values On and Off,
// reporting whether the game overrides it
func (s *SettingsSection) buildShaderRow(focus types.FocusManager, key string, info shader.ShaderInfo, rowWidth int) (*widget.Container, bool) {
	current := 0
	if on, ok := s.gameSettings.Overrides.Shader(info.ID); ok {
		current = 2
		if on {
			current = 1
		}
	}
	row := s.buildChoiceRow(focus, key, info.Name, info.Description, []string{inheritLabel, "On", "Off"}, current, rowWidth, func(i int) {
		switch i {
		case 0:
			s.gameSettings.Overrides.ClearShader(info.ID)
		case 1:
			s.gameSettings.Overrides.SetShader(info.ID, true)
		case 2:
			s.gameSettings.Overrides.SetShader(info.ID, false)
		}
	})
	return row, current != 0
}

// buildChoiceRow creates a row with the label and description, each
// truncated to fit, and a cycle button showing labels[current]. Pressing
// the button calls set with the next label's index and saves the game's
// settings file. The button is as wide as its widest label so it keeps
// its size as it cycles.
func (s *SettingsSection) buildChoiceRow(focus types.FocusManager, key, label, description string, labels []string, current, rowWidth int, set func(int)) *widget.Container {
	buttonWidth := 0
	for _, l := range labels {
		buttonWidth = max(buttonWidth, widgets.ButtonWidth(l))
	}
	labelWidth := float64(widgets.SettingsRowLabelWidth(rowWidth, buttonWidth))
	face := *style.FontFace()
	displayName, _ := style.TruncateToWidth(label, face, labelWidth)
	displayDesc, _ := style.TruncateToWidth(description, face, labelWidth)

	row := widgets.SettingsRow(2)
	row.AddChild(widgets.LabeledText(displayName, displayDesc))

	btn := widgets.CycleButton(labels[current], current, len(labels), buttonWidth, func(next int) {
		set(next)
		s.saveSettings()
		focus.SetPendingFocus(key)
		s.callback.RequestRebuild()
	})
	focus.RegisterFocusButton(key, btn)
	row.AddChild(btn)
	return row
}

// buildResetRow creates the row holding Reset All to Global at the right,
// disabled when the game overrides none of the listed settings
func (s *SettingsSection) buildResetRow(focus types.FocusManager, enabled bool) *widget.Container {
	row := widgets.SettingsRow(2)
	row.AddChild(widget.NewContainer())
	btn := widgets.EnabledTextButton("Reset All to Global", style.ButtonPaddingSmall, enabled, func(*widget.ButtonClickedEventArgs) {
		s.gameSettings.Overrides.Reset()
		s.saveSettings()
		focus.SetPendingFocus(resetKey)
		s.callback.RequestRebuild()
	})
	btn.GetWidget().LayoutData = widget.GridLayoutData{VerticalPosition: widget.GridLayoutPositionCenter}
	focus.RegisterFocusButton(resetKey, btn)
	row.AddChild(btn)
	return row
}

// saveSettings writes the game's settings file
func (s *SettingsSection) saveSettings() {
	if err := s.save(s.game.CRC32, s.gameSettings); err != nil {
		log.Printf("Failed to save game settings for %s: %v", s.game.CRC32, err)
	}
}
