package chd

import "testing"

// makeMode1Sector builds a Mode 1 sector with sync, a header, and a
// deterministic user-data pattern. EDC/ECC regions are left zero.
func makeMode1Sector(seed byte) []byte {
	s := make([]byte, sectorBytes)
	s[0] = 0x00
	for i := 1; i <= 10; i++ {
		s[i] = 0xFF
	}
	s[11] = 0x00
	s[12], s[13], s[14], s[15] = 0x00, 0x02, 0x00, 0x01 // MSF 00:02:00, mode 1
	for i := 16; i < 2064; i++ {
		s[i] = byte(i*31 + int(seed))
	}
	return s
}

// edcBitwise is an independent bit-at-a-time reference for the table-driven
// edcCompute, used to validate the LUT construction.
func edcBitwise(data []byte) uint32 {
	var c uint32
	for _, b := range data {
		c ^= uint32(b)
		for i := 0; i < 8; i++ {
			if c&1 != 0 {
				c = (c >> 1) ^ edcPoly
			} else {
				c >>= 1
			}
		}
	}
	return c
}

func TestEDCTableMatchesBitwise(t *testing.T) {
	data := makeMode1Sector(5)[0:2064]
	if got, want := edcCompute(data), edcBitwise(data); got != want {
		t.Errorf("edcCompute = %08x, bitwise reference = %08x", got, want)
	}
}

// TestMode1ECCSpec verifies a regenerated Mode 1 sector against the ECMA-130
// definitions directly: the EDC divisibility property (a correct codeword over
// bytes 0..2067 hashes to zero) and the P/Q parity-check equations
// HP*VP = 0 / HQ*VQ = 0. These checks use the spec's matrices and indexing
// independently of the encoder, so they catch arithmetic and indexing errors
// without needing an external vector.
func TestMode1ECCSpec(t *testing.T) {
	s := makeMode1Sector(9)
	eccGenerate(s)

	if got := edcCompute(s[0:2068]); got != 0 {
		t.Errorf("EDC divisibility: edc over bytes 0..2067 = %08x, want 0", got)
	}

	// Reassemble both byte planes (word n -> bytes 2n+12 LSB, 2n+13 MSB).
	var planes [2][rspcWords]byte
	for n := 0; n < rspcWords; n++ {
		planes[0][n] = s[2*n+12]
		planes[1][n] = s[2*n+13]
	}

	for pi := range planes {
		p := &planes[pi]
		// P-vectors: 26 symbols at 43*pos+np, parity weights alpha^(25-pos).
		for np := 0; np < 43; np++ {
			var e1, e2 byte
			for pos := 0; pos < 26; pos++ {
				sym := p[43*pos+np]
				e1 ^= sym
				e2 ^= gfMul(sym, gfExp[25-pos])
			}
			if e1 != 0 || e2 != 0 {
				t.Fatalf("plane %d P-vector %d: syndromes %02x %02x, want 0 0", pi, np, e1, e2)
			}
		}
		// Q-vectors: 45 symbols, data at (44*pos+43*nq) mod 1118, parity at
		// 1118+nq / 1144+nq, weights alpha^(44-pos).
		for nq := 0; nq < 26; nq++ {
			var e1, e2 byte
			for pos := 0; pos < 45; pos++ {
				var idx int
				switch {
				case pos < 43:
					idx = (44*pos + 43*nq) % 1118
				case pos == 43:
					idx = 1118 + nq
				default:
					idx = 1144 + nq
				}
				sym := p[idx]
				e1 ^= sym
				e2 ^= gfMul(sym, gfExp[44-pos])
			}
			if e1 != 0 || e2 != 0 {
				t.Fatalf("plane %d Q-vector %d: syndromes %02x %02x, want 0 0", pi, nq, e1, e2)
			}
		}
	}
}

func TestNonMode1Untouched(t *testing.T) {
	s := makeMode1Sector(3)
	s[15] = 0x02 // Mode 2: no EDC/ECC, must be left as-is
	before := make([]byte, len(s))
	copy(before, s)
	eccGenerate(s)
	for i := range s {
		if s[i] != before[i] {
			t.Fatalf("Mode 2 sector modified at byte %d", i)
		}
	}
}

func TestShortBufferIgnored(t *testing.T) {
	s := make([]byte, 100)
	eccGenerate(s) // must not panic
}

// TestMode1ECCSnapshot checks eccGenerate produces the expected EDC and P/Q
// parity bytes for a fixed synthetic Mode 1 sector.
func TestMode1ECCSnapshot(t *testing.T) {
	s := makeMode1Sector(9)
	eccGenerate(s)

	if got := edcCompute(s[0:2064]); got != 0x41ff16ab {
		t.Errorf("EDC = %08x, want 41ff16ab", got)
	}
	wantP := []byte{0xa2, 0x61, 0xc8, 0x4b, 0xc4, 0xcf, 0x7f, 0xdd}
	if got := s[2076:2084]; string(got) != string(wantP) {
		t.Errorf("P-parity head = % x, want % x", got, wantP)
	}
	wantQ := []byte{0x11, 0x6e, 0x65, 0x11, 0x7d, 0x13, 0xb0, 0xa4}
	if got := s[2248:2256]; string(got) != string(wantQ) {
		t.Errorf("Q-parity head = % x, want % x", got, wantQ)
	}
}
