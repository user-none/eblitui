package romloader

import (
	"archive/zip"
	"fmt"
	"path/filepath"
)

// extractFromZIP extracts the first ROM file from a ZIP archive
func extractFromZIP(path string, extensions []string) ([]byte, string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if !isROMFile(f.Name, extensions) {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, "", fmt.Errorf("failed to open %s in archive: %w", f.Name, err)
		}
		defer rc.Close()

		data, err := limitedRead(rc)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read %s: %w", f.Name, err)
		}
		return data, filepath.Base(f.Name), nil
	}

	return nil, "", ErrNoROMFile
}
