// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package scanner

import (
	"testing"

	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/metadata"
)

// repoURL builds a repository download URL for tests. Rumble files only
// exist in a per-system subdirectory, never at the repository root.
func repoURL(name string) string {
	return "https://raw.githubusercontent.com/user-none/eblitui-rumble-files/main/md/" + name
}

func TestERumbleListingExactMatch(t *testing.T) {
	el := newERumbleListing()
	el.addERumbleEntry("1b0f8b12.erumble", repoURL("1b0f8b12.erumble"))

	files := resolveERumbleFiles(el, "1b0f8b12")
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Name != "1b0f8b12.erumble" {
		t.Errorf("unexpected name %q", files[0].Name)
	}
	if files[0].DownloadURL != repoURL("1b0f8b12.erumble") {
		t.Errorf("unexpected download URL %q", files[0].DownloadURL)
	}
}

func TestERumbleListingDiscVariants(t *testing.T) {
	el := newERumbleListing()
	el.addERumbleEntry("T-14310G.erumble", repoURL("T-14310G.erumble"))
	el.addERumbleEntry("T-14310G.disc1.erumble", repoURL("T-14310G.disc1.erumble"))
	el.addERumbleEntry("T-14310G.disc2.erumble", repoURL("T-14310G.disc2.erumble"))
	el.addERumbleEntry("T-99999G.erumble", repoURL("T-99999G.erumble"))

	files := resolveERumbleFiles(el, "T-14310G")
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}
	names := make(map[string]bool)
	for _, f := range files {
		names[f.Name] = true
	}
	for _, want := range []string{
		"T-14310G.erumble",
		"T-14310G.disc1.erumble",
		"T-14310G.disc2.erumble",
	} {
		if !names[want] {
			t.Errorf("missing file %q", want)
		}
	}
}

func TestERumbleListingDiscOnly(t *testing.T) {
	el := newERumbleListing()
	el.addERumbleEntry("T-14310G.disc2.erumble", repoURL("T-14310G.disc2.erumble"))

	files := resolveERumbleFiles(el, "T-14310G")
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Name != "T-14310G.disc2.erumble" {
		t.Errorf("unexpected name %q", files[0].Name)
	}
}

func TestERumbleListingNonDiscSuffixKeepsFullID(t *testing.T) {
	el := newERumbleListing()
	el.addERumbleEntry("game.discx.erumble", repoURL("game.discx.erumble"))

	if files := resolveERumbleFiles(el, "game"); len(files) != 0 {
		t.Errorf("non-numeric disc suffix should not key under the base ID")
	}
	if files := resolveERumbleFiles(el, "game.discx"); len(files) != 1 {
		t.Errorf("expected full name to be its own game ID")
	}
}

func TestERumbleListingIgnoresInvalidEntries(t *testing.T) {
	el := newERumbleListing()
	el.addERumbleEntry("readme.md", repoURL("readme.md"))
	el.addERumbleEntry(".erumble", repoURL(".erumble"))
	el.addERumbleEntry("../evil.erumble", repoURL("../evil.erumble"))
	el.addERumbleEntry("a/b.erumble", repoURL("a/b.erumble"))
	el.addERumbleEntry("a\\b.erumble", repoURL("a\\b.erumble"))

	if len(el.files) != 0 {
		t.Errorf("expected no entries, got %d", len(el.files))
	}
}

func TestFetchERumbleListingEmptyDirFetchesNothing(t *testing.T) {
	if l := fetchERumbleListing(""); l != nil {
		t.Errorf("empty repo dir must not produce a repository root listing")
	}
}

func TestHasERumbleRepoDir(t *testing.T) {
	md := metadata.NewMetadataManager([]coreif.MetadataVariant{
		{Name: "With", RumbleRepoDir: "md"},
		{Name: "Without"},
	})
	s := NewScanner(nil, nil, nil, false, nil, md, 0, false, nil)

	if !s.hasERumbleRepoDir(0) {
		t.Errorf("variant with a repo dir should support rumble downloads")
	}
	if s.hasERumbleRepoDir(1) {
		t.Errorf("variant without a repo dir must not support rumble downloads")
	}
	if !s.hasERumbleRepoDir(-1) {
		t.Errorf("non-RDB games should queue when any variant has a repo dir")
	}

	none := metadata.NewMetadataManager([]coreif.MetadataVariant{
		{Name: "Without"},
	})
	sn := NewScanner(nil, nil, nil, false, nil, none, 0, false, nil)

	if sn.hasERumbleRepoDir(0) || sn.hasERumbleRepoDir(-1) {
		t.Errorf("core declaring no repo dirs must not support rumble downloads")
	}
}

func TestERumbleListingNoMatch(t *testing.T) {
	el := newERumbleListing()
	el.addERumbleEntry("1b0f8b12.erumble", repoURL("1b0f8b12.erumble"))

	if files := resolveERumbleFiles(el, "deadbeef"); files != nil {
		t.Errorf("expected nil for unknown game ID")
	}
	if files := resolveERumbleFiles(el, ""); files != nil {
		t.Errorf("expected nil for empty game ID")
	}
	if files := resolveERumbleFiles(nil, "1b0f8b12"); files != nil {
		t.Errorf("expected nil for nil listing")
	}
}
