// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	// MaxManifestSize is the largest manifest.json accepted.
	MaxManifestSize = 64 * 1024

	// MaxSignatureSize is the largest manifest.sig accepted.
	MaxSignatureSize = 4 * 1024

	// maxSignatureLines is the most lines manifest.sig may have.
	maxSignatureLines = 16

	// manifestSchema is the manifest format version this build reads.
	manifestSchema = 1
)

// Manifest is a release's update manifest.
type Manifest struct {
	Schema  int     `json:"schema"`
	App     string  `json:"app"`
	Version string  `json:"version"`
	Assets  []Asset `json:"assets"`
}

// Asset is one platform's release zip.
type Asset struct {
	Platform string `json:"platform"`
	File     string `json:"file"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
	Binary   Binary `json:"binary"`
}

// Binary is the application binary inside a release zip.
type Binary struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Status is the result of checking a manifest.
type Status int

const (
	// Invalid means the manifest failed verification.
	Invalid Status = iota
	// NoUpdate means the manifest verified but does not offer a newer
	// version for the running platform.
	NoUpdate
	// Available means the manifest offers a newer version for the running
	// platform.
	Available
)

// Target is the running application a manifest is checked against.
type Target struct {
	App      string
	Version  Version
	Platform string
	Keys     Keys
}

// Check verifies a manifest and its signature file and reports whether it
// offers an update. The returned manifest is set only when an update is
// available. The error is set only for Invalid and gives the reason.
func (t *Target) Check(manifest, sig []byte) (Status, *Manifest, error) {
	if len(manifest) > MaxManifestSize {
		return Invalid, nil, errors.New("manifest too large")
	}
	if !verifySignature(manifest, sig, t.Keys) {
		return Invalid, nil, errors.New("no valid signature")
	}

	var m Manifest
	if err := json.Unmarshal(manifest, &m); err != nil {
		return Invalid, nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Schema != manifestSchema {
		return Invalid, nil, fmt.Errorf("unknown manifest schema %d", m.Schema)
	}
	if m.App != t.App {
		return Invalid, nil, fmt.Errorf("manifest is for %q", m.App)
	}
	version, ok := ParseVersion(m.Version)
	if !ok {
		return Invalid, nil, fmt.Errorf("invalid manifest version %q", m.Version)
	}

	if !t.Version.Less(version) {
		return NoUpdate, nil, nil
	}
	if t.Platform == "" || m.Asset(t.Platform) == nil {
		return NoUpdate, nil, nil
	}
	return Available, &m, nil
}

// Asset returns the asset for a platform, or nil if there is none.
func (m *Manifest) Asset(platform string) *Asset {
	for i := range m.Assets {
		if m.Assets[i].Platform == platform {
			return &m.Assets[i]
		}
	}
	return nil
}

// verifySignature reports whether any line of the signature file is a valid
// signature of manifest by a trusted key. Each line is an algorithm name and
// a base64 signature separated by a space. Lines with an unknown algorithm
// are skipped.
func verifySignature(manifest, sig []byte, keys Keys) bool {
	if len(sig) > MaxSignatureSize {
		return false
	}
	lines := strings.Split(strings.TrimRight(string(sig), "\r\n"), "\n")
	if len(lines) > maxSignatureLines {
		return false
	}

	for _, line := range lines {
		alg, encoded, ok := strings.Cut(strings.TrimSuffix(line, "\r"), " ")
		if !ok {
			continue
		}
		switch alg {
		case "ed25519":
			s, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil || len(s) != ed25519.SignatureSize {
				continue
			}
			for _, key := range keys.ed25519 {
				if ed25519.Verify(key, manifest, s) {
					return true
				}
			}
		}
	}
	return false
}
