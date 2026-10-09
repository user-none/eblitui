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
// over and published at.
const perfWindow = time.Second

// perfGap is the pause between two frames beyond which the window
// restarts rather than counting the pause as elapsed time, such as
// after the pause menu closes.
const perfGap = 500 * time.Millisecond

// PerfStats accumulates per-frame measurements on the emulation
// goroutine and publishes their averages once per window for the
// Ebiten thread to read.
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

	// Window accumulation, emulation goroutine only.
	windowStart     time.Time
	lastFrame       time.Time
	frames          int
	runTime         time.Duration
	gameFramesStart uint64
}

// AddFrame records one emulator frame that took d to run and finished
// at now, with gameFrames the core's game frame count after it. A
// frame that starts a window, the first or one after a gap, is not
// counted, so the window's counts and its game frame count start
// together. When the window has elapsed the averages are published
// and a new window starts at this frame. The core's render info is
// read with each publish, so a renderer change shows by the next one.
func (p *PerfStats) AddFrame(d time.Duration, gameFrames uint64, now time.Time) {
	if p.windowStart.IsZero() || now.Sub(p.lastFrame) > perfGap {
		p.startWindow(now, gameFrames)
		return
	}
	p.lastFrame = now
	p.frames++
	p.runTime += d
	elapsed := now.Sub(p.windowStart)
	if elapsed < perfWindow {
		return
	}
	sec := elapsed.Seconds()
	var gameDelta uint64
	if gameFrames > p.gameFramesStart {
		gameDelta = gameFrames - p.gameFramesStart
	}
	var info *coreif.RenderInfo
	if p.provider != nil {
		ri := p.provider.RenderInfo()
		info = &ri
	}
	p.mu.Lock()
	p.emuFPS = float64(p.frames) / sec
	p.gameFPS = float64(gameDelta) / sec
	p.frameTimeMS = float64(p.runTime.Microseconds()) / 1000 / float64(p.frames)
	p.info = info
	p.mu.Unlock()
	p.startWindow(now, gameFrames)
}

// startWindow begins a window at now with the game frame count as it
// stands.
func (p *PerfStats) startWindow(now time.Time, gameFrames uint64) {
	p.windowStart = now
	p.lastFrame = now
	p.frames = 0
	p.runTime = 0
	p.gameFramesStart = gameFrames
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
// is first published with the first window. Called before the
// emulation goroutine starts.
func (p *PerfStats) Reset(provider coreif.RenderInfoProvider) {
	p.provider = provider
	p.mu.Lock()
	p.emuFPS, p.gameFPS, p.frameTimeMS = 0, 0, 0
	p.info = nil
	p.mu.Unlock()
	p.windowStart = time.Time{}
	p.lastFrame = time.Time{}
	p.frames = 0
	p.runTime = 0
	p.gameFramesStart = 0
}

// perfOverlayMode is what the performance overlay shows.
type perfOverlayMode int

const (
	perfOverlayOff      perfOverlayMode = iota
	perfOverlayBasic                    // emulator and game frame rates
	perfOverlayExtended                 // basic plus frame time and renderer
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
// left out when info is nil. The renderer line is the device name
// when the renderer has one, otherwise the renderer name, followed by
// the scale when it is above native.
func perfOverlayLines(mode perfOverlayMode, emuFPS, gameFPS, frameTimeMS float64, info *coreif.RenderInfo) []string {
	if mode == perfOverlayOff {
		return nil
	}
	lines := []string{fmt.Sprintf("%.1f fps", emuFPS)}
	if info != nil {
		lines = append(lines, fmt.Sprintf("%.1f gfps", gameFPS))
	}
	if mode != perfOverlayExtended {
		return lines
	}
	lines = append(lines, fmt.Sprintf("%.1f ms", frameTimeMS))
	if info != nil {
		name := info.Backend
		if info.Device != "" {
			name = fmt.Sprintf("%s (%s)", info.Device, info.Backend)
		}
		if info.Scale > 1 {
			name = fmt.Sprintf("%s %dx", name, info.Scale)
		}
		lines = append(lines, name)
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
