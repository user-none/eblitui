// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"strings"
	"testing"

	"github.com/user-none/eblitui/coreif"
)

// testOptions declares options across two categories with an
// ungrouped option after a grouped one and two groups interleaved, so
// the display order differs from the declared order.
var testOptions = []coreif.CoreOption{
	{Key: "v1", Category: coreif.CoreOptionCategoryVideo},
	{Key: "v2", Category: coreif.CoreOptionCategoryVideo, Group: "Enhancements"},
	{Key: "c1", Category: coreif.CoreOptionCategoryCore},
	{Key: "v3", Category: coreif.CoreOptionCategoryVideo, Group: "Timing"},
	{Key: "v4", Category: coreif.CoreOptionCategoryVideo},
	{Key: "v5", Category: coreif.CoreOptionCategoryVideo, Group: "Enhancements"},
	{Key: "c2", Category: coreif.CoreOptionCategoryCore, Group: "Performance"},
}

func keysOf(opts []coreif.CoreOption) string {
	var keys []string
	for _, o := range opts {
		keys = append(keys, o.Key)
	}
	return strings.Join(keys, " ")
}

func TestCategoryOptionsOrder(t *testing.T) {
	// Ungrouped first in declared order, then Enhancements (first
	// declared group) with both its options, then Timing.
	if got := keysOf(CategoryOptions(testOptions, coreif.CoreOptionCategoryVideo)); got != "v1 v4 v2 v5 v3" {
		t.Fatalf("video order %q", got)
	}
	if got := keysOf(CategoryOptions(testOptions, coreif.CoreOptionCategoryCore)); got != "c1 c2" {
		t.Fatalf("core order %q", got)
	}
	if got := CategoryOptions(testOptions, coreif.CoreOptionCategoryAudio); len(got) != 0 {
		t.Fatalf("audio options %v", got)
	}
}

func TestCoreOptionFocusKeysFollowDisplayOrder(t *testing.T) {
	got := strings.Join(coreOptionFocusKeys(testOptions, coreif.CoreOptionCategoryVideo, "video"), " ")
	want := "video-opt-v1 video-opt-v4 video-opt-v2 video-opt-v5 video-opt-v3"
	if got != want {
		t.Fatalf("focus keys %q, want %q", got, want)
	}
	if got := coreOptionFocusKeys(testOptions, coreif.CoreOptionCategoryAudio, "audio"); len(got) != 0 {
		t.Fatalf("audio focus keys %v", got)
	}
}
