// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package scanner

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/user-none/eblitui/desktop/storage"
)

// newSigningKey returns a signing key and its public key in the
// algorithm:base64 form emulators give as ArtworkConfig.Keys
func newSigningKey(t *testing.T) (ed25519.PrivateKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv, "ed25519:" + base64.StdEncoding.EncodeToString(pub)
}

// artworkServer serves an index listing boxart and boxback for T-1, signed
// with priv.
func artworkServer(t *testing.T, priv ed25519.PrivateKey, front, back []byte) *httptest.Server {
	t.Helper()
	ent := func(typ string, data []byte) string {
		sum := sha256.Sum256(data)
		return fmt.Sprintf(`{"type":%q,"path":"%s/T-1.webp","size":%d,"crc32":"%08x","sha256":"%x"}`,
			typ, typ, len(data), crc32.ChecksumIEEE(data), sum)
	}
	index := []byte(`{"version":1,"games":{"T-1":[` + ent("boxart", front) + `,` + ent("boxback", back) + `]}}`)
	sig := "ed25519 " + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, index)) + "\n"
	return tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.sig":
			w.Write([]byte(sig))
		case "/index.json":
			w.Write(index)
		case "/boxart/T-1.webp":
			w.Write(front)
		case "/boxback/T-1.webp":
			w.Write(back)
		default:
			http.NotFound(w, r)
		}
	}))
}

// tlsServer starts an HTTPS test server and makes the default transport
// trust its certificate for the rest of the test
func tlsServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	orig := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() {
		http.DefaultTransport = orig
		srv.Close()
	})
	return srv
}

func TestResolveArtworkDownloadsMissingAndChanged(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	front, back := []byte("front image"), []byte("back image")
	priv, pub := newSigningKey(t)
	srv := artworkServer(t, priv, front, back)

	s := NewScanner(nil, nil, nil, nil, 0, true, nil, Sources{ArtworkURL: srv.URL + "/", ArtworkKeys: []string{pub}})
	s.games["T-1"] = &storage.GameEntry{GameID: "T-1"}
	s.games["T-2"] = &storage.GameEntry{GameID: "T-2"}

	artDir, err := storage.GetGameArtworkDir("T-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artDir, "boxback.webp"), back, 0o644); err != nil {
		t.Fatal(err)
	}

	jobs := s.resolveArtwork()
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1 (boxart only; boxback matches, T-2 unlisted)", len(jobs))
	}
	s.downloadAssets(jobs, ScanPhaseArtwork, "")
	if errs := s.getErrors(); len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	if got, err := os.ReadFile(filepath.Join(artDir, "boxart.webp")); err != nil || string(got) != string(front) {
		t.Errorf("boxart.webp = %q, %v", got, err)
	}

	if err := os.WriteFile(filepath.Join(artDir, "boxback.webp"), []byte("back imagf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if jobs := s.resolveArtwork(); len(jobs) != 1 {
		t.Errorf("changed boxback: jobs = %d, want 1", len(jobs))
	}
}

// allTypesServer serves a signed index listing T-1's front, back, disc 2
// from Europe and flyer a, and returns the local names they are stored as
func allTypesServer(t *testing.T, priv ed25519.PrivateKey) (*httptest.Server, map[string]string) {
	t.Helper()
	files := []struct{ typ, path, fields, local string }{
		{"boxart", "boxart/T-1.webp", "", "boxart.webp"},
		{"boxback", "boxback/T-1.webp", "", "boxback.webp"},
		{"disc", "disc/T-1.2.eu.webp", `"disc":2,"region":"eu",`, "disc_2_eu.webp"},
		{"flyer", "flyer/T-1.a.webp", `"variant":"a",`, "flyer_a.webp"},
	}
	data := map[string][]byte{}
	locals := map[string]string{}
	var entries []string
	for _, f := range files {
		d := []byte("image " + f.path)
		data["/"+f.path] = d
		locals[f.typ] = f.local
		sum := sha256.Sum256(d)
		entries = append(entries, fmt.Sprintf(`{"type":%q,"path":%q,%s"size":%d,"crc32":"%08x","sha256":"%x"}`,
			f.typ, f.path, f.fields, len(d), crc32.ChecksumIEEE(d), sum))
	}
	index := []byte(`{"version":1,"games":{"T-1":[` + strings.Join(entries, ",") + `]}}`)
	sig := "ed25519 " + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, index)) + "\n"
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.sig":
			w.Write([]byte(sig))
		case "/index.json":
			w.Write(index)
		default:
			if d, ok := data[r.URL.Path]; ok {
				w.Write(d)
				return
			}
			http.NotFound(w, r)
		}
	}))
	return srv, locals
}

func TestResolveArtworkDownloadLevel(t *testing.T) {
	for _, tc := range []struct {
		level string
		want  []string
	}{
		{storage.ArtworkDownloadFront, []string{"boxart"}},
		{storage.ArtworkDownloadCovers, []string{"boxart", "boxback"}},
		{storage.ArtworkDownloadAll, []string{"boxart", "boxback", "disc", "flyer"}},
	} {
		t.Run(tc.level, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_DATA_HOME", dir)
			t.Setenv("HOME", dir)
			t.Setenv("APPDATA", dir)
			priv, pub := newSigningKey(t)
			srv, locals := allTypesServer(t, priv)

			s := NewScanner(nil, nil, nil, nil, 0, true, nil, Sources{ArtworkURL: srv.URL + "/", ArtworkKeys: []string{pub}, ArtworkDownload: tc.level})
			s.games["T-1"] = &storage.GameEntry{GameID: "T-1"}
			jobs := s.resolveArtwork()
			if len(jobs) != len(tc.want) {
				t.Fatalf("jobs = %d, want %d", len(jobs), len(tc.want))
			}
			s.downloadAssets(jobs, ScanPhaseArtwork, "")

			artDir, err := storage.GetGameArtworkDir("T-1")
			if err != nil {
				t.Fatal(err)
			}
			for typ, name := range locals {
				_, err := os.Stat(filepath.Join(artDir, name))
				if want := slices.Contains(tc.want, typ); want != (err == nil) {
					t.Errorf("%s: present = %v, want %v", name, err == nil, want)
				}
			}
		})
	}
}

func TestResolveArtworkKeepsOtherFiles(t *testing.T) {
	// A lower level only limits downloads; artwork already on disk stays
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	priv, pub := newSigningKey(t)
	srv, _ := allTypesServer(t, priv)

	artDir, err := storage.GetGameArtworkDir("T-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatal(err)
	}
	kept := []string{"flyer_a.webp", "disc_2_eu.webp", "poster_c.webp", "boxback_jp.webp"}
	for _, n := range kept {
		if err := os.WriteFile(filepath.Join(artDir, n), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := NewScanner(nil, nil, nil, nil, 0, true, nil, Sources{ArtworkURL: srv.URL + "/", ArtworkKeys: []string{pub}, ArtworkDownload: storage.ArtworkDownloadFront})
	s.games["T-1"] = &storage.GameEntry{GameID: "T-1"}
	s.downloadAssets(s.resolveArtwork(), ScanPhaseArtwork, "")
	for _, n := range kept {
		if got, err := os.ReadFile(filepath.Join(artDir, n)); err != nil || string(got) != "old" {
			t.Errorf("%s: %q, %v, want it left as it was", n, got, err)
		}
	}
}

func TestResolveArtworkIndexFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	_, pub := newSigningKey(t)
	srv := tlsServer(t, http.NotFoundHandler())

	s := NewScanner(nil, nil, nil, nil, 0, true, nil, Sources{ArtworkURL: srv.URL + "/", ArtworkKeys: []string{pub}})
	s.games["T-1"] = &storage.GameEntry{GameID: "T-1"}

	if jobs := s.resolveArtwork(); len(jobs) != 0 {
		t.Errorf("jobs = %d, want none", len(jobs))
	}
	if errs := s.getErrors(); len(errs) != 0 {
		t.Errorf("errors = %v, want none; artwork failures are logged, not reported", errs)
	}
}

func TestResolveArtworkNoneSkipsIndex(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	_, pub := newSigningKey(t)
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request for %s, want none", r.URL.Path)
		http.NotFound(w, r)
	}))

	s := NewScanner(nil, nil, nil, nil, 0, true, nil, Sources{ArtworkURL: srv.URL + "/", ArtworkKeys: []string{pub}, ArtworkDownload: storage.ArtworkDownloadNone})
	s.games["T-1"] = &storage.GameEntry{GameID: "T-1"}
	if jobs := s.resolveArtwork(); len(jobs) != 0 {
		t.Errorf("jobs = %d, want none", len(jobs))
	}
}

func TestResolveArtworkRequiresTrustedSignature(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	signer, _ := newSigningKey(t)
	_, trusted := newSigningKey(t)
	srv := artworkServer(t, signer, []byte("front image"), []byte("back image"))

	for name, keys := range map[string][]string{"untrusted signer": {trusted}, "no keys": nil} {
		s := NewScanner(nil, nil, nil, nil, 0, true, nil, Sources{ArtworkURL: srv.URL + "/", ArtworkKeys: keys})
		s.games["T-1"] = &storage.GameEntry{GameID: "T-1"}
		if jobs := s.resolveArtwork(); len(jobs) != 0 {
			t.Errorf("%s: jobs = %d, want none", name, len(jobs))
		}
	}
}

func TestDownloadFailureNotReported(t *testing.T) {
	s := NewScanner(nil, nil, nil, nil, 0, true, nil, Sources{})
	s.downloadAssets([]resolvedJob{{download: func() error { return fmt.Errorf("boom") }}}, ScanPhaseArtwork, "")

	if errs := s.getErrors(); len(errs) != 0 {
		t.Errorf("errors = %v, want none; download failures are logged, not reported", errs)
	}
}
