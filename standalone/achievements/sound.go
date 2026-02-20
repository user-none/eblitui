//go:build !libretro && !ios

package achievements

import "math"

// generateUnlockSound creates a gentle achievement chime (48kHz stereo S16LE)
func generateUnlockSound() []byte {
	sampleRate := 48000
	duration := 0.8
	numSamples := int(float64(sampleRate) * duration)

	// Notes for a warm major chord arpeggio (C4, E4, G4)
	notes := []struct {
		freq   float64
		start  float64
		volume float64
	}{
		{261.63, 0.0, 0.4},
		{329.63, 0.08, 0.3},
		{392.00, 0.16, 0.3},
	}

	samples := make([]byte, numSamples*4) // 2 bytes * 2 channels

	for i := 0; i < numSamples; i++ {
		t := float64(i) / float64(sampleRate)
		sample := 0.0

		for _, note := range notes {
			if t < note.start {
				continue
			}

			noteT := t - note.start
			attackTime := 0.05
			decayTime := 0.6
			var envelope float64

			if noteT < attackTime {
				envelope = (1 - math.Cos(math.Pi*noteT/attackTime)) / 2
			} else {
				envelope = math.Exp(-2.5 * (noteT - attackTime) / decayTime)
			}

			fundamental := math.Sin(2 * math.Pi * note.freq * noteT)
			harmonic := math.Sin(2*math.Pi*note.freq*2*noteT) * 0.15
			sample += (fundamental + harmonic) * envelope * note.volume
		}

		if sample > 1.0 {
			sample = 1.0
		} else if sample < -1.0 {
			sample = -1.0
		}

		value := int16(sample * 12000)

		idx := i * 4
		samples[idx] = byte(value)
		samples[idx+1] = byte(value >> 8)
		samples[idx+2] = byte(value)
		samples[idx+3] = byte(value >> 8)
	}

	return samples
}
