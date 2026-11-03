// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

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

// maxArchiveRead is the most bytes decompressed from one tar.gz stream,
// including entries skipped while searching for a ROM. Skipping an entry
// still decompresses all of it, so without a limit a small archive can
// hold a huge filler entry that takes minutes to get past.
const maxArchiveRead = 64 * 1024 * 1024

// limitReader reads from r and returns ErrArchiveTooLarge once more than
// maxArchiveRead bytes have been read.
type limitReader struct {
	r         io.Reader
	remaining int64
}

func newLimitReader(r io.Reader) *limitReader {
	return &limitReader{r: r, remaining: maxArchiveRead + 1}
}

func (l *limitReader) Read(p []byte) (int, error) {
	if l.remaining <= 0 {
		return 0, ErrArchiveTooLarge
	}
	if int64(len(p)) > l.remaining {
		p = p[:l.remaining]
	}
	n, err := l.r.Read(p)
	l.remaining -= int64(n)
	if l.remaining <= 0 {
		return n, ErrArchiveTooLarge
	}
	return n, err
}

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
		return extractFromTar(newLimitReader(gr), extensions)
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

	return nil, "", ErrNoFile
}
