// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package coreif

// Emulator is the core interface that every emulator adapter must implement.
type Emulator interface {
	// RunFrame executes one frame of emulation.
	RunFrame()

	// GetFramebuffer returns the current frame as RGBA pixel data.
	GetFramebuffer() []byte

	// GetFramebufferStride returns bytes per row in the framebuffer.
	GetFramebufferStride() int

	// GetActiveHeight returns the current active display height in pixels.
	GetActiveHeight() int

	// GetAudioSamples returns stereo 16-bit PCM audio samples for the frame.
	GetAudioSamples() []int16

	// SetInput sets controller state as a button bitmask for the given player.
	SetInput(player int, buttons uint32)

	// GetTiming returns FPS and scanline count for the current region.
	GetTiming() Timing

	// SetOption applies a core option change identified by key.
	SetOption(key string, value string)

	// SetRom provides cartridge ROM data. Called after CreateEmulator and
	// before Start(). Disc-based cores ignore this and receive content via
	// SetDisc instead.
	SetRom(data []byte)

	// SetDisc provides a streaming disc reader for disc-based cores. Called
	// after CreateEmulator and before Start(). Cartridge cores ignore this.
	SetDisc(disc DiscReader)

	// SetBIOS provides BIOS data for the given key. Called after
	// CreateEmulator and before Start(). Cores without BIOS ignore this.
	// Returns an error if the data is invalid for the given key.
	SetBIOS(key string, data []byte) error

	// Start finalizes emulator state after all options are applied.
	// Must be called after SetOption and before the first RunFrame.
	Start()

	// Close releases any resources held by the emulator.
	Close()
}

// AspectProvider is an optional interface a core may implement when its
// pixel aspect ratio depends on the video mode (e.g. consoles that
// switch horizontal resolution). When implemented, the UI uses this
// per-frame value instead of the static SystemInfo.PixelAspectRatio.
// Implementations must be cheap to call: the value should be cached and
// recomputed only on a mode change, not derived per call.
type AspectProvider interface {
	// PixelAspectRatio returns the current pixel aspect ratio.
	PixelAspectRatio() float64
}

// SaveStater enables save states, rewind, and auto-save.
type SaveStater interface {
	// Serialize captures the complete emulator state.
	Serialize() ([]byte, error)

	// Deserialize restores emulator state from previously serialized data.
	Deserialize(data []byte) error
}

// BatterySaver enables SRAM persistence for battery-backed saves.
type BatterySaver interface {
	// HasSRAM reports whether the loaded ROM uses battery-backed save.
	HasSRAM() bool

	// GetSRAM returns a copy of the current SRAM contents.
	GetSRAM() []byte

	// SetSRAM loads SRAM contents into the emulator.
	SetSRAM(data []byte)
}

// Memory provides canonical native bus address based access to the
// console's emulated RAM.
type Memory interface {
	// ReadMemory reads from a native bus address into buf and returns
	// the number of bytes read.
	ReadMemory(addr uint32, buf []byte) uint32

	// WriteMemory writes data to a native bus address and returns the
	// number of bytes written.
	WriteMemory(addr uint32, data []byte) uint32

	// Regions describes the accessible bus regions in canonical
	// addresses. The table is static per machine.
	Regions() []BusRegion

	// ReadMemoryFlat reads from the console's flat memory convention
	// (the RetroAchievements layout) into buf and returns the number
	// of bytes read. Reads are contiguous across region boundaries
	// within the flat space.
	ReadMemoryFlat(off uint32, buf []byte) uint32

	// WriteMemoryFlat writes data to the console's flat memory
	// convention and returns the number of bytes written, matching
	// ReadMemoryFlat's layout and boundary behavior.
	WriteMemoryFlat(off uint32, data []byte) uint32
}

// BusRegion describes one region of the console's native address bus in
// canonical addresses.
type BusRegion struct {
	Name  string
	Start uint32 // native bus address of the region's first byte
	Size  uint32 // region size in bytes
}
