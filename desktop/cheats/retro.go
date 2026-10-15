// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package cheats

import (
	"errors"
	"fmt"
	"strconv"
)

// maxRepeat caps repeat_count so one entry cannot expand into an
// unbounded number of ops.
const maxRepeat = 4096

// retroBigEndian is the byte order of the memory RetroArch format
// addresses refer to: the system's memory as RetroArch exposes it, with
// 16-bit words stored little-endian. An entry's big_endian field is only
// the byte order of its value within that memory.
const retroBigEndian = false

// errDisabled is returned for RetroArch format entries with cheat_type 0.
var errDisabled = errors.New("entry is disabled")

// RetroArch format cheat_type values.
const (
	retroTypeDisabled = 0
	retroTypeSet      = 1
	retroTypeAdd      = 2
	retroTypeSub      = 3
	retroTypeIfEqual  = 4
	retroTypeIfGT     = 7
)

// parseRetroEntry parses a RetroArch format entry (handler 1). Its fields:
//
//	address: flat memory offset, in memory stored in retroBigEndian
//	  order
//	value: value to set, add, or subtract
//	memory_search_size: value width as 2^n bits, 0-5 for 1-32 bits
//	cheat_type: 0 disabled, 1 set, 2 add, 3 subtract, 4-7 run the next
//	  entry only if the current value is equal, not equal, less than, or
//	  greater than the value
//	address_bit_position: bit mask of the bits written for widths under
//	  8 bits
//	big_endian: byte order of 16 and 32-bit values; little-endian when
//	  not set
//	repeat_count, repeat_add_to_value, repeat_add_to_address: apply the
//	  entry to successive addresses, adding to the value each time
//
// Entries with cheat_type 0 carry only rumble settings and return
// errDisabled. Conditional types gate the following entry rather than an
// op in the same cheat, which a Cheat cannot express, so they are
// reported as unsupported.
func parseRetroEntry(e entry) (Cheat, error) {
	fields, err := retroFields(e)
	if err != nil {
		return Cheat{}, err
	}
	if fields.cheatType == retroTypeDisabled {
		return Cheat{}, errDisabled
	}
	ops, err := retroOps(fields)
	if err != nil {
		return Cheat{}, err
	}

	id := fmt.Sprintf("ra:%d:%d:%d:%d:%d:%t:%d:%d:%d",
		fields.address, fields.searchSize, fields.cheatType, fields.value,
		fields.mask, fields.bigEndian, fields.repeatCount,
		fields.repeatAddValue, fields.repeatAddAddress)
	return Cheat{Name: e["desc"], ID: id, Ops: ops}, nil
}

// retroEntry holds a RetroArch format entry's numeric fields.
type retroEntry struct {
	address          uint32
	value            uint32
	searchSize       uint32
	cheatType        uint32
	mask             uint32
	bigEndian        bool
	repeatCount      uint32
	repeatAddValue   uint32
	repeatAddAddress uint32
}

// retroFields reads the numeric fields of a RetroArch format entry.
// address, memory_search_size, and cheat_type are required.
func retroFields(e entry) (retroEntry, error) {
	f := retroEntry{repeatCount: 1}
	required := []struct {
		key string
		dst *uint32
	}{
		{"address", &f.address},
		{"memory_search_size", &f.searchSize},
		{"cheat_type", &f.cheatType},
	}
	for _, r := range required {
		v, ok := e[r.key]
		if !ok {
			return f, fmt.Errorf("missing %s", r.key)
		}
		n, err := strconv.ParseUint(v, 0, 32)
		if err != nil {
			return f, fmt.Errorf("invalid %s %q", r.key, v)
		}
		*r.dst = uint32(n)
	}

	optional := []struct {
		key string
		dst *uint32
	}{
		{"value", &f.value},
		{"address_bit_position", &f.mask},
		{"repeat_count", &f.repeatCount},
		{"repeat_add_to_value", &f.repeatAddValue},
		{"repeat_add_to_address", &f.repeatAddAddress},
	}
	for _, o := range optional {
		v, ok := e[o.key]
		if !ok {
			continue
		}
		n, err := strconv.ParseUint(v, 0, 32)
		if err != nil {
			return f, fmt.Errorf("invalid %s %q", o.key, v)
		}
		*o.dst = uint32(n)
	}

	f.bigEndian = e["big_endian"] == "true" || e["big_endian"] == "1"
	return f, nil
}

// retroOps converts a RetroArch format entry into ops, expanding repeats
// into one op per address.
func retroOps(f retroEntry) ([]Op, error) {
	if f.searchSize > 5 {
		return nil, fmt.Errorf("invalid memory_search_size %d", f.searchSize)
	}
	bits := uint32(1) << f.searchSize

	var kind OpKind
	switch f.cheatType {
	case retroTypeSet:
		kind = OpSet
	case retroTypeAdd:
		kind = OpAdd
	case retroTypeSub:
		kind = OpSub
	default:
		if f.cheatType >= retroTypeIfEqual && f.cheatType <= retroTypeIfGT {
			return nil, errors.New("conditional entries are not supported")
		}
		return nil, fmt.Errorf("invalid cheat_type %d", f.cheatType)
	}

	size := 1
	mask := uint8(0xFF)
	switch {
	case bits < 8:
		if f.mask == 0 || f.mask > 0xFF {
			return nil, fmt.Errorf("invalid address_bit_position %d for a %d-bit value", f.mask, bits)
		}
		mask = uint8(f.mask)
		if f.value > 0xFF {
			return nil, fmt.Errorf("value %d does not fit in a byte", f.value)
		}
	case bits == 16:
		size = 2
	case bits == 32:
		size = 4
	}
	if bits == 8 || bits == 16 {
		if f.value >= 1<<bits {
			return nil, fmt.Errorf("value %d does not fit in %d bits", f.value, bits)
		}
	}

	if f.repeatCount == 0 {
		return nil, errors.New("repeat_count is 0")
	}
	if f.repeatCount > maxRepeat {
		return nil, fmt.Errorf("repeat_count %d exceeds %d", f.repeatCount, maxRepeat)
	}
	if f.repeatCount > 1 {
		if kind != OpSet {
			return nil, errors.New("repeats are only supported for set entries")
		}
		if bits < 8 {
			return nil, errors.New("repeats are not supported for values under 8 bits")
		}
	}

	valueMask := uint32(0xFFFFFFFF)
	if bits < 32 {
		valueMask = 1<<bits - 1
	}

	ops := make([]Op, 0, f.repeatCount)
	addr := f.address
	value := f.value
	for i := uint32(0); i < f.repeatCount; i++ {
		ops = append(ops, Op{
			Kind:      kind,
			Space:     SpaceFlat,
			Addr:      addr,
			Size:      size,
			Mask:      mask,
			Value:     value,
			BigEndian: f.bigEndian,
		})
		addr += f.repeatAddAddress * uint32(size)
		value = (value + f.repeatAddValue) & valueMask
	}
	return ops, nil
}
