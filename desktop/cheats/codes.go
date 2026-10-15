// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package cheats

import (
	"errors"
	"strings"
	"unicode"
)

// codeDecoder decodes one code string format. ok is false when the code
// is not in the decoder's format. A code in the format that decodes to no
// ops, such as a master code, returns ok with nil ops and nil error.
type codeDecoder func(code string) (ops []Op, ok bool, err error)

// codeFormat is a code string format: its decoder and the byte order of
// the memory its addresses refer to.
type codeFormat struct {
	decode    codeDecoder
	bigEndian bool
}

// systemFormats lists the code string formats for each system.
var systemFormats = map[System][]codeFormat{
	SystemSaturn: {{decode: decodeSaturnAR, bigEndian: true}},
}

var errUnsupportedFormat = errors.New("unsupported code format")

// decodeCode decodes a code string with the first of the system's formats
// that recognizes it. bigEndian is the byte order of the memory the
// format's addresses refer to.
func decodeCode(code string, sys System) (ops []Op, bigEndian bool, err error) {
	for _, f := range systemFormats[sys] {
		if ops, ok, err := f.decode(code); ok {
			return ops, f.bigEndian, err
		}
	}
	return nil, false, errUnsupportedFormat
}

var (
	errNoCode = errors.New("no code")
	// errNoOps is returned for codes that decode to no ops: codes made
	// only of master codes, which are not supported.
	errNoOps = errors.New("code contains only master codes")
)

// ParseCode parses one cheat's code string, such as Action Replay codes
// separated by "+", spaces, or new lines. sys selects the decoders.
// coreBigEndian is the byte order of the core's memory; the ops are
// converted to it. The returned cheat has Code and ID set and no name.
// When the code cannot be applied, the error gives the reason.
func ParseCode(code string, sys System, coreBigEndian bool) (Cheat, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return Cheat{}, errNoCode
	}
	ops, bigEndian, err := decodeCode(code, sys)
	if err != nil {
		return Cheat{}, err
	}
	if len(ops) == 0 {
		return Cheat{}, errNoOps
	}
	if err := toByteOrder(ops, bigEndian, coreBigEndian); err != nil {
		return Cheat{}, err
	}
	return Cheat{Code: code, ID: codeID(code), Ops: ops}, nil
}

// codeID normalizes a code string for use as a cheat ID: upper case with
// whitespace and "+" separators removed, so the same code written with
// different separators has the same ID.
func codeID(code string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '+' {
			return -1
		}
		return unicode.ToUpper(r)
	}, code)
}
