// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package rumble

import (
	"fmt"
	"strings"
)

// Region describes one memory region of a system's bus in canonical
// addresses.
type Region struct {
	Name  string
	Start uint32 // native bus address of the region's first byte
	Size  uint32 // region size in bytes
}

// System describes the memory layout that addresses validate against
// and the native byte order values decode with.
type System struct {
	BigEndian bool
	Regions   []Region
}

// validate checks that a value's address plus what it reads there lies
// inside a known region. The width, a pointer watch's 4-byte pointer
// cell, or an array watch's whole span, first slot through the last
// slot's furthest read. A pointer's target is the game's to move, so
// it is checked at read time instead. Addresses are canonical.
func (sys *System) validate(v *Watch) error {
	span := v.slotSpan()
	if v.Count > 1 {
		span += uint64(v.Stride) * uint64(v.Count-1)
	}
	for i := range sys.Regions {
		r := &sys.Regions[i]
		if v.Address >= r.Start && v.Address-r.Start < r.Size {
			if span > uint64(r.Size)-uint64(v.Address-r.Start) {
				return errf(v.Line, "watch %q address 0x%X does not fit inside %s", v.Name, v.Address, r.Name)
			}
			return nil
		}
	}
	return errf(v.Line, "watch %q address 0x%X is not in a known memory region (%s)", v.Name, v.Address, describeRegions(sys.Regions))
}

func describeRegions(regions []Region) string {
	if len(regions) == 0 {
		return "no regions"
	}
	var parts []string
	for i := range regions {
		r := &regions[i]
		parts = append(parts, fmt.Sprintf("%s 0x%08X-0x%08X", r.Name, r.Start, r.Start+r.Size-1))
	}
	return strings.Join(parts, ", ")
}
