package chd

import "testing"

// makeSector builds a sector with sync, a header for the given mode, a
// deterministic user-data pattern and a stand-in EDC. The P/Q parity region
// is left zero, as CHD compression stores a flagged sector.
func makeSector(seed, mode byte) []byte {
	s := make([]byte, sectorBytes)
	s[0] = 0x00
	for i := 1; i <= 10; i++ {
		s[i] = 0xFF
	}
	s[11] = 0x00
	s[12], s[13], s[14], s[15] = 0x00, 0x02, 0x00, mode // MSF 00:02:00
	for i := 16; i < 2076; i++ {
		s[i] = byte(i*31 + int(seed))
	}
	return s
}

// checkParity verifies the P/Q parity-check equations HP*VP = 0 and
// HQ*VQ = 0 from ECMA-130 over both byte planes, using the spec's matrices
// and indexing independently of the encoder. When zeroHeader is set the four
// header bytes count as zero, as they do for Mode 2.
func checkParity(t *testing.T, s []byte, zeroHeader bool) {
	t.Helper()
	// Reassemble both byte planes (word n -> bytes 2n+12 LSB, 2n+13 MSB).
	var planes [2][rspcWords]byte
	for n := 0; n < rspcWords; n++ {
		planes[0][n] = s[2*n+12]
		planes[1][n] = s[2*n+13]
	}
	if zeroHeader {
		planes[0][0], planes[1][0], planes[0][1], planes[1][1] = 0, 0, 0, 0
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

// checkOnlyParityChanged fails if eccGenerate changed any byte outside the
// P/Q parity region (sector bytes 2076-2351).
func checkOnlyParityChanged(t *testing.T, before, after []byte) {
	t.Helper()
	for i := 0; i < 2076; i++ {
		if before[i] != after[i] {
			t.Fatalf("byte %d changed from %02x to %02x", i, before[i], after[i])
		}
	}
}

// TestECCParity verifies regenerated parity against the ECMA-130 parity-check
// equations for Mode 1, where the header is covered, and Mode 2, where it
// counts as zero, and that nothing but the parity changes: the header and
// EDC are stored and must be kept as they are.
func TestECCParity(t *testing.T) {
	for _, mode := range []byte{1, 2} {
		s := makeSector(9, mode)
		before := append([]byte(nil), s...)
		eccGenerate(s)
		checkParity(t, s, mode == 2)
		checkOnlyParityChanged(t, before, s)
	}
}

// TestECCMode2IgnoresHeader tests that a Mode 2 sector's parity does not
// depend on its header bytes.
func TestECCMode2IgnoresHeader(t *testing.T) {
	a := makeSector(4, 2)
	b := makeSector(4, 2)
	b[12], b[13], b[14] = 0x12, 0x34, 0x56
	eccGenerate(a)
	eccGenerate(b)
	if string(a[2076:]) != string(b[2076:]) {
		t.Fatal("Mode 2 parity changed with the header address")
	}
}

func TestShortBufferIgnored(t *testing.T) {
	s := make([]byte, 100)
	eccGenerate(s) // must not panic
}

// TestMode1ECCSnapshot checks eccGenerate produces the expected P/Q parity
// bytes for a fixed synthetic Mode 1 sector.
func TestMode1ECCSnapshot(t *testing.T) {
	s := makeSector(9, 1)
	eccGenerate(s)

	wantP := []byte{0xa2, 0x61, 0xc8, 0x4b, 0xc4, 0xcf, 0x7f, 0xdd}
	if got := s[2076:2084]; string(got) != string(wantP) {
		t.Errorf("P-parity head = % x, want % x", got, wantP)
	}
	wantQ := []byte{0x11, 0x6e, 0x65, 0x11, 0x7d, 0x13, 0xb0, 0xa4}
	if got := s[2248:2256]; string(got) != string(wantQ) {
		t.Errorf("Q-parity head = % x, want % x", got, wantQ)
	}
}
