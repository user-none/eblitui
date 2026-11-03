// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"testing"

	"github.com/ebitenui/ebitenui/widget"
)

// countingScanner counts Cancel calls
type countingScanner struct{ cancels int }

func (c *countingScanner) Cancel() { c.cancels++ }

func visible(w widget.HasWidget) bool {
	return w.GetWidget().GetVisibility() == widget.Visibility_Show
}

func TestScanProgressUpdatesInPlace(t *testing.T) {
	s := NewScanProgressScreen()
	s.OnEnter()
	s.Build()
	status, count, download := s.statusLabel, s.countLabel, s.downloadLabel

	s.UpdateProgress(ScanProgress{Phase: 2, Progress: 0.5, CountText: "Found: 3 games", DownloadTotal: 10, DownloadComplete: 4, StatusText: "Downloading artwork..."})
	if s.statusLabel != status || s.countLabel != count || s.downloadLabel != download {
		t.Fatal("progress update replaced the widgets")
	}
	if status.Label != "Downloading artwork..." || count.Label != "Found: 3 games" || s.percentLabel.Label != "50%" {
		t.Errorf("labels = %q, %q, %q", status.Label, count.Label, s.percentLabel.Label)
	}
	if !visible(download) || download.Label != "Checking: 4/10" {
		t.Errorf("download label %q visible %v", download.Label, visible(download))
	}

	s.UpdateProgress(ScanProgress{Phase: 1, Progress: 0.1, StatusText: "Scanning for games..."})
	if visible(download) {
		t.Error("download label shown outside the download phases")
	}
}

func TestScanProgressCancelOnce(t *testing.T) {
	s := NewScanProgressScreen()
	sc := &countingScanner{}
	s.SetScanner(sc)
	s.OnEnter()
	s.Build()
	if !visible(s.cancelButton) || visible(s.cancelingText) {
		t.Fatal("before cancel: want the button, not Canceling...")
	}

	s.Cancel()
	s.Cancel()
	if sc.cancels != 1 {
		t.Errorf("scanner cancelled %d times, want 1", sc.cancels)
	}
	if visible(s.cancelButton) || !visible(s.cancelingText) {
		t.Error("after cancel: want Canceling..., not the button")
	}

	// A progress update after cancelling keeps Canceling... shown
	s.UpdateProgress(ScanProgress{Phase: 2, Progress: 0.9})
	if visible(s.cancelButton) {
		t.Error("progress update brought the button back")
	}

	// The next scan starts with the button again
	s.OnEnter()
	if !visible(s.cancelButton) || visible(s.cancelingText) {
		t.Error("after OnEnter: want the button again")
	}
}

func TestScanProgressCancelBeforeBuild(t *testing.T) {
	s := NewScanProgressScreen()
	sc := &countingScanner{}
	s.SetScanner(sc)
	s.Cancel()
	s.Build()
	if sc.cancels != 1 || visible(s.cancelButton) || !visible(s.cancelingText) {
		t.Errorf("cancel before Build: %d cancels, button %v", sc.cancels, visible(s.cancelButton))
	}

	// No scanner set: Cancel still switches the screen and does not panic
	s = NewScanProgressScreen()
	s.Build()
	s.Cancel()
	if !visible(s.cancelingText) {
		t.Error("no scanner: Canceling... not shown")
	}
}
