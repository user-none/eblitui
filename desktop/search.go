// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"sort"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/user-none/eblitui/desktop/style"
)

// maxSearchRunes is the most characters the search text holds. Longer text
// is far past any game title, so it could not match anything.
const maxSearchRunes = 256

// Overlay text parts: searchPrefix is shown before the search text,
// searchTrimmed in place of the start of text too wide for the screen, and
// searchCursor after the text while typing.
const (
	searchPrefix  = "Filter: "
	searchTrimmed = "..."
	searchCursor  = "_"
)

// SearchOverlay displays a search filter at the bottom-left of the screen
type SearchOverlay struct {
	text      string
	active    bool              // Currently capturing keyboard input
	onChanged func(text string) // Callback when text changes
}

// NewSearchOverlay creates a new search overlay with the given change callback
func NewSearchOverlay(onChanged func(text string)) *SearchOverlay {
	return &SearchOverlay{
		onChanged: onChanged,
	}
}

// IsVisible returns true if the search has text (overlay should be shown)
func (s *SearchOverlay) IsVisible() bool {
	return s.text != ""
}

// IsActive returns true if the overlay is capturing keyboard input
func (s *SearchOverlay) IsActive() bool {
	return s.active
}

// Activate starts capturing keyboard input
func (s *SearchOverlay) Activate() {
	s.active = true
}

// Clear removes all search text and deactivates
func (s *SearchOverlay) Clear() {
	s.text = ""
	s.active = false
	if s.onChanged != nil {
		s.onChanged(s.text)
	}
}

// HandleInput processes keyboard input when active.
// Returns true if input was handled (should not propagate to navigation).
func (s *SearchOverlay) HandleInput() bool {
	if !s.active {
		return false
	}

	// Arrow keys deactivate but keep filter - let navigation proceed
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) ||
		ebiten.IsKeyPressed(ebiten.KeyArrowDown) ||
		ebiten.IsKeyPressed(ebiten.KeyArrowLeft) ||
		ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		s.active = false
		return false // Let navigation proceed
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && s.backspace() {
		return true
	}

	// Character input
	if chars := ebiten.AppendInputChars(nil); len(chars) > 0 {
		s.appendChars(chars)
	}

	return true // Active, consume input even if nothing typed
}

// appendChars adds typed characters to the search text, up to
// maxSearchRunes; characters past the limit are dropped. A '/' typed into
// empty text is the key that activated search and is not added. The change
// callback runs only when the text changed.
func (s *SearchOverlay) appendChars(chars []rune) {
	n := utf8.RuneCountInString(s.text)
	changed := false
	for _, c := range chars {
		if n >= maxSearchRunes {
			break
		}
		if c == '/' && s.text == "" {
			continue
		}
		s.text += string(c)
		n++
		changed = true
	}
	if changed && s.onChanged != nil {
		s.onChanged(s.text)
	}
}

// backspace removes the last character of the search text: one whole rune,
// so a multi-byte character never leaves a partial UTF-8 sequence behind.
// It reports whether there was a character to remove.
func (s *SearchOverlay) backspace() bool {
	if s.text == "" {
		return false
	}
	_, size := utf8.DecodeLastRuneInString(s.text)
	s.text = s.text[:len(s.text)-size]
	if s.onChanged != nil {
		s.onChanged(s.text)
	}
	return true
}

// visibleSearchText returns the overlay text for search text body: the
// prefix, the text and, when active, the cursor. When that is wider than
// maxWidth as measured by width, the start of the text is replaced with
// "..." and as few characters as needed are dropped from it, so the end
// being typed stays in view. The prefix and cursor are always kept.
func visibleSearchText(body string, active bool, maxWidth float64, width func(string) float64) string {
	cursor := ""
	if active {
		cursor = searchCursor
	}
	full := searchPrefix + body + cursor
	if width(full) <= maxWidth {
		return full
	}

	runes := []rune(body)
	trimmed := func(drop int) string {
		return searchPrefix + searchTrimmed + string(runes[drop:]) + cursor
	}
	// Narrower the more is dropped, so find the fewest drops that fit; when
	// even dropping everything is too wide, everything is dropped
	drop := sort.Search(len(runes), func(i int) bool {
		return width(trimmed(i)) <= maxWidth
	})
	return trimmed(drop)
}

// Draw renders the search overlay at bottom-left. The overlay is never
// wider than the screen less its margins.
func (s *SearchOverlay) Draw(screen *ebiten.Image) {
	if !s.IsVisible() && !s.active {
		return
	}

	bounds := screen.Bounds()
	padding := style.OverlayPadding
	margin := style.OverlayMargin
	maxTextWidth := float64(bounds.Dx() - margin*2 - padding*2)
	if maxTextWidth <= 0 {
		return
	}

	face := *style.FontFace()
	width := func(t string) float64 {
		w, _ := text.Measure(t, face, 0)
		return w
	}
	displayText := visibleSearchText(s.text, s.active, maxTextWidth, width)
	textWidth, textHeight := text.Measure(displayText, face, 0)

	// Position: bottom-left, margin (mirrors Notification at bottom-right)
	bgWidth := float32(textWidth) + float32(padding*2)
	bgHeight := float32(textHeight) + float32(padding*2)
	bgX := float32(margin)
	bgY := float32(bounds.Dy()) - bgHeight - float32(margin)

	// Drawn straight onto the screen, so no image the size of the text is
	// ever allocated
	overlayBg := style.OverlayBackground
	overlayBg.A = 153 // 60% opacity
	vector.FillRect(screen, bgX, bgY, bgWidth, bgHeight, overlayBg, false)

	textOpts := &text.DrawOptions{}
	textOpts.GeoM.Translate(float64(bgX)+float64(padding), float64(bgY)+float64(padding))
	textOpts.ColorScale.ScaleWithColor(style.Text)
	text.Draw(screen, displayText, face, textOpts)
}
