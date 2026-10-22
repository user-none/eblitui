// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"
)

// Keys holds the trusted update signing public keys.
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
			errs = append(errs, fmt.Errorf("update key %q: missing algorithm", entry))
			continue
		}
		switch alg {
		case "ed25519":
			key, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil || len(key) != ed25519.PublicKeySize {
				errs = append(errs, fmt.Errorf("update key %q: invalid ed25519 key", entry))
				continue
			}
			keys.ed25519 = append(keys.ed25519, ed25519.PublicKey(key))
		default:
			errs = append(errs, fmt.Errorf("update key %q: unknown algorithm", entry))
		}
	}
	return keys, errs
}

// Empty reports whether there are no usable keys.
func (k Keys) Empty() bool {
	return len(k.ed25519) == 0
}
