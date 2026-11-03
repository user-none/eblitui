package romloader

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeTestCue writes a cue sheet of exactly size bytes made of REM lines.
// It references no files, so it is only useful for checks made before the
// sheet's tracks are laid out.
func writeTestCue(t *testing.T, size int) string {
	t.Helper()
	line := []byte("REM padding\n")
	data := bytes.Repeat(line, size/len(line)+1)[:size]
	path := filepath.Join(t.TempDir(), "test.cue")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("Failed to create cue file: %v", err)
	}
	return path
}

// TestOpenBinCue_TooLarge tests that a cue sheet over the size limit is
// rejected before it is parsed.
func TestOpenBinCue_TooLarge(t *testing.T) {
	path := writeTestCue(t, maxCueSize+1)

	_, err := openBinCue(path)
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("expected ErrFileTooLarge, got %v", err)
	}
}

// TestOpenBinCue_AtLimit tests that a cue sheet of exactly the size limit
// is parsed rather than rejected for its size.
func TestOpenBinCue_AtLimit(t *testing.T) {
	path := writeTestCue(t, maxCueSize)

	_, err := openBinCue(path)
	if err == nil {
		t.Fatal("expected an error for a cue sheet with no tracks")
	}
	if errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("cue sheet at the limit rejected for size: %v", err)
	}
}

func TestSynthIndex01(t *testing.T) {
	n, lba := synthIndex01(1000, 150)
	if n != 1 || lba != 1150 {
		t.Errorf("synthIndex01(1000,150) = {%d %d}, want {1 1150}", n, lba)
	}
	n, lba = synthIndex01(0, 0)
	if n != 1 || lba != 0 {
		t.Errorf("synthIndex01(0,0) = {%d %d}, want {1 0}", n, lba)
	}
}
