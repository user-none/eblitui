// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package rating

import (
	"slices"
	"testing"

	"github.com/user-none/eblitui/desktop/content"
)

// scores returns reviewer scores without categories
func scores(v ...float64) []content.CriticScore {
	out := make([]content.CriticScore, len(v))
	for i, s := range v {
		out[i] = content.CriticScore{Score: s}
	}
	return out
}

var (
	gamepro = content.CriticRating{Source: "GamePro", Region: "USA", Scale: 5, Scores: []content.CriticScore{
		{Category: "Graphics", Score: 4}, {Category: "Sound", Score: 4.5},
		{Category: "Control", Score: 4}, {Category: "FunFactor", Score: 4.5},
	}}
	egm     = content.CriticRating{Source: "EGM", Region: "USA", Scale: 10, Scores: scores(8.5, 8, 7, 8)}
	ssmUK   = content.CriticRating{Source: "SSM UK", Region: "Europe", Scale: 100, Scores: scores(90)}
	maniac  = content.CriticRating{Source: "MAN!AC", Region: "Europe", Scale: 100, Scores: scores(73)}
	famitsu = content.CriticRating{Source: "Famitsu", Region: "Japan", Scale: 10, Scores: scores(8, 7, 7, 9)}
	ssmJP   = content.CriticRating{Source: "SSM JP Readers", Region: "Japan", Scale: 10, Scores: scores(8.6614)}
)

// gameproText is how the gamepro review is shown
const gameproText = "Graphics: 4 / 5, Sound: 4.5 / 5, Control: 4 / 5, FunFactor: 4.5 / 5"

func TestRows(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ratings []content.CriticRating
		regions []string
		want    []CriticRow
	}{
		{"each source formatted", []content.CriticRating{famitsu, ssmJP}, []string{"Japan"},
			[]CriticRow{{"Famitsu", "31 / 40"}, {"SSM JP Readers", "8.6614 / 10"}}},
		{"sources in fixed order", []content.CriticRating{egm, gamepro}, []string{"USA"},
			[]CriticRow{{"GamePro", gameproText}, {"EGM", "7.9 / 10"}}},
		{"grouped by region in region order", []content.CriticRating{gamepro, ssmUK, maniac, famitsu, ssmJP}, []string{"Europe", "USA", "Japan"},
			[]CriticRow{{"SSM UK", "90%"}, {"MAN!AC", "73%"}, {"GamePro", gameproText}, {"Famitsu", "31 / 40"}, {"SSM JP Readers", "8.6614 / 10"}}},
		{"european country groups under Europe", []content.CriticRating{ssmUK}, []string{"Germany"},
			[]CriticRow{{"SSM UK", "90%"}}},
		{"review for a region the game lacks", []content.CriticRating{ssmUK}, []string{"USA"}, nil},
		{"unsupported region", []content.CriticRating{gamepro}, []string{"Brazil"}, nil},
		{"unknown source", []content.CriticRating{{Source: "Saturn Fan", Region: "Japan", Scale: 10, Scores: scores(7)}}, []string{"Japan"}, nil},
		{"egm without four reviewers", []content.CriticRating{{Source: "EGM", Region: "USA", Scale: 10, Scores: scores(7, 7)}}, []string{"USA"}, nil},
		{"famitsu without four reviewers", []content.CriticRating{{Source: "Famitsu", Region: "Japan", Scale: 10, Scores: scores(3.3)}}, []string{"Japan"}, nil},
		{"gamepro without categories", []content.CriticRating{{Source: "GamePro", Region: "USA", Scale: 5, Scores: scores(4, 4, 4, 4)}}, []string{"USA"}, nil},
		{"gamepro without scores", []content.CriticRating{{Source: "GamePro", Region: "USA", Scale: 5}}, []string{"USA"}, nil},
		{"egm average to one decimal", []content.CriticRating{{Source: "EGM", Region: "USA", Scale: 10, Scores: scores(8, 8, 8, 8)}}, []string{"USA"},
			[]CriticRow{{"EGM", "8.0 / 10"}}},
		{"none", nil, []string{"Japan"}, nil},
	} {
		if got := Critic(tc.ratings, tc.regions); !slices.Equal(got, tc.want) {
			t.Errorf("%s: Critic = %q, want %q", tc.name, got, tc.want)
		}
	}
}
