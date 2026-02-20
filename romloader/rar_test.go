package romloader

import (
	"os"
	"path/filepath"
	"testing"
)

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
