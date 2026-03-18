package metadata

import (
	"testing"

	"github.com/user-none/eblitui/coreif"
)

func TestResolveConsoleID(t *testing.T) {
	variants := []coreif.MetadataVariant{
		{Name: "NGP", RDBName: "SNK - Neo Geo Pocket", ConsoleID: 0},
		{Name: "NGPC", RDBName: "SNK - Neo Geo Pocket Color", ConsoleID: 14},
	}
	m := NewMetadataManager(variants)

	tests := []struct {
		name       string
		variantIdx int
		defaultID  int
		want       int
	}{
		{"variant with no override uses default", 0, 11, 11},
		{"variant with override uses override", 1, 11, 14},
		{"negative index uses default", -1, 11, 11},
		{"out of range index uses default", 5, 11, 11},
		{"zero default with no override", 0, 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := m.ResolveConsoleID(tc.variantIdx, tc.defaultID)
			if got != tc.want {
				t.Errorf("ResolveConsoleID(%d, %d) = %d, want %d",
					tc.variantIdx, tc.defaultID, got, tc.want)
			}
		})
	}
}
