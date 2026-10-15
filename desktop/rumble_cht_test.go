package desktop

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/user-none/eblitui/coreif"
)

func TestParseCHTRumble(t *testing.T) {
	content := `cheats = "3"
cheat0_address = "49152"
cheat0_memory_search_size = "3"
cheat0_rumble_type = "1"
cheat0_rumble_value = "0"
cheat0_rumble_port = "0"
cheat0_rumble_primary_strength = "65535"
cheat0_rumble_primary_duration = "200"
cheat0_rumble_secondary_strength = "32768"
cheat0_rumble_secondary_duration = "100"
cheat1_address = "49153"
cheat1_memory_search_size = "4"
cheat1_rumble_type = "5"
cheat1_rumble_value = "255"
cheat1_rumble_port = "16"
cheat1_rumble_primary_strength = "50000"
cheat1_rumble_primary_duration = "300"
cheat1_rumble_secondary_strength = "0"
cheat1_rumble_secondary_duration = "0"
cheat2_address = "100"
cheat2_memory_search_size = "5"
cheat2_rumble_type = "8"
cheat2_rumble_value = "10"
cheat2_rumble_port = "1"
cheat2_rumble_primary_strength = "40000"
cheat2_rumble_primary_duration = "150"
cheat2_rumble_secondary_strength = "20000"
cheat2_rumble_secondary_duration = "150"
`
	entries, err := ParseCHTRumble(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Entry 0
	e := entries[0]
	if e.Address != 49152 {
		t.Errorf("entry 0 address: expected 49152, got %d", e.Address)
	}
	if e.MemorySearchSize != 3 {
		t.Errorf("entry 0 search size: expected 3, got %d", e.MemorySearchSize)
	}
	if e.RumbleType != 1 {
		t.Errorf("entry 0 rumble type: expected 1, got %d", e.RumbleType)
	}
	if e.RumblePort != 0 {
		t.Errorf("entry 0 rumble port: expected 0, got %d", e.RumblePort)
	}
	if e.PrimaryStrength != 65535 {
		t.Errorf("entry 0 primary strength: expected 65535, got %d", e.PrimaryStrength)
	}
	if e.PrimaryDuration != 200 {
		t.Errorf("entry 0 primary duration: expected 200, got %d", e.PrimaryDuration)
	}
	if e.SecondaryStrength != 32768 {
		t.Errorf("entry 0 secondary strength: expected 32768, got %d", e.SecondaryStrength)
	}
	if e.SecondaryDuration != 100 {
		t.Errorf("entry 0 secondary duration: expected 100, got %d", e.SecondaryDuration)
	}

	// Entry 1
	e = entries[1]
	if e.Address != 49153 {
		t.Errorf("entry 1 address: expected 49153, got %d", e.Address)
	}
	if e.MemorySearchSize != 4 {
		t.Errorf("entry 1 search size: expected 4, got %d", e.MemorySearchSize)
	}
	if e.RumbleType != 5 {
		t.Errorf("entry 1 rumble type: expected 5, got %d", e.RumbleType)
	}
	if e.RumbleValue != 255 {
		t.Errorf("entry 1 rumble value: expected 255, got %d", e.RumbleValue)
	}
	if e.RumblePort != 16 {
		t.Errorf("entry 1 rumble port: expected 16, got %d", e.RumblePort)
	}
	if e.PrimaryStrength != 50000 {
		t.Errorf("entry 1 primary strength: expected 50000, got %d", e.PrimaryStrength)
	}

	// Entry 2
	e = entries[2]
	if e.Address != 100 {
		t.Errorf("entry 2 address: expected 100, got %d", e.Address)
	}
	if e.MemorySearchSize != 5 {
		t.Errorf("entry 2 search size: expected 5, got %d", e.MemorySearchSize)
	}
	if e.RumbleType != 8 {
		t.Errorf("entry 2 rumble type: expected 8, got %d", e.RumbleType)
	}
	if e.RumbleValue != 10 {
		t.Errorf("entry 2 rumble value: expected 10, got %d", e.RumbleValue)
	}
	if e.RumblePort != 1 {
		t.Errorf("entry 2 rumble port: expected 1, got %d", e.RumblePort)
	}
}

func TestParseCHTRumbleBigEndian(t *testing.T) {
	content := `cheats = "2"
cheat0_big_endian = "true"
cheat0_address = "100"
cheat0_memory_search_size = "3"
cheat0_rumble_type = "1"
cheat0_rumble_primary_strength = "65535"
cheat0_rumble_primary_duration = "200"
cheat1_big_endian = "false"
cheat1_address = "200"
cheat1_memory_search_size = "3"
cheat1_rumble_type = "1"
cheat1_rumble_primary_strength = "65535"
cheat1_rumble_primary_duration = "200"
`
	entries, err := ParseCHTRumble(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if !entries[0].BigEndian {
		t.Error("entry 0: expected BigEndian=true")
	}
	if entries[1].BigEndian {
		t.Error("entry 1: expected BigEndian=false")
	}
}

func TestCHTRumbleEngineBigEndianSystem(t *testing.T) {
	// On a big-endian system every entry's 8-bit address is read with bit
	// 0 flipped, whatever its big_endian field: the field only orders the
	// bytes of wider values
	entries := []CHTRumbleEntry{
		{
			Address:          100,
			MemorySearchSize: 3,
			RumbleType:       1,
			BigEndian:        true,
			PrimaryStrength:  65535,
			PrimaryDuration:  200,
		},
		{
			Address:          200,
			MemorySearchSize: 3,
			RumbleType:       1,
			BigEndian:        false,
			PrimaryStrength:  65535,
			PrimaryDuration:  200,
		},
	}

	engine := NewCHTRumbleEngine(entries, true)
	mi := newMockMemoryInspector()
	mi.set8(101, 0x42)
	mi.set8(201, 0x55)

	for i := 0; i < 30; i++ {
		engine.Evaluate(mi)
	}

	// Changes at the unflipped addresses are not seen
	mi.set8(100, 0x01)
	mi.set8(200, 0x01)
	if events := engine.Evaluate(mi); len(events) != 0 {
		t.Fatalf("unflipped addresses: expected 0 events, got %d", len(events))
	}

	mi.set8(101, 0x43)
	mi.set8(201, 0x56)
	if events := engine.Evaluate(mi); len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestParseCHTRumbleMissingFields(t *testing.T) {
	content := `cheats = "1"
cheat0_address = "1000"
cheat0_rumble_type = "1"
cheat0_rumble_primary_strength = "65535"
cheat0_rumble_primary_duration = "200"
`
	entries, err := ParseCHTRumble(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Address != 1000 {
		t.Errorf("address: expected 1000, got %d", e.Address)
	}
	if e.MemorySearchSize != 0 {
		t.Errorf("search size: expected 0 (default), got %d", e.MemorySearchSize)
	}
	if e.RumblePort != 1 {
		t.Errorf("port: expected 1 (default), got %d", e.RumblePort)
	}
	if e.SecondaryStrength != 0 {
		t.Errorf("secondary strength: expected 0 (default), got %d", e.SecondaryStrength)
	}
	if e.BigEndian {
		t.Error("big endian: expected false (default)")
	}
}

func TestParseCHTRumbleNoCheatsKey(t *testing.T) {
	content := `cheat0_address = "1000"`
	_, err := ParseCHTRumble(strings.NewReader(content))
	if err == nil {
		t.Fatal("expected error for missing cheats key")
	}
}

func TestParseCHTRumbleFileNotFound(t *testing.T) {
	_, err := ParseCHTRumbleFile(filepath.Join(t.TempDir(), "missing.cht"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestEvaluateConditionChanges(t *testing.T) {
	// Type 0 (changes) and type 1 (changes)
	if !evaluateCondition(0, 5, 3, 0) {
		t.Error("type 0: expected true when value changed")
	}
	if evaluateCondition(0, 5, 5, 0) {
		t.Error("type 0: expected false when value unchanged")
	}
	if !evaluateCondition(1, 5, 3, 0) {
		t.Error("type 1: expected true when value changed")
	}
	if evaluateCondition(1, 5, 5, 0) {
		t.Error("type 1: expected false when value unchanged")
	}
}

func TestEvaluateConditionDoesNotChange(t *testing.T) {
	if !evaluateCondition(2, 5, 5, 0) {
		t.Error("type 2: expected true when value unchanged")
	}
	if evaluateCondition(2, 5, 3, 0) {
		t.Error("type 2: expected false when value changed")
	}
}

func TestEvaluateConditionIncreases(t *testing.T) {
	if !evaluateCondition(3, 10, 5, 0) {
		t.Error("type 3: expected true when increased")
	}
	if evaluateCondition(3, 5, 10, 0) {
		t.Error("type 3: expected false when decreased")
	}
	if evaluateCondition(3, 5, 5, 0) {
		t.Error("type 3: expected false when equal")
	}
}

func TestEvaluateConditionDecreases(t *testing.T) {
	if !evaluateCondition(4, 5, 10, 0) {
		t.Error("type 4: expected true when decreased")
	}
	if evaluateCondition(4, 10, 5, 0) {
		t.Error("type 4: expected false when increased")
	}
}

func TestEvaluateConditionEquals(t *testing.T) {
	if !evaluateCondition(5, 42, 0, 42) {
		t.Error("type 5: expected true when equals value")
	}
	if evaluateCondition(5, 42, 0, 43) {
		t.Error("type 5: expected false when not equals value")
	}
}

func TestEvaluateConditionNotEquals(t *testing.T) {
	if !evaluateCondition(6, 42, 0, 43) {
		t.Error("type 6: expected true when not equals value")
	}
	if evaluateCondition(6, 42, 0, 42) {
		t.Error("type 6: expected false when equals value")
	}
}

func TestEvaluateConditionLessThan(t *testing.T) {
	if !evaluateCondition(7, 5, 0, 10) {
		t.Error("type 7: expected true when less than value")
	}
	if evaluateCondition(7, 10, 0, 5) {
		t.Error("type 7: expected false when greater than value")
	}
	if evaluateCondition(7, 5, 0, 5) {
		t.Error("type 7: expected false when equal to value")
	}
}

func TestEvaluateConditionGreaterThan(t *testing.T) {
	if !evaluateCondition(8, 10, 0, 5) {
		t.Error("type 8: expected true when greater than value")
	}
	if evaluateCondition(8, 5, 0, 10) {
		t.Error("type 8: expected false when less than value")
	}
}

func TestEvaluateConditionIncreasedBy(t *testing.T) {
	if !evaluateCondition(9, 15, 10, 5) {
		t.Error("type 9: expected true when increased by value")
	}
	if evaluateCondition(9, 16, 10, 5) {
		t.Error("type 9: expected false when not increased by exact value")
	}
}

func TestEvaluateConditionDecreasedBy(t *testing.T) {
	if !evaluateCondition(10, 5, 10, 5) {
		t.Error("type 10: expected true when decreased by value")
	}
	if evaluateCondition(10, 6, 10, 5) {
		t.Error("type 10: expected false when not decreased by exact value")
	}
}

func TestEvaluateConditionUnknownType(t *testing.T) {
	if evaluateCondition(99, 1, 2, 3) {
		t.Error("unknown type: expected false")
	}
}

// mockMemoryInspector implements coreif.Memory for testing.
type mockMemoryInspector struct {
	data map[uint32]byte
}

func newMockMemoryInspector() *mockMemoryInspector {
	return &mockMemoryInspector{data: make(map[uint32]byte)}
}

func (m *mockMemoryInspector) ReadMemoryFlat(off uint32, buf []byte) uint32 {
	for i := range buf {
		buf[i] = m.data[off+uint32(i)]
	}
	return uint32(len(buf))
}

func (m *mockMemoryInspector) WriteMemoryFlat(off uint32, data []byte) uint32 {
	for i := range data {
		m.data[off+uint32(i)] = data[i]
	}
	return uint32(len(data))
}

func (m *mockMemoryInspector) ReadMemory(addr uint32, buf []byte) uint32 { return 0 }

func (m *mockMemoryInspector) WriteMemory(addr uint32, data []byte) uint32 { return 0 }

func (m *mockMemoryInspector) Regions() []coreif.BusRegion { return nil }

func (m *mockMemoryInspector) set8(addr uint32, val byte) {
	m.data[addr] = val
}

func (m *mockMemoryInspector) set16(addr uint32, val uint16) {
	m.data[addr] = byte(val)
	m.data[addr+1] = byte(val >> 8)
}

func TestReadMemoryValue8Bit(t *testing.T) {
	mi := newMockMemoryInspector()
	mi.set8(100, 0xAB)
	val := readMemoryValue(mi, 100, 3, false, false) // 8-bit, no swap
	if val != 0xAB {
		t.Errorf("expected 0xAB, got 0x%X", val)
	}
}

func TestReadMemoryValue16Bit(t *testing.T) {
	mi := newMockMemoryInspector()
	mi.set16(200, 0x1234)
	val := readMemoryValue(mi, 200, 4, false, false) // 16-bit, no swap
	if val != 0x1234 {
		t.Errorf("expected 0x1234, got 0x%X", val)
	}
}

func TestReadMemoryValue1Bit(t *testing.T) {
	mi := newMockMemoryInspector()
	mi.set8(50, 0xFF)
	val := readMemoryValue(mi, 50, 0, false, false) // 1-bit, no swap
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}

	mi.set8(50, 0xFE) // bit 0 = 0
	val = readMemoryValue(mi, 50, 0, false, false)
	if val != 0 {
		t.Errorf("expected 0, got %d", val)
	}
}

func TestReadMemoryValue4Bit(t *testing.T) {
	mi := newMockMemoryInspector()
	mi.set8(60, 0xAB)
	val := readMemoryValue(mi, 60, 2, false, false) // 4-bit, no swap
	if val != 0x0B {
		t.Errorf("expected 0x0B, got 0x%X", val)
	}
}

func TestReadMemoryValue8BitBigEndianSystem(t *testing.T) {
	mi := newMockMemoryInspector()
	// Big-endian memory: addr 100 = high byte, addr 101 = low byte
	mi.set8(100, 0x00) // even addr
	mi.set8(101, 0xAB) // odd addr
	// On a big-endian system, reading addr 100 should XOR to 101
	val := readMemoryValue(mi, 100, 3, false, true)
	if val != 0xAB {
		t.Errorf("expected 0xAB, got 0x%X", val)
	}
	// And reading addr 101 should XOR to 100
	val = readMemoryValue(mi, 101, 3, false, true)
	if val != 0x00 {
		t.Errorf("expected 0x00, got 0x%X", val)
	}
}

func TestReadMemoryValue16BitBigEndianSystem(t *testing.T) {
	mi := newMockMemoryInspector()
	// Big-endian memory: 0x12 at addr 200, 0x34 at addr 201
	mi.set8(200, 0x12)
	mi.set8(201, 0x34)
	// A little-endian entry on a big-endian system reads the core's
	// big-endian value: 0x1234
	val := readMemoryValue(mi, 200, 4, false, true)
	if val != 0x1234 {
		t.Errorf("expected 0x1234, got 0x%X", val)
	}
}

func TestReadMemoryValueBigEndianEntry(t *testing.T) {
	mi := newMockMemoryInspector()
	for i, b := range []byte{0x12, 0x34, 0x56, 0x78} {
		mi.set8(uint32(200+i), b)
	}

	tests := []struct {
		name       string
		addr       uint32
		size       int
		systemBig  bool
		wantResult uint32
	}{
		// Big-endian system: RetroArch's view is 34 12 78 56
		{"8-bit, big-endian system", 200, 3, true, 0x34},
		{"16-bit, big-endian system", 200, 4, true, 0x3412},
		{"32-bit, big-endian system", 200, 5, true, 0x34127856},
		// Little-endian system: the view is the memory as stored
		{"8-bit, little-endian system", 200, 3, false, 0x12},
		{"16-bit, little-endian system", 200, 4, false, 0x1234},
		{"32-bit, little-endian system", 200, 5, false, 0x12345678},
	}
	for _, tc := range tests {
		if got := readMemoryValue(mi, tc.addr, tc.size, true, tc.systemBig); got != tc.wantResult {
			t.Errorf("%s: got 0x%X, want 0x%X", tc.name, got, tc.wantResult)
		}
	}
}

func TestReadMemoryValueLittleEndianEntryWide(t *testing.T) {
	mi := newMockMemoryInspector()
	for i, b := range []byte{0x12, 0x34, 0x56, 0x78} {
		mi.set8(uint32(200+i), b)
	}

	tests := []struct {
		name       string
		addr       uint32
		size       int
		systemBig  bool
		wantResult uint32
	}{
		// Big-endian system: RetroArch's view is 34 12 78 56
		{"32-bit, big-endian system", 200, 5, true, 0x56781234},
		{"odd 16-bit, big-endian system", 201, 4, true, 0x7812},
		// Little-endian system: the view is the memory as stored
		{"32-bit, little-endian system", 200, 5, false, 0x78563412},
		{"odd 16-bit, little-endian system", 201, 4, false, 0x5634},
	}
	for _, tc := range tests {
		if got := readMemoryValue(mi, tc.addr, tc.size, false, tc.systemBig); got != tc.wantResult {
			t.Errorf("%s: got 0x%X, want 0x%X", tc.name, got, tc.wantResult)
		}
	}
}

func TestCHTRumbleEngineWarmup(t *testing.T) {
	entries := []CHTRumbleEntry{{
		Address:          100,
		MemorySearchSize: 3,
		RumbleType:       1, // changes
		PrimaryStrength:  65535,
		PrimaryDuration:  200,
	}}

	engine := NewCHTRumbleEngine(entries, false)
	mi := newMockMemoryInspector()

	// During warmup, no events should fire even if value changes
	mi.set8(100, 1)
	for i := 0; i < 30; i++ {
		events := engine.Evaluate(mi)
		if events != nil {
			t.Fatalf("expected no events during warmup frame %d", i)
		}
		mi.set8(100, byte(i%256))
	}
}

func TestCHTRumbleEngineFiresAfterWarmup(t *testing.T) {
	entries := []CHTRumbleEntry{{
		Address:          100,
		MemorySearchSize: 3,
		RumbleType:       1, // changes
		RumblePort:       16,
		PrimaryStrength:  65535,
		PrimaryDuration:  200,
	}}

	engine := NewCHTRumbleEngine(entries, false)
	mi := newMockMemoryInspector()
	mi.set8(100, 0)

	// Run through warmup
	for i := 0; i < 30; i++ {
		engine.Evaluate(mi)
	}

	// Now change the value - should fire
	mi.set8(100, 1)
	events := engine.Evaluate(mi)
	if len(events) != 1 {
		t.Fatalf("expected 1 event after warmup, got %d", len(events))
	}
	if events[0].StrongMagnitude != 1.0 {
		t.Errorf("expected strong magnitude 1.0, got %f", events[0].StrongMagnitude)
	}
}

func TestCHTRumbleEngineNoFireWhenUnchanged(t *testing.T) {
	entries := []CHTRumbleEntry{{
		Address:          100,
		MemorySearchSize: 3,
		RumbleType:       1, // changes
		PrimaryStrength:  65535,
		PrimaryDuration:  200,
	}}

	engine := NewCHTRumbleEngine(entries, false)
	mi := newMockMemoryInspector()
	mi.set8(100, 42)

	// Warmup
	for i := 0; i < 30; i++ {
		engine.Evaluate(mi)
	}

	// Value unchanged - should not fire
	events := engine.Evaluate(mi)
	if len(events) != 0 {
		t.Fatalf("expected no events when value unchanged, got %d", len(events))
	}
}

func TestCHTRumbleEngineReset(t *testing.T) {
	entries := []CHTRumbleEntry{{
		Address:          100,
		MemorySearchSize: 3,
		RumbleType:       1,
		PrimaryStrength:  65535,
		PrimaryDuration:  200,
	}}

	engine := NewCHTRumbleEngine(entries, false)
	mi := newMockMemoryInspector()
	mi.set8(100, 0)

	// Run through warmup
	for i := 0; i < 30; i++ {
		engine.Evaluate(mi)
	}

	// Reset should require warmup again
	engine.Reset()
	mi.set8(100, 1)
	events := engine.Evaluate(mi)
	if events != nil {
		t.Fatal("expected no events after reset (should be in warmup)")
	}
}
