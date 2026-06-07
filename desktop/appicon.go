package desktop

import (
	"bytes"
	"image"
	_ "image/png"

	"github.com/hajimehoshi/ebiten/v2"
)

// appIconPNG holds the PNG-encoded application icon supplied by the core via
// SetAppIcon. It is shown in the settings About section and used for the OS
// window icon. It is nil until a core sets it.
var appIconPNG []byte

// SetAppIcon sets the PNG-encoded application icon. A core calls this before
// Run (or RunDirect) with its embedded icon. If never called the About section
// shows only the name and version and the default window icon is used.
func SetAppIcon(png []byte) {
	appIconPNG = png
}

// applyWindowIcon sets the OS window icon from the configured app icon when one
// is present. ebiten.SetWindowIcon is a no-op on macOS (its windows have no
// icon) and on non-desktop targets, so the unconditional call only takes effect
// where a window icon applies. Missing or undecodable bytes are ignored.
func applyWindowIcon() {
	if len(appIconPNG) == 0 {
		return
	}
	img, _, err := image.Decode(bytes.NewReader(appIconPNG))
	if err != nil {
		return
	}
	ebiten.SetWindowIcon([]image.Image{img})
}
