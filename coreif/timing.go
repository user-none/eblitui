package coreif

// Timing holds the frame rate and scanline count for the current region.
// CPU clocks are core-internal and not exposed here.
type Timing struct {
	FPS       int
	Scanlines int
}
