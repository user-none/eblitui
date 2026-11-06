// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package content defines the per-game data an emulator supplies to the
// desktop UI: game metadata and cheats. The JSON tags define the format
// emulators store their entries in. Entries are keyed by game ID, the
// product number read from the disc for disc systems.
package content

// Publisher is one publisher of a title and the regions, as named in
// GameMetadata.Regions, it published the title in.
type Publisher struct {
	Name    string   `json:"name"`
	Regions []string `json:"regions"`
}

// AgeRating is one age rating of a title: the rating system, such as
// "ESRB", and the rating code as printed, such as "K-A".
type AgeRating struct {
	System string `json:"system"`
	Rating string `json:"rating"`
}

// GameMetadata describes one title.
type GameMetadata struct {
	Serial      string      `json:"serial"`
	Title       string      `json:"title"`
	NativeTitle string      `json:"native_title"`
	Category    string      `json:"category"`
	Regions     []string    `json:"regions"`
	Languages   []string    `json:"languages"`
	Barcodes    []string    `json:"barcodes"`
	ReleaseDate string      `json:"release_date"` // YYYY-MM-DD, or YYYY when only the year is known
	Developer   string      `json:"developer"`
	Publisher   []Publisher `json:"publisher"`
	Genres      []string    `json:"genres"`
	MaxPlayers  int         `json:"max_players"`
	Cooperative bool        `json:"cooperative"`
	AgeRatings  []AgeRating `json:"age_ratings"`
	Overview    string      `json:"overview"`
}

// MetadataSource supplies game metadata.
type MetadataSource interface {
	// GameMetadata returns the metadata for a game ID and true, or false
	// when there is no entry for it.
	GameMetadata(gameID string) (GameMetadata, bool)

	// AllGameMetadata returns every entry keyed by game ID. Each call
	// returns a new map that the caller owns.
	AllGameMetadata() map[string]GameMetadata
}

// GameCheat is one cheat: its name and its code string, in a cheat code
// format the desktop decodes.
type GameCheat struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

// GameCheats holds the cheats for one title. MasterCode is the code a
// cheat cartridge needs to hook the game, empty when none is known; the
// desktop does not apply it.
type GameCheats struct {
	Serial     string      `json:"serial"`
	MasterCode string      `json:"master_code,omitempty"`
	Cheats     []GameCheat `json:"cheats"`
}

// CheatsSource supplies database cheats.
type CheatsSource interface {
	// GameCheats returns the cheats for a game ID and true, or false when
	// there is no entry for it.
	GameCheats(gameID string) (GameCheats, bool)
}

// RumbleSource supplies .erumble rumble files (see the rumble module).
type RumbleSource interface {
	// GameRumble returns the rumble file named name and true, or false
	// when there is none. name is a game ID for the file shared by all of
	// the game's discs, or <gameID>.disc<n> for a file for disc n only.
	GameRumble(name string) ([]byte, bool)
}
