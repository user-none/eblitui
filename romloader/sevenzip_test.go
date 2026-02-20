package romloader

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExtractFrom7z_FileNotFound tests error handling for missing files
func TestExtractFrom7z_FileNotFound(t *testing.T) {
	_, _, err := extractFrom7z("/nonexistent/path/test.7z", testExtensions)
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

// TestExtractFrom7z_InvalidFormat tests error handling for non-7z files
func TestExtractFrom7z_InvalidFormat(t *testing.T) {
	// Create a file with invalid 7z content
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "fake.7z")

	err := os.WriteFile(path, []byte("not a 7z file"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, _, err = extractFrom7z(path, testExtensions)
	if err == nil {
		t.Error("Expected error for invalid 7z file")
	}
}

// TestExtractFrom7z_EmptyFile tests error handling for empty files
func TestExtractFrom7z_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.7z")

	err := os.WriteFile(path, []byte{}, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, _, err = extractFrom7z(path, testExtensions)
	if err == nil {
		t.Error("Expected error for empty file")
	}
}

// TestExtractFrom7z_PartialMagic tests files with partial 7z magic bytes
func TestExtractFrom7z_PartialMagic(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "partial.7z")

	// 7z magic is: 0x37, 0x7A, 0xBC, 0xAF, 0x27, 0x1C
	// Write only partial magic
	err := os.WriteFile(path, []byte{0x37, 0x7A, 0xBC}, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, _, err = extractFrom7z(path, testExtensions)
	if err == nil {
		t.Error("Expected error for file with partial magic bytes")
	}
}

// TestExtractFrom7z_CorruptedArchive tests handling of corrupted archives
func TestExtractFrom7z_CorruptedArchive(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "corrupt.7z")

	// Write valid magic but corrupted data
	content := append(magic7z, make([]byte, 100)...)
	err := os.WriteFile(path, content, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, _, err = extractFrom7z(path, testExtensions)
	if err == nil {
		t.Error("Expected error for corrupted 7z file")
	}
}

// TestLoad_7zFormatDetection tests that 7z files are detected correctly
func TestLoad_7zFormatDetection(t *testing.T) {
	// Test magic byte detection
	header := magic7z
	format := detectFormat(header, "file.dat", testExtensions)
	if format != format7z {
		t.Errorf("7z magic should be detected, got format %d", format)
	}

	// Test extension fallback
	format = detectFormat([]byte{}, "file.7z", testExtensions)
	if format != format7z {
		t.Errorf(".7z extension should be detected, got format %d", format)
	}
}

// TestLoad_7z_Integration tests Load with 7z (expects failure without valid archive)
func TestLoad_7z_Integration(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.7z")

	// Create fake 7z file
	err := os.WriteFile(path, append(magic7z, []byte("invalid")...), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Should fail gracefully
	_, _, err = Load(path, testExtensions)
	if err == nil {
		t.Error("Expected error loading invalid 7z file")
	}
}
