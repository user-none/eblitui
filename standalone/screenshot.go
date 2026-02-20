//go:build !libretro

package standalone

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/standalone/storage"
)

// ScreenshotManager handles taking and saving screenshots
type ScreenshotManager struct {
	notification *Notification
}

// NewScreenshotManager creates a new screenshot manager
func NewScreenshotManager(notification *Notification) *ScreenshotManager {
	return &ScreenshotManager{
		notification: notification,
	}
}

// TakeScreenshot captures and saves a screenshot
// Per design: silent capture with no notification
func (m *ScreenshotManager) TakeScreenshot(screen *ebiten.Image, gameCRC string) error {
	// Use Unix timestamp for filename per design spec
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	var screenshotDir string
	var filename string

	if gameCRC != "" {
		// Gameplay screenshot - save to game-specific directory
		baseDir, err := storage.GetScreenshotDir()
		if err != nil {
			return err
		}
		screenshotDir = filepath.Join(baseDir, gameCRC)
		filename = timestamp + ".png"
	} else {
		// Non-gameplay screenshot
		var err error
		screenshotDir, err = storage.GetScreenshotDir()
		if err != nil {
			return err
		}
		filename = timestamp + ".png"
	}

	// Ensure directory exists
	if err := os.MkdirAll(screenshotDir, 0755); err != nil {
		return fmt.Errorf("failed to create screenshot directory: %w", err)
	}

	fullPath := filepath.Join(screenshotDir, filename)

	// Create file
	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create screenshot file: %w", err)
	}
	defer f.Close()

	// Encode image as PNG
	if err := png.Encode(f, screen); err != nil {
		return fmt.Errorf("failed to encode screenshot: %w", err)
	}

	// No notification per design spec - silent capture
	return nil
}
