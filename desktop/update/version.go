// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package update checks for new application releases. It verifies a release
// manifest signed by the release workflow and decides whether it offers a
// newer version for the running platform.
package update

import (
	"strconv"
	"strings"
)

// Version is a release version.
type Version struct {
	Major, Minor, Patch int
}

// ParseVersion parses a release version of exactly the form
// MAJOR.MINOR.PATCH and reports whether it is one. A leading "v", used by
// release tags, is not part of a version.
func ParseVersion(s string) (Version, bool) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, false
	}

	var n [3]int
	for i, part := range parts {
		if part == "" {
			return Version{}, false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return Version{}, false
			}
		}
		v, err := strconv.Atoi(part)
		if err != nil {
			return Version{}, false
		}
		n[i] = v
	}
	return Version{Major: n[0], Minor: n[1], Patch: n[2]}, true
}

// RunningVersion parses the running application's version. A version that
// is not a release version is treated as 0.0.0, so any release is newer.
func RunningVersion(s string) Version {
	v, _ := ParseVersion(s)
	return v
}

// Less reports whether v is older than o.
func (v Version) Less(o Version) bool {
	if v.Major != o.Major {
		return v.Major < o.Major
	}
	if v.Minor != o.Minor {
		return v.Minor < o.Minor
	}
	return v.Patch < o.Patch
}
