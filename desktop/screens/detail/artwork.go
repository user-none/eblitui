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

// ArtworkSection is the detail Artwork page: the game's artwork on disk in
// a carousel, one image at a time with a slice of its neighbors beside it,
// in artwork.LocalFiles order for the box art region (covers, discs,
// flyers, posters) with artwork.Caption labels. The page shows what is on disk; the library's
// artwork download setting only limits what scans download. The images
// load in the background the first time the page is built for a game.
type ArtworkSection struct {
	callback types.ScreenCallback
	game     *storage.GameEntry
	files    []artwork.LocalFile // the game's artwork files, listed when the load starts
	listed   bool                // files was listed for game
	images   *ImageLoader        // one texture per entry of files
	region   string              // box art region files is ordered for

	index      int       // image shown, among the images the game has
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
	s.files, s.listed = nil, false
	s.index = 0
	s.from = 0
}

// Release frees the textures. The next Build lists and loads them again.
func (s *ArtworkSection) Release() {
	s.images.Release()
	s.files, s.listed = nil, false
}

// SetBoxArtRegion sets the box art region, a storage.BoxArtRegion*
// setting, the images are ordered for. When it changes, the textures are
// freed so the next Build lists and loads them in the new order.
func (s *ArtworkSection) SetBoxArtRegion(region string) {
	if region == s.region {
		return
	}
	s.region = region
	s.Release()
}

// NavZones returns the page's navigation zones from the last Build
func (s *ArtworkSection) NavZones() []string {
	return s.zones
}

// load lists the game's artwork files and starts loading them, unless that
// was done since the game was set or the textures released. Listing reads
// only the game's artwork directory; decoding runs in the background.
func (s *ArtworkSection) load() {
	if s.game == nil || s.listed {
		return
	}
	s.listed = true
	dir, err := s.gameDir(s.game.GameID)
	if err != nil {
		log.Printf("Artwork %s: %v", s.game.GameID, err)
		return
	}
	s.files = artwork.LocalFiles(dir, s.region)
	paths := make([]string, len(s.files))
	for i, f := range s.files {
		paths[i] = f.Path
	}
	s.images.Start(func() []string { return paths })
}

// Build creates the page: the carousel of the game's artwork, with the
// slices beside the image shown in one navigation zone. An image that
// cannot be read is left out. While the images load, and when the game has
// none, a message is shown instead.
func (s *ArtworkSection) Build(focus types.FocusManager) *widget.Container {
	s.focus = focus
	s.zones = s.zones[:0]
	s.load()
	imgs, loading := s.images.Images()

	var shown []*ebiten.Image
	var labels []string
	for i, f := range s.files {
		if i < len(imgs) && imgs[i] != nil {
			shown = append(shown, imgs[i])
			labels = append(labels, artwork.Caption(f))
		}
	}

	if len(shown) == 0 {
		msg := "No artwork"
		if loading {
			msg = "Loading..."
		}
		empty := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
		empty.AddChild(centeredText(msg))
		return empty
	}

	s.index = max(0, min(s.index, len(shown)-1))
	s.from = max(0, min(s.from, len(shown)-1))
	c := widgets.NewCarousel(widgets.CarouselOptions{
		Images: shown,
		Labels: labels,
		Index:  s.index,
		From:   s.from,
		Start:  s.slideStart,
		Now:    s.now,
		Gap:    style.DefaultSpacing,
		Peek:   4 * style.DefaultSpacing,
		Move:   func(dir int) { s.move(dir, len(shown)) },
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
