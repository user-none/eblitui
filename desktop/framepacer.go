package desktop

import (
	"math"
	"time"
)

// Frame pacing controller. The emulation loop is paced on an absolute-deadline
// timer rather than on audio drain. oto's mux reads the ring in coarse,
// Go-scheduled bursts (~33ms), so slaving the producer to drain made frames
// complete in bursts and jittered the frame rate. The timer gives an even
// per-frame cadence; a slow proportional controller nudges the interval from
// the audio ring fill so long-term production stays locked to the device's
// consumption rate without drift. The ring's block-on-full path remains only
// as a backpressure safety net.
const (
	// ringBufferFrames is the audio ring depth in frames. Deep enough to
	// absorb oto's bursty multi-frame reads without the producer hitting the
	// ring's block-on-full path (which would re-couple it to oto's coarse
	// read cadence). Proven at 6 in cmd/debug. The byte size is derived from
	// the sample rate and frame rate at construction.
	ringBufferFrames = 6

	pacingFillAlpha = 0.05 // low-pass coefficient on ring fill (~20-frame smoothing)
	pacingGain      = 0.05 // proportional gain on normalized fill error
	pacingMaxAdjust = 0.02 // clamp the interval to +/-2% of nominal
)

// framePacer paces the emulation loop to an absolute-deadline timer at the
// nominal frame interval, correcting long-term rate from the audio ring fill.
// It is constructed from the audio sample rate and the core frame rate (both
// fixed for the life of a run) and owns the derived audio buffer sizing.
type framePacer struct {
	baseInterval  float64 // nominal frame interval, ns
	frameInterval float64 // current (controller-adjusted) interval, ns
	frameBytesN   int     // bytes of stereo int16 audio per frame
	ringCapacity  int     // audio ring depth, bytes
	target        float64 // controller setpoint: half the ring, bytes
	smoothFill    float64 // low-passed ring fill, bytes
	nextDeadline  time.Time
}

// newFramePacer derives all timing and buffer sizing from the audio sample
// rate (Hz) and the core frame rate (fps).
func newFramePacer(sampleRate, fps int) *framePacer {
	base := float64(time.Second) / float64(fps)
	fb := frameBytes(sampleRate, fps)
	capBytes := ringBufferFrames * fb
	return &framePacer{
		baseInterval:  base,
		frameInterval: base,
		frameBytesN:   fb,
		ringCapacity:  capBytes,
		target:        float64(capBytes) / 2,
		smoothFill:    float64(capBytes) / 2,
	}
}

// frameBytes returns the size in bytes of one frame of stereo 16-bit audio at
// the given sample rate and frame rate (4 bytes per stereo sample frame).
func frameBytes(sampleRate, fps int) int {
	return int(math.Round(float64(sampleRate) * 4 / float64(fps)))
}

// RingCapacity is the audio ring depth in bytes the AudioPlayer should allocate.
func (p *framePacer) RingCapacity() int { return p.ringCapacity }

// FrameBytes is one frame of audio in bytes (for the AudioPlayer silent-frame pad).
func (p *framePacer) FrameBytes() int { return p.frameBytesN }

// wait sleeps until the next frame deadline. Call once per produced audio
// frame (turbo runs several emulated frames per call). bufferedBytes is the
// current audio ring fill, used as the slow rate-lock reference. Absolute
// deadlines (not Sleep(interval)) keep best-effort time.Sleep overshoot from
// accumulating into rate drift.
func (p *framePacer) wait(bufferedBytes int) {
	p.smoothFill, p.frameInterval = pacingInterval(p.baseInterval, p.smoothFill, float64(bufferedBytes), p.target)

	if p.nextDeadline.IsZero() {
		p.nextDeadline = time.Now()
	}
	p.nextDeadline = p.nextDeadline.Add(time.Duration(p.frameInterval))
	if d := time.Until(p.nextDeadline); d > 0 {
		time.Sleep(d)
	} else if d < -time.Duration(p.baseInterval) {
		// More than a frame behind (e.g. a long RunFrame); resync rather
		// than spiral trying to catch up.
		p.nextDeadline = time.Now()
	}
}

// pacingInterval low-passes the ring fill and returns the updated smoothed
// fill and the controller-adjusted frame interval. A fuller-than-target ring
// means the producer is outrunning the device, so the interval lengthens (and
// vice versa), clamped to +/-pacingMaxAdjust. Pure, so it is unit-testable
// without timing.
func pacingInterval(base, smoothFill, fill, target float64) (newSmooth, interval float64) {
	newSmooth = smoothFill + (fill-smoothFill)*pacingFillAlpha
	adjust := pacingGain * (newSmooth - target) / target
	if adjust > pacingMaxAdjust {
		adjust = pacingMaxAdjust
	} else if adjust < -pacingMaxAdjust {
		adjust = -pacingMaxAdjust
	}
	return newSmooth, base * (1 + adjust)
}
