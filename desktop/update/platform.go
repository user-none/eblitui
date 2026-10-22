// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package update

import "runtime"

// Platform returns the manifest platform name of the running build, or ""
// when releases are not built for it.
func Platform() string {
	return platformName(runtime.GOOS, runtime.GOARCH)
}

// platformName returns the manifest platform name for a Go OS and
// architecture, or "" when releases are not built for it.
func platformName(goos, goarch string) string {
	switch goos + "/" + goarch {
	case "windows/amd64":
		return "windows-amd64"
	case "windows/arm64":
		return "windows-arm64"
	case "darwin/arm64":
		return "macos-arm64"
	case "linux/amd64":
		return "linux-x86_64"
	case "linux/arm64":
		return "linux-aarch64"
	}
	return ""
}
