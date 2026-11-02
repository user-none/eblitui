package netutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadClientAndLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	}))
	defer server.Close()

	// The given client is used
	used := false
	client := server.Client()
	base := client.Transport
	client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used = true
		return base.RoundTrip(r)
	})

	if data, err := Download(client, server.URL, 5); err != nil || string(data) != "hello" {
		t.Errorf("body at the limit: got %q, err %v", data, err)
	}
	if !used {
		t.Error("the given client was not used")
	}
	if _, err := Download(client, server.URL, 4); err == nil {
		t.Error("expected an error for a body over the limit")
	}
}

func TestDownload404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	if _, err := Download(server.Client(), server.URL, 1024); err == nil {
		t.Error("expected error for 404")
	}
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
