// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package launch hands things to the operating system to open: a file in the
// file manager or a URL in the browser.
package launch

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// ShowInFileManager shows path in the platform file manager: Finder with the
// file selected on macOS, Explorer with the file selected on Windows, and the
// containing folder through xdg-open elsewhere, where selecting a file is not
// portable across file managers. The file manager is started and left
// running; only a failure to start it is reported.
func ShowInFileManager(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return start("open", "-R", path)
	case "windows":
		return start("explorer", "/select,"+path)
	default:
		return start("xdg-open", filepath.Dir(path))
	}
}

// OpenURL opens url in the default browser. The opener is started and left
// running; only a failure to start it is reported.
func OpenURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return start("open", url)
	case "windows":
		return start("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return start("xdg-open", url)
	}
}

// start starts a program and leaves it running. Only a failure to start it
// is reported. The program is reaped when it exits; its exit status says
// nothing useful (Explorer reports failure even when it opened the folder).
func start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
