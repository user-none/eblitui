//go:build !libretro

package standalone

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/user-none/eblitui/rdb"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
)

const (
	// Base URL for libretro-database RDB files
	rdbBaseURL = "https://github.com/libretro/libretro-database/raw/refs/heads/master/rdb"

	// Base URL for libretro-thumbnails repositories
	thumbnailBaseURL = "https://github.com/libretro-thumbnails"

	// RDB filename
	rdbFilename = "game.rdb"
)

// Artwork types in fallback order
var artworkTypes = []string{
	"Named_Boxarts",
	"Named_Titles",
	"Named_Snaps",
}

// HTTP client with timeout
var httpClient = &http.Client{
	Timeout: style.HTTPTimeout,
}

// MetadataManager handles RDB and artwork downloads
type MetadataManager struct {
	rdb           *rdb.RDB
	rdbName       string // e.g. "Sega - Mega Drive - Genesis"
	thumbnailRepo string // e.g. "Sega_-_Mega_Drive_-_Genesis"
}

// NewMetadataManager creates a new metadata manager
func NewMetadataManager(rdbName, thumbnailRepo string) *MetadataManager {
	return &MetadataManager{
		rdbName:       rdbName,
		thumbnailRepo: thumbnailRepo,
	}
}

// GetRDBPath returns the path to the RDB file
func GetRDBPath() (string, error) {
	metadataDir, err := storage.GetMetadataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(metadataDir, rdbFilename), nil
}

// DownloadRDB downloads the RDB file from libretro-database
// Downloads to a temp file first, then renames on success
func (m *MetadataManager) DownloadRDB() error {
	rdbPath, err := GetRDBPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(rdbPath), 0755); err != nil {
		return fmt.Errorf("failed to create metadata directory: %w", err)
	}

	// Download to temp file
	tempPath := rdbPath + ".tmp"

	rdbURL := fmt.Sprintf("%s/%s.rdb", rdbBaseURL, url.PathEscape(m.rdbName))
	resp, err := httpClient.Get(rdbURL)
	if err != nil {
		return fmt.Errorf("failed to download RDB: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("RDB download failed with status: %d", resp.StatusCode)
	}

	// Read entire response into memory first
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read RDB data: %w", err)
	}

	// Write to temp file
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write RDB temp file: %w", err)
	}

	// Rename temp file to final location (atomic)
	if err := os.Rename(tempPath, rdbPath); err != nil {
		os.Remove(tempPath) // Clean up on failure
		return fmt.Errorf("failed to rename RDB file: %w", err)
	}

	return nil
}

// LoadRDB loads the RDB file into memory
// Returns nil without error if file doesn't exist
func (m *MetadataManager) LoadRDB() error {
	rdbPath, err := GetRDBPath()
	if err != nil {
		return err
	}

	// Check if file exists
	if _, err := os.Stat(rdbPath); os.IsNotExist(err) {
		m.rdb = nil
		return nil
	}

	// Load and parse
	loadedRDB, err := rdb.LoadRDB(rdbPath)
	if err != nil {
		// Delete corrupted file silently
		os.Remove(rdbPath)
		m.rdb = nil
		return nil
	}

	m.rdb = loadedRDB
	return nil
}

// IsRDBLoaded returns true if the RDB is loaded
func (m *MetadataManager) IsRDBLoaded() bool {
	return m.rdb != nil
}

// GetRDB returns the loaded RDB, or nil if not loaded
func (m *MetadataManager) GetRDB() *rdb.RDB {
	return m.rdb
}

// LookupByCRC32 looks up a game by CRC32
// Returns nil if not found or RDB not loaded
func (m *MetadataManager) LookupByCRC32(crc32 uint32) *rdb.Game {
	if m.rdb == nil {
		return nil
	}
	return m.rdb.FindByCRC32(crc32)
}

// RDBExists checks if the RDB file exists on disk
func RDBExists() bool {
	rdbPath, err := GetRDBPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(rdbPath)
	return err == nil
}

// DownloadArtwork downloads artwork for a game using the fallback chain
// Returns silently on any error (per spec)
func (m *MetadataManager) DownloadArtwork(gameCRC string, gameName string) {
	if gameName == "" {
		return
	}

	// Check if artwork already exists
	artworkPath, err := storage.GetGameArtworkPath(gameCRC)
	if err != nil {
		return
	}

	if _, err := os.Stat(artworkPath); err == nil {
		return // Already exists
	}

	// Ensure directory exists
	artworkDir := filepath.Dir(artworkPath)
	if err := os.MkdirAll(artworkDir, 0755); err != nil {
		return
	}

	// URL-encode the game name (spaces as %20)
	encodedName := url.PathEscape(gameName)

	// Try each artwork type in fallback order
	for _, artType := range artworkTypes {
		artURL := fmt.Sprintf("%s/%s/raw/refs/heads/master/%s/%s.png",
			thumbnailBaseURL, m.thumbnailRepo, artType, encodedName)

		data, err := downloadToMemory(artURL)
		if err != nil {
			continue // Try next type
		}

		// Successfully downloaded, write to disk
		if err := os.WriteFile(artworkPath, data, 0644); err != nil {
			return // Silently fail
		}

		return // Success
	}

	// All downloads failed - silently return
}

// downloadToMemory downloads a URL entirely into memory
func downloadToMemory(urlStr string) ([]byte, error) {
	resp, err := httpClient.Get(urlStr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
