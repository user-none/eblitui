package desktop

import (
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

const audioSampleRate = 48000

// ringBufferCapacity is 6400 bytes, exactly two frames at 48kHz stereo
// 16-bit at 60Hz. Small enough that the writer parks every frame or two,
// keeping FPS tightly coupled to the audio device's drain rate; large
// enough to absorb a single slow RunFrame without audio underrun. oto's
// player buffer plus its context buffer provide additional downstream
// headroom.
const ringBufferCapacity = 6400

// AudioPlayer manages audio playback via oto. It writes int16 stereo
// samples to a ring buffer which oto's player reads from in a pull
// model; the ring buffer's blocking Write paces the emulation loop.
//
// When the host has no usable audio device (headless, permission
// denied, device busy) oto initialization fails and the player falls
// back to a timer-driven drain goroutine. That keeps the ring-backed
// pacing model intact so emulation still advances at real-time rate;
// it just plays no sound and uses a software tick instead of the audio
// device's hardware clock.
type AudioPlayer struct {
	player     *oto.Player
	ringBuffer *AudioRingBuffer
	audioBytes []byte // Pre-allocated buffer for int16-to-byte conversion

	// Silent-fallback drain goroutine state. Both channels are nil when
	// a real oto.Player is driving the ring.
	silentStop chan struct{}
	silentDone chan struct{}
}

// oto context singleton — shared between game audio and notification audio
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

// NewAudioPlayer creates and initializes audio playback. The volume
// parameter sets the initial volume before playback starts, preventing
// audio pops when muted.
//
// If the audio device is unavailable, a silent fallback is returned:
// emulation continues under the same blocking-ring timing model,
// driven by a timer-based drain rather than the audio hardware clock.
// The returned player is always usable.
func NewAudioPlayer(volume float64) *AudioPlayer {
	rb := NewAudioRingBuffer(ringBufferCapacity)

	ctx, err := ensureOtoContext()
	if err != nil {
		return newSilentAudioPlayer(rb)
	}

	player := ctx.NewPlayer(rb)
	// Reduce mux player buffer from default 96000 bytes (0.5s) to ~19200 bytes
	// (~50ms). Prevents large internal buffer accumulation at startup.
	player.SetBufferSize(19200)
	// Set volume before Play() to avoid pop when muted.
	player.SetVolume(volume)
	player.Play()

	return &AudioPlayer{
		player:     player,
		ringBuffer: rb,
		audioBytes: make([]byte, 0, 4096),
	}
}

// newSilentAudioPlayer returns an AudioPlayer whose ring is drained at
// the audio device's nominal byte rate by a goroutine with a time.Ticker,
// so that QueueSamples still blocks and paces the emulator. Used when
// no real audio device is available.
func newSilentAudioPlayer(rb *AudioRingBuffer) *AudioPlayer {
	ap := &AudioPlayer{
		ringBuffer: rb,
		audioBytes: make([]byte, 0, 4096),
		silentStop: make(chan struct{}),
		silentDone: make(chan struct{}),
	}
	go ap.silentDrain()
	return ap
}

// silentDrain pulls bytes out of the ring at the same average rate a
// real audio device would consume. tickInterval is coarse enough that
// timer jitter is averaged out and fine enough that ring latency stays
// within a couple of frames.
func (a *AudioPlayer) silentDrain() {
	defer close(a.silentDone)

	const tickInterval = 10 * time.Millisecond
	const bytesPerSec = audioSampleRate * 4 // stereo int16
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

// QueueSamples converts int16 stereo samples to bytes and writes them
// to the ring buffer for oto to consume. Blocks when the ring is full,
// pacing the caller to the audio device's drain rate.
func (a *AudioPlayer) QueueSamples(samples []int16) {
	if len(samples) == 0 {
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

// ClearQueue flushes all buffered audio from the ring buffer.
// Used when entering rewind mode to prevent stale audio playback.
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
	// Close the ring buffer first. Its broadcast wakes any Read that is
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
