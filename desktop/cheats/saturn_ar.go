// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package cheats

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// arPair is one address/value pair of an Action Replay code.
type arPair struct {
	addr  uint32
	value uint32
}

// decodeSaturnAR decodes Saturn Action Replay codes: pairs of an 8 hex
// digit address and a 4 hex digit value, joined by "+" or spaces, or
// written together as 12 digits. The first address digit is the code type
// and the other seven are the bus address:
//
//	1: write the 16-bit value
//	3: write the low byte of the value; the high byte must be 00
//	D: run the next code only if the 16-bit value at the address equals
//	   the value
//	F, B: master codes, which the cartridge uses to hook the game; dropped
//
// ok is false when the code is not made of address/value pairs.
func decodeSaturnAR(code string) ([]Op, bool, error) {
	pairs, ok := splitARPairs(code)
	if !ok {
		return nil, false, nil
	}

	var ops []Op
	for _, p := range pairs {
		addr := p.addr & 0x0FFFFFFF
		switch typ := p.addr >> 28; typ {
		case 0x1:
			ops = append(ops, Op{Kind: OpSet, Space: SpaceBus, Addr: addr, Size: 2, Mask: 0xFF, Value: p.value, BigEndian: true})
		case 0x3:
			if p.value > 0xFF {
				return nil, true, fmt.Errorf("8-bit write value %04X has a non-zero high byte", p.value)
			}
			ops = append(ops, Op{Kind: OpSet, Space: SpaceBus, Addr: addr, Size: 1, Mask: 0xFF, Value: p.value})
		case 0xD:
			ops = append(ops, Op{Kind: OpIfEqual, Space: SpaceBus, Addr: addr, Size: 2, Mask: 0xFF, Value: p.value, BigEndian: true})
		case 0xF, 0xB:
			continue
		default:
			return nil, true, fmt.Errorf("unknown code type %X", typ)
		}
	}

	if n := len(ops); n > 0 && ops[n-1].Kind == OpIfEqual {
		return nil, true, errors.New("compare code has no code after it")
	}
	return ops, true, nil
}

// splitARPairs splits a code into address/value pairs. ok is false when
// any part is not an 8 digit address followed by a 4 digit value.
func splitARPairs(code string) ([]arPair, bool) {
	tokens := strings.FieldsFunc(code, func(r rune) bool {
		return r == '+' || unicode.IsSpace(r)
	})
	if len(tokens) == 0 {
		return nil, false
	}

	var pairs []arPair
	for i := 0; i < len(tokens); i++ {
		var addrStr, valueStr string
		switch {
		case len(tokens[i]) == 12:
			addrStr, valueStr = tokens[i][:8], tokens[i][8:]
		case len(tokens[i]) == 8 && i+1 < len(tokens) && len(tokens[i+1]) == 4:
			addrStr, valueStr = tokens[i], tokens[i+1]
			i++
		default:
			return nil, false
		}

		addr, err := strconv.ParseUint(addrStr, 16, 32)
		if err != nil {
			return nil, false
		}
		value, err := strconv.ParseUint(valueStr, 16, 16)
		if err != nil {
			return nil, false
		}
		pairs = append(pairs, arPair{addr: uint32(addr), value: uint32(value)})
	}
	return pairs, true
}
