// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package cheats

import (
	"fmt"
	"strings"
)

// System is a game system whose cheat code formats are known. The zero
// value is not a valid system.
type System int

const (
	SystemSaturn System = iota + 1
)

// systemNames maps lower case system names, as emulators give them in
// Emulator.CheatFormat, to systems.
var systemNames = map[string]System{
	"saturn":      SystemSaturn,
	"sega saturn": SystemSaturn,
}

// SystemFromName returns the system for a name, compared without case,
// such as "saturn" or "Sega Saturn". It
// returns an error for names of systems without cheat support.
func SystemFromName(name string) (System, error) {
	if sys, ok := systemNames[strings.ToLower(strings.TrimSpace(name))]; ok {
		return sys, nil
	}
	return 0, fmt.Errorf("no cheat support for system %q", name)
}
