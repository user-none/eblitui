package romloader

import (
	"bytes"
	"compress/flate"
	"hash/crc32"
	"io"
	"math"
	"math/rand"
	"testing"
)

// testDeflate compresses data as a raw deflate stream at the given level.
func testDeflate(t testing.TB, data []byte, level int) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, level)
	if err != nil {
		t.Fatalf("flate.NewWriter: %v", err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("deflate write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("deflate close: %v", err)
	}
	return buf.Bytes()
}

// testInflate decodes a whole raw deflate stream, stopping with an error
// once the output passes limit bytes.
func testInflate(comp []byte, limit int) ([]byte, error) {
	var d inflater
	if err := d.reset(bytes.NewReader(comp), 0, int64(len(comp)), inflatePoint{}, nil); err != nil {
		return nil, err
	}
	var out []byte
	for {
		run, err := d.next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, run...)
		if len(out) > limit {
			return nil, ErrFileTooLarge
		}
	}
}

// testMixedData returns n bytes built from a small set of random words, so
// it compresses with matches at many distances, with stretches of random
// bytes and of a single repeated byte between them.
func testMixedData(n int, seed int64) []byte {
	rng := rand.New(rand.NewSource(seed))
	words := make([][]byte, 200)
	for i := range words {
		words[i] = make([]byte, 3+rng.Intn(60))
		rng.Read(words[i])
	}
	out := make([]byte, 0, n+4096)
	for len(out) < n {
		switch r := rng.Intn(100); {
		case r < 90:
			out = append(out, words[rng.Intn(len(words))]...)
		case r < 95:
			noise := make([]byte, rng.Intn(2000))
			rng.Read(noise)
			out = append(out, noise...)
		default:
			out = append(out, bytes.Repeat([]byte{byte(rng.Intn(256))}, rng.Intn(4000))...)
		}
	}
	return out[:n]
}

func testRandomData(n int, seed int64) []byte {
	out := make([]byte, n)
	rand.New(rand.NewSource(seed)).Read(out)
	return out
}

// TestInflate_RoundTrip tests that streams written at every compression
// level, covering stored, fixed and dynamic blocks, decode to their source.
func TestInflate_RoundTrip(t *testing.T) {
	inputs := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"one byte", []byte{0x5a}},
		{"short text", []byte("a cue sheet names the bin files of a disc")},
		{"zeros", make([]byte, 500000)},
		{"random", testRandomData(300000, 1)},
		{"mixed", testMixedData(2000000, 2)},
	}
	levels := []struct {
		name  string
		level int
	}{
		{"stored", flate.NoCompression},
		{"fastest", flate.BestSpeed},
		{"default", flate.DefaultCompression},
		{"best", flate.BestCompression},
		{"huffman only", flate.HuffmanOnly},
	}
	for _, in := range inputs {
		for _, lv := range levels {
			t.Run(in.name+"/"+lv.name, func(t *testing.T) {
				comp := testDeflate(t, in.data, lv.level)
				got, err := testInflate(comp, len(in.data))
				if err != nil {
					t.Fatalf("inflate: %v", err)
				}
				if !bytes.Equal(got, in.data) {
					t.Fatalf("decoded %d bytes that differ from the %d byte source", len(got), len(in.data))
				}
			})
		}
	}
}

// testFixedBlock encodes data, bytes below 144 only, as one final block of
// literals in the fixed codes (RFC 1951 3.2.6). The standard library only
// writes fixed blocks for short input, too short to stop inside one.
func testFixedBlock(data []byte) []byte {
	var out []byte
	var acc uint64
	var n uint
	put := func(v uint64, bits uint) {
		acc |= v << n
		n += bits
		for n >= 8 {
			out = append(out, byte(acc))
			acc >>= 8
			n -= 8
		}
	}
	// Codes are written most significant bit first.
	putCode := func(code uint64, bits int) {
		for i := bits - 1; i >= 0; i-- {
			put(code>>uint(i)&1, 1)
		}
	}
	put(1, 1) // final block
	put(1, 2) // fixed codes
	for _, b := range data {
		putCode(0x30+uint64(b), 8)
	}
	putCode(0, 7) // end of block
	if n > 0 {
		out = append(out, byte(acc))
	}
	return out
}

// TestInflate_Resume tests that decoding resumed at a position reported
// between two runs, with the window reported there, produces the rest of
// the data. Each stream must include positions inside the kind of block it
// is made of, not only at block starts.
func TestInflate_Resume(t *testing.T) {
	mixed := testMixedData(1500000, 3)
	low := testRandomData(300000, 9)
	for i := range low {
		low[i] &= 0x7f
	}
	inStored := func(p inflatePoint) bool { return p.mode == inflateStored && p.stored > 0 }
	inDynamic := func(p inflatePoint) bool { return p.mode == inflateCoded && p.lengths != nil }
	inFixed := func(p inflatePoint) bool { return p.mode == inflateCoded && p.lengths == nil }

	cases := []struct {
		name   string
		data   []byte
		comp   []byte
		inside func(p inflatePoint) bool
	}{
		{"stored", mixed, testDeflate(t, mixed, flate.NoCompression), inStored},
		{"dynamic", mixed, testDeflate(t, mixed, flate.DefaultCompression), inDynamic},
		{"fixed", low, testFixedBlock(low), inFixed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := bytes.NewReader(tc.comp)
			size := int64(len(tc.comp))

			type resume struct {
				out    int
				at     inflatePoint
				window []byte
			}
			var points []resume
			inside := 0
			var d inflater
			if err := d.reset(src, 0, size, inflatePoint{}, nil); err != nil {
				t.Fatalf("reset: %v", err)
			}
			total := 0
			for total < len(tc.data) {
				run, err := d.next()
				if err != nil {
					t.Fatalf("inflate: %v", err)
				}
				total += len(run)
				if total == len(tc.data) {
					break
				}
				p := d.point()
				if tc.inside(p) {
					inside++
				}
				points = append(points, resume{total, p, bytes.Clone(d.window())})
			}
			if inside == 0 {
				t.Fatalf("none of the %d positions is inside a block", len(points))
			}

			for _, r := range points {
				if err := d.reset(src, 0, size, r.at, r.window); err != nil {
					t.Fatalf("resume at output %d: %v", r.out, err)
				}
				var got []byte
				for {
					run, err := d.next()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatalf("resume at output %d: %v", r.out, err)
					}
					got = append(got, run...)
				}
				if !bytes.Equal(got, tc.data[r.out:]) {
					t.Fatalf("resume at output %d decoded different data", r.out)
				}
			}
		})
	}
}

// TestInflate_Truncated tests that a stream cut short at any point is
// reported as an error and never as a complete stream.
func TestInflate_Truncated(t *testing.T) {
	data := testMixedData(200000, 4)
	for _, level := range []int{flate.NoCompression, flate.DefaultCompression} {
		comp := testDeflate(t, data, level)
		for _, cut := range []int{0, 1, 2, 5, len(comp) / 3, len(comp) / 2, len(comp) - 2, len(comp) - 1} {
			if _, err := testInflate(comp[:cut], len(data)); err == nil {
				t.Fatalf("level %d: stream cut to %d of %d bytes decoded without error", level, cut, len(comp))
			}
		}
	}
}

// TestInflate_Corrupt tests that damaged streams never crash the decoder.
func TestInflate_Corrupt(t *testing.T) {
	data := testMixedData(100000, 5)
	comp := testDeflate(t, data, flate.DefaultCompression)
	rng := rand.New(rand.NewSource(6))
	for i := 0; i < 2000; i++ {
		bad := bytes.Clone(comp)
		for flips := 1 + rng.Intn(3); flips > 0; flips-- {
			bad[rng.Intn(len(bad))] ^= 1 << rng.Intn(8)
		}
		// Either an error or some output is acceptable. The limit keeps a
		// damaged stream from being decoded without bound.
		testInflate(bad, 64*len(data))
	}
}

// TestInflate_InvalidBlocks tests hand-built streams that break the format.
func TestInflate_InvalidBlocks(t *testing.T) {
	cases := []struct {
		name string
		comp []byte
	}{
		{"reserved block type", []byte{0x07}},
		{"stored length complement", []byte{0x01, 0x05, 0x00, 0x00, 0x00, 1, 2, 3, 4, 5}},
		// Fixed block whose first symbol is a match, with no output yet
		// for it to copy from: length code 257 (0000001), distance code 0.
		{"match before start", []byte{0x03, 0x02}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := testInflate(tc.comp, 1<<20); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// testSkewedData returns n bytes with no repeats to match but an uneven
// byte distribution, so it compresses to coded literals only. The larger
// spread is, the more even the distribution and the less it compresses.
func testSkewedData(n int, seed int64, spread float64) []byte {
	rng := rand.New(rand.NewSource(seed))
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(128 + rng.NormFloat64()*spread)
	}
	return out
}

// testShortMatchData returns n bytes of random data in which short runs,
// 3 to 12 bytes long, repeat earlier data from within the match window. It
// compresses to short matches mixed with literals, like a game data track.
func testShortMatchData(n int, seed int64) []byte {
	rng := rand.New(rand.NewSource(seed))
	out := make([]byte, 0, n+16)
	for len(out) < n {
		if len(out) > 64 && rng.Intn(100) < 12 {
			back := 1 + rng.Intn(min(len(out), inflateWindow)-12)
			length := 3 + rng.Intn(10)
			start := len(out) - back
			out = append(out, out[start:start+min(length, back)]...)
			continue
		}
		out = append(out, byte(128+rng.NormFloat64()*45))
	}
	return out[:n]
}

// testAudioData returns n bytes shaped like a CD audio track: 16-bit
// little-endian stereo samples of a few mixed tones with noise, the two
// channels nearly alike, with a stretch of silence.
func testAudioData(n int, seed int64) []byte {
	rng := rand.New(rand.NewSource(seed))
	out := make([]byte, 0, n+4)
	for i := 0; len(out) < n; i++ {
		var left, right int16
		// Silence for one part in sixteen.
		if i%(16*44100) >= 44100 {
			t := float64(i)
			v := 6000*math.Sin(t*0.031) + 3000*math.Sin(t*0.173) + 1500*math.Sin(t*0.77)
			left = int16(v + rng.NormFloat64()*300)
			right = int16(v*0.9 + rng.NormFloat64()*300)
		}
		out = append(out, byte(left), byte(left>>8), byte(right), byte(right>>8))
	}
	return out[:n]
}

// BenchmarkInflate measures decode throughput on data that compresses to
// long matches and on data that compresses to coded literals, the two
// extremes of what a decode loop spends its time on. The dense case barely
// compresses, like audio tracks and already compressed video.
func BenchmarkInflate(b *testing.B) {
	inputs := []struct {
		name  string
		data  []byte
		level int
	}{
		{"matches", testMixedData(8<<20, 50), flate.DefaultCompression},
		{"literals", testSkewedData(8<<20, 51, 20), flate.HuffmanOnly},
		{"dense literals", testSkewedData(8<<20, 52, 45), flate.HuffmanOnly},
		{"short matches", testShortMatchData(8<<20, 53), flate.BestCompression},
		{"audio", testAudioData(8<<20, 54), flate.BestCompression},
	}
	for _, in := range inputs {
		comp := testDeflate(b, in.data, in.level)
		src := bytes.NewReader(comp)
		b.Run(in.name, func(b *testing.B) {
			var d inflater
			b.SetBytes(int64(len(in.data)))
			b.ReportAllocs()
			b.ReportMetric(float64(len(comp))/float64(len(in.data)), "ratio")
			for i := 0; i < b.N; i++ {
				if err := d.reset(src, 0, int64(len(comp)), inflatePoint{}, nil); err != nil {
					b.Fatalf("reset: %v", err)
				}
				for {
					if _, err := d.next(); err == io.EOF {
						break
					} else if err != nil {
						b.Fatalf("inflate: %v", err)
					}
				}
			}
		})
	}
}

// BenchmarkZipIndex measures the index build of one deflated entry: the
// decode plus the checksum and the checkpoints taken along the way. Against
// BenchmarkInflate on the same data it shows what the build adds.
func BenchmarkZipIndex(b *testing.B) {
	data := testShortMatchData(8<<20, 53)
	comp := testDeflate(b, data, flate.BestCompression)
	e := &zipEntry{
		r:     bytes.NewReader(comp),
		csize: int64(len(comp)),
		size:  int64(len(data)),
		crc:   crc32.ChecksumIEEE(data),
	}
	var inf inflater
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := e.buildIndex(&inf); err != nil {
			b.Fatalf("buildIndex: %v", err)
		}
	}
}

// TestInflate_ResetPastEnd tests that resuming at a position past the end
// of the stream is reported by reset.
func TestInflate_ResetPastEnd(t *testing.T) {
	comp := testDeflate(t, testMixedData(5000, 70), flate.DefaultCompression)
	var d inflater
	past := inflatePoint{bit: int64(len(comp))*8 + 3}
	if err := d.reset(bytes.NewReader(comp), 0, int64(len(comp)), past, nil); err == nil {
		t.Fatal("expected an error")
	}
}

// FuzzInflate checks the decoder on arbitrary input. It must not crash. A
// stream it and the standard library both accept must decode to the same
// bytes. A stream it accepts must decode to the same bytes when resumed at
// a position between runs.
func FuzzInflate(f *testing.F) {
	f.Add(testDeflate(f, []byte("seed data seed data seed data"), flate.DefaultCompression))
	f.Add(testDeflate(f, testMixedData(5000, 7), flate.BestSpeed))
	f.Add(testDeflate(f, testRandomData(3000, 8), flate.NoCompression))
	f.Add(testDeflate(f, testMixedData(300000, 9), flate.DefaultCompression))
	f.Add([]byte{0x03, 0x02})
	f.Fuzz(func(t *testing.T, comp []byte) {
		const limit = 4 << 20
		got, err := testInflate(comp, limit)
		if err != nil {
			return
		}
		want, werr := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(comp)), limit+1))
		if werr == nil && !bytes.Equal(got, want) {
			t.Fatalf("decoded %d bytes, the standard library decoded %d different bytes", len(got), len(want))
		}

		// Collect the first few positions between runs, then resume at each.
		type resume struct {
			out    int
			at     inflatePoint
			window []byte
		}
		src := bytes.NewReader(comp)
		size := int64(len(comp))
		var d inflater
		if err := d.reset(src, 0, size, inflatePoint{}, nil); err != nil {
			t.Fatalf("reset: %v", err)
		}
		var points []resume
		for total := 0; len(points) < 8; {
			run, err := d.next()
			if err != nil {
				break
			}
			total += len(run)
			if total < len(got) {
				points = append(points, resume{total, d.point(), bytes.Clone(d.window())})
			}
		}
		for _, r := range points {
			if err := d.reset(src, 0, size, r.at, r.window); err != nil {
				t.Fatalf("resume at output %d: %v", r.out, err)
			}
			var rest []byte
			for {
				run, err := d.next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("resume at output %d: %v", r.out, err)
				}
				rest = append(rest, run...)
			}
			if !bytes.Equal(rest, got[r.out:]) {
				t.Fatalf("resume at output %d decoded different data", r.out)
			}
		}
	})
}
