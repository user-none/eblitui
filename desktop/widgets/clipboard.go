// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	"context"
	"time"

	"golang.design/x/clipboard"
)

// clipboardTimeout bounds a clipboard call made from the UI thread. On X11
// a read waits on the selection owner and can otherwise block for seconds.
const clipboardTimeout = time.Second

// clipboardAvailable reports whether clipboard.Init succeeded. Init fails
// without a display server. It runs once and later calls return the first
// result.
func clipboardAvailable() bool {
	return clipboard.Init() == nil
}

// ClipboardWrite puts s on the system clipboard. It reports false when no
// clipboard is available or the write fails.
func ClipboardWrite(s string) bool {
	if !clipboardAvailable() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()
	_, err := clipboard.Write(ctx, clipboard.FmtText, []byte(s))
	return err == nil
}

// ClipboardRead returns the clipboard text. ok is false when no clipboard
// is available or it holds no text.
func ClipboardRead() (string, bool) {
	if !clipboardAvailable() {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()
	b, err := clipboard.Read(ctx, clipboard.FmtText)
	if err != nil {
		return "", false
	}
	return string(b), true
}
