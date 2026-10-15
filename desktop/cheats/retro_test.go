// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package cheats

import (
	"reflect"
	"testing"
)

func retroEntryOf(fields map[string]string) entry {
	e := entry{"handler": "1", "desc": "Test"}
	for k, v := range fields {
		e[k] = v
	}
	return e
}

func TestParseRetroEntry(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]string
		want   []Op
	}{
		{
			"8-bit set",
			map[string]string{"address": "2033760", "memory_search_size": "3", "cheat_type": "1", "value": "9", "address_bit_position": "255"},
			[]Op{{Kind: OpSet, Space: SpaceFlat, Addr: 2033760, Size: 1, Mask: 0xFF, Value: 9}},
		},
		{
			"4-bit set uses the bit mask",
			map[string]string{"address": "1822482", "memory_search_size": "2", "cheat_type": "1", "value": "3", "address_bit_position": "15"},
			[]Op{{Kind: OpSet, Space: SpaceFlat, Addr: 1822482, Size: 1, Mask: 0x0F, Value: 3}},
		},
		{
			"16-bit set, mask ignored",
			map[string]string{"address": "1398610", "memory_search_size": "4", "cheat_type": "1", "value": "65535", "address_bit_position": "0", "big_endian": "true"},
			[]Op{{Kind: OpSet, Space: SpaceFlat, Addr: 1398610, Size: 2, Mask: 0xFF, Value: 0xFFFF, BigEndian: true}},
		},
		{
			"32-bit add, hex fields",
			map[string]string{"address": "0x100", "memory_search_size": "5", "cheat_type": "2", "value": "0x10"},
			[]Op{{Kind: OpAdd, Space: SpaceFlat, Addr: 0x100, Size: 4, Mask: 0xFF, Value: 0x10}},
		},
		{
			"8-bit subtract",
			map[string]string{"address": "16", "memory_search_size": "3", "cheat_type": "3", "value": "1"},
			[]Op{{Kind: OpSub, Space: SpaceFlat, Addr: 16, Size: 1, Mask: 0xFF, Value: 1}},
		},
		{
			"16-bit repeat",
			map[string]string{"address": "100", "memory_search_size": "4", "cheat_type": "1", "value": "65534",
				"repeat_count": "3", "repeat_add_to_value": "1", "repeat_add_to_address": "2"},
			[]Op{
				{Kind: OpSet, Space: SpaceFlat, Addr: 100, Size: 2, Mask: 0xFF, Value: 0xFFFE},
				{Kind: OpSet, Space: SpaceFlat, Addr: 104, Size: 2, Mask: 0xFF, Value: 0xFFFF},
				{Kind: OpSet, Space: SpaceFlat, Addr: 108, Size: 2, Mask: 0xFF, Value: 0},
			},
		},
	}
	for _, tc := range tests {
		c, err := parseRetroEntry(retroEntryOf(tc.fields))
		if err != nil {
			t.Errorf("%s: err %v", tc.name, err)
			continue
		}
		if !reflect.DeepEqual(c.Ops, tc.want) {
			t.Errorf("%s:\n got  %+v\n want %+v", tc.name, c.Ops, tc.want)
		}
	}
}

func TestParseRetroEntryDisabled(t *testing.T) {
	_, err := parseRetroEntry(retroEntryOf(map[string]string{
		"address": "1", "memory_search_size": "3", "cheat_type": "0", "rumble_type": "4",
	}))
	if err != errDisabled {
		t.Errorf("cheat_type 0: err %v, want %v", err, errDisabled)
	}
}

func TestParseRetroEntryInvalid(t *testing.T) {
	tests := map[string]map[string]string{
		"missing address":       {"memory_search_size": "3", "cheat_type": "1"},
		"missing size":          {"address": "1", "cheat_type": "1"},
		"bad address":           {"address": "x", "memory_search_size": "3", "cheat_type": "1"},
		"size out of range":     {"address": "1", "memory_search_size": "6", "cheat_type": "1"},
		"conditional":           {"address": "1", "memory_search_size": "3", "cheat_type": "4"},
		"unknown type":          {"address": "1", "memory_search_size": "3", "cheat_type": "9"},
		"sub-byte without mask": {"address": "1", "memory_search_size": "2", "cheat_type": "1", "address_bit_position": "0"},
		"8-bit value too large": {"address": "1", "memory_search_size": "3", "cheat_type": "1", "value": "256"},
		"repeat count 0":        {"address": "1", "memory_search_size": "3", "cheat_type": "1", "repeat_count": "0"},
		"repeat count too high": {"address": "1", "memory_search_size": "3", "cheat_type": "1", "repeat_count": "4097"},
		"repeat on add":         {"address": "1", "memory_search_size": "3", "cheat_type": "2", "repeat_count": "2"},
		"repeat under 8 bits":   {"address": "1", "memory_search_size": "2", "cheat_type": "1", "address_bit_position": "15", "repeat_count": "2"},
	}
	for name, fields := range tests {
		if _, err := parseRetroEntry(retroEntryOf(fields)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestParseRetroEntryID(t *testing.T) {
	a, _ := parseRetroEntry(retroEntryOf(map[string]string{"address": "16", "memory_search_size": "3", "cheat_type": "1", "value": "5"}))
	b, _ := parseRetroEntry(retroEntryOf(map[string]string{"address": "16", "memory_search_size": "3", "cheat_type": "1", "value": "6"}))
	if a.ID == "" || a.ID == b.ID {
		t.Errorf("IDs should differ by value: %q %q", a.ID, b.ID)
	}
}
