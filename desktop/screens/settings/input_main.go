package settings

import (
	"fmt"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// buildMainView creates the top-level input settings view: general options,
// the players block, and the controller profile list.
func (s *InputSection) buildMainView(focus types.FocusManager, section *widget.Container) {
	// Core options section (options with Category == "Input")
	hasInputOptions := false
	for _, opt := range s.systemInfo.CoreOptions {
		if opt.Category == coreif.CoreOptionCategoryInput {
			hasInputOptions = true
			section.AddChild(buildCoreOptionRow(focus, s.callback, s.config, opt, "input"))
		}
	}

	if hasInputOptions {
		// Spacer between core options and the rest
		section.AddChild(noticeText(""))
	}

	// Analog stick toggle
	section.AddChild(s.buildAnalogStickRow(focus))

	// Rumble toggle
	section.AddChild(s.buildRumbleRow(focus))

	// Players
	section.AddChild(sectionHeader("Players"))
	section.AddChild(s.buildKeyboardPlayerRow())
	for p := 0; p < s.playerCount(); p++ {
		section.AddChild(s.buildPlayerRow(focus, p))
	}

	// Keyboard bindings entry
	section.AddChild(s.buildKeyboardEditRow(focus))

	// Controller profiles
	section.AddChild(sectionHeader("Controller Profiles"))
	if len(s.config.Input.Profiles) == 0 {
		section.AddChild(noticeText("No profiles. Connect a controller and create one."))
	}
	for i := range s.config.Input.Profiles {
		section.AddChild(s.buildProfileRow(focus, &s.config.Input.Profiles[i]))
	}
	section.AddChild(s.buildNewProfileRow(focus))

	s.setupMainNavigation(focus)
}

// buildKeyboardPlayerRow shows the fixed player 1 keyboard row.
func (s *InputSection) buildKeyboardPlayerRow() *widget.Container {
	row := widgets.SettingsRow(2)
	row.AddChild(widget.NewText(
		widget.TextOpts.Text("Player 1 Keyboard", style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	))
	row.AddChild(widget.NewText(
		widget.TextOpts.Text("Always On", style.FontFace(), style.TextSecondary),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	))
	return row
}

// buildPlayerRow creates the profile selector row for one player slot.
func (s *InputSection) buildPlayerRow(focus types.FocusManager, player int) *widget.Container {
	row := widgets.SettingsRow(2)

	row.AddChild(widget.NewText(
		widget.TextOpts.Text(fmt.Sprintf("Player %d Controller", player+1), style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	))

	label := "No Controller"
	assigned := s.config.Input.PlayerProfile(player)
	if assigned != nil {
		label = profileDisplay(assigned)
	}

	focusKey := fmt.Sprintf("input-player-%d", player)
	btn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(assigned != nil)),
		widget.ButtonOpts.Text(label, style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(90), 0),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			s.cyclePlayerProfile(player)
		}),
	)
	focus.RegisterFocusButton(focusKey, btn)
	row.AddChild(btn)

	return row
}

// cyclePlayerProfile advances a player's assignment through
// No Controller -> each profile -> No Controller.
func (s *InputSection) cyclePlayerProfile(player int) {
	profs := s.config.Input.Profiles
	if len(profs) == 0 {
		return
	}
	for len(s.config.Input.Players) <= player {
		s.config.Input.Players = append(s.config.Input.Players, storage.PlayerConfig{})
	}

	cur := s.config.Input.Players[player].Profile
	next := ""
	if cur == "" {
		next = profs[0].ID
	} else {
		for i := range profs {
			if profs[i].ID == cur {
				if i+1 < len(profs) {
					next = profs[i+1].ID
				}
				break
			}
		}
	}
	s.config.Input.Players[player].Profile = next

	storage.SaveConfig(s.config)
	s.focus.SetPendingFocus(fmt.Sprintf("input-player-%d", player))
	s.callback.RequestRebuild()
}

// buildKeyboardEditRow creates the entry row for the keyboard binding editor.
func (s *InputSection) buildKeyboardEditRow(focus types.FocusManager) *widget.Container {
	row := widgets.SettingsRow(2)

	row.AddChild(widget.NewText(
		widget.TextOpts.Text("Keyboard Bindings (Player 1)", style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	))

	btn := widgets.TextButton("Edit", style.ButtonPaddingSmall, func(args *widget.ButtonClickedEventArgs) {
		s.switchView(inputViewKeyboard, "input-kb-"+dpadEntries[0].Name)
	})
	focus.RegisterFocusButton("input-kb-edit", btn)
	row.AddChild(btn)

	return row
}

// buildProfileRow creates a profile list row with edit/delete actions.
func (s *InputSection) buildProfileRow(focus types.FocusManager, p *storage.ControllerProfile) *widget.Container {
	row := widgets.SettingsRow(3)

	row.AddChild(widget.NewText(
		widget.TextOpts.Text(profileDisplay(p), style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	))

	profileID := p.ID
	editBtn := widgets.TextButton("Edit", style.ButtonPaddingSmall, func(args *widget.ButtonClickedEventArgs) {
		s.profileID = profileID
		s.switchView(inputViewProfile, "input-pad-"+dpadEntries[0].Name)
	})
	focus.RegisterFocusButton("input-profile-"+p.ID+"-edit", editBtn)
	row.AddChild(editBtn)

	deleteBtn := widgets.TextButton("Delete", style.ButtonPaddingSmall, func(args *widget.ButtonClickedEventArgs) {
		s.deleteProfile(profileID)
	})
	focus.RegisterFocusButton("input-profile-"+p.ID+"-delete", deleteBtn)
	row.AddChild(deleteBtn)

	return row
}

// deleteProfile removes a profile immediately. Players referencing it are
// set to no controller.
func (s *InputSection) deleteProfile(id string) {
	profs := s.config.Input.Profiles
	for i := range profs {
		if profs[i].ID == id {
			s.config.Input.Profiles = append(profs[:i], profs[i+1:]...)
			break
		}
	}
	if len(s.config.Input.Profiles) == 0 {
		s.config.Input.Profiles = nil
	}
	for i := range s.config.Input.Players {
		if s.config.Input.Players[i].Profile == id {
			s.config.Input.Players[i].Profile = ""
		}
	}

	storage.SaveConfig(s.config)
	s.focus.SetPendingFocus("input-new-profile")
	s.callback.RequestRebuild()
}

// buildNewProfileRow creates the "New Profile..." action row.
func (s *InputSection) buildNewProfileRow(focus types.FocusManager) *widget.Container {
	row := widgets.SettingsRow(2)

	// Spacer pushes the button right
	row.AddChild(widget.NewContainer())

	btn := widgets.TextButton("New Profile...", style.ButtonPaddingSmall, func(args *widget.ButtonClickedEventArgs) {
		s.switchView(inputViewNewProfile, "input-newp-cancel")
	})
	focus.RegisterFocusButton("input-new-profile", btn)
	row.AddChild(btn)

	return row
}

// setupMainNavigation registers navigation zones for the main view
func (s *InputSection) setupMainNavigation(focus types.FocusManager) {
	var zones []string

	coreOptKeys := make([]string, 0)
	for _, opt := range s.systemInfo.CoreOptions {
		if opt.Category == coreif.CoreOptionCategoryInput {
			coreOptKeys = append(coreOptKeys, "input-opt-"+opt.Key)
		}
	}
	if len(coreOptKeys) > 0 {
		focus.RegisterNavZone("input-core-opts", types.NavZoneVertical, coreOptKeys, 0)
		zones = append(zones, "input-core-opts")
	}

	focus.RegisterNavZone("input-analog-stick", types.NavZoneVertical, []string{"input-analog-stick"}, 0)
	zones = append(zones, "input-analog-stick")

	focus.RegisterNavZone("input-rumble", types.NavZoneVertical, []string{"input-rumble"}, 0)
	zones = append(zones, "input-rumble")

	playerKeys := make([]string, 0, s.playerCount())
	for p := 0; p < s.playerCount(); p++ {
		playerKeys = append(playerKeys, fmt.Sprintf("input-player-%d", p))
	}
	focus.RegisterNavZone("input-players", types.NavZoneVertical, playerKeys, 0)
	zones = append(zones, "input-players")

	focus.RegisterNavZone("input-kb-edit", types.NavZoneVertical, []string{"input-kb-edit"}, 0)
	zones = append(zones, "input-kb-edit")

	if len(s.config.Input.Profiles) > 0 {
		profileKeys := make([]string, 0, len(s.config.Input.Profiles)*2)
		for i := range s.config.Input.Profiles {
			id := s.config.Input.Profiles[i].ID
			profileKeys = append(profileKeys, "input-profile-"+id+"-edit")
			profileKeys = append(profileKeys, "input-profile-"+id+"-delete")
		}
		focus.RegisterNavZone("input-profiles", types.NavZoneGrid, profileKeys, 2)
		zones = append(zones, "input-profiles")
	}

	focus.RegisterNavZone("input-new-profile", types.NavZoneVertical, []string{"input-new-profile"}, 0)
	zones = append(zones, "input-new-profile")

	chainZones(focus, zones)
}

// buildAnalogStickRow creates the "Disable Analog Stick" toggle row
func (s *InputSection) buildAnalogStickRow(focus types.FocusManager) *widget.Container {
	row := widgets.SettingsRow(2)

	label := widget.NewText(
		widget.TextOpts.Text("Disable Analog Stick", style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
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

// rumbleLevelLabel returns the display text for a rumble level.
func rumbleLevelLabel(level int) string {
	switch level {
	case 1:
		return "On (1x)"
	case 2:
		return "On (2x)"
	case 3:
		return "On (3x)"
	case 4:
		return "On (4x)"
	case 5:
		return "On (Max)"
	default:
		return "Off"
	}
}

// buildRumbleRow creates the "Rumble" cycle row (Off / On / On 2x / On 3x)
func (s *InputSection) buildRumbleRow(focus types.FocusManager) *widget.Container {
	row := widgets.SettingsRow(2)

	label := widget.NewText(
		widget.TextOpts.Text("Rumble", style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	)
	row.AddChild(label)

	level := s.config.Input.RumbleLevel
	toggleBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(level > 0)),
		widget.ButtonOpts.Text(rumbleLevelLabel(level), style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(50), 0),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			s.config.Input.RumbleLevel = (s.config.Input.RumbleLevel + 1) % 6
			storage.SaveConfig(s.config)
			focus.SetPendingFocus("input-rumble")
			s.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton("input-rumble", toggleBtn)
	row.AddChild(toggleBtn)

	return row
}
