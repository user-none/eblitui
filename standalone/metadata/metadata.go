package metadata

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/rdb"
	"github.com/user-none/eblitui/standalone/netutil"
	"github.com/user-none/eblitui/standalone/storage"
)

const (
	// Base URL for libretro-database RDB files
	rdbBaseURL = "https://github.com/libretro/libretro-database/raw/refs/heads/master/rdb"
)

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
func NewMetadataManager(variants []coreif.MetadataVariant) *MetadataManager {
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
	data, err := netutil.DownloadToMemory(rdbURL)
	if err != nil {
		return fmt.Errorf("failed to download RDB: %w", err)
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

// VariantThumbnailRepo returns the thumbnail repo name for a variant index.
func (m *MetadataManager) VariantThumbnailRepo(idx int) string {
	if idx < 0 || idx >= len(m.variants) {
		return ""
	}
	return m.variants[idx].thumbnailRepo
}

// VariantRDBName returns the RDB name for a variant index.
func (m *MetadataManager) VariantRDBName(idx int) string {
	if idx < 0 || idx >= len(m.variants) {
		return ""
	}
	return m.variants[idx].rdbName
}
