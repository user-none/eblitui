package desktop

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/storage"
)

// Controllers unknown to the standard-layout gamepad database are given a
// generated baseline mapping so every physical input surfaces as some
// standard button. The mapping makes no attempt to be semantically correct
// (physical A may surface as standard X); the controller profile editor is
// where users bind physical buttons to emulator inputs, so it absorbs the
// semantics. Generated mappings are persisted per GUID and re-applied at
// startup so saved profiles stay consistent across sessions, even if a later
// gamepad database update adds a real entry for the device.

// baselineButtonSlots are the standard-layout slots assignable to raw
// buttons, in assignment order. The d-pad slots are reserved for the
// conventional hat mapping. guide is last so it is only spent when a pad has
// more raw buttons than the common slots cover.
var baselineButtonSlots = []string{
	"a", "b", "x", "y",
	"leftshoulder", "rightshoulder",
	"back", "start",
	"leftstick", "rightstick",
	"lefttrigger", "righttrigger",
	"guide",
}

// triggerRestMax is the resting value at or below which an axis is treated
// as a full-range trigger (rests at -1, pressed at +1) rather than a stick
// axis (rests near 0).
const triggerRestMax = -0.9

// GenerateBaselineMapping builds a standard-layout mapping line for a
// controller with the given SDL GUID, display name, raw button count, and
// per-axis resting values. Axes resting at -1 are assigned as triggers, the
// rest as stick axes in leftx/lefty/rightx/righty order. Raw buttons fill
// the remaining standard slots in order. The d-pad is mapped to hat 0 with
// the conventional direction bits, since raw hats are not observable.
// Returns "" if sdlID is empty.
func GenerateBaselineMapping(sdlID, name string, buttonCount int, restingAxes []float64) string {
	if sdlID == "" {
		return ""
	}

	var sticks, triggers []int
	for i, v := range restingAxes {
		if v <= triggerRestMax {
			triggers = append(triggers, i)
		} else {
			sticks = append(sticks, i)
		}
	}

	var elems []string
	for i, slot := range []string{"leftx", "lefty", "rightx", "righty"} {
		if i >= len(sticks) {
			break
		}
		elems = append(elems, fmt.Sprintf("%s:a%d", slot, sticks[i]))
	}

	axisSlots := make(map[string]bool)
	for i, slot := range []string{"lefttrigger", "righttrigger"} {
		if i >= len(triggers) {
			break
		}
		elems = append(elems, fmt.Sprintf("%s:a%d", slot, triggers[i]))
		axisSlots[slot] = true
	}

	button := 0
	for _, slot := range baselineButtonSlots {
		if button >= buttonCount {
			break
		}
		if axisSlots[slot] {
			continue
		}
		elems = append(elems, fmt.Sprintf("%s:b%d", slot, button))
		button++
	}

	elems = append(elems, "dpup:h0.1", "dpright:h0.2", "dpdown:h0.4", "dpleft:h0.8")

	return sdlID + "," + sanitizeMappingName(name) + "," + strings.Join(elems, ",") + ","
}

// sanitizeMappingName strips characters that would break the mapping line
// format from a controller display name.
func sanitizeMappingName(name string) string {
	name = strings.Map(func(r rune) rune {
		switch r {
		case ',', ':', '\n', '\r':
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Controller"
	}
	return name
}

// applyPadMapping applies a single mapping line to the gamepad layout
// database.
func applyPadMapping(line string) error {
	applied, err := ebiten.UpdateStandardGamepadLayoutMappings(line)
	if err != nil {
		return err
	}
	if !applied {
		return errors.New("gamepad mappings are not managed on this platform")
	}
	return nil
}

// applySavedPadMappings applies the persisted per-controller mapping lines.
// Malformed or unappliable entries are logged and skipped so one bad entry
// cannot block startup or the remaining mappings.
func applySavedPadMappings(mappings map[string]string) {
	for guid, line := range mappings {
		if !storage.ValidPadMapping(guid, line) {
			log.Printf("Skipping malformed pad mapping for %q", guid)
			continue
		}
		if err := applyPadMapping(line); err != nil {
			log.Printf("Failed to apply pad mapping for %q: %v", guid, err)
		}
	}
}

// ensurePadMappings gives every connected controller without a standard
// layout a generated baseline mapping, applied immediately and stored in the
// config. Each GUID is attempted once per session. Returns true if the
// config was modified (caller must save).
func (a *App) ensurePadMappings(pads []PadInfo) bool {
	changed := false
	for _, pad := range pads {
		if pad.SDLID == "" || a.padMappingTried[pad.SDLID] {
			continue
		}
		if ebiten.IsStandardGamepadLayoutAvailable(pad.ID) {
			continue
		}
		a.padMappingTried[pad.SDLID] = true

		// A stored mapping that left the pad non-standard means the startup
		// apply failed or was skipped; retry it rather than regenerating so
		// saved profiles keyed to it stay valid.
		if line, ok := a.config.Input.PadMappings[pad.SDLID]; ok {
			if !storage.ValidPadMapping(pad.SDLID, line) {
				log.Printf("Skipping malformed pad mapping for %q", pad.SDLID)
				continue
			}
			if err := applyPadMapping(line); err != nil {
				log.Printf("Failed to apply pad mapping for %q: %v", pad.SDLID, err)
			}
			continue
		}

		line := GenerateBaselineMapping(pad.SDLID, pad.Name, ebiten.GamepadButtonCount(pad.ID), restingAxisValues(pad.ID))
		if line == "" {
			continue
		}
		if err := applyPadMapping(line); err != nil {
			log.Printf("Failed to apply generated pad mapping for %q: %v", pad.SDLID, err)
			continue
		}
		// Persist only well-formed entries; an unusual GUID still gets an
		// in-session mapping above but is regenerated each run.
		if !storage.ValidPadMapping(pad.SDLID, line) {
			continue
		}
		if a.config.Input.PadMappings == nil {
			a.config.Input.PadMappings = make(map[string]string)
		}
		a.config.Input.PadMappings[pad.SDLID] = line
		changed = true
	}
	return changed
}

// restingAxisValues samples the current value of every axis on the pad.
// Called when an unmapped pad is first seen, so the values represent the
// controls at rest.
func restingAxisValues(id ebiten.GamepadID) []float64 {
	vals := make([]float64, ebiten.GamepadAxisCount(id))
	for i := range vals {
		vals[i] = ebiten.GamepadAxisValue(id, ebiten.GamepadAxisType(i))
	}
	return vals
}
