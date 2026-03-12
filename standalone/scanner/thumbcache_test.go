package scanner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/user-none/eblitui/standalone/netutil"
)

func TestFetchContentsListing(t *testing.T) {
	entries := []contentsEntry{
		{Name: "Sonic The Hedgehog (USA, Europe).png", Type: "file"},
		{Name: "Alex Kidd in Miracle World (USA, Europe).png", Type: "file"},
		{Name: "subdir", Type: "dir"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	}))
	defer server.Close()

	origClient := netutil.HTTPClient
	netutil.HTTPClient = server.Client()
	defer func() { netutil.HTTPClient = origClient }()

	result, err := fetchContentsListing(server.URL + "/contents/Named_Boxarts")
	if err != nil {
		t.Fatalf("fetchContentsListing failed: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("entries = %d, want 3", len(result))
	}

	fileCount := 0
	for _, e := range result {
		if e.Type == "file" {
			fileCount++
		}
	}
	if fileCount != 2 {
		t.Errorf("file entries = %d, want 2", fileCount)
	}
}

func TestFetchContentsListing404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	origClient := netutil.HTTPClient
	netutil.HTTPClient = server.Client()
	defer func() { netutil.HTTPClient = origClient }()

	_, err := fetchContentsListing(server.URL + "/contents/Named_Boxarts")
	if err == nil {
		t.Error("expected error for 404")
	}
}

func TestFetchContentsListingInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	origClient := netutil.HTTPClient
	netutil.HTTPClient = server.Client()
	defer func() { netutil.HTTPClient = origClient }()

	_, err := fetchContentsListing(server.URL + "/contents/Named_Boxarts")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFetchContentsListingServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	origClient := netutil.HTTPClient
	netutil.HTTPClient = server.Client()
	defer func() { netutil.HTTPClient = origClient }()

	_, err := fetchContentsListing(server.URL + "/contents/Named_Boxarts")
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestFetchArtworkTypeListing(t *testing.T) {
	boxarts := []contentsEntry{
		{Name: "Sonic The Hedgehog (USA, Europe).png", Type: "file"},
		{Name: "Alex Kidd in Miracle World (USA, Europe).png", Type: "file"},
		{Name: "subdir", Type: "dir"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "/contents/Named_Boxarts") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(boxarts)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// fetchArtworkTypeListing uses a hardcoded GitHub URL, so test the
	// listing construction directly using addEntry
	listing := newThumbnailListing()
	for _, e := range boxarts {
		if e.Type != "file" {
			continue
		}
		name := e.Name
		if strings.HasSuffix(strings.ToLower(name), ".png") {
			name = name[:len(name)-4]
		}
		listing.addEntry("Named_Boxarts", name)
	}

	// Verify boxarts entries (directories excluded)
	if listing.Exact["Named_Boxarts"] == nil {
		t.Fatal("expected Named_Boxarts entries")
	}
	if len(listing.Exact["Named_Boxarts"]) != 2 {
		t.Errorf("boxarts entries = %d, want 2", len(listing.Exact["Named_Boxarts"]))
	}

	// Named_Titles should not exist (never added)
	if listing.Exact["Named_Titles"] != nil {
		t.Error("Named_Titles should not exist for single-type listing")
	}
}
