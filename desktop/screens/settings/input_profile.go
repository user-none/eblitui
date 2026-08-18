// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"fmt"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// buildProfileView creates the profile editor view: name entry plus the
// controller binding table for one profile.
func (s *InputSection) buildProfileView(focus types.FocusManager, section *widget.Container) {
	prof := s.config.Input.ProfileByID(s.profileID)
	if prof == nil {
		// Profile disappeared; fall back to the main view content.
		s.view = inputViewMain
		s.buildMainView(focus, section)
		return
	}

	section.AddChild(sectionHeader("Edit Profile: " + profileDisplay(prof)))
	section.AddChild(s.buildNameRow(prof.Name))
	if s.nameError != "" {
		section.AddChild(widget.NewText(
			widget.TextOpts.Text(s.nameError, style.FontFace(), style.Accent),
		))
	}

	matching := len(matchingPadIDs(prof))
	hasPad := matching > 0
	if hasPad {
		section.AddChild(noticeText(fmt.Sprintf("Capturing from: %s (%d connected)", prof.Controller, matching)))
	} else {
		section.AddChild(noticeText(fmt.Sprintf("No %s controller connected. Connect one to change bindings.", prof.Controller)))
	}

	section.AddChild(s.buildHeaderRow("Controller"))
	for _, dp := range dpadEntries {
		section.AddChild(s.buildPadBindingRow(focus, prof, dp.Name, dp.DefaultPad, hasPad))
	}
	for _, btn := range s.systemInfo.Buttons {
		section.AddChild(s.buildPadBindingRow(focus, prof, btn.Name, btn.DefaultPad, hasPad))
	}

	section.AddChild(s.buildActionRow(focus,
		actionButton{"Reset to Defaults", "input-reset-pad", func() {
			p := s.config.Input.ProfileByID(s.profileID)
			if p == nil {
				return
			}
			p.Bindings = nil
			storage.SaveConfig(s.config)
			s.focus.SetPendingFocus("input-reset-pad")
			s.callback.RequestRebuild()
		}},
		actionButton{"Done", "input-profile-done", func() {
			s.commitProfileName()
		}},
	))

	bindingKeys := s.bindingFocusKeys("input-pad-")
	focus.RegisterNavZone("input-pad-bindings", types.NavZoneVertical, bindingKeys, 0)
	focus.RegisterNavZone("input-profile-actions", types.NavZoneHorizontal, []string{"input-reset-pad", "input-profile-done"}, 0)
	chainZones(focus, []string{"input-pad-bindings", "input-profile-actions"})
}

// updateProfileCapture reads the next button press from a controller
// matching the edited profile and applies it to the captured button.
func (s *InputSection) updateProfileCapture() {
	prof := s.config.Input.ProfileByID(s.profileID)
	if prof == nil {
		s.capturing = false
		return
	}
	for _, id := range matchingPadIDs(prof) {
		for btn := ebiten.StandardGamepadButton(0); btn <= ebiten.StandardGamepadButtonMax; btn++ {
			if !inpututil.IsStandardGamepadButtonJustPressed(id, btn) {
				continue
			}
			if PadToNameFunc == nil {
				continue
			}
			name, ok := PadToNameFunc(btn)
			if !ok {
				continue
			}
			s.applyProfileBinding(s.captureBtn, name)
			return
		}
	}
}

// applyProfileBinding saves a controller binding on the edited profile and
// exits capture mode
func (s *InputSection) applyProfileBinding(buttonName, padName string) {
	prof := s.config.Input.ProfileByID(s.profileID)
	if prof == nil {
		s.capturing = false
		return
	}

	// Check if this is the default - if so, remove the override
	defaultPad := s.defaultPadForButton(buttonName)
	if padName == defaultPad {
		delete(prof.Bindings, buttonName)
		if len(prof.Bindings) == 0 {
			prof.Bindings = nil
		}
	} else {
		if prof.Bindings == nil {
			prof.Bindings = make(map[string]string)
		}
		prof.Bindings[buttonName] = padName
	}

	storage.SaveConfig(s.config)
	s.capturing = false
	s.focus.SetPendingFocus("input-pad-" + buttonName)
	s.callback.RequestRebuild()
}

// commitProfileName validates and saves the edited name, then returns to
// the main view. An invalid name blocks leaving the editor.
func (s *InputSection) commitProfileName() {
	prof := s.config.Input.ProfileByID(s.profileID)
	if prof == nil {
		s.switchView(inputViewMain, "input-new-profile")
		return
	}

	name := strings.TrimSpace(s.nameInput.GetText())
	if name != prof.Name {
		if msg := s.validateProfileName(name, prof.SDLID, prof.Controller, prof.ID); msg != "" {
			s.nameError = msg
			s.callback.RequestRebuild()
			return
		}
		prof.Name = name
		storage.SaveConfig(s.config)
	}
	s.switchView(inputViewMain, "input-profile-"+prof.ID+"-edit")
}

// buildPadBindingRow creates a row for a single controller binding.
// When no matching controller is connected the row is not activatable.
func (s *InputSection) buildPadBindingRow(focus types.FocusManager, prof *storage.ControllerProfile, buttonName, defaultPad string, hasPad bool) *widget.Container {
	row := widgets.SettingsRow(2)

	row.AddChild(widget.NewText(
		widget.TextOpts.Text(buttonName, style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	))

	display := resolvePadDisplay(buttonName, defaultPad, prof.Bindings)
	focusKey := "input-pad-" + buttonName

	if s.capturing && s.captureBtn == buttonName {
		display = "Press a button..."
	}

	btn := widget.NewButton(
		widget.ButtonOpts.Image(s.bindingButtonImage(buttonName, defaultPad, prof.Bindings)),
		widget.ButtonOpts.Text(display, style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(90), 0),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			if !hasPad {
				return
			}
			s.capturing = true
			s.captureBtn = buttonName
			focus.SetPendingFocus(focusKey)
			s.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton(focusKey, btn)
	row.AddChild(btn)

	return row
}
