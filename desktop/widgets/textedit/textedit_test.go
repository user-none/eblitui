// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package textedit

import "testing"

func bufferWith(text string) *Buffer {
	b := New(0, 0)
	b.SetText(text)
	return b
}

func checkText(t *testing.T, b *Buffer, want string, caret Pos) {
	t.Helper()
	if got := b.Text(); got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got := b.Caret(); got != caret {
		t.Errorf("caret = %+v, want %+v", got, caret)
	}
}

func TestEmptyBuffer(t *testing.T) {
	b := New(0, 0)
	if b.LineCount() != 1 || b.Line(0) != "" || b.Text() != "" {
		t.Errorf("empty buffer: %d lines, text %q", b.LineCount(), b.Text())
	}
	// Edits at the edges of an empty buffer do nothing
	b.Backspace()
	b.Delete()
	b.MoveLeft(false)
	b.MoveRight(false)
	checkText(t, b, "", Pos{})
}

func TestSetTextNormalizes(t *testing.T) {
	b := bufferWith("160EF100 0000\r\n160C5238 0000\rX\tY\x01Z")
	checkText(t, b, "160EF100 0000\n160C5238 0000\nX YZ", Pos{Line: 2, Col: 4})
	if b.LineCount() != 3 || b.Line(1) != "160C5238 0000" {
		t.Errorf("lines: %d, line 1 %q", b.LineCount(), b.Line(1))
	}
	if _, _, ok := b.Selection(); ok {
		t.Error("SetText left a selection")
	}
}

func TestInsertMultiLineMidLine(t *testing.T) {
	b := bufferWith("AB")
	b.SetCaret(Pos{Col: 1}, false)
	b.Insert("1\n2\n3")
	checkText(t, b, "A1\n2\n3B", Pos{Line: 2, Col: 1})
}

func TestInsertReplacesSelection(t *testing.T) {
	b := bufferWith("one\ntwo\nthree")
	b.SetCaret(Pos{Line: 0, Col: 1}, false)
	b.SetCaret(Pos{Line: 2, Col: 2}, true)
	if got := b.SelectedText(); got != "ne\ntwo\nth" {
		t.Errorf("selected = %q", got)
	}
	b.Insert("X")
	checkText(t, b, "oXree", Pos{Col: 2})
}

func TestBackspaceAndDelete(t *testing.T) {
	b := bufferWith("ab\ncd")
	b.SetCaret(Pos{Line: 1}, false)
	b.Backspace() // joins the lines
	checkText(t, b, "abcd", Pos{Col: 2})
	b.Backspace()
	checkText(t, b, "acd", Pos{Col: 1})
	b.Delete()
	checkText(t, b, "ad", Pos{Col: 1})

	b = bufferWith("ab\ncd")
	b.SetCaret(Pos{Col: 2}, false)
	b.Delete() // joins the next line
	checkText(t, b, "abcd", Pos{Col: 2})

	// With a selection, both delete only the selection
	b = bufferWith("abcd")
	b.SetCaret(Pos{Col: 1}, false)
	b.SetCaret(Pos{Col: 3}, true)
	b.Backspace()
	checkText(t, b, "ad", Pos{Col: 1})
	b.SetCaret(Pos{Col: 0}, false)
	b.SetCaret(Pos{Col: 2}, true)
	b.Delete()
	checkText(t, b, "", Pos{})
}

func TestMoveLeftRightAcrossLines(t *testing.T) {
	b := bufferWith("ab\ncd")
	b.SetCaret(Pos{Line: 1}, false)
	b.MoveLeft(false)
	if got := b.Caret(); got != (Pos{Line: 0, Col: 2}) {
		t.Errorf("left from line start: %+v", got)
	}
	b.MoveRight(false)
	if got := b.Caret(); got != (Pos{Line: 1, Col: 0}) {
		t.Errorf("right from line end: %+v", got)
	}
}

func TestMoveCollapsesSelection(t *testing.T) {
	b := bufferWith("abcdef")
	b.SetCaret(Pos{Col: 2}, false)
	b.SetCaret(Pos{Col: 4}, true)
	b.MoveLeft(false)
	if got := b.Caret(); got != (Pos{Col: 2}) {
		t.Errorf("left collapses to start: %+v", got)
	}

	b.SetCaret(Pos{Col: 4}, false)
	b.SetCaret(Pos{Col: 2}, true)
	b.MoveRight(false)
	if got := b.Caret(); got != (Pos{Col: 4}) {
		t.Errorf("right collapses to end: %+v", got)
	}
	if _, _, ok := b.Selection(); ok {
		t.Error("selection not cleared")
	}
}

func TestMoveUpDownKeepsGoalColumn(t *testing.T) {
	b := bufferWith("abcdef\nab\nabcdef")
	b.SetCaret(Pos{Line: 0, Col: 5}, false)

	if !b.MoveDown(false) || b.Caret() != (Pos{Line: 1, Col: 2}) {
		t.Errorf("down to short line: %+v", b.Caret())
	}
	if !b.MoveDown(false) || b.Caret() != (Pos{Line: 2, Col: 5}) {
		t.Errorf("down keeps goal column: %+v", b.Caret())
	}
	if b.MoveDown(false) {
		t.Error("down on the last line reported a move")
	}
	if b.Caret() != (Pos{Line: 2, Col: 5}) {
		t.Errorf("down on the last line moved the caret: %+v", b.Caret())
	}

	b.SetCaret(Pos{Line: 0, Col: 1}, false)
	if b.MoveUp(false) {
		t.Error("up on the first line reported a move")
	}
}

func TestHomeEndAndExtend(t *testing.T) {
	b := bufferWith("abc\ndef")
	b.SetCaret(Pos{Line: 1, Col: 1}, false)
	b.MoveEnd(true)
	if got := b.SelectedText(); got != "ef" {
		t.Errorf("shift+end selected %q", got)
	}
	b.MoveHome(true)
	if got := b.SelectedText(); got != "d" {
		t.Errorf("shift+home selected %q", got)
	}
	b.MoveUp(true)
	if got := b.SelectedText(); got != "abc\nd" {
		t.Errorf("shift+up selected %q", got)
	}
}

func TestSelectAll(t *testing.T) {
	b := bufferWith("ab\ncd")
	b.SetCaret(Pos{}, false)
	b.SelectAll()
	if got := b.SelectedText(); got != "ab\ncd" {
		t.Errorf("select all = %q", got)
	}
	if got := b.Caret(); got != (Pos{Line: 1, Col: 2}) {
		t.Errorf("caret after select all = %+v", got)
	}
}

func TestSetCaretClamps(t *testing.T) {
	b := bufferWith("ab\ncd")
	b.SetCaret(Pos{Line: 9, Col: 9}, false)
	if got := b.Caret(); got != (Pos{Line: 1, Col: 2}) {
		t.Errorf("clamped high: %+v", got)
	}
	b.SetCaret(Pos{Line: -1, Col: -1}, false)
	if got := b.Caret(); got != (Pos{}) {
		t.Errorf("clamped low: %+v", got)
	}
}

func TestRuneLimit(t *testing.T) {
	b := New(5, 0)
	b.Insert("ab\ncdef")
	// Line breaks count toward the limit
	checkText(t, b, "ab\ncd", Pos{Line: 1, Col: 2})
	b.Insert("x")
	checkText(t, b, "ab\ncd", Pos{Line: 1, Col: 2})

	// Replacing a selection frees its room first
	b.SelectAll()
	b.Insert("12345678")
	checkText(t, b, "12345", Pos{Col: 5})
}

func TestLineLimit(t *testing.T) {
	b := New(0, 2)
	b.Insert("a\nb\nc\nd")
	// Extra line breaks are dropped, keeping the text
	checkText(t, b, "a\nbcd", Pos{Line: 1, Col: 3})
	b.Insert("\n")
	checkText(t, b, "a\nbcd", Pos{Line: 1, Col: 3})
}

func TestMultiByteRunes(t *testing.T) {
	accent, han := string(rune(0x00e9)), string(rune(0x6f22))
	b := bufferWith(accent + han)
	b.MoveLeft(false)
	b.Backspace()
	checkText(t, b, han, Pos{})
	if got := b.Line(0); got != han {
		t.Errorf("line = %q, want one rune", got)
	}
}
