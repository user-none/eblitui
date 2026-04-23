package desktop

import (
	"testing"
)

func TestPadToFrame_ShortGetsZeroPadded(t *testing.T) {
	var buf []int16
	got := padToFrame([]int16{1, 2, 3}, &buf, 6)
	want := []int16{1, 2, 3, 0, 0, 0}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, v := range want {
		if got[i] != v {
			t.Fatalf("index %d: got %d want %d", i, got[i], v)
		}
	}
}

func TestPadToFrame_ExactReturnsSamplesUnchanged(t *testing.T) {
	var buf []int16
	samples := []int16{1, 2, 3, 4}
	got := padToFrame(samples, &buf, 4)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	if &got[0] != &samples[0] {
		t.Fatal("expected same backing array (no copy)")
	}
}

func TestPadToFrame_LongerReturnsSamplesUnchanged(t *testing.T) {
	var buf []int16
	samples := []int16{1, 2, 3, 4, 5}
	got := padToFrame(samples, &buf, 3)
	if len(got) != 5 {
		t.Fatalf("len = %d, want 5", len(got))
	}
	if &got[0] != &samples[0] {
		t.Fatal("expected same backing array (no copy)")
	}
}

func TestPadToFrame_NilSamplesYieldsAllZeros(t *testing.T) {
	var buf []int16
	got := padToFrame(nil, &buf, 4)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	for i, v := range got {
		if v != 0 {
			t.Fatalf("index %d: got %d want 0", i, v)
		}
	}
}

func TestPadToFrame_NominalZeroDisables(t *testing.T) {
	var buf []int16
	got := padToFrame(nil, &buf, 0)
	if got != nil {
		t.Fatalf("expected nil passthrough, got %v", got)
	}
	got2 := padToFrame([]int16{1, 2}, &buf, 0)
	if len(got2) != 2 || got2[0] != 1 || got2[1] != 2 {
		t.Fatalf("expected passthrough, got %v", got2)
	}
}

func TestPadToFrame_ZeroesTailOnReuse(t *testing.T) {
	// Prime buf with non-zero data in the pad region.
	buf := make([]int16, 6)
	for i := range buf {
		buf[i] = 99
	}
	buf = buf[:0]

	got := padToFrame([]int16{1, 2}, &buf, 6)
	want := []int16{1, 2, 0, 0, 0, 0}
	for i, v := range want {
		if got[i] != v {
			t.Fatalf("index %d: got %d want %d (stale data leaked?)", i, got[i], v)
		}
	}
}

func TestPadToFrame_ReusesBackingArray(t *testing.T) {
	buf := make([]int16, 0, 8)
	cap1 := cap(buf)
	_ = padToFrame([]int16{1, 2}, &buf, 4)
	if cap(buf) != cap1 {
		t.Fatalf("buf cap changed from %d to %d (expected reuse)", cap1, cap(buf))
	}
	_ = padToFrame([]int16{3}, &buf, 4)
	if cap(buf) != cap1 {
		t.Fatalf("buf cap changed from %d to %d on second call", cap1, cap(buf))
	}
}

func TestPadToFrame_GrowsBufWhenNeeded(t *testing.T) {
	buf := make([]int16, 0, 2)
	got := padToFrame([]int16{1}, &buf, 5)
	if len(got) != 5 {
		t.Fatalf("len = %d, want 5", len(got))
	}
	if cap(buf) < 5 {
		t.Fatalf("cap = %d, want >= 5", cap(buf))
	}
	want := []int16{1, 0, 0, 0, 0}
	for i, v := range want {
		if got[i] != v {
			t.Fatalf("index %d: got %d want %d", i, got[i], v)
		}
	}
}
