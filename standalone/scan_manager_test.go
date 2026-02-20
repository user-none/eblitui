//go:build !libretro

package standalone

import (
	"testing"

	"github.com/user-none/eblitui/standalone/storage"
)

func TestScanManagerIsScanning(t *testing.T) {
	sm := &ScanManager{}

	if sm.IsScanning() {
		t.Error("should not be scanning initially")
	}
}

func TestScanManagerCancelNilScanner(t *testing.T) {
	sm := &ScanManager{}
	// Should not panic
	sm.Cancel()
}

func TestScanManagerSetLibrary(t *testing.T) {
	sm := &ScanManager{}
	lib := storage.DefaultLibrary()

	sm.SetLibrary(lib)
	if sm.library != lib {
		t.Error("library should be set")
	}
}

func TestScanManagerUpdateNilScanner(t *testing.T) {
	sm := &ScanManager{}
	// Should not panic when scanner is nil
	sm.Update()
}
