// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package cheats

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestReadEntries(t *testing.T) {
	src := `cheats = 2

# comment
cheat0_desc = "Lives = 99"
cheat0_code = "160FFE68+0063"
cheat0_enable = true
cheat1_desc = "Second"
cheat5_desc = "Outside the count"
cheatX_desc = "Bad number"
not a key value line
`
	entries, err := readEntries(strings.NewReader(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if got := entries[0]["desc"]; got != "Lives = 99" {
		t.Errorf("desc = %q, want %q", got, "Lives = 99")
	}
	if got := entries[0]["code"]; got != "160FFE68+0063" {
		t.Errorf("code = %q", got)
	}
	if got := entries[1]["desc"]; got != "Second" {
		t.Errorf("second desc = %q", got)
	}
}

func TestReadEntriesErrors(t *testing.T) {
	for name, src := range map[string]string{
		"missing count":  `cheat0_desc = "x"`,
		"invalid count":  `cheats = many`,
		"negative count": `cheats = -1`,
		"count too high": `cheats = 10001`,
	} {
		if _, err := readEntries(strings.NewReader(src)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

const mixedFile = `cheats = 6
cheat0_desc = "Master Code"
cheat0_code = "F6000924+FFFF+B6002800+0000"
cheat0_enable = false
cheat1_desc = "Infinite Crystals"
cheat1_code = "160FFE68+0063+360FFEB9+0006"
cheat1_enable = true
cheat2_desc = "Broken"
cheat2_code = "06023542+????"
cheat3_desc = "Infinite lives."
cheat3_handler = "1"
cheat3_cheat_type = "1"
cheat3_address = "1822482"
cheat3_memory_search_size = "3"
cheat3_value = "3"
cheat3_enable = "true"
cheat4_desc = "Rumble only"
cheat4_handler = "1"
cheat4_cheat_type = "0"
cheat4_address = "100"
cheat4_memory_search_size = "3"
cheat5_desc = "No code"
cheat5_code = ""
`

func TestReadMixedFile(t *testing.T) {
	got, err := Read(strings.NewReader(mixedFile), SystemSaturn, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Master code, broken, rumble-only, and empty entries are dropped
	if len(got) != 2 {
		t.Fatalf("got %d cheats, want 2: %+v", len(got), got)
	}
	if got[0].Name != "Infinite Crystals" || got[0].ID != "160FFE680063360FFEB90006" {
		t.Errorf("cheat 0 = %+v", got[0])
	}
	if got[0].Code != "160FFE68+0063+360FFEB9+0006" {
		t.Errorf("cheat 0 Code = %q", got[0].Code)
	}
	if got[1].Name != "Infinite lives." || got[1].Code != "" {
		t.Errorf("cheat 1 = %+v", got[1])
	}
}

func TestReadConvertsToCoreOrder(t *testing.T) {
	ar16 := func(bigEndian bool) Op {
		return Op{Kind: OpSet, Space: SpaceBus, Addr: 0x060FFE68, Size: 2, Mask: 0xFF, Value: 0x63, BigEndian: bigEndian}
	}
	ar8 := func(addr uint32) Op {
		return Op{Kind: OpSet, Space: SpaceBus, Addr: addr, Size: 1, Mask: 0xFF, Value: 6}
	}
	ra8 := func(addr uint32) Op {
		return Op{Kind: OpSet, Space: SpaceFlat, Addr: addr, Size: 1, Mask: 0xFF, Value: 3}
	}

	// Big-endian core: Action Replay addresses big-endian memory and is
	// unchanged; RetroArch memory is little-endian and is swapped
	got, err := Read(strings.NewReader(mixedFile), SystemSaturn, true)
	if err != nil {
		t.Fatalf("big-endian core: %v", err)
	}
	if want := []Op{ar16(true), ar8(0x060FFEB9)}; !reflect.DeepEqual(got[0].Ops, want) {
		t.Errorf("big-endian core AR ops:\n got  %+v\n want %+v", got[0].Ops, want)
	}
	if want := []Op{ra8(1822483)}; !reflect.DeepEqual(got[1].Ops, want) {
		t.Errorf("big-endian core RA ops:\n got  %+v\n want %+v", got[1].Ops, want)
	}

	// Little-endian core: Action Replay is swapped; RetroArch matches
	got, err = Read(strings.NewReader(mixedFile), SystemSaturn, false)
	if err != nil {
		t.Fatalf("little-endian core: %v", err)
	}
	if want := []Op{ar16(false), ar8(0x060FFEB8)}; !reflect.DeepEqual(got[0].Ops, want) {
		t.Errorf("little-endian core AR ops:\n got  %+v\n want %+v", got[0].Ops, want)
	}
	if want := []Op{ra8(1822482)}; !reflect.DeepEqual(got[1].Ops, want) {
		t.Errorf("little-endian core RA ops:\n got  %+v\n want %+v", got[1].Ops, want)
	}
}

// bigEndianRetroFile has RetroArch entries whose values are big-endian.
// Their addresses are still in RetroArch's little-endian memory.
const bigEndianRetroFile = `cheats = 3
cheat0_desc = "8-bit"
cheat0_handler = "1"
cheat0_cheat_type = "1"
cheat0_address = "0x100"
cheat0_memory_search_size = "3"
cheat0_value = "1"
cheat0_big_endian = "true"
cheat1_desc = "16-bit"
cheat1_handler = "1"
cheat1_cheat_type = "1"
cheat1_address = "0x102"
cheat1_memory_search_size = "4"
cheat1_value = "0x1234"
cheat1_big_endian = "true"
cheat2_desc = "32-bit"
cheat2_handler = "1"
cheat2_cheat_type = "1"
cheat2_address = "0x104"
cheat2_memory_search_size = "5"
cheat2_value = "1"
cheat2_big_endian = "true"
`

func TestReadBigEndianRetroValues(t *testing.T) {
	ra := func(addr uint32, size int, value uint32, bigEndian bool) []Op {
		return []Op{{Kind: OpSet, Space: SpaceFlat, Addr: addr, Size: size, Mask: 0xFF, Value: value, BigEndian: bigEndian}}
	}

	// Big-endian core: the 8-bit address flips and the 16-bit value is
	// little-endian in the core's memory; 32-bit cannot be swapped
	got, err := Read(strings.NewReader(bigEndianRetroFile), SystemSaturn, true)
	if err != nil {
		t.Fatalf("big-endian core: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("big-endian core: got %d cheats, want 2: %+v", len(got), got)
	}
	if want := ra(0x101, 1, 1, true); !reflect.DeepEqual(got[0].Ops, want) {
		t.Errorf("big-endian core 8-bit:\n got  %+v\n want %+v", got[0].Ops, want)
	}
	if want := ra(0x102, 2, 0x1234, false); !reflect.DeepEqual(got[1].Ops, want) {
		t.Errorf("big-endian core 16-bit:\n got  %+v\n want %+v", got[1].Ops, want)
	}

	// Little-endian core: RetroArch memory matches, all unchanged
	got, err = Read(strings.NewReader(bigEndianRetroFile), SystemSaturn, false)
	if err != nil {
		t.Fatalf("little-endian core: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("little-endian core: got %d cheats, want 3: %+v", len(got), got)
	}
	for i, want := range [][]Op{ra(0x100, 1, 1, true), ra(0x102, 2, 0x1234, true), ra(0x104, 4, 1, true)} {
		if !reflect.DeepEqual(got[i].Ops, want) {
			t.Errorf("little-endian core cheat %d:\n got  %+v\n want %+v", i, got[i].Ops, want)
		}
	}
}

func TestReadDropsUnconvertible(t *testing.T) {
	src := `cheats = 3
cheat0_desc = "32-bit"
cheat0_handler = "1"
cheat0_cheat_type = "1"
cheat0_address = "256"
cheat0_memory_search_size = "5"
cheat0_value = "1"
cheat1_desc = "Odd 16-bit"
cheat1_handler = "1"
cheat1_cheat_type = "1"
cheat1_address = "257"
cheat1_memory_search_size = "4"
cheat1_value = "1"
cheat2_desc = "8-bit"
cheat2_handler = "1"
cheat2_cheat_type = "1"
cheat2_address = "256"
cheat2_memory_search_size = "3"
cheat2_value = "1"
`
	got, err := Read(strings.NewReader(src), SystemSaturn, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "8-bit" {
		t.Errorf("got %+v, want only the 8-bit cheat", got)
	}

	// An odd 16-bit Action Replay code cannot be swapped for a
	// little-endian core
	got, err = Read(strings.NewReader("cheats = 1\ncheat0_code = \"16000001+0063\"\n"), SystemSaturn, false)
	if err != nil || len(got) != 0 {
		t.Errorf("odd 16-bit AR on little-endian core: got %+v err %v, want none", got, err)
	}
}

func TestReadDuplicateIDs(t *testing.T) {
	src := `cheats = 3
cheat0_desc = "First"
cheat0_code = "160FFE68+0063"
cheat1_desc = "Second"
cheat1_code = "160FFE68 0063"
cheat2_desc = "Other"
cheat2_code = "160FFE6A+0001"
`
	got, err := Read(strings.NewReader(src), SystemSaturn, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].Name != "First" || got[1].Name != "Other" {
		t.Errorf("got %+v, want First and Other", got)
	}
}

func TestReadUnknownSystem(t *testing.T) {
	src := `cheats = 2
cheat0_desc = "Code"
cheat0_code = "160FFE68+0063"
cheat1_desc = "Retro"
cheat1_handler = "1"
cheat1_cheat_type = "1"
cheat1_address = "16"
cheat1_memory_search_size = "3"
cheat1_value = "1"
`
	got, err := Read(strings.NewReader(src), System(0), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Code strings need a decoder for the system; RetroArch format entries
	// do not
	if len(got) != 1 || got[0].Name != "Retro" {
		t.Errorf("got %+v, want only the RetroArch entry", got)
	}
}

func TestReadTotalOpsCap(t *testing.T) {
	var b strings.Builder
	b.WriteString("cheats = 17\n")
	for i := 0; i < 17; i++ {
		fmt.Fprintf(&b, "cheat%d_handler = \"1\"\n", i)
		fmt.Fprintf(&b, "cheat%d_cheat_type = \"1\"\n", i)
		fmt.Fprintf(&b, "cheat%d_address = \"%d\"\n", i, i)
		fmt.Fprintf(&b, "cheat%d_memory_search_size = \"3\"\n", i)
		fmt.Fprintf(&b, "cheat%d_repeat_count = \"4096\"\n", i)
	}
	if _, err := Read(strings.NewReader(b.String()), SystemSaturn, true); err == nil {
		t.Error("expected error when cheats decode to more than the op cap")
	}
}

func TestParseCodePasted(t *testing.T) {
	c, err := ParseCode("160EF100 0000\n160C5238 0000\n", SystemSaturn, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Op{
		{Kind: OpSet, Space: SpaceBus, Addr: 0x060EF100, Size: 2, Mask: 0xFF, Value: 0, BigEndian: true},
		{Kind: OpSet, Space: SpaceBus, Addr: 0x060C5238, Size: 2, Mask: 0xFF, Value: 0, BigEndian: true},
	}
	if !reflect.DeepEqual(c.Ops, want) {
		t.Errorf("ops:\n got  %+v\n want %+v", c.Ops, want)
	}
	if c.Code != "160EF100 0000\n160C5238 0000" || c.Name != "" {
		t.Errorf("Code %q Name %q, want trimmed code and no name", c.Code, c.Name)
	}

	// The same code written with other separators has the same ID
	other, err := ParseCode("160EF100+0000+160C5238+0000", SystemSaturn, true)
	if err != nil || other.ID != c.ID {
		t.Errorf("ID %q err %v, want %q", other.ID, err, c.ID)
	}
}

func TestParseCodeConvertsToCoreOrder(t *testing.T) {
	c, err := ParseCode("360EF101 0005 160EF102 1234", SystemSaturn, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Ops[0].Addr != 0x060EF100 {
		t.Errorf("8-bit op = %+v, want address 0x060EF100", c.Ops[0])
	}
	if c.Ops[1].Addr != 0x060EF102 || c.Ops[1].BigEndian {
		t.Errorf("16-bit op = %+v, want address 0x060EF102 little-endian", c.Ops[1])
	}

	// Big-endian core: unchanged
	c, err = ParseCode("360EF101 0005 160EF102 1234", SystemSaturn, true)
	if err != nil || c.Ops[0].Addr != 0x060EF101 || !c.Ops[1].BigEndian {
		t.Errorf("big-endian core: ops %+v err %v, want unchanged", c.Ops, err)
	}
}

func TestParseCodeErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		code string
		want error
	}{
		"empty":         {"", errNoCode},
		"spaces only":   {" \n\t", errNoCode},
		"master only":   {"F6000924 FFFF\nB6002800 0000", errNoOps},
		"unsupported":   {"not a code", errUnsupportedFormat},
		"missing value": {"160EF100", errUnsupportedFormat},
		"bad code type": {"760EF100 0000", nil},
	} {
		_, err := ParseCode(tc.code, SystemSaturn, true)
		if err == nil || (tc.want != nil && err != tc.want) {
			t.Errorf("%s: err %v, want %v", name, err, tc.want)
		}
	}

	// Odd 16-bit addresses cannot be converted for a little-endian core
	if _, err := ParseCode("160EF101 0000", SystemSaturn, false); err == nil {
		t.Error("odd 16-bit on a little-endian core: want an error")
	}
	if _, err := ParseCode("160EF100 0000", System(0), true); err != errUnsupportedFormat {
		t.Errorf("unknown system: err %v, want %v", err, errUnsupportedFormat)
	}
}

func TestCodeID(t *testing.T) {
	a := codeID("160ffe68+0063 + 160FFEB8+0006")
	b := codeID("160FFE68 0063\t160FFEB8 0006")
	if a != b || a != "160FFE680063160FFEB80006" {
		t.Errorf("codeID mismatch: %q vs %q", a, b)
	}
}
