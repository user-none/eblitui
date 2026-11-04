package desktop

import (
	"testing"
	"unicode/utf8"
)

func TestNewSearchOverlay(t *testing.T) {
	s := NewSearchOverlay(nil)

	if s.IsVisible() {
		t.Error("should not be visible initially")
	}
	if s.IsActive() {
		t.Error("should not be active initially")
	}
}

func TestSearchOverlayActivate(t *testing.T) {
	s := NewSearchOverlay(nil)

	s.Activate()
	if !s.IsActive() {
		t.Error("should be active after Activate()")
	}
}

func TestSearchOverlayIsVisibleEmptyText(t *testing.T) {
	s := NewSearchOverlay(nil)
	s.text = ""

	if s.IsVisible() {
		t.Error("should not be visible with empty text")
	}
}

func TestSearchOverlayIsVisibleWithText(t *testing.T) {
	s := NewSearchOverlay(nil)
	s.text = "sonic"

	if !s.IsVisible() {
		t.Error("should be visible with non-empty text")
	}
}

func TestSearchOverlayClearResetsState(t *testing.T) {
	s := NewSearchOverlay(nil)
	s.text = "sonic"
	s.active = true

	s.Clear()

	if s.text != "" {
		t.Errorf("text should be empty after Clear, got %q", s.text)
	}
	if s.active {
		t.Error("should not be active after Clear")
	}
	if s.IsVisible() {
		t.Error("should not be visible after Clear")
	}
}

func TestSearchOverlayClearTriggersCallback(t *testing.T) {
	called := false
	var receivedText string
	s := NewSearchOverlay(func(text string) {
		called = true
		receivedText = text
	})
	s.text = "sonic"

	s.Clear()

	if !called {
		t.Error("onChanged callback should have been called")
	}
	if receivedText != "" {
		t.Errorf("callback should receive empty string, got %q", receivedText)
	}
}

func TestSearchOverlayClearNilCallback(t *testing.T) {
	s := NewSearchOverlay(nil)
	s.text = "test"

	// Should not panic
	s.Clear()
}

func TestSearchOverlayHandleInputWhenInactive(t *testing.T) {
	s := NewSearchOverlay(nil)
	s.active = false

	handled := s.HandleInput()
	if handled {
		t.Error("should return false when not active")
	}
}

// TestSearchOverlayBackspace tests that backspace removes one whole
// character, multi-byte ones included, leaving valid UTF-8, and reports
// the change.
func TestSearchOverlayBackspace(t *testing.T) {
	var got []string
	s := NewSearchOverlay(func(text string) { got = append(got, text) })
	s.text = "a" + string(rune(0xE9)) + string(rune(0x65E5)) + string(rune(0x1F600))

	want := []string{s.text[:6], s.text[:3], "a", ""}
	for i, w := range want {
		if !s.backspace() {
			t.Fatalf("backspace %d reported nothing removed", i)
		}
		if s.text != w || !utf8.ValidString(s.text) {
			t.Fatalf("after backspace %d text = %q, want %q", i, s.text, w)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("callback ran %d times, want %d", len(got), len(want))
	}
}

// TestSearchOverlayBackspaceEmpty tests that backspace on empty text does
// nothing and does not report a change.
func TestSearchOverlayBackspaceEmpty(t *testing.T) {
	called := false
	s := NewSearchOverlay(func(string) { called = true })
	if s.backspace() || called {
		t.Fatal("backspace on empty text removed something or reported a change")
	}
}
