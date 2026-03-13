package screens

import (
	"bytes"
	goimage "image"
	"os"
	"sync"
	"sync/atomic"

	_ "image/png"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
)

// artworkLoader fills the artwork cache asynchronously in a background goroutine.
// On invalidation events (resize, scan complete), the goroutine is cancelled, the
// cache is cleared, and a new goroutine is started with updated parameters.
//
// Cache keys:
//   - "loading": solid-color image shown while artwork is being loaded
//   - "missing": scaled missing-art PNG shown when no artwork file exists
//   - game CRC: real artwork on success, or same entry as "missing" on failure
//
// Get returning nil for a CRC means it has not been processed yet (still loading).
type artworkLoader struct {
	mu    sync.RWMutex
	cache map[string]*iconArtwork

	cancel chan struct{}
	done   chan struct{}
	halted bool
	hasNew atomic.Bool

	cardWidth int
	artHeight int

	placeholderData []byte
	missingArtData  []byte
}

// newArtworkLoader creates a new artworkLoader with the given image data.
// placeholderData is the loading indicator PNG. missingArtData is the
// missing-art PNG shown for games with no artwork file.
func newArtworkLoader(placeholderData []byte, missingArtData []byte) *artworkLoader {
	return &artworkLoader{
		cache:           make(map[string]*iconArtwork),
		placeholderData: placeholderData,
		missingArtData:  missingArtData,
	}
}

// Get returns cached artwork for the given CRC, or nil if not yet loaded.
func (a *artworkLoader) Get(crc string) *iconArtwork {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cache[crc]
}

// HaveNew returns true if new artwork has been cached since the last call.
// Resets the flag on read.
func (a *artworkLoader) HaveNew() bool {
	return a.hasNew.Swap(false)
}

// stop cancels the running goroutine (if any) and waits for it to finish.
func (a *artworkLoader) stop() {
	if a.cancel != nil {
		close(a.cancel)
		<-a.done
		a.cancel = nil
		a.done = nil
	}
}

// findMissing returns CRCs from gameCRCs that are not yet in the cache.
func (a *artworkLoader) findMissing(gameCRCs []string) []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var missing []string
	for _, crc := range gameCRCs {
		if a.cache[crc] == nil {
			missing = append(missing, crc)
		}
	}
	return missing
}

// Halt permanently stops the loader. The goroutine is cancelled, waited on,
// and no future Start calls will launch a new one.
func (a *artworkLoader) Halt() {
	a.halted = true
	a.stop()
	a.hasNew.Store(false)
}

// CancelAndClear stops the running goroutine (if any), waits for it to finish,
// deallocates all cached images, and resets state.
func (a *artworkLoader) CancelAndClear() {
	a.stop()
	a.hasNew.Store(false)

	a.mu.Lock()
	seen := make(map[*iconArtwork]bool)
	for _, art := range a.cache {
		if art == nil || seen[art] {
			continue
		}
		seen[art] = true
		if art.normal != nil {
			art.normal.Deallocate()
		}
		if art.focused != nil {
			art.focused.Deallocate()
		}
	}
	a.cache = make(map[string]*iconArtwork)
	a.mu.Unlock()

	a.cardWidth = 0
	a.artHeight = 0
}

// Start cancels any running goroutine and starts a new one to load artwork
// for the given game CRCs at the specified dimensions. If dimensions match the
// current run, this is a no-op. If the loader has been halted, this is a no-op.
func (a *artworkLoader) Start(gameCRCs []string, cardWidth, artHeight int) {
	if a.halted {
		return
	}

	if a.cardWidth == cardWidth && a.artHeight == artHeight && a.cancel != nil {
		// Dimensions match but CRC list may have changed (e.g. filter toggle).
		// Load any CRCs not yet in the cache.
		missing := a.findMissing(gameCRCs)
		if len(missing) == 0 {
			return
		}
		a.stop()
		a.cancel = make(chan struct{})
		a.done = make(chan struct{})
		go a.run(missing, cardWidth, artHeight, a.cancel, a.done)
		return
	}

	a.CancelAndClear()

	a.cardWidth = cardWidth
	a.artHeight = artHeight

	// Build loading and missing-art images synchronously so they are
	// available immediately when buildGameCardSized runs.
	a.buildImage("loading", a.placeholderData, cardWidth, artHeight)
	a.buildImage("missing", a.missingArtData, cardWidth, artHeight)

	a.cancel = make(chan struct{})
	a.done = make(chan struct{})

	go a.run(gameCRCs, cardWidth, artHeight, a.cancel, a.done)
}

// run iterates over game CRCs, loading and scaling artwork for each.
// It checks the cancel channel between each game.
func (a *artworkLoader) run(gameCRCs []string, cardWidth, artHeight int, cancel chan struct{}, done chan struct{}) {
	defer close(done)

	for _, crc := range gameCRCs {
		select {
		case <-cancel:
			return
		default:
		}

		a.loadOne(crc, cardWidth, artHeight, cancel)
	}
}

// loadOne loads, scales, and caches artwork for a single game CRC.
// On failure the "missing" entry is stored under the CRC so the UI
// can distinguish "missing" from "still loading" (nil).
func (a *artworkLoader) loadOne(crc string, cardWidth, artHeight int, cancel chan struct{}) {
	artPath, err := storage.GetGameArtworkPath(crc)
	if err != nil {
		a.storeMissing(crc)
		return
	}

	data, err := os.ReadFile(artPath)
	if err != nil {
		a.storeMissing(crc)
		return
	}

	img, _, err := goimage.Decode(bytes.NewReader(data))
	if err != nil {
		a.storeMissing(crc)
		return
	}

	// Check cancel after I/O and decode, before ebiten calls
	select {
	case <-cancel:
		return
	default:
	}

	focusedImg := style.ScaleImage(img, cardWidth, artHeight)
	normalW := int(float64(cardWidth) * style.IconUnfocusedScale)
	normalH := int(float64(artHeight) * style.IconUnfocusedScale)
	normalImg := dimImage(style.ScaleImage(img, normalW, normalH))

	a.mu.Lock()
	a.cache[crc] = &iconArtwork{normal: normalImg, focused: focusedImg}
	a.mu.Unlock()

	a.hasNew.Store(true)
}

// storeMissing caches the pre-built missing-art image under the given CRC.
func (a *artworkLoader) storeMissing(crc string) {
	a.mu.Lock()
	a.cache[crc] = a.cache["missing"]
	a.mu.Unlock()

	a.hasNew.Store(true)
}

// buildImage decodes imageData, scales it to the card dimensions, and caches
// it under key. If imageData is nil or fails to decode, a solid-color
// fallback is used.
func (a *artworkLoader) buildImage(key string, imageData []byte, cardWidth, artHeight int) {
	normalW := int(float64(cardWidth) * style.IconUnfocusedScale)
	normalH := int(float64(artHeight) * style.IconUnfocusedScale)

	if imageData != nil {
		img, _, err := goimage.Decode(bytes.NewReader(imageData))
		if err == nil {
			focusedImg := style.ScaleImage(img, cardWidth, artHeight)
			normalImg := dimImage(style.ScaleImage(img, normalW, normalH))

			a.mu.Lock()
			a.cache[key] = &iconArtwork{normal: normalImg, focused: focusedImg}
			a.mu.Unlock()
			return
		}
	}

	// Fallback to solid color
	focusedImg := ebiten.NewImage(cardWidth, artHeight)
	focusedImg.Fill(style.Surface)
	normalImg := ebiten.NewImage(normalW, normalH)
	normalImg.Fill(style.Surface)

	a.mu.Lock()
	a.cache[key] = &iconArtwork{normal: normalImg, focused: focusedImg}
	a.mu.Unlock()
}
