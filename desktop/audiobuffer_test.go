package desktop

import (
	"io"
	"sync"
	"testing"
	"time"
)

func TestAudioRingBuffer_BasicWriteRead(t *testing.T) {
	rb := NewAudioRingBuffer(16)

	data := []byte{1, 2, 3, 4, 5}
	rb.Write(data)

	if rb.Buffered() != 5 {
		t.Fatalf("expected 5 buffered bytes, got %d", rb.Buffered())
	}

	out := make([]byte, 5)
	n, err := rb.Read(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Fatalf("expected 5 bytes read, got %d", n)
	}
	for i, b := range out {
		if b != data[i] {
			t.Fatalf("byte %d: expected %d, got %d", i, data[i], b)
		}
	}
}

func TestAudioRingBuffer_WriteLargerThanCapacityKeepsTail(t *testing.T) {
	rb := NewAudioRingBuffer(4)

	// Write more data than capacity: only the last capacity bytes are
	// kept; the oldest portion is discarded before the blocking path
	// would ever see it.
	rb.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8})

	if rb.Buffered() != 4 {
		t.Fatalf("expected 4 buffered bytes, got %d", rb.Buffered())
	}

	out := make([]byte, 4)
	n, _ := rb.Read(out)
	if n != 4 {
		t.Fatalf("expected 4 bytes, got %d", n)
	}
	expected := []byte{5, 6, 7, 8}
	for i, b := range out {
		if b != expected[i] {
			t.Fatalf("byte %d: expected %d, got %d", i, expected[i], b)
		}
	}
}

func TestAudioRingBuffer_Clear(t *testing.T) {
	rb := NewAudioRingBuffer(16)
	rb.Write([]byte{1, 2, 3, 4})
	rb.Clear()
	if rb.Buffered() != 0 {
		t.Fatalf("expected 0 buffered after clear, got %d", rb.Buffered())
	}
}

func TestAudioRingBuffer_ClearUnblocksWriter(t *testing.T) {
	rb := NewAudioRingBuffer(4)
	rb.Write([]byte{1, 2, 3, 4})

	writeDone := make(chan struct{})
	go func() {
		rb.Write([]byte{5, 6})
		close(writeDone)
	}()

	time.Sleep(50 * time.Millisecond)
	rb.Clear()

	select {
	case <-writeDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Clear did not unblock a blocked Write")
	}
}

func TestAudioRingBuffer_Close(t *testing.T) {
	rb := NewAudioRingBuffer(16)
	rb.Write([]byte{1, 2})
	rb.Close()

	// Should still read remaining data
	out := make([]byte, 2)
	n, err := rb.Read(out)
	if err != nil {
		t.Fatalf("expected no error reading remaining data, got %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 bytes, got %d", n)
	}

	// Now should get EOF
	_, err = rb.Read(out)
	if err != io.EOF {
		t.Fatalf("expected io.EOF after close and drain, got %v", err)
	}
}

func TestAudioRingBuffer_ReadEmptyReturnsSilence(t *testing.T) {
	rb := NewAudioRingBuffer(16)

	// Underrun on an open buffer: Read does not block, it returns a full
	// buffer of zeroed silence with no error.
	buf := []byte{9, 9, 9, 9}
	n, err := rb.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error on underrun: %v", err)
	}
	if n != len(buf) {
		t.Fatalf("expected %d silence bytes, got %d", len(buf), n)
	}
	for i, b := range buf {
		if b != 0 {
			t.Fatalf("byte %d: expected silence 0, got %d", i, b)
		}
	}

	// Once closed and empty, Read returns io.EOF rather than silence.
	rb.Close()
	if _, err := rb.Read(buf); err != io.EOF {
		t.Fatalf("expected io.EOF on closed empty buffer, got %v", err)
	}
}

func TestAudioRingBuffer_ConcurrentReadWrite(t *testing.T) {
	rb := NewAudioRingBuffer(1024)
	const iterations = 100
	const chunk = 100
	totalBytes := iterations * chunk

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer goroutine. Writes only non-zero bytes so the reader can
	// distinguish real samples from the zeroed silence the ring returns on
	// underrun. Write still blocks on a full buffer, so no data is lost.
	go func() {
		defer wg.Done()
		data := make([]byte, chunk)
		for i := 0; i < iterations; i++ {
			for j := range data {
				data[j] = byte(i%255 + 1)
			}
			rb.Write(data)
		}
		rb.Close()
	}()

	// Reader goroutine. Read never blocks; on underrun it returns zeroed
	// silence, so count only the non-zero real bytes.
	received := 0
	go func() {
		defer wg.Done()
		buf := make([]byte, 64)
		for {
			n, err := rb.Read(buf)
			for _, b := range buf[:n] {
				if b != 0 {
					received++
				}
			}
			if err == io.EOF {
				return
			}
		}
	}()

	wg.Wait()

	if received != totalBytes {
		t.Fatalf("expected %d real bytes, got %d", totalBytes, received)
	}
}

func TestAudioRingBuffer_WrapAround(t *testing.T) {
	rb := NewAudioRingBuffer(8)

	// Write 6 bytes
	rb.Write([]byte{1, 2, 3, 4, 5, 6})

	// Read 4 (readPos advances to 4)
	out := make([]byte, 4)
	rb.Read(out)

	// Now readPos=4, writePos=6, count=2
	// Write 5 more (wraps around writePos)
	rb.Write([]byte{7, 8, 9, 10, 11})

	if rb.Buffered() != 7 {
		t.Fatalf("expected 7 buffered, got %d", rb.Buffered())
	}

	out = make([]byte, 7)
	n, _ := rb.Read(out)
	expected := []byte{5, 6, 7, 8, 9, 10, 11}
	if n != 7 {
		t.Fatalf("expected 7 bytes, got %d", n)
	}
	for i, b := range out {
		if b != expected[i] {
			t.Fatalf("byte %d: expected %d, got %d", i, expected[i], b)
		}
	}
}

func TestAudioRingBuffer_PartialRead(t *testing.T) {
	rb := NewAudioRingBuffer(16)
	rb.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8})

	// Read less than available
	out := make([]byte, 3)
	n, err := rb.Read(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 bytes, got %d", n)
	}
	if rb.Buffered() != 5 {
		t.Fatalf("expected 5 remaining, got %d", rb.Buffered())
	}
}

func TestAudioRingBuffer_WriteAfterClose(t *testing.T) {
	rb := NewAudioRingBuffer(16)
	rb.Close()

	// Write after close should be silently ignored
	rb.Write([]byte{1, 2, 3})

	if rb.Buffered() != 0 {
		t.Fatalf("expected 0 buffered after write to closed buffer, got %d", rb.Buffered())
	}
}

func TestAudioRingBuffer_WriteBlocksWhenFull(t *testing.T) {
	rb := NewAudioRingBuffer(4)
	rb.Write([]byte{1, 2, 3, 4})

	writeDone := make(chan struct{})
	go func() {
		rb.Write([]byte{5, 6})
		close(writeDone)
	}()

	select {
	case <-writeDone:
		t.Fatal("Write should have blocked on full buffer")
	case <-time.After(50 * time.Millisecond):
	}

	buf := make([]byte, 2)
	if n, err := rb.Read(buf); n != 2 || err != nil {
		t.Fatalf("Read returned n=%d err=%v", n, err)
	}

	select {
	case <-writeDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Write did not unblock after Read drained space")
	}

	if got := rb.Buffered(); got != 4 {
		t.Fatalf("expected 4 bytes buffered, got %d", got)
	}
}

func TestAudioRingBuffer_CloseUnblocksWriter(t *testing.T) {
	rb := NewAudioRingBuffer(4)
	rb.Write([]byte{1, 2, 3, 4})

	writeDone := make(chan struct{})
	go func() {
		rb.Write([]byte{5, 6})
		close(writeDone)
	}()

	time.Sleep(50 * time.Millisecond)
	rb.Close()

	select {
	case <-writeDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Close did not unblock a blocked Write")
	}
}

func TestAudioRingBuffer_WriteChunksAcrossBlock(t *testing.T) {
	rb := NewAudioRingBuffer(4)
	rb.Write([]byte{1, 2, 3, 4})

	writeDone := make(chan struct{})
	go func() {
		rb.Write([]byte{5, 6, 7, 8})
		close(writeDone)
	}()

	time.Sleep(50 * time.Millisecond)
	select {
	case <-writeDone:
		t.Fatal("Write should be blocked on full buffer")
	default:
	}

	drain := make([]byte, 2)
	if _, err := rb.Read(drain); err != nil {
		t.Fatalf("Read error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	select {
	case <-writeDone:
		t.Fatal("Write should still be blocked after draining only half the needed space")
	default:
	}

	if _, err := rb.Read(drain); err != nil {
		t.Fatalf("Read error: %v", err)
	}

	select {
	case <-writeDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Write did not complete after reader drained enough space")
	}

	out := make([]byte, 4)
	if _, err := rb.Read(out); err != nil {
		t.Fatalf("Read error: %v", err)
	}
	for i, v := range []byte{5, 6, 7, 8} {
		if out[i] != v {
			t.Fatalf("mismatch at %d: got %d want %d", i, out[i], v)
		}
	}
}
