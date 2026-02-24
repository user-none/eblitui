//go:build !libretro

package standalone

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	emucore "github.com/user-none/eblitui/api"
)

// Minimum thresholds for rumble events. CHT files often specify low intensity
// values that are below the physical actuation threshold for gamepad motors
// via CoreHaptics. These minimums ensure any non-zero rumble is perceptible.
const (
	minRumbleMagnitude  = 0.40
	minRumbleDurationMs = 250
)

// RumbleEntry represents a single rumble definition from a CHT file.
type RumbleEntry struct {
	Address           uint32
	MemorySearchSize  int    // 0=1bit, 1=2bit, 2=4bit, 3=8bit, 4=16bit, 5=32bit
	RumbleType        int    // 0-10 (0 treated as 1/changes)
	RumbleValue       uint32 // comparison value for types 5-10
	RumblePort        int    // 0-15 specific, else all
	BigEndian         bool   // CHT entry's big_endian field
	PrimaryStrength   uint16 // 0-65535
	PrimaryDuration   int    // milliseconds
	SecondaryStrength uint16 // 0-65535
	SecondaryDuration int    // milliseconds
}

// RumbleEvent represents a rumble command to send to a gamepad.
type RumbleEvent struct {
	Port             int
	StrongMagnitude  float64
	WeakMagnitude    float64
	StrongDurationMs int
	WeakDurationMs   int
}

// ParseRumbleFile reads a CHT rumble file and returns the parsed entries.
func ParseRumbleFile(path string) ([]RumbleEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Parse all key-value pairs from the file
	kv := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, " = ", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"")
		kv[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	countStr, ok := kv["cheats"]
	if !ok {
		return nil, fmt.Errorf("missing cheats count")
	}
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return nil, fmt.Errorf("invalid cheats count: %w", err)
	}

	var entries []RumbleEntry
	for i := 0; i < count; i++ {
		prefix := fmt.Sprintf("cheat%d_", i)

		entry := RumbleEntry{
			RumblePort: 1, // default: controller 1
		}

		if v, ok := kv[prefix+"big_endian"]; ok {
			entry.BigEndian = v == "true"
		}
		if v, ok := kv[prefix+"address"]; ok {
			addr, err := strconv.ParseUint(v, 10, 32)
			if err == nil {
				entry.Address = uint32(addr)
			}
		}
		if v, ok := kv[prefix+"memory_search_size"]; ok {
			n, err := strconv.Atoi(v)
			if err == nil {
				entry.MemorySearchSize = n
			}
		}
		if v, ok := kv[prefix+"rumble_type"]; ok {
			n, err := strconv.Atoi(v)
			if err == nil {
				entry.RumbleType = n
			}
		}
		if v, ok := kv[prefix+"rumble_value"]; ok {
			n, err := strconv.ParseUint(v, 10, 32)
			if err == nil {
				entry.RumbleValue = uint32(n)
			}
		}
		if v, ok := kv[prefix+"rumble_port"]; ok {
			n, err := strconv.Atoi(v)
			if err == nil {
				entry.RumblePort = n
			}
		}
		if v, ok := kv[prefix+"rumble_primary_strength"]; ok {
			n, err := strconv.ParseUint(v, 10, 16)
			if err == nil {
				entry.PrimaryStrength = uint16(n)
			}
		}
		if v, ok := kv[prefix+"rumble_primary_duration"]; ok {
			n, err := strconv.Atoi(v)
			if err == nil {
				entry.PrimaryDuration = n
			}
		}
		if v, ok := kv[prefix+"rumble_secondary_strength"]; ok {
			n, err := strconv.ParseUint(v, 10, 16)
			if err == nil {
				entry.SecondaryStrength = uint16(n)
			}
		}
		if v, ok := kv[prefix+"rumble_secondary_duration"]; ok {
			n, err := strconv.Atoi(v)
			if err == nil {
				entry.SecondaryDuration = n
			}
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// RumbleEngine evaluates rumble entries each frame and produces rumble events.
type RumbleEngine struct {
	entries        []RumbleEntry
	prevValues     []uint32
	initialized    int // warmup frame counter
	primaryEnd     []time.Time
	secondaryEnd   []time.Time
	systemBigEndian bool // true when the core uses big-endian memory (e.g. 68K)
}

// NewRumbleEngine creates a new rumble engine from parsed entries.
// systemBigEndian should match SystemInfo.BigEndianMemory for the core.
// Byte swapping is determined per-entry by comparing the CHT entry's
// big_endian field against the system endianness.
func NewRumbleEngine(entries []RumbleEntry, systemBigEndian bool) *RumbleEngine {
	n := len(entries)
	now := time.Now()
	pEnd := make([]time.Time, n)
	sEnd := make([]time.Time, n)
	for i := range pEnd {
		pEnd[i] = now
		sEnd[i] = now
	}
	return &RumbleEngine{
		entries:         entries,
		prevValues:      make([]uint32, n),
		primaryEnd:      pEnd,
		secondaryEnd:    sEnd,
		systemBigEndian: systemBigEndian,
	}
}

// Evaluate reads memory for each entry, checks conditions, and returns rumble events.
func (re *RumbleEngine) Evaluate(mi emucore.MemoryInspector) []RumbleEvent {
	if re.initialized < 30 {
		// Warmup: read values to populate prevValues without triggering
		for i := range re.entries {
			swap := re.entries[i].BigEndian != re.systemBigEndian
			re.prevValues[i] = readMemoryValue(mi, re.entries[i].Address, re.entries[i].MemorySearchSize, swap)
		}
		re.initialized++
		return nil
	}

	now := time.Now()
	var events []RumbleEvent

	for i := range re.entries {
		e := &re.entries[i]
		swap := e.BigEndian != re.systemBigEndian
		current := readMemoryValue(mi, e.Address, e.MemorySearchSize, swap)
		prev := re.prevValues[i]
		re.prevValues[i] = current

		if !evaluateCondition(e.RumbleType, current, prev, e.RumbleValue) {
			continue
		}

		// Check if primary motor timer has expired
		firePrimary := e.PrimaryStrength > 0 && e.PrimaryDuration > 0 && now.After(re.primaryEnd[i])
		fireSecondary := e.SecondaryStrength > 0 && e.SecondaryDuration > 0 && now.After(re.secondaryEnd[i])

		if !firePrimary && !fireSecondary {
			continue
		}

		event := RumbleEvent{
			Port: e.RumblePort,
		}

		if firePrimary {
			event.StrongMagnitude = float64(e.PrimaryStrength) / 65535.0
			event.StrongDurationMs = e.PrimaryDuration
			re.primaryEnd[i] = now.Add(time.Duration(e.PrimaryDuration) * time.Millisecond)
		}
		if fireSecondary {
			event.WeakMagnitude = float64(e.SecondaryStrength) / 65535.0
			event.WeakDurationMs = e.SecondaryDuration
			re.secondaryEnd[i] = now.Add(time.Duration(e.SecondaryDuration) * time.Millisecond)
		}

		events = append(events, event)
	}

	return events
}

// Reset clears engine state (for save state loads or rewind).
func (re *RumbleEngine) Reset() {
	re.initialized = 0
	now := time.Now()
	for i := range re.prevValues {
		re.prevValues[i] = 0
		re.primaryEnd[i] = now
		re.secondaryEnd[i] = now
	}
}

// evaluateCondition checks if a rumble condition is met.
func evaluateCondition(rumbleType int, current, prev, rumbleValue uint32) bool {
	switch rumbleType {
	case 0, 1: // changes
		return current != prev
	case 2: // does not change
		return current == prev
	case 3: // increases
		return current > prev
	case 4: // decreases
		return current < prev
	case 5: // equals value
		return current == rumbleValue
	case 6: // not equals value
		return current != rumbleValue
	case 7: // less than value
		return current < rumbleValue
	case 8: // greater than value
		return current > rumbleValue
	case 9: // increased by value
		return current == prev+rumbleValue
	case 10: // decreased by value
		return current == prev-rumbleValue
	default:
		return false
	}
}

// readMemoryValue reads a value from memory at the given address with the
// appropriate width based on MemorySearchSize.
// byteSwap is true when the CHT entry's endianness differs from the system's,
// requiring address and byte order adjustments.
func readMemoryValue(mi emucore.MemoryInspector, addr uint32, searchSize int, byteSwap bool) uint32 {
	var buf [4]byte

	switch searchSize {
	case 0, 1, 2, 3: // 1bit, 2bit, 4bit, 8bit - read 1 byte
		readAddr := addr
		if byteSwap {
			readAddr ^= 1
		}
		mi.ReadMemory(readAddr, buf[:1])
		val := uint32(buf[0])
		switch searchSize {
		case 0: // 1-bit
			return val & 1
		case 1: // 2-bit
			return val & 3
		case 2: // 4-bit
			return val & 0x0F
		default: // 8-bit
			return val
		}
	case 4: // 16-bit - read 2 bytes
		mi.ReadMemory(addr, buf[:2])
		if byteSwap {
			return uint32(buf[0])<<8 | uint32(buf[1])
		}
		return uint32(buf[0]) | uint32(buf[1])<<8
	case 5: // 32-bit - read 4 bytes
		mi.ReadMemory(addr, buf[:4])
		if byteSwap {
			return uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
		}
		return uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
	default:
		readAddr := addr
		if byteSwap {
			readAddr ^= 1
		}
		mi.ReadMemory(readAddr, buf[:1])
		return uint32(buf[0])
	}
}

// FireRumbleEvents sends rumble events to gamepads via Ebiten.
// Levels 1-3 scale CHT intensity and duration by that multiplier.
// Level 4 scales intensity by 4x but caps duration at 2x.
// Level 5 (Max) uses maximum intensity with 2x duration.
// Minimum thresholds ensure any non-zero rumble is perceptible.
func FireRumbleEvents(events []RumbleEvent, level int) {
	if len(events) == 0 || level <= 0 {
		return
	}

	gamepadIDs := ebiten.AppendGamepadIDs(nil)
	if len(gamepadIDs) == 0 {
		return
	}

	maxMode := level == 5
	mult := float64(level)
	durationMult := level
	if level >= 4 {
		durationMult = 2
	}

	for _, ev := range events {
		// Determine which gamepads to vibrate
		var targets []ebiten.GamepadID
		if ev.Port >= 0 && ev.Port < 16 && ev.Port < len(gamepadIDs) {
			targets = []ebiten.GamepadID{gamepadIDs[ev.Port]}
		} else {
			targets = gamepadIDs
		}

		var strong, weak float64
		if maxMode {
			// Max: full intensity if the CHT entry has any strength at all
			if ev.StrongMagnitude > 0 {
				strong = 1.0
			}
			if ev.WeakMagnitude > 0 {
				weak = 1.0
			}
		} else {
			// Scale by multiplier, clamp to 1.0, apply minimum floor
			strong = ev.StrongMagnitude * mult
			if strong > 1.0 {
				strong = 1.0
			} else if strong > 0 && strong < minRumbleMagnitude {
				strong = minRumbleMagnitude
			}
			weak = ev.WeakMagnitude * mult
			if weak > 1.0 {
				weak = 1.0
			} else if weak > 0 && weak < minRumbleMagnitude {
				weak = minRumbleMagnitude
			}
		}

		durationMs := ev.StrongDurationMs
		if ev.WeakDurationMs > durationMs {
			durationMs = ev.WeakDurationMs
		}
		durationMs *= durationMult
		if durationMs < minRumbleDurationMs {
			durationMs = minRumbleDurationMs
		}

		for _, gpID := range targets {
			ebiten.VibrateGamepad(gpID, &ebiten.VibrateGamepadOptions{
				Duration:        time.Duration(durationMs) * time.Millisecond,
				StrongMagnitude: strong,
				WeakMagnitude:   weak,
			})
		}
	}
}
