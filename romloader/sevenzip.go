package romloader

import (
	"fmt"
	"path/filepath"

	"github.com/bodgit/sevenzip"
)

// extractFrom7z extracts the first ROM file from a 7z archive
func extractFrom7z(path string, extensions []string) ([]byte, string, error) {
	r, err := sevenzip.OpenReader(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open 7z: %w", err)
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
