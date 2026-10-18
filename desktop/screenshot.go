// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/storage"
)

// maxScreenshotSuffix is the highest "-n" suffix tried when a screenshot
// name is already taken
const maxScreenshotSuffix = 100

// screenshotQueueDepth is how many captured frames may wait for the
// encoder. Each holds a full window frame, so the depth bounds memory when
// captures come faster than they can be encoded.
const screenshotQueueDepth = 14

// screenshotJob is one captured frame and the files to write from it
type screenshotJob struct {
	frame   *image.RGBA
	ts      time.Time // capture time, used in the filenames
	gameCRC string
	tags    []string // one file per tag
}

// ScreenshotManager saves screenshots of gameplay. Captures are queued
// from anywhere, including the emulation thread. Flush, at the end of
// Draw, copies the final drawn frame out of the screen and hands it to an
// encoder goroutine, which encodes and writes the files off the draw
// thread.
type ScreenshotManager struct {
	mu     sync.Mutex
	queued []string // filename tags of the captures waiting for the next Flush
	closed bool     // Close has run; captures are dropped

	jobs chan screenshotJob // captured frames for the encoder
	done chan struct{}      // closed when the encoder exits

	// gameDir returns a game's screenshot directory:
	// storage.GetGameScreenshotDir
	gameDir func(gameCRC string) (string, error)
}

// NewScreenshotManager creates a screenshot manager with its encoder
// running. Close stops it.
func NewScreenshotManager() *ScreenshotManager {
	m := newScreenshotManager(screenshotQueueDepth)
	go m.run()
	return m
}

// newScreenshotManager creates a manager that queues up to depth frames.
// The encoder is not started.
func newScreenshotManager(depth int) *ScreenshotManager {
	return &ScreenshotManager{
		jobs:    make(chan screenshotJob, depth),
		done:    make(chan struct{}),
		gameDir: storage.GetGameScreenshotDir,
	}
}

// Queue requests a screenshot of the next drawn frame. tag is the filename
// tag of the capture: the result of achievementTag for an achievement
// unlock, or "" for a manual capture.
func (m *ScreenshotManager) Queue(tag string) {
	m.mu.Lock()
	m.queued = append(m.queued, tag)
	m.mu.Unlock()
}

// Flush copies the picture region of screen for the queued captures and
// empties the queue, so letterbox and pillarbox bars are left out. The
// frame is encoded and written later, once per capture, in the game's
// screenshot directory. Captures queued while no game is loaded are
// dropped. Capture is silent, with no notification; failures are logged.
func (m *ScreenshotManager) Flush(screen *ebiten.Image, picture image.Rectangle, gameCRC string) {
	tags := m.drain()
	if len(tags) == 0 || gameCRC == "" {
		return
	}
	r := screenshotCaptureRect(picture, screen.Bounds())
	frame := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	screen.SubImage(r).(*ebiten.Image).ReadPixels(frame.Pix)
	m.capture(frame, gameCRC, tags)
}

// screenshotCaptureRect returns the region of a screen with the bounds to
// capture: the picture clipped to the screen, or the whole screen when no
// part of the picture is on it.
func screenshotCaptureRect(picture, bounds image.Rectangle) image.Rectangle {
	r := picture.Intersect(bounds)
	if r.Empty() {
		return bounds
	}
	return r
}

// Close stops the encoder once the queued frames are written. Captures
// after Close are dropped.
func (m *ScreenshotManager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	close(m.jobs)
	m.mu.Unlock()
	<-m.done
}

// drain returns the queued tags in order and empties the queue
func (m *ScreenshotManager) drain() []string {
	m.mu.Lock()
	tags := m.queued
	m.queued = nil
	m.mu.Unlock()
	return tags
}

// capture hands frame to the encoder to be written under gameCRC, one file
// per tag. The frame is dropped when the encoder is too far behind or the
// manager is closed.
func (m *ScreenshotManager) capture(frame *image.RGBA, gameCRC string, tags []string) {
	job := screenshotJob{frame: frame, ts: time.Now(), gameCRC: gameCRC, tags: tags}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	select {
	case m.jobs <- job:
	default:
		log.Printf("Screenshot dropped: %d frames already waiting to be saved", len(m.jobs))
	}
}

// run encodes and writes jobs until the job channel is closed, logging
// failures
func (m *ScreenshotManager) run() {
	defer close(m.done)
	for job := range m.jobs {
		if err := m.save(job); err != nil {
			log.Printf("Screenshot failed: %v", err)
		}
	}
}

// save encodes the job's frame once and writes it to a file per tag. A
// file that cannot be written is logged and the rest are still written.
func (m *ScreenshotManager) save(job screenshotJob) error {
	dir, err := m.gameDir(job.gameCRC)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create screenshot directory: %w", err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, job.frame); err != nil {
		return fmt.Errorf("failed to encode screenshot: %w", err)
	}
	for _, tag := range job.tags {
		if err := writeScreenshot(dir, job.ts, tag, buf.Bytes()); err != nil {
			log.Printf("Screenshot failed: %v", err)
		}
	}
	return nil
}

// writeScreenshot writes encoded PNG data to a new file in dir named for
// ts and tag
func writeScreenshot(dir string, ts time.Time, tag string, data []byte) error {
	f, err := createScreenshotFile(dir, ts, tag)
	if err != nil {
		return err
	}

	// Close before removing a failed write: an open file cannot be removed
	// on every platform. Close reports write errors of its own, such as a
	// full disk.
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return fmt.Errorf("failed to write screenshot: %w", err)
	}
	return nil
}

// createScreenshotFile creates a new file in dir for a screenshot taken at
// ts. When the name is taken, "-1", "-2", ... up to maxScreenshotSuffix are
// tried in turn.
func createScreenshotFile(dir string, ts time.Time, tag string) (*os.File, error) {
	for n := 0; n <= maxScreenshotSuffix; n++ {
		path := filepath.Join(dir, storage.ScreenshotFilename(ts, n, tag))
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("failed to create screenshot file: %w", err)
		}
	}
	return nil, fmt.Errorf("failed to create screenshot file: %d names taken", maxScreenshotSuffix+1)
}

// achievementTag returns the screenshot filename tag of an achievement
// unlock: the achievement title made safe for a filename, or the
// achievement id when nothing of the title is usable
func achievementTag(title string, id uint32) string {
	name := storage.SanitizeScreenshotTitle(title)
	if name == "" {
		name = strconv.FormatUint(uint64(id), 10)
	}
	return storage.ScreenshotAchievementTag + name
}
