//go:build !libretro

package standalone

import (
	"testing"

	"github.com/user-none/eblitui/standalone/storage"
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

func TestSetGameRestoresSlot(t *testing.T) {
	lib := storage.DefaultLibrary()
	lib.AddGame(&storage.GameEntry{
		CRC32:    "aabbccdd",
		Settings: storage.GameSettings{SaveSlot: 5},
	})

	m := NewSaveStateManager(nil)
	m.SetLibrary(lib)
	m.SetGame("aabbccdd")

	if m.GetCurrentSlot() != 5 {
		t.Errorf("expected slot 5 from game settings, got %d", m.GetCurrentSlot())
	}
}

func TestSetGameNotInLibrary(t *testing.T) {
	lib := storage.DefaultLibrary()

	m := NewSaveStateManager(nil)
	m.SetLibrary(lib)

	// Set to slot 5 first
	for i := 0; i < 5; i++ {
		m.NextSlot()
	}

	// SetGame with unknown CRC should reset to 0
	m.SetGame("nonexistent")
	if m.GetCurrentSlot() != 0 {
		t.Errorf("expected slot 0 for unknown game, got %d", m.GetCurrentSlot())
	}
}

func TestSetGameNilLibrary(t *testing.T) {
	m := NewSaveStateManager(nil)

	// Set to slot 3 first
	for i := 0; i < 3; i++ {
		m.NextSlot()
	}

	// SetGame without library should reset to 0
	m.SetGame("anything")
	if m.GetCurrentSlot() != 0 {
		t.Errorf("expected slot 0 with nil library, got %d", m.GetCurrentSlot())
	}
}

func TestSetLibrary(t *testing.T) {
	m := NewSaveStateManager(nil)

	if m.library != nil {
		t.Error("library should be nil initially")
	}

	lib := storage.DefaultLibrary()
	m.SetLibrary(lib)

	if m.library == nil {
		t.Error("library should not be nil after SetLibrary")
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

func TestPersistSlotNilLibrary(t *testing.T) {
	m := NewSaveStateManager(nil)
	// persistSlot with nil library should not panic
	m.persistSlot()
}

func TestPersistSlotNoGame(t *testing.T) {
	m := NewSaveStateManager(nil)
	m.SetLibrary(storage.DefaultLibrary())
	// persistSlot with empty gameCRC should not panic
	m.persistSlot()
}
