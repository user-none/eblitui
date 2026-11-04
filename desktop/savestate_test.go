package desktop

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/user-none/eblitui/desktop/storage"
)

// fakeSaveCore serializes to state and holds sram as its battery save.
type fakeSaveCore struct {
	state []byte
	sram  []byte
}

func (f fakeSaveCore) Serialize() ([]byte, error) { return f.state, nil }
func (f fakeSaveCore) Deserialize([]byte) error   { return nil }
func (f fakeSaveCore) HasSRAM() bool              { return true }
func (f fakeSaveCore) GetSRAM() []byte            { return f.sram }
func (f fakeSaveCore) SetSRAM([]byte)             {}

// TestSaveFilesWrittenWhole tests that state slots, the resume state and
// the battery save are written with their full contents and leave no temp
// file, replacing an existing file and a temp file left by an earlier
// failed write.
func TestSaveFilesWrittenWhole(t *testing.T) {
	useTempDataDir(t)
	m := NewSaveStateManager(nil)
	m.SetGame("T-1")
	dir, err := storage.GetGameSaveDir("T-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"state-0.state", "resume.state", "cart.srm"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("old contents that are longer"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".tmp"), []byte("stale"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	core := fakeSaveCore{state: []byte("state"), sram: []byte("sram")}
	if err := m.Save(core); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := m.SaveResume(core); err != nil {
		t.Fatalf("SaveResume: %v", err)
	}
	if err := m.SaveSRAM(core); err != nil {
		t.Fatalf("SaveSRAM: %v", err)
	}

	want := map[string][]byte{"state-0.state": core.state, "resume.state": core.state, "cart.srm": core.sram}
	for name, data := range want {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, data) {
			t.Errorf("%s = %q, want %q", name, got, data)
		}
		if _, err := os.Stat(filepath.Join(dir, name+".tmp")); !os.IsNotExist(err) {
			t.Errorf("%s.tmp left behind", name)
		}
	}
}

func TestNewSaveStateManager(t *testing.T) {
	m := NewSaveStateManager(nil)
	if m.GetCurrentSlot() != 0 {
		t.Errorf("initial slot should be 0, got %d", m.GetCurrentSlot())
	}
	if m.gameID != "" {
		t.Errorf("initial gameID should be empty, got %q", m.gameID)
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
	// persistSlot with empty gameID does nothing and writes no file
	m.persistSlot()
}
