// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package cheats decodes cheat code strings, such as Action Replay codes,
// into memory operations. It depends only on the standard library and has
// no knowledge of the application using it.
package cheats

import "errors"

// Space is the address space of an operation's address.
type Space int

const (
	// SpaceBus addresses are native bus addresses of the system. Code
	// string cheats, such as Action Replay codes, use this space.
	SpaceBus Space = iota
	// SpaceFlat addresses are offsets into the system's flat memory
	// layout. No decoder produces this space today; the engine applies it.
	SpaceFlat
)

// OpKind is what an operation does. A compare kind gates the op after it
// in the same cheat. Only OpSet and OpIfEqual are produced by a decoder
// today; the engine applies every kind.
type OpKind int

const (
	OpSet        OpKind = iota // write Value
	OpAdd                      // add Value to the current value
	OpSub                      // subtract Value from the current value
	OpIfEqual                  // run the next op only if current == Value
	OpIfNotEqual               // run the next op only if current != Value
	OpIfGreater                // run the next op only if current > Value
	OpIfLess                   // run the next op only if current < Value
)

// Op is one decoded memory operation.
type Op struct {
	Kind  OpKind
	Space Space
	Addr  uint32
	Size  int // value width in bytes: 1, 2, or 4
	// Mask selects the bits a 1-byte op reads and writes. It is 0xFF for
	// whole bytes and for wider values.
	Mask      uint8
	Value     uint32
	BigEndian bool // byte order of 2 and 4 byte values
}

// Cheat is one decoded cheat.
type Cheat struct {
	Name string
	// Code is the code string as written, for display.
	Code string
	// ID identifies the cheat independent of its name and position: the
	// normalized code string.
	ID  string
	Ops []Op
}

// toByteOrder converts ops in place from memory stored in one byte order
// to memory stored in the other. fromBigEndian is the byte order of the
// memory the ops were written for and toBigEndian the byte order of the
// memory they are applied to. When they differ, the bytes of each 16-bit
// word are swapped: 1-byte ops flip address bit 0 and 16-bit ops reverse
// their value's byte order. 32-bit values and odd 16-bit addresses span
// two words and cannot be swapped.
func toByteOrder(ops []Op, fromBigEndian, toBigEndian bool) error {
	if fromBigEndian == toBigEndian {
		return nil
	}
	for i := range ops {
		op := &ops[i]
		switch {
		case op.Size == 1:
			op.Addr ^= 1
		case op.Size == 2 && op.Addr%2 == 0:
			op.BigEndian = !op.BigEndian
		case op.Size == 2:
			return errors.New("16-bit values at odd addresses cannot be byte swapped")
		default:
			return errors.New("32-bit values cannot be byte swapped")
		}
	}
	return nil
}
