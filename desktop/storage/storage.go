// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

var appName string

// Init sets the application data directory name. Must be called before
// any storage operations.
func Init(dataDirName string) {
	appName = dataDirName
}

const (
	configFile      = "config.json"
	libraryFile     = "library.json"
	removedFile     = "removed.json"
	artworkDir      = "artwork"
	gameSettingsDir = "gamesettings"
	savesDir        = "saves"
	screenshotDir   = "screenshots"
	updateDir       = "update"
)

// GetBaseDir returns the base directory for application data.
// The directory name is set by Init(). Example paths:
// - macOS: ~/Library/Application Support/<appName>
// - Linux: ~/.local/share/<appName>
// - Windows: %APPDATA%/<appName>
func GetBaseDir() (string, error) {
	var baseDir string

	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		baseDir = filepath.Join(home, "Library", "Application Support", appName)
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA environment variable not set")
		}
		baseDir = filepath.Join(appData, appName)
	default: // Linux and other Unix-like systems
		// Check XDG_DATA_HOME first
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome != "" {
			baseDir = filepath.Join(dataHome, appName)
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("failed to get home directory: %w", err)
			}
			baseDir = filepath.Join(home, ".local", "share", appName)
		}
	}

	return baseDir, nil
}

// EnsureDirectories creates all necessary directories for the application
func EnsureDirectories() error {
	baseDir, err := GetBaseDir()
	if err != nil {
		return err
	}

	dirs := []string{
		baseDir,
		filepath.Join(baseDir, artworkDir),
		filepath.Join(baseDir, gameSettingsDir),
		filepath.Join(baseDir, savesDir),
		filepath.Join(baseDir, screenshotDir),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// GetConfigPath returns the full path to config.json
func GetConfigPath() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, configFile), nil
}

// GetLibraryPath returns the full path to library.json
func GetLibraryPath() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, libraryFile), nil
}

// GetRemovedGamesPath returns the full path to removed.json
func GetRemovedGamesPath() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, removedFile), nil
}

// GetArtworkDir returns the full path to the artwork directory
func GetArtworkDir() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, artworkDir), nil
}

// GetGameSettingsDir returns the full path to the per-game settings directory
func GetGameSettingsDir() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, gameSettingsDir), nil
}

// GetGameSettingsPath returns the path to a game's settings file. An empty
// or unclean game ID is an error.
func GetGameSettingsPath(gameID string) (string, error) {
	if err := gameKeyError(gameID); err != nil {
		return "", err
	}
	dir, err := GetGameSettingsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, gameID+".json"), nil
}

// GetSavesDir returns the full path to the saves directory
func GetSavesDir() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, savesDir), nil
}

// GetScreenshotDir returns the full path to the screenshots directory
func GetScreenshotDir() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, screenshotDir), nil
}

// GetUpdateDir returns the full path to the update state directory
func GetUpdateDir() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, updateDir), nil
}

// GetGameScreenshotDir returns the screenshot directory for a specific game
// (by game ID). An empty or unclean game ID is an error.
func GetGameScreenshotDir(gameID string) (string, error) {
	if err := gameKeyError(gameID); err != nil {
		return "", err
	}
	dir, err := GetScreenshotDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, gameID), nil
}

// GetGameSaveDir returns the save directory for a specific game (by game
// ID). An empty or unclean game ID is an error.
func GetGameSaveDir(gameID string) (string, error) {
	if err := gameKeyError(gameID); err != nil {
		return "", err
	}
	savesDir, err := GetSavesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(savesDir, gameID), nil
}

// GetGameArtworkDir returns the directory holding a game's artwork images.
// An empty or unclean game ID is an error.
func GetGameArtworkDir(gameID string) (string, error) {
	if err := gameKeyError(gameID); err != nil {
		return "", err
	}
	artworkDir, err := GetArtworkDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(artworkDir, gameID), nil
}

// AtomicWriteJSON writes data to a JSON file atomically.
// It writes to a temporary file first, then renames to the target path.
// This ensures the file is never in a partially-written state.
func AtomicWriteJSON(path string, data interface{}) error {
	return atomicWriteJSON(path, data, 0644)
}

// atomicWriteJSON is AtomicWriteJSON writing the file with mode perm.
func atomicWriteJSON(path string, data interface{}, perm os.FileMode) error {
	// Marshal JSON with indentation for readability
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return atomicWriteFile(path, jsonData, perm)
}

// AtomicWriteFile writes data to a file atomically.
// It writes to a temporary file first, then renames to the target path.
// This ensures the file is never in a partially-written state.
func AtomicWriteFile(path string, data []byte) error {
	return atomicWriteFile(path, data, 0644)
}

// writeLocks serialize writes to the same file. A path always maps to the
// same lock, so writers to one file take turns with its temp file, while
// writes to different files almost always proceed in parallel.
var writeLocks [64]sync.Mutex

// writeLock returns the lock for writes to path.
func writeLock(path string) *sync.Mutex {
	h := fnv.New32a()
	h.Write([]byte(path))
	return &writeLocks[h.Sum32()%uint32(len(writeLocks))]
}

// atomicWriteFile is AtomicWriteFile writing the file with mode perm. The
// data is written to path.tmp and flushed to disk, then renamed over path,
// so path holds either its old or its new contents, even after a crash or
// power loss.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	lock := writeLock(path)
	lock.Lock()
	defer lock.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tempFile := path + ".tmp"
	if err := writeSynced(tempFile, data, perm); err != nil {
		os.Remove(tempFile)
		return err
	}

	// Rename temp file to target (atomic on most filesystems)
	if err := os.Rename(tempFile, path); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	syncDir(dir)
	return nil
}

// writeSynced writes data to path with mode perm and flushes it to disk
// before returning. A file left at path by an earlier failed write keeps its
// mode when opened, so the mode is set explicitly.
func writeSynced(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return fmt.Errorf("failed to set temp file mode: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("failed to flush temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	return nil
}

// syncDir flushes a directory so a rename in it is on disk. It is best
// effort: the rename has already taken effect, and some filesystems do not
// support flushing a directory. Windows cannot flush a directory at all.
func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	d.Sync()
	d.Close()
}

// ReadJSON reads and unmarshals a JSON file
func ReadJSON(path string, data interface{}) error {
	jsonData, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(jsonData, data); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	return nil
}
