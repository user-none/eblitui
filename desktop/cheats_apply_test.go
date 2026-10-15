// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"strings"
	"testing"

	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/cheats"
)

// cheatMemory implements coreif.Memory with separate bus and flat byte
// spaces.
type cheatMemory struct {
	bus  map[uint32]byte
	flat map[uint32]byte
}

func newCheatMemory() *cheatMemory {
	return &cheatMemory{bus: make(map[uint32]byte), flat: make(map[uint32]byte)}
}

func (m *cheatMemory) ReadMemory(addr uint32, buf []byte) uint32 {
	for i := range buf {
		buf[i] = m.bus[addr+uint32(i)]
	}
	return uint32(len(buf))
}

func (m *cheatMemory) WriteMemory(addr uint32, data []byte) uint32 {
	for i := range data {
		m.bus[addr+uint32(i)] = data[i]
	}
	return uint32(len(data))
}

func (m *cheatMemory) ReadMemoryFlat(off uint32, buf []byte) uint32 {
	for i := range buf {
		buf[i] = m.flat[off+uint32(i)]
	}
	return uint32(len(buf))
}

func (m *cheatMemory) WriteMemoryFlat(off uint32, data []byte) uint32 {
	for i := range data {
		m.flat[off+uint32(i)] = data[i]
	}
	return uint32(len(data))
}

func (m *cheatMemory) Regions() []coreif.BusRegion { return nil }

func busOp(kind cheats.OpKind, addr uint32, size int, value uint32) cheats.Op {
	return cheats.Op{Kind: kind, Space: cheats.SpaceBus, Addr: addr, Size: size, Mask: 0xFF, Value: value, BigEndian: true}
}

func TestCheatEngineSetBigEndian(t *testing.T) {
	mem := newCheatMemory()
	e := NewCheatEngine([]cheats.Cheat{{Ops: []cheats.Op{
		busOp(cheats.OpSet, 0x060FFE68, 2, 0x0063),
		busOp(cheats.OpSet, 0x060DBD38, 1, 0x05),
	}}})

	e.Apply(mem)

	if mem.bus[0x060FFE68] != 0x00 || mem.bus[0x060FFE69] != 0x63 {
		t.Errorf("16-bit write = %02X %02X, want 00 63", mem.bus[0x060FFE68], mem.bus[0x060FFE69])
	}
	if mem.bus[0x060DBD38] != 0x05 {
		t.Errorf("8-bit write = %02X, want 05", mem.bus[0x060DBD38])
	}
}

func TestCheatEngineCompareGatesNextOpOnly(t *testing.T) {
	cheat := cheats.Cheat{Ops: []cheats.Op{
		busOp(cheats.OpIfEqual, 0x0604A8C4, 2, 0x0001),
		busOp(cheats.OpSet, 0x0604A8C6, 2, 0x0063),
		busOp(cheats.OpSet, 0x0604A8C8, 2, 0x0007),
	}}

	// Compare fails: the gated op is skipped, the one after still runs
	mem := newCheatMemory()
	NewCheatEngine([]cheats.Cheat{cheat}).Apply(mem)
	if mem.bus[0x0604A8C7] != 0 {
		t.Errorf("gated op ran with failed compare: %02X", mem.bus[0x0604A8C7])
	}
	if mem.bus[0x0604A8C9] != 0x07 {
		t.Errorf("op after the gated op = %02X, want 07", mem.bus[0x0604A8C9])
	}

	// Compare passes: the gated op runs
	mem = newCheatMemory()
	mem.bus[0x0604A8C5] = 0x01
	NewCheatEngine([]cheats.Cheat{cheat}).Apply(mem)
	if mem.bus[0x0604A8C7] != 0x63 {
		t.Errorf("gated op with passing compare = %02X, want 63", mem.bus[0x0604A8C7])
	}
}

func TestCheatEngineCompareDoesNotCrossCheats(t *testing.T) {
	// A failed compare that ends one cheat's ops does not skip the
	// next cheat's first op
	mem := newCheatMemory()
	NewCheatEngine([]cheats.Cheat{
		{Ops: []cheats.Op{busOp(cheats.OpSet, 0x100, 1, 1), busOp(cheats.OpIfEqual, 0x200, 2, 0xFFFF)}},
		{Ops: []cheats.Op{busOp(cheats.OpSet, 0x300, 1, 3)}},
	}).Apply(mem)

	if mem.bus[0x300] != 3 {
		t.Errorf("next cheat's op was skipped: %02X", mem.bus[0x300])
	}
}

func TestCheatEngineMaskChangesOnlyMaskedBits(t *testing.T) {
	mem := newCheatMemory()
	mem.flat[0x10] = 0xA5
	NewCheatEngine([]cheats.Cheat{{Ops: []cheats.Op{
		{Kind: cheats.OpSet, Space: cheats.SpaceFlat, Addr: 0x10, Size: 1, Mask: 0x0F, Value: 0x03},
	}}}).Apply(mem)

	if mem.flat[0x10] != 0xA3 {
		t.Errorf("masked write = %02X, want A3", mem.flat[0x10])
	}
}

func TestCheatEngineAddSubLittleEndian(t *testing.T) {
	mem := newCheatMemory()
	mem.flat[0x20] = 0xFE // 0x00FE little-endian
	mem.flat[0x30] = 0x05
	NewCheatEngine([]cheats.Cheat{{Ops: []cheats.Op{
		{Kind: cheats.OpAdd, Space: cheats.SpaceFlat, Addr: 0x20, Size: 2, Mask: 0xFF, Value: 3},
		{Kind: cheats.OpSub, Space: cheats.SpaceFlat, Addr: 0x30, Size: 1, Mask: 0xFF, Value: 2},
	}}}).Apply(mem)

	if mem.flat[0x20] != 0x01 || mem.flat[0x21] != 0x01 {
		t.Errorf("16-bit add = %02X %02X, want 01 01", mem.flat[0x20], mem.flat[0x21])
	}
	if mem.flat[0x30] != 0x03 {
		t.Errorf("8-bit subtract = %02X, want 03", mem.flat[0x30])
	}
}

func TestCheatEngineAppliesParsedRetroEntry(t *testing.T) {
	// A RetroArch format 4-bit set at flat 0x1BCF12 lands on the core's
	// byte 0x1BCF13 on a big-endian core
	src := `cheats = 1
cheat0_desc = "Lives"
cheat0_handler = "1"
cheat0_cheat_type = "1"
cheat0_address = "0x1BCF12"
cheat0_memory_search_size = "2"
cheat0_address_bit_position = "15"
cheat0_value = "3"
`
	list, err := cheats.Read(strings.NewReader(src), cheats.SystemSaturn, true)
	if err != nil || len(list) != 1 {
		t.Fatalf("parse: %v %+v", err, list)
	}
	mem := newCheatMemory()
	mem.flat[0x1BCF13] = 0x50
	NewCheatEngine(list).Apply(mem)

	if mem.flat[0x1BCF13] != 0x53 || mem.flat[0x1BCF12] != 0 {
		t.Errorf("bytes = %02X %02X, want 00 53", mem.flat[0x1BCF12], mem.flat[0x1BCF13])
	}
}
