package netutil

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

func TestDownloadUserAgent(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		w.Write([]byte("ok"))
	}))
	defer server.Close()
	t.Cleanup(func() { SetUserAgent("") })

	SetUserAgent("erings/1.2.3")
	if _, err := Download(server.Client(), server.URL, 16); err != nil {
		t.Fatal(err)
	}
	if got != "erings/1.2.3" {
		t.Errorf("User-Agent = %q, want erings/1.2.3", got)
	}

	// Without one set, Go's default is sent
	SetUserAgent("")
	if _, err := Download(server.Client(), server.URL, 16); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "Go-http-client/") {
		t.Errorf("User-Agent = %q, want Go's default", got)
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

// trustServer makes the default transport trust srv's certificate for the
// rest of the test, which the client from NewClient sends through.
func trustServer(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = orig })
}

// TestNewClientRequiresHTTPS tests that a request for a non-HTTPS URL fails
// before anything is sent.
func TestNewClientRequiresHTTPS(t *testing.T) {
	sent := false
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent = true
	}))
	defer plain.Close()

	_, err := NewClient(HTTPTimeout).Get(plain.URL)
	if !errors.Is(err, ErrNotHTTPS) {
		t.Fatalf("expected ErrNotHTTPS, got %v", err)
	}
	if sent {
		t.Fatal("the plain HTTP request reached the server")
	}
}

// TestNewClientRedirects tests that redirects are followed only to HTTPS
// URLs and at most maxRedirects times.
func TestNewClientRedirects(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("plain"))
	}))
	defer plain.Close()

	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ok":
			w.Write([]byte("hello"))
		case r.URL.Path == "/to-https":
			http.Redirect(w, r, srv.URL+"/ok", http.StatusFound)
		case r.URL.Path == "/to-http":
			http.Redirect(w, r, plain.URL+"/", http.StatusFound)
		case strings.HasPrefix(r.URL.Path, "/hops/"):
			n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/hops/"))
			if n == 0 {
				w.Write([]byte("hello"))
				return
			}
			http.Redirect(w, r, fmt.Sprintf("%s/hops/%d", srv.URL, n-1), http.StatusFound)
		}
	}))
	defer srv.Close()
	trustServer(t, srv)
	client := NewClient(HTTPTimeout)

	tests := []struct {
		path    string
		wantErr bool
	}{
		{"/ok", false},
		{"/to-https", false},
		{"/to-http", true},
		{fmt.Sprintf("/hops/%d", maxRedirects), false},
		{fmt.Sprintf("/hops/%d", maxRedirects+1), true},
	}
	for _, tc := range tests {
		data, err := Download(client, srv.URL+tc.path, 100)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: expected an error", tc.path)
			}
			continue
		}
		if err != nil || string(data) != "hello" {
			t.Errorf("%s: got %q, err %v", tc.path, data, err)
		}
	}
}
