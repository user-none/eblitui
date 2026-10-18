// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"slices"
	"testing"

	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/storage"
)

func TestLaunchCoreOption(t *testing.T) {
	perGame := coreif.CoreOption{Key: "upscale", Type: coreif.CoreOptionSelect, Default: "1x", Values: []string{"1x", "2x", "4x"}, PerGame: true}
	global := coreif.CoreOption{Key: "renderer", Type: coreif.CoreOptionSelect, Default: "GPU", Values: []string{"GPU", "Software"}}

	tests := []struct {
		name      string
		opt       coreif.CoreOption
		config    map[string]string
		overrides map[string]string
		want      string
	}{
		{"declared default", perGame, nil, nil, "1x"},
		{"config value", perGame, map[string]string{"upscale": "2x"}, nil, "2x"},
		{"override over config", perGame, map[string]string{"upscale": "2x"}, map[string]string{"upscale": "4x"}, "4x"},
		{"override over default", perGame, nil, map[string]string{"upscale": "4x"}, "4x"},
		{"invalid override", perGame, map[string]string{"upscale": "2x"}, map[string]string{"upscale": "3x"}, "2x"},
		{"not per game", global, map[string]string{"renderer": "GPU"}, map[string]string{"renderer": "Software"}, "GPU"},
	}
	for _, tt := range tests {
		o := &storage.OverrideSettings{CoreOptions: tt.overrides}
		if got := launchCoreOption(tt.opt, tt.config, o); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestLaunchGameShaders(t *testing.T) {
	tests := []struct {
		name      string
		global    []string
		overrides map[string]bool
		want      []string
	}{
		{"no overrides", []string{"crt", "xbr"}, nil, []string{"crt", "xbr"}},
		{"off removes", []string{"crt", "xbr", "overscan"}, map[string]bool{"xbr": false}, []string{"crt", "overscan"}},
		{"on adds in registry order", []string{"crt"}, map[string]bool{"overscan": true, "xbr": true}, []string{"crt", "xbr", "overscan"}},
		{"on already set", []string{"ghosting"}, map[string]bool{"ghosting": true}, []string{"ghosting"}},
		{"off not set", []string{"crt"}, map[string]bool{"ghosting": false}, []string{"crt"}},
		{"not preprocess ignored", []string{"crt"}, map[string]bool{"crt": false, "scanlines": true}, []string{"crt"}},
	}
	for _, tt := range tests {
		o := &storage.OverrideSettings{Shaders: tt.overrides}
		if got := launchGameShaders(tt.global, o); !slices.Equal(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestLaunchGameShadersKeepsGlobal(t *testing.T) {
	global := []string{"xbr", "crt"}
	launchGameShaders(global, &storage.OverrideSettings{Shaders: map[string]bool{"xbr": false}})
	if !slices.Equal(global, []string{"xbr", "crt"}) {
		t.Errorf("global changed to %v", global)
	}
}
