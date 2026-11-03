// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package chd

// CD-ROM ECC regeneration. CHD CD compression strips the sync and the P/Q
// parity from data sectors whose parity checks out and flags them; this
// reconstructs the P/Q parity so a decoded sector is byte-faithful to the
// original image. Mode 1 and Mode 2 Form 1 sectors are handled; for Mode 2 the
// header bytes count as zero in the parity.

const (
	sectorBytes   = 2352
	gfPrimitive   = 0x11D // x^8 + x^4 + x^3 + x^2 + 1
	rspcDataWords = 1032  // words 0..1031 cover sector bytes 12..2075
	rspcWords     = 1170  // words 0..1169 (data + P/Q parity)
)

var (
	gfExp  [256]byte
	gfLog  [256]byte
	gfInv3 byte
)

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfLog[byte(x)] = byte(i)
		x <<= 1
		if x&0x100 != 0 {
			x ^= gfPrimitive
		}
	}
	gfInv3 = gfExp[(255-int(gfLog[3]))%255]
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[(int(gfLog[a])+int(gfLog[b]))%255]
}

// eccGenerate fills the P/Q parity of a sector in place from the bytes as
// stored. CHD compression strips only the sync and the P/Q parity from a
// flagged sector; the header, EDC and the rest are stored, so only the parity
// is rebuilt. For a Mode 2 sector the four header bytes count as zero in the
// parity, as the Mode 2 Form 1 layout requires. Buffers shorter than a full
// sector are left unmodified.
func eccGenerate(sector []byte) {
	if len(sector) < sectorBytes {
		return
	}

	// The RSPC runs independently on the two byte planes of the 16-bit words:
	// word n holds sector byte 2n+12 (LSB plane) and 2n+13 (MSB plane).
	var lsb, msb [rspcWords]byte
	for n := 0; n < rspcDataWords; n++ {
		lsb[n] = sector[2*n+12]
		msb[n] = sector[2*n+13]
	}
	if sector[15] == 2 {
		lsb[0], msb[0], lsb[1], msb[1] = 0, 0, 0, 0
	}
	eccPlane(&lsb)
	eccPlane(&msb)
	for n := rspcDataWords; n < rspcWords; n++ {
		sector[2*n+12] = lsb[n]
		sector[2*n+13] = msb[n]
	}
}

// eccPlane computes the P then Q parity words of one byte plane in place. P must
// precede Q because the Q-vectors read the P-parity words.
func eccPlane(p *[rspcWords]byte) {
	// 43 P-vectors, each a (26,24) code: data words 43*mp+np (mp 0..23),
	// parity words 1032+np and 1075+np.
	for np := 0; np < 43; np++ {
		var d0, d1 byte
		for mp := 0; mp < 24; mp++ {
			b := p[43*mp+np]
			d0 ^= b
			d1 ^= gfMul(b, gfExp[25-mp])
		}
		p0 := gfMul(d0^d1, gfInv3)
		p[1032+np] = p0
		p[1075+np] = d0 ^ p0
	}

	// 26 Q-vectors, each a (45,43) code: data words (44*mq+43*nq) mod 1118
	// (mq 0..42), parity words 1118+nq and 1144+nq.
	for nq := 0; nq < 26; nq++ {
		var d0, d1 byte
		for mq := 0; mq < 43; mq++ {
			b := p[(44*mq+43*nq)%1118]
			d0 ^= b
			d1 ^= gfMul(b, gfExp[44-mq])
		}
		p0 := gfMul(d0^d1, gfInv3)
		p[1118+nq] = p0
		p[1144+nq] = d0 ^ p0
	}
}
