package desktop

import (
	"math"
	"sync"
	"testing"
	"time"
)

// newTestAudioPlayer constructs an AudioPlayer with demand-signal state
// initialized but no oto.Player and no silent-drain goroutine. Tests
// drive the demand machinery directly via handleDrain or via the ring
// buffer's drain callback, without depending on a real audio device or
// timer goroutine. Mirrors the FPS/bytesPerFrame/maxPending math in
// NewAudioPlayer.
func newTestAudioPlayer(fps int) *AudioPlayer {
	rb := NewAudioRingBuffer(ringBufferCapacity)
	bytesPerFrame := int(math.Round(float64(audioSampleRate) * 4 / float64(fps)))
	if bytesPerFrame <= 0 {
		bytesPerFrame = audioSampleRate * 4 / 60
	}
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
	rb.SetOnDrain(ap.handleDrain)
	return ap
}

// consumeKickstart drains the kickstart frames from a fresh player so
// tests can exercise the post-kickstart wait/drain path from a clean
// state. Asserts each call returns true; failure indicates a regression
// in the kickstart machinery.
func consumeKickstart(t *testing.T, ap *AudioPlayer) {
	t.Helper()
	for i := 0; i < kickstartFrames; i++ {
		if !ap.WaitForDemand() {
			t.Fatalf("WaitForDemand returned false on kickstart frame %d", i)
		}
	}
}

func TestAudioPlayer_NTSCFrameSize(t *testing.T) {
	ap := newTestAudioPlayer(60)
	defer ap.Close()

	if ap.bytesPerFrame != 3200 {
		t.Fatalf("expected bytesPerFrame=3200 for 60fps, got %d", ap.bytesPerFrame)
	}
}

func TestAudioPlayer_PALFrameSize(t *testing.T) {
	ap := newTestAudioPlayer(50)
	defer ap.Close()

	if ap.bytesPerFrame != 3840 {
		t.Fatalf("expected bytesPerFrame=3840 for 50fps, got %d", ap.bytesPerFrame)
	}
}

func TestAudioPlayer_KickstartFrames(t *testing.T) {
	ap := newTestAudioPlayer(60)
	defer ap.Close()

	// Each kickstart frame should release WaitForDemand immediately.
	for i := 0; i < kickstartFrames; i++ {
		if !ap.WaitForDemand() {
			t.Fatalf("WaitForDemand returned false on kickstart frame %d", i)
		}
	}

	// One more call without any drain must block.
	done := make(chan bool, 1)
	go func() {
		done <- ap.WaitForDemand()
	}()

	select {
	case <-done:
		t.Fatal("WaitForDemand returned without a drain signal after kickstart exhausted")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestAudioPlayer_DrainAccumulatesIntoDemand(t *testing.T) {
	ap := newTestAudioPlayer(60)
	defer ap.Close()

	consumeKickstart(t, ap)

	bpf := ap.bytesPerFrame

	ap.handleDrain(bpf - 1)
	ap.demandMu.Lock()
	if ap.pendingFrames != 0 {
		ap.demandMu.Unlock()
		t.Fatalf("expected pendingFrames=0 with bpf-1 drained, got %d", ap.pendingFrames)
	}
	ap.demandMu.Unlock()

	ap.handleDrain(1)
	ap.demandMu.Lock()
	if ap.pendingFrames != 1 {
		ap.demandMu.Unlock()
		t.Fatalf("expected pendingFrames=1 after crossing bpf, got %d", ap.pendingFrames)
	}
	ap.demandMu.Unlock()

	ap.handleDrain(bpf)
	ap.demandMu.Lock()
	if ap.pendingFrames != 2 {
		ap.demandMu.Unlock()
		t.Fatalf("expected pendingFrames=2 after second full frame drained, got %d", ap.pendingFrames)
	}
	ap.demandMu.Unlock()
}

func TestAudioPlayer_DemandClampedAtMaxPending(t *testing.T) {
	ap := newTestAudioPlayer(60)
	defer ap.Close()

	consumeKickstart(t, ap)

	ap.handleDrain((ap.maxPending + 5) * ap.bytesPerFrame)

	ap.demandMu.Lock()
	got := ap.pendingFrames
	ap.demandMu.Unlock()

	if got != ap.maxPending {
		t.Fatalf("expected pendingFrames clamped to %d, got %d", ap.maxPending, got)
	}
}

func TestAudioPlayer_WaitForDemandWakesOnDrain(t *testing.T) {
	ap := newTestAudioPlayer(60)
	defer ap.Close()

	consumeKickstart(t, ap)

	done := make(chan bool, 1)
	go func() {
		done <- ap.WaitForDemand()
	}()

	// Give the goroutine a moment to park on demandCond.
	time.Sleep(20 * time.Millisecond)

	ap.handleDrain(ap.bytesPerFrame)

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("WaitForDemand returned false instead of waking on drain")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("WaitForDemand did not wake within timeout after drain")
	}
}

func TestAudioPlayer_CloseWakesWaiter(t *testing.T) {
	ap := newTestAudioPlayer(60)

	consumeKickstart(t, ap)

	done := make(chan bool, 1)
	go func() {
		done <- ap.WaitForDemand()
	}()

	time.Sleep(20 * time.Millisecond)

	ap.Close()

	select {
	case ok := <-done:
		if ok {
			t.Fatal("WaitForDemand returned true after Close; expected false")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("WaitForDemand did not wake within timeout after Close")
	}
}

func TestAudioPlayer_EmptySamplesPadToFrame(t *testing.T) {
	ap := newTestAudioPlayer(60)
	defer ap.Close()

	consumeKickstart(t, ap)

	ap.QueueSamples(nil)

	if got := ap.ringBuffer.Buffered(); got != ap.bytesPerFrame {
		t.Fatalf("empty QueueSamples should write %d silent bytes, got %d buffered",
			ap.bytesPerFrame, got)
	}

	out := make([]byte, ap.bytesPerFrame)
	if _, err := ap.ringBuffer.Read(out); err != nil {
		t.Fatalf("Read of silent frame failed: %v", err)
	}
	for i, b := range out {
		if b != 0 {
			t.Fatalf("padded silent frame contained non-zero byte at offset %d: %d", i, b)
		}
	}

	// The drain should have released exactly one demand frame (60Hz:
	// bytesPerFrame == drained), unblocking a parked producer.
	ap.demandMu.Lock()
	if ap.pendingFrames != 1 {
		ap.demandMu.Unlock()
		t.Fatalf("draining a silent frame should release 1 demand, got pendingFrames=%d",
			ap.pendingFrames)
	}
	ap.demandMu.Unlock()
}

func TestAudioPlayer_EmptySliceSameAsNil(t *testing.T) {
	ap := newTestAudioPlayer(60)
	defer ap.Close()

	consumeKickstart(t, ap)

	ap.QueueSamples([]int16{})

	if got := ap.ringBuffer.Buffered(); got != ap.bytesPerFrame {
		t.Fatalf("empty slice QueueSamples should write %d silent bytes, got %d buffered",
			ap.bytesPerFrame, got)
	}
}
