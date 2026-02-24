package emucore

// Standard d-pad button bit positions (always bits 0-3).
const (
	ButtonUp    = 0
	ButtonDown  = 1
	ButtonLeft  = 2
	ButtonRight = 3
)

// Button describes a system-specific button with its display name
// and bit position in the input bitmask.
type Button struct {
	Name       string
	ID         int    // Bit position in the uint32 bitmask (4+)
	DefaultKey string // Default keyboard key for standalone UI (e.g., "J", "Enter")
	DefaultPad string // Default gamepad button for standalone UI (e.g., "A", "Start")
}

// CoreOptionType identifies the kind of core option.
type CoreOptionType int

const (
	CoreOptionBool CoreOptionType = iota
	CoreOptionSelect
	CoreOptionRange
)

// CoreOptionCategory identifies the settings section for a core option.
type CoreOptionCategory int

const (
	CoreOptionCategoryAudio CoreOptionCategory = iota
	CoreOptionCategoryVideo
	CoreOptionCategoryInput
	CoreOptionCategoryCore
)

// CoreOption describes a configurable core setting.
type CoreOption struct {
	Key         string
	Label       string
	Description string
	Type        CoreOptionType
	Default     string
	Values      []string           // Options for Select type
	Min         int                // Minimum for Range type
	Max         int                // Maximum for Range type
	Step        int                // Step size for Range type
	Category    CoreOptionCategory // Settings section routing
	PerGame     bool               // Whether this can be overridden per game
}

// SystemInfo describes an emulator system for UI configuration.
type SystemInfo struct {
	Name            string
	ConsoleName     string
	Extensions      []string
	ScreenWidth     int
	MaxScreenHeight int
	AspectRatio     float64
	SampleRate      int
	Buttons         []Button
	Players         int
	CoreOptions     []CoreOption
	RDBName         string
	ThumbnailRepo   string
	DataDirName     string
	ConsoleID       int
	CoreName        string
	CoreVersion     string
	SerializeSize   int
	BigEndianMemory bool // true for big-endian CPUs (e.g. 68K)
}

// CoreFactory creates emulator instances and provides system metadata.
type CoreFactory interface {
	// SystemInfo returns system metadata for UI configuration.
	SystemInfo() SystemInfo

	// CreateEmulator creates a new emulator instance with the given ROM and region.
	CreateEmulator(rom []byte, region Region) (Emulator, error)

	// DetectRegion auto-detects the region from ROM data.
	// The bool return indicates whether the region was found in the database.
	DetectRegion(rom []byte) (Region, bool)
}
