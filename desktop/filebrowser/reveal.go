// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package filebrowser

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// RevealCommand returns the program and arguments that show path in the
// platform file manager for goos, a runtime.GOOS value: Finder with the
// file selected on macOS, Explorer with the file selected on Windows, and
// the containing folder through xdg-open elsewhere, where selecting a file
// is not portable across file managers.
func RevealCommand(goos, path string) (name string, args []string) {
	switch goos {
	case "darwin":
		return "open", []string{"-R", path}
	case "windows":
		return "explorer", []string{"/select," + path}
	default:
		return "xdg-open", []string{filepath.Dir(path)}
	}
}

// Reveal shows path in the file manager. The file manager is started and
// left running; only a failure to start it is reported.
func Reveal(path string) error {
	name, args := RevealCommand(runtime.GOOS, path)
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the launcher when it exits. Its exit status says nothing useful:
	// Explorer reports failure even when it opened the folder.
	go cmd.Wait()
	return nil
}
