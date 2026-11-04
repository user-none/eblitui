package desktop

import (
	"strings"
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

// TestSearchOverlayAppendCap tests that the search text stops at
// maxSearchRunes characters, multi-byte ones counting as one, and that the
// callback runs only when the text changed.
func TestSearchOverlayAppendCap(t *testing.T) {
	calls := 0
	s := NewSearchOverlay(func(string) { calls++ })
	wide := []rune(strings.Repeat(string(rune(0x65E5)), maxSearchRunes-1))

	s.appendChars(wide)
	s.appendChars([]rune("ab"))
	if n := utf8.RuneCountInString(s.text); n != maxSearchRunes {
		t.Fatalf("text has %d characters, want %d", n, maxSearchRunes)
	}
	if !strings.HasSuffix(s.text, "a") {
		t.Fatalf("text ends %q, want the first character past the old end", s.text[len(s.text)-3:])
	}
	s.appendChars([]rune("c"))
	if calls != 2 {
		t.Fatalf("callback ran %d times, want 2", calls)
	}
}

// TestSearchOverlayAppendSkipsActivatingSlash tests that a '/' typed into
// empty text is not added, but one after other text is.
func TestSearchOverlayAppendSkipsActivatingSlash(t *testing.T) {
	calls := 0
	s := NewSearchOverlay(func(string) { calls++ })
	s.appendChars([]rune("/"))
	if s.text != "" || calls != 0 {
		t.Fatalf("text = %q, %d calls after the activating slash", s.text, calls)
	}
	s.appendChars([]rune("/a/b"))
	if s.text != "a/b" {
		t.Fatalf("text = %q, want a/b", s.text)
	}
}

// TestVisibleSearchText tests that the overlay text is shown whole when it
// fits, and otherwise keeps the prefix, "..." and as much of the end of the
// text as fits, with the cursor when active.
func TestVisibleSearchText(t *testing.T) {
	width := func(s string) float64 { return float64(utf8.RuneCountInString(s)) }
	prefix := len(searchPrefix)

	if got := visibleSearchText("sonic", true, 100, width); got != searchPrefix+"sonic"+searchCursor {
		t.Errorf("fitting text = %q", got)
	}
	if got := visibleSearchText("sonic", false, float64(prefix+5), width); got != searchPrefix+"sonic" {
		t.Errorf("exactly fitting text = %q", got)
	}

	maxWidth := float64(prefix + len(searchTrimmed) + 3 + len(searchCursor))
	if got := visibleSearchText("abcdefgh", true, maxWidth, width); got != searchPrefix+searchTrimmed+"fgh"+searchCursor {
		t.Errorf("trimmed text = %q", got)
	}
	if got := visibleSearchText("abcdefgh", false, 1, width); got != searchPrefix+searchTrimmed {
		t.Errorf("text with no room = %q", got)
	}
}
