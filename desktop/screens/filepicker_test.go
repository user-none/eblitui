// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/user-none/eblitui/desktop/filebrowser"
	"github.com/user-none/eblitui/desktop/types"
)

// pickerCallback records notifications. Other ScreenCallback methods are not
// used by the picker's state logic.
type pickerCallback struct {
	ScreenCallback
	notifications []string
}

func (c *pickerCallback) ShowNotification(msg string) {
	c.notifications = append(c.notifications, msg)
}

type pickerHarness struct {
	picker   *FilePickerScreen
	cb       *pickerCallback
	events   []string
	selected string
	home     string
}

// newPickerHarness creates a picker opened in a temporary home directory
// containing roms/genesis, roms/sms, bios/scph.bin and notes.txt.
func newPickerHarness(t *testing.T, mode filebrowser.Mode) *pickerHarness {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, d := range []string{"roms/genesis", "roms/sms", "bios"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"bios/scph.bin", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(home, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	h := &pickerHarness{cb: &pickerCallback{}, home: home}
	h.picker = NewFilePickerScreen(h.cb, func() { h.events = append(h.events, "close") })
	h.picker.Open(types.FilePickerRequest{
		Title: "Test",
		Mode:  mode,
		OnSelect: func(path string) {
			h.events = append(h.events, "select")
			h.selected = path
		},
	})
	return h
}

func entryNames(p *FilePickerScreen) []string {
	shown := filebrowser.Filter(p.entries, p.filter)
	out := make([]string, len(shown))
	for i, e := range shown {
		out[i] = e.Name
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFilePickerOpenStartsAtHome(t *testing.T) {
	h := newPickerHarness(t, filebrowser.ModeFolder)
	p := h.picker

	if p.dir != filepath.Clean(h.home) {
		t.Errorf("dir = %q, want %q", p.dir, h.home)
	}
	if got, want := p.pathTextValue, filebrowser.DirText(h.home); got != want {
		t.Errorf("path text = %q, want %q", got, want)
	}
	if got := entryNames(p); !sameStrings(got, []string{"bios", "roms"}) {
		t.Errorf("entries = %v, want [bios roms]", got)
	}
	if p.pendingFocus != pickerUpKey {
		t.Errorf("pendingFocus = %q, want %q", p.pendingFocus, pickerUpKey)
	}
}

func TestFilePickerEnterDir(t *testing.T) {
	h := newPickerHarness(t, filebrowser.ModeFolder)
	p := h.picker
	p.filter = "x"
	roms := filepath.Join(h.home, "roms")

	p.enterDir(roms)

	if p.dir != roms {
		t.Errorf("dir = %q, want %q", p.dir, roms)
	}
	if p.filter != "" {
		t.Errorf("filter = %q, want empty", p.filter)
	}
	if got, want := p.pathTextValue, filebrowser.DirText(roms); got != want {
		t.Errorf("path text = %q, want %q", got, want)
	}
	if got := entryNames(p); !sameStrings(got, []string{"genesis", "sms"}) {
		t.Errorf("entries = %v, want [genesis sms]", got)
	}
}

func TestFilePickerEnterDirUnreadable(t *testing.T) {
	h := newPickerHarness(t, filebrowser.ModeFolder)
	p := h.picker
	before := p.pathTextValue

	p.enterDir(filepath.Join(h.home, "missing"))

	if p.dir != filepath.Clean(h.home) {
		t.Errorf("dir changed to %q", p.dir)
	}
	if p.pathTextValue != before {
		t.Errorf("path text changed to %q", p.pathTextValue)
	}
	if len(h.cb.notifications) != 1 || h.cb.notifications[0] != "Cannot open folder" {
		t.Errorf("notifications = %v", h.cb.notifications)
	}
}

func TestFilePickerPathChanged(t *testing.T) {
	sep := string(filepath.Separator)

	t.Run("filter only", func(t *testing.T) {
		h := newPickerHarness(t, filebrowser.ModeFolder)
		p := h.picker
		p.onPathChanged(filebrowser.DirText(h.home) + "RO")
		if p.dir != filepath.Clean(h.home) || p.filter != "RO" || p.dirError != "" {
			t.Errorf("dir=%q filter=%q dirError=%q", p.dir, p.filter, p.dirError)
		}
		if got := entryNames(p); !sameStrings(got, []string{"roms"}) {
			t.Errorf("entries = %v, want [roms]", got)
		}
	})

	t.Run("existing folder", func(t *testing.T) {
		h := newPickerHarness(t, filebrowser.ModeFolder)
		p := h.picker
		roms := filepath.Join(h.home, "roms")
		p.onPathChanged(roms + sep + "ge")
		if p.dir != roms || p.filter != "ge" || p.dirError != "" {
			t.Errorf("dir=%q filter=%q dirError=%q", p.dir, p.filter, p.dirError)
		}
	})

	t.Run("missing folder", func(t *testing.T) {
		h := newPickerHarness(t, filebrowser.ModeFolder)
		p := h.picker
		p.onPathChanged(filepath.Join(h.home, "missing") + sep)
		if p.dirError == "" {
			t.Error("dirError should be set")
		}
		if p.dir != filepath.Clean(h.home) {
			t.Errorf("dir changed to %q", p.dir)
		}
		p.onPathChanged(filebrowser.DirText(h.home))
		if p.dirError != "" {
			t.Errorf("dirError = %q after returning to listed folder", p.dirError)
		}
	})

	t.Run("relative folder", func(t *testing.T) {
		h := newPickerHarness(t, filebrowser.ModeFolder)
		p := h.picker
		p.onPathChanged("roms" + sep)
		if p.dirError == "" {
			t.Error("dirError should be set for relative path")
		}
	})

	t.Run("clears selection", func(t *testing.T) {
		h := newPickerHarness(t, filebrowser.ModeFile)
		p := h.picker
		p.selectFile(filepath.Join(h.home, "notes.txt"))
		p.onPathChanged(p.pathTextValue + "x")
		if p.selected != "" {
			t.Errorf("selected = %q, want empty", p.selected)
		}
	})

	t.Run("ignored while setting text", func(t *testing.T) {
		h := newPickerHarness(t, filebrowser.ModeFolder)
		p := h.picker
		p.settingText = true
		p.onPathChanged("zzz")
		if p.filter != "" {
			t.Errorf("filter = %q, want empty", p.filter)
		}
	})
}

func TestFilePickerSelectFileKeepsFilter(t *testing.T) {
	h := newPickerHarness(t, filebrowser.ModeFile)
	p := h.picker
	p.onPathChanged(filebrowser.DirText(h.home) + "no")
	notes := filepath.Join(h.home, "notes.txt")

	p.selectFile(notes)

	if p.selected != notes || p.pathTextValue != notes {
		t.Errorf("selected=%q path=%q, want %q", p.selected, p.pathTextValue, notes)
	}
	if p.filter != "no" {
		t.Errorf("filter = %q, want no", p.filter)
	}
}

func TestFilePickerPathSubmit(t *testing.T) {
	t.Run("folder", func(t *testing.T) {
		h := newPickerHarness(t, filebrowser.ModeFolder)
		p := h.picker
		roms := filepath.Join(h.home, "roms")
		p.onPathSubmit(roms)
		if p.dir != roms || p.pathTextValue != filebrowser.DirText(roms) {
			t.Errorf("dir=%q path=%q", p.dir, p.pathTextValue)
		}
		if len(h.events) != 0 {
			t.Errorf("submit should not accept, events = %v", h.events)
		}
	})

	t.Run("file in file mode", func(t *testing.T) {
		h := newPickerHarness(t, filebrowser.ModeFile)
		p := h.picker
		bin := filepath.Join(h.home, "bios", "scph.bin")
		p.onPathSubmit(bin)
		if p.dir != filepath.Dir(bin) || p.selected != bin || p.filter != "" {
			t.Errorf("dir=%q selected=%q filter=%q", p.dir, p.selected, p.filter)
		}
		if len(h.events) != 0 {
			t.Errorf("submit should not accept, events = %v", h.events)
		}
	})

	t.Run("file in folder mode", func(t *testing.T) {
		h := newPickerHarness(t, filebrowser.ModeFolder)
		p := h.picker
		p.onPathSubmit(filepath.Join(h.home, "notes.txt"))
		if p.dir != filepath.Clean(h.home) || p.selected != "" {
			t.Errorf("dir=%q selected=%q", p.dir, p.selected)
		}
	})
}

func TestFilePickerCanAccept(t *testing.T) {
	sep := string(filepath.Separator)

	t.Run("folder mode", func(t *testing.T) {
		h := newPickerHarness(t, filebrowser.ModeFolder)
		p := h.picker
		roms := filepath.Join(h.home, "roms")
		if !p.canAccept() {
			t.Error("listed folder with empty filter should be acceptable")
		}
		p.onPathChanged(roms)
		if p.canAccept() {
			t.Error("folder without trailing separator is a filter and should not be acceptable")
		}
		p.onPathChanged(roms + sep)
		if !p.canAccept() {
			t.Error("folder with trailing separator should be acceptable")
		}
		p.onPathChanged(roms + sep + "gen")
		if p.canAccept() {
			t.Error("filter text should not be acceptable")
		}
		p.onPathChanged(filepath.Join(h.home, "missing") + sep)
		if p.canAccept() {
			t.Error("missing folder should not be acceptable")
		}
		p.onPathChanged("")
		if p.canAccept() {
			t.Error("empty field should not be acceptable")
		}
	})

	t.Run("file mode", func(t *testing.T) {
		h := newPickerHarness(t, filebrowser.ModeFile)
		p := h.picker
		if p.canAccept() {
			t.Error("nothing selected should not be acceptable")
		}
		p.selectFile(filepath.Join(h.home, "notes.txt"))
		if !p.canAccept() {
			t.Error("selected file should be acceptable")
		}
	})
}

func TestFilePickerAccept(t *testing.T) {
	t.Run("folder", func(t *testing.T) {
		h := newPickerHarness(t, filebrowser.ModeFolder)
		roms := filepath.Join(h.home, "roms")
		h.picker.enterDir(roms)
		h.picker.accept()
		if !sameStrings(h.events, []string{"close", "select"}) {
			t.Errorf("events = %v, want [close select]", h.events)
		}
		if h.selected != roms {
			t.Errorf("selected = %q, want %q", h.selected, roms)
		}
	})

	t.Run("file", func(t *testing.T) {
		h := newPickerHarness(t, filebrowser.ModeFile)
		notes := filepath.Join(h.home, "notes.txt")
		h.picker.selectFile(notes)
		h.picker.accept()
		if !sameStrings(h.events, []string{"close", "select"}) || h.selected != notes {
			t.Errorf("events = %v selected = %q", h.events, h.selected)
		}
	})

	t.Run("not acceptable", func(t *testing.T) {
		h := newPickerHarness(t, filebrowser.ModeFile)
		h.picker.accept()
		if len(h.events) != 0 {
			t.Errorf("events = %v, want none", h.events)
		}
	})

	t.Run("selection removed", func(t *testing.T) {
		h := newPickerHarness(t, filebrowser.ModeFile)
		notes := filepath.Join(h.home, "notes.txt")
		h.picker.selectFile(notes)
		if err := os.Remove(notes); err != nil {
			t.Fatal(err)
		}
		h.picker.accept()
		if len(h.events) != 0 {
			t.Errorf("events = %v, want none", h.events)
		}
		if len(h.cb.notifications) != 1 {
			t.Errorf("notifications = %v", h.cb.notifications)
		}
	})
}

func TestFilePickerCancel(t *testing.T) {
	h := newPickerHarness(t, filebrowser.ModeFolder)
	h.picker.Cancel()
	if !sameStrings(h.events, []string{"close"}) {
		t.Errorf("events = %v, want [close]", h.events)
	}
}

func TestFilePickerFirstListKey(t *testing.T) {
	h := newPickerHarness(t, filebrowser.ModeFolder)
	p := h.picker

	root := filepath.VolumeName(h.home) + string(filepath.Separator)
	p.enterDir(root)
	if got := p.firstListKey(); got == pickerUpKey {
		t.Error("root should not offer Up one level")
	}

	p.enterDir(filepath.Join(h.home, "roms", "sms"))
	if got := p.firstListKey(); got != pickerUpKey {
		t.Errorf("firstListKey = %q, want %q", got, pickerUpKey)
	}

	p.dirError = "Folder not found"
	if got := p.firstListKey(); got != pickerCancelKey {
		t.Errorf("firstListKey with error = %q, want %q", got, pickerCancelKey)
	}
}

func TestFilePickerBuildRegistersNavigation(t *testing.T) {
	h := newPickerHarness(t, filebrowser.ModeFolder)
	p := h.picker

	if p.Build() == nil {
		t.Fatal("Build returned nil")
	}

	list := p.navZones["picker-list"]
	if list == nil || len(list.Keys) == 0 || list.Keys[0] != pickerUpKey {
		t.Fatalf("picker-list zone = %+v", list)
	}
	if got, want := len(list.Keys), 3; got != want {
		t.Errorf("list keys = %v, want %d", list.Keys, want)
	}
	if places := p.navZones["picker-places"]; places == nil || len(places.Keys) == 0 {
		t.Errorf("picker-places zone = %+v", places)
	}
	buttons := p.navZones["picker-buttons"]
	if buttons == nil || !sameStrings(buttons.Keys, []string{pickerAcceptKey, pickerCancelKey}) {
		t.Errorf("picker-buttons zone = %+v", buttons)
	}
	if p.GetPendingFocus() == nil {
		t.Error("pending focus button should resolve after Build")
	}
	if p.acceptBtn.GetWidget().Disabled {
		t.Error("Accept should be enabled for the listed folder")
	}

	p.onPathChanged(p.pathTextValue + "zzz")
	if !p.acceptBtn.GetWidget().Disabled {
		t.Error("Accept should be disabled while filtering")
	}
	if got := len(p.navZones["picker-list"].Keys); got != 1 {
		t.Errorf("list keys after filter = %d, want 1 (Up one level)", got)
	}
}
