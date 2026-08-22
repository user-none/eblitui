// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package scanner

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/user-none/eblitui/desktop/netutil"
)

const (
	rumbleSuffix = " (Rumbles).cht"

	// GitHub Contents API URL for the libretro-database cht directory
	chtContentsBaseURL = "https://api.github.com/repos/libretro/libretro-database/contents/cht"
)

// RumbleListing holds the directory listing for a single variant's rumble
// CHT files. Exact maps the &->_ filename (without suffix) to the original
// filename. ByNorm maps normalized names for fallback matching.
type RumbleListing struct {
	Exact  map[string]string // &->_ name -> original name (no suffix)
	ByNorm map[string]string // normalized name -> original name (no suffix)
}

// newRumbleListing creates an empty RumbleListing.
func newRumbleListing() *RumbleListing {
	return &RumbleListing{
		Exact:  make(map[string]string),
		ByNorm: make(map[string]string),
	}
}

// addRumbleEntry adds a single filename entry to the listing.
// name should be the full filename (e.g. "Sonic (Rumbles).cht").
func (rl *RumbleListing) addRumbleEntry(name string) {
	lower := strings.ToLower(name)
	if !strings.HasSuffix(lower, strings.ToLower(rumbleSuffix)) {
		return
	}

	// Strip the suffix to get the base name
	base := name[:len(name)-len(rumbleSuffix)]

	// Exact map: & replaced with _
	exactKey := strings.ReplaceAll(base, "&", "_")
	rl.Exact[exactKey] = base

	// Normalized map
	norm := normalizeName(base)
	if norm != "" {
		rl.ByNorm[norm] = base
	}
}

// resolveRumbleName tries to find a matching rumble filename in the listing.
// It tries exact match (&->_) first, then normalized fallback.
// Returns the original base name and true, or empty and false.
func resolveRumbleName(listing *RumbleListing, gameName string) (string, bool) {
	if listing == nil || gameName == "" {
		return "", false
	}

	// Exact match: replace & with _ in the game name
	exactKey := strings.ReplaceAll(gameName, "&", "_")
	if orig, ok := listing.Exact[exactKey]; ok {
		return orig, true
	}

	// Normalized fallback
	norm := normalizeName(gameName)
	if norm == "" {
		return "", false
	}
	if orig, ok := listing.ByNorm[norm]; ok {
		return orig, true
	}

	return "", false
}

// contentsEntry is a single entry from the GitHub Contents API. The
// Contents API caps a directory at 1000 entries, which is acceptable
// here: a per-system cht directory holds far fewer files.
type contentsEntry struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
}

// fetchContentsListing fetches a directory listing from the GitHub
// Contents API. Returns an error on any failure including 404.
func fetchContentsListing(contentsURL string) ([]contentsEntry, error) {
	req, err := http.NewRequest(http.MethodGet, contentsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := netutil.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var entries []contentsEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse contents response: %w", err)
	}

	return entries, nil
}

// fetchRumbleListing fetches the rumble CHT file listing for a variant
// using the GitHub Contents API. A single call fetches the system-specific
// directory directly (e.g. cht/Sega - Mega Drive - Genesis). If the
// directory does not exist (404) or any error occurs, returns nil.
func fetchRumbleListing(rdbName string) *RumbleListing {
	contentsURL := fmt.Sprintf("%s/%s", chtContentsBaseURL, url.PathEscape(rdbName))

	entries, err := fetchContentsListing(contentsURL)
	if err != nil {
		return nil
	}

	listing := newRumbleListing()
	for _, entry := range entries {
		if entry.Type != "file" {
			continue
		}
		listing.addRumbleEntry(entry.Name)
	}

	return listing
}
