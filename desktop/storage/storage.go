package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

var appName string

// Init sets the application data directory name. Must be called before
// any storage operations.
func Init(dataDirName string) {
	appName = dataDirName
}

const (
	configFile    = "config.json"
	libraryFile   = "library.json"
	metadataDir   = "metadata"
	artworkDir    = "artwork"
	rumbleDir     = "rumble"
	savesDir      = "saves"
	screenshotDir = "screenshots"
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
		filepath.Join(baseDir, metadataDir),
		filepath.Join(baseDir, artworkDir),
		filepath.Join(baseDir, rumbleDir),
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

// GetMetadataDir returns the full path to the metadata directory
func GetMetadataDir() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, metadataDir), nil
}

// GetArtworkDir returns the full path to the artwork directory
func GetArtworkDir() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, artworkDir), nil
}

// GetRumbleDir returns the full path to the rumble directory
func GetRumbleDir() (string, error) {
	baseDir, err := GetBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, rumbleDir), nil
}

// GetGameRumblePath returns the path to a game's rumble CHT file
func GetGameRumblePath(gameCRC string) (string, error) {
	rumbleDir, err := GetRumbleDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(rumbleDir, gameCRC+".cht"), nil
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

// GetGameSaveDir returns the save directory for a specific game (by CRC32)
func GetGameSaveDir(gameCRC string) (string, error) {
	savesDir, err := GetSavesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(savesDir, gameCRC), nil
}

// GetGameArtworkPath returns the path to a game's box art
func GetGameArtworkPath(gameCRC string) (string, error) {
	artworkDir, err := GetArtworkDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(artworkDir, gameCRC, "boxart.png"), nil
}

// AtomicWriteJSON writes data to a JSON file atomically.
// It writes to a temporary file first, then renames to the target path.
// This ensures the file is never in a partially-written state.
func AtomicWriteJSON(path string, data interface{}) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Marshal JSON with indentation for readability
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Write to temporary file in the same directory
	tempFile := path + ".tmp"
	if err := os.WriteFile(tempFile, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Rename temp file to target (atomic on most filesystems)
	if err := os.Rename(tempFile, path); err != nil {
		os.Remove(tempFile) // Clean up on failure
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
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
