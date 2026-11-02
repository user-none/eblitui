// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package detail

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/artwork"
)

// ImageLoader decodes a set of artwork files into textures in a background
// goroutine, so a screen shows them without decoding on the UI thread. The
// textures are kept at the images' own size until Release; a widget such
// as widgets.FitGraphic scales them when drawn. A load still running when
// Release is called is dropped and its textures freed.
type ImageLoader struct {
	onLoad func() // called from the loading goroutine when a load finishes

	mu      sync.Mutex
	gen     int  // counts Releases; a load started before one is stale
	started bool // a load was started since the last Release
	loading bool
	images  []*ebiten.Image // one per path of the last load, nil when missing or unreadable

	// decode reads an artwork file into a texture: decodeImage
	decode func(path string) (*ebiten.Image, error)
}

// NewImageLoader creates a loader that calls onLoad, from its goroutine,
// each time a load finishes
func NewImageLoader(onLoad func()) *ImageLoader {
	return &ImageLoader{onLoad: onLoad, decode: decodeImage}
}

// Start loads the files paths returns, in the background, unless a load was
// started since the last Release. paths runs on the loading goroutine.
func (l *ImageLoader) Start(paths func() []string) {
	l.mu.Lock()
	if l.started {
		l.mu.Unlock()
		return
	}
	l.started, l.loading = true, true
	gen := l.gen
	l.mu.Unlock()

	go func() {
		ps := paths()
		imgs := make([]*ebiten.Image, len(ps))
		for i, p := range ps {
			imgs[i] = l.loadFile(p)
		}
		l.finish(gen, imgs)
	}()
}

// loadFile decodes the file at path, or returns nil when it is missing or
// cannot be read
func (l *ImageLoader) loadFile(path string) *ebiten.Image {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	img, err := l.decode(path)
	if err != nil {
		log.Printf("Artwork %s: %v", path, err)
		return nil
	}
	return img
}

// finish stores the textures of the load started at generation gen, or
// frees them when a Release has made the load stale
func (l *ImageLoader) finish(gen int, imgs []*ebiten.Image) {
	l.mu.Lock()
	if gen != l.gen {
		l.mu.Unlock()
		deallocate(imgs)
		return
	}
	l.images = imgs
	l.loading = false
	l.mu.Unlock()
	if l.onLoad != nil {
		l.onLoad()
	}
}

// Images returns the textures of the finished load, one per path with nil
// for a missing or unreadable file, and whether a load is still running.
// The textures stay valid until Release.
func (l *ImageLoader) Images() (imgs []*ebiten.Image, loading bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.images, l.loading
}

// Release frees the textures and drops a load still running. The next
// Start loads again.
func (l *ImageLoader) Release() {
	l.mu.Lock()
	imgs := l.images
	l.gen++
	l.images = nil
	l.started, l.loading = false, false
	l.mu.Unlock()
	deallocate(imgs)
}

// deallocate frees the non-nil textures
func deallocate(imgs []*ebiten.Image) {
	for _, img := range imgs {
		if img != nil {
			img.Deallocate()
		}
	}
}

// decodeImage reads the artwork file at path into a texture of its own at
// the image's size (see artwork.Load for the limits)
func decodeImage(path string) (*ebiten.Image, error) {
	img, err := artwork.Load(path)
	if err != nil {
		return nil, err
	}
	return ebiten.NewImageFromImageWithOptions(img, &ebiten.NewImageFromImageOptions{Unmanaged: true}), nil
}
