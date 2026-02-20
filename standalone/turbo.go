//go:build !libretro

package standalone

import "sync"

// TurboState holds fast-forward state shared between the Ebiten thread
// (which sets it from key input) and the emulation goroutine (which reads it).
type TurboState struct {
	mu         sync.Mutex
	multiplier int // 1 (normal), 2, or 3
}

// CycleMultiplier advances through Off(1) → 2x → 3x → Off(1), returning the new value.
// Called from the Ebiten thread.
func (ts *TurboState) CycleMultiplier() int {
	ts.mu.Lock()
	switch ts.multiplier {
	case 1:
		ts.multiplier = 2
	case 2:
		ts.multiplier = 3
	default:
		ts.multiplier = 1
	}
	m := ts.multiplier
	ts.mu.Unlock()
	return m
}

// Read returns the current multiplier.
// Called from the emulation goroutine.
func (ts *TurboState) Read() int {
	ts.mu.Lock()
	m := ts.multiplier
	ts.mu.Unlock()
	return m
}

// averageAudio downsamples concatenated stereo int16 samples from N frames
// into 1 frame's worth by averaging corresponding sample pairs.
func averageAudio(combined []int16, multiplier int) []int16 {
	if multiplier <= 1 || len(combined) == 0 {
		return combined
	}

	frameLen := len(combined) / multiplier
	// Ensure frame length is even (stereo pairs)
	frameLen &^= 1

	if frameLen == 0 {
		return nil
	}

	out := make([]int16, frameLen)
	for i := 0; i < frameLen; i++ {
		var acc int32
		for f := 0; f < multiplier; f++ {
			acc += int32(combined[f*frameLen+i])
		}
		out[i] = int16(acc / int32(multiplier))
	}

	return out
}
