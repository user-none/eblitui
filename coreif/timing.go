// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package coreif

// Timing holds the frame rate and scanline count for the current region.
// CPU clocks are core-internal and not exposed here.
type Timing struct {
	FPS       int
	Scanlines int
}
