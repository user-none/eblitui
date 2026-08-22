// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"fmt"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// buildNewProfileView creates the new-profile view: connected controller
// selection plus name entry.
func (s *InputSection) buildNewProfileView(focus types.FocusManager, section *widget.Container) {
	section.AddChild(sectionHeader("New Profile"))

	models := connectedModels()
	if len(models) == 0 {
		section.AddChild(noticeText("No controllers connected. Connect one to create a profile."))
	} else {
		section.AddChild(noticeText("Controller (connected):"))
		modelKeys := make([]string, 0, len(models))
		for i, m := range models {
			section.AddChild(s.buildModelRow(focus, i, m))
			modelKeys = append(modelKeys, fmt.Sprintf("input-model-%d", i))
		}
		focus.RegisterNavZone("input-models", types.NavZoneVertical, modelKeys, 0)

		section.AddChild(s.buildNameRow(""))
	}

	if s.nameError != "" {
		section.AddChild(widget.NewText(
			widget.TextOpts.Text(s.nameError, style.FontFace(), style.Accent),
		))
	}

	var actions []actionButton
	if len(models) > 0 {
		actions = append(actions, actionButton{"Create", "input-newp-create", func() {
			s.createProfile()
		}})
	}
	actions = append(actions, actionButton{"Cancel", "input-newp-cancel", func() {
		s.switchView(inputViewMain, "input-new-profile")
	}})
	section.AddChild(s.buildActionRow(focus, actions...))

	actionKeys := make([]string, 0, len(actions))
	for _, a := range actions {
		actionKeys = append(actionKeys, a.focusKey)
	}
	focus.RegisterNavZone("input-newp-actions", types.NavZoneHorizontal, actionKeys, 0)
	if len(models) > 0 {
		chainZones(focus, []string{"input-models", "input-newp-actions"})
	}
}

// buildModelRow creates a selectable connected-controller row.
func (s *InputSection) buildModelRow(focus types.FocusManager, index int, m padModel) *widget.Container {
	row := widgets.SettingsRow(2)

	selected := s.newSDLID == m.sdlID && s.newController == m.name
	focusKey := fmt.Sprintf("input-model-%d", index)

	btn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(selected)),
		widget.ButtonOpts.Text(m.name, style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			s.newSDLID = m.sdlID
			s.newController = m.name
			s.nameError = ""
			s.focus.SetPendingFocus(focusKey)
			s.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton(focusKey, btn)
	row.AddChild(btn)

	row.AddChild(widget.NewText(
		widget.TextOpts.Text(fmt.Sprintf("(%d connected)", m.count), style.FontFace(), style.TextSecondary),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	))

	return row
}

// createProfile validates the new-profile form and appends the profile.
func (s *InputSection) createProfile() {
	if s.newSDLID == "" && s.newController == "" {
		s.nameError = "Select a controller"
		s.callback.RequestRebuild()
		return
	}

	name := strings.TrimSpace(s.nameInput.GetText())
	if msg := s.validateProfileName(name, s.newSDLID, s.newController, ""); msg != "" {
		s.nameError = msg
		s.callback.RequestRebuild()
		return
	}

	id := s.config.Input.NewProfileID()
	s.config.Input.Profiles = append(s.config.Input.Profiles, storage.ControllerProfile{
		ID:          id,
		SDLID:       s.newSDLID,
		Controller:  s.newController,
		Name:        name,
		RumbleScale: storage.DefaultRumbleScale,
	})
	storage.SaveConfig(s.config)
	s.switchView(inputViewMain, "input-profile-"+id+"-edit")
}
