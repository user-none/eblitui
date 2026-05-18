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

// treeEntry is a single entry from the GitHub Git Trees API.
type treeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"` // "blob" (file) or "tree" (directory)
	SHA  string `json:"sha"`
}

// treeResponse is the GitHub Git Trees API response. Truncated is true
// only when a directory exceeds the API's very large entry limit, which
// thumbnail directories do not reach.
type treeResponse struct {
	Tree      []treeEntry `json:"tree"`
	Truncated bool        `json:"truncated"`
}

// fetchTree fetches a Git tree from the GitHub API. Unlike the Contents
// API (capped at 1000 entries per directory with no pagination), the
// Trees API returns the full directory listing. Returns an error on any
// failure including 404.
func fetchTree(treeURL string) (*treeResponse, error) {
	req, err := http.NewRequest(http.MethodGet, treeURL, nil)
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

	var tr treeResponse
	if err := json.Unmarshal(data, &tr); err != nil {
		return nil, fmt.Errorf("failed to parse trees response: %w", err)
	}

	return &tr, nil
}

// fetchArtworkTypeListing fetches the directory listing for a single
// artwork type from a thumbnail repo using the GitHub Git Trees API.
// The Contents API caps directory listings at 1000 entries with no
// pagination (cutting large repos off alphabetically); the Trees API
// returns the full listing. Returns nil if the repo or artwork type is
// missing or the fetch fails.
func fetchArtworkTypeListing(repo string, artType string) *ThumbnailListing {
	base := fmt.Sprintf("https://api.github.com/repos/libretro-thumbnails/%s/git/trees",
		url.PathEscape(repo))

	// Step 1: the root tree, to find the artType directory's SHA.
	root, err := fetchTree(base + "/master")
	if err != nil {
		return nil
	}
	var dirSHA string
	for _, e := range root.Tree {
		if e.Type == "tree" && e.Path == artType {
			dirSHA = e.SHA
			break
		}
	}
	if dirSHA == "" {
		return nil
	}

	// Step 2: the artType subtree, which is the full list of files.
	sub, err := fetchTree(base + "/" + dirSHA)
	if err != nil {
		return nil
	}

	listing := newThumbnailListing()
	for _, e := range sub.Tree {
		if e.Type != "blob" {
			continue
		}
		name := e.Path
		if strings.HasSuffix(strings.ToLower(name), ".png") {
			name = name[:len(name)-4]
		}
		listing.addEntry(artType, name)
	}

	return listing
}
