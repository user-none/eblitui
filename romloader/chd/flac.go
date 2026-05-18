package chd

import (
	"errors"
	"fmt"
)

// decodeFLAC decodes raw FLAC frames (no stream header) from src and returns
// interleaved PCM bytes. channels and bitsPerSample are provided by the CHD
// context rather than parsed from a STREAMINFO block.
// If maxSamples > 0, decoding stops after that many samples (per channel)
// have been produced. Returns the decoded bytes and the number of input
// bytes consumed.
func decodeFLAC(src []byte, channels int, bitsPerSample int, maxSamples int) ([]byte, int, error) {
	if len(src) == 0 {
		return nil, 0, nil
	}

	d := &flacDecoder{
		br:            newFlacBitReader(src),
		channels:      channels,
		bitsPerSample: bitsPerSample,
	}
	for i := range d.samples {
		d.samples[i] = make([]int32, 0, 2048)
	}

	bytesPerSample := (bitsPerSample + 7) / 8
	var out []byte
	totalSamples := 0

	for d.br.pos < len(d.br.data) {
		if maxSamples > 0 && totalSamples >= maxSamples {
			break
		}
		blockSize, err := d.decodeFrame()
		if err != nil {
			return nil, 0, err
		}

		for s := 0; s < blockSize; s++ {
			for ch := 0; ch < channels; ch++ {
				sample := d.samples[ch][s]
				switch bytesPerSample {
				case 1:
					out = append(out, byte(sample))
				case 2:
					out = append(out, byte(sample), byte(sample>>8))
				case 3:
					out = append(out, byte(sample), byte(sample>>8), byte(sample>>16))
				}
			}
		}
		totalSamples += blockSize
	}

	// Consumed bytes: pos minus any buffered-but-unused bytes
	consumed := d.br.pos - (d.br.nBits / 8)
	return out, consumed, nil
}

type flacDecoder struct {
	br            *flacBitReader
	channels      int
	bitsPerSample int
	samples       [8][]int32
}

type flacBitReader struct {
	data  []byte
	pos   int
	bits  uint64
	nBits int
}

func newFlacBitReader(data []byte) *flacBitReader {
	return &flacBitReader{data: data}
}

func (r *flacBitReader) refill() {
	for r.nBits <= 56 && r.pos < len(r.data) {
		r.bits = (r.bits << 8) | uint64(r.data[r.pos])
		r.pos++
		r.nBits += 8
	}
}

func (r *flacBitReader) readBits(n int) (uint32, error) {
	if n == 0 {
		return 0, nil
	}
	r.refill()
	if r.nBits < n {
		return 0, errors.New("flac: unexpected end of data")
	}
	r.nBits -= n
	return uint32((r.bits >> r.nBits) & ((1 << n) - 1)), nil
}

func (r *flacBitReader) readSignedBits(n int) (int32, error) {
	v, err := r.readBits(n)
	if err != nil {
		return 0, err
	}
	// Sign extend
	if n > 0 && v&(1<<(n-1)) != 0 {
		v |= ^uint32(0) << n
	}
	return int32(v), nil
}

func (r *flacBitReader) readUnary() (uint32, error) {
	var count uint32
	for {
		bit, err := r.readBits(1)
		if err != nil {
			return 0, err
		}
		if bit != 0 {
			return count, nil
		}
		count++
	}
}

func (r *flacBitReader) readUTF8() (uint64, error) {
	b, err := r.readBits(8)
	if err != nil {
		return 0, err
	}
	first := byte(b)
	var val uint64
	var extra int

	switch {
	case first&0x80 == 0:
		val = uint64(first)
	case first&0xE0 == 0xC0:
		val = uint64(first & 0x1F)
		extra = 1
	case first&0xF0 == 0xE0:
		val = uint64(first & 0x0F)
		extra = 2
	case first&0xF8 == 0xF0:
		val = uint64(first & 0x07)
		extra = 3
	case first&0xFC == 0xF8:
		val = uint64(first & 0x03)
		extra = 4
	case first&0xFE == 0xFC:
		val = uint64(first & 0x01)
		extra = 5
	case first == 0xFE:
		val = 0
		extra = 6
	default:
		return 0, fmt.Errorf("flac: invalid UTF-8 lead byte 0x%02X", first)
	}

	for i := 0; i < extra; i++ {
		b, err := r.readBits(8)
		if err != nil {
			return 0, err
		}
		if b&0xC0 != 0x80 {
			return 0, fmt.Errorf("flac: invalid UTF-8 continuation byte 0x%02X", b)
		}
		val = (val << 6) | uint64(b&0x3F)
	}

	return val, nil
}

func (r *flacBitReader) alignToByte() {
	discard := r.nBits % 8
	r.nBits -= discard
}

func (d *flacDecoder) decodeFrame() (int, error) {
	// Sync code: 14 bits = 0x3FFE
	sync, err := d.br.readBits(14)
	if err != nil {
		return 0, err
	}
	if sync != 0x3FFE {
		return 0, fmt.Errorf("flac: invalid sync code 0x%04X", sync)
	}

	// Reserved bit
	reserved, err := d.br.readBits(1)
	if err != nil {
		return 0, err
	}
	if reserved != 0 {
		return 0, errors.New("flac: reserved bit is not zero")
	}

	// Blocking strategy
	_, err = d.br.readBits(1) // blocking strategy (0=fixed, 1=variable)
	if err != nil {
		return 0, err
	}

	// Block size code
	blockSizeCode, err := d.br.readBits(4)
	if err != nil {
		return 0, err
	}

	// Sample rate code
	sampleRateCode, err := d.br.readBits(4)
	if err != nil {
		return 0, err
	}

	// Channel assignment
	channelAssign, err := d.br.readBits(4)
	if err != nil {
		return 0, err
	}

	// Sample size code
	sampleSizeCode, err := d.br.readBits(3)
	if err != nil {
		return 0, err
	}

	// Reserved bit
	reserved2, err := d.br.readBits(1)
	if err != nil {
		return 0, err
	}
	if reserved2 != 0 {
		return 0, errors.New("flac: reserved bit 2 is not zero")
	}

	// UTF-8 coded frame/sample number (skip value)
	_, err = d.br.readUTF8()
	if err != nil {
		return 0, err
	}

	// Block size
	blockSize, err := decodeBlockSize(d.br, blockSizeCode)
	if err != nil {
		return 0, err
	}

	// Optional sample rate (consume extra bytes for codes 12-14)
	if err := skipSampleRate(d.br, sampleRateCode); err != nil {
		return 0, err
	}

	// CRC-8 (skip validation)
	_, err = d.br.readBits(8)
	if err != nil {
		return 0, err
	}

	// Determine bits per sample
	bps := d.bitsPerSample
	if sampleSizeCode != 0 {
		bps = sampleSizeLookup(sampleSizeCode)
		if bps == 0 {
			return 0, fmt.Errorf("flac: invalid sample size code %d", sampleSizeCode)
		}
	}

	// Determine channel count and decode subframes
	numChannels := d.channels
	if channelAssign <= 7 {
		numChannels = int(channelAssign) + 1
	}

	for ch := 0; ch < numChannels; ch++ {
		chBps := bps
		// Side channel gets +1 bps
		if channelAssign == 8 && ch == 1 { // left-side: side is ch1
			chBps++
		} else if channelAssign == 9 && ch == 0 { // right-side: side is ch0
			chBps++
		} else if channelAssign == 10 && ch == 1 { // mid-side: side is ch1
			chBps++
		}

		if err := d.decodeSubframe(ch, blockSize, chBps); err != nil {
			return 0, fmt.Errorf("flac: subframe ch%d: %w", ch, err)
		}
	}

	// Channel decorrelation
	if err := d.decorrelate(channelAssign, blockSize); err != nil {
		return 0, err
	}

	// Byte align
	d.br.alignToByte()

	// Frame footer: CRC-16 (skip validation)
	_, err = d.br.readBits(16)
	if err != nil {
		return 0, err
	}

	return blockSize, nil
}

func decodeBlockSize(br *flacBitReader, code uint32) (int, error) {
	switch {
	case code == 0:
		return 0, errors.New("flac: reserved block size code 0")
	case code == 1:
		return 192, nil
	case code >= 2 && code <= 5:
		return 576 << (code - 2), nil
	case code == 6:
		v, err := br.readBits(8)
		if err != nil {
			return 0, err
		}
		return int(v) + 1, nil
	case code == 7:
		v, err := br.readBits(16)
		if err != nil {
			return 0, err
		}
		return int(v) + 1, nil
	default: // 8-15
		return 256 << (code - 8), nil
	}
}

func skipSampleRate(br *flacBitReader, code uint32) error {
	switch code {
	case 12:
		// 8-bit sample rate (kHz)
		_, err := br.readBits(8)
		return err
	case 13:
		// 16-bit sample rate (Hz)
		_, err := br.readBits(16)
		return err
	case 14:
		// 16-bit sample rate (tens of Hz)
		_, err := br.readBits(16)
		return err
	}
	return nil
}

func sampleSizeLookup(code uint32) int {
	switch code {
	case 1:
		return 8
	case 2:
		return 12
	case 4:
		return 16
	case 5:
		return 20
	case 6:
		return 24
	default:
		return 0 // reserved
	}
}

func (d *flacDecoder) decodeSubframe(ch int, blockSize int, bps int) error {
	// Zero padding bit
	pad, err := d.br.readBits(1)
	if err != nil {
		return err
	}
	if pad != 0 {
		return errors.New("flac: subframe padding bit is not zero")
	}

	// Subframe type (6 bits)
	sfType, err := d.br.readBits(6)
	if err != nil {
		return err
	}

	// Wasted bits flag
	wastedFlag, err := d.br.readBits(1)
	if err != nil {
		return err
	}
	wastedBits := 0
	if wastedFlag != 0 {
		wb, err := d.br.readUnary()
		if err != nil {
			return err
		}
		wastedBits = int(wb) + 1
		bps -= wastedBits
	}

	// Ensure sample buffer capacity
	if cap(d.samples[ch]) < blockSize {
		d.samples[ch] = make([]int32, blockSize)
	} else {
		d.samples[ch] = d.samples[ch][:blockSize]
	}
	samples := d.samples[ch]

	switch {
	case sfType == 0:
		// CONSTANT
		val, err := d.br.readSignedBits(bps)
		if err != nil {
			return err
		}
		for i := 0; i < blockSize; i++ {
			samples[i] = val
		}
	case sfType == 1:
		// VERBATIM
		for i := 0; i < blockSize; i++ {
			val, err := d.br.readSignedBits(bps)
			if err != nil {
				return err
			}
			samples[i] = val
		}
	case sfType >= 8 && sfType <= 12:
		// FIXED prediction, order 0-4
		order := int(sfType - 8)
		for i := 0; i < order; i++ {
			val, err := d.br.readSignedBits(bps)
			if err != nil {
				return err
			}
			samples[i] = val
		}
		if err := d.decodeResidual(blockSize, order, samples); err != nil {
			return err
		}
		fixedPredict(order, samples, blockSize)
	case sfType >= 32 && sfType <= 63:
		// LPC prediction, order 1-32
		order := int(sfType - 31)
		for i := 0; i < order; i++ {
			val, err := d.br.readSignedBits(bps)
			if err != nil {
				return err
			}
			samples[i] = val
		}
		// QLP precision
		qlpPrec, err := d.br.readBits(4)
		if err != nil {
			return err
		}
		if qlpPrec == 15 {
			return errors.New("flac: invalid QLP precision 15")
		}
		qlpPrecision := int(qlpPrec) + 1

		// QLP shift (signed 5-bit)
		qlpShift, err := d.br.readSignedBits(5)
		if err != nil {
			return err
		}
		if qlpShift < 0 {
			return fmt.Errorf("flac: negative QLP shift %d", qlpShift)
		}

		// Coefficients
		coeffs := make([]int32, order)
		for i := 0; i < order; i++ {
			c, err := d.br.readSignedBits(qlpPrecision)
			if err != nil {
				return err
			}
			coeffs[i] = c
		}

		if err := d.decodeResidual(blockSize, order, samples); err != nil {
			return err
		}
		lpcPredict(order, int(qlpShift), coeffs, samples, blockSize)
	default:
		return fmt.Errorf("flac: reserved subframe type %d", sfType)
	}

	// Shift back for wasted bits
	if wastedBits > 0 {
		for i := 0; i < blockSize; i++ {
			samples[i] <<= wastedBits
		}
	}

	return nil
}

func (d *flacDecoder) decodeResidual(blockSize int, predictorOrder int, samples []int32) error {
	// Coding method
	method, err := d.br.readBits(2)
	if err != nil {
		return err
	}
	if method > 1 {
		return fmt.Errorf("flac: reserved residual coding method %d", method)
	}

	paramBits := 4
	escapeCode := 15
	if method == 1 {
		paramBits = 5
		escapeCode = 31
	}

	// Partition order
	partOrder, err := d.br.readBits(4)
	if err != nil {
		return err
	}
	numPartitions := 1 << partOrder
	samplesPerPartition := blockSize >> partOrder

	sampleIdx := predictorOrder
	for p := 0; p < numPartitions; p++ {
		param, err := d.br.readBits(paramBits)
		if err != nil {
			return err
		}

		var count int
		if p == 0 {
			count = samplesPerPartition - predictorOrder
		} else {
			count = samplesPerPartition
		}

		if int(param) == escapeCode {
			// Escape: raw encoding
			rawBits, err := d.br.readBits(5)
			if err != nil {
				return err
			}
			for i := 0; i < count; i++ {
				val, err := d.br.readSignedBits(int(rawBits))
				if err != nil {
					return err
				}
				samples[sampleIdx] = val
				sampleIdx++
			}
		} else {
			// Rice coded
			if err := d.decodeRice(count, int(param), samples[sampleIdx:]); err != nil {
				return err
			}
			sampleIdx += count
		}
	}

	return nil
}

func (d *flacDecoder) decodeRice(count int, param int, samples []int32) error {
	for i := 0; i < count; i++ {
		// Quotient: count zero bits until a 1
		q, err := d.br.readUnary()
		if err != nil {
			return err
		}

		// Remainder
		var r uint32
		if param > 0 {
			r, err = d.br.readBits(param)
			if err != nil {
				return err
			}
		}

		val := (q << param) | r
		// Zig-zag decode
		if val&1 != 0 {
			samples[i] = -int32(val>>1) - 1
		} else {
			samples[i] = int32(val >> 1)
		}
	}
	return nil
}

// fixedPredict applies FIXED prediction to reconstruct samples in place.
// samples[0..order-1] are warm-up values, samples[order..blockSize-1] hold
// residuals that get replaced with reconstructed values.
func fixedPredict(order int, samples []int32, blockSize int) {
	switch order {
	case 0:
		// Nothing to do
	case 1:
		for i := order; i < blockSize; i++ {
			samples[i] += samples[i-1]
		}
	case 2:
		for i := order; i < blockSize; i++ {
			samples[i] += 2*samples[i-1] - samples[i-2]
		}
	case 3:
		for i := order; i < blockSize; i++ {
			samples[i] += 3*samples[i-1] - 3*samples[i-2] + samples[i-3]
		}
	case 4:
		for i := order; i < blockSize; i++ {
			samples[i] += 4*samples[i-1] - 6*samples[i-2] + 4*samples[i-3] - samples[i-4]
		}
	}
}

// lpcPredict applies LPC prediction to reconstruct samples in place.
// Uses int64 accumulator to avoid overflow.
func lpcPredict(order int, shift int, coeffs []int32, samples []int32, blockSize int) {
	for i := order; i < blockSize; i++ {
		var sum int64
		for j := 0; j < order; j++ {
			sum += int64(coeffs[j]) * int64(samples[i-1-j])
		}
		samples[i] += int32(sum >> shift)
	}
}

func (d *flacDecoder) decorrelate(channelAssign uint32, blockSize int) error {
	switch channelAssign {
	case 8:
		// Left-side stereo: right = left - side
		left := d.samples[0]
		side := d.samples[1]
		for i := 0; i < blockSize; i++ {
			side[i] = left[i] - side[i]
		}
	case 9:
		// Right-side stereo: left = side + right
		side := d.samples[0]
		right := d.samples[1]
		for i := 0; i < blockSize; i++ {
			side[i] = side[i] + right[i]
		}
	case 10:
		// Mid-side stereo
		mid := d.samples[0]
		side := d.samples[1]
		for i := 0; i < blockSize; i++ {
			m := mid[i]
			s := side[i]
			m = (m << 1) | (s & 1)
			mid[i] = (m + s) >> 1
			side[i] = (m - s) >> 1
		}
	default:
		// Independent channels or single channel: no transform
	}
	return nil
}
