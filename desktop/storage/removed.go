// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"errors"
	"log"
	"os"
	"sync"
)

// removedVersion is the current removed.json format version
const removedVersion = 1

// removedMu serializes every read-modify-write of removed.json.
var removedMu sync.Mutex

// LoadRemovedGames loads removed.json. If the file doesn't exist or can't
// be read, it returns an empty set, and the next save overwrites the
// file; the removed games are not worth blocking removal over.
func LoadRemovedGames() (*RemovedGames, error) {
	path, err := GetRemovedGamesPath()
	if err != nil {
		return nil, err
	}

	removed := &RemovedGames{}
	if err := ReadJSON(path, removed); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("Ignoring unreadable removed games file: %v", err)
		}
		removed = &RemovedGames{Version: removedVersion}
	}
	if removed.Games == nil {
		removed.Games = make(map[string]*GameEntry)
	}

	SanitizeRemovedGames(removed)
	return removed, nil
}

// SaveRemovedGames saves removed.json atomically
func SaveRemovedGames(removed *RemovedGames) error {
	path, err := GetRemovedGamesPath()
	if err != nil {
		return err
	}

	removed.Version = removedVersion
	return AtomicWriteJSON(path, removed)
}

// RemoveGames moves the games with the given keys from the library to
// removed.json. removed.json is written before the games leave the
// library, so a failed write never loses a game's user data. Keys not in
// the library are ignored.
func (lib *Library) RemoveGames(keys ...string) error {
	removedMu.Lock()
	defer removedMu.Unlock()

	lib.mu.RLock()
	var games []GameEntry
	for _, key := range keys {
		if game, ok := lib.data.Games[key]; ok {
			games = append(games, game.clone())
		}
	}
	lib.mu.RUnlock()
	if len(games) == 0 {
		return nil
	}

	removed, err := LoadRemovedGames()
	if err != nil {
		return err
	}
	for _, game := range games {
		kept := game
		stripRemoved(&kept)
		removed.Games[kept.GameID] = &kept
	}
	if err := SaveRemovedGames(removed); err != nil {
		return err
	}

	lib.mu.Lock()
	for _, game := range games {
		delete(lib.data.Games, game.GameID)
	}
	lib.mu.Unlock()
	lib.changed()
	return nil
}

// ScanBase returns copies of the entries whose user data a scan keeps: the
// library's games, plus removed games not in the library so a scan that
// finds one again restores it.
func (lib *Library) ScanBase() map[string]*GameEntry {
	removedMu.Lock()
	removed, err := LoadRemovedGames()
	removedMu.Unlock()
	if err != nil {
		log.Printf("Failed to load removed games: %v", err)
		removed = &RemovedGames{Games: map[string]*GameEntry{}}
	}

	lib.mu.RLock()
	defer lib.mu.RUnlock()
	existing := make(map[string]*GameEntry, len(lib.data.Games)+len(removed.Games))
	for key, game := range removed.Games {
		existing[key] = game
	}
	for key, game := range lib.data.Games {
		c := game.clone()
		existing[key] = &c
	}
	return existing
}

// MergeScanned puts the games a scan found into the library, replacing the
// entries they update, and returns how many were not in the library
// before. Restored removed games count as new. Once the library holding
// them is on disk, restored games are dropped from removed.json in the
// background, so their user data is never only in memory.
func (lib *Library) MergeScanned(found map[string]*GameEntry) int {
	lib.mu.Lock()
	newGames := 0
	for key, game := range found {
		if _, ok := lib.data.Games[key]; !ok {
			newGames++
		}
		c := game.clone()
		lib.data.Games[key] = &c
	}
	lib.mu.Unlock()
	lib.changed()

	if lib.saver != nil {
		go func() {
			lib.Flush()
			if err := lib.dropRestoredGames(); err != nil {
				log.Printf("Failed to update removed games: %v", err)
			}
		}()
	}
	return newGames
}

// dropRestoredGames removes every game that is back in the library from
// removed.json.
func (lib *Library) dropRestoredGames() error {
	removedMu.Lock()
	defer removedMu.Unlock()

	removed, err := LoadRemovedGames()
	if err != nil {
		return err
	}

	changed := false
	for key := range removed.Games {
		if lib.HasGame(key) {
			delete(removed.Games, key)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return SaveRemovedGames(removed)
}

// stripRemoved clears what a removed game entry does not keep: its file
// locations, and its added time so a restored game is added when it is
// found again
func stripRemoved(game *GameEntry) {
	game.File = ""
	game.Discs = nil
	game.SelectedDisc = 0
	game.Added = 0
}
