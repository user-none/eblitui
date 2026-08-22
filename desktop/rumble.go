// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/rumble"
)

// launchedDiscNumber returns the 1-based disc number of the disc being
// played, from the library entry where the scanner stored the
// header-reported number. Returns 0 for cartridge systems or when the
// number cannot be determined.
func (gm *GameplayManager) launchedDiscNumber() int {
	g := gm.currentGame
	if !gm.systemInfo.Disc || g == nil {
		return 0
	}
	if g.SelectedDisc < 0 || g.SelectedDisc >= len(g.Discs) {
		return 0
	}
	return g.Discs[g.SelectedDisc].Index + 1
}

// rumbleGameID returns the .erumble lookup ID for the game:
// <gameID>.disc<n> when a disc-specific file exists for the disc being
// played, else gameID unchanged.
func (gm *GameplayManager) rumbleGameID(gameID string) string {
	n := gm.launchedDiscNumber()
	if n < 1 {
		return gameID
	}
	discID := fmt.Sprintf("%s.disc%d", gameID, n)
	path, err := storage.GetGameRumblePath(discID)
	if err != nil {
		return gameID
	}
	if _, err := os.Stat(path); err != nil {
		return gameID
	}
	return discID
}

// loadRumbleRuleset loads and binds a game's .erumble file, returning
// nil when the game has none. A disc game prefers a disc-specific
// <gameID>.disc<n>.erumble file over the shared <gameID>.erumble. A file
// that fails to parse or bind is logged and treated as absent, so the
// CHT fallback can take over.
func (gm *GameplayManager) loadRumbleRuleset(gameID string, mem coreif.Memory) *rumble.Engine {
	path, err := storage.GetGameRumblePath(gm.rumbleGameID(gameID))
	if err != nil {
		return nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	rs, err := rumble.Parse(src)
	if err != nil {
		log.Printf("Rumble file %s: %v", path, err)
		return nil
	}
	sys := rumble.System{BigEndian: gm.systemInfo.BigEndianMemory}
	for _, r := range mem.Regions() {
		sys.Regions = append(sys.Regions, rumble.Region{
			Name: r.Name, Start: r.Start, Size: r.Size})
	}
	eng, err := rumble.NewEngine(rs, sys, gm.systemInfo.Players)
	if err != nil {
		log.Printf("Rumble file %s: %v", path, err)
		return nil
	}
	return eng
}

// motorRefresh is how long each per-frame motor level plays. It
// outlasts the frame gap so held levels stay seamless, and bounds the
// tail after the engine goes silent for a player.
const motorRefresh = 50 * time.Millisecond

// playerRumbleScales returns the per-player rumble intensity from each
// player slot's assigned controller profile, indexed by 0-based player.
func (gm *GameplayManager) playerRumbleScales() [maxPlayers]float64 {
	var scales [maxPlayers]float64
	for p := 0; p < maxPlayers; p++ {
		scales[p] = gm.config.Input.PlayerRumbleScale(p)
	}
	return scales
}

// FireMotorStates applies the rumble engine's per-frame motor levels
// to the gamepads, scaled by each player's profile rumble scale. Levels
// are applied as authored with no minimum floors. A player whose scale
// is 0 (or who has no profile) gets no rumble. active is the set of
// players vibrating from the previous frame; players absent from states
// are stopped. Returns the new active set. Must run on the Ebiten thread.
func FireMotorStates(states []rumble.MotorState, scales [maxPlayers]float64, active map[int]bool) map[int]bool {
	gamepadIDs := ebiten.AppendGamepadIDs(nil)
	next := make(map[int]bool)
	for _, ms := range states {
		if ms.Player < 1 || ms.Player > len(gamepadIDs) || ms.Player > maxPlayers {
			continue
		}
		scale := scales[ms.Player-1]
		if scale <= 0 {
			continue
		}
		ebiten.VibrateGamepad(gamepadIDs[ms.Player-1], &ebiten.VibrateGamepadOptions{
			Duration:        motorRefresh,
			StrongMagnitude: scaleMotor(ms.Strong, scale),
			WeakMagnitude:   scaleMotor(ms.Weak, scale),
		})
		next[ms.Player] = true
	}
	for p := range active {
		if next[p] || p < 1 || p > len(gamepadIDs) {
			continue
		}
		ebiten.VibrateGamepad(gamepadIDs[p-1], &ebiten.VibrateGamepadOptions{
			Duration: time.Millisecond,
		})
	}
	return next
}

// scaleMotor multiplies the authored motor value by the user rumble
// scale, clamped to 1.0, the maximum magnitude the gamepad API accepts.
func scaleMotor(v, scale float64) float64 {
	if v <= 0 || scale <= 0 {
		return 0
	}
	v *= scale
	if v > 1 {
		return 1
	}
	return v
}
