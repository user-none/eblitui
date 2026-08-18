// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/screens/settings"
)

func init() {
	settings.KeyToNameFunc = func(k ebiten.Key) (string, bool) {
		return KeyToName(k)
	}
	settings.PadToNameFunc = func(b ebiten.StandardGamepadButton) (string, bool) {
		return PadToName(b)
	}
	settings.IsReservedFunc = func(k ebiten.Key) bool {
		return IsReservedKey(k)
	}
	settings.ResolveKeyFunc = ResolveKeyDisplay
	settings.ResolvePadFunc = ResolvePadDisplay
}
