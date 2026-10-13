// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package filebrowser

import (
	"strings"
	"testing"
)

func TestParseUserDirs(t *testing.T) {
	input := strings.Join([]string{
		"# comment",
		`XDG_DESKTOP_DIR="$HOME/Schreibtisch"`,
		`XDG_DOCUMENTS_DIR="/data/docs"`,
		`XDG_DOWNLOAD_DIR="$HOME/"`,
		`XDG_MUSIC_DIR="relative/music"`,
		`XDG_VIDEOS_DIR=$HOME/Videos`,
		`XDG_PICTURES_DIR="`,
		`garbage`,
		``,
	}, "\n")

	dirs := parseUserDirs(strings.NewReader(input), "/home/user")

	want := map[string]string{
		"XDG_DESKTOP_DIR":   "/home/user/Schreibtisch",
		"XDG_DOCUMENTS_DIR": "/data/docs",
		"XDG_DOWNLOAD_DIR":  "/home/user",
	}
	if len(dirs) != len(want) {
		t.Errorf("got %d entries %v, want %d", len(dirs), dirs, len(want))
	}
	for k, v := range want {
		if dirs[k] != v {
			t.Errorf("%s = %q, want %q", k, dirs[k], v)
		}
	}
}

func TestParseMountInfo(t *testing.T) {
	input := strings.Join([]string{
		"22 1 8:2 / / rw,relatime shared:1 - ext4 /dev/sda2 rw",
		"23 22 0:5 / /proc rw shared:2 - proc proc rw",
		"40 22 8:17 / /run/media/user/USB\\040STICK rw shared:3 - vfat /dev/sdb1 rw",
		"41 22 8:33 / /media/SDCARD rw shared:4 - exfat /dev/sdc1 rw",
		"42 22 8:49 / /mnt/backup rw shared:5 - ext4 /dev/sdd1 rw",
		"43 22 0:40 / /run/media/user rw shared:6 - tmpfs tmpfs rw",
		"44 22 0:41 / /media rw shared:7 - tmpfs tmpfs rw",
		"45 22 8:33 / /media/SDCARD rw shared:8 - exfat /dev/sdc1 rw",
		"46 22 8:65 / /mnt/bad\\9escape rw shared:9 - ext4 /dev/sde1 rw",
		"short line",
		"",
	}, "\n")

	places := parseMountInfo(strings.NewReader(input))

	want := []Place{
		{Label: "SDCARD", Path: "/media/SDCARD", Kind: PlaceVolume},
		{Label: "USB STICK", Path: "/run/media/user/USB STICK", Kind: PlaceVolume},
		{Label: "backup", Path: "/mnt/backup", Kind: PlaceVolume},
		{Label: `bad\9escape`, Path: `/mnt/bad\9escape`, Kind: PlaceVolume},
	}
	if len(places) != len(want) {
		t.Fatalf("got %+v, want %+v", places, want)
	}
	for i := range want {
		if places[i] != want[i] {
			t.Errorf("place %d = %+v, want %+v", i, places[i], want[i])
		}
	}
}

func TestUnderMountRoot(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/", false},
		{"/media", false},
		{"/media/", false},
		{"/media/disk", true},
		{"/mnt", false},
		{"/mnt/data", true},
		{"/run/media", false},
		{"/run/media/user", false},
		{"/run/media/user/", false},
		{"/run/media/user/disk", true},
		{"/home/user/media/disk", false},
		{"/mediafiles/disk", false},
	}
	for _, tc := range tests {
		if got := underMountRoot(tc.path); got != tc.want {
			t.Errorf("underMountRoot(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestUnescapeMount(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{`a\040b`, "a b"},
		{`tab\011x`, "tab\tx"},
		{`back\134slash`, `back\slash`},
		{`end\04`, `end\04`},
		{`end\`, `end\`},
		{`big\777`, `big\777`},
		{`bad\08x`, `bad\08x`},
	}
	for _, tc := range tests {
		if got := unescapeMount(tc.in); got != tc.want {
			t.Errorf("unescapeMount(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
