package achievements

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"testing"
)

// encodePNG returns a PNG of the given size.
func encodePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

// withPNGSize returns a copy of a PNG whose header claims the given size,
// with the header checksum updated so only the size is wrong.
func withPNGSize(data []byte, width, height uint32) []byte {
	out := append([]byte(nil), data...)
	// The IHDR chunk follows the 8-byte signature: length (4), type (4),
	// width (4), height (4), five more bytes, then the CRC over type and
	// data.
	binary.BigEndian.PutUint32(out[16:20], width)
	binary.BigEndian.PutUint32(out[20:24], height)
	binary.BigEndian.PutUint32(out[29:33], crc32.ChecksumIEEE(out[12:29]))
	return out
}

// TestDecodeBadge tests that images within the size limit decode and that
// larger ones are refused from their header.
func TestDecodeBadge(t *testing.T) {
	small := encodePNG(t, 64, 64)
	cases := []struct {
		name   string
		data   []byte
		wantOK bool
	}{
		{"64x64 badge", small, true},
		{"at the limit", encodePNG(t, maxBadgeDimension, 1), true},
		{"wide", withPNGSize(small, maxBadgeDimension+1, 1), false},
		{"tall", withPNGSize(small, 1, maxBadgeDimension+1), false},
		{"header claims 60000x60000", withPNGSize(small, 60000, 60000), false},
		{"not an image", []byte("not an image"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img, err := decodeBadge(tc.data)
			if !tc.wantOK {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeBadge: %v", err)
			}
			if img == nil {
				t.Fatal("decodeBadge returned no image")
			}
		})
	}
}
