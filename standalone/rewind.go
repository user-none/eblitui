//go:build !libretro

package standalone

import (
	"fmt"

	emucore "github.com/user-none/eblitui/api"
)

// RewindBuffer stores serialized emulator states in a ring buffer
// for rewinding gameplay. States are captured every frameStep frames
// and can be popped in reverse order (LIFO) to step backwards.
type RewindBuffer struct {
	buffer    [][]byte // Ring buffer slots
	head      int      // Next write position
	count     int      // Number of valid entries
	capacity  int      // Max entries
	frameStep int      // Capture every N frames
	frameTick int      // Frame counter for step timing
	rewinding bool     // Currently in rewind mode
}

// NewRewindBuffer allocates a ring buffer sized to fit bufferSizeMB
// worth of serialized states, each stateSize bytes.
func NewRewindBuffer(bufferSizeMB, frameStep, stateSize int) *RewindBuffer {
	if stateSize <= 0 || bufferSizeMB <= 0 || frameStep <= 0 {
		return nil
	}
	capacity := (bufferSizeMB * 1024 * 1024) / stateSize
	if capacity == 0 {
		return nil
	}
	return &RewindBuffer{
		buffer:    make([][]byte, capacity),
		capacity:  capacity,
		frameStep: frameStep,
	}
}

// Capture serializes the emulator state and stores it in the ring buffer.
// Only captures every frameStep frames. Should be called after RunFrame.
func (rb *RewindBuffer) Capture(saveStater emucore.SaveStater) error {
	rb.frameTick++
	if rb.frameTick < rb.frameStep {
		return nil
	}
	rb.frameTick = 0

	state, err := saveStater.Serialize()
	if err != nil {
		return fmt.Errorf("rewind capture: %w", err)
	}

	rb.buffer[rb.head] = state
	rb.head = (rb.head + 1) % rb.capacity
	if rb.count < rb.capacity {
		rb.count++
	}

	return nil
}

// Rewind pops count states from the buffer and deserializes the last one.
// After deserializing, RunFrame is called to regenerate the framebuffer
// since the serialized state doesn't include rendered pixels.
// Returns false if the buffer is empty.
func (rb *RewindBuffer) Rewind(emu emucore.Emulator, saveStater emucore.SaveStater, count int) bool {
	if rb.count == 0 {
		return false
	}

	if count > rb.count {
		count = rb.count
	}

	// Move head back by count entries
	rb.head = (rb.head - count + rb.capacity) % rb.capacity
	rb.count -= count

	// Deserialize the state at the new head position
	// head points to the next write slot, so the most recent entry is head-1
	idx := (rb.head - 1 + rb.capacity) % rb.capacity
	state := rb.buffer[idx]
	if state == nil {
		return false
	}

	if err := saveStater.Deserialize(state); err != nil {
		return false
	}

	// RunFrame to regenerate the framebuffer from video state
	emu.RunFrame()

	return true
}

// Reset clears the buffer. Call on game launch or save state load.
func (rb *RewindBuffer) Reset() {
	rb.head = 0
	rb.count = 0
	rb.frameTick = 0
	// Clear references to allow GC of old state data
	for i := range rb.buffer {
		rb.buffer[i] = nil
	}
}

// IsRewinding returns whether the buffer is currently in rewind mode.
func (rb *RewindBuffer) IsRewinding() bool {
	return rb.rewinding
}

// SetRewinding sets the rewind mode flag.
func (rb *RewindBuffer) SetRewinding(v bool) {
	rb.rewinding = v
}

// Count returns the number of valid entries in the buffer.
func (rb *RewindBuffer) Count() int {
	return rb.count
}

// Capacity returns the maximum number of entries the buffer can hold.
func (rb *RewindBuffer) Capacity() int {
	return rb.capacity
}

// rewindItemsForHoldDuration returns the number of rewind steps to take
// based on how many frames the R key has been held. This provides an
// acceleration curve: slow at first, faster the longer the key is held.
//
// Hold Duration (frames) | Items  | Effective rate
// 1 (just pressed)       | 1      | single step
// 2-15 (~0.25s)          | 0 or 1 | ~15/sec (every 4th frame)
// 16-30 (~0.5s)          | 0 or 1 | ~30/sec (every 2nd frame)
// 31-60 (~1s)            | 1      | 60/sec (every frame)
// 61+ (>1s)              | 2      | 120/sec
func rewindItemsForHoldDuration(holdDuration int) int {
	switch {
	case holdDuration <= 0:
		return 0
	case holdDuration == 1:
		return 1
	case holdDuration <= 15:
		// ~15 steps/sec: fire every 4th frame
		if holdDuration%4 == 0 {
			return 1
		}
		return 0
	case holdDuration <= 30:
		// ~30 steps/sec: fire every 2nd frame
		if holdDuration%2 == 0 {
			return 1
		}
		return 0
	case holdDuration <= 60:
		return 1
	default:
		return 2
	}
}
