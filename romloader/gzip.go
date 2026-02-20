package romloader

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// extractFromGzip extracts the first ROM file from a gzip or tar.gz archive
func extractFromGzip(path string, extensions []string) ([]byte, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open gzip: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	// Check if this is a tar.gz or just a .gz
	lowerPath := strings.ToLower(path)
	if strings.HasSuffix(lowerPath, ".tar.gz") || strings.HasSuffix(lowerPath, ".tgz") {
		return extractFromTar(gr, extensions)
	}

	// Plain .gz file - assume the decompressed content is the ROM
	// Use the base name without .gz extension
	data, err := limitedRead(gr)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decompress gzip: %w", err)
	}

	name := filepath.Base(path)
	if strings.HasSuffix(strings.ToLower(name), ".gz") {
		name = name[:len(name)-3]
	}
	return data, name, nil
}

// extractFromTar extracts the first ROM file from a tar archive
func extractFromTar(r io.Reader, extensions []string) ([]byte, string, error) {
	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("failed to read tar entry: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}
		if !isROMFile(header.Name, extensions) {
			continue
		}

		data, err := limitedRead(tr)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read %s from tar: %w", header.Name, err)
		}
		return data, filepath.Base(header.Name), nil
	}

	return nil, "", ErrNoROMFile
}
