// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package rating

import (
	"testing"

	"github.com/user-none/eblitui/desktop/content"
)

func TestAge(t *testing.T) {
	esrb := content.AgeRating{System: "ESRB", Rating: "K-A"}
	elspa := content.AgeRating{System: "ELSPA", Rating: "15+"}
	for _, tc := range []struct {
		name    string
		ratings []content.AgeRating
		regions []string
		want    string
	}{
		{"region order", []content.AgeRating{esrb, elspa}, []string{"USA", "Europe"}, "ESRB: K-A, ELSPA: 15+"},
		{"region order reversed", []content.AgeRating{esrb, elspa}, []string{"Germany", "USA"}, "ELSPA: 15+, ESRB: K-A"},
		{"sega text", []content.AgeRating{{System: "Sega", Rating: "All"}}, []string{"Japan"}, "Sega: All Ages"},
		{"sega 18", []content.AgeRating{{System: "Sega", Rating: "18"}}, []string{"Japan"}, "Sega: 18+ Recommended"},
		{"sega x", []content.AgeRating{{System: "Sega", Rating: "X"}}, []string{"Japan"}, "Sega: Adults Only (18+)"},
		{"sega v", []content.AgeRating{{System: "Sega", Rating: "V"}}, []string{"Japan"}, "Sega: Violent Content"},
		{"region not in game", []content.AgeRating{esrb, elspa}, []string{"USA"}, "ESRB: K-A"},
		{"unsupported region", []content.AgeRating{esrb}, []string{"Brazil"}, ""},
		{"countries listed once", []content.AgeRating{elspa}, []string{"UK", "Germany", "Europe"}, "ELSPA: 15+"},
		{"unknown system", []content.AgeRating{{System: "BBFC", Rating: "15"}}, []string{"UK"}, ""},
		{"unknown code", []content.AgeRating{{System: "ESRB", Rating: "RP"}}, []string{"USA"}, ""},
		{"none", nil, []string{"Japan"}, ""},
	} {
		if got := Age(tc.ratings, tc.regions); got != tc.want {
			t.Errorf("%s: Age = %q, want %q", tc.name, got, tc.want)
		}
	}
}
