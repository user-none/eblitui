//go:build !libretro

package standalone

import "testing"

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
