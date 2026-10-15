package desktop

import (
	"testing"
)

func TestNewSaveStateManager(t *testing.T) {
	m := NewSaveStateManager(nil)
	if m.GetCurrentSlot() != 0 {
		t.Errorf("initial slot should be 0, got %d", m.GetCurrentSlot())
	}
	if m.gameCRC != "" {
		t.Errorf("initial gameCRC should be empty, got %q", m.gameCRC)
	}
}

func TestNextSlot(t *testing.T) {
	m := NewSaveStateManager(nil)

	for i := 1; i <= 10; i++ {
		m.NextSlot()
		expected := i % 10
		if m.GetCurrentSlot() != expected {
			t.Errorf("after %d NextSlot calls, expected slot %d, got %d", i, expected, m.GetCurrentSlot())
		}
	}
}

func TestPreviousSlot(t *testing.T) {
	m := NewSaveStateManager(nil)

	// First PreviousSlot from 0 should wrap to 9
	m.PreviousSlot()
	if m.GetCurrentSlot() != 9 {
		t.Errorf("expected slot 9, got %d", m.GetCurrentSlot())
	}

	// Continue backwards
	expected := []int{8, 7, 6, 5, 4, 3, 2, 1, 0}
	for i, exp := range expected {
		m.PreviousSlot()
		if m.GetCurrentSlot() != exp {
			t.Errorf("step %d: expected slot %d, got %d", i, exp, m.GetCurrentSlot())
		}
	}
}

func TestNextPreviousSlotRoundTrip(t *testing.T) {
	m := NewSaveStateManager(nil)

	// Go forward 7 slots
	for i := 0; i < 7; i++ {
		m.NextSlot()
	}
	if m.GetCurrentSlot() != 7 {
		t.Fatalf("expected slot 7, got %d", m.GetCurrentSlot())
	}

	// Go backward 7 slots
	for i := 0; i < 7; i++ {
		m.PreviousSlot()
	}
	if m.GetCurrentSlot() != 0 {
		t.Errorf("expected slot 0 after round trip, got %d", m.GetCurrentSlot())
	}
}

func TestSetGameNoGameResetsSlot(t *testing.T) {
	m := NewSaveStateManager(nil)

	// Set to slot 3 first
	for i := 0; i < 3; i++ {
		m.NextSlot()
	}

	// SetGame with no game resets to 0 without reading any file
	m.SetGame("")
	if m.GetCurrentSlot() != 0 {
		t.Errorf("expected slot 0 with no game, got %d", m.GetCurrentSlot())
	}
}

func TestNextSlotNilNotification(t *testing.T) {
	m := NewSaveStateManager(nil)
	// Should not panic
	m.NextSlot()
	m.NextSlot()
}

func TestPreviousSlotNilNotification(t *testing.T) {
	m := NewSaveStateManager(nil)
	// Should not panic
	m.PreviousSlot()
	m.PreviousSlot()
}

func TestHasResumeStateNoGame(t *testing.T) {
	m := NewSaveStateManager(nil)
	if m.HasResumeState() {
		t.Error("should not have resume state with no game set")
	}
}

func TestPersistSlotNoGame(t *testing.T) {
	m := NewSaveStateManager(nil)
	// persistSlot with empty gameCRC does nothing and writes no file
	m.persistSlot()
}
