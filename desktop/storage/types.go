// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// Config represents the application configuration stored in config.json
type Config struct {
	Version           int                     `json:"version"`
	Theme             string                  `json:"theme"`    // Theme name: "Default", "Dark", "Light", "Retro"
	FontSize          int                     `json:"fontSize"` // 10-32, default 14
	Video             VideoConfig             `json:"video"`
	Audio             AudioConfig             `json:"audio"`
	Window            WindowConfig            `json:"window"`
	Library           LibraryView             `json:"library"`
	Shaders           ShaderConfig            `json:"shaders"`
	Rewind            RewindConfig            `json:"rewind"`
	Input             InputConfig             `json:"input"`
	CoreOptions       map[string]string       `json:"coreOptions,omitempty"`
	BIOS              map[string]BIOSConfig   `json:"bios,omitempty"`
	RetroAchievements RetroAchievementsConfig `json:"retroAchievements"`
}

// BIOSConfig stores the user's BIOS configuration for one BIOSOption.
type BIOSConfig struct {
	Active string            `json:"active,omitempty"` // Active variant label, empty = none
	Files  map[string]string `json:"files,omitempty"`  // variant label -> file path
}

// InputConfig contains input binding overrides and controller configuration.
// Empty/nil maps mean "use adaptor defaults." Only user overrides are stored.
type InputConfig struct {
	P1Keyboard         map[string]string   `json:"p1Keyboard,omitempty"`         // button name -> key name override
	DisableAnalogStick bool                `json:"disableAnalogStick,omitempty"` // disable analog stick mirroring d-pad
	RumbleLevel        int                 `json:"rumbleLevel,omitempty"`        // 0=off, 1=1x, 2=2x, 3=3x, 4=4x, 5=Max. Intensity/duration multiplier
	Profiles           []ControllerProfile `json:"profiles,omitempty"`           // named controller mappings, in creation order
	Players            []PlayerConfig      `json:"players,omitempty"`            // player slot -> profile assignment
	PadMappings        map[string]string   `json:"padMappings,omitempty"`        // SDL GUID -> generated standard-layout mapping line
}

// ControllerProfile is a named controller button mapping for a controller
// model. The model is identified by SDLID+Controller; multiple profiles can
// exist for the same model (e.g. third-party pads reported as the same
// device). ID is immutable and referenced by PlayerConfig; Name is the
// user-editable display name, unique per model.
type ControllerProfile struct {
	ID         string            `json:"id"`
	SDLID      string            `json:"sdlId"`
	Controller string            `json:"controller"`
	Name       string            `json:"name"`
	Bindings   map[string]string `json:"bindings,omitempty"` // button name -> pad button name override
}

// PlayerConfig holds the controller assignment for one player slot.
// An empty Profile means no controller is assigned to that player.
type PlayerConfig struct {
	Profile string `json:"profile,omitempty"` // ControllerProfile.ID
}

// MatchesPad reports whether this profile's controller model matches the
// given pad identity (SDL ID and name).
func (p *ControllerProfile) MatchesPad(sdlID, name string) bool {
	return p.SDLID == sdlID && p.Controller == name
}

// ValidPadMapping reports whether guid and line form a well-formed pad
// mapping entry: the GUID is 32 lowercase hex characters, the line starts
// with the GUID, and the line is a single line.
func ValidPadMapping(guid, line string) bool {
	if len(guid) != 32 {
		return false
	}
	for _, c := range guid {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	if !strings.HasPrefix(line, guid+",") {
		return false
	}
	return !strings.ContainsAny(line, "\n\r")
}

// ProfileByID returns the profile with the given ID, or nil if not found.
func (c *InputConfig) ProfileByID(id string) *ControllerProfile {
	if id == "" {
		return nil
	}
	for i := range c.Profiles {
		if c.Profiles[i].ID == id {
			return &c.Profiles[i]
		}
	}
	return nil
}

// PlayerProfile returns the profile assigned to the given player slot,
// or nil if the slot is unassigned or the reference is dangling.
func (c *InputConfig) PlayerProfile(player int) *ControllerProfile {
	if player < 0 || player >= len(c.Players) {
		return nil
	}
	return c.ProfileByID(c.Players[player].Profile)
}

// NewProfileID returns a random 8-hex-char profile ID unique within this
// input config.
func (c *InputConfig) NewProfileID() string {
	for {
		var b [4]byte
		rand.Read(b[:])
		id := hex.EncodeToString(b[:])
		if c.ProfileByID(id) == nil {
			return id
		}
	}
}

// RetroAchievementsConfig contains RetroAchievements integration settings
type RetroAchievementsConfig struct {
	Enabled                 bool   `json:"enabled"`
	EncoreMode              bool   `json:"encoreMode"`              // Allow re-triggering unlocked achievements
	UnlockSound             bool   `json:"unlockSound"`             // Play sound on achievement unlock
	ShowNotification        bool   `json:"showNotification"`        // Show popup notification on achievement unlock
	AutoScreenshot          bool   `json:"autoScreenshot"`          // Take screenshot on achievement unlock
	SuppressHardcoreWarning bool   `json:"suppressHardcoreWarning"` // Hide "Unknown Emulator" hardcore warning
	SpectatorMode           bool   `json:"spectatorMode"`           // Watch achievements without submitting unlocks
	Username                string `json:"username,omitempty"`
	Token                   string `json:"token,omitempty"` // Auth token (password is never stored)
}

// VideoConfig contains video-related settings
type VideoConfig struct {
	AspectRatio string `json:"aspectRatio"`
}

// ValidAspectRatios lists the allowed aspect ratio mode values
var ValidAspectRatios = []string{"dar", "4:3", "1:1", "stretch"}

// AspectRatioDisplayName returns a user-facing label for the given mode.
func AspectRatioDisplayName(mode string) string {
	switch mode {
	case "dar":
		return "Standard (DAR)"
	case "4:3":
		return "4:3"
	case "1:1":
		return "1:1 (PAR)"
	case "stretch":
		return "Stretch"
	default:
		return "Standard (DAR)"
	}
}

// ShaderConfig contains shader effect settings
type ShaderConfig struct {
	UIShaders   []string `json:"uiShaders"`   // Ordered list of shader IDs for UI context
	GameShaders []string `json:"gameShaders"` // Ordered list of shader IDs for Game context
}

// RewindConfig contains rewind feature settings
type RewindConfig struct {
	Enabled      bool `json:"enabled"`      // Default: false (off due to RAM usage)
	BufferSizeMB int  `json:"bufferSizeMB"` // Default: 40
	FrameStep    int  `json:"frameStep"`    // Default: 1 (capture every frame)
}

// AudioConfig contains audio-related settings
type AudioConfig struct {
	Volume          float64 `json:"volume"`
	Muted           bool    `json:"muted"`
	FastForwardMute bool    `json:"fastForwardMute"` // Mute audio during fast-forward (default: true)
}

// WindowConfig contains window position and size
type WindowConfig struct {
	Width      int  `json:"width"`
	Height     int  `json:"height"`
	X          *int `json:"x,omitempty"` // nil = OS decides position
	Y          *int `json:"y,omitempty"`
	Fullscreen bool `json:"fullscreen"`
}

// LibraryView contains library display preferences
type LibraryView struct {
	ViewMode        string `json:"viewMode"`        // "icon" or "list"
	SortBy          string `json:"sortBy"`          // "title", "lastPlayed", "playTime"
	FavoritesFilter bool   `json:"favoritesFilter"` // Show only favorites
}

// Library represents the game library stored in library.json
type Library struct {
	Version         int                   `json:"version"`
	ScanDirectories []ScanDirectory       `json:"scanDirectories"`
	ExcludedPaths   []string              `json:"excludedPaths"`
	Games           map[string]*GameEntry `json:"games"` // CRC32 hex string -> entry
}

// ScanDirectory represents a directory to scan for ROMs
type ScanDirectory struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

// GameEntry represents a single game in the library
type GameEntry struct {
	CRC32           string       `json:"crc32"`
	File            string       `json:"file,omitempty"`   // Cartridge ROM/archive path; disc games leave this empty and use Discs
	Name            string       `json:"name"`             // Full No-Intro name from RDB
	DisplayName     string       `json:"displayName"`      // Cleaned name for display (region info removed)
	Region          string       `json:"region"`           // "us", "eu", "jp" (from RDB)
	Serial          string       `json:"serial,omitempty"` // Disc/product serial (game ID), when known
	Developer       string       `json:"developer,omitempty"`
	Publisher       string       `json:"publisher,omitempty"`
	Genre           string       `json:"genre,omitempty"`
	Franchise       string       `json:"franchise,omitempty"`
	ESRBRating      string       `json:"esrbRating,omitempty"`
	ReleaseDate     string       `json:"releaseDate,omitempty"` // "Month / Year" format
	System          string       `json:"system,omitempty"`      // Variant name (e.g. "Neo Geo Pocket") - set when >1 RDB variant
	ConsoleID       int          `json:"consoleID,omitempty"`   // RetroAchievements console ID; 0 = use system default
	Favorite        bool         `json:"favorite"`              // User marked as favorite
	Missing         bool         `json:"missing"`               // true if ROM file not found
	PlayTimeSeconds int64        `json:"playTimeSeconds"`       // Total play time
	LastPlayed      int64        `json:"lastPlayed"`            // Unix timestamp
	Added           int64        `json:"added"`                 // Unix timestamp when added to library
	Settings        GameSettings `json:"settings"`              // Per-game settings

	// Discs lists the discs of a disc-based game (one entry even for a
	// single-disc game), ordered by Index. Empty for cartridge games,
	// which use File.
	Discs []GameDisc `json:"discs,omitempty"`
	// SelectedDisc is the 0-based slice index into Discs to launch.
	// Persisted so the chosen disc is remembered. Ignored when Discs is
	// empty.
	SelectedDisc int `json:"selectedDisc,omitempty"`
}

// GameDisc is one disc of a multi-disc disc-based game.
type GameDisc struct {
	Index int    `json:"index"` // 0-based disc index (DiscNumber-1)
	File  string `json:"file"`  // Path to this disc's image on disk
	Name  string `json:"name"`  // Per-disc display name
}

// GameSettings contains per-game configuration overrides
type GameSettings struct {
	SaveSlot int `json:"saveSlot,omitempty"` // Last-used save state slot (0-9)
}

// FontSizePresets lists the available font size options
var FontSizePresets = []int{10, 12, 14, 16, 18, 20, 24, 28, 32}

// ValidFontSize returns the nearest valid preset font size.
func ValidFontSize(size int) int {
	best := FontSizePresets[0]
	for _, p := range FontSizePresets {
		if abs(p-size) < abs(best-size) {
			best = p
		}
	}
	return best
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// DefaultConfig returns a new Config with default values
func DefaultConfig() *Config {
	return &Config{
		Version:  1,
		Theme:    "Default",
		FontSize: 14,
		Video: VideoConfig{
			AspectRatio: "dar",
		},
		Audio: AudioConfig{
			Volume:          1.0,
			Muted:           false,
			FastForwardMute: true,
		},
		Window: WindowConfig{
			Width:  900,
			Height: 650,
			X:      nil,
			Y:      nil,
		},
		Library: LibraryView{
			ViewMode:        "icon",
			SortBy:          "title",
			FavoritesFilter: false,
		},
		Shaders: ShaderConfig{
			UIShaders:   []string{},
			GameShaders: []string{},
		},
		Rewind: RewindConfig{
			Enabled:      false,
			BufferSizeMB: 40,
			FrameStep:    1,
		},
		Input: InputConfig{},
		RetroAchievements: RetroAchievementsConfig{
			Enabled:          false,
			EncoreMode:       false,
			UnlockSound:      true, // Default ON
			ShowNotification: true, // Default ON
			AutoScreenshot:   true, // Default ON
		},
	}
}

// DefaultLibrary returns a new Library with default values
func DefaultLibrary() *Library {
	return &Library{
		Version:         1,
		ScanDirectories: []ScanDirectory{},
		ExcludedPaths:   []string{},
		Games:           make(map[string]*GameEntry),
	}
}
