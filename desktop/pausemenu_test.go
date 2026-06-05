package desktop

import (
	"testing"

	"github.com/user-none/eblitui/desktop/types"
)

func TestNewPauseMenu(t *testing.T) {
	m := NewPauseMenu(nil, nil, nil)

	if m.IsVisible() {
		t.Error("should not be visible initially")
	}
	if m.selectedIndex != 0 {
		t.Errorf("selectedIndex should be 0, got %d", m.selectedIndex)
	}
}

func TestPauseMenuShow(t *testing.T) {
	m := NewPauseMenu(nil, nil, nil)

	m.Show()
	if !m.IsVisible() {
		t.Error("should be visible after Show()")
	}
	if m.selectedIndex != 0 {
		t.Errorf("selectedIndex should be 0 after Show(), got %d", m.selectedIndex)
	}
}

func TestPauseMenuHide(t *testing.T) {
	m := NewPauseMenu(nil, nil, nil)

	m.Show()
	m.Hide()
	if m.IsVisible() {
		t.Error("should not be visible after Hide()")
	}
}

func TestPauseMenuShowResetsSelection(t *testing.T) {
	m := NewPauseMenu(nil, nil, nil)

	m.selectedIndex = 2
	m.Show()
	if m.selectedIndex != 0 {
		t.Errorf("Show() should reset selectedIndex to 0, got %d", m.selectedIndex)
	}
}

func TestHandleSelectResume(t *testing.T) {
	resumed := false
	m := NewPauseMenu(func() { resumed = true }, nil, nil)

	m.Show()
	m.selectedIndex = int(PauseMenuResume)
	m.handleSelect()

	if !resumed {
		t.Error("onResume should have been called")
	}
	if m.IsVisible() {
		t.Error("menu should be hidden after Resume")
	}
}

func TestHandleSelectLibrary(t *testing.T) {
	libraryCalled := false
	m := NewPauseMenu(nil, func() { libraryCalled = true }, nil)

	m.Show()
	m.selectedIndex = int(PauseMenuLibrary)
	m.handleSelect()

	if !libraryCalled {
		t.Error("onLibrary should have been called")
	}
	if m.IsVisible() {
		t.Error("menu should be hidden after Library")
	}
}

func TestHandleSelectExit(t *testing.T) {
	exitCalled := false
	m := NewPauseMenu(nil, nil, func() { exitCalled = true })

	m.Show()
	m.selectedIndex = int(PauseMenuExit)
	m.handleSelect()

	if !exitCalled {
		t.Error("onExit should have been called")
	}
	if m.IsVisible() {
		t.Error("menu should be hidden after Exit")
	}
}

func TestHandleSelectNilCallbacks(t *testing.T) {
	m := NewPauseMenu(nil, nil, nil)
	m.Show()

	// None of these should panic
	m.selectedIndex = int(PauseMenuResume)
	m.handleSelect()

	m.Show()
	m.selectedIndex = int(PauseMenuLibrary)
	m.handleSelect()

	m.Show()
	m.selectedIndex = int(PauseMenuExit)
	m.handleSelect()
}

func TestPauseMenuOptionCount(t *testing.T) {
	if PauseMenuOptionCount != 3 {
		t.Errorf("PauseMenuOptionCount should be 3, got %d", PauseMenuOptionCount)
	}
}

func TestApplyNavigationDown(t *testing.T) {
	m := NewPauseMenu(nil, nil, nil)
	m.Show()

	if handled := m.applyNavigation(UINavigation{Direction: types.DirDown}); handled {
		t.Error("Down navigation should not resolve the menu")
	}
	if m.selectedIndex != 1 {
		t.Errorf("selectedIndex should be 1 after Down, got %d", m.selectedIndex)
	}
}

func TestApplyNavigationUp(t *testing.T) {
	m := NewPauseMenu(nil, nil, nil)
	m.Show()
	m.selectedIndex = 2

	m.applyNavigation(UINavigation{Direction: types.DirUp})
	if m.selectedIndex != 1 {
		t.Errorf("selectedIndex should be 1 after Up, got %d", m.selectedIndex)
	}
}

func TestApplyNavigationClampsTop(t *testing.T) {
	m := NewPauseMenu(nil, nil, nil)
	m.Show()
	m.selectedIndex = 0

	m.applyNavigation(UINavigation{Direction: types.DirUp})
	if m.selectedIndex != 0 {
		t.Errorf("Up at top should clamp to 0 (no wrap), got %d", m.selectedIndex)
	}
}

func TestApplyNavigationClampsBottom(t *testing.T) {
	m := NewPauseMenu(nil, nil, nil)
	m.Show()
	m.selectedIndex = int(PauseMenuOptionCount) - 1

	m.applyNavigation(UINavigation{Direction: types.DirDown})
	if m.selectedIndex != int(PauseMenuOptionCount)-1 {
		t.Errorf("Down at bottom should clamp (no wrap), got %d", m.selectedIndex)
	}
}

func TestApplyNavigationActivateSelects(t *testing.T) {
	libraryCalled := false
	m := NewPauseMenu(nil, func() { libraryCalled = true }, nil)
	m.Show()
	m.selectedIndex = int(PauseMenuLibrary)

	if handled := m.applyNavigation(UINavigation{Activate: true}); !handled {
		t.Error("Activate should resolve the menu")
	}
	if !libraryCalled {
		t.Error("Activate should trigger the selected option")
	}
	if m.IsVisible() {
		t.Error("menu should be hidden after Activate")
	}
}

func TestApplyNavigationBackResumes(t *testing.T) {
	resumed := false
	m := NewPauseMenu(func() { resumed = true }, nil, nil)
	m.Show()
	m.selectedIndex = int(PauseMenuExit)

	if handled := m.applyNavigation(UINavigation{Back: true}); !handled {
		t.Error("Back should resolve the menu")
	}
	if !resumed {
		t.Error("Back should resume regardless of selection")
	}
	if m.IsVisible() {
		t.Error("menu should be hidden after Back")
	}
}

func TestApplyNavigationStartResumes(t *testing.T) {
	resumed := false
	m := NewPauseMenu(func() { resumed = true }, nil, nil)
	m.Show()
	m.selectedIndex = int(PauseMenuLibrary)

	if handled := m.applyNavigation(UINavigation{OpenSettings: true}); !handled {
		t.Error("Start should resolve the menu")
	}
	if !resumed {
		t.Error("Start should resume regardless of selection")
	}
}

func TestApplyNavigationNoInput(t *testing.T) {
	m := NewPauseMenu(nil, nil, nil)
	m.Show()
	m.selectedIndex = 1

	if handled := m.applyNavigation(UINavigation{}); handled {
		t.Error("empty navigation should not resolve the menu")
	}
	if m.selectedIndex != 1 {
		t.Errorf("selectedIndex should be unchanged, got %d", m.selectedIndex)
	}
}
