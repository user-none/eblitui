// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/shader"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// transition is a recorded navigation transition target
type transition struct {
	zone  string
	index int
}

// navRecorder records the zones and transitions a section registers
type navRecorder struct {
	zones       map[string][]string
	transitions map[string]map[int]transition
}

func newNavRecorder() *navRecorder {
	return &navRecorder{zones: make(map[string][]string), transitions: make(map[string]map[int]transition)}
}

func (n *navRecorder) RegisterFocusButton(string, *widget.Button) {}
func (n *navRecorder) RegisterFocusWidget(string, widget.Focuser) {}
func (n *navRecorder) SetPendingFocus(string)                     {}
func (n *navRecorder) SetScrollContainer(*widgets.ScrollView)     {}
func (n *navRecorder) SaveScrollPosition()                        {}
func (n *navRecorder) RestoreScrollPosition()                     {}
func (n *navRecorder) RegisterNavZone(name, _ string, keys []string, _ int) {
	n.zones[name] = keys
}
func (n *navRecorder) SetNavTransition(from string, dir int, to string, index int) {
	if n.transitions[from] == nil {
		n.transitions[from] = make(map[int]transition)
	}
	n.transitions[from][dir] = transition{to, index}
}

func TestVideoShaderZonesLinked(t *testing.T) {
	hasPre, hasShader := false, false
	for _, info := range shader.AvailableShaders {
		if info.Preprocess {
			hasPre = true
		} else {
			hasShader = true
		}
	}
	if !hasPre || !hasShader {
		t.Skip("needs both game-only effects and UI+Game shaders")
	}

	v := NewVideoSection(nil, storage.DefaultConfig(), coreif.SystemInfo{}, func() int { return 800 })
	nav := newNavRecorder()
	v.setupNavigation(nav)

	if got := nav.transitions["video-preprocess"][types.DirDown]; got != (transition{"video-shaders", 1}) {
		t.Errorf("down from the game-only effects = %+v, want the first Game button", got)
	}
	if got := nav.transitions["video-shaders"][types.DirUp]; got != (transition{"video-preprocess", types.NavIndexLast}) {
		t.Errorf("up from the shaders = %+v, want the last game-only effect", got)
	}
}
