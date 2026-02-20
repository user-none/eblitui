//go:build !libretro

package standalone

import "testing"

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
