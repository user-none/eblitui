// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package cheats

import (
	"reflect"
	"testing"
)

func TestDecodeSaturnAR(t *testing.T) {
	set16 := func(addr, value uint32) Op {
		return Op{Kind: OpSet, Space: SpaceBus, Addr: addr, Size: 2, Mask: 0xFF, Value: value, BigEndian: true}
	}
	set8 := func(addr, value uint32) Op {
		return Op{Kind: OpSet, Space: SpaceBus, Addr: addr, Size: 1, Mask: 0xFF, Value: value}
	}
	ifEq := func(addr, value uint32) Op {
		return Op{Kind: OpIfEqual, Space: SpaceBus, Addr: addr, Size: 2, Mask: 0xFF, Value: value, BigEndian: true}
	}

	tests := []struct {
		code string
		want []Op
	}{
		{"160FFC12+0000", []Op{set16(0x060FFC12, 0)}},
		{"160FFE68+0063+160FFEB8+0006", []Op{set16(0x060FFE68, 0x63), set16(0x060FFEB8, 6)}},
		{"360DBD38+0005", []Op{set8(0x060DBD38, 5)}},
		{"D604A8C4+0001+1604A8C6+0063", []Op{ifEq(0x0604A8C4, 1), set16(0x0604A8C6, 0x63)}},
		// Master code parts are dropped
		{"F6000914+C305+160FFC12+0000", []Op{set16(0x060FFC12, 0)}},
		{"F6000924+FFFF+B6002800+0000", nil},
		// Separators: spaces, lower case, and 12 digits together
		{"160ffc12 0000", []Op{set16(0x060FFC12, 0)}},
		{"1602e44a0000", []Op{set16(0x0602E44A, 0)}},
		{"102457120000+160252b80009", []Op{set16(0x00245712, 0), set16(0x060252B8, 9)}},
	}
	for _, tc := range tests {
		got, ok, err := decodeSaturnAR(tc.code)
		if !ok || err != nil {
			t.Errorf("%q: ok %v err %v", tc.code, ok, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q:\n got  %+v\n want %+v", tc.code, got, tc.want)
		}
	}
}

func TestDecodeSaturnARNotAR(t *testing.T) {
	for _, code := range []string{
		"",
		"06023542+????",
		"160067C92+0064",
		"1601dba4",
		"160FFC12+00",
		"ABCD-EFGH",
	} {
		if _, ok, _ := decodeSaturnAR(code); ok {
			t.Errorf("%q: expected not recognized", code)
		}
	}
}

func TestDecodeSaturnARInvalid(t *testing.T) {
	for _, code := range []string{
		"0601edce0009",                // unknown type 0
		"6600A000+0001",               // unknown type 6
		"360DBD38+0105",               // 8-bit write with a high byte
		"D604A8C4+0001",               // compare with nothing after it
		"D604A8C4+0001+F6000914+C305", // compare followed only by a master code
	} {
		ops, ok, err := decodeSaturnAR(code)
		if !ok || err == nil || ops != nil {
			t.Errorf("%q: ok %v err %v ops %v, want an error", code, ok, err, ops)
		}
	}
}
