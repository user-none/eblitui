package desktop

import (
	"testing"
	"time"
)

func TestFrameBytes(t *testing.T) {
	// Stereo int16 = 4 bytes per sample frame.
	cases := []struct {
		rate, fps, want int
	}{
		{48000, 60, 3200},
		{48000, 50, 3840},
		{44100, 60, 2940},
	}
	for _, c := range cases {
		if got := frameBytes(c.rate, c.fps); got != c.want {
			t.Fatalf("frameBytes(%d,%d) = %d, want %d", c.rate, c.fps, got, c.want)
		}
	}
}

func TestNewFramePacerSizing(t *testing.T) {
	p := newFramePacer(44100, 60)

	if p.FrameBytes() != 2940 {
		t.Fatalf("FrameBytes = %d, want 2940", p.FrameBytes())
	}
	if want := ringBufferFrames * 2940; p.RingCapacity() != want {
		t.Fatalf("RingCapacity = %d, want %d", p.RingCapacity(), want)
	}
	if want := float64(time.Second) / 60; p.baseInterval != want {
		t.Fatalf("baseInterval = %v, want %v", p.baseInterval, want)
	}
}

func TestPacingIntervalDirection(t *testing.T) {
	base := float64(time.Second) / 60
	target := 1000.0

	// Ring fuller than target: producer outrunning device, interval lengthens.
	if _, hi := pacingInterval(base, target, 2*target, target); hi <= base {
		t.Fatalf("over-target fill should lengthen interval: got %v, base %v", hi, base)
	}
	// Ring emptier than target: interval shortens.
	if _, lo := pacingInterval(base, target, 0, target); lo >= base {
		t.Fatalf("under-target fill should shorten interval: got %v, base %v", lo, base)
	}
	// At target: no adjustment.
	if s, mid := pacingInterval(base, target, target, target); mid != base || s != target {
		t.Fatalf("at-target should not adjust: interval %v (base %v), smooth %v (want %v)", mid, base, s, target)
	}
}

func TestPacingIntervalClamped(t *testing.T) {
	base := float64(time.Second) / 60
	target := 1000.0

	// Extreme over-target (smoothFill already at the extreme so the low-pass
	// doesn't mask it) must clamp to +pacingMaxAdjust.
	if _, hi := pacingInterval(base, 1e9, 1e9, target); hi > base*(1+pacingMaxAdjust)+1e-6 {
		t.Fatalf("interval %v exceeds clamp %v", hi, base*(1+pacingMaxAdjust))
	}
	// Extreme under-target must clamp to -pacingMaxAdjust.
	if _, lo := pacingInterval(base, -1e9, -1e9, target); lo < base*(1-pacingMaxAdjust)-1e-6 {
		t.Fatalf("interval %v below clamp %v", lo, base*(1-pacingMaxAdjust))
	}
}
