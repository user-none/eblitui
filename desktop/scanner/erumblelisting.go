// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package scanner

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	erumbleExt = ".erumble"

	// GitHub Contents API URL for the eblitui rumble file repository.
	// Each system has a directory (coreif.MetadataVariant.RumbleRepoDir)
	// holding files named by game ID: <gameid>.erumble, plus optional
	// per-disc <gameid>.disc<n>.erumble variants for disc systems. The
	// game ID is the ROM CRC32 (lowercase hex) for cartridge systems and
	// the disc serial for disc systems, matching the local storage names.
	erumbleContentsBaseURL = "https://api.github.com/repos/user-none/eblitui-rumble-files/contents"
)

// ERumbleListing holds the directory listing for one rumble repo
// directory. files maps a game ID to the repo filenames for that game:
// the shared <gameid>.erumble and any <gameid>.disc<n>.erumble variants.
// urls maps each filename to its API-provided download URL.
type ERumbleListing struct {
	files map[string][]string
	urls  map[string]string
}

// newERumbleListing creates an empty ERumbleListing.
func newERumbleListing() *ERumbleListing {
	return &ERumbleListing{
		files: make(map[string][]string),
		urls:  make(map[string]string),
	}
}

// addERumbleEntry adds a single repo filename to the listing. Names
// without the .erumble extension, or containing a path separator or
// parent reference, are ignored: filenames become local save paths, so
// only plain names are accepted.
func (el *ERumbleListing) addERumbleEntry(name, downloadURL string) {
	if !strings.HasSuffix(name, erumbleExt) {
		return
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return
	}

	base := name[:len(name)-len(erumbleExt)]
	if base == "" {
		return
	}

	// A per-disc file keys under the shared game ID.
	gameID := base
	if i := strings.LastIndex(base, ".disc"); i > 0 && isDigits(base[i+len(".disc"):]) {
		gameID = base[:i]
	}

	el.files[gameID] = append(el.files[gameID], name)
	el.urls[name] = downloadURL
}

// isDigits reports whether s is a non-empty ASCII digit string.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// resolveERumbleFiles returns the repo filenames and download URLs for
// a game ID: the shared file and any per-disc variants. Returns nil
// when the listing has no files for the game.
func resolveERumbleFiles(listing *ERumbleListing, gameID string) []contentsEntry {
	if listing == nil || gameID == "" {
		return nil
	}
	var out []contentsEntry
	for _, name := range listing.files[gameID] {
		out = append(out, contentsEntry{Name: name, DownloadURL: listing.urls[name]})
	}
	return out
}

// fetchERumbleListing fetches the .erumble file listing for one rumble
// repo directory using the GitHub Contents API. Rumble files only exist
// in a per-system subdirectory declared by a metadata variant, never at
// the repository root, so an empty repoDir fetches nothing. Returns nil
// when the directory does not exist (404, including the repository not
// existing yet) or on any other error.
func fetchERumbleListing(repoDir string) *ERumbleListing {
	if repoDir == "" {
		return nil
	}

	contentsURL := fmt.Sprintf("%s/%s", erumbleContentsBaseURL, url.PathEscape(repoDir))

	entries, err := fetchContentsListing(contentsURL)
	if err != nil {
		return nil
	}

	listing := newERumbleListing()
	for _, entry := range entries {
		if entry.Type != "file" || entry.DownloadURL == "" {
			continue
		}
		listing.addERumbleEntry(entry.Name, entry.DownloadURL)
	}

	return listing
}
