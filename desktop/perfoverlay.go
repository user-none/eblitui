// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"fmt"
	"image"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/style"
)

// perfWindow is the interval the performance statistics are averaged
// over. It is split into perfBuckets buckets, and the statistics are
// published each time a bucket completes, so the window rolls forward
// one bucket at a time.
const (
	perfWindow     = time.Second
	perfBuckets    = 4
	perfBucketSpan = perfWindow / perfBuckets
)

// perfGap is the pause between two frames beyond which the window
// restarts rather than counting the pause as elapsed time, such as
// after the pause menu closes.
const perfGap = 500 * time.Millisecond

// perfBucket accumulates the frames of one bucket of the window.
type perfBucket struct {
	start      time.Time
	frames     int
	runTime    time.Duration
	gameFrames uint64 // the core's game frame count at start
}

// PerfStats accumulates per-frame measurements on the emulation
// goroutine and publishes their averages over the window once per
// bucket for the Ebiten thread to read.
type PerfStats struct {
	mu          sync.Mutex
	emuFPS      float64
	gameFPS     float64
	frameTimeMS float64
	info        *coreif.RenderInfo // nil when the core does not report; never modified once published

	// The core's render info provider, or nil. Set by Reset before the
	// emulation goroutine starts; read only by AddFrame, between RunFrame
	// calls.
	provider coreif.RenderInfoProvider

	// Window accumulation, emulation goroutine only. buckets is a ring:
	// cur is the bucket being filled and filled the number in use,
	// counting cur. filled is 0 when no window has started.
	buckets   [perfBuckets]perfBucket
	cur       int
	filled    int
	lastFrame time.Time
}

// AddFrame records one emulator frame that took d to run and finished
// at now, with gameFrames the core's game frame count after it. A
// frame that starts a window, the first or one after a gap, is not
// counted, so the window's counts and its game frame count start
// together. When the current bucket has spanned perfBucketSpan, the
// averages over the buckets in use are published and a new bucket
// starts at this frame, replacing the oldest once the window is full.
// The core's render info is read with each publish, so a renderer
// change shows by the next one.
func (p *PerfStats) AddFrame(d time.Duration, gameFrames uint64, now time.Time) {
	if p.filled == 0 || now.Sub(p.lastFrame) > perfGap {
		p.startWindow(now, gameFrames)
		return
	}
	p.lastFrame = now
	b := &p.buckets[p.cur]
	b.frames++
	b.runTime += d
	if now.Sub(b.start) < perfBucketSpan {
		return
	}

	var frames int
	var runTime time.Duration
	for i := range p.filled {
		bucket := &p.buckets[(p.cur-i+perfBuckets)%perfBuckets]
		frames += bucket.frames
		runTime += bucket.runTime
	}
	oldest := &p.buckets[(p.cur-p.filled+1+perfBuckets)%perfBuckets]
	sec := now.Sub(oldest.start).Seconds()
	var gameDelta uint64
	if gameFrames > oldest.gameFrames {
		gameDelta = gameFrames - oldest.gameFrames
	}
	var info *coreif.RenderInfo
	if p.provider != nil {
		ri := p.provider.RenderInfo()
		info = &ri
	}
	p.mu.Lock()
	p.emuFPS = float64(frames) / sec
	p.gameFPS = float64(gameDelta) / sec
	p.frameTimeMS = float64(runTime.Microseconds()) / 1000 / float64(frames)
	p.info = info
	p.mu.Unlock()

	p.cur = (p.cur + 1) % perfBuckets
	if p.filled < perfBuckets {
		p.filled++
	}
	p.buckets[p.cur] = perfBucket{start: now, gameFrames: gameFrames}
}

// startWindow begins a window of one bucket at now with the game frame
// count as it stands.
func (p *PerfStats) startWindow(now time.Time, gameFrames uint64) {
	p.buckets = [perfBuckets]perfBucket{}
	p.buckets[0] = perfBucket{start: now, gameFrames: gameFrames}
	p.cur = 0
	p.filled = 1
	p.lastFrame = now
}

// Snapshot returns the last published emulator frames per second,
// game frames per second, average frame time in milliseconds, and
// render info, nil when the core does not report it.
func (p *PerfStats) Snapshot() (emuFPS, gameFPS, frameTimeMS float64, info *coreif.RenderInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.emuFPS, p.gameFPS, p.frameTimeMS, p.info
}

// Reset clears the published values and the window for a new game
// and takes the core's render info provider, or nil. The render info
// is first published with the first bucket. Called before the
// emulation goroutine starts.
func (p *PerfStats) Reset(provider coreif.RenderInfoProvider) {
	p.provider = provider
	p.mu.Lock()
	p.emuFPS, p.gameFPS, p.frameTimeMS = 0, 0, 0
	p.info = nil
	p.mu.Unlock()
	p.buckets = [perfBuckets]perfBucket{}
	p.cur = 0
	p.filled = 0
	p.lastFrame = time.Time{}
}

// perfOverlayMode is what the performance overlay shows.
type perfOverlayMode int

const (
	perfOverlayOff      perfOverlayMode = iota
	perfOverlayBasic                    // emulator and game frame rates and frame time
	perfOverlayExtended                 // basic plus renderer
)

// PerfOverlay draws performance statistics in the top-left corner of
// the screen. Its mode cycles off, basic, extended, off, and persists
// across games; its statistics reset per game.
type PerfOverlay struct {
	mode  perfOverlayMode
	stats PerfStats
	bg    *ebiten.Image
}

// Cycle advances the mode: off, basic, extended, off. Ebiten thread.
func (o *PerfOverlay) Cycle() {
	o.mode = (o.mode + 1) % (perfOverlayExtended + 1)
}

// Stats is the overlay's statistics, reset for each game and fed by
// the emulation goroutine.
func (o *PerfOverlay) Stats() *PerfStats { return &o.stats }

// perfOverlayLines lays out the overlay's text for the mode. The game
// frame rate and renderer lines need the core's render info, and are
// left out when info is nil. The renderer is the backend, with the
// scale when it is above native, and the device on its own line when
// there is one.
func perfOverlayLines(mode perfOverlayMode, emuFPS, gameFPS, frameTimeMS float64, info *coreif.RenderInfo) []string {
	if mode == perfOverlayOff {
		return nil
	}
	lines := []string{fmt.Sprintf("%.1f fps", emuFPS)}
	if info != nil {
		lines = append(lines, fmt.Sprintf("%.1f gfps", gameFPS))
	}
	lines = append(lines, fmt.Sprintf("%.1f ms", frameTimeMS))
	if mode == perfOverlayExtended && info != nil {
		backend := info.Backend
		if info.Scale > 1 {
			backend = fmt.Sprintf("%s (%dx)", backend, info.Scale)
		}
		lines = append(lines, backend)
		if info.Device != "" {
			lines = append(lines, info.Device)
		}
	}
	return lines
}

// Draw renders the overlay when its mode is on. Ebiten thread.
func (o *PerfOverlay) Draw(screen *ebiten.Image) {
	if o.mode == perfOverlayOff {
		return
	}
	emuFPS, gameFPS, frameTimeMS, info := o.stats.Snapshot()
	body := strings.Join(perfOverlayLines(o.mode, emuFPS, gameFPS, frameTimeMS, info), "\n")

	face := *style.FontFace()
	m := face.Metrics()
	lineSpacing := m.HAscent + m.HDescent + m.HLineGap
	textWidth, textHeight := text.Measure(body, face, lineSpacing)

	padding := style.OverlayPadding
	bgWidth := int(textWidth) + padding*2
	bgHeight := int(textHeight) + padding*2
	margin := style.OverlayMargin
	bgX := margin
	bgY := margin

	if o.bg == nil || o.bg.Bounds().Dx() < bgWidth || o.bg.Bounds().Dy() < bgHeight {
		o.bg = ebiten.NewImage(bgWidth, bgHeight)
	}
	o.bg.Clear()
	overlayBg := style.OverlayBackground
	overlayBg.A = 153 // 60% opacity
	o.bg.Fill(overlayBg)

	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(bgX), float64(bgY))
	screen.DrawImage(o.bg.SubImage(image.Rect(0, 0, bgWidth, bgHeight)).(*ebiten.Image), opts)

	textOpts := &text.DrawOptions{}
	textOpts.LineSpacing = lineSpacing
	textOpts.GeoM.Translate(float64(bgX+padding), float64(bgY+padding))
	textOpts.ColorScale.ScaleWithColor(style.Text)
	text.Draw(screen, body, face, textOpts)
}
