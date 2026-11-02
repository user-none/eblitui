// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package detail

import (
	"log"
	"time"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/artwork"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// Navigation of the Artwork page
const (
	artworkZone    = "artwork"      // the slices beside the image shown
	artworkPrevKey = "artwork-prev" // focus key of the slice before
	artworkNextKey = "artwork-next" // focus key of the slice after
)

// ArtworkSection is the detail Artwork page: the game's box art and box
// back in a carousel, one at a time with a slice of the other beside it.
// A side the game has no image for is left out. The images load in the
// background the first time the page is built for a game.
type ArtworkSection struct {
	callback types.ScreenCallback
	game     *storage.GameEntry
	images   *ImageLoader // the box art and box back, in that order

	index      int       // image shown, among the sides the game has
	from       int       // image the last move started at
	slideStart time.Time // when the last move started
	zones      []string  // navigation zones from the last Build
	focus      types.FocusManager

	// gameDir returns a game's artwork directory:
	// storage.GetGameArtworkDir
	gameDir func(gameID string) (string, error)
	now     func() time.Time
}

// NewArtworkSection creates the Artwork page. callback is asked to rebuild
// the screen when the images finish loading and when the carousel moves.
func NewArtworkSection(callback types.ScreenCallback) *ArtworkSection {
	return &ArtworkSection{
		callback: callback,
		images:   NewImageLoader(func() { callback.RequestRebuild() }),
		gameDir:  storage.GetGameArtworkDir,
		now:      time.Now,
	}
}

// SetGame shows a game's artwork, loaded on the next Build, starting at
// the first image. A nil game clears the page. Either way the previous
// textures are freed.
func (s *ArtworkSection) SetGame(game *storage.GameEntry) {
	s.images.Release()
	s.game = game
	s.index = 0
	s.from = 0
}

// Release frees the textures. The next Build loads them again.
func (s *ArtworkSection) Release() {
	s.images.Release()
}

// NavZones returns the page's navigation zones from the last Build
func (s *ArtworkSection) NavZones() []string {
	return s.zones
}

// load starts loading the game's box art and box back unless they are
// loaded or loading
func (s *ArtworkSection) load() {
	if s.game == nil {
		return
	}
	gameID := s.game.GameID
	gameDir := s.gameDir
	s.images.Start(func() []string {
		dir, err := gameDir(gameID)
		if err != nil {
			log.Printf("Artwork %s: %v", gameID, err)
			return nil
		}
		return []string{artwork.BoxArtPath(dir), artwork.BoxBackPath(dir)}
	})
}

// Build creates the page: the carousel of the sides the game has, with
// the slices beside the image shown in one navigation zone. While the
// images load, and when the game has none, a message is shown instead.
func (s *ArtworkSection) Build(focus types.FocusManager) *widget.Container {
	s.focus = focus
	s.zones = s.zones[:0]
	s.load()
	imgs, loading := s.images.Images()

	var sides []*ebiten.Image
	var labels []string
	for i, label := range []string{"Front", "Back"} {
		if i < len(imgs) && imgs[i] != nil {
			sides = append(sides, imgs[i])
			labels = append(labels, label)
		}
	}

	if len(sides) == 0 {
		msg := "No artwork"
		if loading {
			msg = "Loading..."
		}
		empty := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
		empty.AddChild(centeredText(msg))
		return empty
	}

	s.index = max(0, min(s.index, len(sides)-1))
	s.from = max(0, min(s.from, len(sides)-1))
	c := widgets.NewCarousel(widgets.CarouselOptions{
		Images: sides,
		Labels: labels,
		Index:  s.index,
		From:   s.from,
		Start:  s.slideStart,
		Now:    s.now,
		Gap:    style.DefaultSpacing,
		Peek:   4 * style.DefaultSpacing,
		Move:   func(dir int) { s.move(dir, len(sides)) },
	})

	var keys []string
	if b := c.Prev(); b != nil {
		focus.RegisterFocusButton(artworkPrevKey, b)
		keys = append(keys, artworkPrevKey)
	}
	if b := c.Next(); b != nil {
		focus.RegisterFocusButton(artworkNextKey, b)
		keys = append(keys, artworkNextKey)
	}
	if len(keys) > 0 {
		focus.RegisterNavZone(artworkZone, types.NavZoneHorizontal, keys, 0)
		s.zones = append(s.zones, artworkZone)
	}
	return c.Container()
}

// move shows the image dir away in a row of n, sliding from the image
// shown, and keeps focus on the slice pressed, or the one on the other
// side when the row ends there. A move past either end is ignored.
func (s *ArtworkSection) move(dir, n int) {
	next := s.index + dir
	if next < 0 || next >= n {
		return
	}
	s.from = s.index
	s.index = next
	s.slideStart = s.now()

	key := artworkNextKey
	if dir < 0 {
		key = artworkPrevKey
	}
	if next == 0 {
		key = artworkNextKey
	} else if next == n-1 {
		key = artworkPrevKey
	}
	if s.focus != nil {
		s.focus.SetPendingFocus(key)
	}
	s.callback.RequestRebuild()
}
