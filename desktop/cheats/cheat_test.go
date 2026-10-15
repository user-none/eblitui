// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package cheats

import (
	"reflect"
	"testing"
)

func TestToByteOrder(t *testing.T) {
	op := func(addr uint32, size int, bigEndian bool) Op {
		return Op{Kind: OpSet, Addr: addr, Size: size, Mask: 0xFF, BigEndian: bigEndian}
	}
	tests := []struct {
		name     string
		in       []Op
		from, to bool
		want     []Op
	}{
		{
			"little to big: 1-byte flips bit 0, 16-bit value order reversed",
			[]Op{op(0x1BCF12, 1, false), op(0x1BCF13, 1, true), op(0x155852, 2, false), op(0x155854, 2, true)},
			false, true,
			[]Op{op(0x1BCF13, 1, false), op(0x1BCF12, 1, true), op(0x155852, 2, true), op(0x155854, 2, false)},
		},
		{
			"big to little: 1-byte flips bit 0, 16-bit value order reversed",
			[]Op{op(0x06001000, 1, false), op(0x06001002, 2, true)},
			true, false,
			[]Op{op(0x06001001, 1, false), op(0x06001002, 2, false)},
		},
		{
			"same order: unchanged, including odd 16-bit and 32-bit",
			[]Op{op(0x10, 1, true), op(0x11, 2, false), op(0x20, 4, true)},
			false, false,
			[]Op{op(0x10, 1, true), op(0x11, 2, false), op(0x20, 4, true)},
		},
	}
	for _, tc := range tests {
		if err := toByteOrder(tc.in, tc.from, tc.to); err != nil {
			t.Errorf("%s: err %v", tc.name, err)
			continue
		}
		if !reflect.DeepEqual(tc.in, tc.want) {
			t.Errorf("%s:\n got  %+v\n want %+v", tc.name, tc.in, tc.want)
		}
	}
}

func TestToByteOrderMask(t *testing.T) {
	ops := []Op{{Kind: OpSet, Addr: 0x10, Size: 1, Mask: 0x0F, Value: 3}}
	if err := toByteOrder(ops, false, true); err != nil {
		t.Fatalf("err %v", err)
	}
	if ops[0].Addr != 0x11 || ops[0].Mask != 0x0F || ops[0].Value != 3 {
		t.Errorf("got %+v, want address 0x11 with mask and value kept", ops[0])
	}
}

func TestToByteOrderCannotSwap(t *testing.T) {
	for name, op := range map[string]Op{
		"32-bit":            {Size: 4},
		"32-bit big-endian": {Addr: 0, Size: 4, BigEndian: true},
		"odd 16-bit":        {Addr: 0x101, Size: 2},
		"odd at large":      {Addr: 0x1FFFFF, Size: 2, BigEndian: true},
	} {
		if err := toByteOrder([]Op{op}, false, true); err == nil {
			t.Errorf("%s: converted, want an error", name)
		}
	}
}
