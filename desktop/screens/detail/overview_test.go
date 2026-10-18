// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package detail

import (
	"errors"
	"testing"

	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/go-rcheevos"
)

func TestOverviewFinishLoadDropsRetiredLookup(t *testing.T) {
	cb := &fakeCallback{}
	o := NewOverviewSection(cb, storage.DefaultLibrary(), nil, 0, func() int { return 800 })

	// Each SetGame retires the lookups started before it
	o.SetGame(&storage.GameEntry{CRC32: "A"}, storage.FileStatus{})
	staleGen := o.loadGen
	o.SetGame(&storage.GameEntry{CRC32: "B"}, storage.FileStatus{})
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

	o.SetGame(&storage.GameEntry{CRC32: "A", Discs: []storage.GameDisc{{File: "a.cue"}}}, storage.FileStatus{})

	// No manager: state is reset and no lookup starts
	if o.achLoading || o.achFound || o.loadGen != 1 {
		t.Errorf("loading %v found %v gen %d, want reset with generation 1", o.achLoading, o.achFound, o.loadGen)
	}
}
