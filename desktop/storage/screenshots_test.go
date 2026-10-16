// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

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
		if got := ScreenshotFilename(ts, tc.n, tc.tag); got != tc.want {
			t.Errorf("ScreenshotFilename(%d, %q) = %q, want %q", tc.n, tc.tag, got, tc.want)
		}
	}
}

func TestSanitizeScreenshotTitle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "Beat the First Boss", "Beat the First Boss"},
		{"allowed punctuation kept", "Stage 1-2. Done! (Hard)", "Stage 1-2. Done! (Hard)"},
		{"comma dropped", "Beat, the Boss", "Beat the Boss"},
		{"quotes dropped", `Don't "Panic"`, "Dont Panic"},
		{"shell chars dropped", "a;b$c`d|e&f>g<h*i?j%k#l~m{n}o[p]q=r", "abcdefghijklmnopqr"},
		{"path chars dropped", `up/../down\here:there`, "up..downherethere"},
		{"underscore dropped", "snake_case_title", "snakecasetitle"},
		{"control chars dropped", "Line\x00One\x1fTwo\x7f", "LineOneTwo"},
		{"whitespace collapsed", "  Too   many\t\nspaces  ", "Too many spaces"},
		{"unicode letters kept", "ステージ 1 クリア", "ステージ 1 クリア"},
		{"combining marks kept", "été", "été"},
		{"only disallowed", `<>:"/\|?*,;`, ""},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeScreenshotTitle(tc.in); got != tc.want {
				t.Errorf("SanitizeScreenshotTitle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeScreenshotTitleTruncates(t *testing.T) {
	long := strings.Repeat("ab", MaxScreenshotTitleLen)
	got := SanitizeScreenshotTitle(long)
	if len([]rune(got)) != MaxScreenshotTitleLen {
		t.Errorf("len = %d, want %d", len([]rune(got)), MaxScreenshotTitleLen)
	}

	// Truncation at a space leaves no trailing space
	spaced := strings.Repeat("a", MaxScreenshotTitleLen-1) + " bcd"
	got = SanitizeScreenshotTitle(spaced)
	if strings.HasSuffix(got, " ") {
		t.Errorf("trailing space after truncation: %q", got)
	}

	// Multi-byte runes count as one
	wide := strings.Repeat("ス", MaxScreenshotTitleLen+10)
	got = SanitizeScreenshotTitle(wide)
	if len([]rune(got)) != MaxScreenshotTitleLen {
		t.Errorf("wide len = %d, want %d", len([]rune(got)), MaxScreenshotTitleLen)
	}

	// Four-byte runes hit the byte cap before the rune cap, on a rune
	// boundary
	wide4 := strings.Repeat("\U00010400", MaxScreenshotTitleLen)
	got = SanitizeScreenshotTitle(wide4)
	if len(got) > MaxScreenshotTitleBytes {
		t.Errorf("bytes = %d, want at most %d", len(got), MaxScreenshotTitleBytes)
	}
	if len(got) != MaxScreenshotTitleBytes/4*4 {
		t.Errorf("bytes = %d, want %d", len(got), MaxScreenshotTitleBytes/4*4)
	}
}

func TestParseScreenshotName(t *testing.T) {
	tests := []struct {
		name        string
		wantTime    time.Time
		wantAch     string
		wantOK      bool
		description string
	}{
		{"1760540000123.png", time.UnixMilli(1760540000123), "", true, "manual"},
		{"1760540000123-1.png", time.UnixMilli(1760540000123), "", true, "collision suffix"},
		{"1760540000123_achievement_Beat the Boss.png", time.UnixMilli(1760540000123), "Beat the Boss", true, "achievement"},
		{"1760540000123-2_achievement_Beat the Boss.png", time.UnixMilli(1760540000123), "Beat the Boss", true, "suffix and achievement"},
		{"1760540000123_achievement_Stage 1-2. Done! (Hard).png", time.UnixMilli(1760540000123), "Stage 1-2. Done! (Hard)", true, "allowed punctuation"},
		{"1760540000.png", time.Unix(1760540000, 0), "", true, "legacy seconds"},
		{"1760540000123_future_tag.png", time.Time{}, "", false, "unknown tag"},
		{"1760540000123_achievement_.png", time.Time{}, "", false, "empty title"},
		{"1760540000123_achievement_snake_case_title.png", time.Time{}, "", false, "underscore in title"},
		{"1760540000123_achievement_Beat, the Boss.png", time.Time{}, "", false, "comma in title"},
		{"1760540000123_achievement_a;b$c.png", time.Time{}, "", false, "shell chars in title"},
		{"1760540000123_achievement_ Boss.png", time.Time{}, "", false, "leading space in title"},
		{"1760540000123_achievement_Boss .png", time.Time{}, "", false, "trailing space in title"},
		{"1760540000123_achievement_Two  spaces.png", time.Time{}, "", false, "double space in title"},
		{"boxart.png", time.Time{}, "", false, "no timestamp"},
		{"1760540000123.jpg", time.Time{}, "", false, "wrong extension"},
		{"1760540000123abc.png", time.Time{}, "", false, "junk after timestamp"},
		{"1760540000123-.png", time.Time{}, "", false, "empty suffix"},
		{"", time.Time{}, "", false, "empty"},
	}
	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			ts, ach, ok := ParseScreenshotName(tc.name)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if !ts.Equal(tc.wantTime) {
				t.Errorf("time = %v, want %v", ts, tc.wantTime)
			}
			if ach != tc.wantAch {
				t.Errorf("achievement = %q, want %q", ach, tc.wantAch)
			}
		})
	}
}

func TestScreenshotNameRoundTrip(t *testing.T) {
	ts := time.UnixMilli(1760540000123)
	for _, tag := range []string{"", "achievement_Beat the Boss", "achievement_12345"} {
		for _, n := range []int{0, 3} {
			name := ScreenshotFilename(ts, n, tag)
			got, ach, ok := ParseScreenshotName(name)
			if !ok || !got.Equal(ts) {
				t.Errorf("%q: ok=%v time=%v", name, ok, got)
			}
			wantAch := ""
			if tag != "" {
				wantAch = tag[len(ScreenshotAchievementTag):]
			}
			if ach != wantAch {
				t.Errorf("%q: achievement = %q, want %q", name, ach, wantAch)
			}
		}
	}
}

// writeShots creates empty files with the given names in dir
func writeShots(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestListScreenshots(t *testing.T) {
	dir := t.TempDir()
	writeShots(t, dir,
		"1760540000123.png",
		"1760540000123-1.png",
		"1760540000999_achievement_Beat the Boss.png",
		"1760539000.png",
		"notes.txt",
		"boxart.png",
	)
	if err := os.Mkdir(filepath.Join(dir, "1760540000500.png"), 0755); err != nil {
		t.Fatal(err)
	}
	// A symlink named as a screenshot is not one, wherever it points
	if runtime.GOOS != "windows" {
		if err := os.Symlink(filepath.Join(dir, "1760540000123.png"), filepath.Join(dir, "1760540000700.png")); err != nil {
			t.Fatal(err)
		}
	}

	shots, err := ListScreenshots(dir)
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{
		"1760540000999_achievement_Beat the Boss.png",
		"1760540000123.png",
		"1760540000123-1.png",
		"1760539000.png",
	}
	if len(shots) != len(wantNames) {
		t.Fatalf("got %d screenshots, want %d: %+v", len(shots), len(wantNames), shots)
	}
	for i, want := range wantNames {
		if filepath.Base(shots[i].Path) != want {
			t.Errorf("shots[%d] = %q, want %q", i, filepath.Base(shots[i].Path), want)
		}
		if filepath.Dir(shots[i].Path) != dir {
			t.Errorf("shots[%d] dir = %q, want %q", i, filepath.Dir(shots[i].Path), dir)
		}
	}
	if shots[0].Achievement != "Beat the Boss" {
		t.Errorf("achievement = %q", shots[0].Achievement)
	}
	if shots[1].Achievement != "" {
		t.Errorf("manual achievement = %q", shots[1].Achievement)
	}
}

func TestListScreenshotsMissingDir(t *testing.T) {
	shots, err := ListScreenshots(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("missing dir: %v", err)
	}
	if len(shots) != 0 {
		t.Errorf("got %d screenshots from a missing dir", len(shots))
	}
}

func TestDeleteScreenshot(t *testing.T) {
	dir := t.TempDir()
	writeShots(t, dir, "1760540000123.png")
	path := filepath.Join(dir, "1760540000123.png")

	if err := DeleteScreenshot(dir, path); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still exists: %v", err)
	}

	// Deleting again reports the missing file
	if err := DeleteScreenshot(dir, path); err == nil {
		t.Error("expected error deleting a missing file")
	}
}

func TestDeleteScreenshotRejectsOutsideDir(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	writeShots(t, other, "1760540000123.png")
	writeShots(t, dir, "notes.txt")

	cases := []string{
		filepath.Join(other, "1760540000123.png"),
		filepath.Join(dir, "..", filepath.Base(other), "1760540000123.png"),
		filepath.Join(dir, "sub", "1760540000123.png"),
		filepath.Join(dir, "notes.txt"),
	}
	for _, path := range cases {
		if err := DeleteScreenshot(dir, path); err == nil {
			t.Errorf("%q: expected rejection", path)
		}
	}
	if _, err := os.Stat(filepath.Join(other, "1760540000123.png")); err != nil {
		t.Errorf("file outside dir was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Errorf("non-screenshot was removed: %v", err)
	}
}
