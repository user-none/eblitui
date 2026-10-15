// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package cheats

import "testing"

func TestSystemFromName(t *testing.T) {
	for _, name := range []string{"Sega - Saturn", "sega - saturn", "Saturn", "SATURN", " saturn "} {
		sys, err := SystemFromName(name)
		if err != nil || sys != SystemSaturn {
			t.Errorf("%q: got %v, %v; want SystemSaturn", name, sys, err)
		}
	}
}

func TestSystemFromNameUnknown(t *testing.T) {
	for _, name := range []string{"", "Sega - Mega Drive - Genesis", "Sega Saturn", "sat"} {
		if sys, err := SystemFromName(name); err == nil {
			t.Errorf("%q: got %v, want an error", name, sys)
		}
	}
}
