package chd

import "testing"

// TestCRC16CheckValue tests the CRC against the standard check input for
// polynomial 0x1021 with initial value 0xFFFF, no reflection and no final
// XOR.
func TestCRC16CheckValue(t *testing.T) {
	if got := crc16([]byte("123456789")); got != 0x29B1 {
		t.Fatalf("crc16 = %#04x, want 0x29b1", got)
	}
	if got := crc16(nil); got != 0xFFFF {
		t.Fatalf("crc16(nil) = %#04x, want 0xffff", got)
	}
}
