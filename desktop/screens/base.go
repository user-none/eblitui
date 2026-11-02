// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
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
	// Scroll container for scroll position preservation (owns its slider)
	scrollContainer *widgets.ScrollView
	scrollTop       float64

	// Focus targets for navigation and focus restoration (maps key to
	// widget): buttons, and text fields such as widgets.MultiLineInput
	focusTargets map[string]widget.Focuser

	// Key of the target to restore focus to after rebuild
	pendingFocus string

	// Zone-based navigation
	navZones       map[string]*NavZone               // Zone name -> zone definition
	navTransitions map[string]map[int]*NavTransition // Zone name -> direction -> transition
	buttonToZone   map[string]string                 // Button key -> zone name

	// Scroll bars registered as navigation targets, and the key of the
	// target the last one was entered from, which Left returns to
	scrollBars   []scrollBar
	scrollReturn string
}

// scrollBar is a scroll view's slider registered as a navigation target
type scrollBar struct {
	view *widgets.ScrollView
	bar  *widget.Slider
}

// InitBase initializes the base screen state.
// Call this in the screen's constructor.
func (b *BaseScreen) InitBase() {
	b.focusTargets = make(map[string]widget.Focuser)
	b.navZones = make(map[string]*NavZone)
	b.navTransitions = make(map[string]map[int]*NavTransition)
	b.buttonToZone = make(map[string]string)
}

// SetScrollContainer stores the scroll container for position preservation.
// Call this during Build() after creating the scroll container.
func (b *BaseScreen) SetScrollContainer(scrollContainer *widgets.ScrollView) {
	b.scrollContainer = scrollContainer
}

// SaveScrollPosition saves the current scroll position.
// Call this before rebuilding the screen.
func (b *BaseScreen) SaveScrollPosition() {
	if b.scrollContainer != nil {
		b.scrollTop = b.scrollContainer.ScrollTop
	}
}

// resetScroll starts the next build at the top. The scroll container is
// dropped so the save before that rebuild keeps nothing of its offset.
func (b *BaseScreen) resetScroll() {
	b.scrollContainer = nil
	b.scrollTop = 0
}

// RestoreScrollPosition restores the saved scroll position.
// Call this after rebuilding the screen, once the scroll container is set.
func (b *BaseScreen) RestoreScrollPosition() {
	if b.scrollContainer != nil && b.scrollTop > 0 {
		b.scrollContainer.SetScrollTop(b.scrollTop)
	}
}

// RegisterFocusButton registers a button for focus restoration.
// Call this during Build() for each focusable button.
func (b *BaseScreen) RegisterFocusButton(key string, btn *widget.Button) {
	b.RegisterFocusWidget(key, btn)
}

// RegisterFocusWidget registers any focusable widget, such as a text
// field, as a navigation and focus restoration target. Call this during
// Build() like RegisterFocusButton.
func (b *BaseScreen) RegisterFocusWidget(key string, f widget.Focuser) {
	if b.focusTargets == nil {
		b.focusTargets = make(map[string]widget.Focuser)
	}
	b.focusTargets[key] = f
}

// SaveFocusState checks which registered focus target currently has focus
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
	for key, f := range b.focusTargets {
		if f.GetWidget() == focusedWidget {
			b.pendingFocus = key
			return
		}
	}
}

// ClearFocusButtons clears all registered focus targets and navigation zones.
// Call this at the start of Build() before registering new targets.
func (b *BaseScreen) ClearFocusButtons() {
	b.focusTargets = make(map[string]widget.Focuser)
	b.navZones = make(map[string]*NavZone)
	b.navTransitions = make(map[string]map[int]*NavTransition)
	b.buttonToZone = make(map[string]string)
	b.scrollBars = nil
}

// RegisterScrollBar registers a scroll view's slider as a navigation
// target under key. Right at the right edge of a zone inside the view
// moves to it, as does Right from the sidebar when the page has nowhere
// else to go. On it, Up and Down scroll the view (see ScrollBy). Left
// returns to the target it was entered from, and every other way out goes
// to the nearest target in that direction outside the view.
func (b *BaseScreen) RegisterScrollBar(key string, view *widgets.ScrollView, bar *widget.Slider) {
	b.RegisterFocusWidget(key, bar)
	b.scrollBars = append(b.scrollBars, scrollBar{view: view, bar: bar})
}

// ScrollBy scrolls the view of the scroll bar focused one step for Up or
// Down. It reports false, leaving the move to navigation, when focused is
// not a scroll bar, for any other direction, and at the end of the view.
func (b *BaseScreen) ScrollBy(focused widget.Focuser, direction int) bool {
	sb := b.scrollBarOf(focused)
	if sb == nil {
		return false
	}
	switch direction {
	case types.DirUp:
		return sb.view.ScrollStep(-1)
	case types.DirDown:
		return sb.view.ScrollStep(1)
	}
	return false
}

// scrollBarOf returns the registered scroll bar f is, or nil
func (b *BaseScreen) scrollBarOf(f widget.Focuser) *scrollBar {
	if f == nil {
		return nil
	}
	for i := range b.scrollBars {
		if b.scrollBars[i].bar.GetWidget() == f.GetWidget() {
			return &b.scrollBars[i]
		}
	}
	return nil
}

// scrollBarContaining returns the registered scroll bar whose view holds
// w, or nil
func (b *BaseScreen) scrollBarContaining(w *widget.Widget) *scrollBar {
	for i := range b.scrollBars {
		if b.scrollBars[i].view.Contains(w) {
			return &b.scrollBars[i]
		}
	}
	return nil
}

// pageScrollBar returns the registered scroll bar of the page's scroll
// container, or nil
func (b *BaseScreen) pageScrollBar() *scrollBar {
	for i := range b.scrollBars {
		if b.scrollBars[i].view == b.scrollContainer {
			return &b.scrollBars[i]
		}
	}
	return nil
}

// leaveScrollBar returns where a move off scroll bar sb goes. Left goes
// into the view (see enterScrollView), or back to where the bar was
// entered from when nothing in the view is visible. Every other move goes
// to the nearest target outside the view that lies wholly past the bar's
// edge in the direction. The edge, not the center, decides, since the bar
// runs the height of the view.
func (b *BaseScreen) leaveScrollBar(sb *scrollBar, direction int) widget.Focuser {
	if direction == types.DirLeft {
		if t := b.enterScrollView(sb); t != nil {
			return t
		}
		if t := b.target(b.scrollReturn); t != nil && t.GetWidget() != sb.bar.GetWidget() {
			return t
		}
	}
	edge := sb.bar.GetWidget().Rect
	return b.findFocusSpatialExcept(sb.bar, direction, func(f widget.Focuser) bool {
		if sb.view.Contains(f.GetWidget()) {
			return true
		}
		r := f.GetWidget().Rect
		switch direction {
		case types.DirUp:
			return r.Max.Y > edge.Min.Y
		case types.DirDown:
			return r.Min.Y < edge.Max.Y
		case types.DirLeft:
			return r.Max.X > edge.Min.X
		case types.DirRight:
			return r.Min.X < edge.Max.X
		}
		return true
	})
}

// enterScrollView returns the target inside sb's view, at least partly
// visible, nearest the point on the view's height matching the scroll
// position: its top when scrolled to the top, its bottom when scrolled to
// the bottom. Of targets at the same height, the one nearest the bar wins.
// It returns nil when no target in the view is visible.
func (b *BaseScreen) enterScrollView(sb *scrollBar) widget.Focuser {
	view := sb.view.ViewRect()
	y := view.Min.Y + int(sb.view.ScrollTop*float64(view.Dy()))
	x := sb.bar.GetWidget().Rect.Min.X

	var best widget.Focuser
	bestDist := int(^uint(0) >> 1)
	for _, t := range b.focusTargets {
		w := t.GetWidget()
		if w == sb.bar.GetWidget() || !sb.view.Contains(w) || !w.Rect.Overlaps(view) {
			continue
		}
		dx := x - (w.Rect.Min.X+w.Rect.Max.X)/2
		dy := y - (w.Rect.Min.Y+w.Rect.Max.Y)/2
		if dist := dx*dx + dy*dy*4; dist < bestDist {
			bestDist = dist
			best = t
		}
	}
	return best
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

// GetPendingFocus returns the target that should receive focus after
// rebuild. Returns nil if no pending focus or target not found.
func (b *BaseScreen) GetPendingFocus() widget.Focuser {
	if b.pendingFocus == "" {
		return nil
	}
	return b.focusTargets[b.pendingFocus]
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

	focusWidget := focused.GetWidget()
	if focusWidget == nil {
		return
	}
	b.scrollContainer.ScrollRectIntoView(focusWidget.Rect)
}

// FindFocusInDirection finds the next target in the specified direction
// using zone-based navigation. Falls back to spatial navigation if no zones defined.
// Returns nil if no suitable target found.
func (b *BaseScreen) FindFocusInDirection(current widget.Focuser, direction int) widget.Focuser {
	if current == nil || len(b.focusTargets) == 0 {
		return nil
	}

	// Find current target's key
	currentKey := ""
	for key, f := range b.focusTargets {
		if f.GetWidget() == current.GetWidget() {
			currentKey = key
			break
		}
	}

	if currentKey == "" {
		return nil
	}

	if sb := b.scrollBarOf(current); sb != nil {
		return b.leaveScrollBar(sb, direction)
	}

	// If zones are defined, use zone-based navigation
	var next widget.Focuser
	if len(b.navZones) > 0 {
		next = b.findFocusInZone(currentKey, direction)
	} else {
		// Fallback to spatial navigation (legacy behavior)
		next = b.findFocusSpatial(current, direction)
	}
	if b.scrollBarOf(next) != nil {
		b.scrollReturn = currentKey
	}
	return next
}

// FindFocusLinear moves focus to the next or previous target in the current
// target's navigation zone, without wrapping. forward advances to the next
// target; otherwise the previous. It is used as the fallback when
// directional zone navigation has no target (e.g. moving past the right edge
// of a grid row advances to the next cell), replacing ebitenui's
// ChangeFocus, which cannot see content inside the custom scroll view.
// Returns nil at the ends of the zone, when there is no current target, or
// when it is not in a zone.
func (b *BaseScreen) FindFocusLinear(current widget.Focuser, forward bool) widget.Focuser {
	if current == nil {
		return nil
	}

	currentKey := ""
	for key, f := range b.focusTargets {
		if f.GetWidget() == current.GetWidget() {
			currentKey = key
			break
		}
	}
	if currentKey == "" {
		return nil
	}

	zone := b.navZones[b.buttonToZone[currentKey]]
	if zone == nil || len(zone.Keys) == 0 {
		return nil
	}

	idx := -1
	for i, k := range zone.Keys {
		if k == currentKey {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil
	}

	if forward {
		idx++
	} else {
		idx--
	}
	if idx < 0 || idx >= len(zone.Keys) {
		return nil
	}
	return b.target(zone.Keys[idx])
}

// IsFocusTarget reports whether f is one of the screen's registered focus
// targets
func (b *BaseScreen) IsFocusTarget(f widget.Focuser) bool {
	if f == nil {
		return false
	}
	for _, t := range b.focusTargets {
		if t.GetWidget() == f.GetWidget() {
			return true
		}
	}
	return false
}

// target returns the focus target registered under key, or nil. A
// missing key must give an untyped nil, not a nil map value boxed in the
// interface.
func (b *BaseScreen) target(key string) widget.Focuser {
	if f, ok := b.focusTargets[key]; ok && f != nil {
		return f
	}
	return nil
}

// findFocusInZone implements zone-aware navigation
func (b *BaseScreen) findFocusInZone(currentKey string, direction int) widget.Focuser {
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

	// If we should transition to another zone. Right at the right edge of
	// a zone inside a scroll view goes to its scroll bar, and Right from
	// the sidebar with nowhere else to go goes to the page's scroll bar.
	if shouldTransition {
		if direction == types.DirRight {
			if sb := b.scrollBarContaining(b.focusTargets[currentKey].GetWidget()); sb != nil {
				return sb.bar
			}
		}
		next := b.handleZoneTransition(zoneName, currentIndex, zone, direction)
		if next == nil && direction == types.DirRight && zoneName == sidebarZone {
			if sb := b.pageScrollBar(); sb != nil {
				return sb.bar
			}
		}
		return next
	}

	// Stay within zone
	if targetIndex >= 0 && targetIndex < len(zone.Keys) {
		return b.target(zone.Keys[targetIndex])
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
func (b *BaseScreen) handleZoneTransition(fromZone string, fromIndex int, fromZoneData *NavZone, direction int) widget.Focuser {
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
		return b.target(toZone.Keys[targetIndex])
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
func (b *BaseScreen) findFocusSpatial(current widget.Focuser, direction int) widget.Focuser {
	return b.findFocusSpatialExcept(current, direction, nil)
}

// findFocusSpatialExcept is findFocusSpatial over the targets skip does not
// exclude. A nil skip excludes none.
func (b *BaseScreen) findFocusSpatialExcept(current widget.Focuser, direction int, skip func(widget.Focuser) bool) widget.Focuser {
	currentWidget := current.GetWidget()
	if currentWidget == nil {
		return nil
	}

	currentRect := currentWidget.Rect
	currentCX := (currentRect.Min.X + currentRect.Max.X) / 2
	currentCY := (currentRect.Min.Y + currentRect.Max.Y) / 2

	var bestBtn widget.Focuser
	bestDist := int(^uint(0) >> 1)

	for _, btn := range b.focusTargets {
		btnWidget := btn.GetWidget()
		if btnWidget == currentWidget || (skip != nil && skip(btn)) {
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
