package standalone

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	emucore "github.com/user-none/eblitui/api"
	"github.com/user-none/eblitui/rdb"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
)

const (
	// Base URL for libretro-database RDB files
	rdbBaseURL = "https://github.com/libretro/libretro-database/raw/refs/heads/master/rdb"

	// Base URL for libretro-thumbnails repositories
	thumbnailBaseURL = "https://github.com/libretro-thumbnails"

	// Base URL for libretro-database CHT rumble files
	chtBaseURL = "https://raw.githubusercontent.com/libretro/libretro-database/master/cht"
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

// metadataVariant holds the loaded RDB and config for a single variant.
type metadataVariant struct {
	name          string   // Display name
	rdbName       string   // e.g. "SNK - Neo Geo Pocket Color"
	thumbnailRepo string   // e.g. "SNK_-_Neo_Geo_Pocket_Color"
	rdb           *rdb.RDB // Loaded RDB, nil if not loaded
}

// MetadataManager handles RDB and artwork downloads for one or more
// metadata variants (RDB + thumbnail repo pairs).
type MetadataManager struct {
	variants []metadataVariant
}

// NewMetadataManager creates a new metadata manager from the given variants.
func NewMetadataManager(variants []emucore.MetadataVariant) *MetadataManager {
	mv := make([]metadataVariant, len(variants))
	for i, v := range variants {
		mv[i] = metadataVariant{
			name:          v.Name,
			rdbName:       v.RDBName,
			thumbnailRepo: v.ThumbnailRepo,
		}
	}
	return &MetadataManager{variants: mv}
}

// GetRDBPath returns the path to the RDB file for the given rdbName.
func GetRDBPath(rdbName string) (string, error) {
	metadataDir, err := storage.GetMetadataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(metadataDir, rdbName+".rdb"), nil
}

// DownloadRDB downloads all variant RDB files from libretro-database.
// Downloads to a temp file first, then renames on success.
func (m *MetadataManager) DownloadRDB() error {
	for i := range m.variants {
		if err := m.downloadVariantRDB(i); err != nil {
			return err
		}
	}
	return nil
}

// downloadVariantRDB downloads a single variant's RDB file.
func (m *MetadataManager) downloadVariantRDB(idx int) error {
	v := &m.variants[idx]
	rdbPath, err := GetRDBPath(v.rdbName)
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(rdbPath), 0755); err != nil {
		return fmt.Errorf("failed to create metadata directory: %w", err)
	}

	// Download to temp file
	tempPath := rdbPath + ".tmp"

	rdbURL := fmt.Sprintf("%s/%s.rdb", rdbBaseURL, url.PathEscape(v.rdbName))
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

// LoadRDB loads all variant RDB files into memory.
// Returns nil without error if a file doesn't exist.
func (m *MetadataManager) LoadRDB() error {
	for i := range m.variants {
		v := &m.variants[i]
		rdbPath, err := GetRDBPath(v.rdbName)
		if err != nil {
			return err
		}

		loadedRDB, err := rdb.LoadRDB(rdbPath)
		if err != nil {
			if !os.IsNotExist(err) {
				// Delete corrupted file silently
				os.Remove(rdbPath)
			}
			v.rdb = nil
			continue
		}

		v.rdb = loadedRDB
	}
	return nil
}

// IsRDBLoaded returns true if any variant RDB is loaded.
func (m *MetadataManager) IsRDBLoaded() bool {
	for _, v := range m.variants {
		if v.rdb != nil {
			return true
		}
	}
	return false
}

// LookupByCRC32 looks up a game by CRC32 across all loaded RDBs.
// Returns the game and the variant index where it was found.
// Returns nil, -1 if not found or no RDB is loaded.
func (m *MetadataManager) LookupByCRC32(crc32 uint32) (*rdb.Game, int) {
	for i, v := range m.variants {
		if v.rdb == nil {
			continue
		}
		if game := v.rdb.FindByCRC32(crc32); game != nil {
			return game, i
		}
	}
	return nil, -1
}

// GetMD5ByCRC32 searches all loaded RDBs for an MD5 hash matching the CRC32.
func (m *MetadataManager) GetMD5ByCRC32(crc32 uint32) string {
	for _, v := range m.variants {
		if v.rdb == nil {
			continue
		}
		if md5 := v.rdb.GetMD5ByCRC32(crc32); md5 != "" {
			return md5
		}
	}
	return ""
}

// RDBExists checks if all variant RDB files exist on disk.
func (m *MetadataManager) RDBExists() bool {
	for _, v := range m.variants {
		rdbPath, err := GetRDBPath(v.rdbName)
		if err != nil {
			return false
		}
		if _, err := os.Stat(rdbPath); err != nil {
			return false
		}
	}
	return true
}

// VariantName returns the display name for a variant index.
func (m *MetadataManager) VariantName(idx int) string {
	if idx < 0 || idx >= len(m.variants) {
		return ""
	}
	return m.variants[idx].name
}

// VariantCount returns the number of metadata variants.
func (m *MetadataManager) VariantCount() int {
	return len(m.variants)
}

// DownloadArtwork downloads artwork for a game using the specified variant's
// thumbnail repo. Returns silently on any error (per spec).
func (m *MetadataManager) DownloadArtwork(gameCRC, gameName string, variantIdx int) {
	if gameName == "" || variantIdx < 0 || variantIdx >= len(m.variants) {
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

	thumbnailRepo := m.variants[variantIdx].thumbnailRepo

	// Replace & with _ and URL-encode the game name
	encodedName := url.PathEscape(strings.ReplaceAll(gameName, "&", "_"))

	// Try each artwork type in fallback order
	for _, artType := range artworkTypes {
		artURL := fmt.Sprintf("%s/%s/raw/refs/heads/master/%s/%s.png",
			thumbnailBaseURL, thumbnailRepo, artType, encodedName)

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

// DownloadRumble downloads the rumble CHT file for a game using the
// specified variant's RDB name. Returns silently on any error.
func (m *MetadataManager) DownloadRumble(gameCRC, gameName string, variantIdx int) {
	if gameName == "" || variantIdx < 0 || variantIdx >= len(m.variants) {
		return
	}

	// Check if rumble file already exists
	rumblePath, err := storage.GetGameRumblePath(gameCRC)
	if err != nil {
		return
	}
	if _, err := os.Stat(rumblePath); err == nil {
		return
	}

	rdbName := m.variants[variantIdx].rdbName

	// Strip parenthetical metadata from game name
	displayName := rdb.GetDisplayName(gameName)

	// Replace & with _ and URL-encode (same as artwork)
	encodedName := url.PathEscape(strings.ReplaceAll(displayName, "&", "_"))

	// Build URL: cht/{rdbName}/{name} (Rumbles).cht
	chtURL := fmt.Sprintf("%s/%s/%s (Rumbles).cht",
		chtBaseURL, url.PathEscape(rdbName), encodedName)

	data, err := downloadToMemory(chtURL)
	if err != nil {
		// Try title-cased variant for casing mismatches
		tcName := titleCase(displayName)
		if tcName != displayName {
			tcEncoded := url.PathEscape(strings.ReplaceAll(tcName, "&", "_"))
			tcURL := fmt.Sprintf("%s/%s/%s (Rumbles).cht",
				chtBaseURL, url.PathEscape(rdbName), tcEncoded)
			data, err = downloadToMemory(tcURL)
			if err != nil {
				return
			}
		} else {
			return
		}
	}

	if err := os.WriteFile(rumblePath, data, 0644); err != nil {
		return
	}
}

// titleCase capitalizes the first letter of each word in a string.
func titleCase(s string) string {
	prev := ' '
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(rune(prev)) || prev == '-' {
			prev = r
			return unicode.ToUpper(r)
		}
		prev = r
		return r
	}, s)
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
