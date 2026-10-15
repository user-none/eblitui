// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"slices"
	"testing"

	"github.com/ebitenui/ebitenui/event"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/types"
)

func newTestSections(navCalls *int) *sidebarSections {
	build := func(types.FocusManager) *widget.Container { return widget.NewContainer() }
	return &sidebarSections{
		sections: []sectionDescriptor{
			{label: "One", focusKey: "sec-one", build: build, setupNav: func() { *navCalls++ }},
			{label: "Two", focusKey: "sec-two", build: build},
		},
	}
}

func TestSidebarSectionsLabels(t *testing.T) {
	var navCalls int
	ss := newTestSections(&navCalls)
	if got := ss.labels(); !slices.Equal(got, []string{"One", "Two"}) {
		t.Errorf("labels = %v, want [One Two]", got)
	}
}

func TestSidebarSectionsSelectKey(t *testing.T) {
	var navCalls int
	ss := newTestSections(&navCalls)

	if !ss.selectKey("sec-two") {
		t.Fatal("selectKey(sec-two) = false, want true")
	}
	if got := ss.currentKey(); got != "sec-two" {
		t.Errorf("currentKey = %q, want sec-two", got)
	}

	if ss.selectKey("sec-missing") {
		t.Error("selectKey(sec-missing) = true, want false")
	}
	if ss.selected != 1 {
		t.Errorf("selected = %d after unknown key, want 1", ss.selected)
	}
}

func TestSidebarSectionsInvalidSelection(t *testing.T) {
	var navCalls int
	ss := newTestSections(&navCalls)
	ss.selected = 5

	if got := ss.currentKey(); got != "" {
		t.Errorf("currentKey = %q, want empty", got)
	}
	if ss.buildContent(nil) != nil {
		t.Error("buildContent returned content for an invalid selection")
	}
}

func TestSidebarSectionsBuildSidebarSelects(t *testing.T) {
	var navCalls int
	ss := newTestSections(&navCalls)
	var b BaseScreen
	b.InitBase()
	rebuilds := 0

	ss.buildSidebar(&b, 0, func() { rebuilds++ })

	btn, _ := b.target("sec-two").(*widget.Button)
	if btn == nil {
		t.Fatal("sec-two button not registered")
	}
	btn.Click()
	event.ExecuteDeferred()

	if ss.selected != 1 {
		t.Errorf("selected = %d, want 1", ss.selected)
	}
	if b.pendingFocus != "sec-two" {
		t.Errorf("pendingFocus = %q, want sec-two", b.pendingFocus)
	}
	if rebuilds != 1 {
		t.Errorf("rebuilds = %d, want 1", rebuilds)
	}
}

func TestSidebarSectionsSetupNav(t *testing.T) {
	var navCalls int
	ss := newTestSections(&navCalls)
	var b BaseScreen
	b.InitBase()

	ss.setupNav(&b)
	zone := b.navZones[sidebarZone]
	if zone == nil {
		t.Fatal("sidebar zone not registered")
	}
	if navCalls != 1 {
		t.Errorf("section setupNav calls = %d, want 1", navCalls)
	}

	// A section without setupNav is skipped
	ss.selected = 1
	ss.setupNav(&b)
	if navCalls != 1 {
		t.Errorf("section setupNav calls = %d, want 1", navCalls)
	}
}

func TestSidebarSectionsDisabled(t *testing.T) {
	var navCalls int
	ss := newTestSections(&navCalls)
	ss.sections[1].disabled = true
	var b BaseScreen
	b.InitBase()

	ss.buildSidebar(&b, 0, func() {})
	if b.target("sec-two") != nil {
		t.Error("disabled section registered for focus")
	}

	ss.setupNav(&b)
	if keys := b.navZones[sidebarZone].Keys; !slices.Equal(keys, []string{"sec-one"}) {
		t.Errorf("sidebar zone keys = %v, want [sec-one]", keys)
	}

	if ss.selectKey("sec-two") {
		t.Error("selectKey selected a disabled section")
	}
	if ss.selected != 0 {
		t.Errorf("selected = %d, want 0", ss.selected)
	}
}
