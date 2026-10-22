// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

const (
	// updateStateVersion is the current update.json format version
	updateStateVersion = 1

	updateStateFile    = "update.json"
	updateManifestFile = "manifest.json"
	updateSigFile      = "manifest.sig"
)

// LoadUpdateState loads update/update.json. If the file doesn't exist or
// can't be read, it returns an empty state, and the next save overwrites the
// file. Losing it only costs an extra check.
func LoadUpdateState() (*UpdateState, error) {
	dir, err := GetUpdateDir()
	if err != nil {
		return nil, err
	}

	state := &UpdateState{}
	if err := ReadJSON(filepath.Join(dir, updateStateFile), state); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("Ignoring unreadable update state file: %v", err)
		}
		state = &UpdateState{Version: updateStateVersion}
	}
	return state, nil
}

// SaveUpdateState saves update/update.json atomically
func SaveUpdateState(state *UpdateState) error {
	dir, err := GetUpdateDir()
	if err != nil {
		return err
	}

	state.Version = updateStateVersion
	return AtomicWriteJSON(filepath.Join(dir, updateStateFile), state)
}

// LoadUpdateManifest reads the stored update manifest and its signature file
// byte for byte. It returns an error when either is missing, unreadable, or
// larger than its limit.
func LoadUpdateManifest(maxManifest, maxSig int64) (manifest, sig []byte, err error) {
	dir, err := GetUpdateDir()
	if err != nil {
		return nil, nil, err
	}

	manifest, err = readFileLimited(filepath.Join(dir, updateManifestFile), maxManifest)
	if err != nil {
		return nil, nil, err
	}
	sig, err = readFileLimited(filepath.Join(dir, updateSigFile), maxSig)
	if err != nil {
		return nil, nil, err
	}
	return manifest, sig, nil
}

// SaveUpdateManifest stores an update manifest and its signature file byte
// for byte. Each file is written atomically. If only one is replaced the
// pair no longer verifies and is treated as not stored.
func SaveUpdateManifest(manifest, sig []byte) error {
	dir, err := GetUpdateDir()
	if err != nil {
		return err
	}

	if err := AtomicWriteFile(filepath.Join(dir, updateManifestFile), manifest); err != nil {
		return err
	}
	return AtomicWriteFile(filepath.Join(dir, updateSigFile), sig)
}

// readFileLimited reads a file, failing if it is larger than limit bytes
func readFileLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, limit)
	}
	return data, nil
}
