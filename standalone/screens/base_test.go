//go:build !libretro

package screens

import (
	"image/color"
	"testing"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/standalone/types"
)

func TestInitBase(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	if b.focusButtons == nil {
		t.Error("focusButtons should be initialized")
	}
	if b.navZones == nil {
		t.Error("navZones should be initialized")
	}
	if b.navTransitions == nil {
		t.Error("navTransitions should be initialized")
	}
	if b.buttonToZone == nil {
		t.Error("buttonToZone should be initialized")
	}
}

func TestRegisterNavZone(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	keys := []string{"btn1", "btn2", "btn3"}
	b.RegisterNavZone("toolbar", types.NavZoneHorizontal, keys, 0)

	zone := b.navZones["toolbar"]
	if zone == nil {
		t.Fatal("zone should be registered")
	}
	if zone.Type != types.NavZoneHorizontal {
		t.Errorf("zone type = %q, want %q", zone.Type, types.NavZoneHorizontal)
	}
	if len(zone.Keys) != 3 {
		t.Errorf("zone should have 3 keys, got %d", len(zone.Keys))
	}

	// Check button-to-zone mapping
	for _, key := range keys {
		if b.buttonToZone[key] != "toolbar" {
			t.Errorf("button %q should map to zone 'toolbar'", key)
		}
	}
}

func TestSetNavTransition(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	b.SetNavTransition("toolbar", types.DirDown, "grid", types.NavIndexFirst)

	transitions := b.navTransitions["toolbar"]
	if transitions == nil {
		t.Fatal("transitions should be registered")
	}
	tr := transitions[types.DirDown]
	if tr == nil {
		t.Fatal("down transition should exist")
	}
	if tr.ToZone != "grid" {
		t.Errorf("toZone = %q, want 'grid'", tr.ToZone)
	}
	if tr.ToIndex != types.NavIndexFirst {
		t.Errorf("toIndex = %d, want %d", tr.ToIndex, types.NavIndexFirst)
	}
}

func TestSetPendingFocus(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	b.SetPendingFocus("btn1")
	if b.pendingFocus != "btn1" {
		t.Errorf("pendingFocus = %q, want 'btn1'", b.pendingFocus)
	}
}

func TestSetDefaultFocusNoOverride(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	// SetDefaultFocus should set when empty
	b.SetDefaultFocus("btn1")
	if b.pendingFocus != "btn1" {
		t.Errorf("pendingFocus = %q, want 'btn1'", b.pendingFocus)
	}

	// SetDefaultFocus should NOT override existing
	b.SetDefaultFocus("btn2")
	if b.pendingFocus != "btn1" {
		t.Errorf("pendingFocus should remain 'btn1', got %q", b.pendingFocus)
	}
}

func TestClearPendingFocus(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	b.SetPendingFocus("btn1")
	b.ClearPendingFocus()
	if b.pendingFocus != "" {
		t.Errorf("pendingFocus should be empty, got %q", b.pendingFocus)
	}
}

func TestClearFocusButtons(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	b.RegisterNavZone("zone1", types.NavZoneHorizontal, []string{"a", "b"}, 0)
	b.SetNavTransition("zone1", types.DirDown, "zone2", 0)

	b.ClearFocusButtons()

	if len(b.focusButtons) != 0 {
		t.Error("focusButtons should be empty")
	}
	if len(b.navZones) != 0 {
		t.Error("navZones should be empty")
	}
	if len(b.navTransitions) != 0 {
		t.Error("navTransitions should be empty")
	}
	if len(b.buttonToZone) != 0 {
		t.Error("buttonToZone should be empty")
	}
}

func TestGetPendingFocusButtonNoFocus(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	btn := b.GetPendingFocusButton()
	if btn != nil {
		t.Error("should return nil when no pending focus")
	}
}

func TestGetPendingFocusButtonNotFound(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	b.SetPendingFocus("nonexistent")
	btn := b.GetPendingFocusButton()
	if btn != nil {
		t.Error("should return nil when button not registered")
	}
}

// --- Horizontal navigation ---

func TestNavigateHorizontalLeft(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	idx, transition := b.navigateHorizontal(2, 5, types.DirLeft)
	if idx != 1 || transition {
		t.Errorf("left from 2: got idx=%d transition=%v, want idx=1 transition=false", idx, transition)
	}
}

func TestNavigateHorizontalLeftEdge(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	idx, transition := b.navigateHorizontal(0, 5, types.DirLeft)
	if !transition {
		t.Errorf("left from 0: got idx=%d transition=%v, want transition=true", idx, transition)
	}
}

func TestNavigateHorizontalRight(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	idx, transition := b.navigateHorizontal(1, 5, types.DirRight)
	if idx != 2 || transition {
		t.Errorf("right from 1: got idx=%d transition=%v, want idx=2 transition=false", idx, transition)
	}
}

func TestNavigateHorizontalRightEdge(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	idx, transition := b.navigateHorizontal(4, 5, types.DirRight)
	if !transition {
		t.Errorf("right from last: got idx=%d transition=%v, want transition=true", idx, transition)
	}
}

func TestNavigateHorizontalUpDown(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	_, upTransition := b.navigateHorizontal(2, 5, types.DirUp)
	if !upTransition {
		t.Error("up in horizontal should always transition")
	}

	_, downTransition := b.navigateHorizontal(2, 5, types.DirDown)
	if !downTransition {
		t.Error("down in horizontal should always transition")
	}
}

// --- Vertical navigation ---

func TestNavigateVerticalUp(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	idx, transition := b.navigateVertical(2, 5, types.DirUp)
	if idx != 1 || transition {
		t.Errorf("up from 2: got idx=%d transition=%v, want idx=1 transition=false", idx, transition)
	}
}

func TestNavigateVerticalUpEdge(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	_, transition := b.navigateVertical(0, 5, types.DirUp)
	if !transition {
		t.Error("up from 0 should transition")
	}
}

func TestNavigateVerticalDown(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	idx, transition := b.navigateVertical(1, 5, types.DirDown)
	if idx != 2 || transition {
		t.Errorf("down from 1: got idx=%d transition=%v, want idx=2 transition=false", idx, transition)
	}
}

func TestNavigateVerticalDownEdge(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	_, transition := b.navigateVertical(4, 5, types.DirDown)
	if !transition {
		t.Error("down from last should transition")
	}
}

func TestNavigateVerticalLeftRight(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	_, leftTransition := b.navigateVertical(2, 5, types.DirLeft)
	if !leftTransition {
		t.Error("left in vertical should always transition")
	}

	_, rightTransition := b.navigateVertical(2, 5, types.DirRight)
	if !rightTransition {
		t.Error("right in vertical should always transition")
	}
}

// --- Grid navigation ---

func TestNavigateGridLeft(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	// 4x3 grid (12 items, 4 columns), index 5 = row 1, col 1
	idx, transition := b.navigateGrid(5, 12, 4, types.DirLeft)
	if idx != 4 || transition {
		t.Errorf("left from col 1: got idx=%d transition=%v, want idx=4 transition=false", idx, transition)
	}
}

func TestNavigateGridLeftEdge(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	// index 4 = row 1, col 0
	_, transition := b.navigateGrid(4, 12, 4, types.DirLeft)
	if !transition {
		t.Error("left from col 0 should transition")
	}
}

func TestNavigateGridRight(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	// index 5 = row 1, col 1 -> should go to col 2
	idx, transition := b.navigateGrid(5, 12, 4, types.DirRight)
	if idx != 6 || transition {
		t.Errorf("right from col 1: got idx=%d transition=%v, want idx=6 transition=false", idx, transition)
	}
}

func TestNavigateGridRightEdge(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	// index 7 = row 1, col 3 (last column in 4-column grid)
	_, transition := b.navigateGrid(7, 12, 4, types.DirRight)
	if !transition {
		t.Error("right from last col should transition")
	}
}

func TestNavigateGridUp(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	// index 5 = row 1, col 1 -> should go to row 0, col 1 = index 1
	idx, transition := b.navigateGrid(5, 12, 4, types.DirUp)
	if idx != 1 || transition {
		t.Errorf("up from row 1: got idx=%d transition=%v, want idx=1 transition=false", idx, transition)
	}
}

func TestNavigateGridUpEdge(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	// index 1 = row 0, col 1
	_, transition := b.navigateGrid(1, 12, 4, types.DirUp)
	if !transition {
		t.Error("up from row 0 should transition")
	}
}

func TestNavigateGridDown(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	// index 1 = row 0, col 1 -> should go to row 1, col 1 = index 5
	idx, transition := b.navigateGrid(1, 12, 4, types.DirDown)
	if idx != 5 || transition {
		t.Errorf("down from row 0: got idx=%d transition=%v, want idx=5 transition=false", idx, transition)
	}
}

func TestNavigateGridDownEdge(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	// index 9 = row 2, col 1 (last row in 12-item 4-column grid)
	_, transition := b.navigateGrid(9, 12, 4, types.DirDown)
	if !transition {
		t.Error("down from last row should transition")
	}
}

func TestNavigateGridDownIncompleteRow(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	// 10 items, 4 columns: rows [0-3], [4-7], [8-9]
	// index 7 = row 1, col 3 -> down would go to index 11 which doesn't exist
	_, transition := b.navigateGrid(7, 10, 4, types.DirDown)
	if !transition {
		t.Error("down into non-existent position should transition")
	}
}

func TestNavigateGridZeroColumns(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	// columns=0 should default to 1
	idx, transition := b.navigateGrid(0, 5, 0, types.DirDown)
	if idx != 1 || transition {
		t.Errorf("with 0 columns: got idx=%d transition=%v, want idx=1 transition=false", idx, transition)
	}
}

func TestNavigateGridSingleColumn(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	// 1 column = vertical behavior
	_, transition := b.navigateGrid(0, 5, 1, types.DirLeft)
	if !transition {
		t.Error("left in single-column grid should transition")
	}

	_, transition = b.navigateGrid(0, 5, 1, types.DirRight)
	if !transition {
		t.Error("right in single-column grid should transition")
	}
}

func TestNavigateGridRightEdgeLastItem(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	// 10 items, 4 columns. Index 9 = row 2, col 1.
	// Right from col 1 would go to col 2 (index 10) which exists
	// Wait, index 9 is the last item. col 1. Right goes to index 10 but total=10 so 10 >= total.
	// Actually let me recalculate: col = 9 % 4 = 1, and col < columns-1 (1 < 3) and 9+1=10 which is NOT < 10
	_, transition := b.navigateGrid(9, 10, 4, types.DirRight)
	if !transition {
		t.Error("right to beyond total should transition")
	}
}

// --- Zone transition helper ---

func TestHandleZoneTransitionNoTransitions(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	btn := b.handleZoneTransition("zone1", 0, &NavZone{}, types.DirDown)
	if btn != nil {
		t.Error("should return nil when no transitions defined")
	}
}

func TestHandleZoneTransitionNoDirection(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	b.SetNavTransition("zone1", types.DirDown, "zone2", types.NavIndexFirst)

	// Ask for DirUp which has no transition
	btn := b.handleZoneTransition("zone1", 0, &NavZone{}, types.DirUp)
	if btn != nil {
		t.Error("should return nil for undefined direction")
	}
}

func TestHandleZoneTransitionTargetNotFound(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	b.SetNavTransition("zone1", types.DirDown, "nonexistent", types.NavIndexFirst)

	btn := b.handleZoneTransition("zone1", 0, &NavZone{}, types.DirDown)
	if btn != nil {
		t.Error("should return nil when target zone doesn't exist")
	}
}

// --- calculatePreservedIndex ---

func TestCalculatePreservedIndexGridToHorizontal(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	fromZone := &NavZone{Type: types.NavZoneGrid, Columns: 4}
	toZone := &NavZone{Type: types.NavZoneHorizontal, Keys: []string{"a", "b", "c"}}

	// From col 0 of 4 columns -> proportional index in 3 items = 0
	idx := b.calculatePreservedIndex(0, fromZone, toZone, types.DirUp)
	if idx != 0 {
		t.Errorf("col 0 -> horizontal: got %d, want 0", idx)
	}

	// From col 3 of 4 columns -> proportional = 3/4 * 2 = 1.5 -> rounds to 2
	idx = b.calculatePreservedIndex(3, fromZone, toZone, types.DirUp)
	if idx != 2 {
		t.Errorf("col 3 -> horizontal: got %d, want 2", idx)
	}
}

func TestCalculatePreservedIndexGridToGridDown(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	fromZone := &NavZone{Type: types.NavZoneGrid, Columns: 4}
	toZone := &NavZone{Type: types.NavZoneGrid, Columns: 4, Keys: []string{"a", "b", "c", "d", "e", "f"}}

	// Going down: should land at first row, same column
	idx := b.calculatePreservedIndex(2, fromZone, toZone, types.DirDown)
	if idx != 2 {
		t.Errorf("grid->grid down col 2: got %d, want 2", idx)
	}
}

func TestCalculatePreservedIndexGridToGridUp(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	fromZone := &NavZone{Type: types.NavZoneGrid, Columns: 4}
	toZone := &NavZone{Type: types.NavZoneGrid, Columns: 4, Keys: []string{"a", "b", "c", "d", "e", "f"}}

	// Going up: should land at last row, same column
	// 6 items, 4 cols -> last row start = (5/4)*4 = 4, target = 4+2 = 6 but >= len so 5
	idx := b.calculatePreservedIndex(2, fromZone, toZone, types.DirUp)
	if idx != 5 {
		t.Errorf("grid->grid up col 2: got %d, want 5", idx)
	}
}

func TestCalculatePreservedIndexDefaultUp(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	// Non-grid zones fall through to default
	fromZone := &NavZone{Type: types.NavZoneVertical}
	toZone := &NavZone{Type: types.NavZoneVertical, Keys: []string{"a", "b", "c"}}

	idx := b.calculatePreservedIndex(0, fromZone, toZone, types.DirUp)
	// DirUp default = last
	if idx != 2 {
		t.Errorf("default up: got %d, want 2 (last)", idx)
	}
}

func TestCalculatePreservedIndexDefaultDown(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	fromZone := &NavZone{Type: types.NavZoneVertical}
	toZone := &NavZone{Type: types.NavZoneVertical, Keys: []string{"a", "b", "c"}}

	idx := b.calculatePreservedIndex(0, fromZone, toZone, types.DirDown)
	// DirDown default = first
	if idx != 0 {
		t.Errorf("default down: got %d, want 0 (first)", idx)
	}
}

func TestFindFocusInDirectionNilCurrent(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	btn := b.FindFocusInDirection(nil, types.DirUp)
	if btn != nil {
		t.Error("should return nil for nil current")
	}
}

func TestFindFocusInDirectionNoButtons(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	// No buttons registered - even with a mock focuser this should return nil
	// We can't easily mock widget.Focuser without the full widget system,
	// but we can verify the nil/empty guard
	btn := b.FindFocusInDirection(nil, types.DirDown)
	if btn != nil {
		t.Error("should return nil with no buttons")
	}
}

// --- SaveFocusState ---

// testButtonImage creates a minimal ButtonImage for testing
func testButtonImage() *widget.ButtonImage {
	return &widget.ButtonImage{
		Idle: eimage.NewNineSliceColor(color.NRGBA{}),
	}
}

// testButton creates a minimal Button for testing
func testButton() *widget.Button {
	return widget.NewButton(widget.ButtonOpts.Image(testButtonImage()))
}

func TestSaveFocusStateNilFocused(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	b.SaveFocusState(nil)
	if b.pendingFocus != "" {
		t.Errorf("pendingFocus should be empty, got %q", b.pendingFocus)
	}
}

func TestSaveFocusStatePendingAlreadySet(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	btn := testButton()
	b.RegisterFocusButton("play", btn)
	b.SetPendingFocus("play")

	// Pass the same button as focused - should NOT override existing pending
	b.SaveFocusState(btn)
	if b.pendingFocus != "play" {
		t.Errorf("pendingFocus should remain 'play', got %q", b.pendingFocus)
	}
}

func TestSaveFocusStateMatchesRegistered(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	btn := testButton()
	b.RegisterFocusButton("play", btn)

	// Pass the registered button as focused - should save its key
	b.SaveFocusState(btn)
	if b.pendingFocus != "play" {
		t.Errorf("pendingFocus = %q, want 'play'", b.pendingFocus)
	}
}

func TestSaveFocusStateNoMatch(t *testing.T) {
	b := &BaseScreen{}
	b.InitBase()

	btn1 := testButton()
	btn2 := testButton()
	b.RegisterFocusButton("play", btn1)

	// Pass a different button as focused - should not set pendingFocus
	b.SaveFocusState(btn2)
	if b.pendingFocus != "" {
		t.Errorf("pendingFocus should be empty, got %q", b.pendingFocus)
	}
}
