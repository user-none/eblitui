// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

// audioSampleRate is the process-wide oto context rate. oto permits only
// one context rate per process, so all audio - emulator output and the
// synthesized UI/achievement sounds - must share it. It defaults to
// 48000 and is overridden once at startup from the core's
// SystemInfo.SampleRate (see Run). It must match what the core actually
// produces, and the framePacer derives its frame/ring sizing from it.
var audioSampleRate = 48000

// otoPlayerBufferBytes sizes the oto mux player buffer (~50ms at 48kHz
// stereo int16) via player.SetBufferSize.
const otoPlayerBufferBytes = 19200

// AudioPlayer is audio output only: it converts int16 stereo samples to
// bytes, writes them into a ring buffer that oto pulls from, and runs a
// silent fallback when no device is available. It does NOT pace the
// emulation loop - the framePacer does that, reading Buffered() as its
// rate-lock reference. The ring's block-on-full path remains as a
// backpressure safety net against pathological over-production.
//
// When the host has no usable audio device (headless, permission denied,
// device busy) oto initialization fails and a timer-driven drain goroutine
// pulls bytes at the nominal rate a real device would, so the ring still
// drains and the producer's writes do not block indefinitely.
type AudioPlayer struct {
	player     *oto.Player
	ringBuffer *AudioRingBuffer
	audioBytes []byte // Pre-allocated buffer for int16-to-byte conversion
	// silentFrame is one frame's worth of zero bytes, written to the ring
	// when the core produces empty audio so oto always has samples to drain
	// and does not underrun on empty cold-start frames.
	silentFrame []byte

	// Silent-fallback drain goroutine state. Both channels are nil when
	// a real oto.Player is driving the ring.
	silentStop chan struct{}
	silentDone chan struct{}
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

// NewAudioPlayer creates audio output with a ring of ringCapacity bytes and a
// silent-pad frame of silentFrameBytes (both supplied by the framePacer, which
// owns the rate-derived sizing). volume sets the initial volume before playback
// starts, preventing audio pops when muted.
//
// If the audio device is unavailable, a silent fallback drains the ring on a
// timer instead of the hardware clock. The returned player is always usable.
func NewAudioPlayer(volume float64, ringCapacity, silentFrameBytes int) *AudioPlayer {
	rb := NewAudioRingBuffer(ringCapacity)

	ap := &AudioPlayer{
		ringBuffer:  rb,
		audioBytes:  make([]byte, 0, 4096),
		silentFrame: make([]byte, silentFrameBytes),
	}

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

	return ap
}

// Buffered returns the current ring fill in bytes. The framePacer reads this
// as its rate-lock reference.
func (a *AudioPlayer) Buffered() int {
	return a.ringBuffer.Buffered()
}

// silentDrain pulls bytes out of the ring at the same average rate a real
// audio device would consume, so the ring keeps draining (and the producer's
// writes do not block) when no device is present. tickInterval is coarse
// enough that timer jitter averages out and fine enough that the drain tracks
// real time within a frame or two.
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

// QueueSamples converts int16 stereo samples to bytes and writes them to the
// ring buffer for oto to consume. Blocks when the ring is full as a
// backpressure safety net; real-time pacing comes from the framePacer, not
// from this Write.
//
// Empty input is replaced with one frame of silence so oto always has bytes to
// drain and does not underrun on cold-start frames where the core produces no
// audio. Short non-empty frames are passed through unchanged - only
// zero-length input is padded.
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
	// Close the ring buffer first. Its broadcast wakes any Read parked on an
	// empty buffer (oto's reader or the silent-drain goroutine) and any
	// producer blocked in ring.Write on a full buffer, letting them observe
	// the closed state and return. The timer-paced producer otherwise parks
	// only in time.Sleep (bounded by one frame interval), so no demand-side
	// wake is needed.
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
