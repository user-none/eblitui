package desktop

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/update"
)

// updateKey returns a new signing key and its public key in stamped form
func updateKey(t *testing.T) (ed25519.PrivateKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return priv, "ed25519:" + base64.StdEncoding.EncodeToString(pub)
}

// signedManifest returns a manifest offering version for the running
// platform and its signature file signed with priv
func signedManifest(t *testing.T, priv ed25519.PrivateKey, version string) (manifest, sig []byte) {
	t.Helper()
	manifest, err := json.Marshal(update.Manifest{
		Schema:  1,
		App:     "erings",
		Version: version,
		Assets:  []update.Asset{{Platform: update.Platform(), File: "erings.zip"}},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	sig = []byte("ed25519 " + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, manifest)) + "\n")
	return manifest, sig
}

// testUpdateConfig returns an update configuration for erings 0.4.0
// trusting pub
func testUpdateConfig(pub string) UpdateConfig {
	return UpdateConfig{
		ManifestURL:    "https://example.com/erings/latest/manifest.json",
		SignatureURL:   "https://example.com/erings/latest/manifest.sig",
		ReleasePageURL: "https://example.com/erings/releases/{version}",
		AppName:        "erings",
		Version:        "0.4.0",
		Keys:           []string{pub},
	}
}

// newTestUpdateManager returns an update manager for erings 0.4.0 trusting
// pub, with automatic checks on
func newTestUpdateManager(t *testing.T, pub string) *UpdateManager {
	t.Helper()
	if update.Platform() == "" {
		t.Skip("no release platform for this build")
	}
	m := NewUpdateManager(testUpdateConfig(pub), storage.DefaultConfig(), nil)
	if m == nil {
		t.Fatal("update manager is off")
	}
	return m
}

func TestNewUpdateManagerOff(t *testing.T) {
	useTempDataDir(t)
	_, pub := updateKey(t)
	config := storage.DefaultConfig()
	good := testUpdateConfig(pub)

	with := func(change func(cfg *UpdateConfig)) UpdateConfig {
		cfg := testUpdateConfig(pub)
		change(&cfg)
		return cfg
	}
	off := map[string]UpdateConfig{
		"not configured":         {},
		"no keys":                with(func(cfg *UpdateConfig) { cfg.Keys = nil }),
		"only bad keys":          with(func(cfg *UpdateConfig) { cfg.Keys = []string{"ed25519:bad", "mldsa:AAAA"} }),
		"no app name":            with(func(cfg *UpdateConfig) { cfg.AppName = "" }),
		"no manifest URL":        with(func(cfg *UpdateConfig) { cfg.ManifestURL = "" }),
		"HTTP manifest URL":      with(func(cfg *UpdateConfig) { cfg.ManifestURL = "http://example.com/manifest.json" }),
		"HTTP signature URL":     with(func(cfg *UpdateConfig) { cfg.SignatureURL = "http://example.com/manifest.sig" }),
		"HTTP release page URL":  with(func(cfg *UpdateConfig) { cfg.ReleasePageURL = "http://example.com/{version}" }),
		"relative signature URL": with(func(cfg *UpdateConfig) { cfg.SignatureURL = "manifest.sig" }),
	}
	for name, cfg := range off {
		if m := NewUpdateManager(cfg, config, nil); m != nil {
			t.Errorf("%s: update manager is on", name)
		}
	}
	if m := NewUpdateManager(good, config, nil); m == nil {
		t.Error("valid configuration: update manager is off")
	}
}

func TestCheckDue(t *testing.T) {
	now := time.Unix(1760000000, 0)
	tests := []struct {
		name      string
		lastCheck int64
		want      bool
	}{
		{"never checked", 0, true},
		{"one hour ago", now.Add(-time.Hour).Unix(), false},
		{"just under a day ago", now.Add(-updateCheckInterval + time.Second).Unix(), false},
		{"a day ago", now.Add(-updateCheckInterval).Unix(), true},
		{"a week ago", now.Add(-7 * updateCheckInterval).Unix(), true},
		{"in the future", now.Add(time.Hour).Unix(), true},
	}
	for _, tc := range tests {
		if got := checkDue(tc.lastCheck, now); got != tc.want {
			t.Errorf("%s: checkDue = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestFinishCheckAvailable(t *testing.T) {
	useTempDataDir(t)
	priv, pub := updateKey(t)
	m := newTestUpdateManager(t, pub)
	manifest, sig := signedManifest(t, priv, "0.5.0")
	now := time.Unix(1760000000, 0)

	m.finishCheck(manifest, sig, nil, false, now)

	status := m.Status()
	if status != (types.UpdateStatus{Available: "0.5.0", LastCheck: now.Unix()}) {
		t.Errorf("status = %+v", status)
	}
	if got := m.BarVersion(); got != "0.5.0" {
		t.Errorf("BarVersion = %q, want 0.5.0", got)
	}

	// The manifest is stored byte for byte and found again by a new manager
	storedManifest, storedSig, err := storage.LoadUpdateManifest(update.MaxManifestSize, update.MaxSignatureSize)
	if err != nil || !bytes.Equal(storedManifest, manifest) || !bytes.Equal(storedSig, sig) {
		t.Errorf("stored manifest not saved byte for byte (err %v)", err)
	}
	again := newTestUpdateManager(t, pub)
	if got := again.Status(); got.Available != "0.5.0" || got.LastCheck != now.Unix() {
		t.Errorf("reloaded status = %+v", got)
	}
}

func TestFinishCheckManualIgnoresForBar(t *testing.T) {
	useTempDataDir(t)
	priv, pub := updateKey(t)
	m := newTestUpdateManager(t, pub)
	manifest, sig := signedManifest(t, priv, "0.5.0")

	m.finishCheck(manifest, sig, nil, true, time.Now())

	if got := m.Status().Available; got != "0.5.0" {
		t.Errorf("Available = %q, want 0.5.0", got)
	}
	if got := m.BarVersion(); got != "" {
		t.Errorf("Check Now found version shown in the bar: %q", got)
	}
}

func TestFinishCheckFailureKeepsStored(t *testing.T) {
	priv, pub := updateKey(t)
	otherPriv, _ := updateKey(t)
	stored, storedSig := signedManifest(t, priv, "0.5.0")
	forged, forgedSig := signedManifest(t, otherPriv, "0.6.0")

	failures := map[string]func(m *UpdateManager, now time.Time){
		"network error": func(m *UpdateManager, now time.Time) {
			m.finishCheck(nil, nil, errors.New("offline"), false, now)
		},
		"failed verification": func(m *UpdateManager, now time.Time) {
			m.finishCheck(forged, forgedSig, nil, false, now)
		},
	}
	for name, fail := range failures {
		t.Run(name, func(t *testing.T) {
			useTempDataDir(t)
			m := newTestUpdateManager(t, pub)
			m.finishCheck(stored, storedSig, nil, false, time.Unix(1760000000, 0))

			now := time.Unix(1760100000, 0)
			fail(m, now)

			// The stored version is still available and lastCheck is updated
			status := m.Status()
			if status != (types.UpdateStatus{Available: "0.5.0", Failed: true, LastCheck: now.Unix()}) {
				t.Errorf("status = %+v", status)
			}
			gotManifest, _, err := storage.LoadUpdateManifest(update.MaxManifestSize, update.MaxSignatureSize)
			if err != nil || !bytes.Equal(gotManifest, stored) {
				t.Errorf("stored manifest replaced (err %v)", err)
			}
		})
	}
}

func TestFinishCheckFailureNothingStored(t *testing.T) {
	useTempDataDir(t)
	_, pub := updateKey(t)
	m := newTestUpdateManager(t, pub)

	m.finishCheck(nil, nil, errors.New("offline"), false, time.Unix(1760000000, 0))

	if status := m.Status(); status != (types.UpdateStatus{Failed: true, LastCheck: 1760000000}) {
		t.Errorf("status = %+v", status)
	}
	if _, _, err := storage.LoadUpdateManifest(update.MaxManifestSize, update.MaxSignatureSize); err == nil {
		t.Error("a manifest was stored by a failed check")
	}
}

func TestFinishCheckNoUpdateKeepsStored(t *testing.T) {
	useTempDataDir(t)
	priv, pub := updateKey(t)
	m := newTestUpdateManager(t, pub)
	stored, storedSig := signedManifest(t, priv, "0.5.0")
	m.finishCheck(stored, storedSig, nil, false, time.Now())

	// The latest release is the running version
	same, sameSig := signedManifest(t, priv, "0.4.0")
	m.finishCheck(same, sameSig, nil, false, time.Now())

	status := m.Status()
	if status.Failed || status.Available != "0.5.0" {
		t.Errorf("status = %+v", status)
	}
	gotManifest, _, err := storage.LoadUpdateManifest(update.MaxManifestSize, update.MaxSignatureSize)
	if err != nil || !bytes.Equal(gotManifest, stored) {
		t.Errorf("stored manifest replaced (err %v)", err)
	}
}

func TestLoadRejectsTamperedStoredManifest(t *testing.T) {
	useTempDataDir(t)
	priv, pub := updateKey(t)
	manifest, sig := signedManifest(t, priv, "0.5.0")
	tampered := bytes.Replace(manifest, []byte("0.5.0"), []byte("0.6.0"), 1)
	if err := storage.SaveUpdateManifest(tampered, sig); err != nil {
		t.Fatalf("SaveUpdateManifest: %v", err)
	}

	m := newTestUpdateManager(t, pub)
	if got := m.Status().Available; got != "" {
		t.Errorf("tampered stored manifest offered %q", got)
	}
}

func TestLoadStoredManifestNotNewer(t *testing.T) {
	useTempDataDir(t)
	priv, pub := updateKey(t)
	manifest, sig := signedManifest(t, priv, "0.4.0")
	if err := storage.SaveUpdateManifest(manifest, sig); err != nil {
		t.Fatalf("SaveUpdateManifest: %v", err)
	}

	m := newTestUpdateManager(t, pub)
	if got := m.Status().Available; got != "" {
		t.Errorf("stored manifest for the running version offered %q", got)
	}
}

func TestBarVersion(t *testing.T) {
	priv, pub := updateKey(t)
	manifest, sig := signedManifest(t, priv, "0.5.0")

	tests := []struct {
		name      string
		ignored   string
		autoCheck bool
		dismiss   bool
		want      string
	}{
		{"not ignored", "", true, false, "0.5.0"},
		{"older version ignored", "0.4.5", true, false, "0.5.0"},
		{"this version ignored", "0.5.0", true, false, ""},
		{"newer version ignored", "0.6.0", true, false, ""},
		{"unreadable ignored version", "junk", true, false, "0.5.0"},
		{"automatic checks off", "", false, false, ""},
		{"dismissed", "", true, true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			useTempDataDir(t)
			if err := storage.SaveUpdateManifest(manifest, sig); err != nil {
				t.Fatalf("SaveUpdateManifest: %v", err)
			}
			if err := storage.SaveUpdateState(&storage.UpdateState{IgnoredVersion: tc.ignored}); err != nil {
				t.Fatalf("SaveUpdateState: %v", err)
			}

			m := newTestUpdateManager(t, pub)
			m.config.Updates.AutoCheck = tc.autoCheck
			if tc.dismiss {
				m.Dismiss()
			}
			if got := m.BarVersion(); got != tc.want {
				t.Errorf("BarVersion = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBarHiddenWhileChecking(t *testing.T) {
	useTempDataDir(t)
	priv, pub := updateKey(t)
	stored, storedSig := signedManifest(t, priv, "0.5.0")
	if err := storage.SaveUpdateManifest(stored, storedSig); err != nil {
		t.Fatalf("SaveUpdateManifest: %v", err)
	}

	m := newTestUpdateManager(t, pub)
	if got := m.BarVersion(); got != "0.5.0" {
		t.Fatalf("BarVersion before a check = %q, want 0.5.0", got)
	}

	// A running check hides the stored version; the check finds a newer one
	m.mu.Lock()
	m.checking = true
	m.mu.Unlock()
	if got := m.BarVersion(); got != "" {
		t.Errorf("BarVersion while checking = %q, want none", got)
	}
	newer, newerSig := signedManifest(t, priv, "0.6.0")
	m.finishCheck(newer, newerSig, nil, false, time.Now())
	if got := m.BarVersion(); got != "0.6.0" {
		t.Errorf("BarVersion after the check = %q, want 0.6.0", got)
	}
}

func TestReadsDoNotWaitOnSave(t *testing.T) {
	useTempDataDir(t)
	priv, pub := updateKey(t)
	m := newTestUpdateManager(t, pub)
	manifest, sig := signedManifest(t, priv, "0.5.0")
	m.finishCheck(manifest, sig, nil, false, time.Now())

	// A save in progress holds saveMu
	m.saveMu.Lock()
	defer m.saveMu.Unlock()

	done := make(chan string)
	go func() {
		done <- m.Status().Available + " " + m.BarVersion()
	}()
	select {
	case got := <-done:
		if got != "0.5.0 0.5.0" {
			t.Errorf("Status and BarVersion = %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reading the state waited on a save")
	}
}

func TestConcurrentSavesKeepIgnoredVersion(t *testing.T) {
	useTempDataDir(t)
	priv, pub := updateKey(t)
	m := newTestUpdateManager(t, pub)
	manifest, sig := signedManifest(t, priv, "0.5.0")
	m.finishCheck(manifest, sig, nil, false, time.Now())

	// Checks finishing while the user ignores the version, with reads from
	// the UI thread in between
	done := make(chan struct{})
	go func() {
		for range 20 {
			m.finishCheck(manifest, sig, nil, false, time.Now())
		}
		close(done)
	}()
	m.IgnoreVersion()
	for range 20 {
		m.Status()
		m.BarVersion()
	}
	<-done

	state, err := storage.LoadUpdateState()
	if err != nil {
		t.Fatalf("LoadUpdateState: %v", err)
	}
	if state.IgnoredVersion != "0.5.0" {
		t.Errorf("ignored version on disk = %q, want 0.5.0", state.IgnoredVersion)
	}
	if got := m.BarVersion(); got != "" {
		t.Errorf("BarVersion = %q, want none", got)
	}
}

func TestIgnoreVersionPersists(t *testing.T) {
	useTempDataDir(t)
	priv, pub := updateKey(t)
	m := newTestUpdateManager(t, pub)
	manifest, sig := signedManifest(t, priv, "0.5.0")
	m.finishCheck(manifest, sig, nil, false, time.Now())

	m.IgnoreVersion()
	if got := m.BarVersion(); got != "" {
		t.Errorf("BarVersion after ignore = %q", got)
	}
	if got := newTestUpdateManager(t, pub).BarVersion(); got != "" {
		t.Errorf("ignored version shown after restart: %q", got)
	}

	// A newer version shows the bar again
	newer, newerSig := signedManifest(t, priv, "0.6.0")
	m.finishCheck(newer, newerSig, nil, false, time.Now())
	if got := m.BarVersion(); got != "0.6.0" {
		t.Errorf("BarVersion for a newer version = %q, want 0.6.0", got)
	}
}
