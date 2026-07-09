package desktop

import (
	"testing"

	"github.com/user-none/eblitui/coreif"
)

// rewindMockSaveStater records the data passed to the most recent Deserialize
// call so tests can assert which buffered state a rewind landed on.
type rewindMockSaveStater struct {
	lastDeserialized []byte
}

func (m *rewindMockSaveStater) Serialize() ([]byte, error) { return nil, nil }

func (m *rewindMockSaveStater) Deserialize(data []byte) error {
	m.lastDeserialized = data
	return nil
}

// rewindMockEmulator is a no-op emulator; Rewind only calls RunFrame on it.
type rewindMockEmulator struct{}

func (rewindMockEmulator) RunFrame()                             {}
func (rewindMockEmulator) GetFramebuffer() []byte                { return nil }
func (rewindMockEmulator) GetFramebufferStride() int             { return 0 }
func (rewindMockEmulator) GetActiveHeight() int                  { return 0 }
func (rewindMockEmulator) GetAudioSamples() []int16              { return nil }
func (rewindMockEmulator) SetInput(player int, buttons uint32)   {}
func (rewindMockEmulator) GetTiming() coreif.Timing              { return coreif.Timing{} }
func (rewindMockEmulator) SetOption(key string, value string)    {}
func (rewindMockEmulator) SetRom(data []byte)                    {}
func (rewindMockEmulator) SetDisc(disc coreif.DiscReader)        {}
func (rewindMockEmulator) SetBIOS(key string, data []byte) error { return nil }
func (rewindMockEmulator) Start()                                {}
func (rewindMockEmulator) Close()                                {}

func TestNewRewindBuffer(t *testing.T) {
	// 1MB buffer, 100 bytes per state = 10485 entries
	rb := NewRewindBuffer(1, 1, 100)
	if rb == nil {
		t.Fatal("expected non-nil buffer")
	}
	if rb.Capacity() != (1*1024*1024)/100 {
		t.Errorf("capacity = %d, want %d", rb.Capacity(), (1*1024*1024)/100)
	}
	if rb.Count() != 0 {
		t.Errorf("count = %d, want 0", rb.Count())
	}
}

func TestNewRewindBufferInvalidArgs(t *testing.T) {
	tests := []struct {
		name      string
		sizeMB    int
		frameStep int
		stateSize int
	}{
		{"zero state size", 1, 1, 0},
		{"negative state size", 1, 1, -1},
		{"zero buffer size", 0, 1, 100},
		{"zero frame step", 1, 0, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := NewRewindBuffer(tt.sizeMB, tt.frameStep, tt.stateSize)
			if rb != nil {
				t.Error("expected nil buffer for invalid args")
			}
		})
	}
}

func TestRewindBufferCapacityCalculation(t *testing.T) {
	// 40MB buffer, ~57KB state = ~716 entries
	rb := NewRewindBuffer(40, 1, 57000)
	if rb == nil {
		t.Fatal("expected non-nil buffer")
	}
	expected := (40 * 1024 * 1024) / 57000
	if rb.Capacity() != expected {
		t.Errorf("capacity = %d, want %d", rb.Capacity(), expected)
	}
}

func TestRewindBufferRewindEmptyReturnsFalse(t *testing.T) {
	rb := NewRewindBuffer(1, 1, 100)
	if rb == nil {
		t.Fatal("expected non-nil buffer")
	}
	// Rewind on empty buffer should return false
	result := rb.Rewind(nil, nil, 1)
	if result {
		t.Error("expected Rewind on empty buffer to return false")
	}
}

func TestRewindBufferReset(t *testing.T) {
	rb := NewRewindBuffer(1, 1, 100)
	if rb == nil {
		t.Fatal("expected non-nil buffer")
	}
	// Simulate some state by manually setting fields
	rb.head = 5
	rb.count = 5
	rb.frameTick = 3
	rb.buffer[0] = []byte{1, 2, 3}
	rb.buffer[1] = []byte{4, 5, 6}

	rb.Reset()

	if rb.head != 0 {
		t.Errorf("head = %d, want 0", rb.head)
	}
	if rb.count != 0 {
		t.Errorf("count = %d, want 0", rb.count)
	}
	if rb.frameTick != 0 {
		t.Errorf("frameTick = %d, want 0", rb.frameTick)
	}
	if rb.buffer[0] != nil {
		t.Error("buffer[0] should be nil after Reset")
	}
	if rb.buffer[1] != nil {
		t.Error("buffer[1] should be nil after Reset")
	}
}

func TestRewindBufferIsRewinding(t *testing.T) {
	rb := NewRewindBuffer(1, 1, 100)
	if rb == nil {
		t.Fatal("expected non-nil buffer")
	}
	if rb.IsRewinding() {
		t.Error("should not be rewinding initially")
	}
	rb.SetRewinding(true)
	if !rb.IsRewinding() {
		t.Error("should be rewinding after SetRewinding(true)")
	}
	rb.SetRewinding(false)
	if rb.IsRewinding() {
		t.Error("should not be rewinding after SetRewinding(false)")
	}
}

func TestRewindBufferFrameStepSkipping(t *testing.T) {
	rb := NewRewindBuffer(1, 3, 100)
	if rb == nil {
		t.Fatal("expected non-nil buffer")
	}

	// With frameStep=3, only every 3rd call should capture
	// frameTick starts at 0, increments to 1, 2, 3 (captures at 3)
	// We can't call Capture without a real emulator, but we can verify
	// the frameTick logic by checking after manual increments
	if rb.frameStep != 3 {
		t.Errorf("frameStep = %d, want 3", rb.frameStep)
	}
	if rb.frameTick != 0 {
		t.Errorf("initial frameTick = %d, want 0", rb.frameTick)
	}
}

func TestRewindBufferCountNeverExceedsCapacity(t *testing.T) {
	// Small buffer: 1KB / 100 bytes = 10 entries
	rb := NewRewindBuffer(1, 1, (1*1024*1024)/10)
	if rb == nil {
		t.Fatal("expected non-nil buffer")
	}
	if rb.Capacity() != 10 {
		t.Fatalf("capacity = %d, want 10", rb.Capacity())
	}

	// Manually simulate captures filling beyond capacity
	for i := 0; i < 20; i++ {
		rb.buffer[rb.head] = []byte{byte(i)}
		rb.head = (rb.head + 1) % rb.capacity
		if rb.count < rb.capacity {
			rb.count++
		}
	}

	if rb.count > rb.Capacity() {
		t.Errorf("count %d exceeds capacity %d", rb.count, rb.Capacity())
	}
	if rb.count != 10 {
		t.Errorf("count = %d, want 10", rb.count)
	}
}

func TestRewindItemsForHoldDuration(t *testing.T) {
	tests := []struct {
		duration int
		expected int
	}{
		{0, 0},   // Not pressed
		{-1, 0},  // Invalid
		{1, 1},   // Just pressed - single step
		{2, 0},   // Early hold, not on 4th frame
		{4, 1},   // 4th frame fires
		{5, 0},   // Not on 4th frame
		{8, 1},   // 8th frame fires
		{15, 0},  // 15 not divisible by 4
		{16, 1},  // Transition to faster rate, 16%2==0
		{17, 0},  // Odd frame
		{20, 1},  // Even frame
		{30, 1},  // Even frame, boundary
		{31, 1},  // Every frame zone
		{45, 1},  // Every frame
		{60, 1},  // Boundary of every frame zone
		{61, 2},  // Fast zone: 2 items/frame
		{100, 2}, // Still fast
		{999, 2}, // Very long hold
	}
	for _, tt := range tests {
		result := rewindItemsForHoldDuration(tt.duration)
		if result != tt.expected {
			t.Errorf("rewindItemsForHoldDuration(%d) = %d, want %d", tt.duration, result, tt.expected)
		}
	}
}

// TestRewindBufferStopsAtOldestState verifies that rewinding to the end of a
// wrapped buffer holds on the oldest captured state rather than jumping to the
// stale pre-rewind state left in the wrap-around slot.
func TestRewindBufferStopsAtOldestState(t *testing.T) {
	// Capacity of 5.
	rb := NewRewindBuffer(1, 1, (1*1024*1024)/5)
	if rb == nil {
		t.Fatal("expected non-nil buffer")
	}
	if rb.Capacity() != 5 {
		t.Fatalf("capacity = %d, want 5", rb.Capacity())
	}

	// Write 10 states so the buffer wraps once. Each state's single byte
	// encodes its value; the buffer ends holding [5 6 7 8 9] with head at 0.
	for i := 0; i < 10; i++ {
		rb.buffer[rb.head] = []byte{byte(i)}
		rb.head = (rb.head + 1) % rb.capacity
		if rb.count < rb.capacity {
			rb.count++
		}
	}

	emu := rewindMockEmulator{}
	ss := &rewindMockSaveStater{}

	// Stepping back one at a time yields 8, 7, 6, 5, then holds at 5.
	want := []byte{8, 7, 6, 5, 5, 5}
	for step, expected := range want {
		if !rb.Rewind(emu, ss, 1) {
			t.Fatalf("step %d: Rewind returned false", step)
		}
		if len(ss.lastDeserialized) != 1 || ss.lastDeserialized[0] != expected {
			t.Fatalf("step %d: deserialized %v, want state %d", step, ss.lastDeserialized, expected)
		}
	}
}

func TestRewindBufferRingBufferWraparound(t *testing.T) {
	// Capacity of 5
	rb := NewRewindBuffer(1, 1, (1*1024*1024)/5)
	if rb == nil {
		t.Fatal("expected non-nil buffer")
	}
	if rb.Capacity() != 5 {
		t.Fatalf("capacity = %d, want 5", rb.Capacity())
	}

	// Fill with 5 entries
	for i := 0; i < 5; i++ {
		rb.buffer[rb.head] = []byte{byte(i)}
		rb.head = (rb.head + 1) % rb.capacity
		if rb.count < rb.capacity {
			rb.count++
		}
	}
	if rb.head != 0 {
		t.Errorf("head should wrap to 0, got %d", rb.head)
	}
	if rb.count != 5 {
		t.Errorf("count = %d, want 5", rb.count)
	}

	// Write one more (overwrites oldest)
	rb.buffer[rb.head] = []byte{byte(99)}
	rb.head = (rb.head + 1) % rb.capacity
	// count stays at capacity
	if rb.count != 5 {
		t.Errorf("count should still be 5, got %d", rb.count)
	}
	if rb.head != 1 {
		t.Errorf("head should be 1, got %d", rb.head)
	}
	// Oldest entry (index 0) should now be overwritten
	if rb.buffer[0][0] != 99 {
		t.Errorf("buffer[0] = %d, want 99", rb.buffer[0][0])
	}
}
