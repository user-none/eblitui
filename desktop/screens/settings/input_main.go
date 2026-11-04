// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

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
	addCoreOptionRows(section, focus, s.callback, s.config, s.systemInfo.CoreOptions, coreif.CoreOptionCategoryInput, "input")

	// Players
	section.AddChild(widgets.SectionHeader("Players"))
	section.AddChild(s.buildKeyboardPlayerRow())
	for p := 0; p < s.playerCount(); p++ {
		section.AddChild(s.buildPlayerRow(focus, p))
	}

	// Keyboard bindings entry
	section.AddChild(s.buildKeyboardEditRow(focus))

	// Controller profiles
	section.AddChild(widgets.SectionHeader("Controller Profiles"))
	profs := s.config.Values().Input.Profiles
	if len(profs) == 0 {
		section.AddChild(widgets.NoticeText("No profiles. Connect a controller and create one."))
	}
	for i := range profs {
		section.AddChild(s.buildProfileRow(focus, &profs[i]))
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
	input := s.config.Values().Input
	assigned := input.PlayerProfile(player)
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
	input := s.config.Values().Input
	profs := input.Profiles
	if len(profs) == 0 {
		return
	}

	cur := ""
	if player < len(input.Players) {
		cur = input.Players[player].Profile
	}
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
	s.config.SetPlayerProfile(player, next)

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

	deleteKey := "input-profile-" + p.ID + "-delete"
	deleteBtn := s.confirmDelete.Button(deleteKey, style.ButtonPaddingSmall, focus, s.callback.RequestRebuild, func() {
		s.deleteProfile(profileID)
	})
	focus.RegisterFocusButton(deleteKey, deleteBtn)
	row.AddChild(deleteBtn)

	return row
}

// deleteProfile removes a profile, once its Delete is confirmed. Players
// referencing it are set to no controller.
func (s *InputSection) deleteProfile(id string) {
	s.config.DeleteProfile(id)

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

	coreOptKeys := coreOptionFocusKeys(s.systemInfo.CoreOptions, coreif.CoreOptionCategoryInput, "input")
	if len(coreOptKeys) > 0 {
		focus.RegisterNavZone("input-core-opts", types.NavZoneVertical, coreOptKeys, 0)
		zones = append(zones, "input-core-opts")
	}

	playerKeys := make([]string, 0, s.playerCount())
	for p := 0; p < s.playerCount(); p++ {
		playerKeys = append(playerKeys, fmt.Sprintf("input-player-%d", p))
	}
	focus.RegisterNavZone("input-players", types.NavZoneVertical, playerKeys, 0)
	zones = append(zones, "input-players")

	focus.RegisterNavZone("input-kb-edit", types.NavZoneVertical, []string{"input-kb-edit"}, 0)
	zones = append(zones, "input-kb-edit")

	if profs := s.config.Values().Input.Profiles; len(profs) > 0 {
		profileKeys := make([]string, 0, len(profs)*2)
		for i := range profs {
			id := profs[i].ID
			profileKeys = append(profileKeys, "input-profile-"+id+"-edit")
			profileKeys = append(profileKeys, "input-profile-"+id+"-delete")
		}
		focus.RegisterNavZone("input-profiles", types.NavZoneGrid, profileKeys, 2)
		zones = append(zones, "input-profiles")
	}

	focus.RegisterNavZone("input-new-profile", types.NavZoneVertical, []string{"input-new-profile"}, 0)
	zones = append(zones, "input-new-profile")

	types.ChainZones(focus, zones)
}
