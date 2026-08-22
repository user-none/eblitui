// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package rumble

import (
	"errors"
	"fmt"
	"math"
)

func errf(line int, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if line > 0 {
		return fmt.Errorf("line %d: %s", line, msg)
	}
	return errors.New(msg)
}

// watchInterp classifies a watch's value interpretation. Watch
// comparisons require the same class on both sides so the values
// compare in one domain.
type watchInterp int

const (
	interpPlain watchInterp = iota
	interpSigned
	interpAbs
	interpBCD
)

func interpOf(v *Watch) watchInterp {
	switch {
	case v.Signed:
		return interpSigned
	case v.Abs:
		return interpAbs
	case v.BCD:
		return interpBCD
	}
	return interpPlain
}

type declInfo struct {
	isWatch  bool
	isArray  bool
	isConst  bool
	isEvent  bool
	interp   watchInterp // meaningful only for a watch
	line     int
	defIndex int // meaningful only for a def
}

// exprCtx carries the constraints an expression is checked under.
type exprCtx struct {
	constOnly bool // previous-frame conditions are not allowed
	where     string
	refLine   int
	defLimit  int // DefRefs must resolve below this def index; -1 for no index limit
	names     map[string]declInfo
}

// validateRuleset implements the validation checklist in FORMAT.md over a
// ruleset, whether it came from the parser or was built programmatically.
func validateRuleset(rs *Ruleset) error {
	names := make(map[string]declInfo)
	for i := range rs.Watches {
		v := &rs.Watches[i]
		if err := checkName(v.Name, v.Line, "watch"); err != nil {
			return err
		}
		if _, dup := names[v.Name]; dup {
			return errf(v.Line, "duplicate name %q", v.Name)
		}
		names[v.Name] = declInfo{isWatch: true, isArray: (v.Count != 0 || v.Stride != 0) && !v.HasKey, interp: interpOf(v), line: v.Line}
		switch v.Width {
		case 8, 16, 32:
		default:
			return errf(v.Line, "watch %q width must be 8, 16, or 32", v.Name)
		}
		if v.Signed && v.Abs {
			return errf(v.Line, "watch %q cannot be both signed and abs", v.Name)
		}
		if v.BCD && (v.Signed || v.Abs) {
			return errf(v.Line, "watch %q cannot combine bcd with signed or abs", v.Name)
		}
		if v.Pointer && (v.Count != 0 || v.Stride != 0) {
			return errf(v.Line, "watch %q is a pointer watch, which takes no stride or count", v.Name)
		}
		if v.HasKey && (v.Count == 0 && v.Stride == 0) {
			return errf(v.Line, "watch %q key requires stride and count", v.Name)
		}
		if v.HasKey && v.Pointer {
			return errf(v.Line, "watch %q cannot combine ptr with key", v.Name)
		}
		if v.FieldPtr && !v.HasKey {
			return errf(v.Line, "watch %q field ptr requires a keyed slot watch", v.Name)
		}
		if v.Offset != 0 && !v.Pointer && !v.FieldPtr {
			return errf(v.Line, "watch %q offset requires ptr", v.Name)
		}
		if v.Count != 0 || v.Stride != 0 {
			if v.Count < 2 {
				return errf(v.Line, "watch %q count must be at least 2", v.Name)
			}
			if v.Count > 0xFFFF {
				return errf(v.Line, "watch %q count %d out of range", v.Name, v.Count)
			}
			if v.HasKey {
				if uint64(v.KeyOffset)+4 > uint64(v.Stride) {
					return errf(v.Line, "watch %q key does not fit inside the stride", v.Name)
				}
				if uint64(v.FieldOffset)+v.fieldSize() > uint64(v.Stride) {
					return errf(v.Line, "watch %q field does not fit inside the stride", v.Name)
				}
			} else if v.Stride < uint32(v.Width/8) {
				return errf(v.Line, "watch %q stride must be at least its width", v.Name)
			}
			if end := uint64(v.Address) + uint64(v.Stride)*uint64(v.Count-1) + v.slotSpan(); end > 1<<32 {
				return errf(v.Line, "watch %q slots extend past the 32-bit address space", v.Name)
			}
		}
		if v.HasMask && v.Width < 32 && v.Mask>>uint(v.Width) != 0 {
			return errf(v.Line, "watch %q mask 0x%X does not fit width %d", v.Name, v.Mask, v.Width)
		}
		if v.Endian != EndianDefault && v.Endian != EndianLittle && v.Endian != EndianBig {
			return errf(v.Line, "watch %q has an invalid endian", v.Name)
		}
	}
	for i := range rs.Consts {
		c := &rs.Consts[i]
		if err := checkName(c.Name, c.Line, "const"); err != nil {
			return err
		}
		if _, dup := names[c.Name]; dup {
			return errf(c.Line, "duplicate name %q", c.Name)
		}
		names[c.Name] = declInfo{isConst: true, line: c.Line}
	}
	for i := range rs.Events {
		ev := &rs.Events[i]
		if err := checkName(ev.Name, ev.Line, "event"); err != nil {
			return err
		}
		if _, dup := names[ev.Name]; dup {
			return errf(ev.Line, "duplicate name %q", ev.Name)
		}
		names[ev.Name] = declInfo{isEvent: true, line: ev.Line}
	}
	for i := range rs.Defs {
		d := &rs.Defs[i]
		if err := checkName(d.Name, d.Line, "def"); err != nil {
			return err
		}
		if _, dup := names[d.Name]; dup {
			return errf(d.Line, "duplicate name %q", d.Name)
		}
		names[d.Name] = declInfo{line: d.Line, defIndex: i}
	}

	for i := range rs.Events {
		ev := &rs.Events[i]
		if ev.Held {
			if ev.Trigger != (PrevCond{}) {
				return errf(ev.Line, "event %q has both a trigger and a held condition", ev.Name)
			}
			if ev.DurationMs != 0 {
				return errf(ev.Line, "event %q has a duration on a held event", ev.Name)
			}
			if ev.HeldFrames < 1 {
				return errf(ev.Line, "event %q frame count must be at least 1", ev.Name)
			}
			if ev.HeldFrames >= 1<<31 {
				return errf(ev.Line, "event %q frame count %d out of range", ev.Name, ev.HeldFrames)
			}
			if exprReferences(ev.Cond, ev.Name) {
				return errf(ev.Line, "event %q references itself", ev.Name)
			}
			ctx := exprCtx{
				constOnly: true,
				where:     fmt.Sprintf("event %q condition", ev.Name),
				refLine:   ev.Line,
				defLimit:  -1,
				names:     names,
			}
			if err := checkExpr(ev.Cond, ctx); err != nil {
				return err
			}
			continue
		}
		if ev.Cond != nil {
			return errf(ev.Line, "event %q has a held condition on a trigger event", ev.Name)
		}
		if ev.HeldFrames != 0 {
			return errf(ev.Line, "event %q has a frame count on a trigger event", ev.Name)
		}
		if ev.Trigger.Op == OpUnchanged {
			return errf(ev.Line, "event %q trigger cannot be unchanged", ev.Name)
		}
		if ev.DurationMs <= 0 || ev.DurationMs >= maxDurationMs {
			return errf(ev.Line, "event %q duration must be positive", ev.Name)
		}
		ctx := exprCtx{
			where:    fmt.Sprintf("event %q trigger", ev.Name),
			refLine:  ev.Line,
			defLimit: -1,
			names:    names,
		}
		trigger := ev.Trigger
		if err := checkExpr(&trigger, ctx); err != nil {
			return err
		}
	}

	for i := range rs.Defs {
		d := &rs.Defs[i]
		ctx := exprCtx{
			constOnly: true,
			where:     fmt.Sprintf("def %q", d.Name),
			refLine:   d.Line,
			defLimit:  i,
			names:     names,
		}
		if err := checkExpr(d.Expr, ctx); err != nil {
			return err
		}
	}

	for i := range rs.Rules {
		if err := validateRule(&rs.Rules[i], i, names); err != nil {
			return err
		}
	}
	return nil
}

func validateRule(r *Rule, index int, names map[string]declInfo) error {
	where := fmt.Sprintf("rule %d", index+1)
	if r.Line > 0 {
		where = fmt.Sprintf("rule at line %d", r.Line)
	}

	onCtx := exprCtx{
		constOnly: r.Effect.Kind == EffectHold || r.Effect.Kind == EffectDampen ||
			r.Effect.Kind == EffectAmplify,
		where:    where + " condition",
		refLine:  r.Line,
		defLimit: -1,
		names:    names,
	}
	if err := checkExpr(r.On, onCtx); err != nil {
		return err
	}
	if r.While != nil {
		whileCtx := exprCtx{
			constOnly: true,
			where:     where + " while expression",
			refLine:   r.Line,
			defLimit:  -1,
			names:     names,
		}
		if err := checkExpr(r.While, whileCtx); err != nil {
			return err
		}
	}

	if r.Effect.Percent != 0 && r.Effect.Kind != EffectDampen && r.Effect.Kind != EffectAmplify {
		return errf(r.Line, "%s has a percent on an effect that takes none", where)
	}

	switch r.Effect.Kind {
	case EffectPulse:
		if err := checkIntensity(r.Effect.Strong, r.Effect.Weak, r.Line, where); err != nil {
			return err
		}
		if r.Effect.DurationMs <= 0 || r.Effect.DurationMs >= maxDurationMs {
			return errf(r.Line, "%s duration must be positive", where)
		}
		if r.Effect.Steps != nil {
			return errf(r.Line, "%s has steps on a pulse effect", where)
		}
	case EffectPattern:
		if len(r.Effect.Steps) == 0 {
			return errf(r.Line, "%s pattern has no steps", where)
		}
		if r.Effect.Strong != 0 || r.Effect.Weak != 0 || r.Effect.DurationMs != 0 {
			return errf(r.Line, "%s has pulse fields on a pattern effect", where)
		}
		for _, step := range r.Effect.Steps {
			if err := checkIntensity(step.Strong, step.Weak, r.Line, where); err != nil {
				return err
			}
			if step.DurationMs <= 0 || step.DurationMs >= maxDurationMs {
				return errf(r.Line, "%s step duration must be positive", where)
			}
		}
	case EffectHold:
		if err := checkIntensity(r.Effect.Strong, r.Effect.Weak, r.Line, where); err != nil {
			return err
		}
		if r.Effect.Strong == 0 && r.Effect.Weak == 0 {
			return errf(r.Line, "%s hold needs at least one nonzero motor", where)
		}
		if r.Effect.DurationMs != 0 {
			return errf(r.Line, "%s has a duration on a hold effect", where)
		}
		if r.Effect.Steps != nil {
			return errf(r.Line, "%s has steps on a hold effect", where)
		}
		if r.Level {
			return errf(r.Line, "%s: level is not valid on a hold rule", where)
		}
		if r.CooldownMs != 0 {
			return errf(r.Line, "%s: cooldown is not valid on a hold rule", where)
		}
	case EffectDampen, EffectAmplify:
		kind, maxPct := "a dampen", 100
		if r.Effect.Kind == EffectAmplify {
			kind, maxPct = "an amplify", 200
		}
		if r.Effect.Strong != 0 || r.Effect.Weak != 0 {
			return errf(r.Line, "%s has intensity on %s effect", where, kind)
		}
		if r.Effect.DurationMs != 0 {
			return errf(r.Line, "%s has a duration on %s effect", where, kind)
		}
		if r.Effect.Steps != nil {
			return errf(r.Line, "%s has steps on %s effect", where, kind)
		}
		if r.Effect.Percent < 1 || r.Effect.Percent > maxPct {
			return errf(r.Line, "%s percent must be 1 to %d on %s rule", where, maxPct, kind)
		}
		if r.Level {
			return errf(r.Line, "%s: level is not valid on %s rule", where, kind)
		}
		if r.CooldownMs != 0 {
			return errf(r.Line, "%s: cooldown is not valid on %s rule", where, kind)
		}
		if r.Priority != 0 {
			return errf(r.Line, "%s: priority is not valid on %s rule", where, kind)
		}
	default:
		return errf(r.Line, "%s has an unknown effect kind", where)
	}

	if r.Priority < MinPriority || r.Priority > MaxPriority {
		return errf(r.Line, "%s priority must be %d to %d", where, MinPriority, MaxPriority)
	}

	if r.CooldownMs < 0 || r.CooldownMs >= maxDurationMs {
		return errf(r.Line, "%s cooldown must be non-negative", where)
	}

	if r.Clip {
		// A previous-frame condition is true for a single frame, so a
		// clip on one would cancel the effect it just installed.
		if r.Effect.Kind != EffectPulse && r.Effect.Kind != EffectPattern {
			return errf(r.Line, "%s: clip is only valid on a pulse or pattern rule", where)
		}
		if countPrevConds(r.On) != 0 {
			return errf(r.Line, "%s: clip is not valid with a previous-frame condition", where)
		}
		if r.ClipMs < 0 || r.ClipMs >= maxDurationMs {
			return errf(r.Line, "%s clip duration must be non-negative", where)
		}
	} else if r.ClipMs != 0 {
		return errf(r.Line, "%s has a clip duration without clip", where)
	}

	if !r.PlayerAll {
		if r.Player < 1 {
			return errf(r.Line, "%s player must be positive", where)
		}
		if r.Player > 0xFFFF {
			return errf(r.Line, "%s player %d out of range", where, r.Player)
		}
	}

	if s := r.Scale; s != nil {
		if s.Watch == "" {
			// Change form: the magnitude is the change in the rule's
			// one previous-frame condition.
			if r.Effect.Kind != EffectPulse && r.Effect.Kind != EffectPattern {
				return errf(r.Line, "%s: scale without a watch is only valid on a pulse or pattern rule", where)
			}
			if n := countPrevConds(r.On); n != 1 {
				return errf(r.Line, "%s: scale without a watch requires exactly one previous-frame condition, found %d", where, n)
			}
		} else {
			// Level form: the magnitude is a named watch's current
			// value, which a hold can re-read every frame.
			if r.Effect.Kind != EffectPulse && r.Effect.Kind != EffectPattern && r.Effect.Kind != EffectHold {
				return errf(r.Line, "%s: scale is only valid on a pulse, pattern, or hold rule", where)
			}
			d, ok := names[s.Watch]
			if !ok {
				return errf(r.Line, "%s scales on undeclared name %q", where, s.Watch)
			}
			if !d.isWatch {
				return errf(r.Line, "%s scales on %q, which is not a watch", where, s.Watch)
			}
			if d.isArray {
				return errf(r.Line, "%s scales on array watch %q, which has no single value", where, s.Watch)
			}
			if d.line > 0 && r.Line > 0 && d.line > r.Line {
				return errf(r.Line, "%s scales on watch %q before its declaration", where, s.Watch)
			}
		}
		// Minus-signed endpoints order signed, which only a signed
		// watch provides.
		if (s.MagMinNeg || s.MagMaxNeg) && (s.Watch == "" || names[s.Watch].interp != interpSigned) {
			return errf(r.Line, "%s scale range takes a minus sign only on a signed watch", where)
		}
		// On a signed watch the range endpoints order signed; the
		// bias maps that onto the unsigned comparison.
		a, b := s.MagMin, s.MagMax
		if s.Watch != "" && names[s.Watch].interp == interpSigned {
			a ^= 0x80000000
			b ^= 0x80000000
		}
		if a >= b {
			return errf(r.Line, "%s scale magnitude range needs A < B", where)
		}
		if math.IsNaN(s.MulMin) || math.IsNaN(s.MulMax) {
			return errf(r.Line, "%s scale multiplier is not a number", where)
		}
		if math.IsInf(s.MulMin, 0) || math.IsInf(s.MulMax, 0) {
			return errf(r.Line, "%s scale multiplier must be finite", where)
		}
		if s.MulMin < 0 || s.MulMax < 0 || (s.MulMin == 0 && s.MulMax == 0) {
			return errf(r.Line, "%s scale multiplier range needs X and Y at least 0 with one of them positive", where)
		}
	}
	return nil
}

func checkExpr(e Expr, ctx exprCtx) error {
	switch n := e.(type) {
	case nil:
		return errf(ctx.refLine, "%s is missing", ctx.where)
	case *BinaryExpr:
		if err := checkExpr(n.Left, ctx); err != nil {
			return err
		}
		return checkExpr(n.Right, ctx)
	case *CompareCond:
		return ctx.watchRef(n.Watch, n.Line)
	case *CompareWatchCond:
		if err := ctx.watchRef(n.Left, n.Line); err != nil {
			return err
		}
		// The parser folds consts positionally, so a const name
		// arriving here was declared below its use.
		if d, ok := ctx.names[n.Right]; ok && d.isConst {
			return errf(ctx.at(n.Line), "%s compares against const %q, which must be declared above its use", ctx.where, n.Right)
		}
		if err := ctx.watchRef(n.Right, n.Line); err != nil {
			return err
		}
		if n.Left == n.Right {
			return errf(ctx.at(n.Line), "%s compares watch %q with itself", ctx.where, n.Left)
		}
		for _, name := range []string{n.Left, n.Right} {
			if ctx.names[name].isArray {
				return errf(ctx.at(n.Line), "%s compares array watch %q, which has no single value", ctx.where, name)
			}
		}
		if ctx.names[n.Left].interp != ctx.names[n.Right].interp {
			return errf(ctx.at(n.Line), "%s compares %q and %q, which declare different interpretations", ctx.where, n.Left, n.Right)
		}
		return nil
	case *SetCond:
		if len(n.Set) == 0 {
			return errf(ctx.at(n.Line), "%s has an empty set", ctx.where)
		}
		return ctx.watchRef(n.Watch, n.Line)
	case *PrevCond:
		if ctx.constOnly {
			return errf(ctx.at(n.Line), "previous-frame condition is not allowed in %s", ctx.where)
		}
		if d, ok := ctx.names[n.Watch]; ok && d.isArray {
			return errf(ctx.at(n.Line), "%s uses a previous-frame condition on array watch %q, which allows only constant comparisons", ctx.where, n.Watch)
		}
		switch n.Qual {
		case QualNone:
			if n.Operand != 0 {
				return errf(ctx.at(n.Line), "%s has an operand without a qualifier", ctx.where)
			}
		case QualBy, QualAtLeast, QualAtMost:
			if n.Op != OpIncreased && n.Op != OpDecreased {
				return errf(ctx.at(n.Line), "%s: by is only valid on increased and decreased", ctx.where)
			}
			if n.Qual == QualBy && n.Operand == 0 {
				return errf(ctx.at(n.Line), "%s: an exact by delta must be positive", ctx.where)
			}
			if n.Qual == QualAtMost && n.Operand == 0 {
				return errf(ctx.at(n.Line), "%s: an at most bound must be positive", ctx.where)
			}
		case QualFrom, QualTo:
			if n.Op == OpUnchanged {
				return errf(ctx.at(n.Line), "%s: unchanged does not take a from/to anchor", ctx.where)
			}
		default:
			return errf(ctx.at(n.Line), "%s has an unknown condition qualifier", ctx.where)
		}
		return ctx.watchRef(n.Watch, n.Line)
	case *DefRef:
		return ctx.defRef(n.Name, n.Line)
	}
	return errf(ctx.refLine, "%s has an unknown condition node", ctx.where)
}

// at picks the most specific known line for an error.
func (ctx exprCtx) at(nodeLine int) int {
	if nodeLine > 0 {
		return nodeLine
	}
	return ctx.refLine
}

func (ctx exprCtx) watchRef(name string, nodeLine int) error {
	line := ctx.at(nodeLine)
	d, ok := ctx.names[name]
	if !ok {
		return errf(line, "%s references undeclared name %q", ctx.where, name)
	}
	if !d.isWatch {
		kind := "def"
		switch {
		case d.isConst:
			kind = "const"
		case d.isEvent:
			kind = "event"
		}
		return errf(line, "%s uses %s %q where a watch is required", ctx.where, kind, name)
	}
	if d.line > 0 && line > 0 && d.line > line {
		return errf(line, "%s references watch %q before its declaration", ctx.where, name)
	}
	return nil
}

func (ctx exprCtx) defRef(name string, nodeLine int) error {
	line := ctx.at(nodeLine)
	d, ok := ctx.names[name]
	if !ok {
		return errf(line, "%s references undeclared name %q", ctx.where, name)
	}
	if d.isWatch {
		return errf(line, "%s uses watch %q alone; a bare name must reference a def or event", ctx.where, name)
	}
	if d.isConst {
		return errf(line, "%s uses const %q where a def or event is required", ctx.where, name)
	}
	if d.isEvent {
		if d.line > 0 && line > 0 && d.line > line {
			return errf(line, "%s references event %q before its declaration", ctx.where, name)
		}
		return nil
	}
	if ctx.defLimit >= 0 {
		if d.defIndex >= ctx.defLimit {
			return errf(line, "%s references def %q before its declaration", ctx.where, name)
		}
		return nil
	}
	if d.line > 0 && line > 0 && d.line > line {
		return errf(line, "%s references def %q before its declaration", ctx.where, name)
	}
	return nil
}

// exprReferences reports whether the expression references name as a
// def or event, catching a held event naming itself, which the
// line-ordered declaration checks cannot see.
func exprReferences(e Expr, name string) bool {
	switch n := e.(type) {
	case *BinaryExpr:
		return exprReferences(n.Left, name) || exprReferences(n.Right, name)
	case *DefRef:
		return n.Name == name
	}
	return false
}

func countPrevConds(e Expr) int {
	switch n := e.(type) {
	case *BinaryExpr:
		return countPrevConds(n.Left) + countPrevConds(n.Right)
	case *PrevCond:
		return 1
	}
	return 0
}

// checkIntensity rejects NaN explicitly: a NaN fails no range
// comparison and would otherwise pass straight through to the motor
// output. Only a programmatically built ruleset can carry one.
func checkIntensity(strong, weak float64, line int, where string) error {
	if math.IsNaN(strong) || math.IsNaN(weak) {
		return errf(line, "%s intensity is not a number", where)
	}
	if strong < 0 || strong > 1 || weak < 0 || weak > 1 {
		return errf(line, "%s intensity must be within 0 to 1.0", where)
	}
	return nil
}

func checkName(name string, line int, kind string) error {
	if name == "" {
		return errf(line, "%s name is empty", kind)
	}
	if keywords[name] {
		return errf(line, "%s name %q is a keyword", kind, name)
	}
	if !isIdentStart(name[0]) {
		return errf(line, "%s name %q is not a valid identifier", kind, name)
	}
	for i := 1; i < len(name); i++ {
		if !isIdentPart(name[i]) {
			return errf(line, "%s name %q is not a valid identifier", kind, name)
		}
	}
	return nil
}
