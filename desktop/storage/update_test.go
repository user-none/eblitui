package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// updateFilePath returns the path of a file in the update directory
func updateFilePath(t *testing.T, name string) string {
	t.Helper()
	dir, err := GetUpdateDir()
	if err != nil {
		t.Fatalf("GetUpdateDir: %v", err)
	}
	return filepath.Join(dir, name)
}

// writeUpdateFile writes raw contents to a file in the update directory
func writeUpdateFile(t *testing.T, name string, data []byte) {
	t.Helper()
	path := updateFilePath(t, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestLoadUpdateStateMissing(t *testing.T) {
	useTempDataDir(t)

	state, err := LoadUpdateState()
	if err != nil {
		t.Fatalf("LoadUpdateState: %v", err)
	}
	if *state != (UpdateState{Version: updateStateVersion}) {
		t.Errorf("missing file loaded as %+v, want empty", state)
	}
}

func TestLoadUpdateStateCorrupt(t *testing.T) {
	useTempDataDir(t)
	writeUpdateFile(t, updateStateFile, []byte("{not json"))

	state, err := LoadUpdateState()
	if err != nil {
		t.Fatalf("LoadUpdateState: %v", err)
	}
	if *state != (UpdateState{Version: updateStateVersion}) {
		t.Errorf("corrupt file loaded as %+v, want empty", state)
	}
}

func TestUpdateStateRoundTrip(t *testing.T) {
	useTempDataDir(t)

	want := UpdateState{LastCheck: 1760000000, IgnoredVersion: "v0.5.0"}
	if err := SaveUpdateState(&want); err != nil {
		t.Fatalf("SaveUpdateState: %v", err)
	}
	got, err := LoadUpdateState()
	if err != nil {
		t.Fatalf("LoadUpdateState: %v", err)
	}
	want.Version = updateStateVersion
	if *got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestUpdateManifestRoundTrip(t *testing.T) {
	useTempDataDir(t)

	if _, _, err := LoadUpdateManifest(1024, 1024); err == nil {
		t.Error("expected an error with nothing stored")
	}

	manifest := []byte("{\"schema\": 1}\n")
	sig := []byte("ed25519 AAAA\n")
	if err := SaveUpdateManifest(manifest, sig); err != nil {
		t.Fatalf("SaveUpdateManifest: %v", err)
	}
	gotManifest, gotSig, err := LoadUpdateManifest(1024, 1024)
	if err != nil {
		t.Fatalf("LoadUpdateManifest: %v", err)
	}
	if !bytes.Equal(gotManifest, manifest) || !bytes.Equal(gotSig, sig) {
		t.Errorf("got %q, %q; want byte for byte %q, %q", gotManifest, gotSig, manifest, sig)
	}

	// No temporary files are left behind
	entries, err := os.ReadDir(filepath.Dir(updateFilePath(t, updateManifestFile)))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("update directory has %d entries, want 2", len(entries))
	}
}

func TestLoadUpdateManifestLimits(t *testing.T) {
	useTempDataDir(t)
	writeUpdateFile(t, updateManifestFile, make([]byte, 100))
	writeUpdateFile(t, updateSigFile, make([]byte, 10))

	if _, _, err := LoadUpdateManifest(100, 10); err != nil {
		t.Errorf("files at the limits: %v", err)
	}
	if _, _, err := LoadUpdateManifest(99, 10); err == nil {
		t.Error("expected an error for a manifest over its limit")
	}
	if _, _, err := LoadUpdateManifest(100, 9); err == nil {
		t.Error("expected an error for a signature over its limit")
	}
}

func TestLoadUpdateManifestMissingSignature(t *testing.T) {
	useTempDataDir(t)
	writeUpdateFile(t, updateManifestFile, []byte("{}"))

	if _, _, err := LoadUpdateManifest(1024, 1024); err == nil {
		t.Error("expected an error with no signature file")
	}
}
