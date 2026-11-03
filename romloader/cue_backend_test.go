package romloader

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// writeTestBin writes a two-sector bin file filled with fill.
func writeTestBin(t *testing.T, path string, fill byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("Failed to create bin directory: %v", err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte{fill}, 2*2352), 0644); err != nil {
		t.Fatalf("Failed to create bin file: %v", err)
	}
}

// TestOpenBinCue_TrackFileLocation tests that FILE names resolve only to
// files inside the cue's directory, including through symlinks.
func TestOpenBinCue_TrackFileLocation(t *testing.T) {
	cases := []struct {
		name    string
		file    func(base string) string
		symlink string // target of disc/link.bin, if set
		wantOK  bool
	}{
		{"same directory", func(string) string { return "real.bin" }, "", true},
		{"subdirectory with backslash", func(string) string { return `sub\real.bin` }, "", true},
		{"parent traversal", func(string) string { return "../outside.bin" }, "", false},
		{"absolute path", func(base string) string { return filepath.Join(base, "outside.bin") }, "", false},
		{"symlink inside", func(string) string { return "link.bin" }, "real.bin", true},
		{"symlink outside", func(string) string { return "link.bin" }, "../outside.bin", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			disc := filepath.Join(base, "disc")
			writeTestBin(t, filepath.Join(disc, "real.bin"), 0x11)
			writeTestBin(t, filepath.Join(disc, "sub", "real.bin"), 0x11)
			writeTestBin(t, filepath.Join(base, "outside.bin"), 0x22)
			if tc.symlink != "" {
				if err := os.Symlink(tc.symlink, filepath.Join(disc, "link.bin")); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}

			cue := "FILE \"" + tc.file(base) + "\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n"
			backend, err := openTestCue(t, disc, cue)
			if !tc.wantOK {
				if err == nil {
					backend.Close()
					t.Fatal("expected an error for a FILE outside the cue directory")
				}
				return
			}
			if err != nil {
				t.Fatalf("openBinCue: %v", err)
			}
			defer backend.Close()

			sector, err := backend.ReadSector(0)
			if err != nil {
				t.Fatalf("ReadSector: %v", err)
			}
			if sector[0] != 0x11 {
				t.Fatalf("sector[0] = %#x, want 0x11", sector[0])
			}
		})
	}
}

// openTestCue writes cue as game.cue in dir and opens it.
func openTestCue(t *testing.T, dir, cue string) (*binCueBackend, error) {
	t.Helper()
	cuePath := filepath.Join(dir, "game.cue")
	if err := os.WriteFile(cuePath, []byte(cue), 0644); err != nil {
		t.Fatalf("Failed to create cue file: %v", err)
	}
	backend, err := openBinCue(cuePath)
	if err != nil {
		return nil, err
	}
	return backend.(*binCueBackend), nil
}

// TestOpenBinCue_AliasesShareHandle tests that different spellings of the
// same bin filename open the file once.
func TestOpenBinCue_AliasesShareHandle(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), make([]byte, 3*2352), 0644); err != nil {
		t.Fatalf("Failed to create bin file: %v", err)
	}

	cue := "FILE \"a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n" +
		"FILE \"./a.bin\" BINARY\nTRACK 02 MODE1/2352\nINDEX 01 00:00:01\n" +
		"FILE \"sub\\..\\a.bin\" BINARY\nTRACK 03 MODE1/2352\nINDEX 01 00:00:02\n"
	bc, err := openTestCue(t, dir, cue)
	if err != nil {
		t.Fatalf("openBinCue: %v", err)
	}
	defer bc.Close()

	if len(bc.fds) != 1 {
		t.Fatalf("open handles = %d, want 1", len(bc.fds))
	}
	if len(bc.tracks) != 3 {
		t.Fatalf("tracks = %d, want 3", len(bc.tracks))
	}
}

// TestOpenBinCue_UnusedFileNotOpened tests that a FILE directive no track
// refers to does not open its file.
func TestOpenBinCue_UnusedFileNotOpened(t *testing.T) {
	dir := t.TempDir()
	writeTestBin(t, filepath.Join(dir, "a.bin"), 0x11)

	cue := "FILE \"missing.bin\" BINARY\n" +
		"FILE \"a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n"
	bc, err := openTestCue(t, dir, cue)
	if err != nil {
		t.Fatalf("openBinCue: %v", err)
	}
	defer bc.Close()

	if len(bc.fds) != 1 {
		t.Fatalf("open handles = %d, want 1", len(bc.fds))
	}
}

// TestOpenBinCue_TooManyFiles tests that a cue sheet using more distinct
// bin files than a CD can hold tracks is rejected.
func TestOpenBinCue_TooManyFiles(t *testing.T) {
	dir := t.TempDir()
	var cue strings.Builder
	for i := range maxCueFiles + 1 {
		name := fmt.Sprintf("t%03d.bin", i)
		writeTestBin(t, filepath.Join(dir, name), 0x11)
		fmt.Fprintf(&cue, "FILE %q BINARY\nTRACK %02d MODE1/2352\nINDEX 01 00:00:00\n", name, i+1)
	}

	bc, err := openTestCue(t, dir, cue.String())
	if err == nil {
		bc.Close()
		t.Fatalf("expected an error for %d bin files", maxCueFiles+1)
	}
	if !strings.Contains(err.Error(), "track files") {
		t.Fatalf("expected the track file limit error, got %v", err)
	}
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
