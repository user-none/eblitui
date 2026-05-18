package romloader

import "testing"

func TestOpenDiscMissingFile(t *testing.T) {
	d, err := OpenDisc("does-not-exist.chd")
	if err == nil {
		d.Close()
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestOpenDiscNotACHD(t *testing.T) {
	// An existing non-CHD file: chd.Open must reject the magic and
	// OpenDisc must return the error (and not return a usable Disc).
	d, err := OpenDisc("go.mod")
	if err == nil {
		d.Close()
		t.Fatal("expected error for non-CHD file, got nil")
	}
}
