package rumble

import (
	"strings"
	"testing"
)

// The two examples from FORMAT.md, verbatim. They must always parse.
const minimalExample = `game: Bulk Slash
gameid: T-14310G
system: saturn
---

# Player health. Rumbles on any loss.
watch health byte 0x0605C973
# Counts up when the player dies.
watch deaths byte 0x0605CAE3

on health decreased: pulse 0.76 200ms
on deaths increased: pulse 1.0 600ms
`

const broadExample = `game: Sky Runner
gameid: T-99901G
system: saturn
revision: 3
---

watch health      byte 0x0605C973
watch lives       byte 0x0605CAE3
watch boss_hp     word 0x0600A244
# Mode byte: 0=boot 1=title 2=in play 3=boss 4=bonus 5=pause menu
watch gamestate   byte 0x0605D000
# Wall-collision flag is bit 0 of a packed flags byte.
watch collision   byte 0x0605D010 mask 0x01
watch surface     byte 0x0605D018
# This game's engine stores RPM little-endian.
watch rpm         word 0x06020400 little

def ingame gamestate in (2, 3, 4) and (lives > 0 or gamestate == 4)
def boss   gamestate == 3 and boss_hp != 0

# Scaled by the size of the health loss: chip damage at half
# strength, a 32-point hit at full.
on health decreased while ingame:
    pulse 0.6/0.3 200ms cooldown 100ms scale 1..32 -> 0.5..1.0

# Level heartbeat: a weak pulse roughly every 800ms while critical.
on health <= 4 while ingame:
    pulse 0/0.25 120ms level cooldown 800ms

# Pattern: big thump, a gap, then a trailing rumble.
on lives decreased while ingame:
    pattern (1.0 150ms, off 80ms, 0.6 300ms)

# Exact-delta: fires on a 1-up, not on menu rollovers.
on lives increased by 1 while ingame:
    pulse 0/0.5 250ms

on boss_hp decreased while boss:
    pulse 0.3/0.5 120ms cooldown 60ms scale 1..500 -> 0.4..1.0

on collision == 1 while ingame:
    pulse 0.4/0 100ms

# Held while off the track, released on return.
on surface == 3 while ingame:
    hold 0.2/0.4

# Ungated, buzzing while the engine is redlined.
on rpm >= 0x1C00:
    pulse 0.2/0.6 200ms level cooldown 250ms
`

const bandedHoldExample = `game: Rally Dash
gameid: T-99902G
system: saturn
---

watch surface byte 0x06041230
watch speed   word 0x06041244
# Mode byte: 2=racing.
watch mode    byte 0x06041200

def racing mode == 2

# Off-road rumble only while actually moving, harder at speed.
on surface == 3 and speed >= 0x0200 while racing: hold 0.5/0.8
on surface == 3 and speed > 0      while racing: hold 0.25/0.6
`

const countdownExample = `game: Deep Diver
gameid: T-99903G
system: saturn
---

watch air   byte 0x0605E010
# Counts up each time the low-air warning sounds.
watch dings byte 0x0605E014
watch mode  byte 0x0605E000

def diving mode == 3

# Continuous rumble ramping as air runs out.
on air <= 10 while diving: hold 0.5/0.8
on air <= 30 while diving: hold 0.2/0.5
on air <= 60 while diving: hold 0/0.25

# Or: a heartbeat that quickens and strengthens. Exclusive bands.
on air <= 60 and air > 30 while diving:
    pulse 0/0.2 100ms level cooldown 1500ms
on air <= 30 and air > 10 while diving:
    pulse 0/0.35 120ms level cooldown 800ms
on air <= 10 and air > 0 while diving:
    pulse 0.3/0.6 150ms level cooldown 400ms

# Or: synced to the warning itself, severity by first-wins order.
on dings increased and air <= 10 while diving: pulse 0.4/0.7 150ms
on dings increased and air <= 30 while diving: pulse 0/0.4 120ms
on dings increased while diving:               pulse 0/0.2 100ms
`

const perPlayerExample = `game: Tide Arena
gameid: T-99904G
system: saturn
---

watch p1_hp    byte 0x0605F000
watch p2_hp    byte 0x0605F200
# Water depth, 0 on land.
watch p1_depth word 0x0605F010
watch p2_depth word 0x0605F210
# Armor points. Auto-regenerating, so armored is the tuned baseline.
watch p1_armor byte 0x0605F030
watch p2_armor byte 0x0605F230
watch mode     byte 0x0605F0F0

def ingame mode == 2

# Underwater with no armor: the muffling and the extra sting offset,
# leaving a light dampen. First in the file so it wins the pool.
on p1_depth > 0 and p1_armor == 0 while ingame: dampen 10%
on p2_depth > 0 and p2_armor == 0 while ingame: dampen 10% player 2

# Water muffles everything for the submerged player only.
on p1_depth > 0 while ingame: dampen 30%
on p2_depth > 0 while ingame: dampen 30% player 2

# Armor gone: that player's hits land harder.
on p1_armor == 0 while ingame: amplify 25%
on p2_armor == 0 while ingame: amplify 25% player 2

on p1_hp decreased while ingame: pulse 0.6/0.3 200ms
on p2_hp decreased while ingame: pulse 0.6/0.3 200ms player 2
`

func mustParse(t *testing.T, src string) *Ruleset {
	t.Helper()
	rs, err := parseSource([]byte(src))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return rs
}

func TestParseMinimalExample(t *testing.T) {
	rs := mustParse(t, minimalExample)

	want := Metadata{Game: "Bulk Slash", GameID: "T-14310G", System: "saturn", Revision: 1}
	if rs.Metadata != want {
		t.Fatalf("metadata = %+v, want %+v", rs.Metadata, want)
	}
	if len(rs.Watches) != 2 || len(rs.Defs) != 0 || len(rs.Rules) != 2 {
		t.Fatalf("counts: values=%d defs=%d rules=%d", len(rs.Watches), len(rs.Defs), len(rs.Rules))
	}

	v := rs.Watches[0]
	if v.Name != "health" || v.Width != 8 || v.Address != 0x0605C973 || v.HasMask || v.Endian != EndianDefault {
		t.Fatalf("values[0] = %+v", v)
	}

	r := rs.Rules[0]
	cond, ok := r.On.(*PrevCond)
	if !ok || cond.Watch != "health" || cond.Op != OpDecreased || cond.Qual != QualNone {
		t.Fatalf("rules[0].On = %#v", r.On)
	}
	if r.While != nil || r.Level || r.CooldownMs != 0 || r.Player != 1 || r.PlayerAll || r.Scale != nil {
		t.Fatalf("rules[0] defaults = %+v", r)
	}
	e := r.Effect
	if e.Kind != EffectPulse || e.Strong != 0.76 || e.Weak != 0.76 || e.DurationMs != 200 {
		t.Fatalf("rules[0].Effect = %+v", e)
	}
}

func TestParseBroadExample(t *testing.T) {
	rs := mustParse(t, broadExample)

	if len(rs.Watches) != 7 || len(rs.Defs) != 2 || len(rs.Rules) != 8 {
		t.Fatalf("counts: values=%d defs=%d rules=%d", len(rs.Watches), len(rs.Defs), len(rs.Rules))
	}

	collision := rs.Watches[4]
	if collision.Name != "collision" || !collision.HasMask || collision.Mask != 0x01 {
		t.Fatalf("collision = %+v", collision)
	}
	rpm := rs.Watches[6]
	if rpm.Name != "rpm" || rpm.Width != 16 || rpm.Endian != EndianLittle {
		t.Fatalf("rpm = %+v", rpm)
	}

	// def ingame: and(in-set, or(compare, compare))
	ingame, ok := rs.Defs[0].Expr.(*BinaryExpr)
	if !ok || ingame.Op != OpAnd {
		t.Fatalf("ingame = %#v", rs.Defs[0].Expr)
	}
	set, ok := ingame.Left.(*SetCond)
	if !ok || set.Watch != "gamestate" || set.Negate || len(set.Set) != 3 || set.Set[2] != 4 {
		t.Fatalf("ingame.Left = %#v", ingame.Left)
	}
	orExpr, ok := ingame.Right.(*BinaryExpr)
	if !ok || orExpr.Op != OpOr {
		t.Fatalf("ingame.Right = %#v", ingame.Right)
	}

	hit := rs.Rules[0]
	if ref, ok := hit.While.(*DefRef); !ok || ref.Name != "ingame" {
		t.Fatalf("hit.While = %#v", hit.While)
	}
	if hit.CooldownMs != 100 || hit.Scale == nil {
		t.Fatalf("hit = %+v", hit)
	}
	if *hit.Scale != (Scale{MagMin: 1, MagMax: 32, MulMin: 0.5, MulMax: 1.0}) {
		t.Fatalf("hit.Scale = %+v", hit.Scale)
	}
	if hit.Effect.Strong != 0.6 || hit.Effect.Weak != 0.3 {
		t.Fatalf("hit.Effect = %+v", hit.Effect)
	}

	lowHealth := rs.Rules[1]
	if !lowHealth.Level || lowHealth.CooldownMs != 800 {
		t.Fatalf("lowHealth = %+v", lowHealth)
	}
	if lowHealth.Effect.Strong != 0 || lowHealth.Effect.Weak != 0.25 {
		t.Fatalf("lowHealth.Effect = %+v", lowHealth.Effect)
	}

	death := rs.Rules[2]
	if death.Effect.Kind != EffectPattern || len(death.Effect.Steps) != 3 {
		t.Fatalf("death.Effect = %+v", death.Effect)
	}
	steps := death.Effect.Steps
	if steps[0] != (Step{Strong: 1, Weak: 1, DurationMs: 150}) ||
		steps[1] != (Step{Strong: 0, Weak: 0, DurationMs: 80}) ||
		steps[2] != (Step{Strong: 0.6, Weak: 0.6, DurationMs: 300}) {
		t.Fatalf("death steps = %+v", steps)
	}

	oneUp := rs.Rules[3]
	cond, ok := oneUp.On.(*PrevCond)
	if !ok || cond.Op != OpIncreased || cond.Qual != QualBy || cond.Operand != 1 {
		t.Fatalf("oneUp.On = %#v", oneUp.On)
	}

	holdRule := rs.Rules[6]
	if holdRule.Effect.Kind != EffectHold || holdRule.Effect.Strong != 0.2 || holdRule.Effect.Weak != 0.4 {
		t.Fatalf("hold = %+v", holdRule.Effect)
	}
	if surface, ok := holdRule.On.(*CompareCond); !ok || surface.Watch != "surface" || surface.Op != OpEq || surface.Operand != 3 {
		t.Fatalf("hold.On = %#v", holdRule.On)
	}

	redline := rs.Rules[7]
	if redline.While != nil || !redline.Level || redline.CooldownMs != 250 {
		t.Fatalf("redline = %+v", redline)
	}
	if cmp, ok := redline.On.(*CompareCond); !ok || cmp.Op != OpGe || cmp.Operand != 0x1C00 {
		t.Fatalf("redline.On = %#v", redline.On)
	}
}

func TestParsePrecedence(t *testing.T) {
	rs := mustParse(t, "def d a == 1 or b == 2 and c == 3\n")
	// and binds tighter than or: or(a==1, and(b==2, c==3))
	orExpr, ok := rs.Defs[0].Expr.(*BinaryExpr)
	if !ok || orExpr.Op != OpOr {
		t.Fatalf("top = %#v", rs.Defs[0].Expr)
	}
	if _, ok := orExpr.Left.(*CompareCond); !ok {
		t.Fatalf("left = %#v", orExpr.Left)
	}
	andExpr, ok := orExpr.Right.(*BinaryExpr)
	if !ok || andExpr.Op != OpAnd {
		t.Fatalf("right = %#v", orExpr.Right)
	}
}

func TestParseParens(t *testing.T) {
	rs := mustParse(t, "def d (a == 1 or b == 2) and c == 3\n")
	andExpr, ok := rs.Defs[0].Expr.(*BinaryExpr)
	if !ok || andExpr.Op != OpAnd {
		t.Fatalf("top = %#v", rs.Defs[0].Expr)
	}
	if orExpr, ok := andExpr.Left.(*BinaryExpr); !ok || orExpr.Op != OpOr {
		t.Fatalf("left = %#v", andExpr.Left)
	}
}

func TestParseNotIn(t *testing.T) {
	rs := mustParse(t, "def d gamestate not in (0, 1)\n")
	set, ok := rs.Defs[0].Expr.(*SetCond)
	if !ok || !set.Negate || len(set.Set) != 2 || set.Set[0] != 0 || set.Set[1] != 1 {
		t.Fatalf("expr = %#v", rs.Defs[0].Expr)
	}
}

func TestParseConst(t *testing.T) {
	spec := mustParse(t, "const marker 0xFD39\nwatch x word 0x100\non x == marker: pulse 1.0 100ms\n")
	if len(spec.Consts) != 1 || spec.Consts[0].Name != "marker" || spec.Consts[0].Value != 0xFD39 {
		t.Fatalf("consts = %+v", spec.Consts)
	}
	cond, ok := spec.Rules[0].On.(*CompareCond)
	if !ok || cond.Operand != 0xFD39 {
		t.Fatalf("operand not folded: %#v", spec.Rules[0].On)
	}
}

func TestParseConstInSetAndDelta(t *testing.T) {
	spec := mustParse(t, "const a 2\nconst b 3\nwatch x byte 0x100\n"+
		"on x in (a, b): pulse 1.0 100ms\non x increased by a: pulse 1.0 100ms\n")
	set := spec.Rules[0].On.(*SetCond)
	if len(set.Set) != 2 || set.Set[0] != 2 || set.Set[1] != 3 {
		t.Fatalf("set = %+v", set.Set)
	}
	prev := spec.Rules[1].On.(*PrevCond)
	if prev.Qual != QualBy || prev.Operand != 2 {
		t.Fatalf("delta = %+v", prev)
	}
}

func TestParseHexDelta(t *testing.T) {
	rs := mustParse(t, "on x increased by 0x10: pulse 1.0 100ms\n")
	cond := rs.Rules[0].On.(*PrevCond)
	if cond.Qual != QualBy || cond.Operand != 0x10 {
		t.Fatalf("cond = %+v", cond)
	}
}

func TestParseAnchoredPrevConds(t *testing.T) {
	rs := mustParse(t, "const marker 0xFD39\nwatch x word 0x100\n"+
		"on x changed from marker: pulse 1.0 100ms\n"+
		"on x changed to 3: pulse 1.0 100ms\n"+
		"on x increased from 0x10: pulse 1.0 100ms\n"+
		"on x increased to 5: pulse 1.0 100ms\n"+
		"on x decreased from 5: pulse 1.0 100ms\n"+
		"on x decreased to 0: pulse 1.0 100ms\n")
	want := []struct {
		op      PrevOp
		qual    PrevQual
		operand uint32
	}{
		{OpChanged, QualFrom, 0xFD39},
		{OpChanged, QualTo, 3},
		{OpIncreased, QualFrom, 0x10},
		{OpIncreased, QualTo, 5},
		{OpDecreased, QualFrom, 5},
		{OpDecreased, QualTo, 0},
	}
	for i, w := range want {
		cond, ok := rs.Rules[i].On.(*PrevCond)
		if !ok || cond.Op != w.op || cond.Qual != w.qual || cond.Operand != w.operand {
			t.Fatalf("rules[%d].On = %#v, want %+v", i, rs.Rules[i].On, w)
		}
	}
}

func TestParseSignedAbsWatches(t *testing.T) {
	rs := mustParse(t, "watch fall word 0x100 signed\n"+
		"watch slip long 0x104 abs\n"+
		"watch temp word 0x108 mask 0x7FFF signed little\n"+
		"watch plain word 0x10C\n")
	cases := []struct {
		signed, abs bool
	}{
		{true, false},
		{false, true},
		{true, false},
		{false, false},
	}
	for i, w := range cases {
		v := rs.Watches[i]
		if v.Signed != w.signed || v.Abs != w.abs {
			t.Fatalf("watches[%d] = %+v, want signed=%v abs=%v", i, v, w.signed, w.abs)
		}
	}
	if rs.Watches[2].Mask != 0x7FFF || rs.Watches[2].Endian != EndianLittle {
		t.Fatalf("watches[2] = %+v", rs.Watches[2])
	}
}

func TestParseNegativeLiterals(t *testing.T) {
	rs := mustParse(t, "const floor -100\n"+
		"watch x word 0x100 signed\n"+
		"on x <= -6: pulse 1.0 100ms\n"+
		"on x in (-1, 3): pulse 1.0 100ms\n"+
		"on x changed to -0x10: pulse 1.0 100ms\n"+
		"on x < floor: pulse 1.0 100ms\n"+
		"on x decreased: pulse 1.0 100ms scale x -12..-6 -> 1.0..0.4\n"+
		"on x == -0x80000000: pulse 1.0 100ms\n")
	if got := rs.Consts[0].Value; got != 0xFFFFFF9C {
		t.Fatalf("const = 0x%X", got)
	}
	if c := rs.Rules[0].On.(*CompareCond); c.Operand != 0xFFFFFFFA {
		t.Fatalf("compare operand = 0x%X", c.Operand)
	}
	if s := rs.Rules[1].On.(*SetCond); s.Set[0] != 0xFFFFFFFF || s.Set[1] != 3 {
		t.Fatalf("set = %v", s.Set)
	}
	if c := rs.Rules[2].On.(*PrevCond); c.Operand != 0xFFFFFFF0 {
		t.Fatalf("anchor operand = 0x%X", c.Operand)
	}
	if c := rs.Rules[3].On.(*CompareCond); c.Operand != 0xFFFFFF9C {
		t.Fatalf("const operand = 0x%X", c.Operand)
	}
	sc := rs.Rules[4].Scale
	if sc.MagMin != 0xFFFFFFF4 || sc.MagMax != 0xFFFFFFFA {
		t.Fatalf("scale range = 0x%X..0x%X", sc.MagMin, sc.MagMax)
	}
	if !sc.MagMinNeg || !sc.MagMaxNeg {
		t.Fatalf("scale minus signs not recorded: %+v", sc)
	}
	if c := rs.Rules[5].On.(*CompareCond); c.Operand != 0x80000000 {
		t.Fatalf("min operand = 0x%X", c.Operand)
	}
}

func TestParseDeltaBounds(t *testing.T) {
	rs := mustParse(t, "const jump 0x20\nwatch x word 0x100\n"+
		"on x increased by at least 5: pulse 1.0 100ms\n"+
		"on x decreased by at least jump: pulse 1.0 100ms\n"+
		"on x increased by at most 3: pulse 1.0 100ms\n"+
		"on x decreased by at most 0x10: pulse 1.0 100ms\n")
	want := []struct {
		op      PrevOp
		qual    PrevQual
		operand uint32
	}{
		{OpIncreased, QualAtLeast, 5},
		{OpDecreased, QualAtLeast, 0x20},
		{OpIncreased, QualAtMost, 3},
		{OpDecreased, QualAtMost, 0x10},
	}
	for i, w := range want {
		cond, ok := rs.Rules[i].On.(*PrevCond)
		if !ok || cond.Op != w.op || cond.Qual != w.qual || cond.Operand != w.operand {
			t.Fatalf("rules[%d].On = %#v, want %+v", i, rs.Rules[i].On, w)
		}
	}
}

func TestParseClip(t *testing.T) {
	rs := mustParse(t, "on x == 1: pulse 1.0 100ms clip\n"+
		"on y == 2: pattern (1.0 100ms) clip 250ms level\n")
	if r := rs.Rules[0]; !r.Clip || r.ClipMs != 0 {
		t.Fatalf("rules[0] = %+v", r)
	}
	if r := rs.Rules[1]; !r.Clip || r.ClipMs != 250 || !r.Level {
		t.Fatalf("rules[1] = %+v", r)
	}
	// A rule without the modifier carries no clip.
	rs = mustParse(t, "on x == 1: pulse 1.0 100ms\n")
	if r := rs.Rules[0]; r.Clip || r.ClipMs != 0 {
		t.Fatalf("rules[0] = %+v", r)
	}
}

func TestParseModifiersAnyOrder(t *testing.T) {
	rs := mustParse(t, "on x decreased: pulse 1.0 100ms scale 1..2 -> 0.5..1.0 level cooldown 50ms player 2 priority 4\n")
	r := rs.Rules[0]
	if !r.Level || r.Priority != 4 || r.CooldownMs != 50 || r.Player != 2 || r.PlayerAll || r.Scale == nil {
		t.Fatalf("rule = %+v", r)
	}
}

func TestParsePriority(t *testing.T) {
	rs := mustParse(t, "on x == 1: pulse 1.0 100ms priority -1\n"+
		"on x == 2: pulse 1.0 100ms priority 99\n"+
		"on x == 3: pulse 1.0 100ms\n")
	want := []int{-1, 99, 0}
	for i, w := range want {
		if got := rs.Rules[i].Priority; got != w {
			t.Fatalf("rules[%d] priority = %d, want %d", i, got, w)
		}
	}
}

func TestParsePriorityErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"above range", "on x == 1: pulse 1.0 100ms priority 100\n", "priority 100 must be -99 to 99"},
		{"below range", "on x == 1: pulse 1.0 100ms priority -100\n", "must be -99 to 99"},
		{"not a number", "on x == 1: pulse 1.0 100ms priority high\n", "expected priority value"},
		{"missing value", "on x == 1: pulse 1.0 100ms priority\n", "priority value"},
		{"duplicate", "on x == 1: pulse 1.0 100ms priority 1 priority 2\n", "duplicate priority modifier"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseSource([]byte(c.src))
			if err == nil {
				t.Fatalf("expected an error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %q, want it to contain %q", err, c.want)
			}
		})
	}
}

func TestParseScaleOnNamedWatch(t *testing.T) {
	rs := mustParse(t, "on x == 1: pulse 1.0 100ms scale speed 0x10..0x20 -> 0.2..0.9\n")
	got := rs.Rules[0].Scale
	if got == nil || *got != (Scale{Watch: "speed", MagMin: 0x10, MagMax: 0x20, MulMin: 0.2, MulMax: 0.9}) {
		t.Fatalf("scale = %+v", got)
	}
	// The name is optional, and omitting it leaves Watch empty.
	rs = mustParse(t, "on x decreased: pulse 1.0 100ms scale 1..2 -> 0.5..1.0\n")
	if got := rs.Rules[0].Scale; got == nil || got.Watch != "" {
		t.Fatalf("scale = %+v", got)
	}
}

func TestParseHeldEvent(t *testing.T) {
	rs := mustParse(t, "event grinding surface == 3 and mode == 2 held for 12 frames\n"+
		"event spent meter decreased for 2500ms\n")
	ev := rs.Events[0]
	if !ev.Held || ev.HeldFrames != 12 || ev.DurationMs != 0 {
		t.Fatalf("events[0] = %+v", ev)
	}
	if _, ok := ev.Cond.(*BinaryExpr); !ok {
		t.Fatalf("events[0].Cond = %#v", ev.Cond)
	}
	ev = rs.Events[1]
	if ev.Held || ev.Cond != nil || ev.DurationMs != 2500 {
		t.Fatalf("events[1] = %+v", ev)
	}
}

// A non-const name on a comparison's right side parses as a watch
// comparison; a const name still folds to its value.
func TestParseWatchComparison(t *testing.T) {
	rs := mustParse(t, "const k 5\n"+
		"on a > b: pulse 1.0 100ms\n"+
		"on a == k: pulse 1.0 100ms\n")
	wc, ok := rs.Rules[0].On.(*CompareWatchCond)
	if !ok || wc.Left != "a" || wc.Op != OpGt || wc.Right != "b" {
		t.Fatalf("rules[0].On = %#v", rs.Rules[0].On)
	}
	cc, ok := rs.Rules[1].On.(*CompareCond)
	if !ok || cc.Operand != 5 {
		t.Fatalf("rules[1].On = %#v", rs.Rules[1].On)
	}
}

func TestParseBCDWatch(t *testing.T) {
	rs := mustParse(t, "watch score word 0x10 bcd little\n"+
		"watch digit byte 0x20 mask 0x0F bcd\n")
	v := rs.Watches[0]
	if !v.BCD || v.Signed || v.Abs || v.Endian != EndianLittle {
		t.Fatalf("watches[0] = %+v", v)
	}
	v = rs.Watches[1]
	if !v.BCD || !v.HasMask || v.Mask != 0x0F {
		t.Fatalf("watches[1] = %+v", v)
	}
}

func TestParseNot(t *testing.T) {
	rs := mustParse(t, "on not wet: hold 0.5\n"+
		"on x == 1 while g and not wet: pulse 1.0 100ms\n")
	ref, ok := rs.Rules[0].On.(*DefRef)
	if !ok || !ref.Negate || ref.Name != "wet" {
		t.Fatalf("rules[0].On = %#v", rs.Rules[0].On)
	}
	while, ok := rs.Rules[1].While.(*BinaryExpr)
	if !ok || while.Op != OpAnd {
		t.Fatalf("rules[1].While = %#v", rs.Rules[1].While)
	}
	ref, ok = while.Right.(*DefRef)
	if !ok || !ref.Negate || ref.Name != "wet" {
		t.Fatalf("rules[1].While.Right = %#v", while.Right)
	}
	if left, ok := while.Left.(*DefRef); !ok || left.Negate || left.Name != "g" {
		t.Fatalf("rules[1].While.Left = %#v", while.Left)
	}
}

func TestParseDampenAmplify(t *testing.T) {
	rs := mustParse(t, "on x == 1: dampen 30%\n"+
		"on x == 2 while g: amplify 150% player 2\n")
	e := rs.Rules[0].Effect
	if e.Kind != EffectDampen || e.Percent != 30 || e.Strong != 0 || e.Weak != 0 ||
		e.DurationMs != 0 || e.Steps != nil {
		t.Fatalf("rules[0].Effect = %+v", e)
	}
	e = rs.Rules[1].Effect
	if e.Kind != EffectAmplify || e.Percent != 150 {
		t.Fatalf("rules[1].Effect = %+v", e)
	}
	if rs.Rules[1].Player != 2 || rs.Rules[1].While == nil {
		t.Fatalf("rules[1] = %+v", rs.Rules[1])
	}
}

// A scale range endpoint may be a const name. A name right after
// "scale" is the watch unless ".." follows it.
func TestParseScaleConstRange(t *testing.T) {
	rs := mustParse(t, "const lo 5\nconst hi 10\n"+
		"on x decreased: pulse 1.0 100ms scale lo..hi -> 0.5..1.0\n"+
		"on x == 1: pulse 1.0 100ms scale mag lo..hi -> 0.5..1.0\n"+
		"on x == 1: pulse 1.0 100ms scale mag lo..20 -> 0.5..1.0\n")
	got := rs.Rules[0].Scale
	if got == nil || *got != (Scale{MagMin: 5, MagMax: 10, MulMin: 0.5, MulMax: 1.0}) {
		t.Fatalf("rules[0].Scale = %+v", got)
	}
	got = rs.Rules[1].Scale
	if got == nil || *got != (Scale{Watch: "mag", MagMin: 5, MagMax: 10, MulMin: 0.5, MulMax: 1.0}) {
		t.Fatalf("rules[1].Scale = %+v", got)
	}
	got = rs.Rules[2].Scale
	if got == nil || got.Watch != "mag" || got.MagMin != 5 || got.MagMax != 20 {
		t.Fatalf("rules[2].Scale = %+v", got)
	}
}

func TestParseArrayWatch(t *testing.T) {
	rs := mustParse(t, "watch pool byte 0x10 stride 0x70 count 32 mask 0x3F\n")
	v := rs.Watches[0]
	if v.Stride != 0x70 || v.Count != 32 || !v.HasMask || v.Mask != 0x3F {
		t.Fatalf("watch = %+v", v)
	}
	rs = mustParse(t, "watch pool word 0x10 stride 2 count 4 little\n")
	v = rs.Watches[0]
	if v.Stride != 2 || v.Count != 4 || v.Endian != EndianLittle {
		t.Fatalf("watch = %+v", v)
	}
}

func TestParsePlayerAll(t *testing.T) {
	rs := mustParse(t, "on x == 1: pulse 1.0 100ms player all\n")
	r := rs.Rules[0]
	if !r.PlayerAll {
		t.Fatalf("rule = %+v", r)
	}
}

func TestParseContinuationJoinsStatement(t *testing.T) {
	src := "on x == 1\n" +
		"    while g:\n" +
		"    pulse 1.0\n" +
		"    100ms\n"
	rs := mustParse(t, src)
	r := rs.Rules[0]
	if ref, ok := r.While.(*DefRef); !ok || ref.Name != "g" {
		t.Fatalf("while = %#v", r.While)
	}
	if r.Effect.DurationMs != 100 {
		t.Fatalf("effect = %+v", r.Effect)
	}
}

func TestParseEvent(t *testing.T) {
	rs := mustParse(t, "watch meter byte 0x10\nevent spent meter decreased for 2500ms\n")
	if len(rs.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(rs.Events))
	}
	ev := rs.Events[0]
	if ev.Name != "spent" || ev.DurationMs != 2500 || ev.Line != 2 {
		t.Fatalf("event = %+v", ev)
	}
	if ev.Trigger.Watch != "meter" || ev.Trigger.Op != OpDecreased || ev.Trigger.Qual != QualNone {
		t.Fatalf("trigger = %+v", ev.Trigger)
	}
}

func TestParseMetadata(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want Metadata
	}{
		{
			"all fields",
			"game: Sky Runner\ngameid: T-99901G\nsystem: saturn\nrevision: 7\n---\nwatch x byte 1\n",
			Metadata{Game: "Sky Runner", GameID: "T-99901G", System: "saturn", Revision: 7},
		},
		{
			"colon in value",
			"game: Sonic: The Hedgehog\n---\n",
			Metadata{Game: "Sonic: The Hedgehog", Revision: 1},
		},
		{
			"no space after colon",
			"gameid:T-99901G\n---\n",
			Metadata{GameID: "T-99901G", Revision: 1},
		},
		{
			"quotes are part of the value",
			`game: "Sky Runner"` + "\n---\n",
			Metadata{Game: `"Sky Runner"`, Revision: 1},
		},
		{
			"date revision",
			"revision: 20260730\n---\n",
			Metadata{Revision: 20260730},
		},
		{
			"comments and blanks in the block",
			"# who\ngame: Sky Runner\n\n# what\ngameid: T-99901G\n---\n",
			Metadata{Game: "Sky Runner", GameID: "T-99901G", Revision: 1},
		},
		{
			"empty block",
			"---\nwatch x byte 1\n",
			Metadata{Revision: 1},
		},
		{
			"no block",
			"watch x byte 1\n",
			Metadata{Revision: 1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := mustParse(t, tc.src)
			if rs.Metadata != tc.want {
				t.Fatalf("metadata = %+v, want %+v", rs.Metadata, tc.want)
			}
		})
	}
}

// A metadata key is reserved in statement position only, so it stays
// available as a declared name.
func TestParseMetadataKeysAreLegalNames(t *testing.T) {
	src := "gameid: T-99901G\n---\n" +
		"watch system byte 0x100\n" +
		"watch revision byte 0x101\n" +
		"def game system == 1 and revision == 2\n" +
		"on game: pulse 1.0 100ms\n"
	rs := mustParse(t, src)
	if len(rs.Watches) != 2 || len(rs.Defs) != 1 || len(rs.Rules) != 1 {
		t.Fatalf("counts: watches=%d defs=%d rules=%d", len(rs.Watches), len(rs.Defs), len(rs.Rules))
	}
	if rs.Metadata.GameID != "T-99901G" {
		t.Fatalf("metadata = %+v", rs.Metadata)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"unknown statement", "bogus 1 2\n", `line 1: unknown statement "bogus"`},
		{"meta no terminator", "game: A\nwatch x byte 1\n", `line 1: metadata statement "game" must be inside a metadata block`},
		{"meta after terminator", "---\nwatch x byte 1\ngameid: A-1\n", `line 3: metadata statement "gameid" must be inside a metadata block`},
		{"meta without colon", "game Bulk Slash\n---\n", `line 1: metadata statement "game" needs a colon after the key`},
		{"meta empty value", "game:\n---\n", "line 1: game has an empty value"},
		{"meta continuation", "game: A\n   more\n---\n", "line 2: metadata statement does not continue"},
		{"revision not an integer", "revision: v2\n---\n", `line 1: revision "v2" is not an integer`},
		{"revision zero", "revision: 0\n---\n", "line 1: revision must be 1 or greater"},
		{"revision negative", "revision: -1\n---\n", "line 1: revision must be 1 or greater"},
		{"second terminator", "game: A\n---\nwatch x byte 1\n---\n", `line 4: unexpected "---"`},
		{"terminator not alone", "--- extra\nwatch x byte 1\n", `line 1: the --- terminator must be alone on its line`},
		{"terminator continuation", "game: A\n---\n   more\n", "line 3: the --- terminator does not continue"},
		{"event bad trigger", "watch h byte 1\nevent e h == 3 for 100ms\n", "line 2: event trigger must be a previous-frame condition"},
		{"event def trigger", "watch h byte 1\ndef d h == 3\nevent e d for 100ms\n", "line 3: event trigger must be a previous-frame condition"},
		{"event missing for", "watch h byte 1\nevent e h decreased 100ms\n", `line 2: expected "for" after event trigger`},
		{"event missing duration", "watch h byte 1\nevent e h decreased for\n", "line 2: expected event duration at end of statement"},
		{"event trailing", "watch h byte 1\nevent e h decreased for 100ms extra\n", `line 2: unexpected "extra" after event declaration`},
		{"held missing for", "watch h byte 1\nevent e h == 1 held 12 frames\n", `line 2: expected "for" after "held"`},
		{"held missing frames unit", "watch h byte 1\nevent e h == 1 held for 12\n", `line 2: expected "frames" after the frame count`},
		{"held ms duration", "watch h byte 1\nevent e h == 1 held for 200ms\n", `line 2: expected frame count, found "200ms"`},
		{"held trailing", "watch h byte 1\nevent e h == 1 held for 2 frames extra\n", `line 2: unexpected "extra" after event declaration`},
		{"continuation first", "   pulse 1.0 100ms\n", "line 1: continuation line without a statement"},
		{"duplicate metadata", "game: A\ngame: B\n---\n", "line 2: duplicate game statement"},
		{"bad width", "watch h dword 0x100\n", "width must be byte, word, or long"},
		{"missing address", "watch h byte\n", "expected address"},
		{"address range", "watch h byte 0x100000000\n", "address 0x100000000 out of range"},
		{"watch trailing", "watch h byte 0x100 little extra\n", `unexpected "extra" after watch declaration`},
		{"missing colon", "on x == 1 pulse 1.0 100ms\n", `expected ":"`},
		{"missing effect", "on x == 1:\n", "expected effect at end of statement"},
		{"bad effect", "on x == 1: buzz 1.0 100ms\n", "expected pulse, pattern, hold, dampen, or amplify"},
		{"dampen without suffix", "on x == 1: dampen 10\n", `expected dampen percent, found "10"`},
		{"amplify missing percent", "on x == 1: amplify\n", "expected amplify percent at end of statement"},
		{"dampen negative percent", "on x == 1: dampen -10%\n", `expected dampen percent, found "-10%"`},
		{"percent as duration", "on x == 1: pulse 1.0 100%\n", `expected duration, found "100%"`},
		{"bare percent sign", "on x == 1: dampen 10 %\n", `unexpected character "%"`},
		{"not before comparison", "on not x == 5: pulse 1.0 100ms\n", `"not" negates a def or event name; a condition is negated with its complement form`},
		{"not before in", "on not x in (2): pulse 1.0 100ms\n", `"not" negates a def or event name`},
		{"not before prev cond", "on not x changed: pulse 1.0 100ms\n", `"not" negates a def or event name`},
		{"not before paren", "on not (x == 1): pulse 1.0 100ms\n", `expected a def or event name after "not"`},
		{"double not", "on not not wet: pulse 1.0 100ms\n", `expected a def or event name after "not"`},
		{"not without name", "on not: pulse 1.0 100ms\n", `expected a def or event name after "not"`},
		{"scale unknown const in range", "on x decreased: pulse 1.0 100ms scale zzz..5 -> 0.5..1.0\n", `unknown constant "zzz"`},
		{"scale keyword in range", "on x decreased: pulse 1.0 100ms scale level..5 -> 0.5..1.0\n", `expected scale range, found "level"`},
		{"hex intensity", "on x == 1: pulse 0x1 100ms\n", "intensity must be decimal"},
		{"fractional duration", "on x == 1: pulse 1.0 1.5ms\n", `malformed number "1.5ms"`},
		{"missing duration", "on x == 1: pulse 1.0\n", "expected duration at end of statement"},
		{"empty pattern", "on x == 1: pattern ()\n", "expected intensity"},
		{"duplicate modifier", "on x == 1: pulse 1.0 100ms cooldown 10ms cooldown 20ms\n", "duplicate cooldown modifier"},
		{"unknown modifier", "on x == 1: pulse 1.0 100ms loud\n", `unknown modifier "loud"`},
		{"bad scale", "on x decreased: pulse 1.0 100ms scale 1..2 0.5..1.0\n", `expected "->"`},
		{"stride without count", "watch x byte 1 stride 4\n", `expected "count" after stride`},
		{"count out of range", "watch x byte 1 stride 4 count 0x10000\n", "count 65536 out of range"},
		{"keyword condition", "on level == 1: pulse 1.0 100ms\n", `expected condition, found "level"`},
		{"not without in", "def d x not (1, 2)\n", `expected "in"`},
		{"def trailing", "def d x == 1 extra\n", `unexpected "extra" in def`},
		{"unclosed paren", "def d (x == 1\n", `expected ")"`},
		{"bad char", "on x == 1: pulse 1.0 100ms ^\n", `unexpected character "^"`},
		{"const trailing", "const m 5 6\n", `unexpected "6" in const`},
		{"unchanged anchor", "on x unchanged from 1: pulse 1.0 100ms\n", `"from" is not valid after this condition`},
		{"changed by", "on x changed by 1: pulse 1.0 100ms\n", `"by" is not valid after this condition`},
		{"by then from", "on x increased by 2 from 3: pulse 1.0 100ms\n", `at most one of "by", "from", and "to"`},
		{"bound then from", "on x increased by at least 2 from 3: pulse 1.0 100ms\n", `at most one of "by", "from", and "to"`},
		{"bound then bound", "on x increased by at least 2 at most 5: pulse 1.0 100ms\n", `at most one of "by", "from", and "to"`},
		{"at without by", "on x increased at least 5: pulse 1.0 100ms\n", `"at least" and "at most" follow "by"`},
		{"at bad word", "on x increased by at more 5: pulse 1.0 100ms\n", `expected "least" or "most" after "at"`},
		{"at missing word", "on x increased by at: pulse 1.0 100ms\n", `expected "least" or "most" after "at"`},
		{"bound missing operand", "on x increased by at least: pulse 1.0 100ms\n", "expected delta constant"},
		{"negative delta", "on x increased by -5: pulse 1.0 100ms\n", `delta constant must be non-negative, found "-5"`},
		{"negative bound", "on x decreased by at least -5: pulse 1.0 100ms\n", `delta constant must be non-negative, found "-5"`},
		{"negative duration", "on x == 1: pulse 1.0 -100ms\n", `expected duration, found "-100ms"`},
		{"negative intensity", "on x == 1: pulse -0.5 100ms\n", `intensity must be non-negative, found "-0.5"`},
		{"negative address", "watch x byte -1\n", `expected address, found "-1"`},
		{"negative mask", "watch x byte 1 mask -1\n", `expected mask, found "-1"`},
		{"negative count", "watch x byte 1 stride 4 count -2\n", `expected count, found "-2"`},
		{"negative player", "on x == 1: pulse 1.0 100ms player -1\n", `expected player number or all, found "-1"`},
		{"negative cooldown", "on x == 1: pulse 1.0 100ms cooldown -10ms\n", `expected cooldown duration, found "-10ms"`},
		{"negative scale multiplier", "on x decreased: pulse 1.0 100ms scale 1..2 -> -0.5..1.0\n", `scale multiplier must be non-negative, found "-0.5"`},
		{"operand magnitude overflow", "on x == -0x80000001: pulse 1.0 100ms\n", "comparison constant -0x80000001 out of range"},
		{"bare minus", "on x == - 5: pulse 1.0 100ms\n", `unexpected character "-"`},
		{"uppercase hex prefix", "on x == 0X2A: pulse 1.0 100ms\n", `malformed number "0X2A"`},
		{"from then to", "on x changed from 3 to 4: pulse 1.0 100ms\n", `at most one of "by", "from", and "to"`},
		{"missing anchor operand", "on x changed from: pulse 1.0 100ms\n", "expected anchor constant"},
		{"duplicate clip", "on x == 1: pulse 1.0 100ms clip clip 10ms\n", "duplicate clip modifier"},
		{"negative clip duration", "on x == 1: pulse 1.0 100ms clip -10ms\n", `expected clip duration, found "-10ms"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSource([]byte(tc.src))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestParseDepthLimit(t *testing.T) {
	deep := "def d " + strings.Repeat("(", 100) + "x == 1" + strings.Repeat(")", 100) + "\n"
	if _, err := parseSource([]byte(deep)); err == nil || !strings.Contains(err.Error(), "nested too deeply") {
		t.Fatalf("err = %v", err)
	}
	ok := "watch x byte 1\ndef d " + strings.Repeat("(", 32) + "x == 1" + strings.Repeat(")", 32) + "\n"
	if _, err := Parse([]byte(ok)); err != nil {
		t.Fatalf("moderate nesting rejected: %v", err)
	}
}

func TestParseErrorLineNumbers(t *testing.T) {
	src := "watch h byte 0x100\n" +
		"\n" +
		"on h decreased:\n" +
		"    pulse 1.0 100ms\n" +
		"    cooldown 10ms cooldown 20ms\n"
	_, err := parseSource([]byte(src))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "line 5:") {
		t.Fatalf("error %q does not name line 5", err.Error())
	}
}

func TestParsePointerWatch(t *testing.T) {
	rs := mustParse(t, "watch health word ptr 0x0601F320 offset 0x24\n")
	v := rs.Watches[0]
	if !v.Pointer || v.Address != 0x0601F320 || v.Offset != 0x24 || v.Width != 16 {
		t.Fatalf("watch = %+v", v)
	}
	rs = mustParse(t, "watch flags byte ptr 0x0601F320 mask 0x0F little\n")
	v = rs.Watches[0]
	if !v.Pointer || v.Offset != 0 || !v.HasMask || v.Mask != 0x0F || v.Endian != EndianLittle {
		t.Fatalf("watch = %+v", v)
	}
}

func TestParseKeyedSlotWatch(t *testing.T) {
	rs := mustParse(t, "watch locks byte 0x0604D154 stride 0x200 count 255\n"+
		"    key 0 0x06007856 field 0x173\n")
	v := rs.Watches[0]
	if !v.HasKey || v.KeyOffset != 0 || v.KeyValue != 0x06007856 ||
		v.FieldOffset != 0x173 || v.FieldPtr || v.Offset != 0 {
		t.Fatalf("watch = %+v", v)
	}

	rs = mustParse(t, "const dragon 0x06007856\n"+
		"watch part byte 0x10 stride 0x200 count 8 key 4 dragon field 0x190 ptr offset 88\n")
	v = rs.Watches[0]
	if !v.HasKey || v.KeyOffset != 4 || v.KeyValue != 0x06007856 ||
		v.FieldOffset != 0x190 || !v.FieldPtr || v.Offset != 88 {
		t.Fatalf("watch = %+v", v)
	}
}

func TestParsePointerAndKeyErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"ptr with stride", "watch x byte ptr 0x10 stride 4 count 2\n", `unexpected "stride" after watch declaration`},
		{"offset without ptr", "watch x byte 0x10 offset 4\n", `unexpected "offset" after watch declaration`},
		{"key before stride", "watch x byte 0x10 key 0 5 field 4\n", `unexpected "key" after watch declaration`},
		{"key without field", "watch x byte 0x10 stride 8 count 2 key 0 5\n", `expected "field" after the key value`},
		{"field offset missing", "watch x byte 0x10 stride 8 count 2 key 0 5 field\n", "expected field offset"},
		{"key value unknown const", "watch x byte 0x10 stride 8 count 2 key 0 nope field 4\n", `unknown constant "nope"`},
		{"ptr missing address", "watch x byte ptr\n", "expected address"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSource([]byte(tc.src))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}
