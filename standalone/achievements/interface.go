//go:build !libretro && !ios

package achievements

// EmulatorInterface defines the interface for emulator memory access.
// This matches the emucore.MemoryInspector interface, decoupling the
// achievement manager from the concrete emulator type. The core adapter
// handles mapping flat addresses to internal memory regions.
type EmulatorInterface interface {
	ReadMemory(addr uint32, buf []byte) uint32
}
