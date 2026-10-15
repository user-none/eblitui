// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"log"

	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/cheats"
	"github.com/user-none/eblitui/desktop/gamecheats"
	"github.com/user-none/eblitui/desktop/storage"
)

// CheatEngine applies a game's enabled cheats to memory. Apply runs once
// per emulated frame, between frames, while the emulation is stopped.
type CheatEngine struct {
	ops [][]cheats.Op // ops of each enabled cheat
}

// NewCheatEngine creates an engine for the given cheats' ops.
func NewCheatEngine(list []cheats.Cheat) *CheatEngine {
	e := &CheatEngine{}
	for _, c := range list {
		e.ops = append(e.ops, c.Ops)
	}
	return e
}

// Apply runs every cheat's ops in order. A compare op that fails skips
// the next op of the same cheat.
func (e *CheatEngine) Apply(mem coreif.Memory) {
	for _, ops := range e.ops {
		skip := false
		for _, op := range ops {
			if skip {
				skip = false
				continue
			}
			switch op.Kind {
			case cheats.OpSet:
				writeCheatValue(mem, op, op.Value)
			case cheats.OpAdd:
				writeCheatValue(mem, op, readCheatValue(mem, op)+op.Value)
			case cheats.OpSub:
				writeCheatValue(mem, op, readCheatValue(mem, op)-op.Value)
			case cheats.OpIfEqual:
				skip = readCheatValue(mem, op) != maskedValue(op, op.Value)
			case cheats.OpIfNotEqual:
				skip = readCheatValue(mem, op) == maskedValue(op, op.Value)
			case cheats.OpIfGreater:
				skip = readCheatValue(mem, op) <= maskedValue(op, op.Value)
			case cheats.OpIfLess:
				skip = readCheatValue(mem, op) >= maskedValue(op, op.Value)
			}
		}
	}
}

// maskedValue limits a 1-byte op's value to its mask bits.
func maskedValue(op cheats.Op, v uint32) uint32 {
	if op.Size == 1 {
		return v & uint32(op.Mask)
	}
	return v
}

// readCheatValue reads the op's value from memory: op.Size bytes in the
// op's byte order, limited to the mask bits for 1-byte ops.
func readCheatValue(mem coreif.Memory, op cheats.Op) uint32 {
	var buf [4]byte
	b := buf[:op.Size]
	if op.Space == cheats.SpaceFlat {
		mem.ReadMemoryFlat(op.Addr, b)
	} else {
		mem.ReadMemory(op.Addr, b)
	}

	var v uint32
	for i := range b {
		if op.BigEndian {
			v = v<<8 | uint32(b[i])
		} else {
			v |= uint32(b[i]) << (8 * i)
		}
	}
	return maskedValue(op, v)
}

// writeCheatValue writes v to memory as op.Size bytes in the op's byte
// order. For a 1-byte op only the mask bits change.
func writeCheatValue(mem coreif.Memory, op cheats.Op, v uint32) {
	var buf [4]byte
	b := buf[:op.Size]
	for i := range b {
		if op.BigEndian {
			b[i] = byte(v >> (8 * (op.Size - 1 - i)))
		} else {
			b[i] = byte(v >> (8 * i))
		}
	}

	if op.Size == 1 && op.Mask != 0xFF {
		var cur [1]byte
		if op.Space == cheats.SpaceFlat {
			mem.ReadMemoryFlat(op.Addr, cur[:])
		} else {
			mem.ReadMemory(op.Addr, cur[:])
		}
		b[0] = cur[0]&^op.Mask | b[0]&op.Mask
	}

	if op.Space == cheats.SpaceFlat {
		mem.WriteMemoryFlat(op.Addr, b)
	} else {
		mem.WriteMemory(op.Addr, b)
	}
}

// loadCheatEngine builds the engine for a game's enabled cheats, custom
// and database, or returns nil when none apply. Cheats are parsed in the
// core's byte order and only those that can be applied are used.
func (gm *GameplayManager) loadCheatEngine(gameCRC string) *CheatEngine {
	if gm.cheatSystem == 0 {
		return nil
	}

	settings, err := storage.LoadGameSettings(gameCRC)
	if err != nil {
		log.Printf("Failed to load game settings for %s: %v", gameCRC, err)
		return nil
	}
	if len(settings.Cheats.Enabled) == 0 {
		return nil
	}

	set := gamecheats.Load(gameCRC, settings, gm.cheatSystem, gm.systemInfo.BigEndianMemory)
	if set.LoadErr != nil {
		log.Printf("Failed to read cheats for %s: %v", gameCRC, set.LoadErr)
	}
	enabled := set.Enabled(settings)
	if len(enabled) == 0 {
		return nil
	}
	return NewCheatEngine(enabled)
}
