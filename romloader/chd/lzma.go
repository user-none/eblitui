package chd

import (
	"errors"
	"fmt"
)

// decodeLZMA decodes a raw LZMA stream (no file header) into dst.
// props is the LZMA properties byte, dictSize is the dictionary size.
// The stream must end with an end-of-stream marker (distance 0xFFFFFFFF).
// If dstSize > 0, decoding also stops when that many bytes have been produced.
func decodeLZMA(src []byte, dstSize int, props byte, dictSize uint32) ([]byte, error) {
	lc := int(props % 9)
	rem := props / 9
	lp := int(rem % 5)
	pb := int(rem / 5)
	if pb > 4 {
		return nil, fmt.Errorf("lzma: invalid properties byte 0x%02X", props)
	}

	posStates := 1 << pb
	litStates := (1 << lp) * (1 << lc)

	d := &lzmaDecoder{
		lc:        lc,
		lp:        lp,
		posStates: posStates,
		dictSize:  int(dictSize),
		dstSize:   dstSize,
	}

	// Allocate probability arrays - all initialized to kProbInitValue (1024)
	// isMatch and isRep0Long use kNumPosStatesBitsMax=4 for layout,
	// not the pb-derived posStates, per the LZMA specification.
	const kNumPosStatesMax = 1 << 4 // 16
	d.isMatch = makeProbs(12 * kNumPosStatesMax)
	d.isRep = makeProbs(12)
	d.isRepG0 = makeProbs(12)
	d.isRepG1 = makeProbs(12)
	d.isRepG2 = makeProbs(12)
	d.isRep0Long = makeProbs(12 * kNumPosStatesMax)
	d.posSlot = makeProbs(4 * 64)
	d.specPos = makeProbs(114) // kNumFullDistances - kEndPosModelIndex = 128 - 14
	d.align = makeProbs(16)
	// Length decoders use kNumPosBitsMax=4 (16 pos states), not pb
	const kNumPosBitsMax = 4
	lenPosStates := 1 << kNumPosBitsMax
	d.lenCoder = makeProbs(2 + lenPosStates*8*2 + 256)
	d.repLenCoder = makeProbs(2 + lenPosStates*8*2 + 256)
	d.literal = makeProbs(768 * litStates)

	// Initialize dictionary
	if d.dictSize < 4096 {
		d.dictSize = 4096
	}
	d.dict = make([]byte, d.dictSize)

	// Initialize range decoder
	if len(src) < 5 {
		return nil, errors.New("lzma: source too short for range decoder init")
	}
	// First byte must be 0x00, bytes 1-4 are the initial code value
	d.rd.src = src
	d.rd.pos = 5
	d.rd.rng = 0xFFFFFFFF
	d.rd.code = uint32(src[1])<<24 | uint32(src[2])<<16 | uint32(src[3])<<8 | uint32(src[4])

	return d.decode()
}

func makeProbs(n int) []uint16 {
	probs := make([]uint16, n)
	for i := range probs {
		probs[i] = 1024 // (2048 >> 1)
	}
	return probs
}

// rangeDecoder implements LZMA range coding.
type rangeDecoder struct {
	src  []byte
	pos  int
	rng  uint32
	code uint32
}

func (rd *rangeDecoder) normalize() {
	if rd.rng < 0x01000000 {
		rd.rng <<= 8
		var b byte
		if rd.pos < len(rd.src) {
			b = rd.src[rd.pos]
		}
		rd.pos++
		rd.code = (rd.code << 8) | uint32(b)
	}
}

func (rd *rangeDecoder) decodeBit(prob *uint16) int {
	bound := (rd.rng >> 11) * uint32(*prob)
	if rd.code < bound {
		rd.rng = bound
		*prob += (2048 - *prob) >> 5
		rd.normalize()
		return 0
	}
	rd.rng -= bound
	rd.code -= bound
	*prob -= *prob >> 5
	rd.normalize()
	return 1
}

func (rd *rangeDecoder) decodeTree(probs []uint16, numBits int) uint32 {
	m := uint32(1)
	for i := 0; i < numBits; i++ {
		m = (m << 1) | uint32(rd.decodeBit(&probs[m]))
	}
	return m - (1 << numBits)
}

func (rd *rangeDecoder) decodeReverse(probs []uint16, numBits int) uint32 {
	m := uint32(1)
	var result uint32
	for i := 0; i < numBits; i++ {
		bit := uint32(rd.decodeBit(&probs[m]))
		m = (m << 1) | bit
		result |= bit << i
	}
	return result
}

func (rd *rangeDecoder) decodeDirect(numBits int) uint32 {
	var result uint32
	for numBits > 0 {
		rd.rng >>= 1
		rd.code -= rd.rng
		t := uint32(0) - (rd.code >> 31)
		rd.code += rd.rng & t
		rd.normalize()
		result <<= 1
		result += t + 1
		numBits--
	}
	return result
}

// lzmaDecoder holds the full LZMA decoder state.
type lzmaDecoder struct {
	rd rangeDecoder

	lc, lp    int
	posStates int
	dictSize  int
	dstSize   int

	// Probability arrays
	isMatch     []uint16
	isRep       []uint16
	isRepG0     []uint16
	isRepG1     []uint16
	isRepG2     []uint16
	isRep0Long  []uint16
	posSlot     []uint16
	specPos     []uint16
	align       []uint16
	lenCoder    []uint16
	repLenCoder []uint16
	literal     []uint16

	// Dictionary
	dict    []byte
	dictPos int

	// State machine
	state int
	rep0  uint32
	rep1  uint32
	rep2  uint32
	rep3  uint32
}

func (d *lzmaDecoder) decodeLiteral() byte {
	prevByte := byte(0)
	if d.dictPos > 0 {
		prevByte = d.dict[(d.dictPos-1)%d.dictSize]
	}

	lpMask := (1 << d.lp) - 1
	litState := ((d.dictPos & lpMask) << d.lc) | (int(prevByte) >> (8 - d.lc))
	probs := d.literal[litState*768 : litState*768+768]

	if d.state < 7 {
		// Normal literal
		return byte(d.rd.decodeTree(probs, 8))
	}

	// Matched literal: use match byte for context
	matchByte := d.dict[(d.dictPos-int(d.rep0)-1+d.dictSize*2)%d.dictSize]
	symbol := uint32(1)
	for symbol < 0x100 {
		matchBit := uint32((matchByte >> 7) & 1)
		matchByte <<= 1
		bit := uint32(d.rd.decodeBit(&probs[((1+matchBit)<<8)+symbol]))
		symbol = (symbol << 1) | bit
		if matchBit != bit {
			for symbol < 0x100 {
				bit = uint32(d.rd.decodeBit(&probs[symbol]))
				symbol = (symbol << 1) | bit
			}
			break
		}
	}
	return byte(symbol)
}

func (d *lzmaDecoder) decodeLength(coder []uint16, posState int) int {
	// Length decoders always use 16 pos states (kNumPosBitsMax=4)
	const lenPosStates = 1 << 4
	if d.rd.decodeBit(&coder[0]) == 0 {
		// Short: 3-bit, lengths 2-9
		return int(d.rd.decodeTree(coder[2+posState*8:], 3)) + 2
	}
	if d.rd.decodeBit(&coder[1]) == 0 {
		// Mid: 3-bit, lengths 10-17
		return int(d.rd.decodeTree(coder[2+lenPosStates*8+posState*8:], 3)) + 10
	}
	// High: 8-bit, lengths 18-273
	return int(d.rd.decodeTree(coder[2+lenPosStates*8*2:], 8)) + 18
}

func (d *lzmaDecoder) decode() ([]byte, error) {
	var out []byte
	if d.dstSize > 0 {
		out = make([]byte, 0, d.dstSize)
	}

	for {
		if d.dstSize > 0 && len(out) >= d.dstSize {
			break
		}

		posState := d.dictPos & (d.posStates - 1)

		if d.rd.decodeBit(&d.isMatch[(d.state<<4)+posState]) == 0 {
			// Literal
			lit := d.decodeLiteral()
			d.dict[d.dictPos%d.dictSize] = lit
			d.dictPos++
			out = append(out, lit)
			d.state = litNextState[d.state]
			continue
		}

		if d.rd.decodeBit(&d.isRep[d.state]) == 0 {
			// Simple match
			length := d.decodeLength(d.lenCoder, posState)

			lenBucket := length - 2
			if lenBucket > 3 {
				lenBucket = 3
			}
			slot := d.rd.decodeTree(d.posSlot[lenBucket*64:], 6)

			var dist uint32
			if slot < 4 {
				dist = slot
			} else {
				numDirect := int(slot>>1) - 1
				dist = (2 | (slot & 1)) << numDirect
				if slot < 14 {
					// Reverse bit tree decode with offset into specPos.
					// startIndex = dist - slot - 1 (reference: lzma-go reverseDecodeIndex)
					startIdx := dist - slot - 1
					m := uint32(1)
					var rev uint32
					for i := 0; i < numDirect; i++ {
						bit := uint32(d.rd.decodeBit(&d.specPos[startIdx+m]))
						m = (m << 1) | bit
						rev |= bit << i
					}
					dist += rev
				} else {
					// Direct bits + alignment
					dist += d.rd.decodeDirect(numDirect-4) << 4
					dist += d.rd.decodeReverse(d.align[:], 4)
				}
			}

			if dist == 0xFFFFFFFF {
				// End of stream marker
				break
			}

			if int(dist) >= d.dictPos && int(dist) >= d.dictSize {
				return nil, fmt.Errorf("lzma: distance %d exceeds dictionary", dist)
			}

			d.rep3 = d.rep2
			d.rep2 = d.rep1
			d.rep1 = d.rep0
			d.rep0 = dist
			d.state = matchNextState[d.state]

			d.copyMatch(length, &out)
		} else {
			// Rep match
			var length int
			if d.rd.decodeBit(&d.isRepG0[d.state]) == 0 {
				// rep0
				if d.rd.decodeBit(&d.isRep0Long[(d.state<<4)+posState]) == 0 {
					// ShortRep: single byte
					d.state = shortRepNextState[d.state]
					b := d.dict[(d.dictPos-int(d.rep0)-1+d.dictSize*2)%d.dictSize]
					d.dict[d.dictPos%d.dictSize] = b
					d.dictPos++
					out = append(out, b)
					continue
				}
				length = d.decodeLength(d.repLenCoder, posState)
			} else if d.rd.decodeBit(&d.isRepG1[d.state]) == 0 {
				// rep1
				d.rep0, d.rep1 = d.rep1, d.rep0
				length = d.decodeLength(d.repLenCoder, posState)
			} else if d.rd.decodeBit(&d.isRepG2[d.state]) == 0 {
				// rep2
				d.rep0, d.rep1, d.rep2 = d.rep2, d.rep0, d.rep1
				length = d.decodeLength(d.repLenCoder, posState)
			} else {
				// rep3
				d.rep0, d.rep1, d.rep2, d.rep3 = d.rep3, d.rep0, d.rep1, d.rep2
				length = d.decodeLength(d.repLenCoder, posState)
			}
			d.state = repNextState[d.state]
			d.copyMatch(length, &out)
		}
	}

	return out, nil
}

func (d *lzmaDecoder) copyMatch(length int, out *[]byte) {
	for i := 0; i < length; i++ {
		b := d.dict[(d.dictPos-int(d.rep0)-1+d.dictSize*2)%d.dictSize]
		d.dict[d.dictPos%d.dictSize] = b
		d.dictPos++
		*out = append(*out, b)
	}
}

// State transition tables (12 states)
var litNextState = [12]int{0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 4, 5}
var matchNextState = [12]int{7, 7, 7, 7, 7, 7, 7, 10, 10, 10, 10, 10}
var repNextState = [12]int{8, 8, 8, 8, 8, 8, 8, 11, 11, 11, 11, 11}
var shortRepNextState = [12]int{9, 9, 9, 9, 9, 9, 9, 11, 11, 11, 11, 11}
