//go:build !libretro

package screens

import (
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/standalone/types"
)

// NavZone defines a navigation zone with ordered buttons
type NavZone struct {
	Type    string   // types.NavZoneHorizontal, types.NavZoneVertical, or types.NavZoneGrid
	Keys    []string // Button keys in order (left-to-right for horizontal, top-to-bottom for vertical, row-major for grid)
	Columns int      // Number of columns for grid zones (ignored for other types)
}

// NavTransition defines where to go when leaving a zone
type NavTransition struct {
	ToZone  string // Target zone name
	ToIndex int    // Index in target zone (-1 = preserve column/position, -2 = first, -3 = last)
}

// BaseScreen provides common scroll and focus management for screens.
// Embed this in screen structs to get scroll position preservation
// and focus restoration after rebuilds.
type BaseScreen struct {
	// Scroll container and slider for scroll position preservation
	scrollContainer *widget.ScrollContainer
	vSlider         *widget.Slider
	scrollTop       float64

	// Button references for focus restoration (maps key to button)
	focusButtons map[string]*widget.Button

	// Key of button to restore focus to after rebuild
	pendingFocus string

	// Zone-based navigation
	navZones       map[string]*NavZone               // Zone name -> zone definition
	navTransitions map[string]map[int]*NavTransition // Zone name -> direction -> transition
	buttonToZone   map[string]string                 // Button key -> zone name
}

// InitBase initializes the base screen state.
// Call this in the screen's constructor.
func (b *BaseScreen) InitBase() {
	b.focusButtons = make(map[string]*widget.Button)
	b.navZones = make(map[string]*NavZone)
	b.navTransitions = make(map[string]map[int]*NavTransition)
	b.buttonToZone = make(map[string]string)
}

// SetScrollWidgets stores references to the scroll widgets for position preservation.
// Call this during Build() after creating the scroll container.
func (b *BaseScreen) SetScrollWidgets(scrollContainer *widget.ScrollContainer, vSlider *widget.Slider) {
	b.scrollContainer = scrollContainer
	b.vSlider = vSlider
}

// SaveScrollPosition saves the current scroll position.
// Call this before rebuilding the screen.
func (b *BaseScreen) SaveScrollPosition() {
	if b.scrollContainer != nil {
		b.scrollTop = b.scrollContainer.ScrollTop
	}
}

// RestoreScrollPosition restores the saved scroll position.
// Call this after rebuilding the screen, once the scroll container is set.
func (b *BaseScreen) RestoreScrollPosition() {
	if b.scrollContainer != nil && b.scrollTop > 0 {
		b.scrollContainer.ScrollTop = b.scrollTop
		if b.vSlider != nil {
			b.vSlider.Current = int(b.scrollTop * 1000)
		}
	}
}

// RegisterFocusButton registers a button for focus restoration.
// Call this during Build() for each focusable button.
func (b *BaseScreen) RegisterFocusButton(key string, btn *widget.Button) {
	if b.focusButtons == nil {
		b.focusButtons = make(map[string]*widget.Button)
	}
	b.focusButtons[key] = btn
}

// SaveFocusState checks which registered focus button currently has focus
// and saves its key as pending focus. This preserves focus across rebuilds
// triggered by async operations (e.g., achievement loading).
// Does nothing if pendingFocus is already set (e.g., by OnEnter).
func (b *BaseScreen) SaveFocusState(focused widget.Focuser) {
	if b.pendingFocus != "" || focused == nil {
		return
	}
	focusedWidget := focused.GetWidget()
	if focusedWidget == nil {
		return
	}
	for key, btn := range b.focusButtons {
		if btn.GetWidget() == focusedWidget {
			b.pendingFocus = key
			return
		}
	}
}

// ClearFocusButtons clears all registered focus buttons and navigation zones.
// Call this at the start of Build() before registering new buttons.
func (b *BaseScreen) ClearFocusButtons() {
	b.focusButtons = make(map[string]*widget.Button)
	b.navZones = make(map[string]*NavZone)
	b.navTransitions = make(map[string]map[int]*NavTransition)
	b.buttonToZone = make(map[string]string)
}

// SetPendingFocus sets the key of the button to focus after rebuild.
func (b *BaseScreen) SetPendingFocus(key string) {
	b.pendingFocus = key
}

// SetDefaultFocus sets the pending focus only if no focus is currently pending.
// Use this in OnEnter() to set initial focus without overriding restored focus.
func (b *BaseScreen) SetDefaultFocus(key string) {
	if b.pendingFocus == "" {
		b.pendingFocus = key
	}
}

// GetPendingFocusButton returns the button that should receive focus after rebuild.
// Returns nil if no pending focus or button not found.
func (b *BaseScreen) GetPendingFocusButton() *widget.Button {
	if b.pendingFocus == "" {
		return nil
	}
	return b.focusButtons[b.pendingFocus]
}

// ClearPendingFocus clears the pending focus state.
func (b *BaseScreen) ClearPendingFocus() {
	b.pendingFocus = ""
}

// RegisterNavZone registers a navigation zone with ordered buttons.
// For grid zones, keys should be in row-major order (left-to-right, top-to-bottom).
// zoneType should be types.NavZoneHorizontal, types.NavZoneVertical, or types.NavZoneGrid.
// columns is only used for grid zones.
func (b *BaseScreen) RegisterNavZone(name string, zoneType string, keys []string, columns int) {
	zone := &NavZone{
		Type:    zoneType,
		Keys:    keys,
		Columns: columns,
	}
	b.navZones[name] = zone
	for _, key := range keys {
		b.buttonToZone[key] = name
	}
}

// SetNavTransition defines where to navigate when leaving a zone in a direction.
func (b *BaseScreen) SetNavTransition(fromZone string, direction int, toZone string, toIndex int) {
	if b.navTransitions[fromZone] == nil {
		b.navTransitions[fromZone] = make(map[int]*NavTransition)
	}
	b.navTransitions[fromZone][direction] = &NavTransition{
		ToZone:  toZone,
		ToIndex: toIndex,
	}
}

// EnsureFocusedVisible scrolls the view to ensure the focused widget is visible.
// The isScrollableButton function should return true if the focused widget
// should trigger scrolling (e.g., game buttons but not toolbar buttons).
func (b *BaseScreen) EnsureFocusedVisible(focused widget.Focuser, isScrollableButton func(*widget.Button) bool) {
	if focused == nil || b.scrollContainer == nil {
		return
	}

	// Check if this widget should trigger scrolling
	btn, ok := focused.(*widget.Button)
	if !ok {
		return
	}
	if isScrollableButton != nil && !isScrollableButton(btn) {
		return
	}

	// Get the focused widget's rectangle
	focusWidget := focused.GetWidget()
	if focusWidget == nil {
		return
	}
	focusRect := focusWidget.Rect

	// Get the scroll container's view rect (visible area on screen)
	viewRect := b.scrollContainer.ViewRect()
	contentRect := b.scrollContainer.ContentRect()

	// If content fits in view, no scrolling needed
	if contentRect.Dy() <= viewRect.Dy() {
		return
	}

	// Current scroll offset in pixels
	maxScroll := contentRect.Dy() - viewRect.Dy()
	scrollOffset := int(b.scrollContainer.ScrollTop * float64(maxScroll))

	// Widget's position relative to view top
	widgetTopInView := focusRect.Min.Y - viewRect.Min.Y
	widgetBottomInView := focusRect.Max.Y - viewRect.Min.Y
	viewHeight := viewRect.Dy()

	// Check if widget top is above the visible area
	if widgetTopInView < 0 {
		// Scroll up: align widget top with view top
		newScrollOffset := scrollOffset + widgetTopInView
		if newScrollOffset < 0 {
			newScrollOffset = 0
		}
		b.scrollContainer.ScrollTop = float64(newScrollOffset) / float64(maxScroll)
		if b.vSlider != nil {
			b.vSlider.Current = int(b.scrollContainer.ScrollTop * 1000)
		}
	} else if widgetBottomInView > viewHeight {
		// Scroll down: align widget bottom with view bottom (minimal scroll)
		newScrollOffset := scrollOffset + (widgetBottomInView - viewHeight)
		if newScrollOffset > maxScroll {
			newScrollOffset = maxScroll
		}
		b.scrollContainer.ScrollTop = float64(newScrollOffset) / float64(maxScroll)
		if b.vSlider != nil {
			b.vSlider.Current = int(b.scrollContainer.ScrollTop * 1000)
		}
	}
}

// FindFocusInDirection finds the next button in the specified direction
// using zone-based navigation. Falls back to spatial navigation if no zones defined.
// Returns nil if no suitable target found.
func (b *BaseScreen) FindFocusInDirection(current widget.Focuser, direction int) *widget.Button {
	if current == nil || len(b.focusButtons) == 0 {
		return nil
	}

	// Find current button's key
	currentKey := ""
	for key, btn := range b.focusButtons {
		if btn.GetWidget() == current.GetWidget() {
			currentKey = key
			break
		}
	}

	if currentKey == "" {
		return nil
	}

	// If zones are defined, use zone-based navigation
	if len(b.navZones) > 0 {
		return b.findFocusInZone(currentKey, direction)
	}

	// Fallback to spatial navigation (legacy behavior)
	return b.findFocusSpatial(current, direction)
}

// findFocusInZone implements zone-aware navigation
func (b *BaseScreen) findFocusInZone(currentKey string, direction int) *widget.Button {
	zoneName, ok := b.buttonToZone[currentKey]
	if !ok {
		return nil
	}

	zone := b.navZones[zoneName]
	if zone == nil {
		return nil
	}

	// Find current index in zone
	currentIndex := -1
	for i, key := range zone.Keys {
		if key == currentKey {
			currentIndex = i
			break
		}
	}
	if currentIndex == -1 {
		return nil
	}

	// Calculate navigation based on zone type
	var targetIndex int
	var shouldTransition bool

	switch zone.Type {
	case types.NavZoneHorizontal:
		targetIndex, shouldTransition = b.navigateHorizontal(currentIndex, len(zone.Keys), direction)
	case types.NavZoneVertical:
		targetIndex, shouldTransition = b.navigateVertical(currentIndex, len(zone.Keys), direction)
	case types.NavZoneGrid:
		targetIndex, shouldTransition = b.navigateGrid(currentIndex, len(zone.Keys), zone.Columns, direction)
	default:
		return nil
	}

	// If we should transition to another zone
	if shouldTransition {
		return b.handleZoneTransition(zoneName, currentIndex, zone, direction)
	}

	// Stay within zone
	if targetIndex >= 0 && targetIndex < len(zone.Keys) {
		return b.focusButtons[zone.Keys[targetIndex]]
	}

	return nil
}

// navigateHorizontal handles navigation in a horizontal zone
// Returns (targetIndex, shouldTransition)
func (b *BaseScreen) navigateHorizontal(currentIndex, total, direction int) (int, bool) {
	switch direction {
	case types.DirLeft:
		if currentIndex > 0 {
			return currentIndex - 1, false
		}
		return -1, true // Transition out
	case types.DirRight:
		if currentIndex < total-1 {
			return currentIndex + 1, false
		}
		return -1, true // Transition out
	case types.DirUp, types.DirDown:
		return -1, true // Always transition for perpendicular directions
	}
	return -1, false
}

// navigateVertical handles navigation in a vertical zone
// Returns (targetIndex, shouldTransition)
func (b *BaseScreen) navigateVertical(currentIndex, total, direction int) (int, bool) {
	switch direction {
	case types.DirUp:
		if currentIndex > 0 {
			return currentIndex - 1, false
		}
		return -1, true // Transition out
	case types.DirDown:
		if currentIndex < total-1 {
			return currentIndex + 1, false
		}
		return -1, true // Transition out
	case types.DirLeft, types.DirRight:
		return -1, true // Always transition for perpendicular directions
	}
	return -1, false
}

// navigateGrid handles navigation in a grid zone
// Returns (targetIndex, shouldTransition)
func (b *BaseScreen) navigateGrid(currentIndex, total, columns, direction int) (int, bool) {
	if columns <= 0 {
		columns = 1
	}

	row := currentIndex / columns
	col := currentIndex % columns
	totalRows := (total + columns - 1) / columns

	switch direction {
	case types.DirLeft:
		if col > 0 {
			return currentIndex - 1, false
		}
		return -1, true // At left edge, transition out
	case types.DirRight:
		if col < columns-1 && currentIndex+1 < total {
			return currentIndex + 1, false
		}
		return -1, true // At right edge, transition out
	case types.DirUp:
		if row > 0 {
			return currentIndex - columns, false
		}
		return -1, true // At top row, transition out
	case types.DirDown:
		nextIndex := currentIndex + columns
		if row < totalRows-1 && nextIndex < total {
			return nextIndex, false
		}
		return -1, true // At bottom row, transition out
	}
	return -1, false
}

// handleZoneTransition handles transitioning to another zone
func (b *BaseScreen) handleZoneTransition(fromZone string, fromIndex int, fromZoneData *NavZone, direction int) *widget.Button {
	transitions := b.navTransitions[fromZone]
	if transitions == nil {
		return nil
	}

	transition := transitions[direction]
	if transition == nil {
		return nil
	}

	toZone := b.navZones[transition.ToZone]
	if toZone == nil || len(toZone.Keys) == 0 {
		return nil
	}

	// Calculate target index
	targetIndex := 0
	switch transition.ToIndex {
	case types.NavIndexFirst:
		targetIndex = 0
	case types.NavIndexLast:
		targetIndex = len(toZone.Keys) - 1
	case types.NavIndexPreserve:
		// Try to preserve column position when transitioning
		targetIndex = b.calculatePreservedIndex(fromIndex, fromZoneData, toZone, direction)
	default:
		if transition.ToIndex >= 0 && transition.ToIndex < len(toZone.Keys) {
			targetIndex = transition.ToIndex
		}
	}

	if targetIndex >= 0 && targetIndex < len(toZone.Keys) {
		return b.focusButtons[toZone.Keys[targetIndex]]
	}

	return nil
}

// calculatePreservedIndex calculates the best index in the target zone
// that preserves the user's position (e.g., column when going up/down)
func (b *BaseScreen) calculatePreservedIndex(fromIndex int, fromZone, toZone *NavZone, direction int) int {
	if fromZone.Type == types.NavZoneGrid && fromZone.Columns > 0 {
		// Preserve column when transitioning from grid
		col := fromIndex % fromZone.Columns

		if toZone.Type == types.NavZoneHorizontal {
			// Going to horizontal toolbar - try to match position proportionally
			if len(toZone.Keys) > 0 {
				ratio := float64(col) / float64(fromZone.Columns)
				return int(ratio*float64(len(toZone.Keys)-1) + 0.5)
			}
		} else if toZone.Type == types.NavZoneGrid && toZone.Columns > 0 {
			// Going to another grid - preserve column if possible
			if col < toZone.Columns {
				if direction == types.DirUp {
					// Going up: start at last row, same column
					lastRowStart := ((len(toZone.Keys) - 1) / toZone.Columns) * toZone.Columns
					targetIndex := lastRowStart + col
					if targetIndex >= len(toZone.Keys) {
						targetIndex = len(toZone.Keys) - 1
					}
					return targetIndex
				}
				// Going down: start at first row, same column
				return col
			}
		}
	}

	// Default: first or last based on direction
	if direction == types.DirUp || direction == types.DirLeft {
		return len(toZone.Keys) - 1
	}
	return 0
}

// findFocusSpatial is the legacy spatial navigation (fallback)
func (b *BaseScreen) findFocusSpatial(current widget.Focuser, direction int) *widget.Button {
	currentWidget := current.GetWidget()
	if currentWidget == nil {
		return nil
	}

	currentRect := currentWidget.Rect
	currentCX := (currentRect.Min.X + currentRect.Max.X) / 2
	currentCY := (currentRect.Min.Y + currentRect.Max.Y) / 2

	var bestBtn *widget.Button
	bestDist := int(^uint(0) >> 1)

	for _, btn := range b.focusButtons {
		btnWidget := btn.GetWidget()
		if btnWidget == currentWidget {
			continue
		}

		candidateRect := btnWidget.Rect
		candidateCX := (candidateRect.Min.X + candidateRect.Max.X) / 2
		candidateCY := (candidateRect.Min.Y + candidateRect.Max.Y) / 2

		dx := candidateCX - currentCX
		dy := candidateCY - currentCY

		inDirection := false
		switch direction {
		case types.DirUp:
			inDirection = candidateCY < currentCY
		case types.DirDown:
			inDirection = candidateCY > currentCY
		case types.DirLeft:
			inDirection = candidateCX < currentCX
		case types.DirRight:
			inDirection = candidateCX > currentCX
		}

		if !inDirection {
			continue
		}

		var dist int
		switch direction {
		case types.DirUp, types.DirDown:
			dist = dx*dx*4 + dy*dy
		case types.DirLeft, types.DirRight:
			dist = dx*dx + dy*dy*4
		}

		if dist < bestDist {
			bestDist = dist
			bestBtn = btn
		}
	}

	return bestBtn
}
