// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package rumble parses and validates rumble files, which drive gamepad
// vibration from emulated game memory. FORMAT.md specifies the file
// format and its semantics.
package rumble

// Parse parses and validates rumble file source. Address validation
// against a system's memory regions happens in NewEngine, when the
// ruleset is bound to a running system.
func Parse(src []byte) (*Ruleset, error) {
	rs, err := parseSource(src)
	if err != nil {
		return nil, err
	}
	if err := validateRuleset(rs); err != nil {
		return nil, err
	}
	return rs, nil
}
