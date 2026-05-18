package desktop

import (
	"math"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

// audioSampleRate is the process-wide oto context rate. oto permits only
// one context rate per process, so all audio - emulator output and the
// synthesized UI/achievement sounds - must share it. It defaults to
// 48000 and is overridden once at startup from the core's
// SystemInfo.SampleRate (see Run); the demand-pacing math derives
// bytesPerFrame from it, so it must match what the core actually
// produces or the producer/consumer pacing deadlocks.
var audioSampleRate = 48000

// ringBufferCapacity is a smoothing buffer between the producer's
// per-frame writes and oto's bursty reads (roughly two frames of stereo
// 16-bit audio at 60Hz). Pacing is driven by the demand signal, not by
// ring fullness. Large enough to absorb a single slow RunFrame without
// audio underrun while keeping added latency low. oto's player buffer
// plus its context buffer provide additional downstream headroom.
const ringBufferCapacity = 6400

// otoPlayerBufferBytes sizes the mux player buffer (~50ms at 48kHz
// stereo int16). Used both to configure oto and to derive the upper
// bound on demand backlog (maxPending) so a single oto burst cannot
// overrun the producer's catch-up window.
const otoPlayerBufferBytes = 19200

// kickstartFrames is the number of frames the producer is allowed to
// run before it must wait for a consumer-drain demand signal. One frame
// is not enough: a core that under-produces by even a few samples on
// its first frame leaves drainedBytes below bytesPerFrame, so demand
// never fires and the loop deadlocks. Two frames give the cold-start
// drain enough headroom to reliably release the first real demand.
const kickstartFrames = 2

// AudioPlayer manages audio playback via oto. The producer writes int16
// stereo samples into a ring buffer which oto pulls from. Pacing is
// driven by consumer drain: each Read on the ring accumulates a byte
// counter, and once per frame's worth of bytes the producer is signalled
// via WaitForDemand. Emulation loops call WaitForDemand at the top of
// each iteration to park until the audio device is ready for the next
// frame. The ring's block-on-full path remains as a safety net against
// pathological over-production but is not the pacing primitive.
//
// When the host has no usable audio device (headless, permission denied,
// device busy) oto initialization fails and the player falls back to a
// timer-driven drain goroutine that pulls bytes at the same nominal rate
// a real device would, keeping the demand-signal pacing intact.
type AudioPlayer struct {
	player     *oto.Player
	ringBuffer *AudioRingBuffer
	audioBytes []byte // Pre-allocated buffer for int16-to-byte conversion
	// silentFrame is one frame's worth of zero bytes, written to the ring
	// when the core produces empty audio. Without this an empty cold-start
	// frame would queue nothing, oto would have nothing to drain, and the
	// demand signal would never fire - deadlocking the producer.
	silentFrame []byte

	// Silent-fallback drain goroutine state. Both channels are nil when
	// a real oto.Player is driving the ring.
	silentStop chan struct{}
	silentDone chan struct{}

	// Demand-signal pacing. demandCond is signalled when pendingFrames
	// transitions above zero, which happens inside the ring's onDrain
	// callback once drainedBytes crosses bytesPerFrame.
	demandMu      sync.Mutex
	demandCond    *sync.Cond
	bytesPerFrame int
	drainedBytes  int
	pendingFrames int
	maxPending    int
	shutdown      bool
}

// oto context singleton - shared between game audio and notification audio
var (
	otoCtx      *oto.Context
	otoInitOnce sync.Once
	otoInitErr  error
)

// ensureOtoContext initializes the oto audio context on first use.
func ensureOtoContext() (*oto.Context, error) {
	otoInitOnce.Do(func() {
		op := &oto.NewContextOptions{
			SampleRate:   audioSampleRate,
			ChannelCount: 2,
			Format:       oto.FormatSignedInt16LE,
			BufferSize:   50 * time.Millisecond, // Reduce OS AudioQueue from default ~100ms
		}
		var readyChan chan struct{}
		otoCtx, readyChan, otoInitErr = oto.NewContext(op)
		if otoInitErr != nil {
			return
		}
		<-readyChan
	})
	return otoCtx, otoInitErr
}

// NewAudioPlayer creates and initializes audio playback with the
// demand-signal pacing model sized for the given core frame rate.
// volume sets the initial volume before playback starts, preventing
// audio pops when muted.
//
// If the audio device is unavailable, a silent fallback is used:
// emulation continues under the same demand-signal pacing, driven by a
// timer-based drain rather than the audio hardware clock. The returned
// player is always usable.
func NewAudioPlayer(volume float64, fps int) *AudioPlayer {
	rb := NewAudioRingBuffer(ringBufferCapacity)

	bytesPerFrame := int(math.Round(float64(audioSampleRate) * 4 / float64(fps)))
	maxPending := otoPlayerBufferBytes/bytesPerFrame + 1

	ap := &AudioPlayer{
		ringBuffer:    rb,
		audioBytes:    make([]byte, 0, 4096),
		silentFrame:   make([]byte, bytesPerFrame),
		bytesPerFrame: bytesPerFrame,
		maxPending:    maxPending,
		pendingFrames: kickstartFrames,
	}
	ap.demandCond = sync.NewCond(&ap.demandMu)

	ctx, err := ensureOtoContext()
	if err != nil {
		ap.silentStop = make(chan struct{})
		ap.silentDone = make(chan struct{})
		go ap.silentDrain()
	} else {
		player := ctx.NewPlayer(rb)
		player.SetBufferSize(otoPlayerBufferBytes)
		// Set volume before Play() to avoid pop when muted.
		player.SetVolume(volume)
		player.Play()
		ap.player = player
	}

	// Install the drain callback last. The AudioPlayer must be fully
	// constructed before any consumer Read can fire handleDrain.
	rb.SetOnDrain(ap.handleDrain)

	return ap
}

// silentDrain pulls bytes out of the ring at the same average rate a
// real audio device would consume, so handleDrain fires often enough to
// release demand at real-time cadence. tickInterval is coarse enough
// that timer jitter is averaged out and fine enough that demand frames
// are released within a frame or two of when they would have been on
// real hardware.
func (a *AudioPlayer) silentDrain() {
	defer close(a.silentDone)

	const tickInterval = 10 * time.Millisecond
	bytesPerSec := audioSampleRate * 4 // stereo int16
	bytesPerTick := bytesPerSec * int(tickInterval) / int(time.Second)
	buf := make([]byte, bytesPerTick)

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.silentStop:
			return
		case <-ticker.C:
			avail := a.ringBuffer.Buffered()
			if avail == 0 {
				continue
			}
			if avail > len(buf) {
				avail = len(buf)
			}
			if _, err := a.ringBuffer.Read(buf[:avail]); err != nil {
				return
			}
		}
	}
}

// handleDrain is invoked by the ring buffer after each Read with the
// number of bytes consumed. It accumulates a frame-sized counter and
// releases producer demand once per frame's worth of drained bytes,
// capped at maxPending so a bursty consumer (oto's first 50ms pull)
// cannot enqueue unbounded catch-up work.
func (a *AudioPlayer) handleDrain(n int) {
	a.demandMu.Lock()
	a.drainedBytes += n
	for a.drainedBytes >= a.bytesPerFrame {
		a.drainedBytes -= a.bytesPerFrame
		if a.pendingFrames < a.maxPending {
			a.pendingFrames++
		}
	}
	if a.pendingFrames > 0 {
		a.demandCond.Signal()
	}
	a.demandMu.Unlock()
}

// WaitForDemand blocks until the audio consumer has drained enough
// bytes to request another frame, or until Close is called. Returns
// true when the caller should run the next frame, false when the player
// is shutting down and the producer should exit.
func (a *AudioPlayer) WaitForDemand() bool {
	a.demandMu.Lock()
	for !a.shutdown && a.pendingFrames == 0 {
		a.demandCond.Wait()
	}
	if a.shutdown {
		a.demandMu.Unlock()
		return false
	}
	a.pendingFrames--
	a.demandMu.Unlock()
	return true
}

// QueueSamples converts int16 stereo samples to bytes and writes them
// to the ring buffer for oto to consume. Blocks when the ring is full
// as a safety net against pathological over-production; real-time
// pacing comes from WaitForDemand, not from this Write.
//
// Empty input is replaced with one frame of silence so the consumer
// always has bytes to drain. Without this, a cold-start frame that
// produces no audio (common while a core's audio chips initialize)
// would leave the ring empty, demand would never fire, and the
// producer would deadlock. Short non-empty frames are passed through
// unchanged - only zero-length input is padded.
func (a *AudioPlayer) QueueSamples(samples []int16) {
	if len(samples) == 0 {
		a.ringBuffer.Write(a.silentFrame)
		return
	}

	// Convert int16 samples to little-endian bytes using pre-allocated buffer
	needed := len(samples) * 2
	if cap(a.audioBytes) < needed {
		a.audioBytes = make([]byte, 0, needed)
	}
	a.audioBytes = a.audioBytes[:0]
	for _, sample := range samples {
		a.audioBytes = append(a.audioBytes, byte(sample), byte(sample>>8))
	}

	a.ringBuffer.Write(a.audioBytes)
}

// ClearQueue flushes all buffered audio from the ring buffer. Used
// across state transitions (rewind enter, save state load) to prevent
// stale audio from playing once the new state takes over. Callers must
// pause the emulation goroutine first so no producer write races the
// clear.
func (a *AudioPlayer) ClearQueue() {
	a.ringBuffer.Clear()
}

// SetVolume sets the playback volume (0.0 = silent, 1.0 = normal, 2.0 = max).
// Values are clamped to [0.0, 2.0]. No-op on the silent fallback.
func (a *AudioPlayer) SetVolume(vol float64) {
	if a.player == nil {
		return
	}
	if vol < 0 {
		vol = 0
	} else if vol > 2.0 {
		vol = 2.0
	}
	a.player.SetVolume(vol)
}

// Close cleans up audio resources.
func (a *AudioPlayer) Close() {
	// Set shutdown and wake any producer parked on demand BEFORE closing
	// the ring. Otherwise a producer signalled by a stale demand could
	// race past WaitForDemand and into ring.Write at the same moment the
	// ring is being torn down, and there is no demand wake to follow.
	a.demandMu.Lock()
	a.shutdown = true
	a.demandCond.Broadcast()
	a.demandMu.Unlock()

	// Close the ring buffer next. Its broadcast wakes any Read that is
	// parked on an empty buffer (oto's reader or the silent-drain
	// goroutine's Read after a concurrent Clear), letting them observe
	// EOF and return. Without this, stopping the silent-drain goroutine
	// could deadlock if its Read happens to be parked.
	a.ringBuffer.Close()

	if a.silentStop != nil {
		close(a.silentStop)
		<-a.silentDone
		a.silentStop = nil
	}
	if a.player != nil {
		a.player.Close()
	}
}
