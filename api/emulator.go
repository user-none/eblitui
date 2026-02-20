package emucore

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

	// GetRegion returns the current video region.
	GetRegion() Region

	// SetRegion changes the video region.
	SetRegion(region Region)

	// GetTiming returns FPS and scanline count for the current region.
	GetTiming() Timing

	// SetOption applies a core option change identified by key.
	SetOption(key string, value string)

	// Close releases any resources held by the emulator.
	Close()
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

// MemoryInspector enables flat address-based memory reads for RetroAchievements.
type MemoryInspector interface {
	// ReadMemory reads from a flat address into buf and returns the number
	// of bytes read. The core adapter maps flat addresses to internal memory.
	ReadMemory(addr uint32, buf []byte) uint32
}

// Memory region type constants for MemoryMapper.
const (
	MemorySaveRAM   = iota // Maps to RETRO_MEMORY_SAVE_RAM
	MemorySystemRAM        // Maps to RETRO_MEMORY_SYSTEM_RAM
)

// MemoryRegion describes a named memory region and its size.
type MemoryRegion struct {
	Type int
	Size int
}

// MemoryMapper enables libretro-style named memory region access.
type MemoryMapper interface {
	// MemoryMap returns a list of available memory regions with sizes.
	MemoryMap() []MemoryRegion

	// ReadRegion returns a copy of the specified memory region.
	ReadRegion(regionType int) []byte

	// WriteRegion writes data to the specified memory region.
	WriteRegion(regionType int, data []byte)
}
