package desktop

import (
	"io"
	"sync"
)

// AudioRingBuffer is a thread-safe ring buffer implementing io.Reader.
// The emulation goroutine writes samples via Write(), and oto's player
// reads them via Read(). Write blocks when the buffer is full until Read
// frees space; Read blocks when empty until Write adds data. The audio
// device's drain rate paces the producer through this back-pressure.
type AudioRingBuffer struct {
	buf      []byte
	readPos  int
	writePos int
	count    int
	capacity int
	mu       sync.Mutex
	cond     *sync.Cond
	closed   bool
	// onDrain is invoked from Read after the buffer lock is released, so
	// callbacks that take their own lock cannot deadlock against rb.mu.
	onDrain func(n int)
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

// Write copies data into the buffer, blocking on a full ring until Read
// frees space. If the input is larger than the buffer's capacity the
// leading portion is dropped and only the tail is written, because the
// audio device would only ever consume the most recent samples anyway.
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

	// If the input is larger than the buffer's capacity, keep only the
	// last capacity bytes; the leading portion would be overwritten
	// before it could ever be drained anyway.
	if n > rb.capacity {
		p = p[n-rb.capacity:]
		n = rb.capacity
	}

	written := 0
	for written < n {
		for !rb.closed && rb.count == rb.capacity {
			rb.cond.Wait()
		}
		if rb.closed {
			return
		}

		// Signal the reader only on the empty->non-empty transition to
		// avoid a pthread_cond_signal syscall every call.
		wasEmpty := rb.count == 0

		avail := rb.capacity - rb.count
		chunk := n - written
		if chunk > avail {
			chunk = avail
		}

		// Copy into the buffer, splitting the write if it wraps past
		// the end of the underlying slice.
		firstChunk := rb.capacity - rb.writePos
		if firstChunk >= chunk {
			copy(rb.buf[rb.writePos:], p[written:written+chunk])
		} else {
			copy(rb.buf[rb.writePos:], p[written:written+firstChunk])
			copy(rb.buf[0:], p[written+firstChunk:written+chunk])
		}
		rb.writePos = (rb.writePos + chunk) % rb.capacity
		rb.count += chunk
		written += chunk

		if wasEmpty {
			rb.cond.Signal()
		}
	}
}

// Read implements io.Reader. Blocks until data is available or the buffer
// is closed. Returns io.EOF when closed and empty.
func (rb *AudioRingBuffer) Read(p []byte) (int, error) {
	rb.mu.Lock()

	for rb.count == 0 {
		if rb.closed {
			rb.mu.Unlock()
			return 0, io.EOF
		}
		rb.cond.Wait()
	}

	// Signal a blocked writer only when the buffer transitions out of
	// the full state.
	wasFull := rb.count == rb.capacity

	n := len(p)
	if n > rb.count {
		n = rb.count
	}

	// Copy out of the buffer, splitting the read if it wraps past the
	// end of the underlying slice.
	firstChunk := rb.capacity - rb.readPos
	if firstChunk >= n {
		copy(p, rb.buf[rb.readPos:rb.readPos+n])
	} else {
		copy(p, rb.buf[rb.readPos:])
		copy(p[firstChunk:], rb.buf[:n-firstChunk])
	}
	rb.readPos = (rb.readPos + n) % rb.capacity
	rb.count -= n

	if wasFull {
		rb.cond.Signal()
	}

	cb := rb.onDrain
	rb.mu.Unlock()

	if cb != nil {
		cb(n)
	}

	return n, nil
}

// SetOnDrain installs a callback invoked after each Read completes,
// outside the buffer lock, with the number of bytes read. Used by the
// pacing layer to convert real-time consumer drain into producer wake
// signals. Pass nil to clear.
func (rb *AudioRingBuffer) SetOnDrain(fn func(n int)) {
	rb.mu.Lock()
	rb.onDrain = fn
	rb.mu.Unlock()
}

// Buffered returns the number of bytes currently in the buffer.
func (rb *AudioRingBuffer) Buffered() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}

// Clear resets the buffer, discarding all data. Any writer parked on a
// full ring is woken so it can complete its Write against the now-empty
// buffer.
func (rb *AudioRingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	wasFull := rb.count == rb.capacity
	rb.readPos = 0
	rb.writePos = 0
	rb.count = 0
	if wasFull {
		rb.cond.Signal()
	}
}

// Close signals shutdown. Subsequent Reads return io.EOF when the buffer
// is empty. Unblocks any goroutines waiting in Read or Write.
func (rb *AudioRingBuffer) Close() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.closed = true
	rb.cond.Broadcast()
}
