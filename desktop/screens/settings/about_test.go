// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"testing"
	"time"

	"github.com/user-none/eblitui/desktop/types"
)

func TestUpdateStatusText(t *testing.T) {
	tests := []struct {
		name   string
		status types.UpdateStatus
		want   string
	}{
		{"never checked", types.UpdateStatus{}, ""},
		{"up to date", types.UpdateStatus{LastCheck: 1}, "Up to date"},
		{"available", types.UpdateStatus{Available: "0.5.0", LastCheck: 1}, "Update available: 0.5.0"},
		{"failed", types.UpdateStatus{Failed: true, LastCheck: 1}, "Couldn't check for updates"},
		{"failed with a stored update", types.UpdateStatus{Failed: true, Available: "0.5.0", LastCheck: 1}, "Update available: 0.5.0"},
		{"checking", types.UpdateStatus{Checking: true}, "Checking for updates..."},
		{"checking with a stored update", types.UpdateStatus{Checking: true, Available: "0.5.0"}, "Checking for updates..."},
	}
	for _, tc := range tests {
		if got := updateStatusText(tc.status); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestLastCheckedText(t *testing.T) {
	now := time.Unix(1760000000, 0)
	ago := func(d time.Duration) int64 { return now.Add(-d).Unix() }
	tests := []struct {
		lastCheck int64
		want      string
	}{
		{0, "Last checked: never"},
		{ago(10 * time.Second), "Last checked just now"},
		{ago(time.Minute), "Last checked 1 minute ago"},
		{ago(59 * time.Minute), "Last checked 59 minutes ago"},
		{ago(time.Hour), "Last checked 1 hour ago"},
		{ago(2*time.Hour + 30*time.Minute), "Last checked 2 hours ago"},
		{ago(24 * time.Hour), "Last checked 1 day ago"},
		{ago(10 * 24 * time.Hour), "Last checked 10 days ago"},
		{now.Add(time.Hour).Unix(), "Last checked just now"},
	}
	for _, tc := range tests {
		if got := lastCheckedText(tc.lastCheck, now); got != tc.want {
			t.Errorf("lastCheckedText(%d) = %q, want %q", tc.lastCheck, got, tc.want)
		}
	}
}
