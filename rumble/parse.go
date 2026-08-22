// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package rumble

import (
	"fmt"
	"strconv"
	"strings"
)

const maxDurationMs = 1 << 31

var keywords = map[string]bool{
	"watch": true, "const": true, "def": true, "event": true,
	"on": true, "while": true, "byte": true, "word": true, "long": true,
	"mask": true, "little": true, "big": true, "pulse": true, "pattern": true,
	"hold": true, "off": true, "level": true, "priority": true,
	"clip": true, "cooldown": true, "player": true, "scale": true,
	"all": true, "and": true,
	"or": true, "in": true, "not": true, "by": true, "for": true, "from": true,
	"to": true, "changed": true, "unchanged": true, "increased": true,
	"decreased": true, "stride": true, "count": true, "ptr": true,
	"offset": true, "key": true, "field": true, "at": true, "least": true,
	"most": true, "signed": true, "abs": true, "dampen": true,
	"amplify": true, "bcd": true, "held": true, "frames": true,
}

// metaTerminator closes the metadata block. The block and its values
// are read ahead of the lexer, which rejects the "-" they contain.
const metaTerminator = "---"

// metaKeys are the statements the metadata block accepts. They are
// recognized in statement position at the head of a file only, so they
// stay out of keywords and remain legal watch, const, def, and event
// names.
var metaKeys = map[string]bool{
	"game": true, "gameid": true, "system": true, "revision": true,
}

// metaKey returns the metadata key a statement declares. A metadata
// statement is "<key>: <value>", so text without a colon, or with an
// unrecognized key, is not one.
func metaKey(text string) (string, string, bool) {
	key, val, ok := strings.Cut(text, ":")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if !metaKeys[key] {
		return "", "", false
	}
	return key, strings.TrimSpace(val), true
}

type segment struct {
	text string
	line int
}

// statement is one logical statement: a column-0 line plus any
// whitespace-indented continuation lines.
type statement struct {
	segs []segment
	line int
	cont bool // started with whitespace and had no statement to continue
}

func splitStatements(src []byte) []statement {
	var stmts []statement
	for idx, raw := range strings.Split(string(src), "\n") {
		text := raw
		if i := strings.IndexByte(text, '#'); i >= 0 {
			text = text[:i]
		}
		text = strings.TrimRight(text, " \t\r")
		if strings.TrimSpace(text) == "" {
			continue
		}
		seg := segment{text: text, line: idx + 1}
		if text[0] == ' ' || text[0] == '\t' {
			if len(stmts) > 0 && !stmts[len(stmts)-1].cont {
				last := &stmts[len(stmts)-1]
				last.segs = append(last.segs, seg)
				continue
			}
			stmts = append(stmts, statement{segs: []segment{seg}, line: idx + 1, cont: true})
			continue
		}
		stmts = append(stmts, statement{segs: []segment{seg}, line: idx + 1})
	}
	return stmts
}

func lexStatement(st statement) ([]token, error) {
	var toks []token
	for _, seg := range st.segs {
		segToks, err := lexLine(seg.text, seg.line)
		if err != nil {
			return nil, err
		}
		toks = append(toks, segToks...)
	}
	return toks, nil
}

// findMetaTerminator returns the index of the statement closing the
// metadata block, or -1 when the file has no block. The scan stops at
// the first statement that is neither a metadata statement nor the
// terminator, so a stray "---" further down the file cannot swallow the
// rules above it.
func findMetaTerminator(stmts []statement) int {
	for i, st := range stmts {
		if st.cont {
			return -1
		}
		if strings.TrimSpace(st.segs[0].text) == metaTerminator {
			return i
		}
		if _, _, ok := metaKey(st.segs[0].text); !ok {
			return -1
		}
	}
	return -1
}

// parseMetadata parses the statements making up a metadata block, which
// may be empty. The terminator is the caller's to consume.
func parseMetadata(stmts []statement) (Metadata, error) {
	md := Metadata{Revision: 1}
	seen := map[string]bool{}
	for _, st := range stmts {
		if len(st.segs) > 1 {
			return Metadata{}, fmt.Errorf("line %d: metadata statement does not continue", st.segs[1].line)
		}
		key, val, ok := metaKey(st.segs[0].text)
		if !ok {
			return Metadata{}, fmt.Errorf("line %d: %q is not a metadata statement", st.line, st.segs[0].text)
		}
		if seen[key] {
			return Metadata{}, fmt.Errorf("line %d: duplicate %s statement", st.line, key)
		}
		seen[key] = true
		if val == "" {
			return Metadata{}, fmt.Errorf("line %d: %s has an empty value", st.line, key)
		}
		switch key {
		case "game":
			md.Game = val
		case "gameid":
			md.GameID = val
		case "system":
			md.System = val
		case "revision":
			n, err := strconv.Atoi(val)
			if err != nil {
				return Metadata{}, fmt.Errorf("line %d: revision %q is not an integer", st.line, val)
			}
			if n < 1 {
				return Metadata{}, fmt.Errorf("line %d: revision must be 1 or greater", st.line)
			}
			md.Revision = n
		}
	}
	return md, nil
}

// checkOutsideMeta reports a metadata statement or a terminator found
// in the body of a file, where neither belongs.
func checkOutsideMeta(st statement, first string) error {
	if strings.TrimSpace(st.segs[0].text) == metaTerminator {
		return fmt.Errorf("line %d: unexpected %q, a file has one metadata block and it opens the file",
			st.line, metaTerminator)
	}
	if first == metaTerminator {
		return fmt.Errorf("line %d: the %s terminator must be alone on its line", st.line, metaTerminator)
	}
	if key, _, ok := metaKey(st.segs[0].text); ok {
		return fmt.Errorf("line %d: metadata statement %q must be inside a metadata block ending with %q",
			st.line, key, metaTerminator)
	}
	if metaKeys[first] {
		return fmt.Errorf("line %d: metadata statement %q needs a colon after the key", st.line, first)
	}
	return nil
}

// parseSource parses a rumble file into its structured form. It performs
// no validation beyond syntax. Name resolution and range checks are the
// validator's responsibility.
func parseSource(src []byte) (*Ruleset, error) {
	stmts := splitStatements(src)

	// Without a terminator the file has no block, and the loop below
	// reports any metadata statement it finds.
	var block []statement
	body := stmts
	if end := findMetaTerminator(stmts); end >= 0 {
		if len(stmts[end].segs) > 1 {
			return nil, fmt.Errorf("line %d: the %s terminator does not continue", stmts[end].segs[1].line, metaTerminator)
		}
		block, body = stmts[:end], stmts[end+1:]
	}
	md, err := parseMetadata(block)
	if err != nil {
		return nil, err
	}
	rs := &Ruleset{Metadata: md}

	// consts resolves const names to their values as operands are
	// parsed. It is built in statement order, so a const must be
	// declared before use.
	consts := map[string]uint32{}
	for _, st := range body {
		if st.cont {
			return nil, fmt.Errorf("line %d: continuation line without a statement", st.line)
		}
		fields := strings.Fields(st.segs[0].text)
		if err := checkOutsideMeta(st, fields[0]); err != nil {
			return nil, err
		}

		toks, err := lexStatement(st)
		if err != nil {
			return nil, err
		}
		p := &parser{toks: toks, lastLine: st.line, consts: consts}
		switch fields[0] {
		case "watch":
			v, err := p.parseWatch()
			if err != nil {
				return nil, err
			}
			rs.Watches = append(rs.Watches, v)
		case "const":
			c, err := p.parseConst()
			if err != nil {
				return nil, err
			}
			consts[c.Name] = c.Value
			rs.Consts = append(rs.Consts, c)
		case "def":
			d, err := p.parseDef()
			if err != nil {
				return nil, err
			}
			rs.Defs = append(rs.Defs, d)
		case "event":
			ev, err := p.parseEvent()
			if err != nil {
				return nil, err
			}
			rs.Events = append(rs.Events, ev)
		case "on":
			r, err := p.parseRule()
			if err != nil {
				return nil, err
			}
			rs.Rules = append(rs.Rules, r)
		default:
			return nil, fmt.Errorf("line %d: unknown statement %q, which a newer engine version may define", st.line, fields[0])
		}
	}
	return rs, nil
}

// maxExprDepth bounds parenthesis nesting so a hostile file cannot
// drive the recursive parser arbitrarily deep.
const maxExprDepth = 64

type parser struct {
	toks     []token
	pos      int
	lastLine int
	depth    int
	consts   map[string]uint32
}

func (p *parser) atEnd() bool {
	return p.pos >= len(p.toks)
}

func (p *parser) peek() *token {
	if p.atEnd() {
		return nil
	}
	return &p.toks[p.pos]
}

func (p *parser) errLine() int {
	if p.pos < len(p.toks) {
		return p.toks[p.pos].line
	}
	if len(p.toks) > 0 {
		return p.toks[len(p.toks)-1].line
	}
	return p.lastLine
}

func (p *parser) take(what string) (*token, error) {
	if p.atEnd() {
		return nil, fmt.Errorf("line %d: expected %s at end of statement", p.errLine(), what)
	}
	t := &p.toks[p.pos]
	p.pos++
	return t, nil
}

func (p *parser) expect(kind tokenKind, what string) (*token, error) {
	t, err := p.take(what)
	if err != nil {
		return nil, err
	}
	if t.kind != kind {
		return nil, fmt.Errorf("line %d: expected %s, found %q", t.line, what, t.text)
	}
	return t, nil
}

func (p *parser) acceptIdent(word string) bool {
	t := p.peek()
	if t != nil && t.kind == tokIdent && t.text == word {
		p.pos++
		return true
	}
	return false
}

func (p *parser) takeUint32(what string) (uint32, int, error) {
	t, err := p.take(what)
	if err != nil {
		return 0, 0, err
	}
	if t.kind != tokInt || t.neg {
		return 0, 0, fmt.Errorf("line %d: expected %s, found %q", t.line, what, t.text)
	}
	if t.ival > 0xFFFFFFFF {
		return 0, 0, fmt.Errorf("line %d: %s %s out of range", t.line, what, t.text)
	}
	return uint32(t.ival), t.line, nil
}

// takeInt32 takes an integer that may carry a minus sign. A negative
// literal folds to its 32-bit two's-complement pattern, which is how
// operands, consts, and scale ranges are stored.
func (p *parser) takeInt32(what string) (uint32, int, error) {
	t, err := p.take(what)
	if err != nil {
		return 0, 0, err
	}
	if t.kind != tokInt {
		return 0, 0, fmt.Errorf("line %d: expected %s, found %q", t.line, what, t.text)
	}
	return foldInt32(t, what)
}

// takePriority takes a rule's priority: a signed integer inside the
// range a file may write, the values outside it being reserved.
func (p *parser) takePriority() (int, error) {
	t, err := p.take("priority value")
	if err != nil {
		return 0, err
	}
	if t.kind != tokInt {
		return 0, fmt.Errorf("line %d: expected priority value, found %q", t.line, t.text)
	}
	limit := uint64(MaxPriority)
	if t.neg {
		limit = uint64(-MinPriority)
	}
	if t.ival > limit {
		return 0, fmt.Errorf("line %d: priority %s must be %d to %d", t.line, t.text, MinPriority, MaxPriority)
	}
	if t.neg {
		return -int(t.ival), nil
	}
	return int(t.ival), nil
}

// foldInt32 folds an integer token to its uint32 pattern: the value
// itself, or the two's complement of a negative literal's magnitude.
func foldInt32(t *token, what string) (uint32, int, error) {
	if t.neg {
		if t.ival > 0x80000000 {
			return 0, 0, fmt.Errorf("line %d: %s %s out of range", t.line, what, t.text)
		}
		return uint32(-int64(t.ival)), t.line, nil
	}
	if t.ival > 0xFFFFFFFF {
		return 0, 0, fmt.Errorf("line %d: %s %s out of range", t.line, what, t.text)
	}
	return uint32(t.ival), t.line, nil
}

// takeOperand takes a numeric condition operand: an integer literal,
// possibly negative, or a const name that resolves to one. Consts are
// folded to their value here, so the rest of the parser and the engine
// see only literals.
func (p *parser) takeOperand(what string) (uint32, int, error) {
	t, err := p.take(what)
	if err != nil {
		return 0, 0, err
	}
	switch {
	case t.kind == tokInt:
		return foldInt32(t, what)
	case t.kind == tokIdent && !keywords[t.text]:
		v, ok := p.consts[t.text]
		if !ok {
			return 0, 0, fmt.Errorf("line %d: unknown constant %q", t.line, t.text)
		}
		return v, t.line, nil
	default:
		return 0, 0, fmt.Errorf("line %d: expected %s, found %q", t.line, what, t.text)
	}
}

// nextIsNegInt reports whether the next token is an integer literal
// written with a minus sign. Callers record it before folding erases
// the difference between a negative literal and a large hex value.
func (p *parser) nextIsNegInt() bool {
	t := p.peek()
	return t != nil && t.kind == tokInt && t.neg
}

// takeMagnitude takes an operand that is a size rather than a value,
// such as a delta, where a minus sign has no meaning.
func (p *parser) takeMagnitude(what string) (uint32, int, error) {
	if t := p.peek(); t != nil && t.kind == tokInt && t.neg {
		return 0, 0, fmt.Errorf("line %d: %s must be non-negative, found %q", t.line, what, t.text)
	}
	return p.takeOperand(what)
}

func (p *parser) takeDuration(what string) (int, error) {
	t, err := p.take(what)
	if err != nil {
		return 0, err
	}
	if t.kind != tokDuration || t.neg {
		return 0, fmt.Errorf("line %d: expected %s, found %q", t.line, what, t.text)
	}
	if t.ival >= maxDurationMs {
		return 0, fmt.Errorf("line %d: %s %s out of range", t.line, what, t.text)
	}
	return int(t.ival), nil
}

// takePercent takes an integer with a "%" suffix, the form of a dampen
// or amplify amount. Range checks are the validator's responsibility.
func (p *parser) takePercent(what string) (int, error) {
	t, err := p.take(what)
	if err != nil {
		return 0, err
	}
	if t.kind != tokPercent || t.neg {
		return 0, fmt.Errorf("line %d: expected %s, found %q", t.line, what, t.text)
	}
	if t.ival > 0xFFFF {
		return 0, fmt.Errorf("line %d: %s %s out of range", t.line, what, t.text)
	}
	return int(t.ival), nil
}

// takeDecimal takes a non-hex numeric token: an integer or a number with
// a fraction. Used for intensities and scale multipliers.
func (p *parser) takeDecimal(what string) (float64, error) {
	t, err := p.take(what)
	if err != nil {
		return 0, err
	}
	if t.neg {
		return 0, fmt.Errorf("line %d: %s must be non-negative, found %q", t.line, what, t.text)
	}
	switch t.kind {
	case tokNumber:
		return t.fval, nil
	case tokInt:
		if t.hex {
			return 0, fmt.Errorf("line %d: %s must be decimal, found %q", t.line, what, t.text)
		}
		return t.fval, nil
	default:
		return 0, fmt.Errorf("line %d: expected %s, found %q", t.line, what, t.text)
	}
}

func (p *parser) parseWatch() (Watch, error) {
	kw, err := p.take("watch")
	if err != nil {
		return Watch{}, err
	}
	v := Watch{Line: kw.line}

	name, err := p.expect(tokIdent, "watch name")
	if err != nil {
		return Watch{}, err
	}
	v.Name = name.text

	width, err := p.expect(tokIdent, "width")
	if err != nil {
		return Watch{}, err
	}
	switch width.text {
	case "byte":
		v.Width = 8
	case "word":
		v.Width = 16
	case "long":
		v.Width = 32
	default:
		return Watch{}, fmt.Errorf("line %d: width must be byte, word, or long, found %q", width.line, width.text)
	}

	v.Pointer = p.acceptIdent("ptr")
	addr, _, err := p.takeUint32("address")
	if err != nil {
		return Watch{}, err
	}
	v.Address = addr

	if v.Pointer {
		if p.acceptIdent("offset") {
			o, _, err := p.takeUint32("offset")
			if err != nil {
				return Watch{}, err
			}
			v.Offset = o
		}
	} else if p.acceptIdent("stride") {
		s, _, err := p.takeUint32("stride")
		if err != nil {
			return Watch{}, err
		}
		if !p.acceptIdent("count") {
			return Watch{}, fmt.Errorf("line %d: expected \"count\" after stride", p.errLine())
		}
		c, line, err := p.takeUint32("count")
		if err != nil {
			return Watch{}, err
		}
		if c > 0xFFFF {
			return Watch{}, fmt.Errorf("line %d: count %d out of range", line, c)
		}
		v.Stride = s
		v.Count = int(c)
		if p.acceptIdent("key") {
			if err := p.parseKeyed(&v); err != nil {
				return Watch{}, err
			}
		}
	}
	if p.acceptIdent("mask") {
		m, _, err := p.takeUint32("mask")
		if err != nil {
			return Watch{}, err
		}
		v.Mask = m
		v.HasMask = true
	}
	if p.acceptIdent("signed") {
		v.Signed = true
	}
	if p.acceptIdent("abs") {
		v.Abs = true
	}
	if p.acceptIdent("bcd") {
		v.BCD = true
	}
	if p.acceptIdent("little") {
		v.Endian = EndianLittle
	} else if p.acceptIdent("big") {
		v.Endian = EndianBig
	}
	if t := p.peek(); t != nil {
		return Watch{}, fmt.Errorf("line %d: unexpected %q after watch declaration", t.line, t.text)
	}
	return v, nil
}

// parseKeyed parses the key/field tail of a keyed slot watch, after
// the "key" keyword is consumed.
func (p *parser) parseKeyed(v *Watch) error {
	ko, _, err := p.takeUint32("key offset")
	if err != nil {
		return err
	}
	kv, _, err := p.takeOperand("key value")
	if err != nil {
		return err
	}
	if !p.acceptIdent("field") {
		return fmt.Errorf("line %d: expected \"field\" after the key value", p.errLine())
	}
	fo, _, err := p.takeUint32("field offset")
	if err != nil {
		return err
	}
	v.HasKey = true
	v.KeyOffset = ko
	v.KeyValue = kv
	v.FieldOffset = fo
	if p.acceptIdent("ptr") {
		v.FieldPtr = true
		if p.acceptIdent("offset") {
			o, _, err := p.takeUint32("offset")
			if err != nil {
				return err
			}
			v.Offset = o
		}
	}
	return nil
}

func (p *parser) parseConst() (Const, error) {
	kw, err := p.take("const")
	if err != nil {
		return Const{}, err
	}
	name, err := p.expect(tokIdent, "const name")
	if err != nil {
		return Const{}, err
	}
	v, _, err := p.takeInt32("const value")
	if err != nil {
		return Const{}, err
	}
	if t := p.peek(); t != nil {
		return Const{}, fmt.Errorf("line %d: unexpected %q in const", t.line, t.text)
	}
	return Const{Name: name.text, Value: v, Line: kw.line}, nil
}

func (p *parser) parseDef() (Def, error) {
	kw, err := p.take("def")
	if err != nil {
		return Def{}, err
	}
	name, err := p.expect(tokIdent, "def name")
	if err != nil {
		return Def{}, err
	}
	expr, err := p.parseExpr()
	if err != nil {
		return Def{}, err
	}
	if t := p.peek(); t != nil {
		return Def{}, fmt.Errorf("line %d: unexpected %q in def", t.line, t.text)
	}
	return Def{Name: name.text, Expr: expr, Line: kw.line}, nil
}

func (p *parser) parseEvent() (Event, error) {
	kw, err := p.take("event")
	if err != nil {
		return Event{}, err
	}
	name, err := p.expect(tokIdent, "event name")
	if err != nil {
		return Event{}, err
	}
	expr, err := p.parseExpr()
	if err != nil {
		return Event{}, err
	}
	if p.acceptIdent("held") {
		if !p.acceptIdent("for") {
			return Event{}, fmt.Errorf("line %d: expected \"for\" after \"held\"", p.errLine())
		}
		n, line, err := p.takeUint32("frame count")
		if err != nil {
			return Event{}, err
		}
		if n >= 1<<31 {
			return Event{}, fmt.Errorf("line %d: frame count %d out of range", line, n)
		}
		if !p.acceptIdent("frames") {
			return Event{}, fmt.Errorf("line %d: expected \"frames\" after the frame count", p.errLine())
		}
		if t := p.peek(); t != nil {
			return Event{}, fmt.Errorf("line %d: unexpected %q after event declaration", t.line, t.text)
		}
		return Event{Name: name.text, Held: true, Cond: expr, HeldFrames: int(n), Line: kw.line}, nil
	}
	pc, ok := expr.(*PrevCond)
	if !ok {
		return Event{}, fmt.Errorf("line %d: event trigger must be a previous-frame condition", kw.line)
	}
	if !p.acceptIdent("for") {
		return Event{}, fmt.Errorf("line %d: expected \"for\" after event trigger", p.errLine())
	}
	d, err := p.takeDuration("event duration")
	if err != nil {
		return Event{}, err
	}
	if t := p.peek(); t != nil {
		return Event{}, fmt.Errorf("line %d: unexpected %q after event declaration", t.line, t.text)
	}
	return Event{Name: name.text, Trigger: *pc, DurationMs: d, Line: kw.line}, nil
}

func (p *parser) parseRule() (Rule, error) {
	kw, err := p.take("on")
	if err != nil {
		return Rule{}, err
	}
	r := Rule{Player: 1, Line: kw.line}

	r.On, err = p.parseExpr()
	if err != nil {
		return Rule{}, err
	}
	if p.acceptIdent("while") {
		r.While, err = p.parseExpr()
		if err != nil {
			return Rule{}, err
		}
	}
	if _, err := p.expect(tokColon, `":"`); err != nil {
		return Rule{}, err
	}
	r.Effect, err = p.parseEffect()
	if err != nil {
		return Rule{}, err
	}
	if err := p.parseModifiers(&r); err != nil {
		return Rule{}, err
	}
	return r, nil
}

func (p *parser) parseExpr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.acceptIdent("or") {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: OpOr, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (Expr, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	for p.acceptIdent("and") {
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: OpAnd, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseTerm() (Expr, error) {
	t := p.peek()
	if t == nil {
		return nil, fmt.Errorf("line %d: expected condition at end of statement", p.errLine())
	}
	if t.kind == tokLParen {
		p.pos++
		p.depth++
		if p.depth > maxExprDepth {
			return nil, fmt.Errorf("line %d: expression is nested too deeply", t.line)
		}
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokRParen, `")"`); err != nil {
			return nil, err
		}
		p.depth--
		return expr, nil
	}
	// "not" negates a def or event name only: comparisons have
	// complement operators, and a compound group is negated by naming
	// it as a def.
	if t.kind == tokIdent && t.text == "not" {
		p.pos++
		nt := p.peek()
		if nt == nil || nt.kind != tokIdent || keywords[nt.text] {
			return nil, fmt.Errorf("line %d: expected a def or event name after \"not\"", p.errLine())
		}
		p.pos++
		if bad := p.peek(); bad != nil && (bad.kind == tokCompare ||
			(bad.kind == tokIdent && (bad.text == "in" || bad.text == "not" ||
				bad.text == "changed" || bad.text == "unchanged" ||
				bad.text == "increased" || bad.text == "decreased"))) {
			return nil, fmt.Errorf("line %d: \"not\" negates a def or event name; a condition is negated with its complement form", bad.line)
		}
		return &DefRef{Name: nt.text, Negate: true, Line: t.line}, nil
	}
	if t.kind != tokIdent || keywords[t.text] {
		return nil, fmt.Errorf("line %d: expected condition, found %q", t.line, t.text)
	}
	name := t
	p.pos++

	nt := p.peek()
	if nt == nil {
		return &DefRef{Name: name.text, Line: name.line}, nil
	}
	switch {
	case nt.kind == tokCompare:
		p.pos++
		op, err := compareOp(nt)
		if err != nil {
			return nil, err
		}
		// A non-const name on the right compares against another
		// watch; the validator resolves it.
		if rt := p.peek(); rt != nil && rt.kind == tokIdent && !keywords[rt.text] {
			if _, isConst := p.consts[rt.text]; !isConst {
				p.pos++
				return &CompareWatchCond{Left: name.text, Op: op, Right: rt.text, Line: name.line}, nil
			}
		}
		operand, _, err := p.takeOperand("comparison constant")
		if err != nil {
			return nil, err
		}
		return &CompareCond{Watch: name.text, Op: op, Operand: operand, Line: name.line}, nil
	case nt.kind == tokIdent && nt.text == "in":
		p.pos++
		return p.parseSet(name, false)
	case nt.kind == tokIdent && nt.text == "not":
		p.pos++
		if !p.acceptIdent("in") {
			return nil, fmt.Errorf("line %d: expected \"in\" after \"not\"", p.errLine())
		}
		return p.parseSet(name, true)
	case nt.kind == tokIdent && (nt.text == "changed" || nt.text == "unchanged"):
		p.pos++
		cond := &PrevCond{Watch: name.text, Op: OpChanged, Line: name.line}
		if nt.text == "unchanged" {
			cond.Op = OpUnchanged
		} else if err := p.parseAnchor(cond); err != nil {
			return nil, err
		}
		if err := p.checkStrayQual(cond); err != nil {
			return nil, err
		}
		return cond, nil
	case nt.kind == tokIdent && (nt.text == "increased" || nt.text == "decreased"):
		p.pos++
		op := OpIncreased
		if nt.text == "decreased" {
			op = OpDecreased
		}
		cond := &PrevCond{Watch: name.text, Op: op, Line: name.line}
		if p.acceptIdent("by") {
			qual := QualBy
			if p.acceptIdent("at") {
				switch {
				case p.acceptIdent("least"):
					qual = QualAtLeast
				case p.acceptIdent("most"):
					qual = QualAtMost
				default:
					return nil, fmt.Errorf("line %d: expected \"least\" or \"most\" after \"at\"", p.errLine())
				}
			}
			amt, _, err := p.takeMagnitude("delta constant")
			if err != nil {
				return nil, err
			}
			cond.Operand = amt
			cond.Qual = qual
		} else if err := p.parseAnchor(cond); err != nil {
			return nil, err
		}
		if err := p.checkStrayQual(cond); err != nil {
			return nil, err
		}
		return cond, nil
	default:
		return &DefRef{Name: name.text, Line: name.line}, nil
	}
}

func compareOp(t *token) (CompareOp, error) {
	switch t.text {
	case "==":
		return OpEq, nil
	case "!=":
		return OpNe, nil
	case "<":
		return OpLt, nil
	case ">":
		return OpGt, nil
	case "<=":
		return OpLe, nil
	case ">=":
		return OpGe, nil
	}
	return 0, fmt.Errorf("line %d: unknown comparison %q", t.line, t.text)
}

func (p *parser) parseSet(name *token, negate bool) (Expr, error) {
	if _, err := p.expect(tokLParen, `"("`); err != nil {
		return nil, err
	}
	var set []uint32
	for {
		n, _, err := p.takeOperand("set constant")
		if err != nil {
			return nil, err
		}
		set = append(set, n)
		t := p.peek()
		if t != nil && t.kind == tokComma {
			p.pos++
			continue
		}
		break
	}
	if _, err := p.expect(tokRParen, `")"`); err != nil {
		return nil, err
	}
	return &SetCond{Watch: name.text, Negate: negate, Set: set, Line: name.line}, nil
}

// parseAnchor parses an optional "from"/"to" anchor operand onto a
// previous-frame condition.
func (p *parser) parseAnchor(cond *PrevCond) error {
	var qual PrevQual
	switch {
	case p.acceptIdent("from"):
		qual = QualFrom
	case p.acceptIdent("to"):
		qual = QualTo
	default:
		return nil
	}
	v, _, err := p.takeOperand("anchor constant")
	if err != nil {
		return err
	}
	cond.Qual = qual
	cond.Operand = v
	return nil
}

// checkStrayQual rejects a qualifier token left over after a
// previous-frame condition is complete, so the error is specific
// instead of a generic syntax error further along.
func (p *parser) checkStrayQual(cond *PrevCond) error {
	t := p.peek()
	if t == nil || t.kind != tokIdent || (t.text != "by" && t.text != "from" && t.text != "to" && t.text != "at") {
		return nil
	}
	if cond.Qual != QualNone {
		return fmt.Errorf("line %d: a condition takes at most one of \"by\", \"from\", and \"to\"", t.line)
	}
	if t.text == "at" {
		return fmt.Errorf("line %d: \"at least\" and \"at most\" follow \"by\"", t.line)
	}
	return fmt.Errorf("line %d: %q is not valid after this condition", t.line, t.text)
}

func (p *parser) parseEffect() (Effect, error) {
	t, err := p.expect(tokIdent, "effect")
	if err != nil {
		return Effect{}, err
	}
	switch t.text {
	case "pulse":
		s, w, err := p.parseIntensity()
		if err != nil {
			return Effect{}, err
		}
		d, err := p.takeDuration("duration")
		if err != nil {
			return Effect{}, err
		}
		return Effect{Kind: EffectPulse, Strong: s, Weak: w, DurationMs: d}, nil
	case "pattern":
		if _, err := p.expect(tokLParen, `"("`); err != nil {
			return Effect{}, err
		}
		var steps []Step
		for {
			s, w, err := p.parseIntensity()
			if err != nil {
				return Effect{}, err
			}
			d, err := p.takeDuration("step duration")
			if err != nil {
				return Effect{}, err
			}
			steps = append(steps, Step{Strong: s, Weak: w, DurationMs: d})
			if t := p.peek(); t != nil && t.kind == tokComma {
				p.pos++
				continue
			}
			break
		}
		if _, err := p.expect(tokRParen, `")"`); err != nil {
			return Effect{}, err
		}
		return Effect{Kind: EffectPattern, Steps: steps}, nil
	case "hold":
		s, w, err := p.parseIntensity()
		if err != nil {
			return Effect{}, err
		}
		return Effect{Kind: EffectHold, Strong: s, Weak: w}, nil
	case "dampen", "amplify":
		kind := EffectDampen
		if t.text == "amplify" {
			kind = EffectAmplify
		}
		pct, err := p.takePercent(t.text + " percent")
		if err != nil {
			return Effect{}, err
		}
		return Effect{Kind: kind, Percent: pct}, nil
	}
	return Effect{}, fmt.Errorf("line %d: expected pulse, pattern, hold, dampen, or amplify, found %q", t.line, t.text)
}

func (p *parser) parseIntensity() (float64, float64, error) {
	if p.acceptIdent("off") {
		return 0, 0, nil
	}
	s, err := p.takeDecimal("intensity")
	if err != nil {
		return 0, 0, err
	}
	if t := p.peek(); t != nil && t.kind == tokSlash {
		p.pos++
		w, err := p.takeDecimal("intensity")
		if err != nil {
			return 0, 0, err
		}
		return s, w, nil
	}
	return s, s, nil
}

func (p *parser) parseModifiers(r *Rule) error {
	seen := map[string]bool{}
	for !p.atEnd() {
		t, _ := p.take("modifier")
		if t.kind != tokIdent {
			return fmt.Errorf("line %d: expected modifier, found %q", t.line, t.text)
		}
		if seen[t.text] {
			return fmt.Errorf("line %d: duplicate %s modifier", t.line, t.text)
		}
		switch t.text {
		case "level":
			r.Level = true
		case "priority":
			n, err := p.takePriority()
			if err != nil {
				return err
			}
			r.Priority = n
		case "clip":
			// The duration is optional.
			r.Clip = true
			if nt := p.peek(); nt != nil && nt.kind == tokDuration {
				d, err := p.takeDuration("clip duration")
				if err != nil {
					return err
				}
				r.ClipMs = d
			}
		case "cooldown":
			d, err := p.takeDuration("cooldown duration")
			if err != nil {
				return err
			}
			r.CooldownMs = d
		case "player":
			nt, err := p.take("player number or all")
			if err != nil {
				return err
			}
			switch {
			case nt.kind == tokIdent && nt.text == "all":
				r.PlayerAll = true
			case nt.kind == tokInt && !nt.neg:
				if nt.ival > 0xFFFF {
					return fmt.Errorf("line %d: player number %s out of range", nt.line, nt.text)
				}
				r.Player = int(nt.ival)
			default:
				return fmt.Errorf("line %d: expected player number or all, found %q", nt.line, nt.text)
			}
		case "scale":
			// A name right after "scale" is the magnitude watch,
			// unless ".." follows it, which makes it a const range
			// endpoint instead.
			var name string
			if nt := p.peek(); nt != nil && nt.kind == tokIdent {
				if p.pos+1 >= len(p.toks) || p.toks[p.pos+1].kind != tokDotDot {
					name = nt.text
					p.pos++
				}
			}
			aNeg := p.nextIsNegInt()
			a, _, err := p.takeOperand("scale range")
			if err != nil {
				return err
			}
			if _, err := p.expect(tokDotDot, `".."`); err != nil {
				return err
			}
			bNeg := p.nextIsNegInt()
			b, _, err := p.takeOperand("scale range")
			if err != nil {
				return err
			}
			if _, err := p.expect(tokArrow, `"->"`); err != nil {
				return err
			}
			x, err := p.takeDecimal("scale multiplier")
			if err != nil {
				return err
			}
			if _, err := p.expect(tokDotDot, `".."`); err != nil {
				return err
			}
			y, err := p.takeDecimal("scale multiplier")
			if err != nil {
				return err
			}
			r.Scale = &Scale{Watch: name, MagMin: a, MagMax: b,
				MagMinNeg: aNeg, MagMaxNeg: bNeg, MulMin: x, MulMax: y}
		default:
			return fmt.Errorf("line %d: unknown modifier %q", t.line, t.text)
		}
		seen[t.text] = true
	}
	return nil
}
