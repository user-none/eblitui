// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import "golang.design/x/clipboard"

// clipboardReady tracks a successful clipboard.Init. Init fails without
// cgo or a display server, and reading or writing before it succeeds
// panics. Each use retries Init so a clipboard that becomes available
// later still works.
var clipboardReady bool

func clipboardInit() bool {
	if !clipboardReady {
		if err := clipboard.Init(); err == nil {
			clipboardReady = true
		}
	}
	return clipboardReady
}

// ClipboardWrite puts s on the system clipboard. It reports false when no
// clipboard is available.
func ClipboardWrite(s string) bool {
	if !clipboardInit() {
		return false
	}
	clipboard.Write(clipboard.FmtText, []byte(s))
	return true
}

// ClipboardRead returns the clipboard text. ok is false when no clipboard
// is available or it holds no text.
func ClipboardRead() (string, bool) {
	if !clipboardInit() {
		return "", false
	}
	b := clipboard.Read(clipboard.FmtText)
	if b == nil {
		return "", false
	}
	return string(b), true
}
