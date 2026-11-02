// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package detail

import (
	"log"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/artwork"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// ArtworkSection is the detail Artwork page: the game's box art and box
// back, each labeled, side by side or stacked, whichever shows them larger.
// A side the game has no image for is left out. The images load in the
// background the first time the page is built for a game.
type ArtworkSection struct {
	game   *storage.GameEntry
	images *ImageLoader // the box art and box back, in that order

	// gameDir returns a game's artwork directory:
	// storage.GetGameArtworkDir
	gameDir func(gameID string) (string, error)
}

// NewArtworkSection creates the Artwork page. callback is asked to rebuild
// the screen when the images finish loading.
func NewArtworkSection(callback types.ScreenCallback) *ArtworkSection {
	return &ArtworkSection{
		images:  NewImageLoader(func() { callback.RequestRebuild() }),
		gameDir: storage.GetGameArtworkDir,
	}
}

// SetGame shows a game's artwork, loaded on the next Build. A nil game
// clears the page. Either way the previous textures are freed.
func (s *ArtworkSection) SetGame(game *storage.GameEntry) {
	s.images.Release()
	s.game = game
}

// Release frees the textures. The next Build loads them again.
func (s *ArtworkSection) Release() {
	s.images.Release()
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

// Build creates the page: each side the game has with its label directly
// above it, the sides at a shared height or width. While the images load,
// and when the game has none, a message is shown instead.
func (s *ArtworkSection) Build(focus types.FocusManager) *widget.Container {
	s.load()
	imgs, loading := s.images.Images()

	type side struct {
		label string
		img   *ebiten.Image
	}
	var sides []side
	for i, label := range []string{"Front", "Back"} {
		if i < len(imgs) && imgs[i] != nil {
			sides = append(sides, side{label, imgs[i]})
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

	aspects := make([]float64, len(sides))
	for i, sd := range sides {
		b := sd.img.Bounds()
		aspects[i] = float64(b.Dx()) / float64(b.Dy())
	}
	page := widget.NewContainer(
		widget.ContainerOpts.Layout(widgets.NewCaptionedImagesLayout(aspects, style.DefaultSpacing, style.SmallSpacing)),
	)
	for _, sd := range sides {
		page.AddChild(widget.NewText(
			widget.TextOpts.Text(sd.label, style.FontFace(), style.TextSecondary),
		))
		page.AddChild(widgets.NewFitGraphic(sd.img))
	}
	return page
}
