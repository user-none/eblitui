// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package rating defines the game ratings the desktop shows: age ratings
// from rating boards and critic ratings from publications.
//
// The age rating systems are ESRB for USA, ELSPA for Europe and Sega's own
// rating for Japan. Each system belongs to one supported region and has a
// display text for each of its rating codes. Any other system or code is
// not shown.
//
// The critic rating sources are shown as each prints its scores: GamePro's
// categories, the average of EGM's four reviewers, the total of Famitsu's
// four reviewers, and the single score of the others. Any other source is
// not shown.
package rating

import (
	"strings"

	"github.com/user-none/eblitui/desktop/content"
	"github.com/user-none/eblitui/desktop/region"
)

// ageSystem is one age rating system: its region and the display text of
// each of its rating codes
type ageSystem struct {
	region string
	text   map[string]string
}

// ageSystems maps a metadata system name to its definition. ESRB and ELSPA
// show the code as printed on the box; Sega's codes are shown as text.
var ageSystems = map[string]ageSystem{
	"ESRB": {region.US, map[string]string{
		"EC": "EC", "K-A": "K-A", "E": "E", "T": "T", "M": "M", "AO": "AO",
	}},
	"ELSPA": {region.EU, map[string]string{
		"3+": "3+", "11+": "11+", "15+": "15+", "18+": "18+",
	}},
	"Sega": {region.JP, map[string]string{
		"All": "All Ages", "18": "18+ Recommended", "X": "Adults Only (18+)", "V": "Violent Content",
	}},
}

// Age returns a game's age ratings, comma separated, such as
// "ESRB: K-A, ELSPA: 15+", in the order of the game's metadata regions:
// each rating is listed at the first region its system belongs to, and
// once. Only systems of supported regions the game has are listed, and a
// rating with an unknown system or code is left out. Returns "" when none
// is left.
func Age(ratings []content.AgeRating, regions []string) string {
	var out []string
	listed := make([]bool, len(ratings))
	for _, code := range region.Codes(regions) {
		for i, r := range ratings {
			s, ok := ageSystems[r.System]
			if listed[i] || !ok || s.region != code {
				continue
			}
			t, ok := s.text[r.Rating]
			if !ok {
				continue
			}
			listed[i] = true
			out = append(out, r.System+": "+t)
		}
	}
	return strings.Join(out, ", ")
}
