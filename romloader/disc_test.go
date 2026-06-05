package romloader

import (
	"reflect"
	"testing"
)

func TestDiscExtensions(t *testing.T) {
	got := DiscExtensions()
	want := []string{".chd", ".cue"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscExtensions() = %v, want %v", got, want)
	}
	// Every reported extension must be one OpenDisc dispatches on, so the list
	// can never advertise a format OpenDisc would reject as unsupported.
	for _, ext := range got {
		if _, ok := discOpeners[ext]; !ok {
			t.Errorf("DiscExtensions reports %q but discOpeners has no entry", ext)
		}
	}
}

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
