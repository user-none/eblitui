package chd

import (
	"testing"
)

func TestFlacBitReaderReadBits(t *testing.T) {
	// 0xAB = 1010_1011, 0xCD = 1100_1101
	br := newFlacBitReader([]byte{0xAB, 0xCD})

	// Read 4 bits: 1010 = 10
	v, err := br.readBits(4)
	if err != nil {
		t.Fatal(err)
	}
	if v != 0x0A {
		t.Errorf("readBits(4): got 0x%02X, want 0x0A", v)
	}

	// Read 8 bits: 1011_1100 = 0xBC
	v, err = br.readBits(8)
	if err != nil {
		t.Fatal(err)
	}
	if v != 0xBC {
		t.Errorf("readBits(8): got 0x%02X, want 0xBC", v)
	}

	// Read 4 bits: 1101 = 0x0D
	v, err = br.readBits(4)
	if err != nil {
		t.Fatal(err)
	}
	if v != 0x0D {
		t.Errorf("readBits(4): got 0x%02X, want 0x0D", v)
	}
}

func TestFlacBitReaderReadBitsZero(t *testing.T) {
	br := newFlacBitReader([]byte{0xFF})
	v, err := br.readBits(0)
	if err != nil {
		t.Fatal(err)
	}
	if v != 0 {
		t.Errorf("readBits(0): got %d, want 0", v)
	}
}

func TestFlacBitReaderSignedBits(t *testing.T) {
	tests := []struct {
		data []byte
		bits int
		want int32
	}{
		// 0x7F = 0111_1111, read 8 bits signed = 127
		{[]byte{0x7F}, 8, 127},
		// 0xFF = 1111_1111, read 8 bits signed = -1
		{[]byte{0xFF}, 8, -1},
		// 0x80 = 1000_0000, read 8 bits signed = -128
		{[]byte{0x80}, 8, -128},
		// 0xC0 = 1100_0000, read 4 bits = 1100 = -4
		{[]byte{0xC0}, 4, -4},
		// 0x30 = 0011_0000, read 4 bits = 0011 = 3
		{[]byte{0x30}, 4, 3},
	}

	for _, tt := range tests {
		br := newFlacBitReader(tt.data)
		got, err := br.readSignedBits(tt.bits)
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Errorf("readSignedBits(%d) from %v: got %d, want %d",
				tt.bits, tt.data, got, tt.want)
		}
	}
}

func TestFlacBitReaderUnary(t *testing.T) {
	// 0x20 = 0010_0000 -> three zeros then a one = 2
	// Wait, unary counts zeros until a 1.
	// 0x20 = 0b00100000 -> bits are 0,0,1,... -> count=2
	br := newFlacBitReader([]byte{0x20})
	v, err := br.readUnary()
	if err != nil {
		t.Fatal(err)
	}
	if v != 2 {
		t.Errorf("readUnary: got %d, want 2", v)
	}

	// 0x80 = 1000_0000 -> first bit is 1 -> count=0
	br = newFlacBitReader([]byte{0x80})
	v, err = br.readUnary()
	if err != nil {
		t.Fatal(err)
	}
	if v != 0 {
		t.Errorf("readUnary: got %d, want 0", v)
	}

	// 0x04 = 0000_0100 -> five zeros then a one = 5
	br = newFlacBitReader([]byte{0x04})
	v, err = br.readUnary()
	if err != nil {
		t.Fatal(err)
	}
	if v != 5 {
		t.Errorf("readUnary: got %d, want 5", v)
	}
}

func TestFlacBitReaderAlignToByte(t *testing.T) {
	br := newFlacBitReader([]byte{0xAB, 0xCD})
	// Read 3 bits
	_, err := br.readBits(3)
	if err != nil {
		t.Fatal(err)
	}
	br.alignToByte()
	// Should now be aligned - next read should get 0xCD
	v, err := br.readBits(8)
	if err != nil {
		t.Fatal(err)
	}
	if v != 0xCD {
		t.Errorf("after align, readBits(8): got 0x%02X, want 0xCD", v)
	}
}

func TestDecodeRice(t *testing.T) {
	// Encode known values in Rice coding with param=2:
	// Value 5 -> zig-zag: 10 -> quotient=2, remainder=2
	//   unary 2: 00 1, remainder 2 bits: 10 -> 001 10
	// Value -3 -> zig-zag: 5 -> quotient=1, remainder=1
	//   unary 1: 0 1, remainder 2 bits: 01 -> 01 01
	// Value 0 -> zig-zag: 0 -> quotient=0, remainder=0
	//   unary 0: 1, remainder 2 bits: 00 -> 1 00
	// Bit stream: 00110 0101 100 + padding
	// 0011_0010_1100_0000 = 0x32C0
	br := newFlacBitReader([]byte{0x32, 0xC0})
	d := &flacDecoder{br: br}

	samples := make([]int32, 3)
	err := d.decodeRice(3, 2, samples)
	if err != nil {
		t.Fatal(err)
	}

	expected := []int32{5, -3, 0}
	for i, want := range expected {
		if samples[i] != want {
			t.Errorf("rice sample[%d]: got %d, want %d", i, samples[i], want)
		}
	}
}

func TestFixedPredict(t *testing.T) {
	tests := []struct {
		name  string
		order int
		input []int32
		want  []int32
	}{
		{
			name:  "order0",
			order: 0,
			input: []int32{10, 20, 30},
			want:  []int32{10, 20, 30},
		},
		{
			name:  "order1",
			order: 1,
			// warmup: [100], residuals: [5, -3, 10]
			// s[1] = 5 + s[0] = 105
			// s[2] = -3 + s[1] = 102
			// s[3] = 10 + s[2] = 112
			input: []int32{100, 5, -3, 10},
			want:  []int32{100, 105, 102, 112},
		},
		{
			name:  "order2",
			order: 2,
			// warmup: [10, 20], residuals: [0, 0]
			// s[2] = 0 + 2*20 - 10 = 30
			// s[3] = 0 + 2*30 - 20 = 40
			input: []int32{10, 20, 0, 0},
			want:  []int32{10, 20, 30, 40},
		},
		{
			name:  "order3",
			order: 3,
			// warmup: [1, 2, 3], residuals: [0]
			// s[3] = 0 + 3*3 - 3*2 + 1 = 4
			input: []int32{1, 2, 3, 0},
			want:  []int32{1, 2, 3, 4},
		},
		{
			name:  "order4",
			order: 4,
			// warmup: [1, 2, 3, 4], residuals: [0]
			// s[4] = 0 + 4*4 - 6*3 + 4*2 - 1 = 16-18+8-1 = 5
			input: []int32{1, 2, 3, 4, 0},
			want:  []int32{1, 2, 3, 4, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			samples := make([]int32, len(tt.input))
			copy(samples, tt.input)
			fixedPredict(tt.order, samples, len(samples))
			for i, want := range tt.want {
				if samples[i] != want {
					t.Errorf("sample[%d]: got %d, want %d", i, samples[i], want)
				}
			}
		})
	}
}

func TestLPCPredict(t *testing.T) {
	// Order 2, shift 4, coeffs [8, -4]
	// warmup: [100, 200], residuals: [5]
	// sum = 8*200 + (-4)*100 = 1600 - 400 = 1200
	// prediction = 1200 >> 4 = 75
	// sample[2] = 5 + 75 = 80
	samples := []int32{100, 200, 5}
	coeffs := []int32{8, -4}
	lpcPredict(2, 4, coeffs, samples, 3)

	if samples[2] != 80 {
		t.Errorf("lpcPredict sample[2]: got %d, want 80", samples[2])
	}
}

func TestLPCPredictMultiple(t *testing.T) {
	// Order 1, shift 0, coeffs [1] -> same as fixed order 1
	samples := []int32{10, 5, 3, -2}
	coeffs := []int32{1}
	lpcPredict(1, 0, coeffs, samples, 4)

	expected := []int32{10, 15, 18, 16}
	for i, want := range expected {
		if samples[i] != want {
			t.Errorf("sample[%d]: got %d, want %d", i, samples[i], want)
		}
	}
}

func TestDecorrelateLeftSide(t *testing.T) {
	d := &flacDecoder{}
	for i := range d.samples {
		d.samples[i] = make([]int32, 4)
	}

	// left-side: right = left - side
	d.samples[0] = []int32{100, 200, -50, 0}
	d.samples[1] = []int32{10, -20, 30, 0}

	err := d.decorrelate(8, 4)
	if err != nil {
		t.Fatal(err)
	}

	// left stays the same
	expectedLeft := []int32{100, 200, -50, 0}
	expectedRight := []int32{90, 220, -80, 0}

	for i := 0; i < 4; i++ {
		if d.samples[0][i] != expectedLeft[i] {
			t.Errorf("left[%d]: got %d, want %d", i, d.samples[0][i], expectedLeft[i])
		}
		if d.samples[1][i] != expectedRight[i] {
			t.Errorf("right[%d]: got %d, want %d", i, d.samples[1][i], expectedRight[i])
		}
	}
}

func TestDecorrelateRightSide(t *testing.T) {
	d := &flacDecoder{}
	for i := range d.samples {
		d.samples[i] = make([]int32, 3)
	}

	// right-side: left = side + right
	d.samples[0] = []int32{10, -20, 5}    // side
	d.samples[1] = []int32{100, 200, -50} // right

	err := d.decorrelate(9, 3)
	if err != nil {
		t.Fatal(err)
	}

	expectedLeft := []int32{110, 180, -45}
	expectedRight := []int32{100, 200, -50}

	for i := 0; i < 3; i++ {
		if d.samples[0][i] != expectedLeft[i] {
			t.Errorf("left[%d]: got %d, want %d", i, d.samples[0][i], expectedLeft[i])
		}
		if d.samples[1][i] != expectedRight[i] {
			t.Errorf("right[%d]: got %d, want %d", i, d.samples[1][i], expectedRight[i])
		}
	}
}

func TestDecorrelateMidSide(t *testing.T) {
	d := &flacDecoder{}
	for i := range d.samples {
		d.samples[i] = make([]int32, 2)
	}

	// mid-side: m = (m<<1)|(s&1), left=(m+s)>>1, right=(m-s)>>1
	// mid=50, side=10: m = 100|0 = 100, left=(100+10)>>1=55, right=(100-10)>>1=45
	d.samples[0] = []int32{50, 0}
	d.samples[1] = []int32{10, 0}

	err := d.decorrelate(10, 2)
	if err != nil {
		t.Fatal(err)
	}

	if d.samples[0][0] != 55 {
		t.Errorf("left[0]: got %d, want 55", d.samples[0][0])
	}
	if d.samples[1][0] != 45 {
		t.Errorf("right[0]: got %d, want 45", d.samples[1][0])
	}
}

func TestDecorrelateMidSideOdd(t *testing.T) {
	d := &flacDecoder{}
	for i := range d.samples {
		d.samples[i] = make([]int32, 1)
	}

	// Test with odd side value to verify the (s&1) correction
	// mid=50, side=11: m = 100|1 = 101, left=(101+11)>>1=56, right=(101-11)>>1=45
	d.samples[0] = []int32{50}
	d.samples[1] = []int32{11}

	err := d.decorrelate(10, 1)
	if err != nil {
		t.Fatal(err)
	}

	if d.samples[0][0] != 56 {
		t.Errorf("left[0]: got %d, want 56", d.samples[0][0])
	}
	if d.samples[1][0] != 45 {
		t.Errorf("right[0]: got %d, want 45", d.samples[1][0])
	}
}

func TestDecodeBlockSize(t *testing.T) {
	tests := []struct {
		code     uint32
		extra    []byte
		expected int
	}{
		{1, nil, 192},
		{2, nil, 576},
		{3, nil, 1152},
		{4, nil, 2304},
		{5, nil, 4608},
		{6, []byte{0xFF}, 256},       // 255 + 1
		{6, []byte{0x00}, 1},         // 0 + 1
		{7, []byte{0x01, 0xFF}, 512}, // 511 + 1
		{8, nil, 256},
		{9, nil, 512},
		{10, nil, 1024},
		{11, nil, 2048},
		{12, nil, 4096},
		{13, nil, 8192},
		{14, nil, 16384},
		{15, nil, 32768},
	}

	for _, tt := range tests {
		br := newFlacBitReader(tt.extra)
		got, err := decodeBlockSize(br, tt.code)
		if err != nil {
			t.Errorf("decodeBlockSize(%d): %v", tt.code, err)
			continue
		}
		if got != tt.expected {
			t.Errorf("decodeBlockSize(%d): got %d, want %d", tt.code, got, tt.expected)
		}
	}
}

func TestDecodeBlockSizeReserved(t *testing.T) {
	br := newFlacBitReader(nil)
	_, err := decodeBlockSize(br, 0)
	if err == nil {
		t.Error("expected error for reserved block size code 0")
	}
}

func TestSampleSizeLookup(t *testing.T) {
	tests := []struct {
		code uint32
		want int
	}{
		{0, 0},
		{1, 8},
		{2, 12},
		{3, 0},
		{4, 16},
		{5, 20},
		{6, 24},
		{7, 0},
	}

	for _, tt := range tests {
		got := sampleSizeLookup(tt.code)
		if got != tt.want {
			t.Errorf("sampleSizeLookup(%d): got %d, want %d", tt.code, got, tt.want)
		}
	}
}

func TestFlacBitReaderUTF8(t *testing.T) {
	// Single byte: 0x00 -> value 0
	br := newFlacBitReader([]byte{0x00})
	v, err := br.readUTF8()
	if err != nil {
		t.Fatal(err)
	}
	if v != 0 {
		t.Errorf("readUTF8 single byte: got %d, want 0", v)
	}

	// Single byte: 0x7F -> value 127
	br = newFlacBitReader([]byte{0x7F})
	v, err = br.readUTF8()
	if err != nil {
		t.Fatal(err)
	}
	if v != 127 {
		t.Errorf("readUTF8 single byte: got %d, want 127", v)
	}

	// Two byte: 0xC2 0x80 -> value 0x80 = 128
	br = newFlacBitReader([]byte{0xC2, 0x80})
	v, err = br.readUTF8()
	if err != nil {
		t.Fatal(err)
	}
	if v != 128 {
		t.Errorf("readUTF8 two byte: got %d, want 128", v)
	}

	// Three byte: 0xE0 0xA0 0x80 -> value 0x800 = 2048
	br = newFlacBitReader([]byte{0xE0, 0xA0, 0x80})
	v, err = br.readUTF8()
	if err != nil {
		t.Fatal(err)
	}
	if v != 2048 {
		t.Errorf("readUTF8 three byte: got %d, want 2048", v)
	}
}

func TestDecodeFLACEmpty(t *testing.T) {
	out, _, err := decodeFLAC(nil, 2, 16, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty output, got %d bytes", len(out))
	}
}
