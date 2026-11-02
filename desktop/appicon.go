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

// applyWindowIcon sets the OS window icon from the emulator's icon when one
// is present. ebiten.SetWindowIcon is a no-op on macOS (its windows have no
// icon) and on non-desktop targets, so the unconditional call only takes effect
// where a window icon applies. Missing or undecodable bytes are ignored.
func applyWindowIcon(icon []byte) {
	if len(icon) == 0 {
		return
	}
	img, _, err := image.Decode(bytes.NewReader(icon))
	if err != nil {
		return
	}
	ebiten.SetWindowIcon([]image.Image{img})
}
