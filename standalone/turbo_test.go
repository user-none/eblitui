//go:build !libretro

package standalone

import (
	"testing"
)

func TestAverageAudio2x(t *testing.T) {
	// Two stereo frames of 4 samples each (2 stereo pairs per frame)
	// Frame 1: [100, 200, 300, 400]
	// Frame 2: [200, 400, 100, 200]
	// Expected: [(100+200)/2, (200+400)/2, (300+100)/2, (400+200)/2]
	//         = [150, 300, 200, 300]
	input := []int16{100, 200, 300, 400, 200, 400, 100, 200}
	got := averageAudio(input, 2)

	expected := []int16{150, 300, 200, 300}
	if len(got) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(got))
	}
	for i, v := range got {
		if v != expected[i] {
			t.Errorf("index %d: expected %d, got %d", i, expected[i], v)
		}
	}
}

func TestAverageAudio3x(t *testing.T) {
	// Three stereo frames of 2 samples each (1 stereo pair per frame)
	// Frame 1: [100, -100]
	// Frame 2: [200, -200]
	// Frame 3: [300, -300]
	// Expected: [(100+200+300)/3, (-100+-200+-300)/3]
	//         = [200, -200]
	input := []int16{100, -100, 200, -200, 300, -300}
	got := averageAudio(input, 3)

	expected := []int16{200, -200}
	if len(got) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(got))
	}
	for i, v := range got {
		if v != expected[i] {
			t.Errorf("index %d: expected %d, got %d", i, expected[i], v)
		}
	}
}

func TestAverageAudioMultiplier1(t *testing.T) {
	input := []int16{100, 200, 300, 400}
	got := averageAudio(input, 1)

	if len(got) != len(input) {
		t.Fatalf("expected length %d, got %d", len(input), len(got))
	}
	for i, v := range got {
		if v != input[i] {
			t.Errorf("index %d: expected %d, got %d", i, input[i], v)
		}
	}
}

func TestAverageAudioEmpty(t *testing.T) {
	got := averageAudio(nil, 2)
	if len(got) != 0 {
		t.Fatalf("expected empty output, got length %d", len(got))
	}

	got = averageAudio([]int16{}, 4)
	if len(got) != 0 {
		t.Fatalf("expected empty output, got length %d", len(got))
	}
}
