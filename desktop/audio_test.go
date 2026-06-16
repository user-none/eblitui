package desktop

import (
	"testing"
)

// newTestAudioPlayer constructs an AudioPlayer with a ring buffer and silent
// frame sized for fps, but no oto.Player and no silent-drain goroutine, so
// tests exercise QueueSamples without a real audio device.
func newTestAudioPlayer(fps int) *AudioPlayer {
	fb := frameBytes(audioSampleRate, fps)
	return &AudioPlayer{
		ringBuffer:  NewAudioRingBuffer(ringBufferFrames * fb),
		audioBytes:  make([]byte, 0, 4096),
		silentFrame: make([]byte, fb),
	}
}

func TestAudioPlayer_EmptySamplesPadToFrame(t *testing.T) {
	fb := frameBytes(audioSampleRate, 60)
	ap := newTestAudioPlayer(60)

	ap.QueueSamples(nil)

	if got := ap.Buffered(); got != fb {
		t.Fatalf("empty QueueSamples should write %d silent bytes, got %d buffered", fb, got)
	}

	out := make([]byte, fb)
	if _, err := ap.ringBuffer.Read(out); err != nil {
		t.Fatalf("Read of silent frame failed: %v", err)
	}
	for i, b := range out {
		if b != 0 {
			t.Fatalf("padded silent frame contained non-zero byte at offset %d: %d", i, b)
		}
	}
}

func TestAudioPlayer_EmptySliceSameAsNil(t *testing.T) {
	fb := frameBytes(audioSampleRate, 60)
	ap := newTestAudioPlayer(60)

	ap.QueueSamples([]int16{})

	if got := ap.Buffered(); got != fb {
		t.Fatalf("empty slice QueueSamples should write %d silent bytes, got %d buffered", fb, got)
	}
}
