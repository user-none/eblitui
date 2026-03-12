package coreif

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

// BIOSVariant describes a known BIOS dump.
type BIOSVariant struct {
	Label    string // Display name, e.g. "US v1.0"
	SHA256   string // Expected SHA256 hex
	Filename string // Default filename for system directory lookup
}

// BIOSOption describes a BIOS slot that a core supports.
type BIOSOption struct {
	Key      string        // Unique key, e.g. "main_bios"
	Label    string        // Display label, e.g. "System BIOS"
	Required bool          // true = core cannot run without it
	Variants []BIOSVariant // Known BIOS dumps
}

// HasKnownHashes returns true if any variant has a non-empty SHA256.
// When true, files must match a known hash to be accepted.
func (o BIOSOption) HasKnownHashes() bool {
	for _, v := range o.Variants {
		if v.SHA256 != "" {
			return true
		}
	}
	return false
}

// MetadataVariant pairs an RDB database with its thumbnail repository.
// Systems that span multiple libretro databases (e.g. NGP + NGPC) have
// multiple variants so metadata lookups can search all of them.
type MetadataVariant struct {
	Name          string // Display name, e.g. "Neo Geo Pocket"
	RDBName       string // e.g. "SNK - Neo Geo Pocket"
	ThumbnailRepo string // e.g. "SNK_-_Neo_Geo_Pocket"
}

// SystemInfo describes an emulator system for UI configuration.
type SystemInfo struct {
	Name             string
	ConsoleName      string
	Extensions       []string
	ScreenWidth      int
	MaxScreenHeight  int
	PixelAspectRatio float64
	SampleRate       int
	Buttons          []Button
	Players          int
	CoreOptions      []CoreOption
	MetadataVariants []MetadataVariant
	DataDirName      string
	ConsoleID        int
	CoreName         string
	CoreVersion      string
	SerializeSize    int
	BigEndianMemory  bool // true for big-endian CPUs (e.g. 68K)
	BIOSOptions      []BIOSOption
}

// DisplayAspectRatio computes the PAR-corrected display aspect ratio
// from frame dimensions and the system's pixel aspect ratio.
func DisplayAspectRatio(width, height int, par float64) float64 {
	return (float64(width) / float64(height)) * par
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
