// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package textedit holds the text, caret, and selection of a multi-line
// text field, with the editing operations on them. It has no drawing or
// input handling, so a widget maps keys and mouse positions onto it.
package textedit

import (
	"strings"
	"unicode"
)

// Pos is a position in the text: a line index and a column, both
// counted in runes. A column equal to the line length is the end of
// the line.
type Pos struct {
	Line int
	Col  int
}

// before reports whether p comes before q in the text
func (p Pos) before(q Pos) bool {
	return p.Line < q.Line || (p.Line == q.Line && p.Col < q.Col)
}

// Buffer is the text of a multi-line field with its caret and
// selection. The selection runs from the anchor to the caret and is
// empty when they are equal. There is always at least one line: an
// empty text is one empty line.
type Buffer struct {
	lines    [][]rune
	caret    Pos
	anchor   Pos
	goalCol  int // column Up and Down aim for, kept across short lines
	maxRunes int // total runes including line breaks, 0 for no limit
	maxLines int // 0 for no limit
}

// New creates an empty buffer. maxRunes caps the text length, counting
// each line break as one rune, and maxLines caps the line count. Zero
// means no limit.
func New(maxRunes, maxLines int) *Buffer {
	return &Buffer{lines: [][]rune{{}}, maxRunes: maxRunes, maxLines: maxLines}
}

// SetText replaces the text, keeping what fits the limits, and puts the
// caret at the end with no selection
func (b *Buffer) SetText(s string) {
	b.lines = [][]rune{{}}
	b.caret = Pos{}
	b.anchor = Pos{}
	b.insert(s)
	b.anchor = b.caret
	b.goalCol = b.caret.Col
}

// Text returns the text with lines joined by "\n"
func (b *Buffer) Text() string {
	parts := make([]string, len(b.lines))
	for i, l := range b.lines {
		parts[i] = string(l)
	}
	return strings.Join(parts, "\n")
}

// LineCount returns the number of lines, at least 1
func (b *Buffer) LineCount() int {
	return len(b.lines)
}

// Line returns line i
func (b *Buffer) Line(i int) string {
	return string(b.lines[i])
}

// Caret returns the caret position
func (b *Buffer) Caret() Pos {
	return b.caret
}

// Selection returns the selected range in text order. ok is false when
// nothing is selected.
func (b *Buffer) Selection() (start, end Pos, ok bool) {
	if b.anchor == b.caret {
		return b.caret, b.caret, false
	}
	if b.anchor.before(b.caret) {
		return b.anchor, b.caret, true
	}
	return b.caret, b.anchor, true
}

// SelectedText returns the selected text, or "" when nothing is selected
func (b *Buffer) SelectedText() string {
	start, end, ok := b.Selection()
	if !ok {
		return ""
	}
	if start.Line == end.Line {
		return string(b.lines[start.Line][start.Col:end.Col])
	}
	var sb strings.Builder
	sb.WriteString(string(b.lines[start.Line][start.Col:]))
	for i := start.Line + 1; i < end.Line; i++ {
		sb.WriteByte('\n')
		sb.WriteString(string(b.lines[i]))
	}
	sb.WriteByte('\n')
	sb.WriteString(string(b.lines[end.Line][:end.Col]))
	return sb.String()
}

// SelectAll selects the whole text, leaving the caret at the end
func (b *Buffer) SelectAll() {
	b.anchor = Pos{}
	last := len(b.lines) - 1
	b.caret = Pos{Line: last, Col: len(b.lines[last])}
	b.goalCol = b.caret.Col
}

// SetCaret moves the caret to p, clamped into the text. With extend the
// selection grows from its anchor to p; without, the selection is
// cleared.
func (b *Buffer) SetCaret(p Pos, extend bool) {
	b.moveTo(b.clamp(p), extend)
	b.goalCol = b.caret.Col
}

// Insert replaces the selection with s, or inserts s at the caret.
// "\r\n" and "\r" become line breaks, tabs become spaces, and other
// control characters are dropped. Only the part of s that fits the
// limits is inserted.
func (b *Buffer) Insert(s string) {
	b.deleteSelection()
	b.insert(s)
	b.anchor = b.caret
	b.goalCol = b.caret.Col
}

// Backspace deletes the selection, or the rune before the caret, joining
// the line with the one above at the start of a line
func (b *Buffer) Backspace() {
	if !b.deleteSelection() {
		start := b.caret
		switch {
		case start.Col > 0:
			start.Col--
		case start.Line > 0:
			start.Line--
			start.Col = len(b.lines[start.Line])
		}
		// Nothing to delete at the start of the text
		if start != b.caret {
			b.deleteRange(start, b.caret)
		}
	}
	b.anchor = b.caret
	b.goalCol = b.caret.Col
}

// Delete deletes the selection, or the rune after the caret, joining the
// next line at the end of a line
func (b *Buffer) Delete() {
	if !b.deleteSelection() {
		end := b.caret
		switch {
		case end.Col < len(b.lines[end.Line]):
			end.Col++
		case end.Line < len(b.lines)-1:
			end.Line++
			end.Col = 0
		}
		// Nothing to delete at the end of the text
		if end != b.caret {
			b.deleteRange(b.caret, end)
		}
	}
	b.anchor = b.caret
	b.goalCol = b.caret.Col
}

// MoveLeft moves the caret one rune left, to the end of the line above at
// the start of a line. Without extend a selection collapses to its start
// instead.
func (b *Buffer) MoveLeft(extend bool) {
	if start, _, ok := b.Selection(); ok && !extend {
		b.moveTo(start, false)
	} else {
		p := b.caret
		switch {
		case p.Col > 0:
			p.Col--
		case p.Line > 0:
			p.Line--
			p.Col = len(b.lines[p.Line])
		}
		b.moveTo(p, extend)
	}
	b.goalCol = b.caret.Col
}

// MoveRight moves the caret one rune right, to the start of the next line
// at the end of a line. Without extend a selection collapses to its end
// instead.
func (b *Buffer) MoveRight(extend bool) {
	if _, end, ok := b.Selection(); ok && !extend {
		b.moveTo(end, false)
	} else {
		p := b.caret
		switch {
		case p.Col < len(b.lines[p.Line]):
			p.Col++
		case p.Line < len(b.lines)-1:
			p.Line++
			p.Col = 0
		}
		b.moveTo(p, extend)
	}
	b.goalCol = b.caret.Col
}

// MoveUp moves the caret to the line above, at the column it was last
// placed on or the end of that line if shorter. It reports false, and
// changes nothing, on the first line.
func (b *Buffer) MoveUp(extend bool) bool {
	if b.caret.Line == 0 {
		return false
	}
	b.moveVertical(b.caret.Line-1, extend)
	return true
}

// MoveDown moves the caret to the line below, like MoveUp. It reports
// false, and changes nothing, on the last line.
func (b *Buffer) MoveDown(extend bool) bool {
	if b.caret.Line == len(b.lines)-1 {
		return false
	}
	b.moveVertical(b.caret.Line+1, extend)
	return true
}

// MoveHome moves the caret to the start of its line
func (b *Buffer) MoveHome(extend bool) {
	b.moveTo(Pos{Line: b.caret.Line}, extend)
	b.goalCol = 0
}

// MoveEnd moves the caret to the end of its line
func (b *Buffer) MoveEnd(extend bool) {
	b.moveTo(Pos{Line: b.caret.Line, Col: len(b.lines[b.caret.Line])}, extend)
	b.goalCol = b.caret.Col
}

// moveVertical moves the caret to line, keeping the goal column
func (b *Buffer) moveVertical(line int, extend bool) {
	b.moveTo(Pos{Line: line, Col: min(b.goalCol, len(b.lines[line]))}, extend)
}

// moveTo puts the caret at p, keeping the anchor with extend and moving
// it with the caret otherwise
func (b *Buffer) moveTo(p Pos, extend bool) {
	b.caret = p
	if !extend {
		b.anchor = p
	}
}

// clamp limits p to a position in the text
func (b *Buffer) clamp(p Pos) Pos {
	p.Line = min(max(p.Line, 0), len(b.lines)-1)
	p.Col = min(max(p.Col, 0), len(b.lines[p.Line]))
	return p
}

// runeCount returns the text length in runes, counting line breaks
func (b *Buffer) runeCount() int {
	n := len(b.lines) - 1
	for _, l := range b.lines {
		n += len(l)
	}
	return n
}

// deleteSelection deletes the selected text and reports whether there
// was any
func (b *Buffer) deleteSelection() bool {
	start, end, ok := b.Selection()
	if ok {
		b.deleteRange(start, end)
	}
	return ok
}

// deleteRange removes the text from start to end, start not after end,
// and leaves the caret and anchor at start
func (b *Buffer) deleteRange(start, end Pos) {
	tail := b.lines[end.Line][end.Col:]
	joined := make([]rune, 0, start.Col+len(tail))
	joined = append(joined, b.lines[start.Line][:start.Col]...)
	joined = append(joined, tail...)
	b.lines = append(b.lines[:start.Line+1], b.lines[end.Line+1:]...)
	b.lines[start.Line] = joined
	b.caret = start
	b.anchor = start
}

// insert puts the cleaned text of s at the caret, as much as fits, and
// moves the caret to its end
func (b *Buffer) insert(s string) {
	room := -1
	if b.maxRunes > 0 {
		room = max(b.maxRunes-b.runeCount(), 0)
	}
	lineRoom := -1
	if b.maxLines > 0 {
		lineRoom = max(b.maxLines-len(b.lines), 0)
	}

	var add []rune
	breaks := 0
	s = strings.ReplaceAll(s, "\r\n", "\n")
	for _, r := range s {
		if room == 0 {
			break
		}
		switch {
		case r == '\n' || r == '\r':
			if lineRoom >= 0 && breaks == lineRoom {
				continue
			}
			breaks++
			r = '\n'
		case r == '\t':
			r = ' '
		case unicode.IsControl(r):
			continue
		}
		add = append(add, r)
		if room > 0 {
			room--
		}
	}
	if len(add) == 0 {
		return
	}

	line := b.lines[b.caret.Line]
	head := append([]rune(nil), line[:b.caret.Col]...)
	tail := append([]rune(nil), line[b.caret.Col:]...)
	parts := splitRunes(add)
	parts[0] = append(head, parts[0]...)
	last := len(parts) - 1
	col := len(parts[last])
	parts[last] = append(parts[last], tail...)

	newLines := make([][]rune, 0, len(b.lines)+last)
	newLines = append(newLines, b.lines[:b.caret.Line]...)
	newLines = append(newLines, parts...)
	newLines = append(newLines, b.lines[b.caret.Line+1:]...)
	b.lines = newLines
	b.caret = Pos{Line: b.caret.Line + last, Col: col}
}

// splitRunes splits r at line breaks into one slice per line
func splitRunes(r []rune) [][]rune {
	out := [][]rune{{}}
	for _, c := range r {
		if c == '\n' {
			out = append(out, []rune{})
			continue
		}
		out[len(out)-1] = append(out[len(out)-1], c)
	}
	return out
}
