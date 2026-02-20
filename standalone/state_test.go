//go:build !libretro

package standalone

import "testing"

func TestAppStateString(t *testing.T) {
	tests := []struct {
		state    AppState
		expected string
	}{
		{StateLibrary, "Library"},
		{StateDetail, "Detail"},
		{StateSettings, "Settings"},
		{StateScanProgress, "ScanProgress"},
		{StateError, "Error"},
		{StatePlaying, "Playing"},
		{AppState(99), "Unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			got := tc.state.String()
			if got != tc.expected {
				t.Errorf("AppState(%d).String() = %q, want %q", tc.state, got, tc.expected)
			}
		})
	}
}
