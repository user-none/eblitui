// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// sidebarZone is the navigation zone name of a section sidebar
const sidebarZone = "sidebar"

// sectionDescriptor describes one sidebar section of a screen
type sectionDescriptor struct {
	label    string
	focusKey string
	build    func(types.FocusManager) *widget.Container
	setupNav func() // optional; registers the section's zones and transitions
	disabled bool   // shown but not selectable or focusable
}

// sidebarSections manages a list of sections selected from a sidebar. The
// owning screen builds the sidebar and the selected section's content, and
// rebuilds when the selection changes.
type sidebarSections struct {
	sections []sectionDescriptor
	selected int
}

// labels returns the section labels in sidebar order
func (ss *sidebarSections) labels() []string {
	out := make([]string, len(ss.sections))
	for i, sec := range ss.sections {
		out[i] = sec.label
	}
	return out
}

// sidebarWidth returns the sidebar width: minWidth, or wider when a section
// label needs more room
func (ss *sidebarSections) sidebarWidth(minWidth int) int {
	return max(minWidth, widgets.SidebarFitWidth(ss.labels()))
}

// current returns the selected section, or false when none is valid
func (ss *sidebarSections) current() (sectionDescriptor, bool) {
	if ss.selected < 0 || ss.selected >= len(ss.sections) {
		return sectionDescriptor{}, false
	}
	return ss.sections[ss.selected], true
}

// currentKey returns the focus key of the selected section, or "" when none
func (ss *sidebarSections) currentKey() string {
	sec, ok := ss.current()
	if !ok {
		return ""
	}
	return sec.focusKey
}

// selectKey selects the section with the given focus key. Returns false and
// leaves the selection unchanged for unknown or disabled keys.
func (ss *sidebarSections) selectKey(key string) bool {
	for i, sec := range ss.sections {
		if sec.focusKey == key && !sec.disabled {
			ss.selected = i
			return true
		}
	}
	return false
}

// buildSidebar creates the sidebar with one button per enabled section and
// registers the buttons for focus. Disabled sections are shown as disabled
// items. Selecting a section focuses its button and calls rebuild.
func (ss *sidebarSections) buildSidebar(b *BaseScreen, minWidth int, rebuild func()) *widget.Container {
	sidebar := widgets.NewSidebar(minWidth)
	for i, sec := range ss.sections {
		if sec.disabled {
			sidebar.AddDisabledItem(sec.label)
			continue
		}
		idx := i
		key := sec.focusKey
		btn := sidebar.AddButton(sec.label, ss.selected == idx, func() {
			ss.selected = idx
			b.SetPendingFocus(key)
			rebuild()
		})
		b.RegisterFocusButton(key, btn)
	}
	return sidebar.Container
}

// buildContent builds the selected section's content, or nil when none is
// selected
func (ss *sidebarSections) buildContent(fm types.FocusManager) *widget.Container {
	sec, ok := ss.current()
	if !ok {
		return nil
	}
	return sec.build(fm)
}

// setupNav registers the sidebar navigation zone over the enabled sections
// and the selected section's navigation
func (ss *sidebarSections) setupNav(b *BaseScreen) {
	keys := make([]string, 0, len(ss.sections))
	for _, sec := range ss.sections {
		if !sec.disabled {
			keys = append(keys, sec.focusKey)
		}
	}
	b.RegisterNavZone(sidebarZone, types.NavZoneVertical, keys, 0)

	if sec, ok := ss.current(); ok && sec.setupNav != nil {
		sec.setupNav()
	}
}
