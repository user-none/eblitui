package storage

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

// InputConfig contains input binding overrides for P1 keyboard and controller.
// Empty/nil maps mean "use adaptor defaults." Only user overrides are stored.
type InputConfig struct {
	P1Keyboard         map[string]string `json:"p1Keyboard,omitempty"`         // button name -> key name override
	P1Controller       map[string]string `json:"p1Controller,omitempty"`       // button name -> pad button name override
	DisableAnalogStick bool              `json:"disableAnalogStick,omitempty"` // disable analog stick mirroring d-pad
	RumbleLevel        int               `json:"rumbleLevel,omitempty"`        // 0=off, 1=1x, 2=2x, 3=3x, 4=4x, 5=Max. Intensity/duration multiplier
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
	File            string       `json:"file"`        // Path to ROM file or archive on disk
	Name            string       `json:"name"`        // Full No-Intro name from RDB
	DisplayName     string       `json:"displayName"` // Cleaned name for display (region info removed)
	Region          string       `json:"region"`      // "us", "eu", "jp" (from RDB)
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
}

// GameSettings contains per-game configuration overrides
type GameSettings struct {
	RegionOverride string `json:"regionOverride,omitempty"` // "", "ntsc", "pal"
	SaveSlot       int    `json:"saveSlot,omitempty"`       // Last-used save state slot (0-9)
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
