// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user-none/eblitui/desktop/signing"
	"github.com/user-none/eblitui/desktop/storage"
)

// validSHA is a well-formed SHA-256 for entries whose hash is not checked
var validSHA = strings.Repeat("ab", 32)

func sha(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// entry returns an index entry describing data stored at p.
func entry(typ, p string, data []byte) File {
	return File{Type: typ, Path: p, Size: int64(len(data)), CRC32: fmt.Sprintf("%08x", crc32.ChecksumIEEE(data)), SHA256: sha(data)}
}

func indexJSON(files string) []byte {
	return []byte(`{"version":1,"games":{"T-1":[` + files + `]}}`)
}

// fileJSON returns one index entry; fields are given in order
func fileJSON(typ, path string, size int, crc, sha string) string {
	return fmt.Sprintf(`{"type":%q,"path":%q,"size":%d,"crc32":%q,"sha256":%q}`, typ, path, size, crc, sha)
}

// fieldsJSON returns one valid index entry of typ at path with the extra
// JSON members fields, such as `"disc":2,"region":"eu"`
func fieldsJSON(typ, path, fields string) string {
	return fmt.Sprintf(`{"type":%q,"path":%q,%s,"size":10,"crc32":"0a1b2c3d","sha256":%q}`, typ, path, fields, validSHA)
}

func TestParse(t *testing.T) {
	ix, err := Parse(indexJSON(fileJSON("boxart", "boxart/T-1.webp", 10, "0a1b2c3d", validSHA) + "," +
		fieldsJSON("disc", "disc/T-1.2.eu.webp", `"disc":2,"region":"eu"`) + "," +
		fieldsJSON("flyer", "flyer/T-1.b.webp", `"variant":"b"`)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	f := ix.Games["T-1"]
	if len(f) != 3 || f[0].Path != "boxart/T-1.webp" || f[0].Size != 10 {
		t.Fatalf("games = %+v", ix.Games)
	}
	if f[1].Disc != 2 || f[1].Region != "eu" || f[1].Variant != "" {
		t.Errorf("disc entry = %+v, want disc 2 region eu", f[1])
	}
	if f[2].Variant != "b" || f[2].Disc != 0 || f[2].Region != "" {
		t.Errorf("flyer entry = %+v, want variant b", f[2])
	}
}

func TestParseRejects(t *testing.T) {
	for name, data := range map[string]string{
		"version":          `{"version":2,"games":{}}`,
		"not json":         `{"version":1,`,
		"absolute":         string(indexJSON(fileJSON("boxart", "/boxart/T-1.webp", 10, "0a1b2c3d", validSHA))),
		"parent":           string(indexJSON(fileJSON("boxart", "../x.webp", 10, "0a1b2c3d", validSHA))),
		"extension":        string(indexJSON(fileJSON("boxart", "boxart/T-1.exe", 10, "0a1b2c3d", validSHA))),
		"crc":              string(indexJSON(fileJSON("boxart", "boxart/T-1.webp", 10, "XYZ", validSHA))),
		"sha256":           string(indexJSON(fileJSON("boxart", "boxart/T-1.webp", 10, "0a1b2c3d", "x"))),
		"size":             string(indexJSON(fileJSON("boxart", "boxart/T-1.webp", 0, "0a1b2c3d", validSHA))),
		"type":             string(indexJSON(fileJSON("../boxart", "boxart/T-1.webp", 10, "0a1b2c3d", validSHA))),
		"negative disc":    string(indexJSON(fieldsJSON("disc", "disc/T-1.webp", `"disc":-1`))),
		"disc on boxart":   string(indexJSON(fieldsJSON("boxart", "boxart/T-1.webp", `"disc":2`))),
		"disc on flyer":    string(indexJSON(fieldsJSON("flyer", "flyer/T-1.webp", `"disc":2`))),
		"region path":      string(indexJSON(fieldsJSON("disc", "disc/T-1.webp", `"region":"../x"`))),
		"region one":       string(indexJSON(fieldsJSON("disc", "disc/T-1.webp", `"region":"e"`))),
		"region upper":     string(indexJSON(fieldsJSON("boxart", "boxart/T-1.webp", `"region":"EU"`))),
		"region on flyer":  string(indexJSON(fieldsJSON("flyer", "flyer/T-1.webp", `"region":"eu"`))),
		"region on poster": string(indexJSON(fieldsJSON("poster", "poster/T-1.webp", `"region":"eu"`))),
		"variant two":      string(indexJSON(fieldsJSON("flyer", "flyer/T-1.webp", `"variant":"ab"`))),
		"variant upper":    string(indexJSON(fieldsJSON("flyer", "flyer/T-1.webp", `"variant":"B"`))),
		"variant digit":    string(indexJSON(fieldsJSON("poster", "poster/T-1.webp", `"variant":"1"`))),
		"variant path":     string(indexJSON(fieldsJSON("flyer", "flyer/T-1.webp", `"variant":"/"`))),
		"variant on disc":  string(indexJSON(fieldsJSON("disc", "disc/T-1.webp", `"variant":"a"`))),
		"variant on box":   string(indexJSON(fieldsJSON("boxback", "boxback/T-1.webp", `"variant":"a"`))),
	} {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("%s: parsed without an error", name)
		}
	}
}

func TestParseIgnoresUnknownTypeFields(t *testing.T) {
	// Fields of a type this build does not know are not evaluated; the
	// entry is never used
	ix, err := Parse(indexJSON(fieldsJSON("screenshot", "screenshot/T-1.webp", `"disc":3,"region":"X","variant":"zz"`)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := ix.Files("T-1", DownloadTypes(storage.ArtworkDownloadAll)); len(got) != 0 {
		t.Errorf("unknown type used: %+v", got)
	}
}

func TestVerifyUsesSHA256(t *testing.T) {
	data := []byte("vetted image")
	f := entry("boxart", "boxart/T-1.webp", data)
	if err := Verify(data, f); err != nil {
		t.Errorf("matching data: %v", err)
	}
	f.SHA256 = sha([]byte("other image!"))
	if err := Verify(data, f); err == nil {
		t.Error("data with matching size and crc32 but a different sha256 accepted")
	}
	if err := Verify(data[:4], entry("boxart", "boxart/T-1.webp", data)); err == nil {
		t.Error("short data accepted")
	}
}

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// writeFile writes data to name in dir and returns its path
func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadLimitsDimensions(t *testing.T) {
	dir := t.TempDir()
	if img, err := Load(writeFile(t, dir, "limit.png", encodePNG(t, MaxImageDimension, 1))); err != nil || img.Bounds().Dx() != MaxImageDimension {
		t.Errorf("image at the limit: %v", err)
	}
	if _, err := Load(writeFile(t, dir, "wide.png", encodePNG(t, MaxImageDimension+1, 1))); err == nil {
		t.Error("image wider than the limit loaded")
	}
	if _, err := Load(writeFile(t, dir, "tall.png", encodePNG(t, 1, MaxImageDimension+1))); err == nil {
		t.Error("image taller than the limit loaded")
	}
	if _, err := Load(writeFile(t, dir, "text.png", []byte("not an image"))); err == nil {
		t.Error("non-image loaded")
	}
	if _, err := Load(filepath.Join(dir, "missing.png")); err == nil {
		t.Error("missing file loaded")
	}
}

func TestSize(t *testing.T) {
	dir := t.TempDir()
	if w, h, err := Size(writeFile(t, dir, "ok.png", encodePNG(t, 30, 40))); err != nil || w != 30 || h != 40 {
		t.Errorf("Size = %dx%d, %v, want 30x40", w, h, err)
	}
	if _, _, err := Size(writeFile(t, dir, "wide.png", encodePNG(t, MaxImageDimension+1, 1))); err == nil {
		t.Error("image wider than the limit sized")
	}
	if _, _, err := Size(writeFile(t, dir, "text.png", []byte("not an image"))); err == nil {
		t.Error("non-image sized")
	}
	if _, _, err := Size(filepath.Join(dir, "missing.png")); err == nil {
		t.Error("missing file sized")
	}
}

func TestLoadIgnoresTrailingData(t *testing.T) {
	// The file size is not limited: an image within the dimension limit
	// loads even when the file carries far more data than the image
	data := append(encodePNG(t, 2, 2), make([]byte, MaxImageSize+1)...)
	if img, err := Load(writeFile(t, t.TempDir(), "big.png", data)); err != nil || img.Bounds().Dx() != 2 {
		t.Errorf("large file with a small image: %v", err)
	}
}

func TestValidPath(t *testing.T) {
	for p, want := range map[string]bool{
		"boxart/T-1.webp":        true,
		"boxart/GS-9042 V1.webp": true,
		"":                       false,
		"/boxart/T-1.webp":       false,
		"boxart//T-1.webp":       false,
		"boxart/../T-1.webp":     false,
		"./T-1.webp":             false,
		`boxart\T-1.webp`:        false,
		"boxart/T-1\n.webp":      false,
	} {
		if got := ValidPath(p); got != want {
			t.Errorf("ValidPath(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestFileURL(t *testing.T) {
	got, err := FileURL("https://artwork.example.com/", "boxart/GS-9042 V1.webp")
	if err != nil || got != "https://artwork.example.com/boxart/GS-9042%20V1.webp" {
		t.Errorf("FileURL = %q, %v", got, err)
	}
	for _, base := range []string{"file:///etc/", "http://artwork.example.com/", "https:///no-host/", "artwork.example.com/"} {
		if _, err := FileURL(base, "boxart/T-1.webp"); err == nil {
			t.Errorf("base URL %q accepted", base)
		}
	}
	if _, err := FileURL("https://artwork.example.com/", "../x.webp"); err == nil {
		t.Error("parent path accepted")
	}
}

func TestFiles(t *testing.T) {
	ix := &Index{Version: 1, Games: map[string][]File{"T-1": {
		{Type: "boxart", Path: "boxart/T-1.webp"},
		{Type: "boxback", Path: "boxback/T-1.webp"},
		{Type: "disc", Path: "disc/T-1.1.eu.webp", Disc: 1, Region: "eu"},
		{Type: "disc", Path: "disc/T-1.1.us.webp", Disc: 1, Region: "us"},
		{Type: "flyer", Path: "flyer/T-1.a.webp", Variant: "a"},
		{Type: "flyer", Path: "flyer/T-1.b.webp", Variant: "b"},
		{Type: "poster", Path: "poster/T-1.webp"},
		{Type: "screenshot", Path: "screenshot/T-1.webp"},
	}}}
	paths := func(fs []File) []string {
		var out []string
		for _, f := range fs {
			out = append(out, f.Path)
		}
		return out
	}
	for _, tc := range []struct {
		level string
		want  []string
	}{
		{storage.ArtworkDownloadFront, []string{"boxart/T-1.webp"}},
		{storage.ArtworkDownloadCovers, []string{"boxart/T-1.webp", "boxback/T-1.webp"}},
		{storage.ArtworkDownloadAll, []string{"boxart/T-1.webp", "boxback/T-1.webp", "disc/T-1.1.eu.webp", "disc/T-1.1.us.webp",
			"flyer/T-1.a.webp", "flyer/T-1.b.webp", "poster/T-1.webp"}},
	} {
		got := paths(ix.Files("T-1", DownloadTypes(tc.level)))
		if strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Errorf("%s: Files = %v, want %v", tc.level, got, tc.want)
		}
	}
	if got := ix.Files("T-2", DownloadTypes(storage.ArtworkDownloadAll)); got != nil {
		t.Errorf("unlisted game: %+v", got)
	}
	var nilIndex *Index
	if got := nilIndex.Files("T-1", DownloadTypes(storage.ArtworkDownloadAll)); got != nil {
		t.Errorf("nil index: %+v", got)
	}
}

func TestDownloadTypes(t *testing.T) {
	for level, want := range map[string]string{
		storage.ArtworkDownloadFront:  "boxart",
		storage.ArtworkDownloadCovers: "boxart boxback",
		storage.ArtworkDownloadAll:    "boxart boxback disc flyer poster",
		"":                            "boxart boxback disc flyer poster",
	} {
		if got := strings.Join(DownloadTypes(level), " "); got != want {
			t.Errorf("DownloadTypes(%q) = %q, want %q", level, got, want)
		}
	}
}

func TestLocalName(t *testing.T) {
	for _, tc := range []struct {
		f    File
		want string
	}{
		{File{Type: "boxart", Path: "boxart/T-1.webp"}, "boxart.webp"},
		{File{Type: "boxart", Path: "boxart/T-1.eu.webp", Region: "eu"}, "boxart_eu.webp"},
		{File{Type: "boxback", Path: "boxback/T-1.PNG"}, "boxback.png"},
		{File{Type: "disc", Path: "disc/T-1.webp"}, "disc.webp"},
		{File{Type: "disc", Path: "disc/T-1.us.webp", Region: "us"}, "disc_us.webp"},
		{File{Type: "disc", Path: "disc/T-1.2.webp", Disc: 2}, "disc_2.webp"},
		{File{Type: "disc", Path: "disc/T-1.2.eu.webp", Disc: 2, Region: "eu"}, "disc_2_eu.webp"},
		{File{Type: "disc", Path: "disc/T-1.10.kr.webp", Disc: 10, Region: "kr"}, "disc_10_kr.webp"},
		{File{Type: "flyer", Path: "flyer/T-1.webp"}, "flyer.webp"},
		{File{Type: "flyer", Path: "flyer/T-1.b.webp", Variant: "b"}, "flyer_b.webp"},
		{File{Type: "poster", Path: "poster/T-1.a.webp", Variant: "a"}, "poster_a.webp"},
		// The fields name the file, not the path
		{File{Type: "disc", Path: "disc/whatever.name.webp", Disc: 3}, "disc_3.webp"},
	} {
		if got := LocalName(tc.f); got != tc.want {
			t.Errorf("LocalName(%+v) = %q, want %q", tc.f, got, tc.want)
		}
	}
}

func TestNeedsDownload(t *testing.T) {
	dir := t.TempDir()
	data := []byte("image bytes")
	f := entry("boxart", "boxart/T-1.webp", data)

	if !NeedsDownload(dir, f) {
		t.Error("missing file: no download")
	}
	os.WriteFile(filepath.Join(dir, "boxart.webp"), data, 0644)
	if NeedsDownload(dir, f) {
		t.Error("matching file: download")
	}
	os.WriteFile(filepath.Join(dir, "boxart.webp"), []byte("image bytez"), 0644)
	if !NeedsDownload(dir, f) {
		t.Error("same size, different crc: no download")
	}
	os.WriteFile(filepath.Join(dir, "boxart.webp"), []byte("short"), 0644)
	if !NeedsDownload(dir, f) {
		t.Error("different size: no download")
	}
}

func touch(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSupersededBoxArt(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "boxart.webp", "boxart.png", "boxback.webp", "boxback.png")
	if p, ok := SupersededBoxArt(dir); !ok || filepath.Base(p) != "boxart.png" {
		t.Errorf("both present: %q, %v, want boxart.png", p, ok)
	}

	dir = t.TempDir()
	touch(t, dir, "boxart.png")
	if p, ok := SupersededBoxArt(dir); ok {
		t.Errorf("no boxart.webp: %q, want boxart.png kept", p)
	}

	dir = t.TempDir()
	touch(t, dir, "boxart.webp")
	if p, ok := SupersededBoxArt(dir); ok {
		t.Errorf("no boxart.png: %q", p)
	}

	if p, ok := SupersededBoxArt(filepath.Join(t.TempDir(), "none")); ok {
		t.Errorf("missing directory: %q", p)
	}
}

func TestBoxArtPath(t *testing.T) {
	dir := t.TempDir()
	if got := BoxArtPath(dir); got != filepath.Join(dir, "boxart.webp") {
		t.Errorf("none: %s", got)
	}
	touch(t, dir, "boxart.png")
	if got := BoxArtPath(dir); filepath.Base(got) != "boxart.png" {
		t.Errorf("legacy only: %s", got)
	}
	touch(t, dir, "boxart.webp")
	if got := BoxArtPath(dir); filepath.Base(got) != "boxart.webp" {
		t.Errorf("both: %s", got)
	}

	// Regional fronts: USA, then Japan, then Europe, then any other; a
	// front with no region comes first
	dir = t.TempDir()
	steps := []struct{ add, want string }{
		{"boxart_kr.webp", "boxart_kr.webp"},
		{"boxart_eu.webp", "boxart_eu.webp"},
		{"boxart_jp.webp", "boxart_jp.webp"},
		{"boxart_us.webp", "boxart_us.webp"},
		{"boxart.webp", "boxart.webp"},
	}
	for _, s := range steps {
		touch(t, dir, s.add)
		if got := BoxArtPath(dir); filepath.Base(got) != s.want {
			t.Errorf("after adding %s: %s, want %s", s.add, filepath.Base(got), s.want)
		}
	}
}

func TestLocalFiles(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir,
		"poster.webp", "flyer_b.webp", "flyer_a.webp",
		"disc_2_eu.webp", "disc_2_us.webp", "disc_1_kr.webp", "disc_1_eu.webp", "disc_1_us.webp", "disc_1_jp.webp",
		"boxback_eu.webp", "boxback_us.webp",
		"boxart.png", "boxart.webp",
		// Not artwork this build shows
		"notes.txt", "screenshot.webp", "disc_a.webp", "flyer_ab.webp", "flyer_1.webp", "boxart_1.webp",
		"disc_0.webp", "disc_x_.webp", "boxart.exe", ".hidden.webp")
	if err := os.Mkdir(filepath.Join(dir, "disc.webp"), 0o755); err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, f := range LocalFiles(dir) {
		got = append(got, filepath.Base(f.Path)+"="+Caption(f))
	}
	want := []string{
		"boxart.webp=Front",
		"boxback_us.webp=Back (USA)",
		"boxback_eu.webp=Back (Europe)",
		"disc_1_us.webp=Disc 1 (USA)",
		"disc_1_jp.webp=Disc 1 (Japan)",
		"disc_1_eu.webp=Disc 1 (Europe)",
		"disc_1_kr.webp=Disc 1 (Other)",
		"disc_2_us.webp=Disc 2 (USA)",
		"disc_2_eu.webp=Disc 2 (Europe)",
		"flyer_a.webp=Flyer A",
		"flyer_b.webp=Flyer B",
		"poster.webp=Poster",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("LocalFiles =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	if got := LocalFiles(filepath.Join(t.TempDir(), "none")); got != nil {
		t.Errorf("missing directory: %+v", got)
	}
}

func TestCaption(t *testing.T) {
	for _, tc := range []struct {
		f    LocalFile
		want string
	}{
		{LocalFile{Type: "boxart"}, "Front"},
		{LocalFile{Type: "boxart", Region: "jp"}, "Front (Japan)"},
		{LocalFile{Type: "boxback"}, "Back"},
		{LocalFile{Type: "disc"}, "Disc"},
		{LocalFile{Type: "disc", Region: "eu"}, "Disc (Europe)"},
		{LocalFile{Type: "disc", Disc: 2}, "Disc 2"},
		{LocalFile{Type: "disc", Disc: 2, Region: "br"}, "Disc 2 (Other)"},
		{LocalFile{Type: "flyer"}, "Flyer"},
		{LocalFile{Type: "flyer", Variant: "c"}, "Flyer C"},
		{LocalFile{Type: "poster", Variant: "a"}, "Poster A"},
	} {
		if got := Caption(tc.f); got != tc.want {
			t.Errorf("Caption(%+v) = %q, want %q", tc.f, got, tc.want)
		}
	}
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

// testKey returns a signing key and the trusted keys holding its public key
func testKey(t *testing.T) (ed25519.PrivateKey, signing.Keys) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys, errs := signing.ParseKeys([]string{"ed25519:" + base64.StdEncoding.EncodeToString(pub)})
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	return priv, keys
}

// sigFile returns an index.sig signing data with priv
func sigFile(priv ed25519.PrivateKey, data []byte) []byte {
	return []byte("ed25519 " + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data)) + "\n")
}

// indexServer serves index and sig under /art/
func indexServer(t *testing.T, index, sig []byte) string {
	t.Helper()
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/art/index.json":
			w.Write(index)
		case r.URL.Path == "/art/index.sig" && sig != nil:
			w.Write(sig)
		default:
			http.NotFound(w, r)
		}
	}))
	return srv.URL + "/art/"
}

func TestFetchIndexRejectsRedirectToHTTP(t *testing.T) {
	priv, keys := testKey(t)
	index := indexJSON("")
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.sig" {
			w.Write(sigFile(priv, index))
			return
		}
		w.Write(index)
	}))
	defer plain.Close()
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+r.URL.Path, http.StatusFound)
	}))

	if _, err := FetchIndex(srv.URL+"/", keys); err == nil {
		t.Error("index fetched through a redirect to HTTP")
	}
}

func TestFetchIndexVerifiesSignature(t *testing.T) {
	priv, keys := testKey(t)
	otherPriv, _ := testKey(t)
	index := indexJSON(fileJSON("boxart", "boxart/T-1.webp", 10, "0a1b2c3d", validSHA))

	if _, err := FetchIndex(indexServer(t, index, sigFile(priv, index)), keys); err != nil {
		t.Errorf("signed index: %v", err)
	}
	tampered := append(append([]byte(nil), index[:len(index)-1]...), ' ', '}')
	for name, base := range map[string]string{
		"no signature file": indexServer(t, index, nil),
		"untrusted key":     indexServer(t, index, sigFile(otherPriv, index)),
		"index changed":     indexServer(t, tampered, sigFile(priv, index)),
		"empty signature":   indexServer(t, index, []byte{}),
	} {
		if _, err := FetchIndex(base, keys); err == nil {
			t.Errorf("%s: index accepted", name)
		}
	}
	if _, err := FetchIndex(indexServer(t, index, sigFile(priv, index)), signing.Keys{}); err == nil {
		t.Error("index accepted with no keys")
	}
}

func TestFetchIndexAndDownload(t *testing.T) {
	priv, keys := testKey(t)
	img := []byte("webp image data")
	index := indexJSON(fileJSON("boxart", "boxart/GS-9042 V1.webp", len(img), fmt.Sprintf("%08x", crc32.ChecksumIEEE(img)), sha(img)))
	sig := sigFile(priv, index)
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/art/index.sig":
			w.Write(sig)
		case "/art/index.json":
			w.Write(index)
		case "/art/boxart/GS-9042%20V1.webp":
			w.Write(img)
		case "/art/boxart/bad.webp":
			w.Write([]byte("other bytes, same size?"))
		default:
			http.NotFound(w, r)
		}
	}))
	base := srv.URL + "/art/"

	ix, err := FetchIndex(base, keys)
	if err != nil {
		t.Fatalf("FetchIndex: %v", err)
	}
	f := ix.Games["T-1"][0]

	dir := filepath.Join(t.TempDir(), "T-1")
	if err := Download(base, dir, f); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "boxart.webp")); err != nil || string(got) != string(img) {
		t.Errorf("saved %q, %v", got, err)
	}
	if NeedsDownload(dir, f) {
		t.Error("downloaded file still needs download")
	}

	bad := f
	bad.Path = "boxart/bad.webp"
	bad.Type = "boxback"
	if err := Download(base, dir, bad); err == nil {
		t.Error("mismatched download accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "boxback.webp")); !os.IsNotExist(err) {
		t.Error("mismatched download was written")
	}

	if _, err := FetchIndex(srv.URL+"/missing/", keys); err == nil {
		t.Error("missing index fetched without an error")
	}
}
