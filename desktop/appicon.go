// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"bytes"
	"image"

	_ "image/jpeg"
	_ "image/png"

	"github.com/hajimehoshi/ebiten/v2"
	_ "golang.org/x/image/webp"
)

// appIcon holds the png/jpg/webp encoded application icon supplied by the core via
// SetAppIcon. It is shown in the settings About section and used for the OS
// window icon. It is nil until a core sets it.
var appIcon []byte

// SetAppIcon sets the PNG-encoded application icon. A core calls this before
// Run (or RunDirect) with its embedded icon. If never called the About section
// shows only the name and version and the default window icon is used.
func SetAppIcon(data []byte) {
	appIcon = data
}

// applyWindowIcon sets the OS window icon from the configured app icon when one
// is present. ebiten.SetWindowIcon is a no-op on macOS (its windows have no
// icon) and on non-desktop targets, so the unconditional call only takes effect
// where a window icon applies. Missing or undecodable bytes are ignored.
func applyWindowIcon() {
	if len(appIcon) == 0 {
		return
	}
	img, _, err := image.Decode(bytes.NewReader(appIcon))
	if err != nil {
		return
	}
	ebiten.SetWindowIcon([]image.Image{img})
}
