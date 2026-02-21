//go:build !ios && !libretro

package standalone

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/standalone/screens/settings"
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
