package romloader

import (
	"path/filepath"
	"testing"
)

func TestParseMSF(t *testing.T) {
	cases := map[string]int{
		"00:00:00": 0,
		"00:02:00": 150,
		"00:01:74": 149,
		"01:00:00": 4500,
		"10:00:00": 45000,
		"99:59:74": 449999,
	}
	for s, want := range cases {
		got, err := parseMSF(s)
		if err != nil {
			t.Errorf("parseMSF(%q): %v", s, err)
			continue
		}
		if got != want {
			t.Errorf("parseMSF(%q) = %d, want %d", s, got, want)
		}
	}
	for _, bad := range []string{"1:2", "aa:bb:cc", "00:60:00", "00:00:75", "100:00:00", "9223372036854775807:00:00"} {
		if _, err := parseMSF(bad); err == nil {
			t.Errorf("parseMSF(%q): expected error", bad)
		}
	}
}

func TestParseCueNumber(t *testing.T) {
	good := []struct {
		s    string
		min  int
		want int
	}{
		{"01", 1, 1},
		{"99", 1, 99},
		{"00", 0, 0},
		{"10", 0, 10},
	}
	for _, tc := range good {
		got, ok := parseCueNumber(tc.s, tc.min)
		if !ok || got != tc.want {
			t.Errorf("parseCueNumber(%q, %d) = %d, %v, want %d", tc.s, tc.min, got, ok, tc.want)
		}
	}
	bad := []struct {
		s   string
		min int
	}{
		{"1", 0},
		{"001", 0},
		{"00", 1},
		{"-1", 0},
		{"+1", 0},
		{"1a", 0},
		{"", 0},
	}
	for _, tc := range bad {
		if _, ok := parseCueNumber(tc.s, tc.min); ok {
			t.Errorf("parseCueNumber(%q, %d): expected rejection", tc.s, tc.min)
		}
	}
}

func TestParseFileLine(t *testing.T) {
	filename, ftype := parseFileLine(`"a b c.bin" BINARY`)
	if filename != "a b c.bin" || ftype != "BINARY" {
		t.Errorf("quoted = %q,%q", filename, ftype)
	}
	filename, ftype = parseFileLine(`bare.bin BINARY`)
	if filename != "bare.bin" || ftype != "BINARY" {
		t.Errorf("bare = %q,%q", filename, ftype)
	}
}

func TestResolveTrackPath(t *testing.T) {
	if _, err := resolveTrackPath(""); err == nil {
		t.Error("empty name should be rejected")
	}
	got, err := resolveTrackPath("Track 1.bin")
	if err != nil || got != "Track 1.bin" {
		t.Errorf("relative = %q, %v", got, err)
	}
	got, err = resolveTrackPath(`sub\Track 1.bin`)
	if err != nil || got != filepath.FromSlash("sub/Track 1.bin") {
		t.Errorf("backslash = %q, %v", got, err)
	}
	for _, alias := range []string{"./a.bin", ".//a.bin", "sub/../a.bin", `.\a.bin`} {
		got, err = resolveTrackPath(alias)
		if err != nil || got != "a.bin" {
			t.Errorf("alias %q = %q, %v", alias, got, err)
		}
	}
}
