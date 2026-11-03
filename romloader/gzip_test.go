package romloader

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// createTestTarGz writes a .tar.gz holding a zero-filled pad.bin of
// padSize bytes followed by game.sms containing romData.
func createTestTarGz(t *testing.T, padSize int64, romData []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.tar.gz")

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create tar.gz file: %v", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	pad := &tar.Header{Name: "pad.bin", Mode: 0644, Size: padSize, Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(pad); err != nil {
		t.Fatalf("Failed to write pad header: %v", err)
	}
	zeros := make([]byte, 1<<20)
	for left := padSize; left > 0; {
		n := min(left, int64(len(zeros)))
		if _, err := tw.Write(zeros[:n]); err != nil {
			t.Fatalf("Failed to write pad data: %v", err)
		}
		left -= n
	}

	rom := &tar.Header{Name: "game.sms", Mode: 0644, Size: int64(len(romData)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(rom); err != nil {
		t.Fatalf("Failed to write ROM header: %v", err)
	}
	if _, err := tw.Write(romData); err != nil {
		t.Fatalf("Failed to write ROM data: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("Failed to close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("Failed to close gzip: %v", err)
	}
	return path
}

// TestLoad_TarGzSkipsEntryWithinLimit tests that a ROM after a skipped
// entry loads when the whole stream stays within the decompression limit.
func TestLoad_TarGzSkipsEntryWithinLimit(t *testing.T) {
	romData := []byte{0x11, 0x22, 0x33, 0x44}
	path := createTestTarGz(t, 1<<20, romData)

	data, name, err := Load(path, testExtensions)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !bytes.Equal(data, romData) {
		t.Errorf("Data mismatch: expected %v, got %v", romData, data)
	}
	if name != "game.sms" {
		t.Errorf("Name mismatch: expected game.sms, got %s", name)
	}
}

// TestLoad_TarGzExceedsLimit tests that skipped entries count toward the
// decompression limit, so a large entry before the ROM stops the load.
func TestLoad_TarGzExceedsLimit(t *testing.T) {
	path := createTestTarGz(t, maxArchiveRead, []byte{0x11, 0x22, 0x33, 0x44})

	_, _, err := Load(path, testExtensions)
	if !errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("expected ErrArchiveTooLarge, got %v", err)
	}
}
