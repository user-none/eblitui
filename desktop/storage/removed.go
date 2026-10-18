// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"errors"
	"log"
	"os"
)

// removedVersion is the current removed.json format version
const removedVersion = 1

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
// removed.json and saves both files. removed.json is written first so a
// failed write never loses a game's user data. Keys not in the library
// are ignored.
func (lib *Library) RemoveGames(keys ...string) error {
	var games []*GameEntry
	for _, key := range keys {
		if game := lib.GetGame(key); game != nil {
			games = append(games, game)
		}
	}
	if len(games) == 0 {
		return nil
	}

	removed, err := LoadRemovedGames()
	if err != nil {
		return err
	}
	for _, game := range games {
		kept := *game
		stripRemoved(&kept)
		removed.Games[kept.CRC32] = &kept
	}
	if err := SaveRemovedGames(removed); err != nil {
		return err
	}

	for _, game := range games {
		delete(lib.Games, game.CRC32)
	}
	return SaveLibrary(lib)
}

// DropRestoredGames removes every game that is back in the library from
// removed.json. A scan calls it after restoring games and saving the
// library.
func DropRestoredGames(lib *Library) error {
	removed, err := LoadRemovedGames()
	if err != nil {
		return err
	}

	changed := false
	for key := range removed.Games {
		if lib.GetGame(key) != nil {
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
