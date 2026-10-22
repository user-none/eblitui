package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in   string
		want Version
		ok   bool
	}{
		{"0.5.0", Version{0, 5, 0}, true},
		{"1.22.333", Version{1, 22, 333}, true},
		{"10.0.0", Version{10, 0, 0}, true},
		{"0.0.0", Version{0, 0, 0}, true},
		{"v0.5.0", Version{}, false},
		{"V0.5.0", Version{}, false},
		{"0.5", Version{}, false},
		{"0.5.0.1", Version{}, false},
		{"0.5.0-rc1", Version{}, false},
		{"0..0", Version{}, false},
		{".0.0", Version{}, false},
		{"-1.0.0", Version{}, false},
		{"+1.0.0", Version{}, false},
		{" 1.0.0", Version{}, false},
		{"1.0.0 ", Version{}, false},
		{"99999999999999999999.0.0", Version{}, false},
		{"abc1234", Version{}, false},
		{"", Version{}, false},
	}
	for _, tc := range tests {
		got, ok := ParseVersion(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("ParseVersion(%q) = %v, %v; want %v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestRunningVersion(t *testing.T) {
	if got := RunningVersion("0.5.0"); got != (Version{0, 5, 0}) {
		t.Errorf("0.5.0 parsed as %v", got)
	}
	for _, s := range []string{"v0.5.0", "abc1234", ""} {
		if got := RunningVersion(s); got != (Version{}) {
			t.Errorf("%q parsed as %v, want 0.0.0", s, got)
		}
	}
}

func TestVersionLess(t *testing.T) {
	tests := []struct {
		a, b Version
		want bool
	}{
		{Version{0, 4, 0}, Version{0, 5, 0}, true},
		{Version{0, 5, 0}, Version{0, 4, 0}, false},
		{Version{0, 5, 0}, Version{0, 5, 0}, false},
		{Version{0, 9, 9}, Version{1, 0, 0}, true},
		{Version{1, 0, 9}, Version{1, 1, 0}, true},
		{Version{1, 1, 1}, Version{1, 1, 2}, true},
		{Version{2, 0, 0}, Version{1, 9, 9}, false},
	}
	for _, tc := range tests {
		if got := tc.a.Less(tc.b); got != tc.want {
			t.Errorf("%v.Less(%v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestPlatformName(t *testing.T) {
	tests := map[string]string{
		"windows/amd64": "windows-amd64",
		"windows/arm64": "windows-arm64",
		"darwin/arm64":  "macos-arm64",
		"darwin/amd64":  "",
		"linux/amd64":   "linux-x86_64",
		"linux/arm64":   "linux-aarch64",
		"linux/386":     "",
		"freebsd/amd64": "",
	}
	for in, want := range tests {
		goos, goarch, _ := strings.Cut(in, "/")
		if got := platformName(goos, goarch); got != want {
			t.Errorf("platformName(%s) = %q, want %q", in, got, want)
		}
	}
}

// newKey returns a new key pair and its public key in stamped form
func newKey(t *testing.T) (ed25519.PrivateKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return priv, "ed25519:" + base64.StdEncoding.EncodeToString(pub)
}

// sigLine returns a signature file line for data signed with priv
func sigLine(priv ed25519.PrivateKey, data []byte) string {
	return "ed25519 " + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data)) + "\n"
}

func TestParseKeys(t *testing.T) {
	_, good := newKey(t)
	_, good2 := newKey(t)

	keys, errs := ParseKeys([]string{good, good2})
	if len(errs) != 0 || len(keys.ed25519) != 2 {
		t.Errorf("two good keys: %d keys, errors %v", len(keys.ed25519), errs)
	}

	short := "ed25519:" + base64.StdEncoding.EncodeToString(make([]byte, 31))
	bad := []string{"", "ed25519", "ed25519:", "ed25519:not base64!", short, "mldsa:AAAA"}
	keys, errs = ParseKeys(append([]string{good}, bad...))
	if len(keys.ed25519) != 1 {
		t.Errorf("good key among bad ones: got %d keys, want 1", len(keys.ed25519))
	}
	if len(errs) != len(bad) {
		t.Errorf("got %d errors, want %d: %v", len(errs), len(bad), errs)
	}

	keys, _ = ParseKeys(bad)
	if !keys.Empty() {
		t.Error("only bad keys should leave no usable keys")
	}
	keys, _ = ParseKeys(nil)
	if !keys.Empty() {
		t.Error("no keys should be empty")
	}
}

// testManifest returns manifest JSON for app and version with one asset per
// platform
func testManifest(t *testing.T, schema int, app, version string, platforms ...string) []byte {
	t.Helper()
	m := Manifest{Schema: schema, App: app, Version: version}
	for _, p := range platforms {
		m.Assets = append(m.Assets, Asset{
			Platform: p,
			File:     app + "-" + version + "-" + p + ".zip",
			Size:     100,
			SHA256:   strings.Repeat("ab", 32),
			Binary:   Binary{Path: app, Size: 200, SHA256: strings.Repeat("cd", 32)},
		})
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return data
}

func TestCheck(t *testing.T) {
	priv, pub := newKey(t)
	otherPriv, _ := newKey(t)
	keys, _ := ParseKeys([]string{pub})
	target := &Target{App: "erings", Version: Version{0, 4, 0}, Platform: "linux-x86_64", Keys: keys}

	newer := testManifest(t, 1, "erings", "0.5.0", "windows-amd64", "linux-x86_64")
	signed := sigLine(priv, newer)

	tests := []struct {
		name     string
		manifest []byte
		sig      string
		want     Status
	}{
		{"newer version available", newer, signed, Available},
		{"signed by an untrusted key", newer, sigLine(otherPriv, newer), Invalid},
		{"no signature", newer, "", Invalid},
		{"tampered manifest", append([]byte(" "), newer...), signed, Invalid},
		{"one good line among others", newer, "mldsa AAAA\n" + sigLine(otherPriv, newer) + "garbage\n" + signed, Available},
		{"unknown algorithm only", newer, "mldsa " + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, newer)), Invalid},
		{"CRLF line endings", newer, strings.TrimSuffix(signed, "\n") + "\r\n", Available},
		{"too many lines", newer, strings.Repeat("x\n", maxSignatureLines) + signed, Invalid},
		{"signature file too large", newer, signed + strings.Repeat(" ", MaxSignatureSize), Invalid},
	}

	same := testManifest(t, 1, "erings", "0.4.0", "linux-x86_64")
	older := testManifest(t, 1, "erings", "0.3.9", "linux-x86_64")
	otherPlatform := testManifest(t, 1, "erings", "0.5.0", "windows-amd64")
	otherApp := testManifest(t, 1, "emmd", "0.5.0", "linux-x86_64")
	schema2 := testManifest(t, 2, "erings", "0.5.0", "linux-x86_64")
	badVersion := testManifest(t, 1, "erings", "v0.5.0", "linux-x86_64")
	notJSON := []byte("not json")
	large := append(newer, make([]byte, MaxManifestSize)...)
	for _, tc := range []struct {
		name     string
		manifest []byte
		want     Status
	}{
		{"same version", same, NoUpdate},
		{"older version", older, NoUpdate},
		{"no asset for platform", otherPlatform, NoUpdate},
		{"other application", otherApp, Invalid},
		{"unknown schema", schema2, Invalid},
		{"invalid version", badVersion, Invalid},
		{"not JSON", notJSON, Invalid},
		{"manifest too large", large, Invalid},
	} {
		tests = append(tests, struct {
			name     string
			manifest []byte
			sig      string
			want     Status
		}{tc.name, tc.manifest, sigLine(priv, tc.manifest), tc.want})
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, m, err := target.Check(tc.manifest, []byte(tc.sig))
			if status != tc.want {
				t.Fatalf("status = %v, want %v (err %v)", status, tc.want, err)
			}
			if (err != nil) != (status == Invalid) {
				t.Errorf("err = %v for status %v", err, status)
			}
			if (m != nil) != (status == Available) {
				t.Errorf("manifest = %v for status %v", m, status)
			}
			if m != nil && (m.Version != "0.5.0" || m.Asset("linux-x86_64") == nil) {
				t.Errorf("manifest = %+v", m)
			}
		})
	}
}

func TestCheckUnknownPlatform(t *testing.T) {
	priv, pub := newKey(t)
	keys, _ := ParseKeys([]string{pub})
	target := &Target{App: "erings", Version: Version{0, 4, 0}, Keys: keys}
	manifest := testManifest(t, 1, "erings", "0.5.0", "linux-x86_64")

	status, _, err := target.Check(manifest, []byte(sigLine(priv, manifest)))
	if status != NoUpdate || err != nil {
		t.Errorf("status = %v, err = %v; want NoUpdate", status, err)
	}
}

func TestCheckNoKeys(t *testing.T) {
	priv, _ := newKey(t)
	target := &Target{App: "erings", Platform: "linux-x86_64"}
	manifest := testManifest(t, 1, "erings", "0.5.0", "linux-x86_64")

	if status, _, _ := target.Check(manifest, []byte(sigLine(priv, manifest))); status != Invalid {
		t.Errorf("status = %v with no trusted keys, want Invalid", status)
	}
}
