//go:build !ios && !libretro

package standalone

import (
	"io"
	"sync"
	"testing"
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

func TestAudioRingBuffer_Overflow(t *testing.T) {
	rb := NewAudioRingBuffer(8)

	// Write 6 bytes
	rb.Write([]byte{1, 2, 3, 4, 5, 6})

	// Write 5 more (overflows by 3, drops oldest 3)
	rb.Write([]byte{7, 8, 9, 10, 11})

	if rb.Buffered() != 8 {
		t.Fatalf("expected 8 buffered bytes, got %d", rb.Buffered())
	}

	out := make([]byte, 8)
	n, _ := rb.Read(out)
	if n != 8 {
		t.Fatalf("expected 8 bytes, got %d", n)
	}
	// Should have: 4, 5, 6, 7, 8, 9, 10, 11
	expected := []byte{4, 5, 6, 7, 8, 9, 10, 11}
	for i, b := range out {
		if b != expected[i] {
			t.Fatalf("byte %d: expected %d, got %d", i, expected[i], b)
		}
	}
}

func TestAudioRingBuffer_OverflowLargerThanCapacity(t *testing.T) {
	rb := NewAudioRingBuffer(4)

	// Write more data than capacity
	rb.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8})

	if rb.Buffered() != 4 {
		t.Fatalf("expected 4 buffered bytes, got %d", rb.Buffered())
	}

	out := make([]byte, 4)
	n, _ := rb.Read(out)
	if n != 4 {
		t.Fatalf("expected 4 bytes, got %d", n)
	}
	// Should have the last 4 bytes
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
	totalBytes := 10000

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer goroutine
	go func() {
		defer wg.Done()
		data := make([]byte, 100)
		for i := 0; i < 100; i++ {
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

	// With overflow, we may receive fewer bytes than written
	if received == 0 {
		t.Fatal("received 0 bytes")
	}
	if received > totalBytes {
		t.Fatalf("received more bytes (%d) than written (%d)", received, totalBytes)
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
