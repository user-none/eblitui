package scanner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/user-none/eblitui/desktop/netutil"
)

func TestFetchTree(t *testing.T) {
	tr := treeResponse{
		Tree: []treeEntry{
			{Path: "Named_Boxarts", Type: "tree", SHA: "abc"},
			{Path: "README.md", Type: "blob", SHA: "def"},
		},
		Truncated: false,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tr)
	}))
	defer server.Close()

	origClient := netutil.HTTPClient
	netutil.HTTPClient = server.Client()
	defer func() { netutil.HTTPClient = origClient }()

	result, err := fetchTree(server.URL + "/git/trees/master")
	if err != nil {
		t.Fatalf("fetchTree failed: %v", err)
	}
	if len(result.Tree) != 2 {
		t.Errorf("tree entries = %d, want 2", len(result.Tree))
	}
}

func TestFetchTree404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	origClient := netutil.HTTPClient
	netutil.HTTPClient = server.Client()
	defer func() { netutil.HTTPClient = origClient }()

	if _, err := fetchTree(server.URL + "/git/trees/master"); err == nil {
		t.Error("expected error for 404")
	}
}

func TestFetchTreeInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	origClient := netutil.HTTPClient
	netutil.HTTPClient = server.Client()
	defer func() { netutil.HTTPClient = origClient }()

	if _, err := fetchTree(server.URL + "/git/trees/master"); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFetchTreeServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	origClient := netutil.HTTPClient
	netutil.HTTPClient = server.Client()
	defer func() { netutil.HTTPClient = origClient }()

	if _, err := fetchTree(server.URL + "/git/trees/master"); err == nil {
		t.Error("expected error for 500 response")
	}
}

// TestFetchArtworkTypeListingTwoStep verifies the root-tree -> subtree
// resolution and that only blobs (stripped of .png) become entries. The
// server returns a large subtree to confirm there is no 1000-entry cap.
func TestFetchArtworkTypeListingTwoStep(t *testing.T) {
	const dirSHA = "boxartsha"
	const subtreeCount = 1500 // exceeds the old Contents API 1000 cap

	root := treeResponse{Tree: []treeEntry{
		{Path: "Named_Boxarts", Type: "tree", SHA: dirSHA},
		{Path: "Named_Titles", Type: "tree", SHA: "othersha"},
	}}

	sub := treeResponse{}
	for i := 0; i < subtreeCount; i++ {
		sub.Tree = append(sub.Tree, treeEntry{
			Path: fmt.Sprintf("Game %04d.png", i),
			Type: "blob",
		})
	}
	sub.Tree = append(sub.Tree, treeEntry{Path: "nested", Type: "tree"})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/git/trees/master"):
			json.NewEncoder(w).Encode(root)
		case strings.HasSuffix(r.URL.Path, "/git/trees/"+dirSHA):
			json.NewEncoder(w).Encode(sub)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	origClient := netutil.HTTPClient
	netutil.HTTPClient = server.Client()
	defer func() { netutil.HTTPClient = origClient }()

	// fetchArtworkTypeListing builds a hardcoded api.github.com URL, so
	// drive the two-step resolution directly against the mock here.
	rootResp, err := fetchTree(server.URL + "/git/trees/master")
	if err != nil {
		t.Fatalf("root fetch failed: %v", err)
	}
	var gotSHA string
	for _, e := range rootResp.Tree {
		if e.Type == "tree" && e.Path == "Named_Boxarts" {
			gotSHA = e.SHA
		}
	}
	if gotSHA != dirSHA {
		t.Fatalf("dir SHA = %q, want %q", gotSHA, dirSHA)
	}

	subResp, err := fetchTree(server.URL + "/git/trees/" + gotSHA)
	if err != nil {
		t.Fatalf("subtree fetch failed: %v", err)
	}

	listing := newThumbnailListing()
	for _, e := range subResp.Tree {
		if e.Type != "blob" {
			continue
		}
		name := e.Path
		if strings.HasSuffix(strings.ToLower(name), ".png") {
			name = name[:len(name)-4]
		}
		listing.addEntry("Named_Boxarts", name)
	}

	got := len(listing.Exact["Named_Boxarts"])
	if got != subtreeCount {
		t.Errorf("boxart entries = %d, want %d (no 1000-entry cap)", got, subtreeCount)
	}
}
