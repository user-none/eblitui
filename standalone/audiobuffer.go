//go:build !ios && !libretro

package standalone

import (
	"io"
	"sync"
)

// AudioRingBuffer is a thread-safe ring buffer implementing io.Reader.
// The emulation goroutine writes samples via Write(), and oto's player
// reads them via Read(). Read blocks when empty; Write drops oldest
// samples on overflow to prevent stalling the producer.
type AudioRingBuffer struct {
	buf      []byte
	readPos  int
	writePos int
	count    int
	capacity int
	mu       sync.Mutex
	cond     *sync.Cond
	closed   bool
}

// NewAudioRingBuffer creates a ring buffer with the given capacity in bytes.
func NewAudioRingBuffer(capacity int) *AudioRingBuffer {
	rb := &AudioRingBuffer{
		buf:      make([]byte, capacity),
		capacity: capacity,
	}
	rb.cond = sync.NewCond(&rb.mu)
	return rb
}

// Write adds data to the buffer. Non-blocking; if the buffer overflows,
// oldest samples are dropped to make room for new data.
func (rb *AudioRingBuffer) Write(p []byte) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.closed {
		return
	}

	n := len(p)
	if n == 0 {
		return
	}

	// If data is larger than capacity, only write the last capacity bytes
	if n > rb.capacity {
		p = p[n-rb.capacity:]
		n = rb.capacity
	}

	// If we need more space, drop oldest data
	overflow := rb.count + n - rb.capacity
	if overflow > 0 {
		rb.readPos = (rb.readPos + overflow) % rb.capacity
		rb.count -= overflow
	}

	// Write data to buffer (may wrap around)
	firstChunk := rb.capacity - rb.writePos
	if firstChunk >= n {
		copy(rb.buf[rb.writePos:], p)
	} else {
		copy(rb.buf[rb.writePos:], p[:firstChunk])
		copy(rb.buf[0:], p[firstChunk:])
	}
	rb.writePos = (rb.writePos + n) % rb.capacity
	rb.count += n

	// Signal readers that data is available
	rb.cond.Signal()
}

// Read implements io.Reader. Blocks until data is available or the buffer
// is closed. Returns io.EOF when closed and empty.
func (rb *AudioRingBuffer) Read(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// Wait for data
	for rb.count == 0 {
		if rb.closed {
			return 0, io.EOF
		}
		rb.cond.Wait()
	}

	// Read up to len(p) bytes
	n := len(p)
	if n > rb.count {
		n = rb.count
	}

	// Copy data from buffer (may wrap around)
	firstChunk := rb.capacity - rb.readPos
	if firstChunk >= n {
		copy(p, rb.buf[rb.readPos:rb.readPos+n])
	} else {
		copy(p, rb.buf[rb.readPos:])
		copy(p[firstChunk:], rb.buf[:n-firstChunk])
	}
	rb.readPos = (rb.readPos + n) % rb.capacity
	rb.count -= n

	return n, nil
}

// Buffered returns the number of bytes currently in the buffer.
func (rb *AudioRingBuffer) Buffered() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}

// Clear resets the buffer, discarding all data.
func (rb *AudioRingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.readPos = 0
	rb.writePos = 0
	rb.count = 0
}

// Close signals shutdown. Subsequent Reads return io.EOF when the buffer
// is empty. Unblocks any goroutines waiting in Read.
func (rb *AudioRingBuffer) Close() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.closed = true
	rb.cond.Broadcast()
}
