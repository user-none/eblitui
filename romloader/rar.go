// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package romloader

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/nwaples/rardecode/v2"
)

// maxRARDictionary is the largest decode window a RAR entry may request.
// The window is allocated up front from the header, so an unbounded value
// lets a small archive request gigabytes. Non-solid entries are already
// capped at their unpacked size, which ROMs keep small; this covers solid
// archives made with common dictionary settings.
const maxRARDictionary = 64 * 1024 * 1024

// extractFromRAR extracts the first ROM file from a RAR archive
func extractFromRAR(path string, extensions []string) ([]byte, string, error) {
	r, err := rardecode.OpenReader(path, rardecode.MaxDictionarySize(maxRARDictionary))
	if err != nil {
		return nil, "", fmt.Errorf("failed to open rar: %w", err)
	}
	defer r.Close()

	for {
		header, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("failed to read rar entry: %w", err)
		}

		if header.IsDir {
			continue
		}
		if !isROMFile(header.Name, extensions) {
			continue
		}

		data, err := limitedRead(r)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read %s: %w", header.Name, err)
		}
		return data, filepath.Base(header.Name), nil
	}

	return nil, "", ErrNoFile
}
