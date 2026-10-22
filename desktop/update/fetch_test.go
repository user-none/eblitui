package update

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/user-none/eblitui/desktop/netutil"
)

func TestNewSource(t *testing.T) {
	good := "https://example.com/latest/manifest.json"
	if _, err := NewSource(good, good, good); err != nil {
		t.Errorf("HTTPS URLs rejected: %v", err)
	}

	for _, bad := range []string{
		"",
		"http://example.com/manifest.json",
		"ftp://example.com/manifest.json",
		"https:///manifest.json",
		"example.com/manifest.json",
		"/manifest.json",
		"https://exa mple.com/manifest.json",
	} {
		for i := range 3 {
			urls := []string{good, good, good}
			urls[i] = bad
			if _, err := NewSource(urls[0], urls[1], urls[2]); err == nil {
				t.Errorf("URL %d %q accepted", i, bad)
			}
		}
	}
}

func TestReleasePage(t *testing.T) {
	good := "https://example.com/manifest.json"
	tests := map[string]string{
		"https://github.com/user-none/erings/releases/tag/v{version}": "https://github.com/user-none/erings/releases/tag/v0.5.0",
		"https://example.com/erings/{version}/notes?v={version}":      "https://example.com/erings/0.5.0/notes?v=0.5.0",
		"https://example.com/erings/releases":                         "https://example.com/erings/releases",
	}
	for template, want := range tests {
		s, err := NewSource(good, good, template)
		if err != nil {
			t.Fatalf("NewSource: %v", err)
		}
		if got := s.ReleasePage("0.5.0"); got != want {
			t.Errorf("ReleasePage(%q) = %q, want %q", template, got, want)
		}
	}
}

func TestFetchLatest(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/files/erings-manifest.json":
			w.Write([]byte("manifest"))
		case "/files/erings-manifest.sig":
			w.Write([]byte("signature"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	s, err := NewSource(srv.URL+"/files/erings-manifest.json", srv.URL+"/files/erings-manifest.sig", srv.URL+"/{version}")
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	s.client = testClient(srv)
	manifest, sig, err := s.FetchLatest()
	if err != nil || string(manifest) != "manifest" || string(sig) != "signature" {
		t.Errorf("FetchLatest = %q, %q, %v", manifest, sig, err)
	}

	s, _ = NewSource(srv.URL+"/files/erings-manifest.json", srv.URL+"/missing.sig", srv.URL+"/{version}")
	s.client = testClient(srv)
	if _, _, err := s.FetchLatest(); err == nil {
		t.Error("expected an error for a missing signature file")
	}
}

func TestFetchRejectsUntrustedCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	}))
	defer srv.Close()

	// The test server's certificate is not signed by a trusted root
	if _, err := netutil.Download(newHTTPClient(), srv.URL, 10); err == nil {
		t.Error("server with an untrusted certificate accepted")
	}
}

// testClient returns the update HTTP client set up to trust srv's
// certificate
func testClient(srv *httptest.Server) *http.Client {
	client := newHTTPClient()
	client.Transport = srv.Client().Transport
	return client
}

func TestHTTPClientRedirects(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("plain"))
	}))
	defer plain.Close()

	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.Write([]byte("hello"))
		case "/large":
			w.Write([]byte(strings.Repeat("x", 11)))
		case "/missing":
			http.NotFound(w, r)
		case "/to-https":
			http.Redirect(w, r, srv.URL+"/ok", http.StatusFound)
		case "/to-http":
			http.Redirect(w, r, plain.URL+"/", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, srv.URL+"/loop", http.StatusFound)
		}
	}))
	defer srv.Close()
	client := testClient(srv)

	tests := []struct {
		path    string
		want    string
		wantErr bool
	}{
		{"/ok", "hello", false},
		{"/large", "", true},
		{"/missing", "", true},
		{"/to-https", "hello", false},
		{"/to-http", "", true},
		{"/loop", "", true},
	}
	for _, tc := range tests {
		got, err := netutil.Download(client, srv.URL+tc.path, 10)
		if (err != nil) != tc.wantErr || string(got) != tc.want {
			t.Errorf("%s: got %q, err %v; want %q, error %v", tc.path, got, err, tc.want, tc.wantErr)
		}
	}

	// A body exactly at the limit is accepted
	if got, err := netutil.Download(client, srv.URL+"/large", 11); err != nil || len(got) != 11 {
		t.Errorf("body at the limit: got %d bytes, err %v", len(got), err)
	}
}
