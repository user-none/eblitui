// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package launch

import "testing"

func TestStartMissingProgram(t *testing.T) {
	if err := start("eblitui-launch-test-no-such-program"); err == nil {
		t.Error("expected an error for a program that does not exist")
	}
}
