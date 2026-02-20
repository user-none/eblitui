//go:build !libretro

package standalone

// AppState represents the current state of the application
type AppState int

const (
	// StateLibrary is the main library screen showing all games
	StateLibrary AppState = iota
	// StateDetail shows information about a selected game
	StateDetail
	// StateSettings shows application settings
	StateSettings
	// StateScanProgress shows ROM scanning progress
	StateScanProgress
	// StateError shows a startup error (corrupted config)
	StateError
	// StatePlaying is active gameplay
	StatePlaying
)

// String returns the string representation of the state
func (s AppState) String() string {
	switch s {
	case StateLibrary:
		return "Library"
	case StateDetail:
		return "Detail"
	case StateSettings:
		return "Settings"
	case StateScanProgress:
		return "ScanProgress"
	case StateError:
		return "Error"
	case StatePlaying:
		return "Playing"
	default:
		return "Unknown"
	}
}
