package romloader

import "testing"

func TestParseMSF(t *testing.T) {
	cases := map[string]int{
		"00:00:00": 0,
		"00:02:00": 150,
		"00:01:74": 149,
		"01:00:00": 4500,
		"10:00:00": 45000,
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
	for _, bad := range []string{"1:2", "aa:bb:cc", "00:60:00", "00:00:75"} {
		if _, err := parseMSF(bad); err == nil {
			t.Errorf("parseMSF(%q): expected error", bad)
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
	if _, err := resolveTrackPath("/discs", "/etc/passwd"); err == nil {
		t.Error("absolute path should be rejected")
	}
	if _, err := resolveTrackPath("/discs", "../escape.bin"); err == nil {
		t.Error("parent traversal should be rejected")
	}
	if _, err := resolveTrackPath("/discs", "sub/../../escape.bin"); err == nil {
		t.Error("embedded parent traversal should be rejected")
	}
	got, err := resolveTrackPath("/discs", "Track 1.bin")
	if err != nil || got != "/discs/Track 1.bin" {
		t.Errorf("relative = %q, %v", got, err)
	}
	got, err = resolveTrackPath("/discs", `sub\Track 1.bin`)
	if err != nil || got != "/discs/sub/Track 1.bin" {
		t.Errorf("backslash = %q, %v", got, err)
	}
}
