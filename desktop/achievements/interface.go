// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package achievements

// EmulatorInterface defines the interface for emulator memory access.
// This matches the coreif.MemoryInspector interface, decoupling the
// achievement manager from the concrete emulator type. The core adapter
// handles mapping flat addresses to internal memory regions.
type EmulatorInterface interface {
	ReadMemory(addr uint32, buf []byte) uint32
}
