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

// perfInterval is the spacing of fed frames: 10 frames fill a bucket.
const perfInterval = 25 * time.Millisecond

// feedFrames adds frames from through to to p, frame i finishing at
// start plus i intervals after taking d, with the game frame count
// gameBase plus i/2.
func feedFrames(p *PerfStats, start time.Time, from, to int, d time.Duration, gameBase uint64) {
	for i := from; i <= to; i++ {
		p.AddFrame(d, gameBase+uint64(i/2), start.Add(time.Duration(i)*perfInterval))
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestPerfStatsPublishesEachBucket(t *testing.T) {
	var p PerfStats
	t0 := time.Unix(1000, 0)
	p.AddFrame(5*time.Millisecond, 0, t0) // starts the window, not counted

	// 9 frames: 225 ms elapsed, nothing published yet.
	feedFrames(&p, t0, 1, 9, 5*time.Millisecond, 0)
	if e, g, f, _ := p.Snapshot(); e != 0 || g != 0 || f != 0 {
		t.Fatalf("published before the bucket completed: %v %v %v", e, g, f)
	}

	// The 10th frame completes the bucket at 250 ms: 10 frames, 5 game
	// frames, 5 ms each.
	feedFrames(&p, t0, 10, 10, 5*time.Millisecond, 0)
	e, g, f, _ := p.Snapshot()
	if !near(e, 40) || !near(g, 20) || !near(f, 5) {
		t.Fatalf("published %v %v %v, want 40 20 5", e, g, f)
	}
}

func TestPerfStatsRollingWindow(t *testing.T) {
	var p PerfStats
	t0 := time.Unix(1000, 0)
	p.AddFrame(5*time.Millisecond, 0, t0)

	// Two buckets: averaged over the 500 ms they span.
	feedFrames(&p, t0, 1, 20, 5*time.Millisecond, 0)
	e, g, f, _ := p.Snapshot()
	if !near(e, 40) || !near(g, 20) || !near(f, 5) {
		t.Fatalf("two buckets published %v %v %v, want 40 20 5", e, g, f)
	}

	// The window fills at frame 40. Frames 41-50 take 15 ms: the
	// published window drops the first bucket and spans frames 11-50,
	// so the frame time is (30*5 + 10*15) / 40 = 7.5 ms.
	feedFrames(&p, t0, 21, 40, 5*time.Millisecond, 0)
	feedFrames(&p, t0, 41, 50, 15*time.Millisecond, 0)
	e, g, f, _ = p.Snapshot()
	if !near(e, 40) || !near(g, 20) || !near(f, 7.5) {
		t.Fatalf("rolled window published %v %v %v, want 40 20 7.5", e, g, f)
	}
}

func TestPerfStatsGapRestartsWindow(t *testing.T) {
	var p PerfStats
	t0 := time.Unix(1000, 0)
	p.AddFrame(0, 0, t0)
	feedFrames(&p, t0, 1, 40, 4*time.Millisecond, 0)
	if e, _, _, _ := p.Snapshot(); !near(e, 40) {
		t.Fatalf("before the gap published %v, want 40", e)
	}

	// A 2 s pause, then a bucket of frames: the pause and the buckets
	// before it are not counted, so the rate stays 40.
	resume := t0.Add(40*perfInterval + 2*time.Second)
	p.AddFrame(4*time.Millisecond, 20, resume)
	feedFrames(&p, resume, 1, 10, 4*time.Millisecond, 20)
	e, g, f, _ := p.Snapshot()
	if !near(e, 40) || !near(g, 20) || !near(f, 4) {
		t.Fatalf("after the gap published %v %v %v, want 40 20 4", e, g, f)
	}
}

func TestPerfStatsGameCountFallingBack(t *testing.T) {
	var p PerfStats
	t0 := time.Unix(1000, 0)
	p.AddFrame(0, 100, t0)
	// The game count starts the window at 100 and reports 40 after, as
	// after a core reset: the rate is 0, not a wrapped value.
	for i := 1; i <= 10; i++ {
		p.AddFrame(time.Millisecond, 40, t0.Add(time.Duration(i)*perfInterval))
	}
	if _, g, _, _ := p.Snapshot(); g != 0 {
		t.Fatalf("game fps with a falling count %v, want 0", g)
	}
}

// fakeRenderInfo is a render info provider that counts its reads.
type fakeRenderInfo struct {
	info  coreif.RenderInfo
	reads int
}

func (f *fakeRenderInfo) RenderInfo() coreif.RenderInfo {
	f.reads++
	return f.info
}

func (f *fakeRenderInfo) GameFrameCount() uint64 { return 0 }

func TestPerfStatsRenderInfo(t *testing.T) {
	var p PerfStats
	t0 := time.Unix(1000, 0)

	// Without a provider no render info is published.
	p.Reset(nil)
	p.AddFrame(0, 0, t0)
	feedFrames(&p, t0, 1, 10, time.Millisecond, 0)
	if _, _, _, info := p.Snapshot(); info != nil {
		t.Fatalf("render info without a provider %v", info)
	}

	// With a provider, nothing is read or published until a bucket
	// completes.
	fake := &fakeRenderInfo{info: coreif.RenderInfo{Backend: "Vulkan", Device: "Test GPU", Scale: 2}}
	p.Reset(fake)
	t1 := t0.Add(time.Hour)
	p.AddFrame(0, 0, t1)
	feedFrames(&p, t1, 1, 9, time.Millisecond, 0)
	if _, _, _, info := p.Snapshot(); fake.reads != 0 || info != nil {
		t.Fatalf("before the bucket: reads %d, render info %v", fake.reads, info)
	}
	feedFrames(&p, t1, 10, 10, time.Millisecond, 0)
	if _, _, _, info := p.Snapshot(); fake.reads != 1 || info == nil || *info != fake.info {
		t.Fatalf("after the bucket: reads %d, render info %v, want %v", fake.reads, info, fake.info)
	}

	// A renderer change shows at the next publish; the earlier
	// snapshot is unchanged.
	_, _, _, before := p.Snapshot()
	fake.info = coreif.RenderInfo{Backend: "Software", Scale: 2}
	feedFrames(&p, t1, 11, 20, time.Millisecond, 0)
	if _, _, _, info := p.Snapshot(); fake.reads != 2 || info == nil || *info != fake.info {
		t.Fatalf("next bucket: reads %d, render info %v, want %v", fake.reads, info, fake.info)
	}
	if before.Backend != "Vulkan" {
		t.Fatalf("earlier snapshot changed to %v", before)
	}

	// Reset clears the published render info.
	p.Reset(fake)
	if _, _, _, info := p.Snapshot(); info != nil {
		t.Fatalf("render info after reset %v", info)
	}
}

func TestPerfStatsReset(t *testing.T) {
	var p PerfStats
	t0 := time.Unix(1000, 0)
	p.AddFrame(0, 0, t0)
	feedFrames(&p, t0, 1, 30, time.Millisecond, 0)
	p.Reset(nil)
	if e, g, f, _ := p.Snapshot(); e != 0 || g != 0 || f != 0 {
		t.Fatalf("values after reset %v %v %v", e, g, f)
	}
	// The first frame after a reset starts a window of one bucket and
	// is not counted.
	p.AddFrame(0, 7, t0.Add(time.Hour))
	if b := p.buckets[p.cur]; p.filled != 1 || b.frames != 0 || b.gameFrames != 7 {
		t.Fatalf("window after reset: filled %d, frames %d, game start %d", p.filled, b.frames, b.gameFrames)
	}
}

func TestPerfOverlayLines(t *testing.T) {
	sw := &coreif.RenderInfo{Backend: "Software", Scale: 1}
	sw2 := &coreif.RenderInfo{Backend: "Software", Scale: 2}
	gpu := &coreif.RenderInfo{Backend: "Vulkan", Device: "Test GPU", Scale: 1}
	gpu4 := &coreif.RenderInfo{Backend: "Vulkan", Device: "Test GPU", Scale: 4}
	cases := []struct {
		name string
		mode perfOverlayMode
		info *coreif.RenderInfo
		want string
	}{
		{"off", perfOverlayOff, sw, ""},
		{"basic no info", perfOverlayBasic, nil, "60.0 fps\n12.3 ms"},
		{"basic", perfOverlayBasic, sw, "60.0 fps\n30.0 gfps\n12.3 ms"},
		{"extended no info", perfOverlayExtended, nil, "60.0 fps\n12.3 ms"},
		{"extended software native", perfOverlayExtended, sw, "60.0 fps\n30.0 gfps\n12.3 ms\nSoftware"},
		{"extended software scaled", perfOverlayExtended, sw2, "60.0 fps\n30.0 gfps\n12.3 ms\nSoftware (2x)"},
		{"extended gpu native", perfOverlayExtended, gpu, "60.0 fps\n30.0 gfps\n12.3 ms\nVulkan\nTest GPU"},
		{"extended gpu scaled", perfOverlayExtended, gpu4, "60.0 fps\n30.0 gfps\n12.3 ms\nVulkan (4x)\nTest GPU"},
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
