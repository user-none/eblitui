// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import "sync"

const maxPlayers = 2

// SharedInput holds controller state as button bitmasks written by the
// Ebiten thread and read by the emulation goroutine.
type SharedInput struct {
	mu      sync.Mutex
	buttons [maxPlayers]uint32
}

// Set updates button bitmask for a player from the Ebiten thread.
func (si *SharedInput) Set(player int, buttons uint32) {
	if player < 0 || player >= maxPlayers {
		return
	}
	si.mu.Lock()
	si.buttons[player] = buttons
	si.mu.Unlock()
}

// Read returns the current button bitmasks for all players.
func (si *SharedInput) Read() [maxPlayers]uint32 {
	si.mu.Lock()
	result := si.buttons
	si.mu.Unlock()
	return result
}

// SharedFramebuffer holds pixel data written by the emulation goroutine
// and read by Ebiten's Draw() method. Uses separate write and read buffers
// so the emu goroutine can write new data while Draw uses the read copy.
type SharedFramebuffer struct {
	mu           sync.Mutex
	writePixels  []byte // Written by emu goroutine under lock
	readPixels   []byte // Snapshot copied on Read for safe external use
	stride       int
	activeHeight int
	par          float64 // Pixel aspect ratio for the current frame
}

// NewSharedFramebuffer creates a framebuffer pre-allocated for the
// given screen dimensions (width and height in pixels, 4 bytes per
// pixel). The dimensions are the starting size, not a limit: a larger
// frame grows the buffers when it arrives.
func NewSharedFramebuffer(width, height int) *SharedFramebuffer {
	size := width * height * 4
	return &SharedFramebuffer{
		writePixels: make([]byte, size),
		readPixels:  make([]byte, size),
	}
}

// Update copies framebuffer data from the emulation goroutine, or from
// the Ebiten thread while the emulation goroutine is paused (a state
// load, a rewind step). A frame larger than the write buffer, such as
// a core rendering above its native resolution or a loaded state in a
// larger video mode, grows the write buffer to hold it.
func (sf *SharedFramebuffer) Update(pixels []byte, stride, activeHeight int, par float64) {
	sf.mu.Lock()
	n := stride * activeHeight
	if n > len(pixels) {
		n = len(pixels)
	}
	if n > len(sf.writePixels) {
		sf.writePixels = make([]byte, n)
	}
	copy(sf.writePixels[:n], pixels[:n])
	sf.stride = stride
	sf.activeHeight = activeHeight
	sf.par = par
	sf.mu.Unlock()
}

// Read returns a snapshot of the current framebuffer state.
// Copies the write buffer into the read buffer under the lock,
// then returns the read buffer which is safe to use without holding the lock.
// The read buffer grows to the write buffer's size, so the returned
// pixels hold the whole frame whenever Update was given all of it.
func (sf *SharedFramebuffer) Read() (pixels []byte, stride, activeHeight int, par float64) {
	sf.mu.Lock()
	stride = sf.stride
	activeHeight = sf.activeHeight
	par = sf.par
	if len(sf.readPixels) < len(sf.writePixels) {
		sf.readPixels = make([]byte, len(sf.writePixels))
	}
	n := stride * activeHeight
	if n > len(sf.writePixels) {
		n = len(sf.writePixels)
	}
	if n > 0 {
		copy(sf.readPixels[:n], sf.writePixels[:n])
	}
	pixels = sf.readPixels
	sf.mu.Unlock()
	return
}

// EmuControl manages pause/resume/stop coordination between the Ebiten
// thread and the emulation goroutine. Both sides share one lock and wait on
// one condition, which is signaled whenever the state changes.
type EmuControl struct {
	mu       sync.Mutex
	cond     *sync.Cond
	pauseReq bool // the Ebiten thread wants the goroutine paused
	paused   bool // the goroutine is paused; set only by the goroutine
	stopped  bool // the goroutine is to exit
}

// NewEmuControl creates a new emulation control.
func NewEmuControl() *EmuControl {
	ec := &EmuControl{}
	ec.cond = sync.NewCond(&ec.mu)
	return ec
}

// RequestPause asks the emulation goroutine to pause and blocks until it
// is paused or stopped.
func (ec *EmuControl) RequestPause() {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.pauseReq = true
	for !ec.paused && !ec.stopped {
		ec.cond.Wait()
	}
}

// RequestResume lets the emulation goroutine continue. It does not wait for
// the goroutine to leave its pause.
func (ec *EmuControl) RequestResume() {
	ec.mu.Lock()
	ec.pauseReq = false
	ec.cond.Broadcast()
	ec.mu.Unlock()
}

// CheckPause is called by the emulation goroutine between frames. While a
// pause is requested it marks itself paused and waits. Returns false if the
// goroutine should exit.
func (ec *EmuControl) CheckPause() bool {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	for ec.pauseReq && !ec.stopped {
		if !ec.paused {
			ec.paused = true
			ec.cond.Broadcast()
		}
		ec.cond.Wait()
	}
	ec.paused = false
	return !ec.stopped
}

// Stop signals the emulation goroutine to exit and releases a pending
// RequestPause.
func (ec *EmuControl) Stop() {
	ec.mu.Lock()
	ec.stopped = true
	ec.pauseReq = false
	ec.cond.Broadcast()
	ec.mu.Unlock()
}

// IsPaused returns true if the emulation goroutine is currently paused.
func (ec *EmuControl) IsPaused() bool {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	return ec.paused
}
