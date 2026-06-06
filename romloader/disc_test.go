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

// fakeTrack and fakeDiscBackend provide a minimal discBackend so Disc-level
// logic (ReadSectorData / sectorHeader) can be exercised without a real image.
type fakeTrack struct {
	number   int
	typ      string
	frames   int
	pregap   int
	startLBA int
	control  uint8
}

type fakeDiscBackend struct {
	tracks []fakeTrack
}

// ReadSector returns a 2352-byte sector whose byte k holds byte(k), so a cooked
// read's first byte reveals the header offset that was stripped.
func (b *fakeDiscBackend) ReadSector(lba int) ([]byte, error) {
	raw := make([]byte, 2352)
	for i := range raw {
		raw[i] = byte(i)
	}
	return raw, nil
}

func (b *fakeDiscBackend) NumTracks() int { return len(b.tracks) }

func (b *fakeDiscBackend) Track(i int) (int, string, int, int, int, uint8) {
	t := b.tracks[i]
	return t.number, t.typ, t.frames, t.pregap, t.startLBA, t.control
}

func (b *fakeDiscBackend) NumTrackIndexes(i int) int { return 1 }

func (b *fakeDiscBackend) TrackIndex(i, n int) (int, int) { return 1, b.tracks[i].startLBA }

func (b *fakeDiscBackend) Close() error { return nil }

func TestReadSectorData(t *testing.T) {
	cases := []struct {
		name      string
		typ       string
		wantLen   int
		wantFirst byte // byte(header offset) for data; 0 (raw[0]) for audio
	}{
		{"mode1", "MODE1_RAW", cookedUserDataSize, mode1HeaderSize},
		{"mode2", "MODE2_RAW", cookedUserDataSize, mode2HeaderSize},
		{"audio", "AUDIO", 2352, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &Disc{backend: &fakeDiscBackend{tracks: []fakeTrack{
				{number: 1, typ: tc.typ, frames: 100, startLBA: 0, control: 0x41},
			}}}
			data, err := d.ReadSectorData(0)
			if err != nil {
				t.Fatalf("ReadSectorData: %v", err)
			}
			if len(data) != tc.wantLen {
				t.Fatalf("len = %d, want %d", len(data), tc.wantLen)
			}
			if data[0] != tc.wantFirst {
				t.Fatalf("data[0] = %d, want %d (stripped header offset)", data[0], tc.wantFirst)
			}
		})
	}
}

func TestReadSectorDataMultiTrack(t *testing.T) {
	// Track 1 data at LBA 0..149, track 2 audio at LBA 150+. sectorHeader must
	// pick the track containing the LBA.
	d := &Disc{backend: &fakeDiscBackend{tracks: []fakeTrack{
		{number: 1, typ: "MODE1_RAW", frames: 150, startLBA: 0, control: 0x41},
		{number: 2, typ: "AUDIO", frames: 300, startLBA: 150, control: 0x01},
	}}}

	d1, err := d.ReadSectorData(10)
	if err != nil {
		t.Fatalf("data track: %v", err)
	}
	if len(d1) != cookedUserDataSize || d1[0] != mode1HeaderSize {
		t.Fatalf("data track: len=%d data[0]=%d, want %d/%d", len(d1), d1[0], cookedUserDataSize, mode1HeaderSize)
	}

	d2, err := d.ReadSectorData(200)
	if err != nil {
		t.Fatalf("audio track: %v", err)
	}
	if len(d2) != 2352 {
		t.Fatalf("audio track: len=%d, want 2352", len(d2))
	}
}
