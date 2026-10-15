// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package widgets

import (
	img "image"
	"math"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/widgets/textedit"
)

// Key repeat timing in ticks: the first repeat after keyRepeatDelay, then
// one every keyRepeatInterval
const (
	keyRepeatDelay    = 30
	keyRepeatInterval = 3
)

// caretBlinkTicks is how long the caret stays shown, then hidden
const caretBlinkTicks = 30

// MultiLineInput is an editable multi-line text field. It shows a fixed
// number of lines and scrolls in both directions to keep the caret in
// view, with a scrollbar on each axis while the text overflows it.
//
// The field handles typing, Enter, Backspace, Delete, Home, End, the
// Ctrl/Cmd+A/C/X/V clipboard shortcuts, and mouse selection itself.
// Arrow keys and gamepad directions are applied by the owner through
// MoveCaret, so they share the UI navigation repeat and can move focus
// out of the field at its edges.
type MultiLineInput struct {
	widget   *widget.Widget
	buf      *textedit.Buffer
	rows     int
	minWidth int
	changed  func(text string)

	focused  bool
	focusMap map[widget.FocusDirection]widget.Focuser
	dragging bool
	blink    int

	topLine int     // first visible line
	scrollX float64 // horizontal scroll in pixels
	text    string  // text at the last change check

	// Pixel width of the longest line, measured for widthText
	width     float64
	widthText string
	widthOK   bool
}

// NewMultiLineInput creates a field showing rows lines, at least minWidth
// wide. maxRunes and maxLines limit the text as in textedit.New. changed,
// when not nil, is called after each edit with the new text.
func NewMultiLineInput(rows, minWidth, maxRunes, maxLines int, changed func(text string), opts ...widget.WidgetOpt) *MultiLineInput {
	m := &MultiLineInput{
		buf:      textedit.New(maxRunes, maxLines),
		rows:     max(rows, 1),
		minWidth: minWidth,
		changed:  changed,
		focusMap: make(map[widget.FocusDirection]widget.Focuser),
	}
	m.widget = widget.NewWidget(append([]widget.WidgetOpt{widget.WidgetOpts.TrackHover(true)}, opts...)...)
	m.widget.MouseButtonPressedEvent.AddHandler(func(args interface{}) {
		if a, ok := args.(*widget.WidgetMouseButtonPressedEventArgs); ok && a.Button == ebiten.MouseButtonLeft {
			m.pressAt(a.OffsetX+m.widget.Rect.Min.X, a.OffsetY+m.widget.Rect.Min.Y)
		}
	})
	return m
}

// Text returns the field's text, lines joined by "\n"
func (m *MultiLineInput) Text() string {
	return m.buf.Text()
}

// SetText replaces the text without calling the changed handler
func (m *MultiLineInput) SetText(s string) {
	m.buf.SetText(s)
	m.text = m.buf.Text()
	m.topLine = 0
	m.scrollX = 0
}

// GetWidget returns the underlying widget
func (m *MultiLineInput) GetWidget() *widget.Widget {
	return m.widget
}

// SetLocation sets the field's rectangle
func (m *MultiLineInput) SetLocation(rect img.Rectangle) {
	m.widget.Rect = rect
}

// PreferredSize returns minWidth by the height of the visible rows plus
// padding
func (m *MultiLineInput) PreferredSize() (int, int) {
	h := int(math.Ceil(float64(m.rows)*lineHeight())) + style.SmallSpacing*2
	w := m.minWidth
	if m.widget.MinWidth > w {
		w = m.widget.MinWidth
	}
	return w, max(h, m.widget.MinHeight)
}

// Validate is part of the ebitenui widget contract
func (m *MultiLineInput) Validate() {}

// Focus sets whether the field has keyboard focus and tells the UI
func (m *MultiLineInput) Focus(focused bool) {
	m.widget.FireFocusEvent(m, focused, img.Point{X: -1, Y: -1})
	m.focused = focused
	m.dragging = false
	m.blink = 0
}

// IsFocused reports whether the field has keyboard focus
func (m *MultiLineInput) IsFocused() bool {
	return m.focused
}

// TabOrder is part of widget.Focuser
func (m *MultiLineInput) TabOrder() int {
	return 0
}

// GetFocus is part of widget.Focuser
func (m *MultiLineInput) GetFocus(direction widget.FocusDirection) widget.Focuser {
	return m.focusMap[direction]
}

// AddFocus is part of widget.Focuser
func (m *MultiLineInput) AddFocus(direction widget.FocusDirection, focus widget.Focuser) {
	m.focusMap[direction] = focus
}

// MoveCaretUp moves the caret up one line, extending the selection while
// Shift is held. It reports false on the first line, where focus should
// leave the field instead.
func (m *MultiLineInput) MoveCaretUp() bool {
	moved := m.buf.MoveUp(shiftPressed())
	m.caretMoved()
	return moved
}

// MoveCaretDown moves the caret down one line like MoveCaretUp. It
// reports false on the last line.
func (m *MultiLineInput) MoveCaretDown() bool {
	moved := m.buf.MoveDown(shiftPressed())
	m.caretMoved()
	return moved
}

// MoveCaretLeft moves the caret left one character, extending the
// selection while Shift is held
func (m *MultiLineInput) MoveCaretLeft() {
	m.buf.MoveLeft(shiftPressed())
	m.caretMoved()
}

// MoveCaretRight moves the caret right one character, extending the
// selection while Shift is held
func (m *MultiLineInput) MoveCaretRight() {
	m.buf.MoveRight(shiftPressed())
	m.caretMoved()
}

// Update handles mouse dragging and, while focused, editing keys
func (m *MultiLineInput) Update(updObj *widget.UpdateObject) {
	m.widget.Update(updObj)

	if m.dragging {
		if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
			mx, my := ebiten.CursorPosition()
			m.buf.SetCaret(m.hitTest(mx, my), true)
			m.caretMoved()
		} else {
			m.dragging = false
		}
	}

	if m.focused {
		m.handleKeys()
		m.blink = (m.blink + 1) % (caretBlinkTicks * 2)
		if _, wy := ebiten.Wheel(); wy != 0 && m.hovered() {
			m.scrollLines(-int(math.Round(wy)))
		}
	}

	if t := m.buf.Text(); t != m.text {
		m.text = t
		if m.changed != nil {
			m.changed(t)
		}
	}
}

// handleKeys applies the editing keys held this tick
func (m *MultiLineInput) handleKeys() {
	if ShortcutModifierPressed() {
		switch {
		case inpututil.IsKeyJustPressed(ebiten.KeyA):
			m.buf.SelectAll()
			m.caretMoved()
		case inpututil.IsKeyJustPressed(ebiten.KeyC):
			if s := m.buf.SelectedText(); s != "" {
				ClipboardWrite(s)
			}
		case inpututil.IsKeyJustPressed(ebiten.KeyX):
			if s := m.buf.SelectedText(); s != "" && ClipboardWrite(s) {
				m.buf.Insert("")
				m.caretMoved()
			}
		case inpututil.IsKeyJustPressed(ebiten.KeyV):
			if s, ok := ClipboardRead(); ok {
				m.buf.Insert(s)
				m.caretMoved()
			}
		}
		return
	}

	if chars := ebiten.AppendInputChars(nil); len(chars) > 0 {
		m.buf.Insert(string(chars))
		m.caretMoved()
	}
	switch {
	case keyRepeating(ebiten.KeyEnter) || keyRepeating(ebiten.KeyNumpadEnter):
		m.buf.Insert("\n")
	case keyRepeating(ebiten.KeyBackspace):
		m.buf.Backspace()
	case keyRepeating(ebiten.KeyDelete):
		m.buf.Delete()
	case inpututil.IsKeyJustPressed(ebiten.KeyHome):
		m.buf.MoveHome(shiftPressed())
	case inpututil.IsKeyJustPressed(ebiten.KeyEnd):
		m.buf.MoveEnd(shiftPressed())
	default:
		return
	}
	m.caretMoved()
}

// keyRepeating reports whether key was just pressed or has been held long
// enough to repeat this tick
func keyRepeating(key ebiten.Key) bool {
	d := inpututil.KeyPressDuration(key)
	return d == 1 || (d > keyRepeatDelay && (d-keyRepeatDelay)%keyRepeatInterval == 0)
}

// shiftPressed reports whether either Shift key is held
func shiftPressed() bool {
	return ebiten.IsKeyPressed(ebiten.KeyShift) ||
		ebiten.IsKeyPressed(ebiten.KeyShiftLeft) ||
		ebiten.IsKeyPressed(ebiten.KeyShiftRight)
}

// pressAt focuses the field and places the caret at a screen position,
// extending the selection while Shift is held, and starts a drag
func (m *MultiLineInput) pressAt(x, y int) {
	if !m.focused {
		m.Focus(true)
	}
	m.buf.SetCaret(m.hitTest(x, y), shiftPressed())
	m.dragging = true
	m.caretMoved()
}

// caretMoved restarts the caret blink and scrolls the caret into view
func (m *MultiLineInput) caretMoved() {
	m.blink = 0
	area := m.textArea()
	visible := m.visibleRows(area)
	caret := m.buf.Caret()
	if caret.Line < m.topLine {
		m.topLine = caret.Line
	} else if caret.Line >= m.topLine+visible {
		m.topLine = caret.Line - visible + 1
	}
	m.clampScroll(area)

	x := text.Advance(string([]rune(m.buf.Line(caret.Line))[:caret.Col]), *style.FontFace())
	w := float64(area.Dx())
	if x < m.scrollX {
		m.scrollX = x
	} else if x > m.scrollX+w-caretWidth() {
		m.scrollX = x - w + caretWidth()
	}
	m.clampScroll(area)
}

// scrollLines scrolls the view by n lines without moving the caret
func (m *MultiLineInput) scrollLines(n int) {
	m.topLine += n
	m.clampScroll(m.textArea())
}

// clampScroll keeps the scroll positions within the text
func (m *MultiLineInput) clampScroll(area img.Rectangle) {
	m.topLine = min(max(m.topLine, 0), max(m.buf.LineCount()-m.visibleRows(area), 0))
	maxX := math.Max(m.contentWidth()-float64(area.Dx())+caretWidth(), 0)
	m.scrollX = math.Min(math.Max(m.scrollX, 0), maxX)
}

// contentWidth returns the pixel width of the longest line. It is
// measured again only when the text has changed.
func (m *MultiLineInput) contentWidth() float64 {
	t := m.buf.Text()
	if m.widthOK && t == m.widthText {
		return m.width
	}
	w := 0.0
	for i := 0; i < m.buf.LineCount(); i++ {
		w = math.Max(w, text.Advance(m.buf.Line(i), *style.FontFace()))
	}
	m.width, m.widthText, m.widthOK = w, t, true
	return w
}

// textArea returns the rectangle text is drawn in: the field less its
// padding and the scrollbars that are showing
func (m *MultiLineInput) textArea() img.Rectangle {
	r := m.widget.Rect.Inset(style.SmallSpacing)
	vBar, hBar := m.scrollbarsShown(r)
	if vBar {
		r.Max.X -= scrollbarThickness()
	}
	if hBar {
		r.Max.Y -= scrollbarThickness()
	}
	return r
}

// scrollbarsShown reports which scrollbars the text needs inside the
// padded rectangle r. A horizontal bar takes height, which can make the
// vertical one needed too, and the reverse.
func (m *MultiLineInput) scrollbarsShown(r img.Rectangle) (vBar, hBar bool) {
	w, h := r.Dx(), r.Dy()
	cw := m.contentWidth() + caretWidth()
	for range 2 {
		vBar = m.buf.LineCount() > rowsIn(h)
		if vBar {
			w = r.Dx() - scrollbarThickness()
		}
		hBar = cw > float64(w)
		if hBar {
			h = r.Dy() - scrollbarThickness()
		}
	}
	return vBar, hBar
}

// visibleRows returns how many whole lines fit in area, at least 1
func (m *MultiLineInput) visibleRows(area img.Rectangle) int {
	return rowsIn(area.Dy())
}

// rowsIn returns how many whole lines fit in height h, at least 1
func rowsIn(h int) int {
	return max(int(float64(h)/lineHeight()), 1)
}

// hitTest maps a screen position to the nearest text position
func (m *MultiLineInput) hitTest(x, y int) textedit.Pos {
	area := m.textArea()
	line := m.topLine + int(math.Floor(float64(y-area.Min.Y)/lineHeight()))
	line = min(max(line, 0), m.buf.LineCount()-1)

	target := float64(x-area.Min.X) + m.scrollX
	return textedit.Pos{Line: line, Col: nearestColumn([]rune(m.buf.Line(line)), target)}
}

// nearestColumn returns the column of runes whose boundary is nearest to
// pixel offset x from the start of the line. Prefix widths never shrink as
// the prefix grows, so the column at or left of x is found by binary
// search, then the nearer of it and the next is taken.
func nearestColumn(runes []rune, x float64) int {
	face := *style.FontFace()
	adv := func(col int) float64 {
		return text.Advance(string(runes[:col]), face)
	}

	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if adv(mid) <= x {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == len(runes) {
		return lo
	}
	if left, right := adv(lo), adv(lo+1); x >= (left+right)/2 {
		return lo + 1
	}
	return lo
}

// hovered reports whether the cursor is over the field
func (m *MultiLineInput) hovered() bool {
	return img.Pt(ebiten.CursorPosition()).In(m.widget.Rect)
}

// Render draws the background, the visible text with its selection, the
// caret, and the scrollbars
func (m *MultiLineInput) Render(screen *ebiten.Image) {
	r := m.widget.Rect
	if r.Empty() {
		return
	}
	vector.DrawFilledRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), style.Surface, false)

	area := m.textArea()
	m.clampScroll(area)
	clip, ok := screen.SubImage(area).(*ebiten.Image)
	if !ok || area.Empty() {
		return
	}

	face := *style.FontFace()
	lineH := lineHeight()
	start, end, hasSel := m.buf.Selection()
	caret := m.buf.Caret()
	spaceW := text.Advance(" ", face)
	last := min(m.topLine+m.visibleRows(area)+1, m.buf.LineCount())

	for i := m.topLine; i < last; i++ {
		line := m.buf.Line(i)
		runes := []rune(line)
		x := float64(area.Min.X) - m.scrollX
		y := float64(area.Min.Y) + float64(i-m.topLine)*lineH

		if hasSel && i >= start.Line && i <= end.Line {
			c0, c1 := 0, len(runes)
			if i == start.Line {
				c0 = start.Col
			}
			if i == end.Line {
				c1 = end.Col
			}
			x0 := x + text.Advance(string(runes[:c0]), face)
			x1 := x + text.Advance(string(runes[:c1]), face)
			if i < end.Line {
				x1 += spaceW // the selected line break
			}
			vector.DrawFilledRect(clip, float32(x0), float32(y), float32(x1-x0), float32(lineH), style.Primary, false)
		}

		op := &text.DrawOptions{}
		op.GeoM.Translate(x, y)
		op.ColorScale.ScaleWithColor(style.Text)
		text.Draw(clip, line, face, op)

		if m.focused && i == caret.Line && m.blink < caretBlinkTicks {
			cx := x + text.Advance(string(runes[:caret.Col]), face)
			vector.DrawFilledRect(clip, float32(cx), float32(y), float32(caretWidth()), float32(lineH), style.Text, false)
		}
	}

	m.renderScrollbars(screen, area)
}

// renderScrollbars draws a track and thumb along each axis the text
// overflows
func (m *MultiLineInput) renderScrollbars(screen *ebiten.Image, area img.Rectangle) {
	t := float32(scrollbarThickness())
	vBar, hBar := m.scrollbarsShown(m.widget.Rect.Inset(style.SmallSpacing))
	if vBar {
		total := float32(m.buf.LineCount())
		visible := float32(m.visibleRows(area))
		x, y, h := float32(area.Max.X), float32(area.Min.Y), float32(area.Dy())
		vector.DrawFilledRect(screen, x, y, t, h, style.Border, false)
		vector.DrawFilledRect(screen, x, y+h*float32(m.topLine)/total, t, h*visible/total, style.Primary, false)
	}
	if hBar {
		total := float32(m.contentWidth() + caretWidth())
		visible := float32(area.Dx())
		x, y, w := float32(area.Min.X), float32(area.Max.Y), float32(area.Dx())
		vector.DrawFilledRect(screen, x, y, w, t, style.Border, false)
		vector.DrawFilledRect(screen, x+w*float32(m.scrollX)/total, y, w*visible/total, t, style.Primary, false)
	}
}

// lineHeight returns the height of one line of text
func lineHeight() float64 {
	m := (*style.FontFace()).Metrics()
	return m.HAscent + m.HDescent + m.HLineGap
}

// caretWidth returns the caret's width
func caretWidth() float64 {
	return float64(max(style.Px(2), 1))
}

// scrollbarThickness returns the width of the vertical scrollbar and the
// height of the horizontal one
func scrollbarThickness() int {
	return max(style.Px(8), 4)
}
