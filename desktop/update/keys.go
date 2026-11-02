// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package update

import "github.com/user-none/eblitui/desktop/signing"

// Keys holds the trusted update signing public keys.
type Keys = signing.Keys

// ParseKeys parses public keys of the form algorithm:base64. See
// signing.ParseKeys.
func ParseKeys(entries []string) (Keys, []error) {
	return signing.ParseKeys(entries)
}
