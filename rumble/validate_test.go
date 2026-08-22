package rumble

import (
	"math"
	"strings"
	"testing"
)

func TestParseExamplesValidate(t *testing.T) {
	if _, err := Parse([]byte(minimalExample)); err != nil {
		t.Fatalf("minimal example: %v", err)
	}
	if _, err := Parse([]byte(broadExample)); err != nil {
		t.Fatalf("broad example: %v", err)
	}
	if _, err := Parse([]byte(bandedHoldExample)); err != nil {
		t.Fatalf("banded hold example: %v", err)
	}
	if _, err := Parse([]byte(countdownExample)); err != nil {
		t.Fatalf("countdown example: %v", err)
	}
	if _, err := Parse([]byte(perPlayerExample)); err != nil {
		t.Fatalf("per-player example: %v", err)
	}
}

func TestProgrammaticSpecValidates(t *testing.T) {
	rs := Ruleset{
		Watches: []Watch{
			{Name: "health", Width: 8, Address: 0x0605C973},
			{Name: "deaths", Width: 8, Address: 0x0605CAE3},
		},
		Rules: []Rule{
			{
				On:     &PrevCond{Watch: "health", Op: OpDecreased},
				Effect: Effect{Kind: EffectPulse, Strong: 0.76, Weak: 0.76, DurationMs: 200},
				Player: 1,
			},
			{
				On:     &PrevCond{Watch: "deaths", Op: OpIncreased},
				Effect: Effect{Kind: EffectPulse, Strong: 1, Weak: 1, DurationMs: 600},
				Player: 1,
			},
		},
	}
	if err := validateRuleset(&rs); err != nil {
		t.Fatalf("validation failed: %v", err)
	}
}

func TestScaleDescendingMultiplierValidates(t *testing.T) {
	src := "watch x byte 1\non x decreased: pulse 1.0 100ms scale 1..10 -> 1.0..0.25\n"
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatalf("descending multiplier range: %v", err)
	}
	src = "watch x byte 1\non x decreased: pulse 1.0 100ms scale 1..10 -> 0.5..0\n"
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatalf("descending to zero: %v", err)
	}
}

func TestArrayWatchValidates(t *testing.T) {
	src := "watch pool byte 0x10 stride 4 count 8\n" +
		"watch mode byte 0\n" +
		"def busy pool != 0\n" +
		"on pool in (40, 41) while busy and mode == 6: pulse 0.5 100ms\n"
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatalf("array watch file: %v", err)
	}
}

func TestValidateErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"duplicate watch name",
			"watch x byte 1\nwatch x byte 2\n",
			`line 2: duplicate name "x"`},
		{"duplicate event name",
			"watch x byte 1\nevent x x decreased for 100ms\n",
			`line 2: duplicate name "x"`},
		{"event unchanged trigger",
			"watch x byte 1\nevent e x unchanged for 100ms\n",
			`line 2: event "e" trigger cannot be unchanged`},
		{"event zero duration",
			"watch x byte 1\nevent e x decreased for 0ms\n",
			`line 2: event "e" duration must be positive`},
		{"event undeclared watch",
			"event e x decreased for 100ms\n",
			`references undeclared name "x"`},
		{"event as watch",
			"watch x byte 1\nevent e x decreased for 100ms\non e == 1: pulse 1.0 100ms\n",
			`uses event "e" where a watch is required`},
		{"def references later event",
			"watch x byte 1\ndef d e and x == 1\nevent e x decreased for 100ms\n",
			`references event "e" before its declaration`},
		{"duplicate across kinds",
			"watch x byte 1\ndef x x == 1\n",
			`line 2: duplicate name "x"`},
		{"keyword name",
			"watch level byte 1\n",
			`watch name "level" is a keyword`},
		{"clip keyword name",
			"watch clip byte 1\n",
			`watch name "clip" is a keyword`},
		{"clip on a hold rule",
			"watch x byte 1\non x == 1: hold 0.5 clip\n",
			"clip is only valid on a pulse or pattern rule"},
		{"clip on a dampen rule",
			"watch x byte 1\non x == 1: dampen 30% clip\n",
			"clip is only valid on a pulse or pattern rule"},
		{"clip with a previous-frame condition",
			"watch x byte 1\non x decreased: pulse 1.0 100ms clip\n",
			"clip is not valid with a previous-frame condition"},
		{"clip with a previous-frame condition in a compound",
			"watch x byte 1\nwatch y byte 2\non y == 1 and x changed: pulse 1.0 100ms clip 50ms\n",
			"clip is not valid with a previous-frame condition"},
		{"undeclared reference",
			"on x == 1: pulse 1.0 100ms\n",
			`references undeclared name "x"`},
		{"bare watch as condition",
			"watch x byte 1\non x: pulse 1.0 100ms\n",
			`uses watch "x" alone; a bare name must reference a def or event`},
		{"def where watch required",
			"watch x byte 1\ndef d x == 1\non d == 2: pulse 1.0 100ms\n",
			`uses def "d" where a watch is required`},
		{"watch used before declaration",
			"def d x == 1\nwatch x byte 1\n",
			`references watch "x" before its declaration`},
		{"def used before declaration",
			"watch x byte 1\non d: pulse 1.0 100ms\ndef d x == 1\n",
			`references def "d" before its declaration`},
		{"def forward reference",
			"watch x byte 1\ndef a b\ndef b x == 1\n",
			`references def "b" before its declaration`},
		{"def self reference",
			"watch x byte 1\ndef a a\n",
			`references def "a" before its declaration`},
		{"prev condition in def",
			"watch x byte 1\ndef d x changed\n",
			`previous-frame condition is not allowed in def "d"`},
		{"prev condition in while",
			"watch x byte 1\non x == 1 while x changed: pulse 1.0 100ms\n",
			"previous-frame condition is not allowed"},
		{"prev condition on hold",
			"watch x byte 1\non x changed: hold 0.5\n",
			"previous-frame condition is not allowed"},
		{"level on hold",
			"watch x byte 1\non x == 1: hold 0.5 level\n",
			"level is not valid on a hold rule"},
		{"cooldown on hold",
			"watch x byte 1\non x == 1: hold 0.5 cooldown 100ms\n",
			"cooldown is not valid on a hold rule"},
		{"change scale on hold",
			"watch x byte 1\non x == 1: hold 0.5 scale 1..2 -> 0.5..1.0\n",
			"scale without a watch is only valid on a pulse or pattern rule"},
		{"hold all zero",
			"watch x byte 1\non x == 1: hold off\n",
			"hold needs at least one nonzero motor"},
		{"intensity range",
			"watch x byte 1\non x == 1: pulse 1.5 100ms\n",
			"intensity must be within 0 to 1.0"},
		{"zero duration",
			"watch x byte 1\non x == 1: pulse 1.0 0ms\n",
			"duration must be positive"},
		{"scale needs one prev cond",
			"watch x byte 1\non x == 1: pulse 1.0 100ms scale 1..2 -> 0.5..1.0\n",
			"scale without a watch requires exactly one previous-frame condition, found 0"},
		{"scale two prev conds",
			"watch x byte 1\nwatch y byte 2\non x decreased and y decreased: pulse 1.0 100ms scale 1..2 -> 0.5..1.0\n",
			"scale without a watch requires exactly one previous-frame condition, found 2"},
		{"scale on undeclared watch",
			"watch x byte 1\non x == 1: pulse 1.0 100ms scale nope 1..2 -> 0.5..1.0\n",
			`scales on undeclared name "nope"`},
		{"scale on a const",
			"watch x byte 1\nconst k 5\non x == 1: pulse 1.0 100ms scale k 1..2 -> 0.5..1.0\n",
			`scales on "k", which is not a watch`},
		{"scale on a watch below the rule",
			"watch x byte 1\non x decreased: pulse 1.0 100ms scale y 1..2 -> 0.5..1.0\nwatch y byte 2\n",
			`scales on watch "y" before its declaration`},
		{"scale magnitude range",
			"watch x byte 1\non x decreased: pulse 1.0 100ms scale 2..2 -> 0.5..1.0\n",
			"scale magnitude range needs A < B"},
		{"scale multiplier zero",
			"watch x byte 1\non x decreased: pulse 1.0 100ms scale 1..2 -> 0..0\n",
			"scale multiplier range needs X and Y at least 0 with one of them positive"},
		{"mask fit",
			"watch v byte 1 mask 0x1FF\n",
			`watch "v" mask 0x1FF does not fit width 8`},
		{"player zero",
			"watch x byte 1\non x == 1: pulse 1.0 100ms player 0\n",
			"player must be positive"},
		{"const duplicate name",
			"watch x byte 1\nconst x 5\n",
			`duplicate name "x"`},
		{"const as watch",
			"const m 5\non m == 1: pulse 1.0 100ms\n",
			`uses const "m" where a watch is required`},
		{"const as def",
			"const m 5\non m: pulse 1.0 100ms\n",
			`uses const "m" where a def or event is required`},
		{"const keyword name",
			"const level 5\n",
			`const name "level" is a keyword`},
		{"from keyword name",
			"watch from byte 1\n",
			`watch name "from" is a keyword`},
		{"at keyword name",
			"watch at byte 1\n",
			`watch name "at" is a keyword`},
		{"least keyword name",
			"const least 5\n",
			`const name "least" is a keyword`},
		{"most keyword name",
			"watch most byte 1\n",
			`watch name "most" is a keyword`},
		{"at most zero",
			"watch x byte 1\non x decreased by at most 0: pulse 1.0 100ms\n",
			"at most bound must be positive"},
		{"by zero",
			"watch x byte 1\non x increased by 0: pulse 1.0 100ms\n",
			"an exact by delta must be positive"},
		{"signed and abs",
			"watch x byte 1 signed abs\n",
			`watch "x" cannot be both signed and abs`},
		{"signed keyword name",
			"watch signed byte 1\n",
			`watch name "signed" is a keyword`},
		{"abs keyword name",
			"const abs 5\n",
			`const name "abs" is a keyword`},
		{"signed scale range backwards",
			"watch v word 0x10 signed\non v decreased: pulse 1.0 100ms scale v -4..-20 -> 0.5..1.0\n",
			"scale magnitude range needs A < B"},
		{"unsigned scale range with negative",
			"watch x byte 1\non x decreased: pulse 1.0 100ms scale x -5..5 -> 0.5..1.0\n",
			"scale range takes a minus sign only on a signed watch"},
		{"change scale range with negative",
			"watch x byte 1\non x decreased: pulse 1.0 100ms scale -12..-6 -> 1.0..0.4\n",
			"scale range takes a minus sign only on a signed watch"},
		{"abs scale range with negative",
			"watch v word 0x10 abs\non v decreased: pulse 1.0 100ms scale v -12..-6 -> 0.5..1.0\n",
			"scale range takes a minus sign only on a signed watch"},
		{"stride keyword name",
			"watch stride byte 1\n",
			`watch name "stride" is a keyword`},
		{"array count one",
			"watch x byte 1 stride 4 count 1\n",
			`watch "x" count must be at least 2`},
		{"array stride narrow",
			"watch x word 2 stride 1 count 2\n",
			`watch "x" stride must be at least its width`},
		{"array span past 32 bits",
			"watch x byte 0xFFFFFFF0 stride 0x10 count 2\n",
			`watch "x" slots extend past the 32-bit address space`},
		{"prev cond on array",
			"watch x byte 1 stride 4 count 2\non x changed: pulse 1.0 100ms\n",
			`previous-frame condition on array watch "x"`},
		{"event trigger on array",
			"watch x byte 1 stride 4 count 2\nevent e x decreased for 100ms\n",
			`previous-frame condition on array watch "x"`},
		{"scale on array",
			"watch x byte 1 stride 4 count 2\nwatch y byte 0\non y decreased: pulse 1.0 100ms scale x 1..2 -> 0.5..1.0\n",
			`scales on array watch "x"`},
		{"dampen zero percent",
			"watch x byte 1\non x == 1: dampen 0%\n",
			"percent must be 1 to 100 on a dampen rule"},
		{"dampen over 100",
			"watch x byte 1\non x == 1: dampen 101%\n",
			"percent must be 1 to 100 on a dampen rule"},
		{"amplify zero percent",
			"watch x byte 1\non x == 1: amplify 0%\n",
			"percent must be 1 to 200 on an amplify rule"},
		{"amplify over 200",
			"watch x byte 1\non x == 1: amplify 201%\n",
			"percent must be 1 to 200 on an amplify rule"},
		{"prev condition on dampen",
			"watch x byte 1\non x changed: dampen 50%\n",
			"previous-frame condition is not allowed"},
		{"level on dampen",
			"watch x byte 1\non x == 1: dampen 50% level\n",
			"level is not valid on a dampen rule"},
		{"cooldown on amplify",
			"watch x byte 1\non x == 1: amplify 50% cooldown 100ms\n",
			"cooldown is not valid on an amplify rule"},
		{"priority on dampen",
			"watch x byte 1\non x == 1: dampen 50% priority 2\n",
			"priority is not valid on a dampen rule"},
		{"priority on amplify",
			"watch x byte 1\non x == 1: amplify 50% priority -2\n",
			"priority is not valid on an amplify rule"},
		{"change scale on dampen",
			"watch x byte 1\non x == 1: dampen 50% scale 1..2 -> 0.5..1.0\n",
			"scale without a watch is only valid on a pulse or pattern rule"},
		{"watch scale on amplify",
			"watch x byte 1\non x == 1: amplify 50% scale x 1..2 -> 0.5..1.0\n",
			"scale is only valid on a pulse, pattern, or hold rule"},
		{"dampen keyword name",
			"watch dampen byte 1\n",
			`watch name "dampen" is a keyword`},
		{"amplify keyword name",
			"const amplify 5\n",
			`const name "amplify" is a keyword`},
		{"not on watch",
			"watch x byte 1\non not x: pulse 1.0 100ms\n",
			`uses watch "x" alone; a bare name must reference a def or event`},
		{"not on const",
			"const k 5\non not k: pulse 1.0 100ms\n",
			`uses const "k" where a def or event is required`},
		{"not on undeclared name",
			"on not d: pulse 1.0 100ms\n",
			`references undeclared name "d"`},
		{"not on later def",
			"watch x byte 1\non not d: pulse 1.0 100ms\ndef d x == 1\n",
			`references def "d" before its declaration`},
		{"signed and bcd",
			"watch x byte 1 signed bcd\n",
			`watch "x" cannot combine bcd with signed or abs`},
		{"abs and bcd",
			"watch x byte 1 abs bcd\n",
			`watch "x" cannot combine bcd with signed or abs`},
		{"bcd keyword name",
			"watch bcd byte 1\n",
			`watch name "bcd" is a keyword`},
		{"comparison against undeclared name",
			"watch x byte 1\non x == nope: pulse 1.0 100ms\n",
			`references undeclared name "nope"`},
		{"comparison against later const",
			"watch x byte 1\non x == m: pulse 1.0 100ms\nconst m 5\n",
			`compares against const "m", which must be declared above its use`},
		{"watch compare with itself",
			"watch a byte 1\non a == a: pulse 1.0 100ms\n",
			`compares watch "a" with itself`},
		{"watch compare mixed interpretation",
			"watch a byte 1 signed\nwatch b byte 2\non a < b: pulse 1.0 100ms\n",
			`compares "a" and "b", which declare different interpretations`},
		{"watch compare array right",
			"watch a byte 0x10 stride 4 count 2\nwatch b byte 0\non b < a: pulse 1.0 100ms\n",
			`compares array watch "a"`},
		{"watch compare array left",
			"watch a byte 0x10 stride 4 count 2\nwatch b byte 0\non a < b: pulse 1.0 100ms\n",
			`compares array watch "a"`},
		{"watch compare against def",
			"watch a byte 1\ndef d a == 1\non a == d: pulse 1.0 100ms\n",
			`uses def "d" where a watch is required`},
		{"watch compare before declaration",
			"watch a byte 1\non a == b: pulse 1.0 100ms\nwatch b byte 2\n",
			`references watch "b" before its declaration`},
		{"held zero frames",
			"watch x byte 1\nevent e x == 1 held for 0 frames\n",
			`event "e" frame count must be at least 1`},
		{"prev cond in held event",
			"watch x byte 1\nevent e x changed held for 2 frames\n",
			`previous-frame condition is not allowed in event "e" condition`},
		{"held event self reference",
			"watch x byte 1\nevent e e held for 2 frames\n",
			`event "e" references itself`},
		{"held keyword name",
			"watch held byte 1\n",
			`watch name "held" is a keyword`},
		{"frames keyword name",
			"const frames 5\n",
			`const name "frames" is a keyword`},
		{"const scale range backwards",
			"watch x byte 1\nconst lo 5\nconst hi 10\non x decreased: pulse 1.0 100ms scale hi..lo -> 0.5..1.0\n",
			"scale magnitude range needs A < B"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.src))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestProgrammaticSpecErrors(t *testing.T) {
	pulse := Effect{Kind: EffectPulse, Strong: 0.5, Weak: 0.5, DurationMs: 100}
	watch := Watch{Name: "x", Width: 8, Address: 0x100}
	cond := func() Expr { return &CompareCond{Watch: "x", Op: OpEq, Operand: 1} }

	cases := []struct {
		name string
		rs   Ruleset
		want string
	}{
		{"missing condition",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{Effect: pulse, Player: 1}},
			},
			"rule 1 condition is missing"},
		{"empty pattern",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: cond(), Effect: Effect{Kind: EffectPattern}, Player: 1}},
			},
			"pattern has no steps"},
		{"negative cooldown",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: cond(), Effect: pulse, Player: 1, CooldownMs: -5}},
			},
			"cooldown must be non-negative"},
		{"priority above range",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: cond(), Effect: pulse, Player: 1, Priority: MaxPriority + 1}},
			},
			"priority must be -99 to 99"},
		{"priority below range",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: cond(), Effect: pulse, Player: 1, Priority: MinPriority - 1}},
			},
			"priority must be -99 to 99"},
		{"bad width",
			Ruleset{
				Watches: []Watch{{Name: "x", Width: 12, Address: 0x100}},
			},
			`watch "x" width must be 8, 16, or 32`},
		{"bad name",
			Ruleset{
				Watches: []Watch{{Name: "9bad", Width: 8, Address: 0x100}},
			},
			`watch name "9bad" is not a valid identifier`},
		{"clip duration without clip",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: cond(), Effect: pulse, Player: 1, ClipMs: 100}},
			},
			"has a clip duration without clip"},
		{"negative clip duration",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: cond(), Effect: pulse, Player: 1, Clip: true, ClipMs: -5}},
			},
			"clip duration must be non-negative"},
		{"player unset",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: cond(), Effect: pulse}},
			},
			"player must be positive"},
		{"unchanged with anchor",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: &PrevCond{Watch: "x", Op: OpUnchanged, Qual: QualFrom, Operand: 1}, Effect: pulse, Player: 1}},
			},
			"unchanged does not take a from/to anchor"},
		{"changed with by",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: &PrevCond{Watch: "x", Op: OpChanged, Qual: QualBy, Operand: 1}, Effect: pulse, Player: 1}},
			},
			"by is only valid on increased and decreased"},
		{"changed with at least",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: &PrevCond{Watch: "x", Op: OpChanged, Qual: QualAtLeast, Operand: 1}, Effect: pulse, Player: 1}},
			},
			"by is only valid on increased and decreased"},
		{"at most zero programmatic",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: &PrevCond{Watch: "x", Op: OpDecreased, Qual: QualAtMost}, Effect: pulse, Player: 1}},
			},
			"at most bound must be positive"},
		{"by zero programmatic",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: &PrevCond{Watch: "x", Op: OpIncreased, Qual: QualBy}, Effect: pulse, Player: 1}},
			},
			"an exact by delta must be positive"},
		{"player above range",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: cond(), Effect: pulse, Player: 65536}},
			},
			"player 65536 out of range"},
		{"held frames above range",
			Ruleset{
				Watches: []Watch{watch},
				Events:  []Event{{Name: "e", Held: true, Cond: cond(), HeldFrames: 1 << 31}},
			},
			"frame count 2147483648 out of range"},
		{"signed and abs programmatic",
			Ruleset{
				Watches: []Watch{{Name: "x", Width: 8, Address: 0x100, Signed: true, Abs: true}},
			},
			"cannot be both signed and abs"},
		{"signed and bcd programmatic",
			Ruleset{
				Watches: []Watch{{Name: "x", Width: 8, Address: 0x100, Signed: true, BCD: true}},
			},
			"cannot combine bcd with signed or abs"},
		{"operand without qualifier",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: &PrevCond{Watch: "x", Op: OpChanged, Operand: 1}, Effect: pulse, Player: 1}},
			},
			"has an operand without a qualifier"},
		{"unknown qualifier",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: &PrevCond{Watch: "x", Op: OpChanged, Qual: PrevQual(99)}, Effect: pulse, Player: 1}},
			},
			"has an unknown condition qualifier"},
		{"percent on pulse",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: cond(), Effect: Effect{Kind: EffectPulse, Strong: 0.5, Weak: 0.5, DurationMs: 100, Percent: 10}, Player: 1}},
			},
			"has a percent on an effect that takes none"},
		{"dampen with intensity",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: cond(), Effect: Effect{Kind: EffectDampen, Percent: 10, Strong: 0.5}, Player: 1}},
			},
			"has intensity on a dampen effect"},
		{"amplify with duration",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: cond(), Effect: Effect{Kind: EffectAmplify, Percent: 10, DurationMs: 100}, Player: 1}},
			},
			"has a duration on an amplify effect"},
		{"dampen with steps",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: cond(), Effect: Effect{Kind: EffectDampen, Percent: 10, Steps: []Step{{DurationMs: 10}}}, Player: 1}},
			},
			"has steps on a dampen effect"},
		{"nan intensity",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: cond(), Effect: Effect{Kind: EffectPulse, Strong: math.NaN(), Weak: 0.5, DurationMs: 100}, Player: 1}},
			},
			"intensity is not a number"},
		{"nan hold intensity",
			Ruleset{
				Watches: []Watch{watch},
				Rules:   []Rule{{On: cond(), Effect: Effect{Kind: EffectHold, Strong: 0.5, Weak: math.NaN()}, Player: 1}},
			},
			"intensity is not a number"},
		{"nan scale multiplier",
			Ruleset{
				Watches: []Watch{watch},
				Rules: []Rule{{On: &PrevCond{Watch: "x", Op: OpDecreased}, Effect: pulse, Player: 1,
					Scale: &Scale{MagMin: 1, MagMax: 2, MulMin: math.NaN(), MulMax: 1}}},
			},
			"scale multiplier is not a number"},
		{"infinite scale multiplier",
			Ruleset{
				Watches: []Watch{watch},
				Rules: []Rule{{On: &PrevCond{Watch: "x", Op: OpDecreased}, Effect: pulse, Player: 1,
					Scale: &Scale{MagMin: 1, MagMax: 2, MulMin: 0.5, MulMax: math.Inf(1)}}},
			},
			"scale multiplier must be finite"},
		{"held event with duration",
			Ruleset{
				Watches: []Watch{watch},
				Events:  []Event{{Name: "e", Held: true, Cond: cond(), HeldFrames: 2, DurationMs: 100}},
			},
			`event "e" has a duration on a held event`},
		{"trigger event with frame count",
			Ruleset{
				Watches: []Watch{watch},
				Events:  []Event{{Name: "e", Trigger: PrevCond{Watch: "x", Op: OpDecreased}, DurationMs: 100, HeldFrames: 2}},
			},
			`event "e" has a frame count on a trigger event`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRuleset(&tc.rs)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

// A pattern accepts both scale forms, like a pulse.
func TestPatternScaleValidates(t *testing.T) {
	src := "watch x byte 1\n" +
		"watch mag byte 2\n" +
		"on x decreased: pattern (1.0 100ms, off 50ms) scale 1..32 -> 0.5..1.0\n" +
		"on x == 1: pattern (0.8 100ms) scale mag 0..10 -> 0.2..1.0\n"
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

// A held event takes a full constant-only expression, referencing
// defs and events declared above it, and is usable and negatable like
// any event.
func TestHeldEventValidates(t *testing.T) {
	src := "watch x byte 1\n" +
		"watch y byte 2\n" +
		"def busy x == 1\n" +
		"event surge y increased for 100ms\n" +
		"event steady busy and not surge and y < 5 held for 3 frames\n" +
		"on steady: hold 0.5\n" +
		"on x == 2 while not steady: pulse 1.0 100ms\n"
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

// Watch comparisons pair same-interpretation watches of any width and
// count as constant comparisons: valid in defs, gates, and holds.
// Keyed slot and pointer watches are single-valued and allowed.
func TestWatchComparisonValidates(t *testing.T) {
	src := "watch speed   word 0x10\n" +
		"watch cap     byte 0x20\n" +
		"watch p1_vel  word 0x22 signed\n" +
		"watch p2_vel  word 0x24 signed\n" +
		"watch best    word 0x30 bcd\n" +
		"watch current word 0x32 bcd\n" +
		"watch hp      byte ptr 0x40\n" +
		"watch cap2    byte 0x28\n" +
		"def speeding speed > cap\n" +
		"on current > best while speeding: pulse 1.0 100ms\n" +
		"on p1_vel < p2_vel: pulse 1.0 100ms\n" +
		"on hp < cap2: hold 0.5\n"
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

// A negated reference is accepted anywhere a def or event reference
// is: rule conditions, gates, def bodies, and hold rules.
func TestNotValidate(t *testing.T) {
	src := "watch x byte 1\n" +
		"def wet x == 1\n" +
		"event surge x increased for 100ms\n" +
		"def dry not wet\n" +
		"on not wet while x < 5 and not surge: pulse 1.0 100ms\n" +
		"on dry and not surge: hold 0.5\n"
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

// Dampen and amplify accept defs, events, and gates like a hold, take
// the player modifier, and allow their full percent ranges.
func TestDampenAmplifyValidate(t *testing.T) {
	src := "watch x byte 1\n" +
		"def wet x == 1\n" +
		"event surge x increased for 100ms\n" +
		"on wet while x < 10: dampen 100%\n" +
		"on surge: amplify 200% player 2\n" +
		"on x == 2: dampen 1% player all\n" +
		"on x == 3: amplify 1%\n"
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

// Negative consts fold into scale ranges and order signed on a signed
// watch, like negative literals.
func TestScaleConstRangeValidates(t *testing.T) {
	src := "watch v word 0x10 signed\n" +
		"const nlo -20\n" +
		"const nhi -4\n" +
		"on v decreased: pulse 1.0 100ms scale v nlo..nhi -> 0.5..1.0\n"
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

// A signed watch orders its scale range signed, so a negative range
// in ascending signed order validates.
func TestSignedScaleRangeValidates(t *testing.T) {
	src := "watch v word 0x10 signed\n" +
		"on v decreased: pulse 1.0 100ms scale v -20..-4 -> 0.5..1.0\n" +
		"on v decreased: pulse 1.0 100ms scale v -4..20 -> 0.5..1.0\n"
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

// A high range written as hex is not a negative one: on an unsigned
// long watch the same bit patterns a minus sign would fold to are
// legitimate values.
func TestHighHexScaleRangeValidates(t *testing.T) {
	src := "watch v long 0x10\n" +
		"on v decreased: pulse 1.0 100ms scale v 0xFFFFFFF4..0xFFFFFFFA -> 0.5..1.0\n"
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

// Bounded deltas are previous-frame conditions: rule conditions and
// event triggers accept them, and the change form of scale counts them.
func TestDeltaBoundsValidate(t *testing.T) {
	src := "watch x word 0x10\n" +
		"event surge x increased by at least 5 for 100ms\n" +
		"on x decreased by at least 4 while surge: pulse 1.0 100ms\n" +
		"on x decreased by at most 50: pulse 1.0 100ms scale 4..20 -> 0.5..1.0\n"
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

// A keyed slot watch is a single value: previous-frame conditions,
// event triggers, and scale all accept it. A pointer watch likewise.
func TestKeyedAndPointerWatchValidate(t *testing.T) {
	src := "watch locks byte 0x10 stride 0x200 count 8 key 0 0x1234 field 0x173\n" +
		"watch p word ptr 0x40 offset 2\n" +
		"event burst locks increased for 100ms\n" +
		"on locks increased: pulse 1.0 100ms scale locks 1..8 -> 0.5..1.0\n" +
		"on p decreased while burst: pulse 1.0 100ms\n"
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

func TestKeyedAndPointerValidateErrors(t *testing.T) {
	pulse := Effect{Kind: EffectPulse, Strong: 0.5, Weak: 0.5, DurationMs: 100}
	cases := []struct {
		name string
		rs   Ruleset
		want string
	}{
		{"pointer with stride",
			Ruleset{
				Watches: []Watch{{Name: "x", Width: 8, Address: 0x10, Pointer: true, Stride: 8, Count: 2}},
			},
			"pointer watch, which takes no stride or count"},
		{"key without stride",
			Ruleset{
				Watches: []Watch{{Name: "x", Width: 8, Address: 0x10, HasKey: true}},
			},
			"key requires stride and count"},
		{"field ptr without key",
			Ruleset{
				Watches: []Watch{{Name: "x", Width: 8, Address: 0x10, FieldPtr: true}},
			},
			"field ptr requires a keyed slot watch"},
		{"offset without ptr",
			Ruleset{
				Watches: []Watch{{Name: "x", Width: 8, Address: 0x10, Offset: 4}},
			},
			"offset requires ptr"},
		{"key outside stride",
			Ruleset{
				Watches: []Watch{{Name: "x", Width: 8, Address: 0x10, Stride: 4, Count: 2,
					HasKey: true, KeyOffset: 2}},
			},
			"key does not fit inside the stride"},
		{"field outside stride",
			Ruleset{
				Watches: []Watch{{Name: "x", Width: 16, Address: 0x10, Stride: 8, Count: 2,
					HasKey: true, FieldOffset: 7}},
			},
			"field does not fit inside the stride"},
		{"field ptr outside stride",
			Ruleset{
				Watches: []Watch{{Name: "x", Width: 8, Address: 0x10, Stride: 8, Count: 2,
					HasKey: true, FieldOffset: 6, FieldPtr: true}},
			},
			"field does not fit inside the stride"},
		{"prev cond on plain array still rejected",
			Ruleset{
				Watches: []Watch{{Name: "x", Width: 8, Address: 0x10, Stride: 8, Count: 2}},
				Rules:   []Rule{{On: &PrevCond{Watch: "x", Op: OpIncreased}, Effect: pulse, Player: 1}},
			},
			"previous-frame condition on array watch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRuleset(&tc.rs)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}
