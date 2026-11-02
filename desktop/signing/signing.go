// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package signing verifies detached signatures made with the project's
// signing keys. A signature file holds one signature per line, an
// algorithm name and a base64 signature separated by a space; data is
// trusted when any line is a valid signature by a trusted key. The update
// manifest and the artwork index use it.
package signing

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	// MaxSignatureSize is the largest signature file accepted.
	MaxSignatureSize = 4 * 1024

	// MaxSignatureLines is the most lines a signature file may have.
	MaxSignatureLines = 16
)

// Keys holds trusted signing public keys.
type Keys struct {
	ed25519 []ed25519.PublicKey
}

// ParseKeys parses public keys of the form algorithm:base64. Entries with an
// unknown algorithm or a malformed key are skipped and reported in the
// returned errors.
func ParseKeys(entries []string) (Keys, []error) {
	var keys Keys
	var errs []error
	for _, entry := range entries {
		alg, encoded, ok := strings.Cut(entry, ":")
		if !ok {
			errs = append(errs, fmt.Errorf("signing key %q: missing algorithm", entry))
			continue
		}
		switch alg {
		case "ed25519":
			key, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil || len(key) != ed25519.PublicKeySize {
				errs = append(errs, fmt.Errorf("signing key %q: invalid ed25519 key", entry))
				continue
			}
			keys.ed25519 = append(keys.ed25519, ed25519.PublicKey(key))
		default:
			errs = append(errs, fmt.Errorf("signing key %q: unknown algorithm", entry))
		}
	}
	return keys, errs
}

// Empty reports whether there are no usable keys.
func (k Keys) Empty() bool {
	return len(k.ed25519) == 0
}

// Verify reports whether any line of the signature file sig is a valid
// signature of data by one of keys. Lines with an unknown algorithm are
// skipped. A signature file over MaxSignatureSize or MaxSignatureLines is
// rejected.
func Verify(data, sig []byte, keys Keys) bool {
	if len(sig) > MaxSignatureSize {
		return false
	}
	lines := strings.Split(strings.TrimRight(string(sig), "\r\n"), "\n")
	if len(lines) > MaxSignatureLines {
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
				if ed25519.Verify(key, data, s) {
					return true
				}
			}
		}
	}
	return false
}
