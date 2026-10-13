package desktop

import (
	"testing"
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

func TestScanManagerUpdateNilScanner(t *testing.T) {
	sm := &ScanManager{}
	// Should not panic when scanner is nil
	sm.Update()
}
