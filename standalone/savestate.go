//go:build !libretro

package standalone

import (
	"fmt"
	"os"
	"path/filepath"

	emucore "github.com/user-none/eblitui/api"
	"github.com/user-none/eblitui/standalone/storage"
)

// SaveStateManager handles save state operations
type SaveStateManager struct {
	currentSlot  int
	gameCRC      string
	notification *Notification
	library      *storage.Library
}

// NewSaveStateManager creates a new save state manager
func NewSaveStateManager(notification *Notification) *SaveStateManager {
	return &SaveStateManager{
		currentSlot:  0,
		notification: notification,
	}
}

// SetLibrary sets the library reference for slot persistence
func (m *SaveStateManager) SetLibrary(library *storage.Library) {
	m.library = library
}

// SetGame sets the current game for save states
// Restores the last-used slot from the game's settings
func (m *SaveStateManager) SetGame(gameCRC string) {
	m.gameCRC = gameCRC

	// Restore last-used slot from library
	if m.library != nil {
		if game := m.library.GetGame(gameCRC); game != nil {
			m.currentSlot = game.Settings.SaveSlot
		} else {
			m.currentSlot = 0
		}
	} else {
		m.currentSlot = 0
	}
}

// GetCurrentSlot returns the current save slot
func (m *SaveStateManager) GetCurrentSlot() int {
	return m.currentSlot
}

// NextSlot cycles to the next save slot
func (m *SaveStateManager) NextSlot() {
	m.currentSlot = (m.currentSlot + 1) % 10
	m.persistSlot()
	if m.notification != nil {
		m.notification.ShowShort(fmt.Sprintf("Slot %d", m.currentSlot))
	}
}

// PreviousSlot cycles to the previous save slot
func (m *SaveStateManager) PreviousSlot() {
	m.currentSlot--
	if m.currentSlot < 0 {
		m.currentSlot = 9
	}
	m.persistSlot()
	if m.notification != nil {
		m.notification.ShowShort(fmt.Sprintf("Slot %d", m.currentSlot))
	}
}

// persistSlot saves the current slot to the library for the current game
func (m *SaveStateManager) persistSlot() {
	if m.library == nil || m.gameCRC == "" {
		return
	}
	if game := m.library.GetGame(m.gameCRC); game != nil {
		game.Settings.SaveSlot = m.currentSlot
		storage.SaveLibrary(m.library)
	}
}

// Save saves the current state to the current slot
func (m *SaveStateManager) Save(saveStater emucore.SaveStater) error {
	if m.gameCRC == "" {
		return fmt.Errorf("no game set")
	}

	saveDir, err := storage.GetGameSaveDir(m.gameCRC)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(saveDir, 0755); err != nil {
		return fmt.Errorf("failed to create save directory: %w", err)
	}

	statePath := filepath.Join(saveDir, fmt.Sprintf("state-%d.state", m.currentSlot))

	// Serialize emulator state
	state, err := saveStater.Serialize()
	if err != nil {
		return fmt.Errorf("failed to serialize state: %w", err)
	}

	if err := os.WriteFile(statePath, state, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	if m.notification != nil {
		m.notification.ShowShort(fmt.Sprintf("State saved to slot %d", m.currentSlot))
	}

	return nil
}

// Load loads the state from the current slot
func (m *SaveStateManager) Load(saveStater emucore.SaveStater) error {
	if m.gameCRC == "" {
		return fmt.Errorf("no game set")
	}

	saveDir, err := storage.GetGameSaveDir(m.gameCRC)
	if err != nil {
		return err
	}

	statePath := filepath.Join(saveDir, fmt.Sprintf("state-%d.state", m.currentSlot))

	// Check if state exists
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		if m.notification != nil {
			m.notification.ShowShort(fmt.Sprintf("No save in slot %d", m.currentSlot))
		}
		return fmt.Errorf("no save in slot %d", m.currentSlot)
	}

	state, err := os.ReadFile(statePath)
	if err != nil {
		return fmt.Errorf("failed to read state file: %w", err)
	}

	if err := saveStater.Deserialize(state); err != nil {
		return fmt.Errorf("failed to deserialize state: %w", err)
	}

	if m.notification != nil {
		m.notification.ShowShort("State loaded")
	}

	return nil
}

// SaveResume saves the resume state
func (m *SaveStateManager) SaveResume(saveStater emucore.SaveStater) error {
	if m.gameCRC == "" {
		return fmt.Errorf("no game set")
	}

	saveDir, err := storage.GetGameSaveDir(m.gameCRC)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(saveDir, 0755); err != nil {
		return fmt.Errorf("failed to create save directory: %w", err)
	}

	statePath := filepath.Join(saveDir, "resume.state")

	state, err := saveStater.Serialize()
	if err != nil {
		return fmt.Errorf("failed to serialize state: %w", err)
	}

	return os.WriteFile(statePath, state, 0644)
}

// SaveResumeData saves pre-serialized state data as the resume state.
// Used by the auto-save system where the emu goroutine caches serialized state.
func (m *SaveStateManager) SaveResumeData(state []byte) error {
	if m.gameCRC == "" {
		return fmt.Errorf("no game set")
	}

	saveDir, err := storage.GetGameSaveDir(m.gameCRC)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(saveDir, 0755); err != nil {
		return fmt.Errorf("failed to create save directory: %w", err)
	}

	statePath := filepath.Join(saveDir, "resume.state")
	return os.WriteFile(statePath, state, 0644)
}

// LoadResume loads the resume state
func (m *SaveStateManager) LoadResume(saveStater emucore.SaveStater) error {
	if m.gameCRC == "" {
		return fmt.Errorf("no game set")
	}

	saveDir, err := storage.GetGameSaveDir(m.gameCRC)
	if err != nil {
		return err
	}

	statePath := filepath.Join(saveDir, "resume.state")

	state, err := os.ReadFile(statePath)
	if err != nil {
		return err
	}

	return saveStater.Deserialize(state)
}

// HasResumeState checks if a resume state exists
func (m *SaveStateManager) HasResumeState() bool {
	if m.gameCRC == "" {
		return false
	}

	saveDir, err := storage.GetGameSaveDir(m.gameCRC)
	if err != nil {
		return false
	}

	statePath := filepath.Join(saveDir, "resume.state")
	_, err = os.Stat(statePath)
	return err == nil
}

// SaveSRAM saves the cartridge SRAM
func (m *SaveStateManager) SaveSRAM(batterySaver emucore.BatterySaver) error {
	if m.gameCRC == "" {
		return fmt.Errorf("no game set")
	}

	if !batterySaver.HasSRAM() {
		return nil
	}

	saveDir, err := storage.GetGameSaveDir(m.gameCRC)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(saveDir, 0755); err != nil {
		return fmt.Errorf("failed to create save directory: %w", err)
	}

	sramPath := filepath.Join(saveDir, "cart.srm")
	return os.WriteFile(sramPath, batterySaver.GetSRAM(), 0644)
}

// LoadSRAM loads the cartridge SRAM
func (m *SaveStateManager) LoadSRAM(batterySaver emucore.BatterySaver) error {
	if m.gameCRC == "" {
		return nil
	}

	if !batterySaver.HasSRAM() {
		return nil
	}

	saveDir, err := storage.GetGameSaveDir(m.gameCRC)
	if err != nil {
		return nil
	}

	sramPath := filepath.Join(saveDir, "cart.srm")

	data, err := os.ReadFile(sramPath)
	if err != nil {
		return nil // No SRAM file is OK
	}

	batterySaver.SetSRAM(data)
	return nil
}
