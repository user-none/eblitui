// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package rating

import (
	"strconv"
	"strings"

	"github.com/user-none/eblitui/desktop/content"
	"github.com/user-none/eblitui/desktop/region"
)

// criticSources lists the shown critic sources in the order they are
// listed. format returns a review's score text, or false when its scores
// are not the shape the source prints.
var criticSources = []struct {
	name   string
	format func(r content.CriticRating) (string, bool)
}{
	{"GamePro", categories},
	{"EGM", average(4)},
	{"SSM UK", percent},
	{"MAN!AC", percent},
	{"Famitsu", total(4)},
	{"SSM JP Readers", single},
}

// number returns v without trailing zeros, such as "4.5" or "90"
func number(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// outOf shows a score out of a maximum, such as "4.5 / 5"
func outOf(score, max string) string {
	return score + " / " + max
}

// categories shows every category's score out of the scale, in the order
// printed, such as "Graphics: 4 / 5, Sound: 4.5 / 5". Every score must
// name its category.
func categories(r content.CriticRating) (string, bool) {
	if len(r.Scores) == 0 {
		return "", false
	}
	parts := make([]string, 0, len(r.Scores))
	for _, s := range r.Scores {
		if s.Category == "" {
			return "", false
		}
		parts = append(parts, s.Category+": "+outOf(number(s.Score), number(r.Scale)))
	}
	return strings.Join(parts, ", "), true
}

// average shows the average of n reviewer scores to one decimal out of
// the scale, such as "7.9 / 10"
func average(n int) func(content.CriticRating) (string, bool) {
	return func(r content.CriticRating) (string, bool) {
		if len(r.Scores) != n {
			return "", false
		}
		var sum float64
		for _, s := range r.Scores {
			sum += s.Score
		}
		return outOf(strconv.FormatFloat(sum/float64(n), 'f', 1, 64), number(r.Scale)), true
	}
}

// total shows the sum of n reviewer scores out of n times the scale, such
// as "31 / 40"
func total(n int) func(content.CriticRating) (string, bool) {
	return func(r content.CriticRating) (string, bool) {
		if len(r.Scores) != n {
			return "", false
		}
		var sum float64
		for _, s := range r.Scores {
			sum += s.Score
		}
		return outOf(number(sum), number(r.Scale*float64(n))), true
	}
}

// percent shows a single score out of 100 as a percentage, such as "90%"
func percent(r content.CriticRating) (string, bool) {
	if len(r.Scores) != 1 || r.Scale != 100 {
		return "", false
	}
	return number(r.Scores[0].Score) + "%", true
}

// single shows a single score out of the scale, such as "8.6614 / 10"
func single(r content.CriticRating) (string, bool) {
	if len(r.Scores) != 1 {
		return "", false
	}
	return outOf(number(r.Scores[0].Score), number(r.Scale)), true
}

// CriticRow is one shown critic rating: its source and its score text
type CriticRow struct {
	Source string
	Score  string
}

// Critic returns a game's critic ratings, one per source, grouped by
// supported region in the order of the game's metadata regions and in a
// fixed source order within a region, such as Famitsu "31 / 40" then SSM
// JP Readers "8.6614 / 10". A rating with an unknown source, a region the
// game does not have, or scores not shaped as its source prints is left
// out.
func Critic(ratings []content.CriticRating, regions []string) []CriticRow {
	var rows []CriticRow
	for _, code := range region.Codes(regions) {
		for _, src := range criticSources {
			for _, r := range ratings {
				if r.Source != src.name || region.Code(r.Region) != code {
					continue
				}
				if t, ok := src.format(r); ok {
					rows = append(rows, CriticRow{Source: src.name, Score: t})
				}
				break
			}
		}
	}
	return rows
}
