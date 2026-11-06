// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package detail

import (
	"errors"
	"testing"

	"github.com/user-none/eblitui/desktop/content"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/go-rcheevos"
)

func TestRegionText(t *testing.T) {
	for _, tc := range []struct {
		regions []string
		want    string
	}{
		{[]string{"USA"}, "US"},
		{[]string{"Europe", "USA"}, "US, EU"},
		{[]string{"Europe", "Germany", "France"}, "EU"},
		{[]string{"USA", "Brazil", "Europe", "Korea"}, "US, EU"},
		{[]string{"Japan", "USA", "Europe"}, "US, EU, JP"},
		{[]string{"Japan", "Atlantis"}, "JP"},
		{[]string{"Korea"}, "KR"},
		{[]string{"Atlantis"}, "Unknown"},
		{nil, "Unknown"},
	} {
		if got := regionText(tc.regions); got != tc.want {
			t.Errorf("regionText(%v) = %q, want %q", tc.regions, got, tc.want)
		}
	}
}

func TestPublisherText(t *testing.T) {
	soa := content.Publisher{Name: "Sega of America", Regions: []string{"USA"}}
	se := content.Publisher{Name: "Sega Europe", Regions: []string{"Europe", "Germany"}}
	tec := content.Publisher{Name: "Tec Toy", Regions: []string{"Brazil"}}
	for _, tc := range []struct {
		publishers []content.Publisher
		regions    []string
		want       string
	}{
		{[]content.Publisher{soa}, []string{"USA"}, "Sega of America"},
		{[]content.Publisher{se, soa}, []string{"USA", "Europe", "Germany"}, "Sega of America, Sega Europe"},
		{[]content.Publisher{soa, tec, se}, []string{"USA", "Brazil", "Europe"}, "Sega of America, Tec Toy, Sega Europe"},
		{[]content.Publisher{se, soa}, []string{"Germany", "USA"}, "Sega Europe, Sega of America"},
		{[]content.Publisher{tec, soa}, []string{"USA"}, "Sega of America, Tec Toy"},
		{nil, []string{"USA"}, ""},
	} {
		if got := publisherText(tc.publishers, tc.regions); got != tc.want {
			t.Errorf("publisherText(%v, %v) = %q, want %q", tc.publishers, tc.regions, got, tc.want)
		}
	}
}

func TestOverviewFinishLoadDropsRetiredLookup(t *testing.T) {
	cb := &fakeCallback{}
	o := NewOverviewSection(cb, storage.DefaultLibrary(), nil, 0, func() int { return 800 })

	// Each SetGame retires the lookups started before it
	o.SetGame(&storage.GameEntry{GameID: "A"}, storage.FileStatus{})
	staleGen := o.loadGen
	o.SetGame(&storage.GameEntry{GameID: "B"}, storage.FileStatus{})
	o.achLoading = true

	o.finishLoad(staleGen, true, &rcheevos.UserProgressEntry{NumAchievements: 5}, nil)
	if o.achFound || o.achProgress != nil || !o.achLoading || cb.rebuilds != 0 {
		t.Errorf("retired lookup stored its result: found %v progress %v loading %v rebuilds %d",
			o.achFound, o.achProgress, o.achLoading, cb.rebuilds)
	}

	o.finishLoad(o.loadGen, true, &rcheevos.UserProgressEntry{NumAchievements: 5}, nil)
	if !o.achFound || o.achProgress == nil || o.achLoading || cb.rebuilds != 1 {
		t.Errorf("current lookup not stored: found %v progress %v loading %v rebuilds %d",
			o.achFound, o.achProgress, o.achLoading, cb.rebuilds)
	}

	wantErr := errors.New("open failed")
	o.finishLoad(o.loadGen, false, nil, wantErr)
	if o.achLoadErr != wantErr || o.achFound || o.achProgress != nil || cb.rebuilds != 2 {
		t.Errorf("error not stored: err %v found %v rebuilds %d", o.achLoadErr, o.achFound, cb.rebuilds)
	}
}

func TestOverviewSetGameWithoutManager(t *testing.T) {
	o := NewOverviewSection(&fakeCallback{}, storage.DefaultLibrary(), nil, 0, func() int { return 800 })
	o.achLoading = true
	o.achFound = true

	o.SetGame(&storage.GameEntry{GameID: "A", Discs: []storage.GameDisc{{File: "a.cue"}}}, storage.FileStatus{})

	// No manager: state is reset and no lookup starts
	if o.achLoading || o.achFound || o.loadGen != 1 {
		t.Errorf("loading %v found %v gen %d, want reset with generation 1", o.achLoading, o.achFound, o.loadGen)
	}
}

func TestPlayersText(t *testing.T) {
	tests := []struct {
		max  int
		coop bool
		want string
	}{
		{0, false, ""},
		{0, true, "Co-op"},
		{1, false, "1"},
		{2, false, "1-2"},
		{4, true, "1-4, co-op"},
	}
	for _, tt := range tests {
		if got := playersText(tt.max, tt.coop); got != tt.want {
			t.Errorf("playersText(%d, %v) = %q, want %q", tt.max, tt.coop, got, tt.want)
		}
	}
}
