# Host Integration

How an emulator hosts the rumble engine. FORMAT.md describes rumble
files from the author's side; this document describes the contract on
the program driving the engine.

## Setup

```go
rs, err := rumble.Parse(src)          // file -> ruleset, syntax errors
sys := rumble.System{
    BigEndian: true,                  // the system's native byte order
    Regions:   []rumble.Region{...},  // canonical bus regions
}
eng, err := rumble.NewEngine(rs, sys, players)
```

- Regions come from the emulator's memory map, in canonical native
  bus addresses. Every watch address in the file must land inside a
  region or NewEngine fails, so a file authored for a different
  system is rejected at load. A pointer watch's target moves at run
  time; the engine re-checks it against these regions every frame
  and treats an out-of-region target as unresolved (FORMAT.md).
- players is the number of emulated ports a `player all` rule fans
  out to.
- Parse and NewEngine errors name the offending line. Treat a file
  that fails either as a startup error; there is no partial load.
- The engine keeps references into the ruleset, so the ruleset must
  not be mutated after NewEngine returns. A host that builds rulesets
  programmatically and wants to change one builds a new ruleset and a
  new engine.
- `ReadMemory` must serve every byte asked for at an address inside a
  declared region. A short read is treated as no value for that watch
  for the frame, so a host whose memory view disagrees with the
  regions it declared gets silence rather than decoded fragments.

## Metadata

```go
md := eng.Metadata()                  // game, gameid, system, revision
```

The file's metadata block, described in FORMAT.md. The engine reads
none of it and it never affects evaluation, so a host takes it for
display or logging only. Every field is optional in the format: the
strings are empty when the file omits them and Revision is 1.

Nothing here identifies which file to load. The host resolves that
itself, and `gameid` records what the author targeted rather than
directing the lookup. A host that wants to check the two agree does
so on its own terms, since the format mandates no filename.

## Per-frame evaluation

```go
states := eng.Evaluate(mem, now)
```

Call once per emulated frame, between frames, while emulated memory
is stable. mem is the engine's view of emulated memory through the
Reader interface; reads happen only inside this call.

`now` is the clock every duration in the engine runs on: cooldowns,
event windows, and effect playback. Pass the current wall time when
the emulator runs at real time. The engine never calls a clock
itself, so time only advances between Evaluate calls.

The engine holds no goroutines and no locks. All calls (Evaluate,
Reset) belong on one goroutine, normally the emulation loop.

## Output contract

Evaluate returns the final per-player motor levels for the frame,
after rule arbitration and hold mixing. A player absent from the slice
has both motors off, and a nil slice means every player is silent.

The converse does not hold: a player can be present with both motors
at 0. A pattern's `off` step, a pulse authored `off`, and an effect
or hold muted by `dampen 100%` all produce a player entry reading
0/0. Treat the levels as the
whole answer and drive them as given - presence is not a promise that
something is vibrating.

The engine only reports levels. The host owns the hardware:

- Map player numbers (1-based emulated ports) to physical devices,
  through whatever pad-to-player assignment the host maintains.
- Drive vibration every frame from the returned levels, as authored
  with no scaling. Using a short per-command duration slightly above
  one frame keeps held levels seamless while letting the motors
  decay naturally if evaluation stops.
- Stop the hardware for a player that goes absent from the states, a
  player whose device assignment changes, and on shutdown. The
  engine never remembers what the host last sent.

## Suspending: pause, rewind, fast-forward

The engine only runs inside Evaluate, so suspension is the host not
calling it:

1. Stop calling Evaluate.
2. Silence the motors the host already started (engine output stops,
   hardware does not).
3. On resume, call Reset, then resume per-frame Evaluate calls.

Reset drops playing effects, cooldowns, edge state, event windows,
held event counts, and previous watch values, and re-arms the settle
window (30 silent frames that reseed previous values). The Reset on
resume is not optional after an operation that moves emulated memory
(rewind, save state load, a fast-forward stretch evaluated without the
engine):
every value that changed while the engine was not looking would
otherwise read as an edge on the first frame back and fire a burst
of spurious rumble.

A plain pause that does not move memory can skip the Reset: resuming
Evaluate continues from consistent previous values.

## Save state load and rewind

Call Reset immediately after the state is applied, before the next
Evaluate. Same reasoning as resume-after-suspension: the memory jump
must not read as gameplay.
