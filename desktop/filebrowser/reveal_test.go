// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package filebrowser

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestRevealCommand(t *testing.T) {
	path := filepath.Join("data", "screenshots", "ABCD1234", "1760540000123.png")
	tests := []struct {
		goos     string
		wantName string
		wantArgs []string
	}{
		{"darwin", "open", []string{"-R", path}},
		{"windows", "explorer", []string{"/select," + path}},
		{"linux", "xdg-open", []string{filepath.Dir(path)}},
		{"freebsd", "xdg-open", []string{filepath.Dir(path)}},
	}
	for _, tc := range tests {
		t.Run(tc.goos, func(t *testing.T) {
			name, args := RevealCommand(tc.goos, path)
			if name != tc.wantName {
				t.Errorf("name = %q, want %q", name, tc.wantName)
			}
			if !reflect.DeepEqual(args, tc.wantArgs) {
				t.Errorf("args = %q, want %q", args, tc.wantArgs)
			}
		})
	}
}

func TestRevealCommandKeepsPathAsOneArgument(t *testing.T) {
	// A path with spaces and shell characters is passed through as a single
	// argument, never split or interpreted
	path := filepath.Join("my data", "shots; rm -rf", "1760540000123_achievement_Beat the Boss.png")
	for _, goos := range []string{"darwin", "windows", "linux"} {
		_, args := RevealCommand(goos, path)
		if len(args) > 2 {
			t.Errorf("%s: path split into %q", goos, args)
		}
	}
}
