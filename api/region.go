package emucore

// Region represents a console video region.
type Region int

const (
	RegionNTSC Region = iota
	RegionPAL
)

// String returns the display name of the region.
func (r Region) String() string {
	switch r {
	case RegionNTSC:
		return "NTSC"
	case RegionPAL:
		return "PAL"
	default:
		return "Unknown"
	}
}

// Timing holds the frame rate and scanline count for the current region.
// CPU clocks are core-internal and not exposed here.
type Timing struct {
	FPS       int
	Scanlines int
}
