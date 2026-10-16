// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"errors"
	"fmt"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/storage"
)

// achievementTagPrefix starts the filename tag of a screenshot taken on an
// achievement unlock. The achievement title follows it.
const achievementTagPrefix = "achievement_"

// maxScreenshotTitleLen is the most runes of an achievement title kept in a
// screenshot filename
const maxScreenshotTitleLen = 64

// maxScreenshotTitleBytes is the most UTF-8 bytes of an achievement title
// kept in a screenshot filename. It keeps the whole filename under the 255
// byte component limit of common filesystems when the title is made of
// wide runes.
const maxScreenshotTitleBytes = 192

// maxScreenshotSuffix is the highest "-n" suffix tried when a screenshot
// name is already taken
const maxScreenshotSuffix = 100

// invalidFilenameChars are the characters a filename may not contain on
// Windows, the strictest of the supported platforms
const invalidFilenameChars = `<>:"/\|?*`

// ScreenshotManager saves screenshots of gameplay. Captures are queued
// from anywhere, including the emulation thread, and written from the
// final drawn frame by Flush at the end of Draw.
type ScreenshotManager struct {
	mu     sync.Mutex
	queued []string // filename tags of the captures waiting for the next Flush
}

// NewScreenshotManager creates a new screenshot manager
func NewScreenshotManager() *ScreenshotManager {
	return &ScreenshotManager{}
}

// Queue requests a screenshot of the next drawn frame. tag is the filename
// tag of the capture: the result of achievementTag for an achievement
// unlock, or "" for a manual capture.
func (m *ScreenshotManager) Queue(tag string) {
	m.mu.Lock()
	m.queued = append(m.queued, tag)
	m.mu.Unlock()
}

// Flush saves screen once per queued capture in the game's screenshot
// directory and empties the queue. Captures queued while no game is loaded
// are dropped. Capture is silent, with no notification; failures are
// logged.
func (m *ScreenshotManager) Flush(screen *ebiten.Image, gameCRC string) {
	tags := m.drain()
	if gameCRC == "" {
		return
	}
	for _, tag := range tags {
		if err := takeScreenshot(screen, gameCRC, tag); err != nil {
			log.Printf("Screenshot failed: %v", err)
		}
	}
}

// drain returns the queued tags in order and empties the queue
func (m *ScreenshotManager) drain() []string {
	m.mu.Lock()
	tags := m.queued
	m.queued = nil
	m.mu.Unlock()
	return tags
}

// takeScreenshot saves screen as a PNG in the game's screenshot directory
// under a name carrying tag
func takeScreenshot(screen *ebiten.Image, gameCRC string, tag string) error {
	dir, err := storage.GetGameScreenshotDir(gameCRC)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create screenshot directory: %w", err)
	}

	f, err := createScreenshotFile(dir, time.Now(), tag)
	if err != nil {
		return err
	}

	// Close before removing a failed write: an open file cannot be removed
	// on every platform. Close reports write errors of its own, such as a
	// full disk.
	err = png.Encode(f, screen)
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
		path := filepath.Join(dir, screenshotFilename(ts, n, tag))
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

// screenshotFilename returns the name of a screenshot taken at ts: the
// millisecond Unix timestamp, "-n" when n is above zero, "_" and the tag
// when one is set, and the .png extension
func screenshotFilename(ts time.Time, n int, tag string) string {
	var b strings.Builder
	b.WriteString(strconv.FormatInt(ts.UnixMilli(), 10))
	if n > 0 {
		b.WriteByte('-')
		b.WriteString(strconv.Itoa(n))
	}
	if tag != "" {
		b.WriteByte('_')
		b.WriteString(tag)
	}
	b.WriteString(".png")
	return b.String()
}

// achievementTag returns the screenshot filename tag of an achievement
// unlock: the achievement title made safe for a filename, or the
// achievement id when nothing of the title is usable
func achievementTag(title string, id uint32) string {
	name := sanitizeFilename(title)
	if name == "" {
		name = strconv.FormatUint(uint64(id), 10)
	}
	return achievementTagPrefix + name
}

// sanitizeFilename reduces s to text usable in a filename on every
// supported platform. Characters that are invalid on Windows and control
// characters are dropped, runs of whitespace become one space, and the
// result is trimmed and capped at maxScreenshotTitleLen runes and
// maxScreenshotTitleBytes bytes.
func sanitizeFilename(s string) string {
	var b strings.Builder
	pendingSpace := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			pendingSpace = true
		case unicode.IsControl(r), strings.ContainsRune(invalidFilenameChars, r):
			// dropped
		default:
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			pendingSpace = false
			b.WriteRune(r)
		}
	}
	runes := []rune(b.String())
	if len(runes) > maxScreenshotTitleLen {
		runes = runes[:maxScreenshotTitleLen]
	}
	for len(string(runes)) > maxScreenshotTitleBytes {
		runes = runes[:len(runes)-1]
	}
	return strings.TrimSpace(string(runes))
}
