// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package rumble

// Ruleset is the structured form of a rumble file: the metadata block,
// the declared watches, consts, events, and defs, and the rules in file
// order.
type Ruleset struct {
	Metadata Metadata
	Watches  []Watch
	Consts   []Const
	Events   []Event
	Defs     []Def
	Rules    []Rule
}

// Metadata is a rumble file's documentation block, the statements above
// the "---" terminator. Every field is optional and the engine reads
// none of them: they describe the file for a host or a tool. Revision
// is 1 when omitted.
type Metadata struct {
	Game     string
	GameID   string
	System   string
	Revision int
}

// Endian selects the byte order used to decode a multi-byte value.
type Endian int

const (
	// EndianDefault decodes with the system's native byte order.
	EndianDefault Endian = iota
	EndianLittle
	EndianBig
)

// Watch declares a named memory watch. Address is the canonical
// native bus address. A Count of 0 declares a single value. A Count
// of 2 or more declares an array watch reading Count slots spaced
// Stride bytes apart, whose conditions test whether any slot matches.
// Pointer declares a pointer watch and HasKey a keyed slot watch;
// both have a single value and can be unresolved (see FORMAT.md).
type Watch struct {
	Name    string
	Width   int // 8, 16, or 32 bits
	Address uint32
	Pointer bool   // Address holds a pointer to the value
	Offset  uint32 // added to a pointer's target; needs Pointer or FieldPtr
	Stride  uint32 // bytes between slots; 0 on a single-value watch
	Count   int    // number of slots; 0 on a single-value watch

	HasKey      bool   // keyed slot watch; needs Stride and Count
	KeyOffset   uint32 // slot offset of the key long
	KeyValue    uint32 // value selecting the slot
	FieldOffset uint32 // slot offset of the watched field
	FieldPtr    bool   // the field holds a pointer to the value

	// Signed, Abs, and BCD interpret the masked value as two's
	// complement, its absolute value, or packed BCD (see FORMAT.md).
	// At most one may be set, and like Mask and Endian they apply to
	// the watched value only, never to a pointer or a key. Condition
	// operands stay uint32 bit patterns.
	Signed bool
	Abs    bool
	BCD    bool

	Mask    uint32
	HasMask bool
	Endian  Endian
	Line    int
}

// fieldSize returns the number of bytes a keyed slot watch reads at
// its field offset: the width, or a 4-byte pointer with FieldPtr.
func (v *Watch) fieldSize() uint64 {
	if v.FieldPtr {
		return 4
	}
	return uint64(v.Width / 8)
}

// slotSpan returns the number of bytes the watch reads at its address
// (a single value or a pointer cell) or inside each slot (the slot
// value, or the furthest of a keyed slot watch's key and field reads).
func (v *Watch) slotSpan() uint64 {
	if v.Pointer {
		return 4
	}
	if v.HasKey {
		key := uint64(v.KeyOffset) + 4
		field := uint64(v.FieldOffset) + v.fieldSize()
		if key > field {
			return key
		}
		return field
	}
	return uint64(v.Width / 8)
}

// Const names a fixed integer usable wherever a numeric literal
// appears as a condition operand.
type Const struct {
	Name  string
	Value uint32
	Line  int
}

// Def names a reusable constant-comparison expression.
type Def struct {
	Name string
	Expr Expr
	Line int
}

// Event names a timed condition, in one of two forms: a trigger event
// opens a window of DurationMs when Trigger fires, and a held event
// (Held set) is true once Cond has held for HeldFrames consecutive
// frames. Each form uses only its own fields.
type Event struct {
	Name       string
	Trigger    PrevCond
	DurationMs int
	Held       bool
	Cond       Expr
	HeldFrames int
	Line       int
}

// EffectKind identifies which effect a rule carries.
type EffectKind int

const (
	EffectPulse EffectKind = iota
	EffectPattern
	EffectHold
	EffectDampen
	EffectAmplify
)

// Step is one step of a pattern effect.
type Step struct {
	Strong     float64
	Weak       float64
	DurationMs int
}

// Effect is a rule's effect. Strong, Weak, and DurationMs describe a
// pulse or hold (DurationMs unused for hold). Steps describes a pattern.
// A dampen or amplify effect uses only Percent.
type Effect struct {
	Kind       EffectKind
	Strong     float64
	Weak       float64
	DurationMs int
	Steps      []Step
	Percent    int
}

// Scale maps a magnitude clamped to [MagMin, MagMax] linearly onto an
// intensity multiplier in [MulMin, MulMax]. Watch selects where the
// magnitude comes from. Empty is the frame's change in the rule's one
// previous-frame condition, and a name is that watch's current value.
//
// MagMinNeg and MagMaxNeg record an endpoint written with a minus
// sign, which folding otherwise erases; validation allows one only
// on a signed magnitude watch.
type Scale struct {
	Watch     string
	MagMin    uint32
	MagMax    uint32
	MagMinNeg bool
	MagMaxNeg bool
	MulMin    float64
	MulMax    float64
}

// The range a rule's Priority may take; values outside it are
// reserved.
const (
	MinPriority = -99
	MaxPriority = 99
)

// Rule binds a condition to an effect with its modifiers. Clip bounds
// the effect's life by On. Once On has been false for ClipMs the
// installed effect is cancelled. Priority ranks the rule inside its
// pool, strictly greater winning.
type Rule struct {
	On         Expr
	While      Expr // nil when the rule has no gate
	Effect     Effect
	Level      bool
	Priority   int
	Clip       bool
	ClipMs     int
	CooldownMs int
	Player     int // 1-based; ignored when PlayerAll is set
	PlayerAll  bool
	Scale      *Scale
	Line       int
}

// Expr is a node of a condition expression.
type Expr interface{ isExpr() }

// BoolOp is a boolean connective.
type BoolOp int

const (
	OpAnd BoolOp = iota
	OpOr
)

// BinaryExpr combines two subexpressions with and/or.
type BinaryExpr struct {
	Op    BoolOp
	Left  Expr
	Right Expr
}

// CompareOp is a constant comparison operator.
type CompareOp int

const (
	OpEq CompareOp = iota
	OpNe
	OpLt
	OpGt
	OpLe
	OpGe
)

// CompareCond compares a value against a constant.
type CompareCond struct {
	Watch   string
	Op      CompareOp
	Operand uint32
	Line    int
}

// CompareWatchCond compares two watches' current values each frame.
// Both sides must declare the same interpretation so the values
// compare in one domain. Either side unresolved makes the condition
// false.
type CompareWatchCond struct {
	Left  string
	Op    CompareOp
	Right string
	Line  int
}

// SetCond tests a value for membership in a constant set.
type SetCond struct {
	Watch  string
	Negate bool
	Set    []uint32
	Line   int
}

// PrevOp is a previous-frame condition operator.
type PrevOp int

const (
	OpChanged PrevOp = iota
	OpUnchanged
	OpIncreased
	OpDecreased
)

// PrevQual qualifies a previous-frame condition with an operand. A
// condition takes at most one qualifier, and unchanged takes none.
type PrevQual int

const (
	QualNone PrevQual = iota
	QualBy
	QualAtLeast
	QualAtMost
	QualFrom
	QualTo
)

// PrevCond compares a value against its previous-frame value. Operand
// is the qualifier's operand and is meaningful only when Qual is not
// QualNone.
type PrevCond struct {
	Watch   string
	Op      PrevOp
	Qual    PrevQual
	Operand uint32
	Line    int
}

// DefRef references a def or an event by name. Negate inverts the
// reference. A negated def evaluates as its body with the negation
// pushed to the leaves, so an unresolved watch makes the negated
// conditions false too, and a negated event is true while its window
// is closed.
type DefRef struct {
	Name   string
	Negate bool
	Line   int
}

func (*BinaryExpr) isExpr()       {}
func (*CompareCond) isExpr()      {}
func (*CompareWatchCond) isExpr() {}
func (*SetCond) isExpr()          {}
func (*PrevCond) isExpr()         {}
func (*DefRef) isExpr()           {}
