// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import "testing"

func TestCenterScrollFraction(t *testing.T) {
	tests := []struct {
		name     string
		itemY    int
		itemH    int
		total    int
		viewport int
		want     float64
	}{
		// 1000 px of content in a 200 px viewport: 800 px of scroll range
		{"top", 0, 20, 1000, 200, 0},
		{"near top clamps to 0", 60, 20, 1000, 200, 0},
		{"middle", 490, 20, 1000, 200, 400.0 / 800},
		{"deep", 890, 20, 1000, 200, 800.0 / 800},
		{"bottom", 980, 20, 1000, 200, 1},
		{"before bottom", 790, 20, 1000, 200, 700.0 / 800},
		{"content fits", 100, 20, 150, 200, 0},
		{"empty", 0, 20, 0, 200, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := centerScrollFraction(tt.itemY, tt.itemH, tt.total, tt.viewport)
			if got != tt.want {
				t.Errorf("centerScrollFraction(%d, %d, %d, %d) = %v, want %v",
					tt.itemY, tt.itemH, tt.total, tt.viewport, got, tt.want)
			}
		})
	}
}
