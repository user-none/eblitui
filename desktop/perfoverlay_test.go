// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/user-none/eblitui/coreif"
)

// feedFrames adds n frames to p at interval spacing starting after
// start, each taking d, with the game frame count rising every other
// frame from gameBase, and returns the time of the last frame and the
// game count after it.
func feedFrames(p *PerfStats, start time.Time, n int, interval, d time.Duration, gameBase uint64) (time.Time, uint64) {
	now := start
	games := gameBase
	for i := 1; i <= n; i++ {
		now = start.Add(time.Duration(i) * interval)
		games = gameBase + uint64(i/2)
		p.AddFrame(d, games, now)
	}
	return now, games
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestPerfStatsPublishesAfterWindow(t *testing.T) {
	var p PerfStats
	t0 := time.Unix(1000, 0)
	p.AddFrame(5*time.Millisecond, 0, t0) // starts the window, not counted

	// 49 frames at 20 ms: 980 ms elapsed, nothing published yet.
	last, games := feedFrames(&p, t0, 49, 20*time.Millisecond, 5*time.Millisecond, 0)
	if e, g, f := p.Snapshot(); e != 0 || g != 0 || f != 0 {
		t.Fatalf("published before the window elapsed: %v %v %v", e, g, f)
	}

	// The 50th frame reaches 1000 ms: 50 frames, 25 game frames, 5 ms each.
	p.AddFrame(5*time.Millisecond, games+1, last.Add(20*time.Millisecond))
	e, g, f := p.Snapshot()
	if !near(e, 50) || !near(g, 25) || !near(f, 5) {
		t.Fatalf("published %v %v %v, want 50 25 5", e, g, f)
	}
}

func TestPerfStatsGapRestartsWindow(t *testing.T) {
	var p PerfStats
	t0 := time.Unix(1000, 0)
	p.AddFrame(0, 0, t0)
	last, games := feedFrames(&p, t0, 50, 20*time.Millisecond, 4*time.Millisecond, 0)
	if e, _, _ := p.Snapshot(); !near(e, 50) {
		t.Fatalf("first window published %v, want 50", e)
	}

	// A 2 s pause, then a full window of frames: the pause is not
	// counted as elapsed time, so the rate stays 50.
	resume := last.Add(2 * time.Second)
	p.AddFrame(4*time.Millisecond, games, resume)
	feedFrames(&p, resume, 50, 20*time.Millisecond, 4*time.Millisecond, games)
	e, g, f := p.Snapshot()
	if !near(e, 50) || !near(g, 25) || !near(f, 4) {
		t.Fatalf("window after the gap published %v %v %v, want 50 25 4", e, g, f)
	}
}

func TestPerfStatsGameCountFallingBack(t *testing.T) {
	var p PerfStats
	t0 := time.Unix(1000, 0)
	p.AddFrame(0, 100, t0)
	// The game count starts the window at 100 and reports 40 after, as
	// after a core reset: the rate is 0, not a wrapped value.
	for i := 1; i <= 50; i++ {
		p.AddFrame(time.Millisecond, 40, t0.Add(time.Duration(i)*20*time.Millisecond))
	}
	if _, g, _ := p.Snapshot(); g != 0 {
		t.Fatalf("game fps with a falling count %v, want 0", g)
	}
}

func TestPerfStatsReset(t *testing.T) {
	var p PerfStats
	t0 := time.Unix(1000, 0)
	p.AddFrame(0, 0, t0)
	feedFrames(&p, t0, 50, 20*time.Millisecond, time.Millisecond, 0)
	p.Reset()
	if e, g, f := p.Snapshot(); e != 0 || g != 0 || f != 0 {
		t.Fatalf("values after reset %v %v %v", e, g, f)
	}
	// The first frame after a reset starts a window and is not counted.
	p.AddFrame(0, 7, t0.Add(time.Hour))
	if p.frames != 0 || p.gameFramesStart != 7 {
		t.Fatalf("window after reset: frames %d, game start %d", p.frames, p.gameFramesStart)
	}
}

func TestPerfOverlayLines(t *testing.T) {
	sw := &coreif.RenderInfo{Backend: "Software", Scale: 1}
	sw2 := &coreif.RenderInfo{Backend: "Software", Scale: 2}
	gpu4 := &coreif.RenderInfo{Backend: "Vulkan", Device: "Test GPU", Scale: 4}
	cases := []struct {
		name string
		mode perfOverlayMode
		info *coreif.RenderInfo
		want string
	}{
		{"off", perfOverlayOff, sw, ""},
		{"basic no info", perfOverlayBasic, nil, "60.0 fps"},
		{"basic", perfOverlayBasic, sw, "60.0 fps\n30.0 gfps"},
		{"extended no info", perfOverlayExtended, nil, "60.0 fps\n12.3 ms"},
		{"extended software native", perfOverlayExtended, sw, "60.0 fps\n30.0 gfps\n12.3 ms\nSoftware"},
		{"extended software scaled", perfOverlayExtended, sw2, "60.0 fps\n30.0 gfps\n12.3 ms\nSoftware 2x"},
		{"extended gpu scaled", perfOverlayExtended, gpu4, "60.0 fps\n30.0 gfps\n12.3 ms\nTest GPU (Vulkan) 4x"},
	}
	for _, c := range cases {
		got := strings.Join(perfOverlayLines(c.mode, 60, 30, 12.34, c.info), "\n")
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPerfOverlayCycle(t *testing.T) {
	var o PerfOverlay
	want := []perfOverlayMode{perfOverlayBasic, perfOverlayExtended, perfOverlayOff, perfOverlayBasic}
	for i, m := range want {
		o.Cycle()
		if o.mode != m {
			t.Fatalf("cycle %d: mode %d, want %d", i+1, o.mode, m)
		}
	}
}
