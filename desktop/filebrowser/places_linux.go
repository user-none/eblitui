// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package filebrowser

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// standardFolders returns Desktop, Documents and Downloads from the XDG
// user-dirs file, falling back to the English names under home. Labels use the
// folder's own name so localized folders display as the file manager shows them.
func standardFolders(home string) []Place {
	dirs := map[string]string{}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" || !filepath.IsAbs(configHome) {
		configHome = filepath.Join(home, ".config")
	}
	if f, err := os.Open(filepath.Join(configHome, "user-dirs.dirs")); err == nil {
		dirs = parseUserDirs(f, home)
		f.Close()
	}

	keys := []struct {
		key      string
		fallback string
	}{
		{"XDG_DESKTOP_DIR", "Desktop"},
		{"XDG_DOCUMENTS_DIR", "Documents"},
		{"XDG_DOWNLOAD_DIR", "Downloads"},
	}
	out := make([]Place, 0, len(keys))
	for _, k := range keys {
		p, ok := dirs[k.key]
		if !ok {
			p = filepath.Join(home, k.fallback)
		}
		out = append(out, Place{Label: filepath.Base(p), Path: p, Kind: PlaceFolder})
	}
	return out
}

// volumes returns the filesystem root followed by the volumes mounted under
// the removable media mount roots.
func volumes() []Place {
	out := []Place{{Label: "/", Path: "/", Kind: PlaceVolume}}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return out
	}
	defer f.Close()
	return append(out, parseMountInfo(f)...)
}

// parseUserDirs reads XDG user-dirs.dirs lines of the form
// XDG_NAME_DIR="$HOME/Name" or XDG_NAME_DIR="/abs/path". Comments, malformed
// quoting and relative paths are ignored.
func parseUserDirs(r io.Reader, home string) map[string]string {
	dirs := make(map[string]string)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || len(val) < 2 || val[0] != '"' || val[len(val)-1] != '"' {
			continue
		}
		val = val[1 : len(val)-1]
		if rest, ok := strings.CutPrefix(val, "$HOME"); ok {
			val = home + rest
		}
		if !filepath.IsAbs(val) {
			continue
		}
		dirs[strings.TrimSpace(key)] = filepath.Clean(val)
	}
	return dirs
}

// parseMountInfo returns the mount points in a mountinfo listing that sit below
// one of the removable media mount roots, sorted by label. The mount point is
// the fifth field with spaces, tabs, newlines and backslashes octal-escaped.
func parseMountInfo(r io.Reader) []Place {
	var places []Place
	seen := make(map[string]bool)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 5 {
			continue
		}
		mp := unescapeMount(fields[4])
		if seen[mp] || !underMountRoot(mp) {
			continue
		}
		seen[mp] = true
		places = append(places, Place{Label: filepath.Base(mp), Path: mp, Kind: PlaceVolume})
	}
	sort.SliceStable(places, func(i, j int) bool { return places[i].Label < places[j].Label })
	return places
}

// underMountRoot reports whether mp is a mount below /media, /mnt or
// /run/media. /run/media holds a directory per user, so its mounts are one
// level deeper; the per-user directory itself is not a mount of interest.
func underMountRoot(mp string) bool {
	clean := filepath.Clean(mp)
	for _, root := range []string{"/media/", "/mnt/"} {
		if rest, ok := strings.CutPrefix(clean, root); ok && rest != "" {
			return true
		}
	}
	if rest, ok := strings.CutPrefix(clean, "/run/media/"); ok {
		_, vol, ok := strings.Cut(rest, "/")
		return ok && vol != ""
	}
	return false
}

// unescapeMount decodes the \NNN octal escapes mountinfo uses. Anything that
// is not a complete escape is kept as is.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		// The first digit is limited to 0-3 so the value fits in a byte
		if s[i] == '\\' && i+3 < len(s) && s[i+1] >= '0' && s[i+1] <= '3' && isOctal(s[i+2]) && isOctal(s[i+3]) {
			v := (s[i+1]-'0')<<6 | (s[i+2]-'0')<<3 | (s[i+3] - '0')
			b.WriteByte(v)
			i += 3
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isOctal(c byte) bool {
	return c >= '0' && c <= '7'
}
