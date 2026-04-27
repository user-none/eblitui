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

func TestAudioRingBuffer_CloseUnblocksReader(t *testing.T) {
	rb := NewAudioRingBuffer(16)

	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 4)
		_, err := rb.Read(buf)
		done <- err
	}()

	// Close should unblock the reader
	rb.Close()

	err := <-done
	if err != io.EOF {
		t.Fatalf("expected io.EOF from blocked reader, got %v", err)
	}
}

func TestAudioRingBuffer_ConcurrentReadWrite(t *testing.T) {
	rb := NewAudioRingBuffer(1024)
	const iterations = 100
	const chunk = 100
	totalBytes := iterations * chunk

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer goroutine
	go func() {
		defer wg.Done()
		data := make([]byte, chunk)
		for i := 0; i < iterations; i++ {
			for j := range data {
				data[j] = byte(i)
			}
			rb.Write(data)
		}
		rb.Close()
	}()

	// Reader goroutine
	received := 0
	go func() {
		defer wg.Done()
		buf := make([]byte, 64)
		for {
			n, err := rb.Read(buf)
			received += n
			if err == io.EOF {
				return
			}
		}
	}()

	wg.Wait()

	if received != totalBytes {
		t.Fatalf("expected %d bytes with blocking write, got %d", totalBytes, received)
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

func TestAudioRingBuffer_DrainCallbackInvoked(t *testing.T) {
	rb := NewAudioRingBuffer(16)

	var got int
	rb.SetOnDrain(func(n int) {
		got = n
	})

	rb.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8})

	out := make([]byte, 5)
	if _, err := rb.Read(out); err != nil {
		t.Fatalf("Read error: %v", err)
	}

	if got != 5 {
		t.Fatalf("expected drain callback to receive 5 bytes, got %d", got)
	}
}

func TestAudioRingBuffer_DrainCallbackNilSafe(t *testing.T) {
	rb := NewAudioRingBuffer(16)

	rb.Write([]byte{1, 2, 3, 4})

	out := make([]byte, 4)
	if _, err := rb.Read(out); err != nil {
		t.Fatalf("Read with nil callback returned error: %v", err)
	}
}

func TestAudioRingBuffer_DrainCallbackOutsideLock(t *testing.T) {
	rb := NewAudioRingBuffer(16)

	// A callback that calls back into the buffer would deadlock if the
	// callback fired while rb.mu was still held.
	var observed int
	rb.SetOnDrain(func(n int) {
		observed = rb.Buffered()
	})

	rb.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8})

	out := make([]byte, 5)
	done := make(chan struct{})
	go func() {
		rb.Read(out)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Read deadlocked when callback re-entered ring buffer")
	}

	if observed != 3 {
		t.Fatalf("expected callback to observe 3 bytes still buffered, got %d", observed)
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
