// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	img "image"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/widgets/textedit"
)

func newTestInput(rows int) *MultiLineInput {
	m := NewMultiLineInput(rows, 300, 0, 0, nil)
	_, h := m.PreferredSize()
	m.SetLocation(img.Rect(0, 0, 300, h))
	return m
}

func TestMultiLineInputPreferredSize(t *testing.T) {
	m := NewMultiLineInput(12, 250, 0, 0, nil)
	w, h := m.PreferredSize()
	want := int(math.Ceil(12*lineHeight())) + style.SmallSpacing*2
	if w != 250 || h != want {
		t.Errorf("PreferredSize = %d x %d, want 250 x %d", w, h, want)
	}
}

func TestMultiLineInputSetText(t *testing.T) {
	m := newTestInput(3)
	m.SetText("160EF100 0000\r\n160C5238 0000")
	if got := m.Text(); got != "160EF100 0000\n160C5238 0000" {
		t.Errorf("Text = %q", got)
	}
}

func TestMultiLineInputCaretEdges(t *testing.T) {
	m := newTestInput(3)
	m.SetText("a\nb")
	m.buf.SetCaret(textedit.Pos{}, false)

	if m.MoveCaretUp() {
		t.Error("up on the first line reported a move")
	}
	if !m.MoveCaretDown() {
		t.Error("down from the first line reported no move")
	}
	if m.MoveCaretDown() {
		t.Error("down on the last line reported a move")
	}
}

func TestMultiLineInputScrollsToCaret(t *testing.T) {
	m := newTestInput(3)
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "line"
	}
	m.SetText(strings.Join(lines, "\n"))
	m.buf.SetCaret(textedit.Pos{}, false)
	m.caretMoved()
	if m.topLine != 0 {
		t.Fatalf("topLine = %d, want 0", m.topLine)
	}

	m.buf.SetCaret(textedit.Pos{Line: 9}, false)
	m.caretMoved()
	visible := m.visibleRows(m.textArea())
	if m.topLine != 10-visible {
		t.Errorf("topLine = %d, want %d to show the last line", m.topLine, 10-visible)
	}

	m.buf.SetCaret(textedit.Pos{Line: 2}, false)
	m.caretMoved()
	if m.topLine != 2 {
		t.Errorf("topLine = %d, want 2 after moving up past the top", m.topLine)
	}
}

func TestMultiLineInputScrollbars(t *testing.T) {
	m := newTestInput(3)
	padded := m.widget.Rect.Inset(style.SmallSpacing)

	m.SetText("short")
	if v, h := m.scrollbarsShown(padded); v || h {
		t.Errorf("short text: scrollbars %v %v, want none", v, h)
	}

	m.SetText(strings.Repeat("W", 200))
	if _, h := m.scrollbarsShown(padded); !h {
		t.Error("long line: no horizontal scrollbar")
	}

	m.SetText(strings.Repeat("x\n", 20))
	if v, _ := m.scrollbarsShown(padded); !v {
		t.Error("many lines: no vertical scrollbar")
	}
}

// linearNearestColumn finds the nearest column by measuring every prefix
// in turn, the reference for nearestColumn
func linearNearestColumn(runes []rune, x float64) int {
	face := *style.FontFace()
	prev := 0.0
	for i := range runes {
		next := text.Advance(string(runes[:i+1]), face)
		if x < (prev+next)/2 {
			return i
		}
		prev = next
	}
	return len(runes)
}

func TestNearestColumnMatchesLinear(t *testing.T) {
	for _, line := range []string{"", "a", "160EF100 0000", "Wi WiW il1 MMM ..", strings.Repeat("W", 120)} {
		runes := []rune(line)
		end := text.Advance(line, *style.FontFace())
		for x := -5.0; x <= end+5; x++ {
			if got, want := nearestColumn(runes, x), linearNearestColumn(runes, x); got != want {
				t.Fatalf("%q at x=%.1f: column %d, want %d", line, x, got, want)
			}
		}
	}
}

func TestMultiLineInputHitTest(t *testing.T) {
	m := newTestInput(3)
	m.SetText("abc\ndef")
	area := m.textArea()

	// Far right of the second line lands at its end
	p := m.hitTest(area.Max.X, area.Min.Y+int(lineHeight()*1.5))
	if p != (textedit.Pos{Line: 1, Col: 3}) {
		t.Errorf("hit = %+v, want line 1 col 3", p)
	}
	// Left of the text and above it clamps to the start
	p = m.hitTest(area.Min.X-50, area.Min.Y-50)
	if p != (textedit.Pos{}) {
		t.Errorf("hit = %+v, want the start", p)
	}
}
