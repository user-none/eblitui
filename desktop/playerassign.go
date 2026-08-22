// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/storage"
)

// PadInfo identifies a connected gamepad for player assignment.
type PadInfo struct {
	ID    ebiten.GamepadID
	SDLID string
	Name  string
}

// AssignEvent reports a change to a player's controller binding.
type AssignEvent struct {
	Player     int    // player slot index (0-based)
	Controller string // controller display name
	Bound      bool   // true = controller bound, false = unbound
}

// PlayerAssignment tracks which connected gamepad is bound to each player
// slot. Pads are matched to players by the controller model key
// (SDL ID + name) of the player's assigned profile. Identity is model-level
// only, so with two identical controllers connection order decides which
// unit fills which slot.
type PlayerAssignment struct {
	players int
	bound   [maxPlayers]ebiten.GamepadID
	has     [maxPlayers]bool
}

// NewPlayerAssignment creates an assignment tracker for the given number of
// player slots, clamped to [1, maxPlayers].
func NewPlayerAssignment(players int) *PlayerAssignment {
	if players < 1 {
		players = 1
	}
	if players > maxPlayers {
		players = maxPlayers
	}
	return &PlayerAssignment{players: players}
}

// PadFor returns the gamepad bound to the player slot, if any.
func (pa *PlayerAssignment) PadFor(player int) (ebiten.GamepadID, bool) {
	if player < 0 || player >= pa.players || !pa.has[player] {
		return 0, false
	}
	return pa.bound[player], true
}

// Update reconciles player bindings with the currently connected pads and
// the player profile assignments in input. pads must be in ebiten gamepad ID
// order so startup binding follows enumeration order and later binds follow
// connection order. Returns bind/unbind events for notifications.
func (pa *PlayerAssignment) Update(pads []PadInfo, input *storage.InputConfig) []AssignEvent {
	var events []AssignEvent

	// Unbind players whose pad is gone or whose assigned profile no longer
	// exists or no longer matches the pad's model.
	for p := 0; p < pa.players; p++ {
		if !pa.has[p] {
			continue
		}
		prof := input.PlayerProfile(p)
		pad, connected := findPad(pads, pa.bound[p])
		if connected && prof != nil && prof.MatchesPad(pad.SDLID, pad.Name) {
			continue
		}
		pa.has[p] = false
		name := pad.Name
		if name == "" && prof != nil {
			name = prof.Controller
		}
		events = append(events, AssignEvent{Player: p, Controller: name, Bound: false})
	}

	// Bind each unbound pad to the lowest unbound player whose assigned
	// profile matches the pad's model.
	for _, pad := range pads {
		if pa.padBound(pad.ID) {
			continue
		}
		for p := 0; p < pa.players; p++ {
			if pa.has[p] {
				continue
			}
			prof := input.PlayerProfile(p)
			if prof == nil || !prof.MatchesPad(pad.SDLID, pad.Name) {
				continue
			}
			pa.bound[p] = pad.ID
			pa.has[p] = true
			events = append(events, AssignEvent{Player: p, Controller: pad.Name, Bound: true})
			break
		}
	}

	return events
}

// padBound reports whether the pad is currently bound to any player.
func (pa *PlayerAssignment) padBound(id ebiten.GamepadID) bool {
	for p := 0; p < pa.players; p++ {
		if pa.has[p] && pa.bound[p] == id {
			return true
		}
	}
	return false
}

// findPad returns the pad with the given ID from the snapshot.
func findPad(pads []PadInfo, id ebiten.GamepadID) (PadInfo, bool) {
	for _, pad := range pads {
		if pad.ID == id {
			return pad, true
		}
	}
	return PadInfo{}, false
}

// SnapshotPads returns the currently connected gamepads in ID order.
func SnapshotPads() []PadInfo {
	ids := ebiten.AppendGamepadIDs(nil)
	pads := make([]PadInfo, 0, len(ids))
	for _, id := range ids {
		pads = append(pads, PadInfo{
			ID:    id,
			SDLID: ebiten.GamepadSDLID(id),
			Name:  ebiten.GamepadName(id),
		})
	}
	return pads
}

// AutoCreateFirstProfile creates a profile for the first connected pad and
// assigns it to player 1. This happens exactly once: only when no controller
// profiles of any kind exist and no player has an assignment. All other
// profile creation is explicit through settings. Returns true if the config
// was modified (caller must save).
func AutoCreateFirstProfile(input *storage.InputConfig, pads []PadInfo) bool {
	if len(input.Profiles) != 0 || len(pads) == 0 {
		return false
	}
	for _, pl := range input.Players {
		if pl.Profile != "" {
			return false
		}
	}

	pad := pads[0]
	id := input.NewProfileID()
	input.Profiles = append(input.Profiles, storage.ControllerProfile{
		ID:          id,
		SDLID:       pad.SDLID,
		Controller:  pad.Name,
		Name:        "default",
		RumbleScale: storage.DefaultRumbleScale,
	})
	if len(input.Players) == 0 {
		input.Players = append(input.Players, storage.PlayerConfig{})
	}
	input.Players[0].Profile = id
	return true
}
