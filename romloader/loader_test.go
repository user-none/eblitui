package romloader

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// testExtensions is a common set of ROM extensions used across tests
var testExtensions = []string{".sms"}

// createTestROMFile creates a temporary ROM file with the given extension and test data
func createTestROMFile(t *testing.T, data []byte, ext string) string {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test"+ext)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("Failed to create test ROM file: %v", err)
	}
	return path
}

// createTestZipFile creates a temporary .zip file containing a ROM file
func createTestZipFile(t *testing.T, romData []byte, romName string) string {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.zip")

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	fw, err := w.Create(romName)
	if err != nil {
		t.Fatalf("Failed to create file in zip: %v", err)
	}
	if _, err := fw.Write(romData); err != nil {
		t.Fatalf("Failed to write to zip: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Failed to close zip: %v", err)
	}
	return path
}

// createTestGzipFile creates a temporary .gz file containing ROM data
func createTestGzipFile(t *testing.T, romData []byte, ext string) string {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test"+ext+".gz")

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create gzip file: %v", err)
	}
	defer f.Close()

	w := gzip.NewWriter(f)
	if _, err := w.Write(romData); err != nil {
		t.Fatalf("Failed to write to gzip: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Failed to close gzip: %v", err)
	}
	return path
}

// TestLoad_RawROM tests loading plain ROM files
func TestLoad_RawROM(t *testing.T) {
	testData := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	path := createTestROMFile(t, testData, ".sms")

	data, name, err := Load(path, testExtensions)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !bytes.Equal(data, testData) {
		t.Errorf("Data mismatch: expected %v, got %v", testData, data)
	}

	if name != "test.sms" {
		t.Errorf("Name mismatch: expected test.sms, got %s", name)
	}
}

// TestLoad_RawROMMultipleExtensions tests loading with multiple valid extensions
func TestLoad_RawROMMultipleExtensions(t *testing.T) {
	exts := []string{".sms", ".md", ".bin"}
	testData := []byte{0x01, 0x02, 0x03}

	for _, ext := range exts {
		path := createTestROMFile(t, testData, ext)
		data, name, err := Load(path, exts)
		if err != nil {
			t.Fatalf("Load failed for %s: %v", ext, err)
		}
		if !bytes.Equal(data, testData) {
			t.Errorf("Data mismatch for %s", ext)
		}
		if name != "test"+ext {
			t.Errorf("Name mismatch for %s: expected test%s, got %s", ext, ext, name)
		}
	}
}

// TestLoad_ZipArchive tests loading ROM from ZIP archives
func TestLoad_ZipArchive(t *testing.T) {
	testData := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	path := createTestZipFile(t, testData, "game.sms")

	data, name, err := Load(path, testExtensions)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !bytes.Equal(data, testData) {
		t.Errorf("Data mismatch: expected %v, got %v", testData, data)
	}

	if name != "game.sms" {
		t.Errorf("Name mismatch: expected game.sms, got %s", name)
	}
}

// TestLoad_GzipFile tests loading ROM from gzip files
func TestLoad_GzipFile(t *testing.T) {
	testData := []byte{0x11, 0x22, 0x33, 0x44, 0x55}
	path := createTestGzipFile(t, testData, ".sms")

	data, _, err := Load(path, testExtensions)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !bytes.Equal(data, testData) {
		t.Errorf("Data mismatch: expected %v, got %v", testData, data)
	}
}

// TestLoad_FormatDetectionMagic tests detection via magic bytes
func TestLoad_FormatDetectionMagic(t *testing.T) {
	testCases := []struct {
		header   []byte
		path     string
		expected formatType
	}{
		{[]byte{0x50, 0x4B, 0x03, 0x04}, "file.dat", formatZIP},
		{[]byte{0x50, 0x4B, 0x05, 0x06}, "file.dat", formatZIP},
		{[]byte{0x37, 0x7A, 0xBC, 0xAF, 0x27, 0x1C}, "file.dat", format7z},
		{[]byte{0x1F, 0x8B}, "file.dat", formatGzip},
		{[]byte{0x52, 0x61, 0x72, 0x21}, "file.dat", formatRAR},
	}

	for _, tc := range testCases {
		result := detectFormat(tc.header, tc.path, testExtensions)
		if result != tc.expected {
			t.Errorf("detectFormat(%v, %s): expected %d, got %d", tc.header, tc.path, tc.expected, result)
		}
	}
}

// TestLoad_FormatDetectionExtension tests fallback to extension
func TestLoad_FormatDetectionExtension(t *testing.T) {
	testCases := []struct {
		path     string
		expected formatType
	}{
		{"game.sms", formatRaw},
		{"game.SMS", formatRaw},
		{"game.zip", formatZIP},
		{"game.ZIP", formatZIP},
		{"game.7z", format7z},
		{"game.gz", formatGzip},
		{"game.tgz", formatGzip},
		{"game.tar.gz", formatGzip},
		{"game.rar", formatRAR},
		{"game.unknown", formatUnknown},
	}

	for _, tc := range testCases {
		// Use empty header to force extension-based detection
		result := detectFormat([]byte{}, tc.path, testExtensions)
		if result != tc.expected {
			t.Errorf("detectFormat([], %s): expected %d, got %d", tc.path, tc.expected, result)
		}
	}
}

// TestLoad_FormatDetectionCustomExtensions tests extension detection with non-SMS extensions
func TestLoad_FormatDetectionCustomExtensions(t *testing.T) {
	mdExts := []string{".md", ".bin", ".gen"}
	testCases := []struct {
		path     string
		expected formatType
	}{
		{"game.md", formatRaw},
		{"game.bin", formatRaw},
		{"game.gen", formatRaw},
		{"game.sms", formatUnknown}, // .sms not in mdExts
		{"game.zip", formatZIP},     // archive formats always detected
	}

	for _, tc := range testCases {
		result := detectFormat([]byte{}, tc.path, mdExts)
		if result != tc.expected {
			t.Errorf("detectFormat([], %s, mdExts): expected %d, got %d", tc.path, tc.expected, result)
		}
	}
}

// TestLoad_NoROMInArchive tests error when no matching ROM found in archive
func TestLoad_NoROMInArchive(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.zip")

	// Create zip with non-ROM file
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create zip: %v", err)
	}

	w := zip.NewWriter(f)
	fw, _ := w.Create("readme.txt")
	fw.Write([]byte("hello"))
	w.Close()
	f.Close()

	_, _, err = Load(path, testExtensions)
	if err == nil {
		t.Error("Expected error when no ROM file in archive")
	}
	if err != ErrNoROMFile {
		t.Errorf("Expected ErrNoROMFile, got %v", err)
	}
}

// TestLoad_FileTooLarge tests rejection of files exceeding size limit
func TestLoad_FileTooLarge(t *testing.T) {
	// Create a large file that exceeds maxROMSize
	largeData := make([]byte, maxROMSize+1)

	tmpDir := t.TempDir()

	// Test with a gzip file
	gzPath := filepath.Join(tmpDir, "large.sms.gz")
	f, err := os.Create(gzPath)
	if err != nil {
		t.Fatalf("Failed to create gzip: %v", err)
	}

	w := gzip.NewWriter(f)
	w.Write(largeData)
	w.Close()
	f.Close()

	_, _, err = Load(gzPath, testExtensions)
	if err == nil {
		t.Error("Expected error for oversized file")
	}
}

// TestLoad_FileNotFound tests error for missing files
func TestLoad_FileNotFound(t *testing.T) {
	_, _, err := Load("/nonexistent/path/game.sms", testExtensions)
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

// TestLoad_IsROMFile tests the ROM file extension check
func TestLoad_IsROMFile(t *testing.T) {
	smsExts := []string{".sms"}
	testCases := []struct {
		name     string
		expected bool
	}{
		{"game.sms", true},
		{"game.SMS", true},
		{"game.Sms", true},
		{"game.txt", false},
		{"game.sms.bak", false},
		{"game", false},
		{"sms", false},
		{".sms", true},
	}

	for _, tc := range testCases {
		result := isROMFile(tc.name, smsExts)
		if result != tc.expected {
			t.Errorf("isROMFile(%q, smsExts): expected %v, got %v", tc.name, tc.expected, result)
		}
	}
}

// TestLoad_IsROMFileMultipleExtensions tests isROMFile with multiple extensions
func TestLoad_IsROMFileMultipleExtensions(t *testing.T) {
	exts := []string{".sms", ".md", ".bin"}
	testCases := []struct {
		name     string
		expected bool
	}{
		{"game.sms", true},
		{"game.md", true},
		{"game.bin", true},
		{"game.BIN", true},
		{"game.gen", false},
		{"game.txt", false},
	}

	for _, tc := range testCases {
		result := isROMFile(tc.name, exts)
		if result != tc.expected {
			t.Errorf("isROMFile(%q, multiExts): expected %v, got %v", tc.name, tc.expected, result)
		}
	}
}

// TestLoad_ZipWithSubdirectory tests extracting ROM from nested directory
func TestLoad_ZipWithSubdirectory(t *testing.T) {
	testData := []byte{0x12, 0x34, 0x56}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.zip")

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create zip: %v", err)
	}

	w := zip.NewWriter(f)
	// Create file in subdirectory
	fw, _ := w.Create("roms/games/test.sms")
	fw.Write(testData)
	w.Close()
	f.Close()

	data, name, err := Load(path, testExtensions)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !bytes.Equal(data, testData) {
		t.Errorf("Data mismatch: expected %v, got %v", testData, data)
	}

	if name != "test.sms" {
		t.Errorf("Name should be just the filename, got %s", name)
	}
}

// TestLoad_EmptyFile tests handling of empty files
func TestLoad_EmptyFile(t *testing.T) {
	path := createTestROMFile(t, []byte{}, ".sms")

	data, _, err := Load(path, testExtensions)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(data) != 0 {
		t.Errorf("Expected empty data, got %d bytes", len(data))
	}
}

// TestLoad_MaxROMSizeConstant tests that the size limit is reasonable
func TestLoad_MaxROMSizeConstant(t *testing.T) {
	if maxROMSize < 4*1024*1024 {
		t.Errorf("maxROMSize too small: %d bytes (should be at least 4MB)", maxROMSize)
	}
	if maxROMSize > 16*1024*1024 {
		t.Errorf("maxROMSize unexpectedly large: %d bytes", maxROMSize)
	}
}

// TestLoad_MagicBytesDefinition tests that magic byte arrays are correct
func TestLoad_MagicBytesDefinition(t *testing.T) {
	// ZIP magic: "PK\x03\x04"
	if !bytes.Equal(magicZIP, []byte{0x50, 0x4B, 0x03, 0x04}) {
		t.Error("ZIP magic bytes incorrect")
	}

	// 7z magic
	if !bytes.Equal(magic7z, []byte{0x37, 0x7A, 0xBC, 0xAF, 0x27, 0x1C}) {
		t.Error("7z magic bytes incorrect")
	}

	// Gzip magic
	if !bytes.Equal(magicGzip, []byte{0x1F, 0x8B}) {
		t.Error("Gzip magic bytes incorrect")
	}

	// RAR magic: "Rar!"
	if !bytes.Equal(magicRAR, []byte{0x52, 0x61, 0x72, 0x21}) {
		t.Error("RAR magic bytes incorrect")
	}
}

// TestLoad_UnsupportedExtension tests that unsupported extensions return error
func TestLoad_UnsupportedExtension(t *testing.T) {
	testData := []byte{0x01, 0x02, 0x03}
	path := createTestROMFile(t, testData, ".xyz")

	_, _, err := Load(path, testExtensions)
	if err == nil {
		t.Error("Expected error for unsupported extension")
	}
}
