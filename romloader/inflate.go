// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package romloader

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math/bits"
	"sync"
)

// Deflate stream decoding (RFC 1951). The decoder differs from a plain
// streaming one in what the zip disc backend depends on: between any two
// runs of output it can describe its position, and it can start decoding at
// such a position when given the 32 KiB of output that preceded it.

const (
	// inflateWindow is the farthest back a match can reach (RFC 1951 2).
	inflateWindow = 32768
	// inflateMaxMatch is the longest match (RFC 1951 3.2.5).
	inflateMaxMatch = 258
	// inflateWordCopyMax is the longest match the fast path copies a word
	// at a time. Longer ones go through the general copy.
	inflateWordCopyMax = 32
	// inflateCopySlack is how far past the end of a match a word copy may
	// write: 7 bytes, when the last word copied holds 1 byte of the match.
	inflateCopySlack = 7
	// inflateFastRoom is the output room one pass of the fast path needs:
	// the literals before a match, the match, and the word copy slack.
	inflateFastRoom = litPerFill + inflateMaxMatch + inflateCopySlack
	// inflateRunMax is the most output one call to next returns once a
	// whole window of output precedes the run. Before that, at the start
	// of a stream, a run can fill the whole buffer, up to inflateBufSize.
	inflateRunMax = 65536
	// inflateBufSize is the output buffer: one window of history plus room
	// for new output.
	inflateBufSize = inflateWindow + inflateRunMax
	// inflateInSize is the compressed input buffer.
	inflateInSize = 65536

	// Primary lookup widths of the code tables. Codes longer than the
	// primary width resolve through a second lookup.
	litPrimaryBits  = 11
	distPrimaryBits = 9
	clPrimaryBits   = 7

	// litPerFill is how many literals whose codes resolve in the primary
	// table can be decoded from one top up of the bit buffer, which leaves
	// at least 56 bits.
	litPerFill = 56 / litPrimaryBits

	// maxPrimaryBits is the widest primary lookup of any table.
	maxPrimaryBits = max(litPrimaryBits, distPrimaryBits, clPrimaryBits)

	// The most codes a dynamic block may define (RFC 1951 3.2.7).
	maxLitCodes  = 286
	maxDistCodes = 30
	// The fixed codes cover two literal/length and two distance symbols
	// that never occur in data (RFC 1951 3.2.6).
	fixedLitCodes  = 288
	fixedDistCodes = 32
	maxCodeLen     = 15
)

var (
	errInflateTruncated = errors.New("deflate: stream ends before the final block")
	errInflateCode      = errors.New("deflate: invalid code")
	errInflateTable     = errors.New("deflate: invalid code lengths")
	errInflateBlock     = errors.New("deflate: invalid block header")
	errInflateDistance  = errors.New("deflate: match reaches before the start of the data")
)

// Length and distance code bases and extra bit counts (RFC 1951 3.2.5).
var (
	lenBase = [29]uint16{
		3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 15, 17, 19, 23, 27, 31,
		35, 43, 51, 59, 67, 83, 99, 115, 131, 163, 195, 227, 258,
	}
	lenExtra = [29]uint8{
		0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2,
		3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0,
	}
	distBase = [30]uint16{
		1, 2, 3, 4, 5, 7, 9, 13, 17, 25, 33, 49, 65, 97, 129, 193,
		257, 385, 513, 769, 1025, 1537, 2049, 3073, 4097, 6145,
		8193, 12289, 16385, 24577,
	}
	distExtra = [30]uint8{
		0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6,
		7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13,
	}
	// clOrder is the order code length code lengths are stored in
	// (RFC 1951 3.2.7).
	clOrder = [19]uint8{16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15}
)

const (
	// huffLink marks a primary table entry that points at a second-level
	// table.
	huffLink = 0x80
	// huffLiteral marks an entry of a literal/length table whose symbol is
	// a literal byte, so the decode loop can tell with one test.
	huffLiteral = 1 << 31
	// huffSymbol masks the symbol of an entry once shifted down by 8.
	huffSymbol = 0xffff
)

// huffTable decodes one prefix code. tab starts with 1<<bits primary entries
// indexed by the next input bits. An entry is symbol<<8 | code length, or 0
// when no code maps there. For codes longer than the primary width the
// primary entry is offset<<8 | huffLink | n: the symbol's entry is at
// tab[offset + the next n bits].
type huffTable struct {
	tab  []uint32
	bits uint
}

// build fills the table from the code length of each symbol, 0 for an
// unused one. lengths holds at most fixedLitCodes lengths, each at most
// maxCodeLen, and primary is at most maxPrimaryBits. Lengths that describe
// more codes than fit are rejected. An incomplete set is accepted, and input
// that selects an unassigned code fails when decoded. literals marks symbols
// below 256 with huffLiteral.
func (h *huffTable) build(lengths []uint8, primary uint, literals bool) error {
	var count [maxCodeLen + 1]int
	for _, l := range lengths {
		count[l]++
	}
	count[0] = 0
	left := 1
	for l := 1; l <= maxCodeLen; l++ {
		left <<= 1
		left -= count[l]
		if left < 0 {
			return errInflateTable
		}
	}

	// First code of each length (RFC 1951 3.2.2).
	var next [maxCodeLen + 1]uint32
	code := uint32(0)
	for l := 1; l <= maxCodeLen; l++ {
		code = (code + uint32(count[l-1])) << 1
		next[l] = code
	}

	size := uint32(1) << primary
	if cap(h.tab) < int(size) {
		h.tab = make([]uint32, size, 2*size)
	}
	h.tab = h.tab[:size]
	clear(h.tab)
	h.bits = primary

	// Codes are sent most significant bit first while the bit reader
	// delivers the first bit lowest, so each code is stored bit-reversed.
	// For every primary slot shared by long codes, record how many more
	// bits its longest code needs.
	var codes [fixedLitCodes]uint32
	var subBits [1 << maxPrimaryBits]uint8
	for sym, l := range lengths {
		if l == 0 {
			continue
		}
		c := uint32(bits.Reverse16(uint16(next[l])) >> (16 - l))
		next[l]++
		codes[sym] = c
		if uint(l) > primary {
			low := c & (size - 1)
			if extra := l - uint8(primary); extra > subBits[low] {
				subBits[low] = extra
			}
		}
	}
	for low := uint32(0); low < size; low++ {
		extra := subBits[low]
		if extra == 0 {
			continue
		}
		off := len(h.tab)
		end := off + 1<<extra
		if end > cap(h.tab) {
			grown := make([]uint32, off, 2*end)
			copy(grown, h.tab)
			h.tab = grown
		}
		h.tab = h.tab[:end]
		clear(h.tab[off:end])
		h.tab[low] = uint32(off)<<8 | huffLink | uint32(extra)
	}

	for sym, l := range lengths {
		if l == 0 {
			continue
		}
		c := codes[sym]
		e := uint32(sym)<<8 | uint32(l)
		if literals && sym < 256 {
			e |= huffLiteral
		}
		if uint(l) <= primary {
			for i := c; i < size; i += 1 << l {
				h.tab[i] = e
			}
			continue
		}
		link := h.tab[c&(size-1)]
		off := link >> 8
		span := uint32(1) << (link & 0x0f)
		for i := c >> primary; i < span; i += 1 << (uint(l) - primary) {
			h.tab[off+i] = e
		}
	}
	return nil
}

// Fixed code tables (RFC 1951 3.2.6), built on first use.
var (
	fixedOnce sync.Once
	fixedLit  huffTable
	fixedDist huffTable
)

func buildFixedTables() {
	var lit [fixedLitCodes]uint8
	for i := range lit {
		switch {
		case i < 144:
			lit[i] = 8
		case i < 256:
			lit[i] = 9
		case i < 280:
			lit[i] = 7
		default:
			lit[i] = 8
		}
	}
	var dist [fixedDistCodes]uint8
	for i := range dist {
		dist[i] = 5
	}
	// Both sets of lengths are complete codes, so build cannot fail.
	_ = fixedLit.build(lit[:], litPrimaryBits, true)
	_ = fixedDist.build(dist[:], distPrimaryBits, false)
}

const (
	inflateHeader = iota // the next bits are a block header
	inflateStored        // inside a stored block
	inflateCoded         // inside a fixed or dynamic block
)

// inflater decodes one deflate stream held in a section of an io.ReaderAt.
// It is reused across streams with reset.
type inflater struct {
	src     io.ReaderAt
	srcOff  int64 // offset of the stream in src
	srcLen  int64 // compressed length of the stream
	readErr error // first error from src other than end of data

	in     []byte
	ip, n  int
	inBase int64 // stream offset of in[0]

	bits  uint64 // unconsumed input bits, next bit lowest
	nbits uint

	// buf[:pos] is output: the bytes still reachable by a match followed
	// by the output of the current run.
	buf []byte
	pos int

	mode       int
	final      bool // the current block is the last
	done       bool // the final block has ended
	storedLeft int  // bytes left in the current stored block

	lit, dist       *huffTable
	dynLit, dynDist huffTable
	clTab           huffTable

	// The code lengths of the current dynamic block, literal/length codes
	// then distance codes. nlit is 0 outside a dynamic block.
	lengths     [maxLitCodes + maxDistCodes]uint8
	nlit, ndist int
}

// inflatePoint is a position between two symbols of a stream, with what
// decoding needs to resume there apart from the output that preceded it. The
// zero value is the start of a stream.
type inflatePoint struct {
	bit     int64   // bit offset of the next unconsumed bit
	mode    int     // inflateHeader, inflateStored or inflateCoded
	final   bool    // the block the position is in is the last
	stored  int     // inside a stored block: bytes left in it
	nlit    int     // inside a dynamic block: literal/length code count
	lengths []uint8 // inside a dynamic block: its code lengths
}

// point returns the decoder's position. It is meaningful between calls to
// next, before the stream has ended.
func (d *inflater) point() inflatePoint {
	p := inflatePoint{
		bit:    (d.inBase+int64(d.ip))*8 - int64(d.nbits),
		mode:   d.mode,
		final:  d.final,
		stored: d.storedLeft,
	}
	if d.mode == inflateCoded && d.nlit != 0 {
		p.nlit = d.nlit
		p.lengths = bytes.Clone(d.lengths[:d.nlit+d.ndist])
	}
	return p
}

// reset points the decoder at a stream and at a position in it. window is
// the output that preceded the position, up to inflateWindow bytes, and is
// empty at the start of the stream.
func (d *inflater) reset(src io.ReaderAt, off, length int64, p inflatePoint, window []byte) error {
	if d.buf == nil {
		d.buf = make([]byte, inflateBufSize)
		d.in = make([]byte, inflateInSize)
	}
	d.src, d.srcOff, d.srcLen = src, off, length
	d.readErr = nil
	d.ip, d.n = 0, 0
	d.inBase = p.bit / 8
	d.bits, d.nbits = 0, 0
	d.pos = copy(d.buf, window)
	d.mode, d.final, d.storedLeft = p.mode, p.final, p.stored
	d.done = false
	d.nlit, d.ndist = 0, 0
	// The position may be part way into a byte.
	if _, err := d.take(uint(p.bit % 8)); err != nil {
		return err
	}

	if p.mode != inflateCoded {
		return nil
	}
	if p.lengths == nil {
		d.useFixed()
		return nil
	}
	d.nlit, d.ndist = p.nlit, len(p.lengths)-p.nlit
	copy(d.lengths[:], p.lengths)
	return d.buildDynamic()
}

// loadInput replaces the consumed input buffer with the next bytes of the
// stream. It reports false when there are none.
func (d *inflater) loadInput() bool {
	d.inBase += int64(d.n)
	d.ip, d.n = 0, 0
	remain := d.srcLen - d.inBase
	if remain <= 0 {
		return false
	}
	want := int64(len(d.in))
	if remain < want {
		want = remain
	}
	n, err := d.src.ReadAt(d.in[:want], d.srcOff+d.inBase)
	if n == 0 {
		if err != nil && err != io.EOF && d.readErr == nil {
			d.readErr = err
		}
		return false
	}
	d.n = n
	return true
}

// truncated returns the error for input that ended early: the read error
// that caused it, if there was one.
func (d *inflater) truncated() error {
	if d.readErr != nil {
		return d.readErr
	}
	return errInflateTruncated
}

// fillBits tops up the bit buffer with whole bytes while input remains. It
// stops below 64 bits, which codedFast relies on.
func (d *inflater) fillBits() {
	for d.nbits < 56 {
		if d.ip == d.n && !d.loadInput() {
			return
		}
		d.bits |= uint64(d.in[d.ip]) << d.nbits
		d.ip++
		d.nbits += 8
	}
}

// take consumes and returns the next n bits, n at most 32.
func (d *inflater) take(n uint) (uint32, error) {
	if d.nbits < n {
		d.fillBits()
		if d.nbits < n {
			return 0, d.truncated()
		}
	}
	v := uint32(d.bits) & (uint32(1)<<n - 1)
	d.bits >>= n
	d.nbits -= n
	return v, nil
}

// symbol decodes the next symbol of the given code.
func (d *inflater) symbol(h *huffTable) (uint32, error) {
	if d.nbits < maxCodeLen {
		d.fillBits()
	}
	e := h.tab[uint32(d.bits)&(uint32(1)<<h.bits-1)]
	if e&huffLink != 0 {
		idx := uint32(d.bits>>h.bits) & (uint32(1)<<(e&0x0f) - 1)
		e = h.tab[e>>8+idx]
	}
	n := uint(e & 0xff)
	if n == 0 {
		return 0, errInflateCode
	}
	if n > d.nbits {
		return 0, d.truncated()
	}
	d.bits >>= n
	d.nbits -= n
	return e >> 8 & huffSymbol, nil
}

// window returns the most recent output a later match can reach. It is valid
// until the next call to next.
func (d *inflater) window() []byte {
	start := d.pos - inflateWindow
	if start < 0 {
		start = 0
	}
	return d.buf[start:d.pos]
}

// next decodes and returns the next run of output. A run ends at the end of
// a block or when the output buffer has to be recycled, so it may be empty.
// The slice is valid until the following call. It returns io.EOF once the
// final block has ended.
func (d *inflater) next() ([]byte, error) {
	if d.done {
		return nil, io.EOF
	}
	// Keep one window of history at the front once the room for new
	// output runs low.
	if d.pos > len(d.buf)-inflateWindow {
		copy(d.buf, d.buf[d.pos-inflateWindow:d.pos])
		d.pos = inflateWindow
	}
	start := d.pos

	if d.mode == inflateHeader {
		if err := d.header(); err != nil {
			return nil, err
		}
	}
	var ended bool
	var err error
	if d.mode == inflateStored {
		ended, err = d.stored()
	} else {
		ended, err = d.coded()
	}
	if err != nil {
		return nil, err
	}
	if ended {
		d.mode = inflateHeader
		d.done = d.final
	}
	return d.buf[start:d.pos], nil
}

// header reads a block header (RFC 1951 3.2.3) and prepares the block.
func (d *inflater) header() error {
	v, err := d.take(3)
	if err != nil {
		return err
	}
	d.final = v&1 != 0
	switch v >> 1 {
	case 0:
		// A stored block starts at the next byte boundary with its
		// length and the complement of its length (RFC 1951 3.2.4).
		pad := d.nbits % 8
		d.bits >>= pad
		d.nbits -= pad
		lens, err := d.take(32)
		if err != nil {
			return err
		}
		if lens&0xffff != ^(lens>>16)&0xffff {
			return errInflateBlock
		}
		d.storedLeft = int(lens & 0xffff)
		d.mode = inflateStored
	case 1:
		d.useFixed()
		d.mode = inflateCoded
	case 2:
		if err := d.dynamic(); err != nil {
			return err
		}
		d.mode = inflateCoded
	default:
		return errInflateBlock
	}
	return nil
}

// useFixed selects the fixed codes for the current block.
func (d *inflater) useFixed() {
	fixedOnce.Do(buildFixedTables)
	d.lit, d.dist = &fixedLit, &fixedDist
	d.nlit, d.ndist = 0, 0
}

// buildDynamic builds and selects the codes that lengths, nlit and ndist
// describe.
func (d *inflater) buildDynamic() error {
	if err := d.dynLit.build(d.lengths[:d.nlit], litPrimaryBits, true); err != nil {
		return err
	}
	if err := d.dynDist.build(d.lengths[d.nlit:d.nlit+d.ndist], distPrimaryBits, false); err != nil {
		return err
	}
	d.lit, d.dist = &d.dynLit, &d.dynDist
	return nil
}

// dynamic reads the code definitions of a dynamic block (RFC 1951 3.2.7).
func (d *inflater) dynamic() error {
	v, err := d.take(14)
	if err != nil {
		return err
	}
	nlit := int(v&0x1f) + 257
	ndist := int(v>>5&0x1f) + 1
	ncl := int(v>>10) + 4
	if nlit > maxLitCodes || ndist > maxDistCodes {
		return errInflateBlock
	}

	var cl [19]uint8
	for i := 0; i < ncl; i++ {
		v, err := d.take(3)
		if err != nil {
			return err
		}
		cl[clOrder[i]] = uint8(v)
	}
	if err := d.clTab.build(cl[:], clPrimaryBits, false); err != nil {
		return err
	}

	lengths := &d.lengths
	total := nlit + ndist
	for i := 0; i < total; {
		s, err := d.symbol(&d.clTab)
		if err != nil {
			return err
		}
		if s < 16 {
			lengths[i] = uint8(s)
			i++
			continue
		}
		var val uint8
		var rep, extra uint
		switch s {
		case 16:
			if i == 0 {
				return errInflateTable
			}
			val = lengths[i-1]
			rep, extra = 3, 2
		case 17:
			rep, extra = 3, 3
		default:
			rep, extra = 11, 7
		}
		v, err := d.take(extra)
		if err != nil {
			return err
		}
		rep += uint(v)
		if i+int(rep) > total {
			return errInflateTable
		}
		for ; rep > 0; rep-- {
			lengths[i] = val
			i++
		}
	}
	// Without an end-of-block code the block could never end.
	if lengths[256] == 0 {
		return errInflateTable
	}
	d.nlit, d.ndist = nlit, ndist
	return d.buildDynamic()
}

// stored copies stored block bytes to the output. It reports true when the
// block has ended and false when the output buffer filled first.
func (d *inflater) stored() (bool, error) {
	for d.storedLeft > 0 && d.pos < len(d.buf) {
		// Whole bytes already moved into the bit buffer come first.
		if d.nbits >= 8 {
			d.buf[d.pos] = byte(d.bits)
			d.bits >>= 8
			d.nbits -= 8
			d.pos++
			d.storedLeft--
			continue
		}
		if d.ip == d.n && !d.loadInput() {
			return false, d.truncated()
		}
		src := d.in[d.ip:d.n]
		if len(src) > d.storedLeft {
			src = src[:d.storedLeft]
		}
		n := copy(d.buf[d.pos:], src)
		d.ip += n
		d.pos += n
		d.storedLeft -= n
	}
	return d.storedLeft == 0, nil
}

// coded decodes a fixed or dynamic block (RFC 1951 3.2.5). It reports true
// when the end-of-block code was read and false when the output buffer has
// no room left for a longest match.
//
// Most symbols go through codedFast. It stops short of the end of the input
// buffer, where codedOne decodes one symbol with checked reads and loads
// more input, after which the fast path resumes.
func (d *inflater) coded() (bool, error) {
	for {
		ended, err := d.codedFast()
		if ended || err != nil {
			return ended, err
		}
		if d.pos > len(d.buf)-inflateMaxMatch {
			return false, nil
		}
		ended, err = d.codedOne()
		if ended || err != nil {
			return ended, err
		}
	}
}

// codedFast decodes symbols while at least 8 input bytes are buffered and
// the output buffer has inflateFastRoom bytes free. It keeps the bit buffer in
// locals and tops it up 8 bytes at a time, which leaves at least 56 bits:
// more than the 48 the longest length and distance pair takes. No read in
// the loop can therefore run out of input. It reports true when the
// end-of-block code was read.
func (d *inflater) codedFast() (bool, error) {
	buf := d.buf
	pos := d.pos
	in := d.in[:d.n]
	ip := d.ip
	bitbuf, nbits := d.bits, d.nbits
	// The primary tables have a fixed size, so indexing them through an
	// array with a constant mask needs no bounds check.
	lit, dist := d.lit.tab, d.dist.tab
	litPrimary := (*[1 << litPrimaryBits]uint32)(lit)
	distPrimary := (*[1 << distPrimaryBits]uint32)(dist)
	const litMask = 1<<litPrimaryBits - 1
	const distMask = 1<<distPrimaryBits - 1

	ended := false
	var err error
	// Every shift count in the loop is under 64: a code length is at most
	// 15 and an extra bit count at most 13. Masking the count to 6 bits
	// states that, so the shift compiles without a range check.
	for pos <= len(buf)-inflateFastRoom && ip+8 <= len(in) {
		// Bytes are consumed only as whole bytes fit below bit 64. The
		// partial byte above nbits is read again by the next top up.
		bitbuf |= binary.LittleEndian.Uint64(in[ip:]) << nbits
		ip += int((63 - nbits) >> 3)
		nbits |= 56

		e := litPrimary[bitbuf&litMask]
		if e&huffLiteral != 0 {
			// A literal found in the primary table takes at most
			// litPrimaryBits bits, so litPerFill of them can be decoded
			// before the next top up.
			pending := false
			for k := 1; ; k++ {
				n := uint(e & 63)
				bitbuf >>= n
				nbits -= n
				buf[pos] = byte(e >> 8)
				pos++
				if k == litPerFill {
					break
				}
				e = litPrimary[bitbuf&litMask]
				if e&huffLiteral == 0 {
					pending = true
					break
				}
			}
			// The run ended on an entry that is not a primary literal.
			// It was read from bits that a top up leaves in place, so
			// it is still the next entry: top up and decode it below
			// without looking it up again.
			if !pending || ip+8 > len(in) {
				continue
			}
			bitbuf |= binary.LittleEndian.Uint64(in[ip:]) << nbits
			ip += int((63 - nbits) >> 3)
			nbits |= 56
		}
		if e&huffLink != 0 {
			e = lit[uint64(e>>8)+(bitbuf>>litPrimaryBits)&(uint64(1)<<(e&0x0f)-1)]
		}
		n := uint(e & 0xff)
		if n == 0 {
			err = errInflateCode
			break
		}
		bitbuf >>= n & 63
		nbits -= n
		if e&huffLiteral != 0 {
			buf[pos] = byte(e >> 8)
			pos++
			continue
		}
		s := e >> 8
		if s == 256 {
			ended = true
			break
		}
		s -= 257
		if s >= uint32(len(lenBase)) {
			err = errInflateCode
			break
		}
		n = uint(lenExtra[s])
		length := int(lenBase[s]) + int(bitbuf&(uint64(1)<<n-1))
		bitbuf >>= n & 63
		nbits -= n

		e = distPrimary[bitbuf&distMask]
		if e&huffLink != 0 {
			e = dist[uint64(e>>8)+(bitbuf>>distPrimaryBits)&(uint64(1)<<(e&0x0f)-1)]
		}
		n = uint(e & 0xff)
		if n == 0 {
			err = errInflateCode
			break
		}
		bitbuf >>= n & 63
		nbits -= n
		s = e >> 8
		if s >= uint32(len(distBase)) {
			err = errInflateCode
			break
		}
		n = uint(distExtra[s])
		back := int(distBase[s]) + int(bitbuf&(uint64(1)<<n-1))
		bitbuf >>= n & 63
		nbits -= n
		if back > pos {
			err = errInflateDistance
			break
		}
		if back < 8 || length > inflateWordCopyMax {
			pos = copyMatch(buf, pos, back, length)
			continue
		}
		// A short match is copied a word at a time, without a call. The
		// source is at least a word back, so each word read was already
		// written, even when the match overlaps its own output. The last
		// word may run inflateCopySlack bytes past the match.
		src := pos - back
		for i := 0; i < length; i += 8 {
			binary.LittleEndian.PutUint64(buf[pos+i:], binary.LittleEndian.Uint64(buf[src+i:]))
		}
		pos += length
	}

	// Drop the partial byte above nbits so the checked path sees only
	// consumed-from-input bits.
	d.bits = bitbuf & (uint64(1)<<nbits - 1)
	d.nbits = nbits
	d.ip = ip
	d.pos = pos
	return ended, err
}

// codedOne decodes one symbol with reads that check for the end of the
// input and load more of it. It reports true for the end-of-block code.
func (d *inflater) codedOne() (bool, error) {
	s, err := d.symbol(d.lit)
	if err != nil {
		return false, err
	}
	if s < 256 {
		d.buf[d.pos] = byte(s)
		d.pos++
		return false, nil
	}
	if s == 256 {
		return true, nil
	}
	s -= 257
	if s >= uint32(len(lenBase)) {
		return false, errInflateCode
	}
	length := int(lenBase[s])
	if n := uint(lenExtra[s]); n != 0 {
		v, err := d.take(n)
		if err != nil {
			return false, err
		}
		length += int(v)
	}

	s, err = d.symbol(d.dist)
	if err != nil {
		return false, err
	}
	if s >= uint32(len(distBase)) {
		return false, errInflateCode
	}
	back := int(distBase[s])
	if n := uint(distExtra[s]); n != 0 {
		v, err := d.take(n)
		if err != nil {
			return false, err
		}
		back += int(v)
	}
	if back > d.pos {
		return false, errInflateDistance
	}
	d.pos = copyMatch(d.buf, d.pos, back, length)
	return false, nil
}

// copyMatch appends length bytes copied from back bytes before pos and
// returns the new position. The caller has checked back <= pos and that
// length bytes fit.
func copyMatch(buf []byte, pos, back, length int) int {
	src := pos - back
	if back >= length {
		copy(buf[pos:pos+length], buf[src:src+length])
		return pos + length
	}
	// The match overlaps its own output and repeats it.
	for i := 0; i < length; i++ {
		buf[pos+i] = buf[src+i]
	}
	return pos + length
}
