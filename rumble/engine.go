// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package rumble

import (
	"sort"
	"time"
)

// settleFrames is the number of frames after start or reset during which
// watches seed without any rule firing.
const settleFrames = 30

// signBias maps signed order onto unsigned order. XOR with it
// preserves ordering and differences. So, signed values and operands
// are biased once and compared unsigned downstream.
const signBias = 0x80000000

// Reader is the memory interface the engine reads through, matching the
// shape of the core memory interface. Addresses are native bus
// addresses.
type Reader interface {
	ReadMemory(addr uint32, buf []byte) uint32
}

// MotorState is the final motor output for one player on one frame,
// after arbitration and hold mixing. A player absent from a frame's
// output has both motors off. The converse does not hold: a silent
// pattern step, a pulse authored off, and an effect or hold muted by
// a full dampen all leave a player present at zero.
type MotorState struct {
	Player int
	Strong float64
	Weak   float64
}

// Engine evaluates a ruleset once per emulated frame.
type Engine struct {
	meta       Metadata
	watches    []engineWatch
	regions    []Region // for run-time pointer target checks
	events     []engineEvent
	eventVals  []bool
	defs       []compiledExpr
	defsNeg    []compiledExpr
	defVals    []bool
	defValsNeg []bool
	evalOrder  []evalStep
	rules      []engineRule
	conds      []bool // this frame's rule conditions, in rule order
	players    map[int]*playerState
	allPlayers []int
	settle     int
}

// engineWatch is a declared watch's runtime state. An array watch is
// valid once its slots are read: a slot whose own read failed is
// marked invalid for the frame rather than making the watch
// unresolved, and an invalid slot matches no condition.
type engineWatch struct {
	val        Watch
	addr       uint32 // native address as declared, validated in-region
	bigEndian  bool
	sysBig     bool // system byte order, for pointer and key reads
	widthMask  uint32
	bcdMod     uint32 // decimal capacity of a bcd watch; 0 otherwise
	cur        uint32
	prev       uint32
	valid      bool     // resolved this frame; unresolved makes conditions false
	wasValid   bool     // resolved last frame from the same source
	lastPtr    uint32   // last pointer read, to re-seed when the target moves
	slot       int      // keyed slot watch's selected slot; -1 without a match
	slots      []uint32 // per-slot current values; nil on a single-value watch
	slotsValid []bool   // per-slot validity
	buf        [4]byte  // read scratch; kept here so reads through Reader do not allocate
}

type engineRule struct {
	rule       Rule
	on         compiledExpr
	while      compiledExpr // nil when the rule has no gate
	target     []int        // nil when the rule targets all players
	scaleIdx   int          // value index feeding scale; -1 without scale
	magMin     uint32       // scale magnitude range, biased on a signed watch
	magMax     uint32
	byDelta    uint32 // exact by operand; the change-form magnitude when useByDelta is set
	useByDelta bool
	prevWas    bool
	readyAt    time.Time
	condTrueAt time.Time // last frame the condition held, for clip
}

// engineEvent is a declared event's runtime state.
type engineEvent struct {
	trigger    compiledExpr // nil on a held event
	durationMs int
	until      time.Time
	held       compiledExpr // nil on a trigger event
	heldFrames int
	run        int // capped at heldFrames so the counter cannot overflow
}

// evalStep is one entry of the per-frame def and held-event pass.
type evalStep struct {
	isDef bool
	idx   int
}

// playerState is a player's effect slot. strong and weak are the
// slot's levels for the frame being evaluated, settled by resolve.
type playerState struct {
	effect *activeEffect
	strong float64
	weak   float64
}

// resolve retires a spent effect and records a playing one's levels.
// It runs before the frame's fires and on each install, so a non-nil
// effect always means one playing at now.
func (ps *playerState) resolve(now time.Time) {
	if ps.effect == nil {
		return
	}
	strong, weak, playing := effectLevels(ps.effect, now)
	if !playing {
		ps.clear()
		return
	}
	ps.strong, ps.weak = strong, weak
}

func (ps *playerState) clear() {
	ps.effect = nil
	ps.strong, ps.weak = 0, 0
}

// heldLevels is a player's winning hold levels and the priority that
// won them.
type heldLevels struct {
	strong float64
	weak   float64
	prio   int
}

// activeEffect is the pulse or pattern occupying a player's effect
// slot.
type activeEffect struct {
	steps []Step
	start time.Time
	owner int // rule that installed it; only its clip cancels it
	prio  int // a later fire must exceed this to take the slot
}

type compiledExpr func(en *Engine) bool

// NewEngine validates a ruleset and binds it to a system's memory
// layout. Parsed and programmatically built rulesets pass through the
// same validation. players is the number of emulated players a
// "player all" rule fans out to; a count below 1 is treated as 1.
//
// The engine keeps references into rs - a pattern's steps and a rule's
// scale - so rs must not be mutated after it is bound. Build a fresh
// ruleset and a fresh engine to change a rule.
func NewEngine(rs *Ruleset, sys System, players int) (*Engine, error) {
	if err := validateRuleset(rs); err != nil {
		return nil, err
	}
	if players < 1 {
		players = 1
	}
	en := &Engine{
		meta:       rs.Metadata,
		regions:    append([]Region(nil), sys.Regions...),
		players:    make(map[int]*playerState),
		defVals:    make([]bool, len(rs.Defs)),
		defValsNeg: make([]bool, len(rs.Defs)),
		settle:     settleFrames,
	}
	for p := 1; p <= players; p++ {
		en.allPlayers = append(en.allPlayers, p)
	}

	watchIdx := make(map[string]int)
	arrays := make(map[string]bool)
	signed := make(map[string]bool)
	for i := range rs.Watches {
		v := &rs.Watches[i]
		if err := sys.validate(v); err != nil {
			return nil, err
		}
		if v.Count > 1 && !v.HasKey {
			arrays[v.Name] = true
		}
		if v.Signed {
			signed[v.Name] = true
		}
		bigEndian := sys.BigEndian
		switch v.Endian {
		case EndianLittle:
			bigEndian = false
		case EndianBig:
			bigEndian = true
		}
		// Signed and abs values are sign-extended to 32 bits at read
		// time, so the exact-delta arithmetic runs at 32 bits: the
		// exact by form does not wrap at the declared width on them.
		var widthMask uint32 = 0xFFFFFFFF
		if v.Width < 32 && !v.Signed && !v.Abs {
			widthMask = 1<<uint(v.Width) - 1
		}
		watchIdx[v.Name] = i
		ew := engineWatch{
			val:       *v,
			addr:      v.Address,
			bigEndian: bigEndian,
			sysBig:    sys.BigEndian,
			widthMask: widthMask,
			slot:      -1,
		}
		if v.BCD {
			ew.bcdMod = bcdCapacity(v)
		}
		if v.Count > 1 && !v.HasKey {
			ew.slots = make([]uint32, v.Count)
			ew.slotsValid = make([]bool, v.Count)
		}
		en.watches = append(en.watches, ew)
	}

	// Both name maps exist before any event or def compiles: a held
	// event's condition may reference defs, and def bodies may
	// reference events.
	eventIdx := make(map[string]int)
	for i := range rs.Events {
		eventIdx[rs.Events[i].Name] = i
	}
	defIdx := make(map[string]int)
	for i := range rs.Defs {
		defIdx[rs.Defs[i].Name] = i
	}

	en.eventVals = make([]bool, len(rs.Events))
	for i := range rs.Events {
		ev := &rs.Events[i]
		if ev.Held {
			en.events = append(en.events, engineEvent{
				held:       compileExpr(ev.Cond, arrays, signed, watchIdx, defIdx, eventIdx),
				heldFrames: ev.HeldFrames,
			})
			continue
		}
		trigger := ev.Trigger
		en.events = append(en.events, engineEvent{
			trigger:    compileExpr(&trigger, arrays, signed, watchIdx, nil, nil),
			durationMs: ev.DurationMs,
		})
	}

	for i := range rs.Defs {
		d := &rs.Defs[i]
		en.defs = append(en.defs, compileExpr(d.Expr, arrays, signed, watchIdx, defIdx, eventIdx))
		en.defsNeg = append(en.defsNeg, compileNegExpr(d.Expr, arrays, signed, watchIdx, defIdx, eventIdx))
	}

	// Line-less programmatic rulesets evaluate held events ahead of
	// defs.
	for i := range rs.Events {
		if rs.Events[i].Held {
			en.evalOrder = append(en.evalOrder, evalStep{idx: i})
		}
	}
	for i := range rs.Defs {
		en.evalOrder = append(en.evalOrder, evalStep{isDef: true, idx: i})
	}
	sort.SliceStable(en.evalOrder, func(a, b int) bool {
		return en.evalStepLine(rs, en.evalOrder[a]) < en.evalStepLine(rs, en.evalOrder[b])
	})

	for i := range rs.Rules {
		r := &rs.Rules[i]
		er := engineRule{
			rule:     *r,
			on:       compileExpr(r.On, arrays, signed, watchIdx, defIdx, eventIdx),
			scaleIdx: -1,
		}
		if r.While != nil {
			er.while = compileExpr(r.While, arrays, signed, watchIdx, defIdx, eventIdx)
		}
		if !r.PlayerAll {
			er.target = []int{r.Player}
		}
		if r.Scale != nil {
			er.magMin, er.magMax = r.Scale.MagMin, r.Scale.MagMax
			if r.Scale.Watch != "" {
				er.scaleIdx = watchIdx[r.Scale.Watch]
				if signed[r.Scale.Watch] {
					er.magMin ^= signBias
					er.magMax ^= signBias
				}
			} else {
				pc := findPrevCond(r.On)
				er.scaleIdx = watchIdx[pc.Watch]
				if pc.Qual == QualBy {
					er.byDelta = pc.Operand
					er.useByDelta = true
				}
			}
		}
		en.rules = append(en.rules, er)
	}
	en.conds = make([]bool, len(en.rules))
	return en, nil
}

// bcdCapacity returns the decimal capacity a bcd watch's exact delta
// forms wrap at. One power of ten per nibble through the mask's
// highest set nibble, or through the full width without a mask.
func bcdCapacity(v *Watch) uint32 {
	nibbles := v.Width / 4
	if v.HasMask && v.Mask != 0 {
		nibbles = 0
		for m := v.Mask; m != 0; m >>= 4 {
			nibbles++
		}
	}
	mod := uint32(1)
	for ; nibbles > 0; nibbles-- {
		mod *= 10
	}
	return mod
}

func (en *Engine) evalStepLine(rs *Ruleset, s evalStep) int {
	if s.isDef {
		return rs.Defs[s.idx].Line
	}
	return rs.Events[s.idx].Line
}

// Metadata returns the ruleset's metadata block.
func (en *Engine) Metadata() Metadata {
	return en.meta
}

// Reset clears all runtime state and re-arms the settle window.
// Called on save state load, rewind, and resume (see INTEGRATION.md).
func (en *Engine) Reset() {
	en.settle = settleFrames
	for i := range en.watches {
		v := &en.watches[i]
		v.cur = 0
		v.prev = 0
		v.valid = false
		v.wasValid = false
		v.lastPtr = 0
		v.slot = -1
		for j := range v.slots {
			v.slots[j] = 0
			v.slotsValid[j] = false
		}
	}
	for i := range en.events {
		en.events[i].until = time.Time{}
		en.events[i].run = 0
	}
	for i := range en.rules {
		en.rules[i].prevWas = false
		en.rules[i].readyAt = time.Time{}
		en.rules[i].condTrueAt = time.Time{}
	}
	en.players = make(map[int]*playerState)
}

// Evaluate runs one frame and returns the per-player motor output.
func (en *Engine) Evaluate(mem Reader, now time.Time) []MotorState {
	for i := range en.watches {
		v := &en.watches[i]
		switch {
		case v.slots != nil:
			// Array watches keep no previous values; a failed or
			// invalid-bcd read marks the slot invalid.
			for j := range v.slots {
				nv, ok := v.readAt(mem, v.addr+uint32(j)*v.val.Stride)
				v.slots[j] = nv
				v.slotsValid[j] = ok
			}
			v.valid = true
		case v.val.HasKey:
			en.readKeyed(mem, v)
		case v.val.Pointer:
			en.readPointer(mem, v)
		default:
			if nv, ok := v.readAt(mem, v.addr); ok {
				v.update(nv, true)
			} else {
				v.unresolve()
			}
		}
	}

	settling := en.settle > 0
	if settling {
		en.settle--
	}

	for i := range en.events {
		ev := &en.events[i]
		if ev.trigger == nil {
			continue
		}
		if !settling && ev.trigger(en) {
			ev.until = now.Add(time.Duration(ev.durationMs) * time.Millisecond)
		}
		en.eventVals[i] = now.Before(ev.until)
	}

	// Defs (both polarities) and held events evaluate in declaration
	// order, so each sees current-frame values for everything above
	// it. Held counters run during settle: their conditions need no
	// seeded previous values.
	for _, st := range en.evalOrder {
		if st.isDef {
			en.defVals[st.idx] = en.defs[st.idx](en)
			en.defValsNeg[st.idx] = en.defsNeg[st.idx](en)
			continue
		}
		ev := &en.events[st.idx]
		if ev.held(en) {
			if ev.run < ev.heldFrames {
				ev.run++
			}
		} else {
			ev.run = 0
		}
		en.eventVals[st.idx] = ev.run >= ev.heldFrames
	}

	holds := make(map[int]heldLevels)
	mods := make(map[int]float64)

	for _, ps := range en.players {
		ps.resolve(now)
	}

	// Conditions are evaluated for every rule before any fire is
	// arbitrated, so a clip releases the slot ahead of the frame's
	// fires whatever the rule's position in the file.
	for i := range en.rules {
		en.conds[i] = en.rules[i].on(en)
		if en.rules[i].rule.Clip {
			en.applyClip(i, en.conds[i], now)
		}
	}

	for i := range en.rules {
		r := &en.rules[i]
		cond := en.conds[i]
		gate := r.while == nil || r.while(en)

		if k := r.rule.Effect.Kind; k == EffectDampen || k == EffectAmplify {
			if gate && cond && !settling {
				mul := 1 + float64(r.rule.Effect.Percent)/100
				if k == EffectDampen {
					mul = 1 - float64(r.rule.Effect.Percent)/100
				}
				for _, p := range r.targetPlayers(en) {
					if _, set := mods[p]; !set {
						mods[p] = mul
					}
				}
			}
			continue
		}

		if r.rule.Effect.Kind == EffectHold {
			if gate && cond && !settling {
				strong, weak := r.rule.Effect.Strong, r.rule.Effect.Weak
				if r.rule.Scale != nil {
					mul := en.scaleMultiplier(r)
					strong = min(1, strong*mul)
					weak = min(1, weak*mul)
				}
				for _, p := range r.targetPlayers(en) {
					if h, set := holds[p]; set && r.rule.Priority <= h.prio {
						continue
					}
					holds[p] = heldLevels{strong: strong, weak: weak, prio: r.rule.Priority}
				}
			}
			continue
		}

		fire := cond
		if !r.rule.Level {
			fire = cond && !r.prevWas
		}
		r.prevWas = cond
		if settling || !gate || !fire || now.Before(r.readyAt) {
			continue
		}

		// Spent effects were retired above, so a non-nil effect is
		// playing and holds the slot against any fire that does not
		// outrank it.
		fired := false
		for _, p := range r.targetPlayers(en) {
			ps := en.player(p)
			if ps.effect != nil && r.rule.Priority <= ps.effect.prio {
				continue
			}
			ps.effect = &activeEffect{
				steps: en.ruleSteps(r),
				start: now,
				owner: i,
				prio:  r.rule.Priority,
			}
			ps.resolve(now)
			fired = true
		}
		if fired {
			cd := r.rule.CooldownMs
			if total := effectLengthMs(&r.rule.Effect); total > cd {
				cd = total
			}
			r.readyAt = now.Add(time.Duration(cd) * time.Millisecond)
		}
	}

	return en.output(holds, mods)
}

// output mixes each player's effect slot, held levels, and multiplier
// into the frame's motor levels. It only reads state, and returns
// players in ascending order, omitting the silent.
func (en *Engine) output(holds map[int]heldLevels, mods map[int]float64) []MotorState {
	ids := make(map[int]bool)
	for p, ps := range en.players {
		if ps.effect != nil {
			ids[p] = true
		}
	}
	for p := range holds {
		ids[p] = true
	}
	if len(ids) == 0 {
		return nil
	}
	sorted := make([]int, 0, len(ids))
	for p := range ids {
		sorted = append(sorted, p)
	}
	sort.Ints(sorted)

	var out []MotorState
	for _, p := range sorted {
		var strong, weak float64
		active := false
		if ps := en.players[p]; ps != nil && ps.effect != nil {
			strong, weak = ps.strong, ps.weak
			active = true
		}
		if h, ok := holds[p]; ok {
			strong = max(strong, h.strong)
			weak = max(weak, h.weak)
			active = true
		}
		if m, ok := mods[p]; ok {
			strong = min(1, strong*m)
			weak = min(1, weak*m)
		}
		if active {
			out = append(out, MotorState{Player: p, Strong: strong, Weak: weak})
		}
	}
	return out
}

// applyClip cancels the effect the rule installed once its condition
// has been false for the clip delay. Only the rule's own effect is
// cancelled: one a higher priority fire replaced belongs to another
// rule.
func (en *Engine) applyClip(idx int, cond bool, now time.Time) {
	r := &en.rules[idx]
	if cond {
		r.condTrueAt = now
		return
	}
	if now.Before(r.condTrueAt.Add(time.Duration(r.rule.ClipMs) * time.Millisecond)) {
		return
	}
	for _, p := range r.targetPlayers(en) {
		ps := en.players[p]
		if ps != nil && ps.effect != nil && ps.effect.owner == idx {
			ps.clear()
		}
	}
}

func (en *Engine) player(p int) *playerState {
	ps := en.players[p]
	if ps == nil {
		ps = &playerState{}
		en.players[p] = ps
	}
	return ps
}

func (r *engineRule) targetPlayers(en *Engine) []int {
	if r.target != nil {
		return r.target
	}
	return en.allPlayers
}

// ruleSteps builds the effect envelope for a fire, applying scale to
// the intensities. The multiplier is a property of the fire, computed
// once and baked into every step; a pattern scales into a fresh slice
// so the authored steps stay unscaled for the next fire.
func (en *Engine) ruleSteps(r *engineRule) []Step {
	e := &r.rule.Effect
	if e.Kind == EffectPattern {
		if r.rule.Scale == nil {
			return e.Steps
		}
		mul := en.scaleMultiplier(r)
		steps := make([]Step, len(e.Steps))
		for i, s := range e.Steps {
			steps[i] = Step{
				Strong:     min(1, s.Strong*mul),
				Weak:       min(1, s.Weak*mul),
				DurationMs: s.DurationMs,
			}
		}
		return steps
	}
	strong, weak := e.Strong, e.Weak
	if r.rule.Scale != nil {
		mul := en.scaleMultiplier(r)
		strong = min(1, strong*mul)
		weak = min(1, weak*mul)
	}
	return []Step{{Strong: strong, Weak: weak, DurationMs: e.DurationMs}}
}

func (en *Engine) scaleMultiplier(r *engineRule) float64 {
	v := &en.watches[r.scaleIdx]
	sc := r.rule.Scale
	var mag uint32
	switch {
	case r.useByDelta:
		mag = r.byDelta
	case !v.valid:
		// An unresolved magnitude watch clamps to the bottom of the
		// range.
		mag = r.magMin
	case sc.Watch != "":
		mag = v.cur
	case v.cur >= v.prev:
		mag = v.cur - v.prev
	default:
		mag = v.prev - v.cur
	}
	if mag < r.magMin {
		mag = r.magMin
	}
	if mag > r.magMax {
		mag = r.magMax
	}
	t := float64(mag-r.magMin) / float64(r.magMax-r.magMin)
	return sc.MulMin + t*(sc.MulMax-sc.MulMin)
}

func effectLengthMs(e *Effect) int {
	if e.Kind == EffectPattern {
		total := 0
		for _, s := range e.Steps {
			total += s.DurationMs
		}
		return total
	}
	return e.DurationMs
}

// effectLevels returns the motor levels of a playing effect at now, or
// playing=false once the effect has run out.
func effectLevels(e *activeEffect, now time.Time) (strong, weak float64, playing bool) {
	elapsed := now.Sub(e.start)
	for _, s := range e.steps {
		d := time.Duration(s.DurationMs) * time.Millisecond
		if elapsed < d {
			return s.Strong, s.Weak, true
		}
		elapsed -= d
	}
	return 0, 0, false
}

// prevPlus and prevMinus shift the previous value by an exact delta in
// the watch's wrap arithmetic: the decimal capacity on a bcd watch, the
// width otherwise. The bcd operand reduces first so the sum cannot
// overflow 32 bits.
func (v *engineWatch) prevPlus(operand uint32) uint32 {
	if v.bcdMod != 0 {
		return (v.prev + operand%v.bcdMod) % v.bcdMod
	}
	return (v.prev + operand) & v.widthMask
}

func (v *engineWatch) prevMinus(operand uint32) uint32 {
	if v.bcdMod != 0 {
		return (v.prev + v.bcdMod - operand%v.bcdMod) % v.bcdMod
	}
	return (v.prev - operand) & v.widthMask
}

// update stores a resolved frame's value. sameSource is false when the
// value came from a different place than last frame (a moved pointer or
// a different slot), which re-seeds so no previous-frame condition can
// fire across the move.
func (v *engineWatch) update(nv uint32, sameSource bool) {
	if v.wasValid && sameSource {
		v.prev = v.cur
	} else {
		v.prev = nv
	}
	v.cur = nv
	v.valid = true
	v.wasValid = true
}

func (v *engineWatch) unresolve() {
	v.valid = false
	v.wasValid = false
}

// pointerTarget resolves a pointer read against the engine's regions:
// the target is the pointer plus the watch's offset, rejected when the
// pointer is 0, the sum overflows 32 bits, or the target plus width
// leaves every known region.
func (en *Engine) pointerTarget(v *engineWatch, ptr uint32) (uint32, bool) {
	target := uint64(ptr) + uint64(v.val.Offset)
	if ptr == 0 || target >= 1<<32 {
		return 0, false
	}
	if !en.inRegion(uint32(target), uint32(v.val.Width/8)) {
		return 0, false
	}
	return uint32(target), true
}

// readPointer reads a pointer watch: the pointer cell, then the value
// at its target. A moved pointer re-seeds the watch.
func (en *Engine) readPointer(mem Reader, v *engineWatch) {
	ptr, ok := v.readLong(mem, v.addr)
	if !ok {
		v.unresolve()
		return
	}
	target, ok := en.pointerTarget(v, ptr)
	if !ok {
		v.unresolve()
		return
	}
	same := ptr == v.lastPtr
	v.lastPtr = ptr
	nv, ok := v.readAt(mem, target)
	if !ok {
		v.unresolve()
		return
	}
	v.update(nv, same)
}

// readKeyed reads a keyed slot watch. The selection is sticky: the
// previously selected slot is kept while its key still matches, and
// only on a mismatch is the pool rescanned for the lowest-indexed
// match. A selection change re-seeds the watch.
func (en *Engine) readKeyed(mem Reader, v *engineWatch) {
	slot := v.slot
	if slot < 0 || !v.keyMatches(mem, slot) {
		slot = -1
		for j := 0; j < v.val.Count; j++ {
			if v.keyMatches(mem, j) {
				slot = j
				break
			}
		}
	}
	if slot < 0 {
		v.slot = -1
		v.unresolve()
		return
	}
	same := slot == v.slot
	v.slot = slot
	fieldAddr := v.addr + uint32(slot)*v.val.Stride + v.val.FieldOffset
	if !v.val.FieldPtr {
		if nv, ok := v.readAt(mem, fieldAddr); ok {
			v.update(nv, same)
		} else {
			v.unresolve()
		}
		return
	}
	ptr, ok := v.readLong(mem, fieldAddr)
	if !ok {
		v.unresolve()
		return
	}
	target, ok := en.pointerTarget(v, ptr)
	if !ok {
		v.unresolve()
		return
	}
	same = same && ptr == v.lastPtr
	v.lastPtr = ptr
	nv, ok := v.readAt(mem, target)
	if !ok {
		v.unresolve()
		return
	}
	v.update(nv, same)
}

func (v *engineWatch) slotKeyAddr(slot int) uint32 {
	return v.addr + uint32(slot)*v.val.Stride + v.val.KeyOffset
}

// keyMatches reports whether the slot's key long reads whole and
// equals the watch's key value. A read the host cannot serve is not a
// match: the bytes that did arrive can carry the key value with the
// rest missing.
func (v *engineWatch) keyMatches(mem Reader, slot int) bool {
	key, ok := v.readLong(mem, v.slotKeyAddr(slot))
	return ok && key == v.val.KeyValue
}

// inRegion reports whether addr through addr+size-1 lies inside one
// known memory region.
func (en *Engine) inRegion(addr, size uint32) bool {
	for i := range en.regions {
		r := &en.regions[i]
		if addr >= r.Start && addr-r.Start < r.Size {
			return uint64(size) <= uint64(r.Size)-uint64(addr-r.Start)
		}
	}
	return false
}

// readLong reads a 32-bit value in the system byte order, unmasked:
// the shape of a pointer or a slot key. ok is false on a short read.
func (v *engineWatch) readLong(mem Reader, addr uint32) (uint32, bool) {
	buf := &v.buf
	if mem.ReadMemory(addr, buf[:]) != 4 {
		return 0, false
	}
	if v.sysBig {
		return uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3]), true
	}
	return uint32(buf[3])<<24 | uint32(buf[2])<<16 | uint32(buf[1])<<8 | uint32(buf[0]), true
}

// readAt reads and interprets the watch's value at addr. ok is false
// on a short read, and on a bcd watch reading a nibble above 9.
func (v *engineWatch) readAt(mem Reader, addr uint32) (val uint32, ok bool) {
	buf := &v.buf
	n := v.val.Width / 8
	if mem.ReadMemory(addr, buf[:n]) != uint32(n) {
		return 0, false
	}
	if v.bigEndian {
		for i := 0; i < n; i++ {
			val = val<<8 | uint32(buf[i])
		}
	} else {
		for i := n - 1; i >= 0; i-- {
			val = val<<8 | uint32(buf[i])
		}
	}
	if v.val.HasMask {
		val &= v.val.Mask
	}
	switch {
	case v.val.Signed:
		val = signExtend(val, v.val.Width) ^ signBias
	case v.val.Abs:
		s := signExtend(val, v.val.Width)
		if s&signBias != 0 {
			s = -s // |INT32_MIN| stays 0x80000000
		}
		val = s
	case v.val.BCD:
		return decodeBCD(val, v.val.Width)
	}
	return val, true
}

// decodeBCD converts packed BCD to its decimal value, high nibble
// first, reporting failure on a nibble above 9.
func decodeBCD(v uint32, width int) (uint32, bool) {
	var out uint32
	for shift := width - 4; shift >= 0; shift -= 4 {
		d := (v >> uint(shift)) & 0xF
		if d > 9 {
			return 0, false
		}
		out = out*10 + d
	}
	return out, true
}

// signExtend widens a masked value to 32 bits from the sign bit of its
// declared width.
func signExtend(v uint32, width int) uint32 {
	if width < 32 && v&(1<<uint(width-1)) != 0 {
		v |= ^uint32(0) << uint(width)
	}
	return v
}

// compileExpr lowers an expression to a closure over watch, def, and
// event indexes. Name resolution already happened in validation, so
// lookups here cannot fail. arrays holds the names of array watches,
// whose constant conditions test whether any slot matches. signed
// holds the names of signed watches, whose values are stored biased:
// their value operands are biased to match, while delta operands stay
// as written because the bias preserves differences.
func compileExpr(x Expr, arrays, signed map[string]bool, watchIdx, defIdx, eventIdx map[string]int) compiledExpr {
	switch n := x.(type) {
	case *BinaryExpr:
		left := compileExpr(n.Left, arrays, signed, watchIdx, defIdx, eventIdx)
		right := compileExpr(n.Right, arrays, signed, watchIdx, defIdx, eventIdx)
		if n.Op == OpAnd {
			return func(en *Engine) bool { return left(en) && right(en) }
		}
		return func(en *Engine) bool { return left(en) || right(en) }
	case *CompareCond:
		idx := watchIdx[n.Watch]
		operand := n.Operand
		if signed[n.Watch] {
			operand ^= signBias
		}
		cmp := compareFn(n.Op)
		if arrays[n.Watch] {
			return func(en *Engine) bool {
				v := &en.watches[idx]
				for i, s := range v.slots {
					if !v.slotsValid[i] {
						continue
					}
					if cmp(s, operand) {
						return true
					}
				}
				return false
			}
		}
		return func(en *Engine) bool {
			v := &en.watches[idx]
			return v.valid && cmp(v.cur, operand)
		}
	case *CompareWatchCond:
		// Both sides share one interpretation, so no operand bias
		// applies.
		li, ri := watchIdx[n.Left], watchIdx[n.Right]
		cmp := compareFn(n.Op)
		return func(en *Engine) bool {
			l, r := &en.watches[li], &en.watches[ri]
			return l.valid && r.valid && cmp(l.cur, r.cur)
		}
	case *SetCond:
		idx := watchIdx[n.Watch]
		set := n.Set
		if signed[n.Watch] {
			set = make([]uint32, len(n.Set))
			for i, s := range n.Set {
				set[i] = s ^ signBias
			}
		}
		negate := n.Negate
		inSet := func(v uint32) bool {
			for _, s := range set {
				if v == s {
					return true
				}
			}
			return false
		}
		if arrays[n.Watch] {
			return func(en *Engine) bool {
				v := &en.watches[idx]
				for i, s := range v.slots {
					if !v.slotsValid[i] {
						continue
					}
					if inSet(s) != negate {
						return true
					}
				}
				return false
			}
		}
		return func(en *Engine) bool {
			v := &en.watches[idx]
			return v.valid && inSet(v.cur) != negate
		}
	case *PrevCond:
		idx := watchIdx[n.Watch]
		operand := n.Operand
		if signed[n.Watch] && (n.Qual == QualFrom || n.Qual == QualTo) {
			operand ^= signBias
		}
		var test func(v *engineWatch) bool
		switch n.Qual {
		case QualBy:
			if n.Op == OpIncreased {
				test = func(v *engineWatch) bool { return v.cur == v.prevPlus(operand) }
			} else {
				test = func(v *engineWatch) bool { return v.cur == v.prevMinus(operand) }
			}
		case QualAtLeast:
			// The bounds do not wrap: the value must rise or fall in
			// the plain unsigned comparison, and the bound applies to
			// the plain difference.
			if n.Op == OpIncreased {
				test = func(v *engineWatch) bool { return v.cur > v.prev && v.cur-v.prev >= operand }
			} else {
				test = func(v *engineWatch) bool { return v.cur < v.prev && v.prev-v.cur >= operand }
			}
		case QualAtMost:
			if n.Op == OpIncreased {
				test = func(v *engineWatch) bool { return v.cur > v.prev && v.cur-v.prev <= operand }
			} else {
				test = func(v *engineWatch) bool { return v.cur < v.prev && v.prev-v.cur <= operand }
			}
		case QualFrom:
			switch n.Op {
			case OpChanged:
				test = func(v *engineWatch) bool { return v.prev == operand && v.cur != operand }
			case OpIncreased:
				test = func(v *engineWatch) bool { return v.prev == operand && v.cur > operand }
			default:
				test = func(v *engineWatch) bool { return v.prev == operand && v.cur < operand }
			}
		case QualTo:
			switch n.Op {
			case OpChanged:
				test = func(v *engineWatch) bool { return v.cur == operand && v.prev != operand }
			case OpIncreased:
				test = func(v *engineWatch) bool { return v.cur == operand && v.prev < operand }
			default:
				test = func(v *engineWatch) bool { return v.cur == operand && v.prev > operand }
			}
		default:
			switch n.Op {
			case OpChanged:
				test = func(v *engineWatch) bool { return v.cur != v.prev }
			case OpUnchanged:
				test = func(v *engineWatch) bool { return v.cur == v.prev }
			case OpIncreased:
				test = func(v *engineWatch) bool { return v.cur > v.prev }
			default:
				test = func(v *engineWatch) bool { return v.cur < v.prev }
			}
		}
		return func(en *Engine) bool {
			v := &en.watches[idx]
			return v.valid && test(v)
		}
	case *DefRef:
		if idx, ok := defIdx[n.Name]; ok {
			if n.Negate {
				return func(en *Engine) bool { return en.defValsNeg[idx] }
			}
			return func(en *Engine) bool { return en.defVals[idx] }
		}
		idx := eventIdx[n.Name]
		if n.Negate {
			return func(en *Engine) bool { return !en.eventVals[idx] }
		}
		return func(en *Engine) bool { return en.eventVals[idx] }
	}
	return func(*Engine) bool { return false }
}

// compileNegExpr lowers the negation of a def body, pushing the "not"
// down to the leaves: and/or swap by De Morgan, a comparison compiles
// as its complement so an unresolved watch still makes the term false,
// an array condition requires that no slot match, and a nested
// reference reads the opposite polarity. Def bodies hold only constant
// comparisons, so previous-frame conditions cannot appear here.
func compileNegExpr(x Expr, arrays, signed map[string]bool, watchIdx, defIdx, eventIdx map[string]int) compiledExpr {
	switch n := x.(type) {
	case *BinaryExpr:
		left := compileNegExpr(n.Left, arrays, signed, watchIdx, defIdx, eventIdx)
		right := compileNegExpr(n.Right, arrays, signed, watchIdx, defIdx, eventIdx)
		if n.Op == OpAnd {
			return func(en *Engine) bool { return left(en) || right(en) }
		}
		return func(en *Engine) bool { return left(en) && right(en) }
	case *CompareCond:
		if arrays[n.Watch] {
			pos := compileExpr(n, arrays, signed, watchIdx, defIdx, eventIdx)
			return func(en *Engine) bool { return !pos(en) }
		}
		c := *n
		c.Op = complementOp(n.Op)
		return compileExpr(&c, arrays, signed, watchIdx, defIdx, eventIdx)
	case *CompareWatchCond:
		c := *n
		c.Op = complementOp(n.Op)
		return compileExpr(&c, arrays, signed, watchIdx, defIdx, eventIdx)
	case *SetCond:
		if arrays[n.Watch] {
			pos := compileExpr(n, arrays, signed, watchIdx, defIdx, eventIdx)
			return func(en *Engine) bool { return !pos(en) }
		}
		c := *n
		c.Negate = !c.Negate
		return compileExpr(&c, arrays, signed, watchIdx, defIdx, eventIdx)
	case *DefRef:
		c := *n
		c.Negate = !c.Negate
		return compileExpr(&c, arrays, signed, watchIdx, defIdx, eventIdx)
	}
	return func(*Engine) bool { return false }
}

func complementOp(op CompareOp) CompareOp {
	switch op {
	case OpEq:
		return OpNe
	case OpNe:
		return OpEq
	case OpLt:
		return OpGe
	case OpGe:
		return OpLt
	case OpGt:
		return OpLe
	default:
		return OpGt
	}
}

func compareFn(op CompareOp) func(a, b uint32) bool {
	switch op {
	case OpEq:
		return func(a, b uint32) bool { return a == b }
	case OpNe:
		return func(a, b uint32) bool { return a != b }
	case OpLt:
		return func(a, b uint32) bool { return a < b }
	case OpGt:
		return func(a, b uint32) bool { return a > b }
	case OpLe:
		return func(a, b uint32) bool { return a <= b }
	default:
		return func(a, b uint32) bool { return a >= b }
	}
}

// findPrevCond returns the expression's single previous-frame condition.
// Validation guarantees exactly one exists on scaled rules.
func findPrevCond(x Expr) *PrevCond {
	switch n := x.(type) {
	case *PrevCond:
		return n
	case *BinaryExpr:
		if c := findPrevCond(n.Left); c != nil {
			return c
		}
		return findPrevCond(n.Right)
	}
	return nil
}
