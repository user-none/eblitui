# Rumble File Format

A rumble file drives gamepad vibration from emulated game memory. It
is evaluated once per emulated frame against the running system's
memory and produces motor output. Files describe what memory to
watch, when an event has happened, and what the motors should do.

## File Structure

Files are plain text. Only ASCII is meaningful to the format itself:
a non-ASCII byte anywhere a statement is read is an error. Comment
text and metadata values are not read as statements and may contain
any UTF-8, so a game name in its own script is fine.

- A `#` starts a comment that runs to the end of the line.
- Blank lines are ignored.
- A statement starts at column 0. A line that starts with whitespace
  is a continuation of the previous statement.
- Names and keywords are case sensitive. All keywords are lower case.
- Integers are unsigned decimal (`42`) or hex (`0x2A`). Where a value
  operand appears - a comparison constant, a set member, a `from`/`to`
  anchor, a const value, or a scale magnitude range - a leading minus
  sign is allowed (`-100`, `-0x10`) and the integer is stored as its
  32-bit two's-complement pattern. A `priority` also takes a minus
  sign, and is a plain signed integer rather than a stored pattern.
  Deltas, durations, intensities, and every other integer position
  reject a minus sign.
- Intensities are decimals from `0` to `1.0` with an optional
  fractional part (`0`, `1`, `0.6`). A leading digit is required.
- Durations are an integer with an `ms` suffix (`200ms`).
- Percents are an integer with a `%` suffix (`30%`).

Names (for watches, consts, events, and defs) are
`[A-Za-z_][A-Za-z0-9_]*` and must not be keywords. The keywords are:

```
watch const def event on while byte word long mask little big
pulse pattern hold off level priority clip cooldown player scale all
and or in not by for from to changed unchanged increased decreased
stride count ptr offset key field at least most signed abs
dampen amplify bcd held frames
```

There are five statements: `watch`, `const`, `event`, `def`, and `on`
(a rule). A name must be declared before it is referenced, so watches,
consts, events, and defs appear above their first use.

The metadata block below has four keys of its own - `game`, `gameid`,
`system`, and `revision` - reserved in statement position at the head
of a file only. They are not keywords and stay available as names.

## Metadata

A file may open with a metadata block describing what it is. The block
runs from the first line to a `---` terminator alone on its own line,
and everything after the terminator is the file proper.

```
game: Bulk Slash
gameid: T-14310G
system: saturn
revision: 3
---
```

The block is documentation. The engine reads none of it, and a host or
a tool uses it to identify and compare files. There are four
statements, each written as `<key>: <value>`:

- `game` is a display name.
- `gameid` identifies the game. Disc games use the serial. Cart games
  use the ROM CRC32, since they carry no serial. The scheme is fixed
  per system, so `system` says how to read this value.
- `system` names the system the file targets.
- `revision` is a positive integer, and is 1 when the file omits it.

Every statement is optional and an empty block is allowed, so a bare
`---` is a valid metadata block. The terminator is what makes a block,
though: a file without one has no metadata, and a metadata statement
in a file that lacks a terminator is an error, as is one below the
terminator.

A statement appears at most once. The value is everything after the
first colon, trimmed, so it may contain further colons and spaces. It
is taken literally with no quoting or escaping, meaning quotes around
a value become part of it, and no value can contain `#` because a
comment starts there. A value may not be empty.

```
game: Sonic: The Hedgehog
```

Comments and blank lines are allowed inside the block. The four keys
are reserved in statement position only, so they remain legal watch,
const, def, and event names.

### Revision

A revision distinguishes two copies of the same file, telling a reader
which one is newer. Compare revisions only between files with the same
`gameid` and `system`, since the number means nothing across games.

Either of two schemes works:

- An incrementing counter: `1`, `2`, `3`.
- A date as `YYYYMMDD`: `20260730`.

Pick one and stay with it. Moving from a counter to a date works,
because any date is larger than a plausible counter. Moving from a
date back to a counter does not, and permanently breaks comparison
for that file.

## watch

```
watch <name> <width> <address> [stride <int> count <int>
      [key <int> <value> field <int> [ptr [offset <int>]]]]
      [mask <int>] [signed|abs|bcd] [little|big]
watch <name> <width> ptr <address> [offset <int>]
      [mask <int>] [signed|abs|bcd] [little|big]
```

Declares a named memory watch. Every declared watch is read once per
frame no matter how many rules use it, and the engine keeps the
previous frame's value for the previous-frame conditions.

- `<width>` is `byte`, `word`, or `long` (8, 16, or 32 bits).
- `<address>` is the canonical native system bus address. Mirror and
  CPU-partition spellings, and addresses outside a known memory
  region, are load errors.
- `stride <int> count <int>` declares an array watch over `count`
  slots read `stride` bytes apart; see below. The two appear
  together, `count` is at least 2, and on a plain array `stride` is
  at least the width, so slots do not overlap.
- `key <int> <value> field <int>` turns an array watch into a keyed
  slot watch; see below. The key value is an integer literal or a
  const name.
- `ptr` before the address declares a pointer watch, and after a
  keyed watch's field offset marks the field as a pointer; see below.
  `offset <int>` follows a `ptr` and defaults to 0.
- `mask <int>` optionally ANDs the read value. The mask must fit the
  width.
- `signed` or `abs` optionally interprets the masked value as
  width-wide two's complement, and `bcd` as packed BCD. At most one
  of the three may appear.
- `little` or `big` optionally overrides byte order for `word` and
  `long` watches. The default is the system's native byte order.

A watch is unsigned unless declared `signed` or `abs`. Masking
applies to the endian-decoded value, and all comparisons are over the
masked, decoded, interpreted result. Mask, byte order, and the signed
interpretations apply to the watched value only, never to a pointer
or a key, which are always read as longs in the system's native byte
order.

With `signed`, the masked value is read as two's complement with the
sign bit at the top of the declared width: comparisons and the
previous-frame conditions order signed, `from`/`to` anchors and set
members take negative operands, and a scale range on the watch may be
negative. A mask that clears the width's sign bit leaves values that
are never negative. With `abs`, the value is read the same way and
then replaced by its absolute value, so downstream the watch is an
unsigned magnitude - the natural form for speeds and deflections
where only size matters.

```
# Vertical velocity, negative while falling.
watch fall word 0x0602FFB0 signed
on action changed to 5 and fall <= -6 while ingame:
    pulse 0.8/0.7 150ms scale fall -12..-6 -> 1.0..0.4

# Slip angle magnitude: rumble follows how deep the slide is.
watch slip long 0x0606D174 abs
on slip >= 0x30000 while racing: hold 0.2/0.45 scale slip 0x30000..0x180000 -> 0.5..1.0
```

With `bcd`, the masked value is packed BCD: each nibble is one
decimal digit, high digit first, and the watch's value is the
decoded number - 0 to 99 for a byte, 0 to 9999 for a word, 0 to
99999999 for a long. Operands are written as ordinary integers, so
`score >= 100` matches a stored 0x0100. A nibble above 9 is not a
value: the watch is unresolved for that frame, which swallows
sentinel writes like 0xFF instead of decoding them as garbage. On an
array watch an invalid slot simply matches no condition while the
other slots read normally. The exact delta forms wrap at the digit
capacity (see Expressions), and masking before decoding lets a lone
digit share a byte with flag bits: `mask 0x0F bcd`. The capacity
follows the mask - one power of ten per nibble through the mask's
highest set nibble - so an unmasked byte wraps at 100 while that lone
digit wraps at 10, and going 9 to 0 it has increased by exactly 1.

```
# Score in packed BCD, four digits.
watch score word 0x0605E020 bcd
on score increased by 100 while ingame: pulse 0/0.4 120ms
```

```
watch health    byte 0x0605C973
watch boss_hp   word 0x0600A244
watch collision byte 0x0605D010 mask 0x01
watch rpm       word 0x06020400 little
```

### Array watches

An array watch reads one value of the declared width from each of
`count` addresses: slot `i` is read at `address + i * stride`. The
stride is the size of the game's per-object record, so the watch
plucks the same field out of every slot of an object pool. The whole
span, from the first slot through the last slot plus the width, must
lie inside one memory region. Mask and byte order apply to every slot
alike.

An array watch has no single value, so its conditions ask whether ANY
slot currently matches: only the constant comparisons (`==`, `!=`,
`<`, `>`, `<=`, `>=`, `in`, `not in`) are valid on one, everywhere an
expression appears. The engine keeps no previous values for the
slots: previous-frame conditions, event triggers, and `scale` cannot
use an array watch, and are load errors. A keyed slot watch (below)
is exempt from all of this: it selects one slot and has a single
value.

Once-per-appearance firing still works without previous values,
because edge mode lives on the rule: the `on` expression's
false-to-true transition fires, so "any slot == N" fires when N first
appears anywhere and cannot fire again until no slot holds N. A value
that moves between slots, or appears in a second slot while still
present in a first, is one appearance.

```
# Sound-command pool: 32 objects of 0x70 bytes; byte +1 of a live
# slot is the sound id.
watch sound byte 0x06058735 stride 0x70 count 32

# One tick when sound 40 starts, at most one per 90ms.
on sound == 40 while ingame: pulse 0.3/0.4 60ms cooldown 90ms
```

### Pointer watches

```
watch <name> <width> ptr <address> [offset <int>]
```

A pointer watch reads through a pointer the game maintains: each
frame the engine reads a 32-bit native bus address from `<address>`,
adds the offset, and reads the declared width there. The pointer cell
must lie inside a known memory region; the target is checked each
frame instead, since it moves at the game's whim.

The watch is unresolved on a frame when the pointer is 0, when the
pointer plus offset overflows 32 bits, or when the target plus width
does not lie inside a known region. When the pointer's value differs
from the previous frame the watch re-seeds: its previous value
becomes the new target's value, so the move to a different object
cannot register as a change.

```
# Player struct allocated at load time; health lives at +0x24 of
# wherever the pointer at 0x0601F320 points.
watch health word ptr 0x0601F320 offset 0x24

on health decreased while ingame: pulse 0.75 200ms
```

### Keyed slot watches

```
watch <name> <width> <address> stride <int> count <int>
      key <int> <value> field <int> [ptr [offset <int>]]
```

A keyed slot watch finds one slot of an object pool by content and
watches a field of that slot. Each frame the engine selects the slot
whose long at slot + key-offset equals the key value, and the watch's
value is the declared width read at slot + field-offset. Unlike a
plain array watch, a keyed slot watch has a single value: the full
condition set, event triggers, and `scale` all apply.

Selection is sticky: the engine keeps the previously selected slot as
long as its key still matches, and only when it stops matching does
it select the lowest-indexed matching slot. No matching slot leaves
the watch unresolved. When the selection moves to a different slot
the watch re-seeds, so a different object taking over the match
cannot register as a change.

With `ptr` after the field offset, the field holds a pointer instead
of the value: the engine reads a 32-bit native address at
slot + field-offset, adds the offset, and reads the declared width
there. The pointer follows the same resolution rules as a pointer
watch, including the re-seed when its value changes.

The key is always read as a long in the system's native byte order.
The key plus its 4 bytes and the field plus its read size (the width,
or 4 with `ptr`) must both fit inside the stride.

```
# 255-slot object pool; the player object's update function pointer
# at slot +0 identifies it. Watch its lock counter at +0x173.
watch locks byte 0x0604D154 stride 0x200 count 255
    key 0 0x06007856 field 0x173

on locks increased while ingame: pulse 0.3/0.45 40ms
```

### Unresolved watches

A pointer watch, a keyed slot watch, or a bcd watch reading invalid
digits can be unresolved on any frame. So can any watch whose read the
host could not serve whole, which should not happen once a file's
addresses are checked against the running system's regions at load,
but is treated as no value rather than as the bytes that did arrive.
An unresolved watch has no value: every condition naming it is false,
constant and previous-frame alike, so rules stay dormant, holds
release, and event triggers cannot fire. A `scale` reading an
unresolved watch clamps to the bottom of its magnitude range. The
watch's previous-value state clears, so on the first resolved frame
the previous value seeds to the current one and the return cannot
register as a change.

## const

```
const <name> <value>
```

Names a fixed integer, decimal or 0x hex. A const may be used wherever
a numeric literal appears as a condition operand - the right side of a
comparison, a member of an `in`/`not in` set, the amount in
`increased by` / `decreased by`, a `from`/`to` anchor - and as a
`scale` magnitude range endpoint. It exists only to name a magic
number in one place; it adds no behavior, and `x == 0xFD39` and
`x == marker` compile to the same check.

```
const level_marker 0xFD39
```

## event

```
event <name> <trigger> for <duration>
event <name> <expression> held for <int> frames
```

Names a timed condition, in one of two forms. Either form may be
used anywhere a def reference appears: in rule conditions, in
`while` gates, and in defs declared below it. Because an event has
duration, it is also a valid hold condition, and `not` negates it
like any reference.

The first form is a window. The trigger is a single previous-frame
condition on a declared watch (`unchanged` is not a trigger). The
event is true from the frame the trigger fires until the duration
elapses, and a trigger firing while the window is open restarts it.
Windows turn a momentary change into gateable state - pairing a
cause with an effect that lands some frames later, or suppressing
rules for a stretch after a transition.

```
# A hyper was started within the last 2.5 seconds.
event p2_hyper p2_meter decreased for 2500ms

on p1_hp decreased while ingame and p2_hyper:
    pulse 1.0 250ms
```

The second form is an on-delay. The expression may use only constant
comparisons, defs, and events declared above it, like a def body.
The event is true once the expression has been true for the given
number of consecutive frames, stays true while the expression keeps
holding, and goes false the frame it breaks - which also resets the
count, so the next stretch starts from zero. An unresolved watch
makes the expression false and resets the count.

The count is in frames, not milliseconds, because it measures how
long an observed state persisted, and the engine observes memory
once per emulated frame. A count of 1 is true the frame the
expression goes true. At 60 frames per second, 12 frames is roughly
200ms.

A held event filters state the game holds only briefly: transition
blips that toggle a flag for a frame or two, or contact that should
rumble only when sustained.

```
# Grinding the rail for real, not a glancing touch.
event grinding surface == 3 held for 12 frames

on grinding while ingame: hold 0.3/0.5
```

## def

```
def <name> <expression>
```

Names a reusable condition. The expression may use only constant
comparisons (no previous-frame conditions) and may reference defs
and events declared above it. A def is evaluated once per frame and
can be used anywhere an expression appears in a rule.

```
def ingame gamestate in (2, 3, 4) and (lives > 0 or gamestate == 4)
def boss   gamestate == 3 and boss_hp != 0
```

## Rules

```
on <expression> [while <expression>]: <effect> [<modifier> ...]
```

A rule binds a condition to a motor effect.

- The `on` expression is the trigger condition. It may use any
  condition, including the previous-frame conditions.
- The optional `while` expression is a gate: the rule is dormant
  while it is false. It may use only constant comparisons, defs, and
  events, like a def body.
- The effect and its modifiers follow the colon, on the same line or
  on indented continuation lines.

Rules have no names. Where order matters (arbitration below), it is
the order rules appear in the file.

## Expressions

An expression combines conditions with `and`, `or`, and parentheses.
`and` binds tighter than `or`. Every condition names a declared watch
on its left side, or references a def.

A def or event reference may be negated: `not underwater` is true
when `underwater` is false. `not` applies only to a def or event
name - a comparison is negated with its complement operator (`!=`
for `==`, `not in` for `in`, `>=` for `<`), and a compound group is
negated by naming it as a def and negating that.

Negating a def negates its body condition by condition, not its
result: `and` and `or` swap, each comparison becomes its complement,
and a condition on an array watch asks that no slot match. The
practical difference is unresolved watches - an unresolved watch
makes every condition naming it false, inside a def and its negation
alike, so `not underwater` cannot become true while the watch behind
`underwater` is unresolved. A negated event is true while its window
is closed.

```
def underwater depth > 0
on hit changed while ingame and not underwater: pulse 0.8 200ms
```

Constant comparisons (allowed everywhere):

```
<watch> == N          equal
<watch> != N          not equal
<watch> <  N          less than
<watch> >  N          greater than
<watch> <= N          at most
<watch> >= N          at least
<watch> in (N, N, ...)      value is one of the listed constants
<watch> not in (N, N, ...)  value is none of the listed constants
```

The right side of a comparison may name another watch instead of a
constant: the two current values compare each frame. Both sides must
declare the same interpretation - plain, `signed`, `abs`, or `bcd` -
so the values compare in one domain; width, mask, and byte order may
differ freely. Neither side may be a plain array watch, and a watch
never compares against itself. Either side unresolved makes the
condition false. A watch comparison reads no previous-frame state,
so it is a constant comparison and is valid everywhere they are.

```
# Fires the frame the lead changes hands.
on p1_score > p2_score while ingame: pulse 0/0.5 200ms
```

On an array watch a constant comparison is true when any slot
matches it.

Previous-frame conditions (allowed only in `on` expressions, and not
on array watches other than keyed slot watches):

```
<watch> changed                  value differs from the previous frame
<watch> unchanged                value equals the previous frame
<watch> increased                value is greater than the previous frame
<watch> decreased                value is less than the previous frame
<watch> increased by N           value is exactly the previous frame plus N
<watch> decreased by N           value is exactly the previous frame minus N
<watch> increased by at least N  value rose by N or more
<watch> decreased by at least N  value fell by N or more
<watch> increased by at most N   value rose by no more than N
<watch> decreased by at most N   value fell by no more than N
<watch> changed from N           previous frame was N, value differs from N
<watch> changed to N             value is N, previous frame was not N
<watch> increased from N         previous frame was N, value is greater
<watch> increased to N           value is N, previous frame was less
<watch> decreased from N         previous frame was N, value is less
<watch> decreased to N           value is N, previous frame was greater
```

Comparisons are unsigned over the watch's masked width; on a `signed`
watch they order signed instead, and on an `abs` watch they compare
the magnitude. `increased by` and `decreased by` compare exactly,
with wraparound arithmetic at the width - except on a `signed` or
`abs` watch, where the exact form matches the exact signed difference
and does not wrap at the width. On a `bcd` watch comparisons order
the decoded decimal values, and the exact forms wrap at the digit
capacity instead of the width: a two-digit counter going 99 to 00
has increased by exactly 1. Deltas themselves are always
non-negative; direction lives in the words `increased` and
`decreased`, and a signed watch going -2 to 2 has increased by
exactly 4. The `from` and `to` anchors compare plainly, like the bare
forms. A condition takes at most one of `by`, `from`, and `to`, a
`by` takes at most one bound, and `unchanged` takes none. Every `N`
above is an integer literal or a const name.

The `at least` and `at most` bounds do not wrap: the value must rise
or fall in the plain unsigned comparison, like the bare forms, and
the bound applies to the plain difference. A byte going 5 to 250 is a
rise of 245, never a fall. An `at most` bound is positive, since a
bound of 0 could never hold alongside the rise or fall it requires.
An exact `by` delta is positive too: a change of zero is `unchanged`,
not a rise or fall.

The anchored forms pin one side of the transition. `changed from N`
fires only on the frame the value leaves N, where a bare `changed`
gated on `== N` can never fire: on the transition frame the current
value has already left N, so the gate is false exactly when the
condition is true.

## Effects

Every rule has exactly one effect. An intensity `I` is written either
as `S/W` (strong motor / weak motor) or as a single number applied to
both motors. `off` is shorthand for `0/0`.

Intensities are perceptual motor levels from 0 to 1.0, output as
authored with no minimum floors.

The five effects do not all compete for the same thing. Each player
has three places a rule can act on, resolved separately every frame:

- The effect slot holds the one pulse or pattern the player is
  feeling. A pulse or pattern fires into it, occupies it for the
  length of the effect, and blocks the fires that do not outrank it.
- The held levels come from the one hold that wins among the player's
  active holds. A hold has no duration: it tracks its condition every
  frame.
- The output multiplier comes from the one dampen or amplify that wins
  among the player's active ones.

Each frame the player's motors are the greater of the slot's levels
and the held levels, per motor, multiplied by the multiplier and
clamped to 1.0:

```
strong = min(1.0, max(slot_strong, held_strong) * multiplier)
weak   = min(1.0, max(slot_weak,   held_weak)   * multiplier)
```

So a hold layers under whatever pulse or pattern is playing rather
than taking its place, and a pulse below the held level is not felt on
that motor while the hold is up. `priority` picks the winner inside
the effect slot and inside the held levels, never between them; the
priority modifier below and the Arbitration and Mixing section give
the full rules.

A silent effect still occupies the slot. `pulse off 200ms` reserves
the slot for 200ms at level 0, which blocks every fire that does not
outrank it - the way to say "nothing here for a moment" after an event
that should not be stepped on. It silences nothing already running: a
hold keeps layering through it, and a playing effect it does not
outrank keeps the slot instead.

### pulse

```
pulse I <duration>
```

A single vibration: both motors at the given levels for the duration.

```
pulse 0.6/0.3 200ms
pulse 0.76 200ms
```

### pattern

```
pattern (I <duration>, I <duration>, ...)
```

An ordered sequence of steps played as one effect. A playing pattern
always runs to completion; it cannot retrigger or be replaced until
it finishes. A pattern's effect length is the sum of its step
durations.

```
pattern (1.0 150ms, off 80ms, 0.6 300ms)
```

### hold

```
hold I
```

A sustain: the motors are held at the given levels while the rule's
gate passes and its condition is true, re-evaluated every frame, and
released the frame either goes false. There is no duration and no
tail.

A hold sets the player's held levels rather than firing into the
effect slot, so it layers under any pulse or pattern instead of
competing with one. Use it for a state the player is in - an engine
running, a surface being ridden - and a pulse or pattern for a thing
that happens.

The `on` condition of a hold rule must use only constant
comparisons, defs, and events: a previous-frame condition is true
for a single frame, so there is nothing to hold. An event's window
has duration, so a hold on an event holds while the window is open.
A hold accepts `player`, `priority`, which ranks it against the
player's other active holds, and `scale` in its named-watch form,
which lets the held levels track a value while the hold runs.

```
on surface == 3 while ingame: hold 0.2/0.4
```

### dampen / amplify

```
dampen <percent>
amplify <percent>
```

An output modifier: while the rule's gate passes and its condition is
true, the player's final motor output is scaled - dampen reduces it
by the percentage, amplify raises it by the percentage. `dampen 10%`
multiplies output by 0.9; `amplify 10%` multiplies by 1.1. The result
is clamped to 1.0 per motor, so amplify does nothing to a motor
already at 1.0 - rules that should feel stronger need headroom in
their authored intensities.

Like a hold, a dampen or amplify rule has no duration and no tail: it
is re-evaluated every frame and released the frame its gate or
condition goes false, and its `on` expression must use only constant
comparisons, defs, and events. The multiplier applies to everything
the player feels - pulses, patterns, and holds alike - and takes
effect mid-effect, so a pulse fired at the surface weakens the frame
the player submerges and recovers when they surface. Small
percentages may be imperceptible, and a dampened low intensity can
fall below the motor's physical start threshold and go silent.

A dampen or amplify rule takes `player` and no other modifier: there
is no fire to pace or rank, and no intensity to scale.

A percent is an integer with a `%` suffix. `dampen` takes 1% to 100%
(100% mutes the player). `amplify` takes 1% to 200%.

```
# Water dampens everything by 30%.
on depth > 0 while ingame: dampen 30%

# Armor gone: every hit lands harder.
on armor == 0 while ingame: amplify 25%
```

## Modifiers

Modifiers follow the effect in any order. Each may appear at most
once.

### level

```
level
```

Fire mode. The default (no modifier) is edge: the rule fires only on
the false-to-true transition of its `on` expression. With `level`,
the rule fires every frame the expression holds, paced by the
cooldown. Not valid on hold, dampen, or amplify rules.

### priority

```
priority <integer>
```

Ranks the rule against the others contending for the same one of the
player's three places. A rule with no `priority` has priority 0, and
the range is -99 to 99.

The rule wins where it is strictly greater: a pulse or pattern takes
the effect slot from an effect still playing only if it outranks that
effect, and the highest priority hold provides the held levels. Ties
leave what is already in place, which for two rules firing on one
frame is the earlier one in the file. Use it where an event must
always be felt, such as a death, so neither file order nor a lesser
effect already playing can swallow it.

A negative priority loses those ties without losing anything else:
the rule still fires into a free slot and still cancels nothing. Rules
that should never interrupt anything - a screen shake among damage
and death rules - belong below 0, which leaves the middle of the range
free for the rules that matter more.

```
on shake changed:      pulse 0.3 120ms priority -1
on health decreased:   pulse 0.75 200ms
on health == 0:        pattern (1.0 400ms, 0.6 500ms) priority 1
on boss_hp == 0:       pattern (1.0 500ms, 0.8 600ms) priority 2
```

Priority never reaches across the three places. A hold's priority
ranks it against the player's other holds and against nothing else,
so a hold at priority 99 neither blocks a fire nor is blocked by one -
the two layer, as the Effects section describes. Dampen and amplify
take no priority at all: they scale the mixed output rather than
contend for it, and the earliest in the file provides the multiplier.

### clip

```
clip [<duration>]
```

Bounds the effect's life by the rule's `on` expression: once the
expression has been false for the duration, the effect the rule
installed is cancelled where it stands. Without a duration the cut
lands the frame the expression goes false. Use it where the rule
expresses a state that ends at no fixed time, such as a boss death
whose pattern must not outlive the death.

The cut reaches only the effect this rule installed, and only while
that effect is still in the player's slot. An effect a higher priority
fire put there belongs to another rule and is left alone. Clips are
applied before any of the frame's fires are arbitrated, so a
clipped slot is free for any rule that fires the same frame, wherever
it sits in the file.

The gate does not clip. A gate is permission to fire, so a rule may
start on a gate that closes a moment later and keep playing, the same
way a gate going false never cuts short a playing effect. Everything
that bounds the effect belongs in the `on` expression.

The duration doubles as a debounce: an expression that goes true again
inside the window restarts it, so a single frame's dropout - an array
watch finding no match while the game shuffles its objects - does not
cut the effect. An effect that ends on its own before the window
closes is simply gone, and the clip does nothing.

A clip does not change the rule's cooldown, which still runs from the
fire and still covers the authored effect length. Valid on pulse and
pattern rules whose `on` expression carries no previous-frame
condition: such a condition is true for a single frame, leaving
nothing to play.

```
# The boss object is freed when its explosion animation ends. The
# tumbling figure stops there instead of finishing its last cycle.
on dying while ingame:
    pattern (0.85 130ms, off 90ms, 0.5/0.6 180ms) level cooldown 900ms clip
```

### cooldown

```
cooldown <duration>
```

Refractory time after a fire, in both modes. The effective cooldown
is the greater of the stated cooldown and the effect length, so an
effect always finishes before its rule can fire again. Default 0.
Not valid on hold, dampen, or amplify rules.

The cooldown belongs to the rule, not to each player it targets. A
`player all` rule whose fire some players dropped - their slots held
by something it did not outrank - still starts its cooldown, so those
players do not receive that fire late. A dropped fire is gone rather
than deferred, whether the cooldown is involved or not.

### player

```
player <n>
player all
```

Which player the effect targets. `<n>` is a positive emulated port
number. `all` targets every player. Default 1.

### scale

```
scale [<watch>] A..B -> X..Y
```

Scales the effect's intensity by a magnitude. The magnitude is clamped
to `[A, B]` and mapped linearly to a multiplier, `A` to `X` and `B` to
`Y`, applied to both motors, and the scaled intensity is clamped to
1.0 at fire time.

On a pattern the multiplier is computed once, at fire time, and
applies to every step's intensities alike: durations are untouched,
`off` steps stay off, and each step clamps to 1.0 - so a step
authored at 1.0 has no headroom and flattens against the clamp while
quieter steps keep scaling.

`A` and `B` are integers, written as literals or const names, with
`A < B`. On a `signed` watch they may be negative and order signed,
so `-12..-6` is a valid range. A minus sign is valid only there: the
change form and every other interpretation measure non-negative
magnitudes, so a negative endpoint on them is rejected. A large hex
value is not a negative one - on a plain `long` watch
`0xFFFFFFF4..0xFFFFFFFA` is an ordinary high range. A name right
after `scale` names the
magnitude watch unless `..` follows it, which makes it the range's
first endpoint. `X` and `Y` are decimals of at least 0, with at least
one of them positive. `Y` may be below `X`, mapping a larger magnitude
to a weaker multiplier - useful when the watched value runs opposite
to the intended strength.

Where the magnitude comes from depends on whether a watch is named.

Without a watch it is how much the triggering value changed this
frame. The rule must be a pulse or pattern whose `on` expression
contains exactly one previous-frame condition, and that condition's
watch supplies the magnitude. It is a single frame's change: a loss
animated over several frames fires once at the start with the first
frame's change. With an exact `by` condition the magnitude is the
stated delta itself - the condition matched exactly that change, so a
fire through a wrap scales the same as a plain one, and the
multiplier is constant.

```
on health decreased while ingame:
    pulse 0.6/0.3 200ms cooldown 100ms scale 1..32 -> 0.5..1.0
```

With a watch it is that watch's current value, read fresh each time
the rule fires. Trigger and magnitude are then separate values, so a
rule can fire on one thing and take its strength from another, and it
needs no previous-frame condition at all. Valid on a pulse, a
pattern, or a hold, and a hold re-reads the magnitude every frame the
way it re-reads its condition.

```
# Rumbles while off the track, harder the faster the car is going.
on offroad while racing:
    hold 0.8/1.0 scale speed 0x4000..0x30000 -> 0.15..1.0
```

## Evaluation Model

Once per emulated frame:

1. Every declared watch is read once. Previous value and change are
   updated.
2. Trigger events update: a firing trigger opens or restarts its
   window.
3. Defs and held events evaluate in declaration order, so each sees
   the current frame's values for everything declared above it.
4. Every rule's `on` expression is evaluated, in file order, whatever
   its gate says. A clip rule then cancels the effect it installed if
   its expression has been false for the clip duration. Both happen
   before any of the frame's fires, so a clip frees the slot for every
   rule alike.
5. Rules act, in file order. The `on` result from step 4 advances the
   rule's edge state first, so a rule tracks transitions whether or
   not it may fire. Then the gate is evaluated: a false gate makes the
   rule dormant for the frame, and otherwise the result feeds the
   rule's mode and cooldown and then its effect. Hold, dampen, and
   amplify rules set or release their levels and multipliers directly
   from the gate and condition result.

Boundary behavior:

- Values keep updating every frame even when the rules that use them
  are gated off, and edge state keeps tracking while gated, so after
  a gate reopens only a fresh transition fires.
- Pointer and keyed slot watches resolve before their values are
  read, per their sections above. An unresolved watch makes every
  condition naming it false for the frame.
- The engine seeds previous values without firing for a settle window
  of 30 frames after start. Event triggers cannot fire during the
  settle window; a held event's count runs through it, since its
  expression needs no seeded previous values. Save state load and
  rewind reset previous values, edge state, cooldowns, event windows,
  held counts, clip timing, and any playing pattern, and re-arm the
  settle window.
- Holds carry no state and recompute every frame.
- A gate going false does not cut short a playing pulse or pattern;
  it releases a hold, a dampen, or an amplify immediately. Only a
  `clip` ends a playing effect from its own rule's condition, and it
  reads that condition whether the gate passes or not.

### Arbitration and Mixing

Each player's three places - the effect slot, the held levels, and the
output multiplier - are resolved independently every frame, then
combined. Priority decides a winner within one place and is never
compared across two.

The effect slot, contended by pulse and pattern rules:

- A fire takes the slot when it is empty, when the effect in it has
  run out, or when the firing rule's priority is strictly greater than
  that of an effect still playing. Otherwise the fire is dropped.
  Equal priorities leave the playing effect in place.
- That covers both a playing effect carried over from an earlier frame
  and one installed earlier the same frame, so among rules firing for
  one player on one frame the highest priority wins, and the earliest
  in the file wins a tie.
- A playing pulse or pattern ends early only through a higher priority
  fire or a clip on the rule that installed it. A dropped fire is gone,
  not queued.
- `player all` behaves as a fire for every player, arbitrated per
  player, but the rule's cooldown is one clock shared across them.

The held levels, contended by hold rules:

- If several holds are active for one player, the highest priority one
  provides the levels and the rest are ignored, the earliest in the
  file winning a tie.
- Holds never enter the arbitration above and never occupy the effect
  slot. A hold's priority is compared only against other holds.

The output multiplier, contended by dampen and amplify rules:

- If several are active for one player, the earliest in the file
  provides the multiplier and the rest are ignored, dampen and amplify
  in one pool. They take no priority.

Mixing, once all three are resolved:

- Each motor outputs the greater of the effect slot's level and the
  held level, multiplied by the multiplier and clamped to 1.0. A hold
  therefore layers under a playing pulse or pattern, and the
  multiplier reaches everything the player feels.

## Validation

A file that fails any check is rejected whole, with a message naming
the line.

- Syntax follows the grammar below. Unknown keywords, malformed
  numbers, and malformed durations are errors.
- Names are unique across watches, consts, events, and defs, are not
  keywords, and every reference resolves to a declaration above the
  use. A const may appear only as a condition operand; used as a
  watch, event, or def it is rejected.
- A trigger event's trigger is a single previous-frame condition on a
  declared watch; `unchanged` is not a trigger. Its duration is
  positive.
- A held event's expression uses only constant comparisons, its
  frame count is at least 1 and below 2147483648, and it does not
  reference itself.
- Every address lies inside a known memory region of the running
  system, and address plus width stays inside the region. An array
  watch's whole span, first slot through last slot plus its widest
  in-slot read, stays inside one region. A pointer watch's pointer
  cell plus 4 stays inside a region; its target is checked at run
  time.
- `stride` and `count` appear together, `count` is 2 to 65535, the
  span stays inside the 32-bit address space, and on a plain array
  `stride` is at least the declared width.
- Plain array watches appear only in constant comparisons: a
  previous-frame condition, an event trigger, or a `scale` naming one
  is rejected. Keyed slot watches carry no such restriction.
- A pointer watch takes no `stride` or `count`. `key` and `field`
  appear together and only with `stride` and `count`. A field `ptr`
  appears only on a keyed slot watch, and `offset` only after a
  `ptr`. The key plus 4 and the field plus its read size fit inside
  the stride.
- `mask` fits the declared width.
- `signed`, `abs`, and `bcd` are mutually exclusive on one watch.
- Intensities are within 0 to 1.0. A hold has at least one nonzero
  motor. Durations are positive. Cooldowns are non-negative. Every
  duration, cooldown and clip delay included, is below 2147483648ms.
- A pattern has at least one step.
- A dampen takes a percent of 1 to 100, an amplify a percent of 1 to
  200, and neither takes an intensity, a duration, or steps.
- `while` expressions and def bodies use only constant comparisons.
  A hold, dampen, or amplify rule's `on` expression uses only
  constant comparisons.
- `in` and `not in` lists are non-empty.
- A comparison against a watch names two distinct single-value
  watches declaring the same interpretation.
- An exact `by` delta and an `at most` delta bound are positive.
- Modifiers appear at most once each, `level` and `cooldown` do not
  appear on hold, dampen, or amplify rules, and `priority` does not
  appear on dampen or amplify rules.
- A `priority` is an integer from -99 to 99.
- A rule targets `all` or a positive player number of 65535 or less.
- `clip` appears only on a pulse or pattern rule whose `on` expression
  contains no previous-frame condition. Its duration, when written, is
  non-negative.
- `scale` without a watch appears only on a pulse or pattern rule
  whose `on` expression contains exactly one previous-frame
  condition. `scale` with a watch appears only on a pulse, pattern,
  or hold rule, and the name resolves to a watch that is not an array
  watch and is declared above the rule.
- `scale` ranges are well formed: `A < B`, `X >= 0`, `Y >= 0`, and
  `X` or `Y` positive. A minus-signed endpoint appears only when the
  magnitude watch is declared `signed`.
- Each metadata statement appears at most once, carries a non-empty
  value, and sits inside a terminated metadata block.
- `revision` is an integer of 1 or greater.

## Grammar

```
file        = [ metablock ] { line } ;
line        = statement | comment | blank ;
statement   = watch | const | event | def | rule ;

metablock   = { metaline } "---" ;
metaline    = metakey ":" value | comment | blank ;
metakey     = "game" | "gameid" | "system" | "revision" ;
value       = { any character except "#" } ;

watch       = "watch" name width
              ( "ptr" int [ "offset" int ]
              | int [ "stride" int "count" int [ keyed ] ] )
              [ "mask" int ] [ "signed" | "abs" | "bcd" ] [ endian ] ;
keyed       = "key" int operand "field" int [ "ptr" [ "offset" int ] ] ;
width       = "byte" | "word" | "long" ;
endian      = "little" | "big" ;
const       = "const" name [ "-" ] int ;
event       = "event" name ( condition "for" duration
                           | expr "held" "for" int "frames" ) ;
def         = "def" name expr ;
rule        = "on" expr [ "while" expr ] ":" effect { modifier } ;

effect      = pulse | pattern | hold | damp ;
pulse       = "pulse" intensity duration ;
pattern     = "pattern" "(" step { "," step } ")" ;
step        = intensity duration ;
hold        = "hold" intensity ;
damp        = ( "dampen" | "amplify" ) percent ;
intensity   = number [ "/" number ] | "off" ;

modifier    = "level"
            | "priority" [ "-" ] int
            | "clip" [ duration ]
            | "cooldown" duration
            | "player" ( int | "all" )
            | "scale" [ name ] operand ".." operand "->" number ".." number ;

expr        = andexpr { "or" andexpr } ;
andexpr     = term { "and" term } ;
term        = "(" expr ")" | condition | [ "not" ] defname ;
condition   = name compareop ( operand | watchname )
            | name [ "not" ] "in" "(" operand { "," operand } ")"
            | name "changed" [ ( "from" | "to" ) operand ]
            | name "unchanged"
            | name ( "increased" | "decreased" )
                  [ "by" [ "at" ( "least" | "most" ) ] operand
                  | ( "from" | "to" ) operand ] ;
compareop   = "==" | "!=" | "<" | ">" | "<=" | ">=" ;

operand     = [ "-" ] int | constname ;
duration    = int "ms" ;
percent     = int "%" ;
int         = decimal or 0x hex, unsigned ;
number      = decimal with optional fraction ;
```

A trigger event's condition is restricted to the previous-frame forms,
without `unchanged`; a held event's expression is constant-only,
like a def body. A bare name as a term references a def or an event,
optionally negated with `not`; `not` never precedes a condition or a
parenthesized group. The minus sign an operand allows is rejected
where the operand is a delta or a bound, which are magnitudes, and on
a scale range endpoint unless the magnitude watch is declared
`signed`. The only other place a sign appears is `priority`, which is
signed throughout its range.

## Examples

Minimal file: two watches, two rules, everything defaulted (edge
mode, player 1, no gate, no cooldown).

```
game: Bulk Slash
gameid: T-14310G
system: saturn
---

# Player health. Rumbles on any loss.
watch health byte 0x0605C973
# Counts up when the player dies.
watch deaths byte 0x0605CAE3

on health decreased: pulse 0.76 200ms
on deaths increased: pulse 1.0 600ms
```

A broader file showing gates, both modes, cooldowns, scaling, a
pattern, and a hold:

```
game: Sky Runner
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
```

Compound hold conditions and banded intensity. A hold releases the
frame its condition goes false, so off-road rumble stops with the car
even while still off the track. Intensity bands come from rule order:
no rule here sets a priority, so the holds tie and the first active
one in the file provides the levels - which makes ordering them
strongest first the whole of the banding.

```
game: Rally Dash
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
```

A countdown ramping up as it runs out, three ways. Stepped bands
substitute for proportional intensity. Banded holds ramp a continuous
rumble through rule order, all of them tied at the default priority.
Banded level pulses need exclusive ranges, because overlapping level
rules would fire into each other's cooldown gaps; exclusive bands give
one clean heartbeat that quickens and strengthens as the value drops.
When the warning itself leaves a memory trace, edge rules on that
value sync every pulse to the game's own warning, with severity picked
by the same-frame tie among equal priorities, which the file resolves
by putting the most severe rule first.

```
game: Deep Diver
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
```

Per-player output modifiers. Each player's environment is watched at
its own address and scales only that player's output, so player 1
underwater and player 2 unarmored dampen and amplify independently on
the same frame. A player matching two modifier rules gets the
earliest in the file, so the combined state is written first with its
own blended value.

```
game: Tide Arena
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
```
