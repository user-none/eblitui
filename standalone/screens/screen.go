//go:build !libretro

package screens

import (
	"github.com/user-none/eblitui/standalone/types"
)

// Re-export interfaces from types package for backward compatibility
type (
	ScreenCallback = types.ScreenCallback
	FocusRestorer  = types.FocusRestorer
	FocusManager   = types.FocusManager
)
