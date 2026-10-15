// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"testing"

	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/storage"
)

func newTestSettingsScreen() *SettingsScreen {
	return NewSettingsScreen(nil, storage.DefaultLibrary(), storage.DefaultConfig(), nil, 0, coreif.SystemInfo{}, nil)
}

func TestSettingsSelectSection(t *testing.T) {
	s := newTestSettingsScreen()

	s.SelectSection(SettingsSectionLibrary)

	if got := s.sections[s.selected].focusKey; got != SettingsSectionLibrary {
		t.Errorf("selected section = %q, want %q", got, SettingsSectionLibrary)
	}
	if s.pendingFocus != SettingsSectionLibrary {
		t.Errorf("pendingFocus = %q, want %q", s.pendingFocus, SettingsSectionLibrary)
	}
}

func TestSettingsSelectSectionUnknown(t *testing.T) {
	s := newTestSettingsScreen()
	s.selected = 2
	s.SetPendingFocus("section-input")

	s.SelectSection("section-missing")

	if s.selected != 2 {
		t.Errorf("selected = %d, want 2", s.selected)
	}
	if s.pendingFocus != "section-input" {
		t.Errorf("pendingFocus = %q, want 'section-input'", s.pendingFocus)
	}
}
