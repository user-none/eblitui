package rumble

import (
	"strings"
	"testing"
)

// A Saturn-shaped system: the two work RAM regions in canonical
// addresses.
func testSystem() System {
	return System{
		BigEndian: true,
		Regions: []Region{
			{Name: "Work RAM-L", Start: 0x00200000, Size: 0x100000},
			{Name: "Work RAM-H", Start: 0x06000000, Size: 0x100000},
		},
	}
}

func mustValidate(t *testing.T, sys System, v Watch) {
	t.Helper()
	if err := sys.validate(&v); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
}

func TestValidateCanonical(t *testing.T) {
	sys := testSystem()
	mustValidate(t, sys, Watch{Name: "v", Width: 8, Address: 0x00200010})
	mustValidate(t, sys, Watch{Name: "v", Width: 8, Address: 0x0605C973})
	mustValidate(t, sys, Watch{Name: "v", Width: 8, Address: 0x00200000})
	mustValidate(t, sys, Watch{Name: "v", Width: 8, Address: 0x060FFFFF})
}

// Mirror and partition spellings are the core's internal decode; only
// canonical addresses are valid in a ruleset.
func TestValidateRejectsNonCanonical(t *testing.T) {
	sys := testSystem()
	for _, addr := range []uint32{
		0x2605C973, // partition spelling of 0x0605C973
		0x0615C973, // mirror spelling of 0x0605C973
		0x07F0C973, // high mirror
	} {
		v := Watch{Name: "v", Width: 8, Address: addr}
		if err := sys.validate(&v); err == nil {
			t.Fatalf("0x%X: expected rejection", addr)
		}
	}
}

func TestValidateUnknownRegion(t *testing.T) {
	sys := testSystem()
	v := Watch{Name: "v", Width: 8, Address: 0x05000000, Line: 3}
	err := sys.validate(&v)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"line 3:", `watch "v"`, "not in a known memory region", "Work RAM-L", "Work RAM-H"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err.Error(), want)
		}
	}
}

func TestValidateWidthPastRegionEnd(t *testing.T) {
	sys := testSystem()
	v := Watch{Name: "v", Width: 32, Address: 0x002FFFFE}
	if err := sys.validate(&v); err == nil || !strings.Contains(err.Error(), "does not fit inside") {
		t.Fatalf("err = %v", err)
	}
	mustValidate(t, sys, Watch{Name: "v", Width: 16, Address: 0x002FFFFE})
}

// An array watch's whole span, first slot through last slot plus
// width, must fit inside the region.
func TestValidateArraySpan(t *testing.T) {
	sys := testSystem()
	mustValidate(t, sys, Watch{Name: "v", Width: 8, Address: 0x002FF000, Stride: 0x70, Count: 32})
	v := Watch{Name: "v", Width: 8, Address: 0x002FF800, Stride: 0x70, Count: 32}
	if err := sys.validate(&v); err == nil || !strings.Contains(err.Error(), "does not fit inside") {
		t.Fatalf("err = %v", err)
	}
	mustValidate(t, sys, Watch{Name: "v", Width: 16, Address: 0x002FFFFC, Stride: 2, Count: 2})
	v = Watch{Name: "v", Width: 16, Address: 0x002FFFFC, Stride: 2, Count: 3}
	if err := sys.validate(&v); err == nil || !strings.Contains(err.Error(), "does not fit inside") {
		t.Fatalf("err = %v", err)
	}
}

// A pointer watch's 4-byte pointer cell must fit; the target is the
// game's to move and is checked at read time instead.
func TestValidatePointerCell(t *testing.T) {
	sys := testSystem()
	mustValidate(t, sys, Watch{Name: "v", Width: 8, Address: 0x002FFFFC, Pointer: true, Offset: 0x9000})
	v := Watch{Name: "v", Width: 8, Address: 0x002FFFFE, Pointer: true}
	if err := sys.validate(&v); err == nil || !strings.Contains(err.Error(), "does not fit inside") {
		t.Fatalf("err = %v", err)
	}
}

// A keyed slot watch's span runs to the last slot's furthest read,
// the deeper of key+4 and field+size.
func TestValidateKeyedSpan(t *testing.T) {
	sys := testSystem()
	// Base 0x002FFF00: 7 strides of 0x20 plus the field read at
	// +0x1F ends exactly at the region end. One byte later misses.
	mustValidate(t, sys, Watch{Name: "v", Width: 8, Address: 0x002FFF00, Stride: 0x20, Count: 8,
		HasKey: true, KeyOffset: 0, FieldOffset: 0x1F})
	v := Watch{Name: "v", Width: 8, Address: 0x002FFF01, Stride: 0x20, Count: 8,
		HasKey: true, KeyOffset: 0, FieldOffset: 0x1F}
	if err := sys.validate(&v); err == nil || !strings.Contains(err.Error(), "does not fit inside") {
		t.Fatalf("err = %v", err)
	}
}
