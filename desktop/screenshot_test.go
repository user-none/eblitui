// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "Beat the First Boss", "Beat the First Boss"},
		{"invalid chars dropped", `A<B>C:D"E/F\G|H?I*J`, "ABCDEFGHIJ"},
		{"control chars dropped", "Line\x00One\x1fTwo\x7f", "LineOneTwo"},
		{"whitespace collapsed", "  Too   many\t\nspaces  ", "Too many spaces"},
		{"unicode kept", "ステージ 1 クリア", "ステージ 1 クリア"},
		{"only invalid", `<>:"/\|?*`, ""},
		{"empty", "", ""},
		{"underscores kept", "snake_case_title", "snake_case_title"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeFilename(tc.in); got != tc.want {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeFilenameTruncates(t *testing.T) {
	long := strings.Repeat("ab", maxScreenshotTitleLen)
	got := sanitizeFilename(long)
	if len([]rune(got)) != maxScreenshotTitleLen {
		t.Errorf("len = %d, want %d", len([]rune(got)), maxScreenshotTitleLen)
	}

	// Truncation at a space leaves no trailing space
	spaced := strings.Repeat("a", maxScreenshotTitleLen-1) + " bcd"
	got = sanitizeFilename(spaced)
	if strings.HasSuffix(got, " ") {
		t.Errorf("trailing space after truncation: %q", got)
	}

	// Multi-byte runes count as one
	wide := strings.Repeat("ス", maxScreenshotTitleLen+10)
	got = sanitizeFilename(wide)
	if len([]rune(got)) != maxScreenshotTitleLen {
		t.Errorf("wide len = %d, want %d", len([]rune(got)), maxScreenshotTitleLen)
	}

	// Four-byte runes hit the byte cap before the rune cap, on a rune
	// boundary
	wide4 := strings.Repeat("\U0001F3AE", maxScreenshotTitleLen)
	got = sanitizeFilename(wide4)
	if len(got) > maxScreenshotTitleBytes {
		t.Errorf("bytes = %d, want at most %d", len(got), maxScreenshotTitleBytes)
	}
	if len(got) != maxScreenshotTitleBytes/4*4 {
		t.Errorf("bytes = %d, want %d", len(got), maxScreenshotTitleBytes/4*4)
	}
}

func TestAchievementTag(t *testing.T) {
	if got := achievementTag("Beat the First Boss", 12345); got != "achievement_Beat the First Boss" {
		t.Errorf("got %q", got)
	}
	// An unusable title falls back to the id
	if got := achievementTag(`???`, 12345); got != "achievement_12345" {
		t.Errorf("fallback got %q", got)
	}
	if got := achievementTag("", 7); got != "achievement_7" {
		t.Errorf("empty got %q", got)
	}
}

func TestScreenshotFilename(t *testing.T) {
	ts := time.UnixMilli(1760540000123)
	tests := []struct {
		n    int
		tag  string
		want string
	}{
		{0, "", "1760540000123.png"},
		{1, "", "1760540000123-1.png"},
		{0, "achievement_Beat the Boss", "1760540000123_achievement_Beat the Boss.png"},
		{2, "achievement_Beat the Boss", "1760540000123-2_achievement_Beat the Boss.png"},
	}
	for _, tc := range tests {
		if got := screenshotFilename(ts, tc.n, tc.tag); got != tc.want {
			t.Errorf("screenshotFilename(%d, %q) = %q, want %q", tc.n, tc.tag, got, tc.want)
		}
	}
}

func TestCreateScreenshotFileCollision(t *testing.T) {
	dir := t.TempDir()
	ts := time.UnixMilli(1760540000123)

	// First capture takes the bare name
	f, err := createScreenshotFile(dir, ts, "")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	f.Close()
	if filepath.Base(f.Name()) != "1760540000123.png" {
		t.Errorf("first name = %q", filepath.Base(f.Name()))
	}

	// Same timestamp gets -1, then -2
	for i := 1; i <= 2; i++ {
		f, err = createScreenshotFile(dir, ts, "")
		if err != nil {
			t.Fatalf("collision %d: %v", i, err)
		}
		f.Close()
		want := "1760540000123-" + string(rune('0'+i)) + ".png"
		if filepath.Base(f.Name()) != want {
			t.Errorf("collision %d name = %q, want %q", i, filepath.Base(f.Name()), want)
		}
	}

	// A tagged capture at the same timestamp does not collide with the
	// untagged ones
	f, err = createScreenshotFile(dir, ts, "achievement_X")
	if err != nil {
		t.Fatalf("tagged: %v", err)
	}
	f.Close()
	if filepath.Base(f.Name()) != "1760540000123_achievement_X.png" {
		t.Errorf("tagged name = %q", filepath.Base(f.Name()))
	}
}

func TestCreateScreenshotFileSuffixBound(t *testing.T) {
	dir := t.TempDir()
	ts := time.UnixMilli(1760540000123)
	for n := 0; n <= maxScreenshotSuffix; n++ {
		path := filepath.Join(dir, screenshotFilename(ts, n, ""))
		if err := os.WriteFile(path, nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := createScreenshotFile(dir, ts, ""); err == nil {
		t.Error("expected error when every suffix is taken")
	}
}

func TestCreateScreenshotFileBadDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	if _, err := createScreenshotFile(dir, time.Now(), ""); err == nil {
		t.Error("expected error for missing directory")
	}
}

func TestScreenshotQueue(t *testing.T) {
	m := NewScreenshotManager()

	if tags := m.drain(); len(tags) != 0 {
		t.Errorf("empty queue returned %v", tags)
	}

	// A manual capture and two unlocks on one frame are all kept, in order
	m.Queue("")
	m.Queue("achievement_A")
	m.Queue("achievement_B")
	tags := m.drain()
	if len(tags) != 3 || tags[0] != "" || tags[1] != "achievement_A" || tags[2] != "achievement_B" {
		t.Errorf("got %v", tags)
	}

	// Draining empties the queue
	if tags := m.drain(); len(tags) != 0 {
		t.Errorf("queue not drained: %v", tags)
	}

	// Flushing with no game drops the queue without writing
	m.Queue("achievement_C")
	m.Flush(nil, "")
	if tags := m.drain(); len(tags) != 0 {
		t.Errorf("queue kept after flush with no game: %v", tags)
	}
}
