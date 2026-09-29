package desktop

import (
	"testing"
	"time"
)

func TestSharedInput_SetAndRead(t *testing.T) {
	si := &SharedInput{}

	// Set player 0 buttons
	si.Set(0, 0b1010_0101)

	buttons := si.Read()
	if buttons[0] != 0b1010_0101 {
		t.Fatalf("player 0 mismatch: expected 0x%X, got 0x%X", uint32(0b1010_0101), buttons[0])
	}
	if buttons[1] != 0 {
		t.Fatalf("player 1 should be 0, got 0x%X", buttons[1])
	}

	// Set player 1 buttons
	si.Set(1, 0xFF)
	buttons = si.Read()
	if buttons[0] != 0b1010_0101 {
		t.Fatalf("player 0 changed unexpectedly: 0x%X", buttons[0])
	}
	if buttons[1] != 0xFF {
		t.Fatalf("player 1 mismatch: expected 0xFF, got 0x%X", buttons[1])
	}

	// Out-of-range player should be ignored
	si.Set(-1, 0xDEAD)
	si.Set(maxPlayers, 0xDEAD)
	buttons = si.Read()
	if buttons[0] != 0b1010_0101 || buttons[1] != 0xFF {
		t.Fatal("out-of-range Set should not change state")
	}
}

func TestSharedFramebuffer_UpdateAndRead(t *testing.T) {
	sf := NewSharedFramebuffer(256, 224)

	// Create some test pixel data
	stride := 256 * 4
	height := 192
	pixels := make([]byte, stride*height)
	for i := range pixels {
		pixels[i] = byte(i % 256)
	}

	sf.Update(pixels, stride, height, 1.25)

	readPixels, readStride, readHeight, readPAR := sf.Read()

	if readStride != stride {
		t.Fatalf("stride mismatch: expected %d, got %d", stride, readStride)
	}
	if readHeight != height {
		t.Fatalf("height mismatch: expected %d, got %d", height, readHeight)
	}
	if readPAR != 1.25 {
		t.Fatalf("par mismatch: expected 1.25, got %v", readPAR)
	}

	// Verify pixel data (readPixels is a copy, safe to use)
	for i := 0; i < stride*height; i++ {
		if readPixels[i] != pixels[i] {
			t.Fatalf("pixel mismatch at %d: expected %d, got %d", i, pixels[i], readPixels[i])
		}
	}
}

// testFrame is a frame of the given width and height whose bytes
// identify their position, offset by seed.
func testFrame(width, height, seed int) (pixels []byte, stride int) {
	stride = width * 4
	pixels = make([]byte, stride*height)
	for i := range pixels {
		pixels[i] = byte(i*7 + seed)
	}
	return pixels, stride
}

// checkFrame fails unless Read returns the frame whole.
func checkFrame(t *testing.T, sf *SharedFramebuffer, want []byte, stride, height int) {
	t.Helper()
	got, gotStride, gotHeight, _ := sf.Read()
	if gotStride != stride || gotHeight != height {
		t.Fatalf("stride %d height %d, want %d and %d", gotStride, gotHeight, stride, height)
	}
	if len(got) < stride*height {
		t.Fatalf("read %d bytes, want at least %d", len(got), stride*height)
	}
	for i := range stride * height {
		if got[i] != want[i] {
			t.Fatalf("byte %d = %d, want %d", i, got[i], want[i])
		}
	}
}

// TestSharedFramebuffer_GrowsForLargerFrame holds a frame larger than
// the starting size, as a core rendering above its native resolution
// sends, to being read whole, and a smaller frame after it to being
// read from the front of the grown buffers.
func TestSharedFramebuffer_GrowsForLargerFrame(t *testing.T) {
	sf := NewSharedFramebuffer(704, 512)
	large, largeStride := testFrame(1408, 448, 3)
	sf.Update(large, largeStride, 448, 1.0)
	checkFrame(t, sf, large, largeStride, 448)
	small, smallStride := testFrame(320, 224, 9)
	sf.Update(small, smallStride, 224, 1.0)
	checkFrame(t, sf, small, smallStride, 224)
}

// TestSharedFramebuffer_LargerFrameBetweenFrames holds a larger frame
// arriving between two reads, as a state loaded in a larger video
// mode delivers it, to being read whole, and the snapshot the earlier
// read returned to being left as it was.
func TestSharedFramebuffer_LargerFrameBetweenFrames(t *testing.T) {
	sf := NewSharedFramebuffer(704, 512)
	small, smallStride := testFrame(352, 240, 1)
	sf.Update(small, smallStride, 240, 1.0)
	held, _, _, _ := sf.Read()
	snapshot := append([]byte(nil), held[:smallStride*240]...)
	large, largeStride := testFrame(1408, 1024, 5)
	sf.Update(large, largeStride, 1024, 1.0)
	checkFrame(t, sf, large, largeStride, 1024)
	for i := range snapshot {
		if held[i] != snapshot[i] {
			t.Fatalf("held snapshot byte %d changed to %d, was %d", i, held[i], snapshot[i])
		}
	}
}

func TestEmuControl_PauseResume(t *testing.T) {
	ec := NewEmuControl()

	// Start an emulation goroutine
	paused := make(chan struct{})
	resumed := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			if !ec.CheckPause() {
				return
			}
			// Signal that we completed a CheckPause cycle
			select {
			case paused <- struct{}{}:
			default:
			}
			time.Sleep(time.Millisecond)
		}
	}()

	// Wait a bit for goroutine to start
	time.Sleep(20 * time.Millisecond)

	// Request pause (should block until ack)
	ec.RequestPause()

	if !ec.IsPaused() {
		t.Fatal("expected paused after RequestPause")
	}

	// Resume
	go func() {
		ec.RequestResume()
		close(resumed)
	}()
	<-resumed

	// Wait a bit for goroutine to resume
	time.Sleep(20 * time.Millisecond)

	if ec.IsPaused() {
		t.Fatal("expected not paused after RequestResume")
	}

	// Stop
	ec.Stop()
	<-done
}

func TestEmuControl_Stop(t *testing.T) {
	ec := NewEmuControl()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for ec.ShouldRun() {
			if !ec.CheckPause() {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	ec.Stop()

	select {
	case <-done:
		// Goroutine exited
	case <-time.After(time.Second):
		t.Fatal("goroutine did not exit after Stop")
	}
}

func TestEmuControl_StopWhilePaused(t *testing.T) {
	ec := NewEmuControl()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if !ec.CheckPause() {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	// Pause first
	ec.RequestPause()

	// Stop while paused — should unblock the goroutine
	ec.Stop()

	select {
	case <-done:
		// Goroutine exited
	case <-time.After(time.Second):
		t.Fatal("goroutine did not exit after Stop while paused")
	}
}

func TestEmuControl_DoubleRequestPause(t *testing.T) {
	ec := NewEmuControl()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if !ec.CheckPause() {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	// First pause
	ec.RequestPause()

	// Second pause should be a no-op (already paused)
	ec.RequestPause()

	if !ec.IsPaused() {
		t.Fatal("expected still paused")
	}

	ec.Stop()
	<-done
}
