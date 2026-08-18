// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"github.com/user-none/eblitui/desktop/types"
)

// Re-export interfaces from types package for backward compatibility
type (
	ScreenCallback = types.ScreenCallback
	FocusRestorer  = types.FocusRestorer
	FocusManager   = types.FocusManager
)
