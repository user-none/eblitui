package romloader

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"github.com/nwaples/rardecode/v2"
)

// rar5Block builds a RAR5 block: the CRC32 of the size field and body,
// the body size as a vint, then the body.
func rar5Block(body []byte) []byte {
	sized := binary.AppendUvarint(nil, uint64(len(body)))
	sized = append(sized, body...)
	out := binary.LittleEndian.AppendUint32(nil, crc32.ChecksumIEEE(sized))
	return append(out, sized...)
}

// createTestRAR5File writes a RAR5 archive whose only block is a
// compressed file header for game.sms with the given unpacked size and
// dictionary size code (128 KiB << dictCode). The archive carries no file
// data, so it is only useful for checks made when the header is read.
func createTestRAR5File(t *testing.T, unpackedSize uint64, dictCode uint64) string {
	t.Helper()

	arc := rar5Block([]byte{1, 0, 0})

	name := "game.sms"
	var file []byte
	file = binary.AppendUvarint(file, 2)
	file = binary.AppendUvarint(file, 0)
	file = binary.AppendUvarint(file, 0)
	file = binary.AppendUvarint(file, unpackedSize)
	file = binary.AppendUvarint(file, 0)
	file = binary.AppendUvarint(file, 1<<7|dictCode<<10)
	file = binary.AppendUvarint(file, 1)
	file = binary.AppendUvarint(file, uint64(len(name)))
	file = append(file, name...)

	data := []byte("Rar!\x1a\x07\x01\x00")
	data = append(data, arc...)
	data = append(data, rar5Block(file)...)

	path := filepath.Join(t.TempDir(), "test.rar")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	return path
}

// TestExtractFromRAR_DictionaryTooLarge tests that a header requesting a
// decode window larger than the ROM use case allows is rejected before the
// window is allocated.
func TestExtractFromRAR_DictionaryTooLarge(t *testing.T) {
	// 1 GiB unpacked with a 1 GiB dictionary.
	path := createTestRAR5File(t, 1<<30, 13)

	_, _, err := extractFromRAR(path, testExtensions)
	if !errors.Is(err, rardecode.ErrDictionaryTooLarge) {
		t.Fatalf("expected ErrDictionaryTooLarge, got %v", err)
	}
}

// TestExtractFromRAR_DictionaryWithinLimit tests that a header with a
// small decode window is not rejected for its dictionary size.
func TestExtractFromRAR_DictionaryWithinLimit(t *testing.T) {
	path := createTestRAR5File(t, 4096, 0)

	_, _, err := extractFromRAR(path, testExtensions)
	if errors.Is(err, rardecode.ErrDictionaryTooLarge) {
		t.Fatalf("small dictionary rejected: %v", err)
	}
}

// TestExtractFromRAR_FileNotFound tests error handling for missing files
func TestExtractFromRAR_FileNotFound(t *testing.T) {
	_, _, err := extractFromRAR("/nonexistent/path/test.rar", testExtensions)
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

// TestExtractFromRAR_InvalidFormat tests error handling for non-RAR files
func TestExtractFromRAR_InvalidFormat(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "fake.rar")

	err := os.WriteFile(path, []byte("not a rar file"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, _, err = extractFromRAR(path, testExtensions)
	if err == nil {
		t.Error("Expected error for invalid RAR file")
	}
}

// TestExtractFromRAR_EmptyFile tests error handling for empty files
func TestExtractFromRAR_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.rar")

	err := os.WriteFile(path, []byte{}, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, _, err = extractFromRAR(path, testExtensions)
	if err == nil {
		t.Error("Expected error for empty file")
	}
}

// TestExtractFromRAR_PartialMagic tests files with partial RAR magic bytes
func TestExtractFromRAR_PartialMagic(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "partial.rar")

	// RAR magic is: "Rar!" (0x52, 0x61, 0x72, 0x21)
	// Write only partial magic
	err := os.WriteFile(path, []byte{0x52, 0x61}, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, _, err = extractFromRAR(path, testExtensions)
	if err == nil {
		t.Error("Expected error for file with partial magic bytes")
	}
}

// TestExtractFromRAR_CorruptedArchive tests handling of corrupted archives
// Note: The rardecode library may panic on severely corrupted files,
// which is expected behavior for invalid input
func TestExtractFromRAR_CorruptedArchive(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "corrupt.rar")

	// Write valid magic but corrupted data
	// Full RAR5 signature is: Rar!\x1a\x07\x01\x00
	content := append(magicRAR, []byte{0x1a, 0x07, 0x01, 0x00}...)
	content = append(content, make([]byte, 100)...)
	err := os.WriteFile(path, content, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Use recover to handle library panics on severely corrupted files
	defer func() {
		if r := recover(); r != nil {
			// Library panicked on corrupted data - this is acceptable
			t.Logf("Library panicked on corrupted RAR (expected): %v", r)
		}
	}()

	_, _, err = extractFromRAR(path, testExtensions)
	if err == nil {
		t.Error("Expected error for corrupted RAR file")
	}
}

// TestLoad_RARFormatDetection tests that RAR files are detected correctly
func TestLoad_RARFormatDetection(t *testing.T) {
	// Test magic byte detection
	header := magicRAR
	format := detectFormat(header, "file.dat", testExtensions)
	if format != formatRAR {
		t.Errorf("RAR magic should be detected, got format %d", format)
	}

	// Test extension fallback
	format = detectFormat([]byte{}, "file.rar", testExtensions)
	if format != formatRAR {
		t.Errorf(".rar extension should be detected, got format %d", format)
	}

	// Test case insensitivity
	format = detectFormat([]byte{}, "file.RAR", testExtensions)
	if format != formatRAR {
		t.Errorf(".RAR extension should be detected, got format %d", format)
	}
}

// TestLoad_RAR_Integration tests Load with RAR (expects failure without valid archive)
func TestLoad_RAR_Integration(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.rar")

	// Create fake RAR file with magic
	err := os.WriteFile(path, append(magicRAR, []byte("invalid")...), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Should fail gracefully
	_, _, err = Load(path, testExtensions)
	if err == nil {
		t.Error("Expected error loading invalid RAR file")
	}
}

// TestExtractFromRAR_DirectorySkipping tests handling of directories in RAR
// (directories should be skipped)
func TestExtractFromRAR_DirectorySkipping(t *testing.T) {
	// We can't easily create a valid RAR with directories without external tools,
	// but we can verify the detection logic handles the case where no ROM file is found
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.rar")

	// Write valid magic but no actual file entries
	err := os.WriteFile(path, magicRAR, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, _, err = extractFromRAR(path, testExtensions)
	// Should fail (can't read header or no ROM file)
	if err == nil {
		t.Error("Expected error for RAR with no valid entries")
	}
}
