package scanner

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/user-none/eblitui/standalone/netutil"
)

// contentsEntry is a single entry from the GitHub Contents API.
type contentsEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// fetchContentsListing fetches a directory listing from the GitHub Contents
// API and returns the entries. Returns an error on any failure including 404.
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

// fetchArtworkTypeListing fetches the directory listing for a single artwork
// type from a thumbnail repo using the GitHub Contents API. Returns a
// ThumbnailListing containing only entries for the requested artType.
// Returns nil if the repo is empty or the fetch fails.
func fetchArtworkTypeListing(repo string, artType string) *ThumbnailListing {
	contentsURL := fmt.Sprintf(
		"https://api.github.com/repos/libretro-thumbnails/%s/contents/%s",
		url.PathEscape(repo), artType)

	entries, err := fetchContentsListing(contentsURL)
	if err != nil {
		return nil
	}

	listing := newThumbnailListing()
	for _, entry := range entries {
		if entry.Type != "file" {
			continue
		}
		name := entry.Name
		if strings.HasSuffix(strings.ToLower(name), ".png") {
			name = name[:len(name)-4]
		}
		listing.addEntry(artType, name)
	}

	return listing
}
