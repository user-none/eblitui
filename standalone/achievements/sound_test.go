//go:build !libretro && !ios

package achievements

import (
	"encoding/binary"
	"testing"
)

func TestGenerateUnlockSound(t *testing.T) {
	data := generateUnlockSound()

	if data == nil {
		t.Fatal("should not return nil")
	}
	if len(data) == 0 {
		t.Fatal("should not return empty slice")
	}
}

func TestGenerateUnlockSoundLength(t *testing.T) {
	data := generateUnlockSound()

	// 48000 Hz * 0.8 seconds * 4 bytes per sample (stereo S16LE)
	expected := int(48000 * 0.8 * 4)
	if len(data) != expected {
		t.Errorf("expected %d bytes, got %d", expected, len(data))
	}
}

func TestGenerateUnlockSoundNotSilent(t *testing.T) {
	data := generateUnlockSound()

	allZero := true
	for i := 0; i < len(data)-1; i += 2 {
		sample := int16(binary.LittleEndian.Uint16(data[i : i+2]))
		if sample != 0 {
			allZero = false
			break
		}
	}

	if allZero {
		t.Error("sound data should not be all zeros")
	}
}

func TestGenerateUnlockSoundClipping(t *testing.T) {
	data := generateUnlockSound()

	// Check all S16LE samples are within valid range
	// The function clamps to [-1.0, 1.0] then multiplies by 12000,
	// so values should be well within int16 range
	for i := 0; i < len(data)-1; i += 2 {
		sample := int16(binary.LittleEndian.Uint16(data[i : i+2]))
		if sample > 12000 || sample < -12000 {
			t.Errorf("sample at byte %d has value %d, expected within [-12000, 12000]", i, sample)
			break
		}
	}
}

func TestGenerateUnlockSoundStereo(t *testing.T) {
	data := generateUnlockSound()

	// Stereo S16LE: left and right channels should be identical
	// (the code writes the same value to both channels)
	for i := 0; i < len(data)-3; i += 4 {
		left := int16(binary.LittleEndian.Uint16(data[i : i+2]))
		right := int16(binary.LittleEndian.Uint16(data[i+2 : i+4]))
		if left != right {
			t.Errorf("at sample %d: left=%d, right=%d (should be identical)", i/4, left, right)
			break
		}
	}
}
