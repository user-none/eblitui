// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package chd

// crc16 returns the CRC-16 CHD images use for the hunk map and for each
// hunk: polynomial 0x1021, initial value 0xFFFF, most significant bit
// first, no final XOR.
func crc16(data []byte) uint16 {
	return crc16Update(0xFFFF, data)
}

// crc16Update continues a CRC-16 over more data.
func crc16Update(crc uint16, data []byte) uint16 {
	for _, b := range data {
		crc ^= uint16(b) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
