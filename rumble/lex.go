// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package rumble

import (
	"fmt"
	"strconv"
	"strings"
)

type tokenKind int

const (
	tokIdent tokenKind = iota
	tokInt
	tokNumber
	tokDuration
	tokPercent
	tokLParen
	tokRParen
	tokComma
	tokColon
	tokSlash
	tokDotDot
	tokArrow
	tokCompare
)

type token struct {
	kind tokenKind
	text string
	ival uint64 // magnitude; neg records a leading minus sign
	fval float64
	hex  bool
	neg  bool
	line int
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isHexDigit(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// lexLine tokenizes one physical line. Comments are stripped by the
// caller before lexing.
func lexLine(s string, line int) ([]token, error) {
	var toks []token
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			i++
		case isIdentStart(c):
			j := i + 1
			for j < len(s) && isIdentPart(s[j]) {
				j++
			}
			toks = append(toks, token{kind: tokIdent, text: s[i:j], line: line})
			i = j
		case isDigit(c):
			tok, n, err := lexNumber(s[i:], line)
			if err != nil {
				return nil, err
			}
			toks = append(toks, tok)
			i += n
		default:
			rest := s[i:]
			switch {
			case strings.HasPrefix(rest, "=="), strings.HasPrefix(rest, "!="),
				strings.HasPrefix(rest, "<="), strings.HasPrefix(rest, ">="):
				toks = append(toks, token{kind: tokCompare, text: rest[:2], line: line})
				i += 2
			case strings.HasPrefix(rest, "->"):
				toks = append(toks, token{kind: tokArrow, text: "->", line: line})
				i += 2
			case strings.HasPrefix(rest, ".."):
				toks = append(toks, token{kind: tokDotDot, text: "..", line: line})
				i += 2
			case c == '-' && i+1 < len(s) && isDigit(s[i+1]):
				tok, n, err := lexNumber(s[i+1:], line)
				if err != nil {
					return nil, err
				}
				tok.neg = true
				tok.text = "-" + tok.text
				toks = append(toks, tok)
				i += n + 1
			case c == '<' || c == '>':
				toks = append(toks, token{kind: tokCompare, text: string(c), line: line})
				i++
			case c == '(':
				toks = append(toks, token{kind: tokLParen, text: "(", line: line})
				i++
			case c == ')':
				toks = append(toks, token{kind: tokRParen, text: ")", line: line})
				i++
			case c == ',':
				toks = append(toks, token{kind: tokComma, text: ",", line: line})
				i++
			case c == ':':
				toks = append(toks, token{kind: tokColon, text: ":", line: line})
				i++
			case c == '/':
				toks = append(toks, token{kind: tokSlash, text: "/", line: line})
				i++
			default:
				return nil, fmt.Errorf("line %d: unexpected character %q", line, string(c))
			}
		}
	}
	return toks, nil
}

// lexNumber scans an integer, decimal number, or duration at the start of
// s and returns the token and the number of bytes consumed. A '.' is part
// of the number only when it begins a fraction; ".." is left for the
// caller as a range operator.
func lexNumber(s string, line int) (token, int, error) {
	// The hex prefix is lowercase, like every keyword in the format.
	if strings.HasPrefix(s, "0x") {
		j := 2
		for j < len(s) && isHexDigit(s[j]) {
			j++
		}
		if j == 2 {
			return token{}, 0, fmt.Errorf("line %d: malformed number %q", line, s)
		}
		v, err := strconv.ParseUint(s[2:j], 16, 64)
		if err != nil {
			return token{}, 0, fmt.Errorf("line %d: malformed number %q", line, s[:j])
		}
		return finishInt(s, j, v, true, line)
	}

	j := 0
	for j < len(s) && isDigit(s[j]) {
		j++
	}
	if j < len(s) && s[j] == '.' && !strings.HasPrefix(s[j:], "..") {
		if j+1 >= len(s) || !isDigit(s[j+1]) {
			return token{}, 0, fmt.Errorf("line %d: malformed number %q", line, s[:j+1])
		}
		k := j + 1
		for k < len(s) && isDigit(s[k]) {
			k++
		}
		if k < len(s) && isIdentPart(s[k]) {
			return token{}, 0, malformedThrough(s, k, line)
		}
		f, err := strconv.ParseFloat(s[:k], 64)
		if err != nil {
			return token{}, 0, fmt.Errorf("line %d: malformed number %q", line, s[:k])
		}
		return token{kind: tokNumber, text: s[:k], fval: f, line: line}, k, nil
	}
	v, err := strconv.ParseUint(s[:j], 10, 64)
	if err != nil {
		return token{}, 0, fmt.Errorf("line %d: malformed number %q", line, s[:j])
	}
	return finishInt(s, j, v, false, line)
}

// finishInt handles the tail of an integer literal: a bare integer, an
// "ms" duration suffix, a "%" percent suffix, or trailing identifier
// characters (an error).
func finishInt(s string, j int, v uint64, hex bool, line int) (token, int, error) {
	if strings.HasPrefix(s[j:], "ms") && (j+2 >= len(s) || !isIdentPart(s[j+2])) {
		return token{kind: tokDuration, text: s[:j+2], ival: v, hex: hex, line: line}, j + 2, nil
	}
	if j < len(s) && s[j] == '%' {
		return token{kind: tokPercent, text: s[:j+1], ival: v, hex: hex, line: line}, j + 1, nil
	}
	if j < len(s) && isIdentPart(s[j]) {
		return token{}, 0, malformedThrough(s, j, line)
	}
	return token{kind: tokInt, text: s[:j], ival: v, fval: float64(v), hex: hex, line: line}, j, nil
}

// malformedThrough reports a malformed-number error covering the literal
// plus its trailing identifier characters.
func malformedThrough(s string, j int, line int) error {
	k := j
	for k < len(s) && isIdentPart(s[k]) {
		k++
	}
	return fmt.Errorf("line %d: malformed number %q", line, s[:k])
}
