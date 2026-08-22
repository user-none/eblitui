package rumble

import (
	"math"
	"testing"
	"time"
)

type fakeMem struct {
	data [64]byte
}

func (m *fakeMem) ReadMemory(addr uint32, buf []byte) uint32 {
	return uint32(copy(buf, m.data[addr:]))
}

// shortMem truncates reads from an address onward, modelling a host
// whose memory view cannot serve a read the engine validated as
// in-region.
type shortMem struct {
	fakeMem
	from uint32 // reads at or above this address are truncated
	max  int    // bytes served for a truncated read
}

func (m *shortMem) ReadMemory(addr uint32, buf []byte) uint32 {
	if addr >= m.from && len(buf) > m.max {
		buf = buf[:m.max]
	}
	return m.fakeMem.ReadMemory(addr, buf)
}

const frame = 16 * time.Millisecond

func byteVal(name string, addr uint32) Watch {
	return Watch{Name: name, Width: 8, Address: addr}
}

func pulseRule(on Expr, strong, weak float64, durMs int) Rule {
	return Rule{
		On:     on,
		Effect: Effect{Kind: EffectPulse, Strong: strong, Weak: weak, DurationMs: durMs},
		Player: 1,
	}
}

func decreased(name string) Expr {
	return &PrevCond{Watch: name, Op: OpDecreased}
}

func eq(name string, n uint32) Expr {
	return &CompareCond{Watch: name, Op: OpEq, Operand: n}
}

func TestEngineMetadata(t *testing.T) {
	src := "game: Sky Runner\ngameid: T-99901G\nsystem: saturn\nrevision: 4\n---\n" +
		"watch health byte 0x10\n" +
		"on health decreased: pulse 1.0 100ms\n"
	rs, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	en := buildEngine(t, *rs, 1)
	want := Metadata{Game: "Sky Runner", GameID: "T-99901G", System: "saturn", Revision: 4}
	if got := en.Metadata(); got != want {
		t.Fatalf("metadata = %+v, want %+v", got, want)
	}
}

func buildEngine(t *testing.T, rs Ruleset, players int) *Engine {
	t.Helper()
	sys := System{
		BigEndian: true,
		Regions:   []Region{{Name: "RAM", Start: 0, Size: 64}},
	}
	en, err := NewEngine(&rs, sys, players)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	return en
}

// warmup runs the settle window and asserts it stays silent.
func warmup(t *testing.T, en *Engine, mem Reader, start time.Time) time.Time {
	t.Helper()
	tt := start
	for i := 0; i < settleFrames; i++ {
		if out := en.Evaluate(mem, tt); out != nil {
			t.Fatalf("output during settle frame %d: %+v", i, out)
		}
		tt = tt.Add(frame)
	}
	return tt
}

func wantMotors(t *testing.T, out []MotorState, player int, strong, weak float64) {
	t.Helper()
	for _, ms := range out {
		if ms.Player != player {
			continue
		}
		if math.Abs(ms.Strong-strong) > 1e-9 || math.Abs(ms.Weak-weak) > 1e-9 {
			t.Fatalf("player %d = %.3f/%.3f, want %.3f/%.3f", player, ms.Strong, ms.Weak, strong, weak)
		}
		return
	}
	t.Fatalf("player %d absent from %+v, want %.3f/%.3f", player, out, strong, weak)
}

func wantSilent(t *testing.T, out []MotorState) {
	t.Helper()
	if len(out) != 0 {
		t.Fatalf("expected no output, got %+v", out)
	}
}

var t0 = time.Unix(1000, 0)

func TestEdgePulse(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{pulseRule(decreased("x"), 0.8, 0.4, 100)},
	}, 1)
	tt := warmup(t, en, mem, t0)

	wantSilent(t, en.Evaluate(mem, tt))
	mem.data[0] = 9
	fireAt := tt.Add(frame)
	wantMotors(t, en.Evaluate(mem, fireAt), 1, 0.8, 0.4)
	wantMotors(t, en.Evaluate(mem, fireAt.Add(frame)), 1, 0.8, 0.4)
	wantSilent(t, en.Evaluate(mem, fireAt.Add(150*time.Millisecond)))
}

func TestEdgeNoRefireOnContinuousDrain(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{pulseRule(decreased("x"), 0.8, 0.8, 20)},
	}, 1)
	tt := warmup(t, en, mem, t0)

	step := 30 * time.Millisecond
	mem.data[0] = 9
	if out := en.Evaluate(mem, tt); len(out) == 0 {
		t.Fatal("first drop did not fire")
	}
	mem.data[0] = 8
	wantSilent(t, en.Evaluate(mem, tt.Add(step))) // still draining: no fresh transition
	wantSilent(t, en.Evaluate(mem, tt.Add(2*step)))
	mem.data[0] = 7
	if out := en.Evaluate(mem, tt.Add(3*step)); len(out) == 0 {
		t.Fatal("fresh transition did not fire")
	}
}

func TestLevelHeartbeat(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 1
	rule := pulseRule(eq("x", 1), 0.5, 0.5, 20)
	rule.Level = true
	rule.CooldownMs = 100
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5)
	wantMotors(t, en.Evaluate(mem, tt.Add(16*time.Millisecond)), 1, 0.5, 0.5)
	wantSilent(t, en.Evaluate(mem, tt.Add(32*time.Millisecond))) // pulse over, cooldown running
	wantSilent(t, en.Evaluate(mem, tt.Add(96*time.Millisecond)))
	wantMotors(t, en.Evaluate(mem, tt.Add(112*time.Millisecond)), 1, 0.5, 0.5) // cooldown lapsed
}

func TestSettleSeedsEdgeState(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 1
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{pulseRule(eq("x", 1), 0.5, 0.5, 20)},
	}, 1)
	tt := warmup(t, en, mem, t0)

	// Condition was true throughout settle: no stale edge fire.
	for i := 0; i < 5; i++ {
		wantSilent(t, en.Evaluate(mem, tt))
		tt = tt.Add(frame)
	}
	mem.data[0] = 0
	wantSilent(t, en.Evaluate(mem, tt))
	mem.data[0] = 1
	if out := en.Evaluate(mem, tt.Add(frame)); len(out) == 0 {
		t.Fatal("fresh transition after settle did not fire")
	}
}

func TestGateKeepsEdgeTracking(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10 // x
	mem.data[1] = 0  // g
	rule := pulseRule(decreased("x"), 0.8, 0.8, 20)
	rule.While = eq("g", 1)
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0), byteVal("g", 1)},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9 // transition while gated off
	wantSilent(t, en.Evaluate(mem, tt))
	mem.data[1] = 1 // gate opens: the gated-off transition must not fire
	wantSilent(t, en.Evaluate(mem, tt.Add(frame)))
	wantSilent(t, en.Evaluate(mem, tt.Add(2*frame)))
	mem.data[0] = 8 // fresh transition with the gate open
	if out := en.Evaluate(mem, tt.Add(3*frame)); len(out) == 0 {
		t.Fatal("fresh transition did not fire")
	}
}

func TestFirstWinsSameFrame(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules: []Rule{
			pulseRule(decreased("x"), 0.7, 0, 100),
			pulseRule(decreased("x"), 0, 0.9, 100),
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.7, 0)
}

func TestPlayingEffectBlocksEqualPriority(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 0 // x
	mem.data[1] = 5 // y
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0), byteVal("y", 1)},
		Rules: []Rule{
			pulseRule(eq("x", 1), 0.8, 0.8, 200),
			pulseRule(decreased("y"), 0.2, 0.2, 100),
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 1
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.8, 0.8)
	mem.data[1] = 4 // fires while the first effect plays: dropped
	wantMotors(t, en.Evaluate(mem, tt.Add(2*frame)), 1, 0.8, 0.8)
	// The dropped fire is gone, not deferred.
	wantSilent(t, en.Evaluate(mem, tt.Add(250*time.Millisecond)))
}

func TestHoldMixesUnderDiscrete(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 0  // s
	mem.data[1] = 10 // x
	hold := Rule{
		On:     eq("s", 3),
		Effect: Effect{Kind: EffectHold, Strong: 0.3, Weak: 0.6},
		Player: 1,
	}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("s", 0), byteVal("x", 1)},
		Rules: []Rule{
			hold,
			pulseRule(decreased("x"), 0.9, 0.1, 100),
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 3
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.3, 0.6)
	mem.data[1] = 9 // pulse over the hold: per-motor max
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 0.9, 0.6)
	// Pulse over: back to the hold levels.
	wantMotors(t, en.Evaluate(mem, tt.Add(150*time.Millisecond)), 1, 0.3, 0.6)
	mem.data[0] = 0 // released the frame the condition goes false
	wantSilent(t, en.Evaluate(mem, tt.Add(160*time.Millisecond)))
}

func TestFirstHoldWinsAtEqualPriority(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 3
	first := Rule{On: eq("s", 3), Effect: Effect{Kind: EffectHold, Strong: 0.3, Weak: 0.3}, Player: 1}
	second := Rule{On: eq("s", 3), Effect: Effect{Kind: EffectHold, Strong: 0.9, Weak: 0.9}, Player: 1}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("s", 0)},
		Rules:   []Rule{first, second},
	}, 1)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 1, 0.3, 0.3)
}

// A dampen tracks its condition frame by frame: a playing pulse
// attenuates the frame the condition goes true and recovers the frame
// it goes false.
func TestDampenScalesOutputMidEffect(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10 // x
	mem.data[1] = 0  // env
	dampen := Rule{On: eq("env", 1), Effect: Effect{Kind: EffectDampen, Percent: 50}, Player: 1}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0), byteVal("env", 1)},
		Rules:   []Rule{dampen, pulseRule(decreased("x"), 0.8, 0.4, 200)},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.8, 0.4)
	mem.data[1] = 1 // condition true mid-pulse: output attenuates
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 0.4, 0.2)
	mem.data[1] = 0 // condition false: full strength resumes
	wantMotors(t, en.Evaluate(mem, tt.Add(2*frame)), 1, 0.8, 0.4)
}

func TestAmplifyRaisesAndClamps(t *testing.T) {
	mem := &fakeMem{}
	amplify := Rule{On: eq("s", 3), Effect: Effect{Kind: EffectAmplify, Percent: 50}, Player: 1}
	hold := Rule{On: eq("s", 3), Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.8}, Player: 1}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("s", 0)},
		Rules:   []Rule{amplify, hold},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 3
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.75, 1.0) // weak clamps at 1.0
}

// Dampen and amplify share one first-wins pool: the earliest active
// rule in the file provides the multiplier.
func TestFirstModifierWins(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 3
	dampen := Rule{On: eq("s", 3), Effect: Effect{Kind: EffectDampen, Percent: 50}, Player: 1}
	amplify := Rule{On: eq("s", 3), Effect: Effect{Kind: EffectAmplify, Percent: 100}, Player: 1}
	hold := Rule{On: eq("s", 3), Effect: Effect{Kind: EffectHold, Strong: 0.4, Weak: 0.4}, Player: 1}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("s", 0)},
		Rules:   []Rule{dampen, amplify, hold},
	}, 1)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 1, 0.2, 0.2)
}

func TestDampenFullMutes(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 3
	dampen := Rule{On: eq("s", 3), Effect: Effect{Kind: EffectDampen, Percent: 100}, Player: 1}
	hold := Rule{On: eq("s", 3), Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5}, Player: 1}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("s", 0)},
		Rules:   []Rule{dampen, hold},
	}, 1)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 1, 0, 0)
}

// A modifier attenuates only its own player's output.
func TestModifierPerPlayer(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 3
	dampen := Rule{On: eq("s", 3), Effect: Effect{Kind: EffectDampen, Percent: 50}, Player: 2}
	hold := Rule{On: eq("s", 3), Effect: Effect{Kind: EffectHold, Strong: 0.6, Weak: 0.6}, PlayerAll: true}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("s", 0)},
		Rules:   []Rule{dampen, hold},
	}, 2)
	tt := warmup(t, en, mem, t0)

	out := en.Evaluate(mem, tt)
	wantMotors(t, out, 1, 0.6, 0.6)
	wantMotors(t, out, 2, 0.3, 0.3)
}

// Negating a def negates its body by De Morgan: either failing leg of
// an "and" makes the negation true.
func TestNegatedDefDeMorgan(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 1 // a
	mem.data[1] = 2 // b
	rs := Ruleset{
		Watches: []Watch{byteVal("a", 0), byteVal("b", 1)},
		Defs: []Def{{Name: "d", Expr: &BinaryExpr{Op: OpAnd,
			Left:  &CompareCond{Watch: "a", Op: OpEq, Operand: 1},
			Right: &CompareCond{Watch: "b", Op: OpEq, Operand: 2}}}},
		Rules: []Rule{{
			On:     &DefRef{Name: "d", Negate: true},
			Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5},
			Player: 1,
		}},
	}
	en := buildEngine(t, rs, 1)
	tt := warmup(t, en, mem, t0)

	wantSilent(t, en.Evaluate(mem, tt)) // d true: not d false
	mem.data[1] = 3
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 0.5, 0.5)
	mem.data[0], mem.data[1] = 0, 2
	wantMotors(t, en.Evaluate(mem, tt.Add(2*frame)), 1, 0.5, 0.5)
}

// An unresolved watch makes a def and its negation both false: neither
// polarity's hold runs until the pointer resolves.
func TestNegatedDefUnresolvedStaysFalse(t *testing.T) {
	mem := &fakeMem{}
	rs := Ruleset{
		Watches: []Watch{{Name: "p", Width: 8, Address: 0x20, Pointer: true}},
		Defs:    []Def{{Name: "low", Expr: &CompareCond{Watch: "p", Op: OpLt, Operand: 10}}},
		Rules: []Rule{
			{On: &DefRef{Name: "low"}, Effect: Effect{Kind: EffectHold, Strong: 0.3, Weak: 0.3}, Player: 1},
			{On: &DefRef{Name: "low", Negate: true}, Effect: Effect{Kind: EffectHold, Strong: 0.7, Weak: 0.7}, Player: 1},
		},
	}
	en := buildEngine(t, rs, 1)
	tt := warmup(t, en, mem, t0)

	wantSilent(t, en.Evaluate(mem, tt)) // pointer 0: unresolved
	mem.data[0x23] = 0x10               // pointer -> 0x10
	mem.data[0x10] = 5
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 0.3, 0.3)
	mem.data[0x10] = 20
	wantMotors(t, en.Evaluate(mem, tt.Add(2*frame)), 1, 0.7, 0.7)
	mem.data[0x23] = 0 // pointer cleared: both polarities silent again
	wantSilent(t, en.Evaluate(mem, tt.Add(3*frame)))
}

// A negated array condition asks that no slot match.
func TestNegatedDefArrayNoSlotMatches(t *testing.T) {
	mem := &fakeMem{}
	rs := Ruleset{
		Watches: []Watch{{Name: "pool", Width: 8, Address: 0x10, Stride: 1, Count: 4}},
		Defs:    []Def{{Name: "has", Expr: &CompareCond{Watch: "pool", Op: OpEq, Operand: 40}}},
		Rules: []Rule{{
			On:     &DefRef{Name: "has", Negate: true},
			Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5},
			Player: 1,
		}},
	}
	en := buildEngine(t, rs, 1)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5) // no slot is 40
	mem.data[0x12] = 40
	wantSilent(t, en.Evaluate(mem, tt.Add(frame)))
	mem.data[0x12] = 0
	wantMotors(t, en.Evaluate(mem, tt.Add(2*frame)), 1, 0.5, 0.5)
}

// Negation recurses through nested def references with the polarity
// flipped: not b with b = a and y == 2 is (not a) or y != 2.
func TestNegatedDefNested(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 1 // x
	mem.data[1] = 2 // y
	rs := Ruleset{
		Watches: []Watch{byteVal("x", 0), byteVal("y", 1)},
		Defs: []Def{
			{Name: "a", Expr: &CompareCond{Watch: "x", Op: OpEq, Operand: 1}},
			{Name: "b", Expr: &BinaryExpr{Op: OpAnd,
				Left:  &DefRef{Name: "a"},
				Right: &CompareCond{Watch: "y", Op: OpEq, Operand: 2}}},
		},
		Rules: []Rule{{
			On:     &DefRef{Name: "b", Negate: true},
			Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5},
			Player: 1,
		}},
	}
	en := buildEngine(t, rs, 1)
	tt := warmup(t, en, mem, t0)

	wantSilent(t, en.Evaluate(mem, tt)) // b true
	mem.data[1] = 3
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 0.5, 0.5)
	mem.data[0], mem.data[1] = 0, 2
	wantMotors(t, en.Evaluate(mem, tt.Add(2*frame)), 1, 0.5, 0.5)
}

// A negated event is true while its window is closed.
func TestNegatedEvent(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10
	rs := Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Events:  []Event{{Name: "ev", Trigger: PrevCond{Watch: "x", Op: OpDecreased}, DurationMs: 100}},
		Rules: []Rule{{
			On:     &DefRef{Name: "ev", Negate: true},
			Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5},
			Player: 1,
		}},
	}
	en := buildEngine(t, rs, 1)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5) // window closed
	mem.data[0] = 9                                  // window opens
	wantSilent(t, en.Evaluate(mem, tt.Add(frame)))
	wantMotors(t, en.Evaluate(mem, tt.Add(200*time.Millisecond)), 1, 0.5, 0.5)
}

// A scaled pattern bakes the fire-time multiplier into every step's
// intensities, leaving durations and off steps alone. A second fire
// at a different magnitude scales the authored steps, not the
// previous fire's scaled copy.
func TestScaledPattern(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10 // x: trigger
	mem.data[1] = 5  // mag
	rule := Rule{
		On: decreased("x"),
		Effect: Effect{Kind: EffectPattern, Steps: []Step{
			{Strong: 1, Weak: 0.8, DurationMs: 100},
			{DurationMs: 50},
			{Strong: 0.5, Weak: 0.5, DurationMs: 100},
		}},
		Scale:  &Scale{Watch: "mag", MagMin: 0, MagMax: 10, MulMin: 0, MulMax: 1},
		Player: 1,
	}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0), byteVal("mag", 1)},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9 // fire at magnitude 5: multiplier 0.5
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.4)
	wantMotors(t, en.Evaluate(mem, tt.Add(120*time.Millisecond)), 1, 0, 0) // off step stays off
	wantMotors(t, en.Evaluate(mem, tt.Add(200*time.Millisecond)), 1, 0.25, 0.25)

	mem.data[0] = 8 // fire at magnitude 10: full authored levels
	mem.data[1] = 10
	wantMotors(t, en.Evaluate(mem, tt.Add(300*time.Millisecond)), 1, 1, 0.8)
}

// A held event is an on-delay: true once its condition has held for
// the frame count, false the frame it breaks, and the count restarts
// from zero on the next stretch.
func TestHeldEventDelaysOn(t *testing.T) {
	mem := &fakeMem{}
	rs := Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Events:  []Event{{Name: "e", Held: true, Cond: eq("x", 3), HeldFrames: 3}},
		Rules: []Rule{
			{On: &DefRef{Name: "e"}, Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5}, Player: 1},
			{On: &DefRef{Name: "e", Negate: true}, Effect: Effect{Kind: EffectHold, Strong: 0.2, Weak: 0.2}, Player: 2},
		},
	}
	en := buildEngine(t, rs, 2)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 2, 0.2, 0.2) // not held yet
	mem.data[0] = 3
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 2, 0.2, 0.2)   // 1 frame
	wantMotors(t, en.Evaluate(mem, tt.Add(2*frame)), 2, 0.2, 0.2) // 2 frames
	out := en.Evaluate(mem, tt.Add(3*frame))                      // 3 frames: event true
	wantMotors(t, out, 1, 0.5, 0.5)
	for _, ms := range out {
		if ms.Player == 2 {
			t.Fatalf("negated event still active: %+v", out)
		}
	}
	wantMotors(t, en.Evaluate(mem, tt.Add(4*frame)), 1, 0.5, 0.5) // stays true
	mem.data[0] = 0                                               // breaks: false immediately
	wantMotors(t, en.Evaluate(mem, tt.Add(5*frame)), 2, 0.2, 0.2)
	mem.data[0] = 3 // count restarted from zero
	wantMotors(t, en.Evaluate(mem, tt.Add(6*frame)), 2, 0.2, 0.2)
}

// The held count runs during the settle window, so state that held
// through settle is honest the moment it ends.
func TestHeldEventAccumulatesDuringSettle(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 3
	rs := Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Events:  []Event{{Name: "e", Held: true, Cond: eq("x", 3), HeldFrames: 5}},
		Rules: []Rule{{
			On:     &DefRef{Name: "e"},
			Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5},
			Player: 1,
		}},
	}
	en := buildEngine(t, rs, 1)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5)
}

// A held event's condition sees the current frame's value of a def
// declared above it, so the count starts the frame the def goes true.
func TestHeldEventSeesCurrentFrameDef(t *testing.T) {
	mem := &fakeMem{}
	rs := Ruleset{
		Watches: []Watch{{Name: "x", Width: 8, Address: 0, Line: 1}},
		Defs:    []Def{{Name: "d", Expr: eq("x", 1), Line: 2}},
		Events:  []Event{{Name: "e", Held: true, Cond: &DefRef{Name: "d"}, HeldFrames: 2, Line: 3}},
		Rules: []Rule{{
			On:     &DefRef{Name: "e"},
			Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5},
			Player: 1, Line: 4,
		}},
	}
	en := buildEngine(t, rs, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 1
	wantSilent(t, en.Evaluate(mem, tt)) // 1 frame
	// A stale def value would make this frame the first count.
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 0.5, 0.5)
}

// A watch comparison tracks both values frame by frame.
func TestWatchComparison(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 5  // a
	mem.data[1] = 10 // b
	rs := Ruleset{
		Watches: []Watch{byteVal("a", 0), byteVal("b", 1)},
		Rules: []Rule{{
			On:     &CompareWatchCond{Left: "a", Op: OpGt, Right: "b"},
			Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5},
			Player: 1,
		}},
	}
	en := buildEngine(t, rs, 1)
	tt := warmup(t, en, mem, t0)

	wantSilent(t, en.Evaluate(mem, tt))
	mem.data[0] = 11
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 0.5, 0.5)
	mem.data[1] = 11 // equal: strict greater releases
	wantSilent(t, en.Evaluate(mem, tt.Add(2*frame)))
}

// Signed watches compare bias-mapped alike, so ordering is signed.
func TestWatchComparisonSigned(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 0xFB // a = -5
	mem.data[1] = 0x02 // b = 2
	rs := Ruleset{
		Watches: []Watch{
			{Name: "a", Width: 8, Address: 0, Signed: true},
			{Name: "b", Width: 8, Address: 1, Signed: true},
		},
		Rules: []Rule{{
			On:     &CompareWatchCond{Left: "a", Op: OpLt, Right: "b"},
			Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5},
			Player: 1,
		}},
	}
	en := buildEngine(t, rs, 1)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5) // -5 < 2
	mem.data[0] = 0x03                               // 3 > 2
	wantSilent(t, en.Evaluate(mem, tt.Add(frame)))
}

// Either side unresolved makes a watch comparison false, in both a
// def and its negation.
func TestWatchComparisonUnresolved(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0x28] = 10 // cap
	rs := Ruleset{
		Watches: []Watch{
			{Name: "hp", Width: 8, Address: 0x20, Pointer: true},
			byteVal("cap", 0x28),
		},
		Defs: []Def{{Name: "low", Expr: &CompareWatchCond{Left: "hp", Op: OpLt, Right: "cap"}}},
		Rules: []Rule{
			{On: &DefRef{Name: "low"}, Effect: Effect{Kind: EffectHold, Strong: 0.3, Weak: 0.3}, Player: 1},
			{On: &DefRef{Name: "low", Negate: true}, Effect: Effect{Kind: EffectHold, Strong: 0.7, Weak: 0.7}, Player: 1},
		},
	}
	en := buildEngine(t, rs, 1)
	tt := warmup(t, en, mem, t0)

	wantSilent(t, en.Evaluate(mem, tt)) // pointer 0: both polarities false
	mem.data[0x23] = 0x10               // pointer -> 0x10
	mem.data[0x10] = 5
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 0.3, 0.3)
	mem.data[0x10] = 15
	wantMotors(t, en.Evaluate(mem, tt.Add(2*frame)), 1, 0.7, 0.7)
}

// A bcd watch compares its decoded decimal value against plain
// integer operands.
func TestBCDDecodeAndCompare(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0x10] = 0x12
	mem.data[0x11] = 0x34
	rs := Ruleset{
		Watches: []Watch{{Name: "score", Width: 16, Address: 0x10, BCD: true}},
		Rules: []Rule{{
			On:     &CompareCond{Watch: "score", Op: OpEq, Operand: 1234},
			Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5},
			Player: 1,
		}},
	}
	en := buildEngine(t, rs, 1)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5)
	mem.data[0x11] = 0x35 // 1235
	wantSilent(t, en.Evaluate(mem, tt.Add(frame)))
}

// Invalid bcd digits leave the watch unresolved: holds release, and
// the return re-seeds so no change fires across the gap.
func TestBCDInvalidDigitsUnresolve(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 0x42
	rs := Ruleset{
		Watches: []Watch{{Name: "score", Width: 8, Address: 0, BCD: true}},
		Rules: []Rule{
			{On: &CompareCond{Watch: "score", Op: OpEq, Operand: 42},
				Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5}, Player: 1},
			pulseRule(&PrevCond{Watch: "score", Op: OpChanged}, 0.9, 0.9, 50),
		},
	}
	en := buildEngine(t, rs, 1)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5)
	mem.data[0] = 0x4F // invalid: unresolved, hold releases, nothing fires
	wantSilent(t, en.Evaluate(mem, tt.Add(frame)))
	mem.data[0] = 0x50 // return re-seeds: changed does not fire
	wantSilent(t, en.Evaluate(mem, tt.Add(2*frame)))
	mem.data[0] = 0x51 // fresh change fires
	wantMotors(t, en.Evaluate(mem, tt.Add(3*frame)), 1, 0.9, 0.9)
}

// Exact deltas wrap at the decimal capacity; the bounded forms do not
// wrap, per the rule shared with every other watch.
func TestBCDExactDeltaWrapsDecimally(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 0x99
	exact := Rule{
		On:     &PrevCond{Watch: "c", Op: OpIncreased, Qual: QualBy, Operand: 1},
		Effect: Effect{Kind: EffectPulse, Strong: 0.5, Weak: 0.5, DurationMs: 50},
		Player: 1,
	}
	bounded := Rule{
		On:     &PrevCond{Watch: "c", Op: OpIncreased, Qual: QualAtLeast, Operand: 1},
		Effect: Effect{Kind: EffectPulse, Strong: 0.9, Weak: 0.9, DurationMs: 50},
		Player: 2,
	}
	rs := Ruleset{
		Watches: []Watch{{Name: "c", Width: 8, Address: 0, BCD: true}},
		Rules:   []Rule{exact, bounded},
	}
	en := buildEngine(t, rs, 2)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 0x00 // 99 -> 00: increased by exactly 1, wrapped
	out := en.Evaluate(mem, tt)
	wantMotors(t, out, 1, 0.5, 0.5)
	for _, ms := range out {
		if ms.Player == 2 {
			t.Fatalf("bounded delta fired on wrap: %+v", out)
		}
	}
	wantSilent(t, en.Evaluate(mem, tt.Add(60*time.Millisecond))) // steady frame resets the edge
	mem.data[0] = 0x01                                           // 0 -> 1: both forms fire
	out = en.Evaluate(mem, tt.Add(120*time.Millisecond))
	wantMotors(t, out, 1, 0.5, 0.5)
	wantMotors(t, out, 2, 0.9, 0.9)
}

// A short-read slot on a plain array watch has no value: it matches
// no comparison and no set condition while the other slots read
// normally.
func TestArrayShortReadSlotMatchesNothing(t *testing.T) {
	mem := &shortMem{from: 0x12, max: 1}
	mem.data[0x10], mem.data[0x11] = 0, 5       // slot 0: 0x0005
	mem.data[0x12], mem.data[0x13] = 0xFF, 0xFF // slot 1: read short
	rs := Ruleset{
		Watches: []Watch{{Name: "pool", Width: 16, Address: 0x10, Stride: 2, Count: 2}},
		Rules: []Rule{
			{On: &CompareCond{Watch: "pool", Op: OpEq, Operand: 5},
				Effect: Effect{Kind: EffectHold, Strong: 0.3, Weak: 0.3}, Player: 1},
			{On: &CompareCond{Watch: "pool", Op: OpGt, Operand: 100},
				Effect: Effect{Kind: EffectHold, Strong: 0.7, Weak: 0.7}, Player: 2},
			{On: &SetCond{Watch: "pool", Negate: true, Set: []uint32{0, 5}},
				Effect: Effect{Kind: EffectHold, Strong: 0.9, Weak: 0.9}, Player: 3},
		},
	}
	en := buildEngine(t, rs, 3)
	tt := warmup(t, en, mem, t0)

	out := en.Evaluate(mem, tt)
	wantMotors(t, out, 1, 0.3, 0.3)
	for _, ms := range out {
		if ms.Player != 1 {
			t.Fatalf("short-read slot matched a condition: %+v", out)
		}
	}
}

// A masked bcd digit wraps at its own capacity: with mask 0x0F the
// exact delta forms wrap at 10, so 9 to 0 is an increase of exactly 1
// whatever the flag bits around it do.
func TestBCDMaskedDigitWrap(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 0x79 // flag bits 0x70, digit 9
	en := buildEngine(t, Ruleset{
		Watches: []Watch{{Name: "d", Width: 8, Address: 0, HasMask: true, Mask: 0x0F, BCD: true}},
		Rules: []Rule{pulseRule(
			&PrevCond{Watch: "d", Op: OpIncreased, Qual: QualBy, Operand: 1},
			0.5, 0.5, 20)},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 0xA0 // flag bits change, digit wraps 9 -> 0
	if out := en.Evaluate(mem, tt); len(out) == 0 {
		t.Fatal("masked digit wrap did not fire")
	}
}

// A bcd array slot with invalid digits matches no condition; the
// other slots read normally.
func TestBCDArrayInvalidSlotMatchesNothing(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0x10] = 0x0F // invalid digits
	mem.data[0x11] = 0x40
	rs := Ruleset{
		Watches: []Watch{{Name: "pool", Width: 8, Address: 0x10, Stride: 1, Count: 4, BCD: true}},
		Rules: []Rule{
			{On: &CompareCond{Watch: "pool", Op: OpEq, Operand: 40},
				Effect: Effect{Kind: EffectHold, Strong: 0.3, Weak: 0.3}, Player: 1},
			{On: &CompareCond{Watch: "pool", Op: OpGt, Operand: 90},
				Effect: Effect{Kind: EffectHold, Strong: 0.7, Weak: 0.7}, Player: 2},
		},
	}
	en := buildEngine(t, rs, 2)
	tt := warmup(t, en, mem, t0)

	out := en.Evaluate(mem, tt)
	wantMotors(t, out, 1, 0.3, 0.3) // the 0x40 slot decodes to 40
	for _, ms := range out {
		if ms.Player == 2 {
			t.Fatalf("invalid slot matched a comparison: %+v", out)
		}
	}
}

func TestPatternEnvelope(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10
	pattern := Rule{
		On: decreased("x"),
		Effect: Effect{Kind: EffectPattern, Steps: []Step{
			{Strong: 1, Weak: 1, DurationMs: 100},
			{Strong: 0, Weak: 0, DurationMs: 50},
			{Strong: 0.5, Weak: 0.5, DurationMs: 100},
		}},
		Player: 1,
	}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{pattern},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9
	wantMotors(t, en.Evaluate(mem, tt), 1, 1, 1)
	mem.data[0] = 8                                                        // re-trigger attempt mid-pattern: dropped
	wantMotors(t, en.Evaluate(mem, tt.Add(120*time.Millisecond)), 1, 0, 0) // the gap step still outputs
	wantMotors(t, en.Evaluate(mem, tt.Add(180*time.Millisecond)), 1, 0.5, 0.5)
	wantSilent(t, en.Evaluate(mem, tt.Add(260*time.Millisecond)))
}

// An array watch's condition is true when any slot matches, and the
// rule-level edge makes an appearance fire once: the value staying
// present, or moving between slots, cannot re-fire until it is gone
// from every slot.
func TestArrayWatchAnyMatch(t *testing.T) {
	mem := &fakeMem{}
	rule := pulseRule(eq("pool", 40), 1, 1, 100)
	en := buildEngine(t, Ruleset{
		Watches: []Watch{{Name: "pool", Width: 8, Address: 0, Stride: 4, Count: 8}},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	wantSilent(t, en.Evaluate(mem, tt))
	tt = tt.Add(frame)
	mem.data[12] = 40 // slot 3
	wantMotors(t, en.Evaluate(mem, tt), 1, 1, 1)
	tt = tt.Add(150 * time.Millisecond)
	wantSilent(t, en.Evaluate(mem, tt)) // still present: no re-fire
	tt = tt.Add(frame)
	mem.data[12] = 0
	mem.data[28] = 40 // moved to slot 7 the same frame: still present
	wantSilent(t, en.Evaluate(mem, tt))
	tt = tt.Add(frame)
	mem.data[28] = 0
	wantSilent(t, en.Evaluate(mem, tt)) // gone everywhere
	tt = tt.Add(frame)
	mem.data[4] = 40 // fresh appearance
	wantMotors(t, en.Evaluate(mem, tt), 1, 1, 1)
}

// not in over an array asks whether any slot holds a value outside
// the set.
func TestArrayWatchNotIn(t *testing.T) {
	mem := &fakeMem{}
	rule := pulseRule(&SetCond{Watch: "pool", Negate: true, Set: []uint32{0}}, 1, 1, 100)
	en := buildEngine(t, Ruleset{
		Watches: []Watch{{Name: "pool", Width: 8, Address: 0, Stride: 2, Count: 4}},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	wantSilent(t, en.Evaluate(mem, tt)) // every slot is 0
	tt = tt.Add(frame)
	mem.data[2] = 5 // slot 1 leaves the set
	wantMotors(t, en.Evaluate(mem, tt), 1, 1, 1)
}

func TestScaleMultiplier(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 100
	rule := pulseRule(decreased("x"), 1, 1, 100)
	rule.Scale = &Scale{MagMin: 1, MagMax: 10, MulMin: 0.5, MulMax: 1.0}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 99 // drop of 1: low end of the scale
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5)
	tt = tt.Add(150 * time.Millisecond)
	en.Evaluate(mem, tt) // stable frame so the next drop is a fresh edge
	tt = tt.Add(frame)
	mem.data[0] = 89 // drop of 10: full scale
	wantMotors(t, en.Evaluate(mem, tt), 1, 1, 1)
	tt = tt.Add(150 * time.Millisecond)
	en.Evaluate(mem, tt)
	tt = tt.Add(frame)
	mem.data[0] = 69 // drop of 20: clamped to the top
	wantMotors(t, en.Evaluate(mem, tt), 1, 1, 1)
}

// A descending multiplier range maps the low end of the magnitude to
// the strong end of the output, so a larger change produces a weaker
// effect.
func TestScaleMultiplierDescending(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 100
	rule := pulseRule(decreased("x"), 1, 1, 100)
	rule.Scale = &Scale{MagMin: 1, MagMax: 10, MulMin: 1.0, MulMax: 0.25}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 99 // drop of 1: low end of the magnitude, full multiplier
	wantMotors(t, en.Evaluate(mem, tt), 1, 1, 1)
	tt = tt.Add(150 * time.Millisecond)
	en.Evaluate(mem, tt) // stable frame so the next drop is a fresh edge
	tt = tt.Add(frame)
	mem.data[0] = 89 // drop of 10: high end, weakest multiplier
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.25, 0.25)
	tt = tt.Add(150 * time.Millisecond)
	en.Evaluate(mem, tt)
	tt = tt.Add(frame)
	mem.data[0] = 69 // drop of 20: clamped to the high end
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.25, 0.25)
}

// A change-form scale on an exact by condition takes the stated delta
// as its magnitude: the condition matched exactly that change, so a
// wrap fire scales the same as a plain one instead of reading the
// plain difference.
func TestScaleExactByUsesStatedDelta(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 0xFF
	rule := pulseRule(&PrevCond{Watch: "x", Op: OpIncreased, Qual: QualBy, Operand: 1}, 1, 1, 100)
	rule.Scale = &Scale{MagMin: 1, MagMax: 32, MulMin: 0.5, MulMax: 1.0}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 0x00 // 0xFF -> 0x00: increased by exactly 1, wrapped
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5)
	tt = tt.Add(150 * time.Millisecond)
	en.Evaluate(mem, tt) // stable frame so the next rise is a fresh edge
	tt = tt.Add(frame)
	mem.data[0] = 0x01 // a plain rise of 1 scales identically
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5)
}

// The same holds through a bcd watch's decimal wrap.
func TestScaleExactByBCDWrap(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 0x99
	rule := pulseRule(&PrevCond{Watch: "c", Op: OpIncreased, Qual: QualBy, Operand: 1}, 1, 1, 100)
	rule.Scale = &Scale{MagMin: 1, MagMax: 10, MulMin: 0.5, MulMax: 1.0}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{{Name: "c", Width: 8, Address: 0, BCD: true}},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 0x00 // 99 -> 00: increased by exactly 1, wrapped decimally
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5)
}

// A scale naming a watch reads that watch's current value rather than
// the frame's change, so the trigger and the magnitude can be separate
// values.
func TestScaleOnNamedWatchLevel(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 1
	mem.data[1] = 0
	rule := pulseRule(eq("x", 1), 1, 1, 100)
	rule.Level = true
	rule.Scale = &Scale{Watch: "speed", MagMin: 0, MagMax: 100, MulMin: 0, MulMax: 1.0}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0), byteVal("speed", 1)},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[1] = 50 // half the magnitude range
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5)
	tt = tt.Add(150 * time.Millisecond)
	mem.data[1] = 100 // full, and the trigger has not changed
	wantMotors(t, en.Evaluate(mem, tt), 1, 1, 1)
	tt = tt.Add(150 * time.Millisecond)
	mem.data[1] = 200 // clamped to the top
	wantMotors(t, en.Evaluate(mem, tt), 1, 1, 1)
}

// A hold accepts the named form, and re-reads the magnitude every
// frame the way it re-reads its condition.
func TestScaleOnHoldTracksLevel(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 1
	mem.data[1] = 0
	rule := Rule{
		On:     eq("x", 1),
		Effect: Effect{Kind: EffectHold, Strong: 1, Weak: 1},
		Player: 1,
		Scale:  &Scale{Watch: "speed", MagMin: 0, MagMax: 100, MulMin: 0, MulMax: 1.0},
	}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0), byteVal("speed", 1)},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[1] = 25
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.25, 0.25)
	tt = tt.Add(frame)
	mem.data[1] = 75
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.75, 0.75)
	tt = tt.Add(frame)
	mem.data[0] = 0 // condition false: the hold releases
	wantSilent(t, en.Evaluate(mem, tt))
}

func TestResetReArmsSettle(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{pulseRule(decreased("x"), 0.8, 0.8, 500)},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9
	if out := en.Evaluate(mem, tt); len(out) == 0 {
		t.Fatal("did not fire")
	}
	en.Reset()
	// The playing effect is gone and the settle window is back.
	mem.data[0] = 8
	tt = warmup(t, en, mem, tt.Add(frame))
	mem.data[0] = 7
	if out := en.Evaluate(mem, tt); len(out) == 0 {
		t.Fatal("post-reset transition did not fire")
	}
}

func TestPlayerAllArbitratedPerPlayer(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10
	single := pulseRule(decreased("x"), 0.8, 0.8, 100)
	all := pulseRule(decreased("x"), 0.3, 0.3, 100)
	all.Player = 0
	all.PlayerAll = true
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{single, all},
	}, 2)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9
	out := en.Evaluate(mem, tt)
	wantMotors(t, out, 1, 0.8, 0.8) // player 1 taken by the earlier rule
	wantMotors(t, out, 2, 0.3, 0.3) // player 2 free for the all rule
}

func TestIncreasedByWraps(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 0xFF
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{pulseRule(&PrevCond{Watch: "x", Op: OpIncreased, Qual: QualBy, Operand: 1}, 0.5, 0.5, 100)},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 0x00
	if out := en.Evaluate(mem, tt); len(out) == 0 {
		t.Fatal("wrapped exact delta did not fire")
	}
}

func TestDeltaBoundConds(t *testing.T) {
	cases := []struct {
		name    string
		op      PrevOp
		qual    PrevQual
		operand uint32
		prev    byte
		cur     byte
		fire    bool
	}{
		{"increased at least hit", OpIncreased, QualAtLeast, 10, 10, 25, true},
		{"increased at least edge", OpIncreased, QualAtLeast, 10, 10, 20, true},
		{"increased at least below", OpIncreased, QualAtLeast, 10, 10, 19, false},
		{"increased at least on decrease", OpIncreased, QualAtLeast, 10, 20, 5, false},
		{"increased at most hit", OpIncreased, QualAtMost, 5, 10, 13, true},
		{"increased at most edge", OpIncreased, QualAtMost, 5, 10, 15, true},
		{"increased at most over", OpIncreased, QualAtMost, 5, 10, 16, false},
		{"increased at most no change", OpIncreased, QualAtMost, 5, 10, 10, false},
		{"increased at most on decrease", OpIncreased, QualAtMost, 5, 10, 8, false},
		{"decreased at least hit", OpDecreased, QualAtLeast, 10, 30, 20, true},
		{"decreased at least below", OpDecreased, QualAtLeast, 10, 30, 21, false},
		{"decreased at least on increase", OpDecreased, QualAtLeast, 10, 30, 40, false},
		{"decreased at most hit", OpDecreased, QualAtMost, 5, 30, 27, true},
		{"decreased at most over", OpDecreased, QualAtMost, 5, 30, 24, false},
		// The bounds do not wrap: a byte going 5 to 250 is a rise of
		// 245, never a fall, and 250 to 5 is a fall of 245.
		{"wrap is not a decrease", OpDecreased, QualAtLeast, 1, 5, 250, false},
		{"wrap is not an increase", OpIncreased, QualAtLeast, 1, 250, 5, false},
		{"wrap counts as full rise", OpIncreased, QualAtLeast, 245, 5, 250, true},
		{"wrap counts as full fall", OpDecreased, QualAtLeast, 245, 250, 5, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := &fakeMem{}
			mem.data[0] = tc.prev
			en := buildEngine(t, Ruleset{
				Watches: []Watch{byteVal("x", 0)},
				Rules: []Rule{pulseRule(
					&PrevCond{Watch: "x", Op: tc.op, Qual: tc.qual, Operand: tc.operand},
					0.5, 0.5, 20)},
			}, 1)
			tt := warmup(t, en, mem, t0)

			mem.data[0] = tc.cur
			out := en.Evaluate(mem, tt)
			if tc.fire && len(out) == 0 {
				t.Fatal("did not fire")
			}
			if !tc.fire && len(out) != 0 {
				t.Fatalf("unexpected fire: %+v", out)
			}
		})
	}
}

func TestEventOnDeltaBound(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Events: []Event{{
			Name:       "surge",
			Trigger:    PrevCond{Watch: "x", Op: OpIncreased, Qual: QualAtLeast, Operand: 5},
			DurationMs: 100,
		}},
		Rules: []Rule{{
			On:     &DefRef{Name: "surge"},
			Effect: Effect{Kind: EffectHold, Strong: 0.25, Weak: 0.5},
			Player: 1,
		}},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 13 // +3: below the bound, no window
	wantSilent(t, en.Evaluate(mem, tt))
	tt = tt.Add(frame)
	mem.data[0] = 20 // +7: opens the window
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.25, 0.5)
	wantMotors(t, en.Evaluate(mem, tt.Add(90*time.Millisecond)), 1, 0.25, 0.5)
	wantSilent(t, en.Evaluate(mem, tt.Add(110*time.Millisecond)))
}

func TestScaleOnDeltaBound(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 40
	rule := pulseRule(
		&PrevCond{Watch: "x", Op: OpDecreased, Qual: QualAtLeast, Operand: 4},
		1.0, 1.0, 100)
	rule.Scale = &Scale{MagMin: 4, MagMax: 20, MulMin: 0.5, MulMax: 1.0}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	// A drop of 12 sits halfway through 4..20: multiplier 0.75.
	mem.data[0] = 28
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.75, 0.75)
}

func signedByte(name string, addr uint32) Watch {
	return Watch{Name: name, Width: 8, Address: addr, Signed: true}
}

// holdActive asserts whether a single-hold engine outputs after warmup
// with the watch already at the given value.
func holdActive(t *testing.T, rs Ruleset, setup func(*fakeMem), want bool) {
	t.Helper()
	mem := &fakeMem{}
	setup(mem)
	en := buildEngine(t, rs, 1)
	tt := warmup(t, en, mem, t0)
	out := en.Evaluate(mem, tt)
	if want && len(out) == 0 {
		t.Fatal("hold not active")
	}
	if !want && len(out) != 0 {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestSignedCompareOrdering(t *testing.T) {
	cases := []struct {
		name    string
		op      CompareOp
		operand uint32
		value   byte
		want    bool
	}{
		{"neg below neg", OpLt, 0xFFFFFFFF, 0xFE, true},       // -2 < -1
		{"pos not below neg", OpLt, 0xFFFFFFFF, 0x7F, false},  // 127 < -1
		{"neg above lower neg", OpGt, 0xFFFFFFFB, 0xFE, true}, // -2 > -5
		{"zero above neg", OpGt, 0xFFFFFFFF, 0x00, true},      // 0 > -1
		{"min at most -100", OpLe, 0xFFFFFF9C, 0x80, true},    // -128 <= -100
		{"neg not equal pos", OpEq, 0x7E, 0xFE, false},        // -2 != 126
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			holdActive(t, Ruleset{
				Watches: []Watch{signedByte("x", 0)},
				Rules: []Rule{{
					On:     &CompareCond{Watch: "x", Op: tc.op, Operand: tc.operand},
					Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5},
					Player: 1,
				}},
			}, func(mem *fakeMem) { mem.data[0] = tc.value }, tc.want)
		})
	}
}

func TestSignedDeltasAcrossZero(t *testing.T) {
	cases := []struct {
		name    string
		op      PrevOp
		qual    PrevQual
		operand uint32
		prev    byte
		cur     byte
		fire    bool
	}{
		{"decrease across zero", OpDecreased, QualNone, 0, 2, 0xFE, true},
		{"not an increase", OpIncreased, QualNone, 0, 2, 0xFE, false},
		{"increase across zero", OpIncreased, QualNone, 0, 0xFE, 2, true},
		{"at least across zero", OpDecreased, QualAtLeast, 4, 2, 0xFE, true},
		{"at least too small", OpDecreased, QualAtLeast, 5, 2, 0xFE, false},
		{"at most across zero", OpDecreased, QualAtMost, 4, 2, 0xFE, true},
		{"at most exceeded", OpDecreased, QualAtMost, 3, 2, 0xFE, false},
		{"exact by across zero", OpDecreased, QualBy, 4, 2, 0xFE, true},
		// Signed watches do not wrap the exact form at the width: a
		// signed byte going 127 to -128 is a fall of 255, not a rise
		// of 1.
		{"exact by no width wrap", OpIncreased, QualBy, 1, 0x7F, 0x80, false},
		{"exact by full fall", OpDecreased, QualBy, 255, 0x7F, 0x80, true},
		{"anchor to negative", OpChanged, QualTo, 0xFFFFFFFF, 0, 0xFF, true},
		{"anchor from negative", OpIncreased, QualFrom, 0xFFFFFFFF, 0xFF, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := &fakeMem{}
			mem.data[0] = tc.prev
			en := buildEngine(t, Ruleset{
				Watches: []Watch{signedByte("x", 0)},
				Rules: []Rule{pulseRule(
					&PrevCond{Watch: "x", Op: tc.op, Qual: tc.qual, Operand: tc.operand},
					0.5, 0.5, 20)},
			}, 1)
			tt := warmup(t, en, mem, t0)

			mem.data[0] = tc.cur
			out := en.Evaluate(mem, tt)
			if tc.fire && len(out) == 0 {
				t.Fatal("did not fire")
			}
			if !tc.fire && len(out) != 0 {
				t.Fatalf("unexpected fire: %+v", out)
			}
		})
	}
}

func TestSignedSetMembership(t *testing.T) {
	rs := func() Ruleset {
		return Ruleset{
			Watches: []Watch{signedByte("x", 0)},
			Rules: []Rule{{
				On:     &SetCond{Watch: "x", Set: []uint32{0xFFFFFFFF, 3}}, // (-1, 3)
				Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5},
				Player: 1,
			}},
		}
	}
	holdActive(t, rs(), func(mem *fakeMem) { mem.data[0] = 0xFF }, true)
	holdActive(t, rs(), func(mem *fakeMem) { mem.data[0] = 0xFE }, false)
}

func TestSignedScaleRange(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0], mem.data[1] = 0xFF, 0xFA // -6
	rule := pulseRule(&PrevCond{Watch: "v", Op: OpDecreased}, 1.0, 1.0, 100)
	rule.Scale = &Scale{Watch: "v", MagMin: 0xFFFFFFF4, MagMax: 0xFFFFFFFA, MulMin: 1.0, MulMax: 0.4} // -12..-6
	en := buildEngine(t, Ruleset{
		Watches: []Watch{{Name: "v", Width: 16, Address: 0, Signed: true}},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	// -9 sits halfway through -12..-6: multiplier 0.7.
	mem.data[0], mem.data[1] = 0xFF, 0xF7
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.7, 0.7)
}

func TestSignedChangeScaleAcrossZero(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 2
	rule := pulseRule(&PrevCond{Watch: "x", Op: OpDecreased}, 1.0, 1.0, 100)
	rule.Scale = &Scale{MagMin: 0, MagMax: 8, MulMin: 0, MulMax: 1.0}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{signedByte("x", 0)},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	// 2 to -2 is a fall of 4: halfway through 0..8.
	mem.data[0] = 0xFE
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5)
}

func TestAbsWatch(t *testing.T) {
	absByte := Watch{Name: "x", Width: 8, Address: 0, Abs: true}
	holdActive(t, Ruleset{
		Watches: []Watch{absByte},
		Rules: []Rule{{
			On:     &CompareCond{Watch: "x", Op: OpEq, Operand: 128},
			Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5},
			Player: 1,
		}},
	}, func(mem *fakeMem) { mem.data[0] = 0x80 }, true)

	// Magnitude deltas: -3 to -9 is |3| to |9|, a rise of 6.
	mem := &fakeMem{}
	mem.data[0] = 0xFD
	en := buildEngine(t, Ruleset{
		Watches: []Watch{absByte},
		Rules: []Rule{pulseRule(
			&PrevCond{Watch: "x", Op: OpIncreased, Qual: QualAtLeast, Operand: 6},
			0.5, 0.5, 20)},
	}, 1)
	tt := warmup(t, en, mem, t0)
	mem.data[0] = 0xF7
	if out := en.Evaluate(mem, tt); len(out) == 0 {
		t.Fatal("magnitude rise did not fire")
	}
}

func TestAbsIntMin(t *testing.T) {
	holdActive(t, Ruleset{
		Watches: []Watch{{Name: "x", Width: 32, Address: 0, Abs: true}},
		Rules: []Rule{{
			On:     &CompareCond{Watch: "x", Op: OpGe, Operand: 0x80000000},
			Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5},
			Player: 1,
		}},
	}, func(mem *fakeMem) { mem.data[0] = 0x80 }, true)
}

func TestSignedArrayAnyMatch(t *testing.T) {
	rs := func() Ruleset {
		return Ruleset{
			Watches: []Watch{{Name: "arr", Width: 8, Address: 0, Stride: 1, Count: 2, Signed: true}},
			Rules: []Rule{{
				On:     &CompareCond{Watch: "arr", Op: OpLt, Operand: 0xFFFFFFFF}, // < -1
				Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5},
				Player: 1,
			}},
		}
	}
	holdActive(t, rs(), func(mem *fakeMem) { mem.data[0], mem.data[1] = 5, 0xFE }, true)
	holdActive(t, rs(), func(mem *fakeMem) { mem.data[0], mem.data[1] = 5, 3 }, false)
}

func TestEventOnSignedDelta(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 2
	en := buildEngine(t, Ruleset{
		Watches: []Watch{signedByte("x", 0)},
		Events: []Event{{
			Name:       "drop",
			Trigger:    PrevCond{Watch: "x", Op: OpDecreased},
			DurationMs: 100,
		}},
		Rules: []Rule{{
			On:     &DefRef{Name: "drop"},
			Effect: Effect{Kind: EffectHold, Strong: 0.25, Weak: 0.5},
			Player: 1,
		}},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 0xFE // 2 to -2: a signed fall opens the window
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.25, 0.5)
	wantSilent(t, en.Evaluate(mem, tt.Add(110*time.Millisecond)))
}

func TestAnchoredPrevConds(t *testing.T) {
	cases := []struct {
		name    string
		op      PrevOp
		qual    PrevQual
		operand uint32
		prev    byte
		cur     byte
		fire    bool
	}{
		{"changed from hit", OpChanged, QualFrom, 5, 5, 9, true},
		{"changed from wrong prev", OpChanged, QualFrom, 5, 4, 9, false},
		{"changed from no change", OpChanged, QualFrom, 5, 5, 5, false},
		{"changed to hit", OpChanged, QualTo, 5, 9, 5, true},
		{"changed to wrong cur", OpChanged, QualTo, 5, 9, 4, false},
		{"changed to no change", OpChanged, QualTo, 5, 5, 5, false},
		{"increased from hit", OpIncreased, QualFrom, 5, 5, 6, true},
		{"increased from decrease", OpIncreased, QualFrom, 5, 5, 4, false},
		{"increased from wrong prev", OpIncreased, QualFrom, 5, 4, 6, false},
		{"increased to hit", OpIncreased, QualTo, 5, 4, 5, true},
		{"increased to from above", OpIncreased, QualTo, 5, 6, 5, false},
		{"decreased from hit", OpDecreased, QualFrom, 5, 5, 4, true},
		{"decreased from increase", OpDecreased, QualFrom, 5, 5, 6, false},
		{"decreased from wrong prev", OpDecreased, QualFrom, 5, 6, 4, false},
		{"decreased to hit", OpDecreased, QualTo, 5, 6, 5, true},
		{"decreased to from below", OpDecreased, QualTo, 5, 4, 5, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := &fakeMem{}
			mem.data[0] = tc.prev
			en := buildEngine(t, Ruleset{
				Watches: []Watch{byteVal("x", 0)},
				Rules: []Rule{pulseRule(
					&PrevCond{Watch: "x", Op: tc.op, Qual: tc.qual, Operand: tc.operand},
					0.5, 0.5, 20)},
			}, 1)
			tt := warmup(t, en, mem, t0)

			mem.data[0] = tc.cur
			out := en.Evaluate(mem, tt)
			if tc.fire && len(out) == 0 {
				t.Fatal("did not fire")
			}
			if !tc.fire && len(out) != 0 {
				t.Fatalf("unexpected fire: %+v", out)
			}
		})
	}
}

func TestEndianDecode(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 0x12
	mem.data[1] = 0x34
	mem.data[2] = 0x12
	mem.data[3] = 0x34
	big := pulseRule(eq("w", 0x1234), 0.5, 0.5, 20)
	big.Level = true
	big.CooldownMs = 1000
	little := pulseRule(eq("l", 0x3412), 0.5, 0.5, 20)
	little.Level = true
	little.CooldownMs = 1000
	little.Player = 2
	en := buildEngine(t, Ruleset{
		Watches: []Watch{
			{Name: "w", Width: 16, Address: 0},
			{Name: "l", Width: 16, Address: 2, Endian: EndianLittle},
		},
		Rules: []Rule{big, little},
	}, 2)
	tt := warmup(t, en, mem, t0)

	out := en.Evaluate(mem, tt)
	wantMotors(t, out, 1, 0.5, 0.5)
	wantMotors(t, out, 2, 0.5, 0.5)
}

// prioPulseRule is a pulse carrying an explicit priority.
func prioPulseRule(on Expr, strong, weak float64, durMs, prio int) Rule {
	r := pulseRule(on, strong, weak, durMs)
	r.Priority = prio
	return r
}

func TestHigherPriorityBeatsEarlierSameFrameFire(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules: []Rule{
			pulseRule(decreased("x"), 0.75, 0.75, 200),
			prioPulseRule(decreased("x"), 1.0, 1.0, 600, 1),
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9 // both fire: the later, higher rule replaces the pulse
	wantMotors(t, en.Evaluate(mem, tt), 1, 1.0, 1.0)
}

func TestHigherPriorityCancelsPlayingEffect(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10 // x
	mem.data[1] = 0  // d
	death := prioPulseRule(&PrevCond{Watch: "d", Op: OpIncreased}, 1.0, 1.0, 600, 1)
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0), byteVal("d", 1)},
		Rules: []Rule{
			pulseRule(decreased("x"), 0.75, 0.75, 200),
			death,
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9 // damage pulse starts
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.75, 0.75)
	mem.data[1] = 1 // death lands mid-pulse and outranks it
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 1.0, 1.0)
}

// A playing effect holds the slot against anything that does not
// outrank it, however far below the incumbent the fire sits.
func TestPlayingEffectBlocksLowerPriority(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10 // x
	mem.data[1] = 5  // y
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0), byteVal("y", 1)},
		Rules: []Rule{
			prioPulseRule(decreased("x"), 1.0, 1.0, 400, 2),
			prioPulseRule(decreased("y"), 0.2, 0.2, 100, -1),
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9 // the death pattern takes the slot
	wantMotors(t, en.Evaluate(mem, tt), 1, 1.0, 1.0)
	mem.data[1] = 4 // shake fires under it: dropped
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 1.0, 1.0)
}

// The middle tier of a three-tier file outranks what is below it and
// is blocked by what is above it.
func TestMidPriorityBeatsLowAndLosesToHigh(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10 // shake
	mem.data[1] = 10 // damage
	mem.data[2] = 10 // death
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("shake", 0), byteVal("damage", 1), byteVal("death", 2)},
		Rules: []Rule{
			prioPulseRule(decreased("shake"), 0.3, 0.3, 400, -1),
			prioPulseRule(decreased("damage"), 0.7, 0.7, 400, 0),
			prioPulseRule(decreased("death"), 1.0, 1.0, 400, 1),
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9 // shake takes the free slot
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.3, 0.3)
	mem.data[1] = 9 // damage outranks the playing shake
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 0.7, 0.7)
	mem.data[2] = 9 // death outranks the playing damage
	wantMotors(t, en.Evaluate(mem, tt.Add(2*frame)), 1, 1.0, 1.0)
}

// A negative priority is only a loss of ties: with the slot free the
// rule fires like any other.
func TestNegativePriorityFiresIntoFreeSlot(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{prioPulseRule(decreased("x"), 0.3, 0.3, 100, MinPriority)},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.3, 0.3)
}

// File order does not save a negative priority: the default-priority
// rule below it still takes the slot on the same frame.
func TestNegativePriorityLosesToDefault(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules: []Rule{
			prioPulseRule(decreased("x"), 0.3, 0.3, 100, -1),
			pulseRule(decreased("x"), 0.7, 0.7, 100),
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.7, 0.7)
}

// Priority ranks fires against a playing effect, not against a spent
// one: once the high effect has run out, the low fire takes the slot.
func TestFinishedEffectBlocksNothing(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10 // x
	mem.data[1] = 5  // y
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0), byteVal("y", 1)},
		Rules: []Rule{
			prioPulseRule(decreased("x"), 1.0, 1.0, 100, MaxPriority),
			prioPulseRule(decreased("y"), 0.2, 0.2, 100, MinPriority),
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9
	wantMotors(t, en.Evaluate(mem, tt), 1, 1.0, 1.0)
	mem.data[1] = 4 // fires after the high effect has run out
	wantMotors(t, en.Evaluate(mem, tt.Add(150*time.Millisecond)), 1, 0.2, 0.2)
}

// A read the host cannot serve whole has no value: decoding what
// arrived would invent one. The second hold shows the frame still
// evaluates normally around it.
func TestShortReadLeavesTheWatchUnresolved(t *testing.T) {
	mem := &shortMem{from: 0x10, max: 1}
	mem.data[0] = 3
	mem.data[0x10] = 0x12
	mem.data[0x11] = 0x34
	en := buildEngine(t, Ruleset{
		Watches: []Watch{{Name: "w", Width: 16, Address: 0x10}, byteVal("s", 0)},
		Rules: []Rule{
			{On: eq("w", 0x1200), Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5}, Player: 1},
			{On: eq("s", 3), Effect: Effect{Kind: EffectHold, Strong: 0.2, Weak: 0.2}, Player: 1},
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	// One byte of the word arrives, which would decode as 0x1200.
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.2, 0.2)
}

// A slot key read short must not match: the bytes that arrived can
// equal the key value with the rest missing.
func TestShortKeyReadDoesNotMatch(t *testing.T) {
	mem := &shortMem{from: 0x10, max: 2}
	mem.data[0] = 3
	mem.data[0x10], mem.data[0x11] = 0x11, 0x22 // key long, high half
	mem.data[0x12], mem.data[0x13] = 0x33, 0x44
	mem.data[0x14] = 5 // the field the watch would read
	keyed := Watch{
		Name: "k", Width: 8, Address: 0x10,
		Stride: 8, Count: 2,
		HasKey: true, KeyOffset: 0, KeyValue: 0x11220000, FieldOffset: 4,
	}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{keyed, byteVal("s", 0)},
		Rules: []Rule{
			{On: eq("k", 5), Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5}, Player: 1},
			{On: eq("s", 3), Effect: Effect{Kind: EffectHold, Strong: 0.2, Weak: 0.2}, Player: 1},
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	// Two bytes arrive and read as the key value; no slot may match.
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.2, 0.2)
}

// A silent effect still occupies the slot: the player stays in the
// output at 0/0, lower fires are blocked for its duration, and a hold
// keeps layering through it.
func TestSilentPulseReservesTheSlot(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10 // x
	mem.data[1] = 5  // y
	mem.data[2] = 0  // s
	hold := Rule{On: eq("s", 3), Effect: Effect{Kind: EffectHold, Strong: 0.1, Weak: 0.1}, Player: 1}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0), byteVal("y", 1), byteVal("s", 2)},
		Rules: []Rule{
			prioPulseRule(decreased("x"), 0, 0, 200, 1),
			prioPulseRule(decreased("y"), 0.8, 0.8, 100, 0),
			hold,
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9 // the silent pulse takes the slot: present, at zero
	wantMotors(t, en.Evaluate(mem, tt), 1, 0, 0)
	mem.data[1] = 4 // a lower fire inside the window is blocked
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 0, 0)
	mem.data[2] = 3 // a hold layers through the silence
	wantMotors(t, en.Evaluate(mem, tt.Add(2*frame)), 1, 0.1, 0.1)
}

// A cooldown belongs to the rule, not to each player it targets: a
// "player all" fire that only some players took still starts it, and
// the players whose slots were busy get no retry inside the window.
func TestPlayerAllCooldownIsPerRule(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10 // x
	mem.data[1] = 10 // y
	all := pulseRule(decreased("x"), 0.5, 0.5, 400)
	all.PlayerAll = true
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0), byteVal("y", 1)},
		Rules: []Rule{
			prioPulseRule(decreased("y"), 1.0, 1.0, 50, 1),
			all,
		},
	}, 2)
	tt := warmup(t, en, mem, t0)

	mem.data[1] = 9 // a higher rule takes player 1's slot
	wantMotors(t, en.Evaluate(mem, tt), 1, 1.0, 1.0)
	mem.data[0] = 9 // the all rule wins player 2 only, and starts its cooldown
	out := en.Evaluate(mem, tt.Add(frame))
	wantMotors(t, out, 1, 1.0, 1.0)
	wantMotors(t, out, 2, 0.5, 0.5)
	wantMotors(t, en.Evaluate(mem, tt.Add(2*frame)), 2, 0.5, 0.5) // clears the edge

	// Player 1's slot is free again, but the rule is still cooling
	// down, so the fire it missed is gone rather than deferred.
	mem.data[0] = 8
	out = en.Evaluate(mem, tt.Add(100*time.Millisecond))
	wantMotors(t, out, 2, 0.5, 0.5)
	if len(out) != 1 {
		t.Fatalf("player 1 fired inside the rule's cooldown: %+v", out)
	}
}

func TestFirstFireWinsAtEqualPriority(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 10
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules: []Rule{
			prioPulseRule(decreased("x"), 0.4, 0.4, 200, 3),
			prioPulseRule(decreased("x"), 0.9, 0.9, 200, 3),
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 9
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.4, 0.4)
}

// The hold pool ranks the same way: highest wins, and file order only
// breaks a tie.
func TestHighestPriorityHoldWins(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 3
	shake := Rule{On: eq("s", 3), Effect: Effect{Kind: EffectHold, Strong: 0.2, Weak: 0.2}, Player: 1, Priority: -1}
	damage := Rule{On: eq("s", 3), Effect: Effect{Kind: EffectHold, Strong: 0.8, Weak: 0.8}, Player: 1}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("s", 0)},
		Rules:   []Rule{shake, damage},
	}, 1)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 1, 0.8, 0.8)
}

// A hold and a fire are separate pools: the hold's priority does not
// rank it against a playing effect, both still mix.
func TestHoldPriorityDoesNotRankAgainstFires(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 3  // s
	mem.data[1] = 10 // x
	hold := Rule{On: eq("s", 3), Effect: Effect{Kind: EffectHold, Strong: 0.6, Weak: 0.1}, Player: 1, Priority: MaxPriority}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("s", 0), byteVal("x", 1)},
		Rules: []Rule{
			hold,
			prioPulseRule(decreased("x"), 0.2, 0.9, 100, MinPriority),
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[1] = 9 // the lower pulse still fires and mixes with the hold
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.6, 0.9)
}

// clipPulseRule is a 300ms pulse bounded by its own condition, long
// enough that every clip below lands mid-effect.
func clipPulseRule(on Expr, clipMs int) Rule {
	r := pulseRule(on, 0.8, 0.8, 300)
	r.Clip = true
	r.ClipMs = clipMs
	return r
}

func TestClipCutsPatternMidPlay(t *testing.T) {
	mem := &fakeMem{}
	dying := Rule{
		On: eq("x", 1),
		Effect: Effect{Kind: EffectPattern, Steps: []Step{
			{Strong: 1, Weak: 1, DurationMs: 100},
			{Strong: 0, Weak: 0, DurationMs: 50},
			{Strong: 0.5, Weak: 0.5, DurationMs: 200},
		}},
		Player: 1,
		Clip:   true,
	}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{dying},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 1
	wantMotors(t, en.Evaluate(mem, tt), 1, 1, 1)
	wantMotors(t, en.Evaluate(mem, tt.Add(180*time.Millisecond)), 1, 0.5, 0.5)
	mem.data[0] = 0 // the state it expresses ends: cut, not played out
	wantSilent(t, en.Evaluate(mem, tt.Add(200*time.Millisecond)))
}

func TestClipDelayKeepsEffectBriefly(t *testing.T) {
	mem := &fakeMem{}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{clipPulseRule(eq("x", 1), 100)},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 1
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.8, 0.8)
	// Last true frame is tt+frame, so the cut lands at tt+frame+100ms.
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 0.8, 0.8)
	mem.data[0] = 0
	wantMotors(t, en.Evaluate(mem, tt.Add(100*time.Millisecond)), 1, 0.8, 0.8)
	wantSilent(t, en.Evaluate(mem, tt.Add(frame+100*time.Millisecond)))
}

func TestClipDelayRestartsOnRetruth(t *testing.T) {
	mem := &fakeMem{}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{clipPulseRule(eq("x", 1), 100)},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 1
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.8, 0.8)
	mem.data[0] = 0 // one frame's dropout
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 0.8, 0.8)
	mem.data[0] = 1 // back before the delay runs out: window restarts
	wantMotors(t, en.Evaluate(mem, tt.Add(2*frame)), 1, 0.8, 0.8)
	mem.data[0] = 0
	// The original window would have closed here, the restarted one has not.
	wantMotors(t, en.Evaluate(mem, tt.Add(110*time.Millisecond)), 1, 0.8, 0.8)
	wantSilent(t, en.Evaluate(mem, tt.Add(2*frame+100*time.Millisecond)))
}

func TestClipIgnoresTheGate(t *testing.T) {
	mem := &fakeMem{}
	mem.data[1] = 1 // g
	rule := clipPulseRule(eq("x", 0), 0)
	rule.While = eq("g", 1)
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0), byteVal("g", 1)},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 1 // arm the edge
	wantSilent(t, en.Evaluate(mem, tt))
	mem.data[0] = 0
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 0.8, 0.8)
	mem.data[1] = 0 // gate closes: a playing effect is not cut short
	wantMotors(t, en.Evaluate(mem, tt.Add(2*frame)), 1, 0.8, 0.8)
	mem.data[0] = 1 // the condition ends: clipped even with the gate closed
	wantSilent(t, en.Evaluate(mem, tt.Add(3*frame)))
}

func TestClipLeavesAnotherRulesEffect(t *testing.T) {
	mem := &fakeMem{}
	mem.data[1] = 10 // y
	death := prioPulseRule(decreased("y"), 1, 1, 300, 1)
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0), byteVal("y", 1)},
		Rules: []Rule{
			clipPulseRule(eq("x", 1), 0),
			death,
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 1
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.8, 0.8)
	mem.data[1] = 9 // the higher rule takes the slot
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 1, 1)
	mem.data[0] = 0 // the clip owns nothing now: the death effect survives
	wantMotors(t, en.Evaluate(mem, tt.Add(2*frame)), 1, 1, 1)
}

func TestClipFreesSlotForAnEarlierRule(t *testing.T) {
	mem := &fakeMem{}
	mem.data[1] = 10 // y
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0), byteVal("y", 1)},
		Rules: []Rule{
			pulseRule(decreased("y"), 0.2, 0.2, 100),
			clipPulseRule(eq("x", 1), 0),
		},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 1
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.8, 0.8)
	// The clip is applied before the frame's fires, so the rule above it
	// lands rather than being dropped against a slot about to be freed.
	mem.data[0] = 0
	mem.data[1] = 9
	wantMotors(t, en.Evaluate(mem, tt.Add(frame)), 1, 0.2, 0.2)
}

func TestClipDoesNotShortenCooldown(t *testing.T) {
	mem := &fakeMem{}
	rule := clipPulseRule(eq("x", 1), 0)
	rule.Effect.DurationMs = 50
	rule.Level = true
	rule.CooldownMs = 200
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0] = 1
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.8, 0.8)
	mem.data[0] = 0 // clipped well inside the cooldown
	wantSilent(t, en.Evaluate(mem, tt.Add(frame)))
	mem.data[0] = 1 // the cooldown still runs from the fire
	wantSilent(t, en.Evaluate(mem, tt.Add(2*frame)))
	wantMotors(t, en.Evaluate(mem, tt.Add(200*time.Millisecond)), 1, 0.8, 0.8)
}

func TestUnchangedEdge(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 7
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0)},
		Rules:   []Rule{pulseRule(&PrevCond{Watch: "x", Op: OpUnchanged}, 0.5, 0.5, 20)},
	}, 1)
	tt := warmup(t, en, mem, t0)

	// Static through settle: tracked, no stale fire.
	wantSilent(t, en.Evaluate(mem, tt))
	mem.data[0] = 5 // change: condition goes false
	wantSilent(t, en.Evaluate(mem, tt.Add(frame)))
	// Static again: fresh false-to-true transition.
	if out := en.Evaluate(mem, tt.Add(2*frame)); len(out) == 0 {
		t.Fatal("unchanged edge did not fire")
	}
}

func TestEventWindowGatesHits(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 3  // meter
	mem.data[1] = 50 // health
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("meter", 0), byteVal("health", 1)},
		Events: []Event{{
			Name:       "spent",
			Trigger:    PrevCond{Watch: "meter", Op: OpDecreased},
			DurationMs: 100,
		}},
		Rules: []Rule{{
			On:     decreased("health"),
			While:  &DefRef{Name: "spent"},
			Effect: Effect{Kind: EffectPulse, Strong: 1, Weak: 1, DurationMs: 32},
			Player: 1,
		}},
	}, 1)
	tt := warmup(t, en, mem, t0)

	// Health drop with no window open: gated off.
	mem.data[1] = 45
	wantSilent(t, en.Evaluate(mem, tt))
	tt = tt.Add(frame)

	// Meter spend opens the window; nothing fires by itself.
	mem.data[0] = 2
	wantSilent(t, en.Evaluate(mem, tt))
	tt = tt.Add(frame)

	// A hit inside the window fires.
	mem.data[1] = 40
	wantMotors(t, en.Evaluate(mem, tt), 1, 1, 1)

	// Past the pulse and the window: closed again.
	tt = tt.Add(200 * time.Millisecond)
	wantSilent(t, en.Evaluate(mem, tt))
	tt = tt.Add(frame)
	mem.data[1] = 35
	wantSilent(t, en.Evaluate(mem, tt))
}

func TestEventSameFrameHitFires(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 3
	mem.data[1] = 50
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("meter", 0), byteVal("health", 1)},
		Events: []Event{{
			Name:       "spent",
			Trigger:    PrevCond{Watch: "meter", Op: OpDecreased},
			DurationMs: 100,
		}},
		Rules: []Rule{{
			On:     decreased("health"),
			While:  &DefRef{Name: "spent"},
			Effect: Effect{Kind: EffectPulse, Strong: 1, Weak: 1, DurationMs: 32},
			Player: 1,
		}},
	}, 1)
	tt := warmup(t, en, mem, t0)

	// Trigger and hit on the same frame: the window opens first.
	mem.data[0] = 2
	mem.data[1] = 45
	wantMotors(t, en.Evaluate(mem, tt), 1, 1, 1)
}

func TestEventRetriggerExtends(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 3
	mem.data[1] = 50
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("meter", 0), byteVal("health", 1)},
		Events: []Event{{
			Name:       "spent",
			Trigger:    PrevCond{Watch: "meter", Op: OpDecreased},
			DurationMs: 100,
		}},
		Rules: []Rule{{
			On:     decreased("health"),
			While:  &DefRef{Name: "spent"},
			Effect: Effect{Kind: EffectPulse, Strong: 1, Weak: 1, DurationMs: 32},
			Player: 1,
		}},
	}, 1)
	tt := warmup(t, en, mem, t0)

	// Open at tt, retrigger at +64ms: the window now runs to +164ms.
	mem.data[0] = 2
	wantSilent(t, en.Evaluate(mem, tt))
	mem.data[0] = 1
	wantSilent(t, en.Evaluate(mem, tt.Add(64*time.Millisecond)))

	// A hit at +120ms is outside the first window but inside the
	// extension.
	mem.data[1] = 45
	wantMotors(t, en.Evaluate(mem, tt.Add(120*time.Millisecond)), 1, 1, 1)
}

func TestEventResetClosesWindow(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 3
	mem.data[1] = 50
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("meter", 0), byteVal("health", 1)},
		Events: []Event{{
			Name:       "spent",
			Trigger:    PrevCond{Watch: "meter", Op: OpDecreased},
			DurationMs: 60000,
		}},
		Rules: []Rule{{
			On:     decreased("health"),
			While:  &DefRef{Name: "spent"},
			Effect: Effect{Kind: EffectPulse, Strong: 1, Weak: 1, DurationMs: 32},
			Player: 1,
		}},
	}, 1)
	tt := warmup(t, en, mem, t0)

	// Open a long window, then reset: the window must not survive.
	mem.data[0] = 2
	wantSilent(t, en.Evaluate(mem, tt))
	en.Reset()
	tt = warmup(t, en, mem, tt.Add(frame))
	mem.data[1] = 45
	wantSilent(t, en.Evaluate(mem, tt))
}

func TestHoldOnEvent(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 3
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("meter", 0)},
		Events: []Event{{
			Name:       "spent",
			Trigger:    PrevCond{Watch: "meter", Op: OpDecreased},
			DurationMs: 100,
		}},
		Rules: []Rule{{
			On:     &DefRef{Name: "spent"},
			Effect: Effect{Kind: EffectHold, Strong: 0.25, Weak: 0.5},
			Player: 1,
		}},
	}, 1)
	tt := warmup(t, en, mem, t0)

	// The hold tracks the window: on while open, released at close.
	mem.data[0] = 2
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.25, 0.5)
	wantMotors(t, en.Evaluate(mem, tt.Add(50*time.Millisecond)), 1, 0.25, 0.5)
	wantSilent(t, en.Evaluate(mem, tt.Add(150*time.Millisecond)))
}

// setLong writes a big-endian long, the shape of a pointer or key in
// the test system.
func (m *fakeMem) setLong(addr uint32, v uint32) {
	m.data[addr] = byte(v >> 24)
	m.data[addr+1] = byte(v >> 16)
	m.data[addr+2] = byte(v >> 8)
	m.data[addr+3] = byte(v)
}

func TestPointerWatchFollowsPointer(t *testing.T) {
	mem := &fakeMem{}
	mem.setLong(0, 0x20) // pointer cell -> 0x20
	mem.data[0x24] = 10  // value at target+offset
	en := buildEngine(t, Ruleset{
		Watches: []Watch{{Name: "x", Width: 8, Address: 0, Pointer: true, Offset: 4}},
		Rules:   []Rule{pulseRule(decreased("x"), 0.8, 0.4, 20)},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0x24] = 9 // drop through the pointer fires
	if out := en.Evaluate(mem, tt); len(out) == 0 {
		t.Fatal("drop through pointer did not fire")
	}
	tt = tt.Add(50 * time.Millisecond)

	// The pointer moves to an object holding a smaller value: the
	// watch re-seeds, so the move is not a drop.
	mem.setLong(0, 0x30)
	mem.data[0x34] = 3
	wantSilent(t, en.Evaluate(mem, tt))
	tt = tt.Add(frame)
	mem.data[0x34] = 2 // a real drop at the new target fires
	if out := en.Evaluate(mem, tt); len(out) == 0 {
		t.Fatal("drop at moved target did not fire")
	}
}

func TestPointerWatchUnresolved(t *testing.T) {
	mem := &fakeMem{}
	mem.setLong(0, 0x20)
	mem.data[0x20] = 3
	hold := Rule{
		On:     eq("x", 3),
		Effect: Effect{Kind: EffectHold, Strong: 0.3, Weak: 0.6},
		Player: 1,
	}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{{Name: "x", Width: 8, Address: 0, Pointer: true}},
		Rules:   []Rule{hold},
	}, 1)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 1, 0.3, 0.6)
	tt = tt.Add(frame)
	mem.setLong(0, 0) // null pointer: unresolved, the hold releases
	wantSilent(t, en.Evaluate(mem, tt))
	tt = tt.Add(frame)
	mem.setLong(0, 0x100) // outside every region: still unresolved
	wantSilent(t, en.Evaluate(mem, tt))
	tt = tt.Add(frame)
	mem.setLong(0, 0x20) // back: resolves again
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.3, 0.6)
}

// Returning from unresolved seeds the previous value: the return
// itself cannot register as a change.
func TestPointerWatchReturnSeeds(t *testing.T) {
	mem := &fakeMem{}
	mem.setLong(0, 0x20)
	mem.data[0x20] = 10
	en := buildEngine(t, Ruleset{
		Watches: []Watch{{Name: "x", Width: 8, Address: 0, Pointer: true}},
		Rules:   []Rule{pulseRule(decreased("x"), 0.8, 0.8, 20)},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.setLong(0, 0) // unresolved
	wantSilent(t, en.Evaluate(mem, tt))
	tt = tt.Add(frame)
	mem.data[0x20] = 2 // smaller value while away
	mem.setLong(0, 0x20)
	wantSilent(t, en.Evaluate(mem, tt)) // return seeds, no drop
	tt = tt.Add(frame)
	mem.data[0x20] = 1 // fresh drop fires
	if out := en.Evaluate(mem, tt); len(out) == 0 {
		t.Fatal("post-return drop did not fire")
	}
}

// keyedWatch is a 4-slot pool at 0x10, stride 8: a long key at slot+0
// and a byte field at slot+4.
func keyedWatch() Watch {
	return Watch{Name: "k", Width: 8, Address: 0x10, Stride: 8, Count: 4,
		HasKey: true, KeyOffset: 0, KeyValue: 0xAA, FieldOffset: 4}
}

func TestKeyedSlotWatch(t *testing.T) {
	mem := &fakeMem{}
	mem.setLong(0x20, 0xAA) // slot 2 holds the key
	mem.data[0x24] = 10
	en := buildEngine(t, Ruleset{
		Watches: []Watch{keyedWatch()},
		Rules:   []Rule{pulseRule(decreased("k"), 0.8, 0.8, 20)},
	}, 1)
	tt := warmup(t, en, mem, t0)

	mem.data[0x24] = 9 // field drop in the selected slot fires
	if out := en.Evaluate(mem, tt); len(out) == 0 {
		t.Fatal("field drop did not fire")
	}
	tt = tt.Add(50 * time.Millisecond)

	// The key moves to slot 0 with a smaller field value: the watch
	// re-seeds on the new slot, so the move is not a drop.
	mem.setLong(0x20, 0)
	mem.setLong(0x10, 0xAA)
	mem.data[0x14] = 2
	wantSilent(t, en.Evaluate(mem, tt))
	tt = tt.Add(frame)
	mem.data[0x14] = 1 // a real drop in the new slot fires
	if out := en.Evaluate(mem, tt); len(out) == 0 {
		t.Fatal("drop in moved slot did not fire")
	}
	tt = tt.Add(50 * time.Millisecond)

	mem.setLong(0x10, 0) // no slot matches: unresolved
	mem.data[0x14] = 0
	wantSilent(t, en.Evaluate(mem, tt))
}

// Selection is sticky: a second matching slot appearing at a lower
// index does not steal the selection while the current key matches.
func TestKeyedSlotSticky(t *testing.T) {
	mem := &fakeMem{}
	mem.setLong(0x28, 0xAA) // slot 3
	mem.data[0x2C] = 7
	hold := Rule{
		On:     eq("k", 7),
		Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: 0.5},
		Player: 1,
	}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{keyedWatch()},
		Rules:   []Rule{hold},
	}, 1)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5)
	tt = tt.Add(frame)
	mem.setLong(0x18, 0xAA) // slot 1 also matches, field differs
	mem.data[0x1C] = 3
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5) // still reading slot 3
	tt = tt.Add(frame)
	mem.setLong(0x28, 0) // slot 3 stops matching: falls to slot 1
	wantSilent(t, en.Evaluate(mem, tt))
	tt = tt.Add(frame)
	mem.data[0x1C] = 7
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.5, 0.5)
}

func TestKeyedFieldPointer(t *testing.T) {
	mem := &fakeMem{}
	mem.setLong(0x10, 0xAA) // slot 0 holds the key
	mem.setLong(0x14, 0x30) // its field is a pointer
	mem.data[0x32] = 5      // value at pointer+offset
	v := keyedWatch()
	v.FieldPtr = true
	v.Offset = 2
	hold := Rule{
		On:     eq("k", 5),
		Effect: Effect{Kind: EffectHold, Strong: 0.4, Weak: 0.4},
		Player: 1,
	}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{v},
		Rules:   []Rule{hold},
	}, 1)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 1, 0.4, 0.4)
	tt = tt.Add(frame)
	mem.setLong(0x14, 0) // null component pointer: unresolved
	wantSilent(t, en.Evaluate(mem, tt))
	tt = tt.Add(frame)
	mem.setLong(0x14, 0x30)
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.4, 0.4)
}

// A scale naming an unresolved watch clamps to the bottom of its
// magnitude range.
func TestScaleOnUnresolvedWatch(t *testing.T) {
	mem := &fakeMem{}
	mem.data[0] = 1
	mem.setLong(4, 0x20)
	mem.data[0x20] = 100
	rule := Rule{
		On:     eq("x", 1),
		Effect: Effect{Kind: EffectHold, Strong: 1, Weak: 1},
		Player: 1,
		Scale:  &Scale{Watch: "mag", MagMin: 0, MagMax: 100, MulMin: 0.25, MulMax: 1.0},
	}
	en := buildEngine(t, Ruleset{
		Watches: []Watch{byteVal("x", 0), {Name: "mag", Width: 8, Address: 4, Pointer: true}},
		Rules:   []Rule{rule},
	}, 1)
	tt := warmup(t, en, mem, t0)

	wantMotors(t, en.Evaluate(mem, tt), 1, 1, 1)
	tt = tt.Add(frame)
	mem.setLong(4, 0) // magnitude watch unresolved: bottom of the range
	wantMotors(t, en.Evaluate(mem, tt), 1, 0.25, 0.25)
}
