// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"crypto/md5"
	"crypto/sha1"
	"errors"
	"fmt"
	"hash/crc32"
	"strings"
	"testing"
)

// fakeTrack describes one track of a fakeDisc.
type fakeTrack struct {
	number   int
	typ      string
	frames   int
	pregap   int
	startLBA int
	control  uint8
}

// fakeDisc is an in-memory discReader. Each sector is deterministically
// derived from its LBA so tests can recompute expected hashes independently.
type fakeDisc struct {
	tracks     []fakeTrack
	sectorSize int
	header     []byte         // optional sector-0 user-data override (raw bytes from offset 16)
	isoSectors map[int][]byte // optional LBA -> ISO logical sector, served as Mode 2 Form 1
	rawSectors map[int][]byte // optional LBA -> full raw 2352-byte sector, returned verbatim
}

func (d *fakeDisc) NumTracks() int { return len(d.tracks) }

func (d *fakeDisc) Track(i int) (int, string, int, int, int, uint8) {
	t := d.tracks[i]
	return t.number, t.typ, t.frames, t.pregap, t.startLBA, t.control
}

func (d *fakeDisc) ReadSector(lba int) ([]byte, error) {
	if sec, ok := d.rawSectors[lba]; ok {
		return sec, nil
	}
	data := make([]byte, d.sectorSize)
	if user, ok := d.isoSectors[lba]; ok {
		data[15] = 0x02 // Mode 2 -> user data at offset 24
		copy(data[24:], user)
		return data, nil
	}
	for i := range data {
		data[i] = byte((lba*31 + i*7) & 0xFF)
	}
	if lba == 0 && d.header != nil {
		copy(data[16:], d.header)
	}
	return data, nil
}

// concatTracks rebuilds the exact byte stream hashDisc feeds its hashers for
// one track, using the same sector derivation as fakeDisc.ReadSector.
func (d *fakeDisc) concatTrack(t fakeTrack) []byte {
	buf := make([]byte, 0, t.frames*d.sectorSize)
	for s := 0; s < t.frames; s++ {
		lba := t.startLBA + s
		sec := make([]byte, d.sectorSize)
		for i := range sec {
			sec[i] = byte((lba*31 + i*7) & 0xFF)
		}
		buf = append(buf, sec...)
	}
	return buf
}

func newFakeDisc() *fakeDisc {
	return &fakeDisc{
		sectorSize: 2352,
		tracks: []fakeTrack{
			{number: 1, typ: "MODE1_RAW", frames: 40, pregap: 0, startLBA: 0, control: 0x41},
			{number: 2, typ: "AUDIO", frames: 25, pregap: 150, startLBA: 40, control: 0x01},
			{number: 3, typ: "AUDIO", frames: 10, pregap: 150, startLBA: 65, control: 0x01},
		},
	}
}

func TestFramesToMSF(t *testing.T) {
	cases := []struct {
		frames int
		want   string
	}{
		{0, "00:00:00"},
		{75, "00:01:00"},
		{150, "00:02:00"},
		{71908, "15:58:58"},
		{214396, "47:38:46"},
		{-5, "00:00:00"},
	}
	for _, c := range cases {
		if got := framesToMSF(c.frames); got != c.want {
			t.Errorf("framesToMSF(%d) = %q, want %q", c.frames, got, c.want)
		}
	}
}

func TestTrackTypeLabel(t *testing.T) {
	cases := []struct {
		typ  string
		want string
	}{
		{"AUDIO", "Audio"},
		{"MODE1", "Data/Mode 1"},
		{"MODE1_RAW", "Data/Mode 1"},
		{"MODE2", "Data/Mode 2"},
		{"MODE2_FORM1", "Data/Mode 2"},
		{"WEIRD", "WEIRD"},
	}
	for _, c := range cases {
		if got := trackTypeLabel(c.typ); got != c.want {
			t.Errorf("trackTypeLabel(%q) = %q, want %q", c.typ, got, c.want)
		}
	}
}

func TestIdentifySaturn(t *testing.T) {
	// Build a valid IP header in user-data space (offset 0 = identifier).
	hdr := make([]byte, 0xD0)
	for i := range hdr {
		hdr[i] = ' '
	}
	copy(hdr[0x00:], "SEGA SEGASATURN ")
	copy(hdr[0x20:], "T-1234G   ")
	copy(hdr[0x60:], "GAME TITLE")
	d := newFakeDisc()
	d.header = hdr

	id, name, ok := identifySaturn(d)
	if !ok || id != "T-1234G" || name != "GAME TITLE" {
		t.Errorf("identifySaturn = (%q, %q, %v), want T-1234G / GAME TITLE / true", id, name, ok)
	}
	if sys, gotID, gotName := identifyDisc(d); sys != "Saturn" || gotID != "T-1234G" || gotName != "GAME TITLE" {
		t.Errorf("identifyDisc = (%q, %q, %q), want Saturn / T-1234G / GAME TITLE", sys, gotID, gotName)
	}

	// Non-Saturn sector 0 does not match.
	if _, _, ok := identifySaturn(newFakeDisc()); ok {
		t.Error("identifySaturn matched a non-Saturn disc")
	}
}

// dirRecord builds an ISO 9660 directory record for name pointing at the given
// extent LBA and byte size.
func dirRecord(name string, extent, size int) []byte {
	rec := make([]byte, 33+len(name))
	rec[2], rec[3], rec[4], rec[5] = byte(extent), byte(extent>>8), byte(extent>>16), byte(extent>>24)
	rec[10], rec[11], rec[12], rec[13] = byte(size), byte(size>>8), byte(size>>16), byte(size>>24)
	rec[32] = byte(len(name))
	copy(rec[33:], name)
	if len(rec)%2 == 1 {
		rec = append(rec, 0)
	}
	rec[0] = byte(len(rec))
	return rec
}

// newISODisc builds a fake disc with a minimal ISO 9660 layout: PVD at LBA 16,
// root directory at LBA 20, and SYSTEM.CNF at LBA 30 holding cnf.
func newISODisc(cnf string) *fakeDisc {
	pvd := make([]byte, 2048)
	pvd[0] = 1
	copy(pvd[1:], "CD001")
	copy(pvd[156:], dirRecord("\x00", 20, 2048)) // root dir record

	dir := make([]byte, 2048)
	p := 0
	for _, r := range [][]byte{
		dirRecord("\x00", 20, 2048),
		dirRecord("\x01", 20, 2048),
		dirRecord("SYSTEM.CNF;1", 30, len(cnf)),
	} {
		copy(dir[p:], r)
		p += len(r)
	}

	return &fakeDisc{
		sectorSize: 2352,
		isoSectors: map[int][]byte{16: pvd, 20: dir, 30: []byte(cnf)},
	}
}

func TestIdentifyDreamcast(t *testing.T) {
	sec := make([]byte, 2352)
	sec[15] = 0x01 // Mode 1 -> user data at offset 16
	u := sec[16:]
	for i := 0; i < 0x100; i++ {
		u[i] = ' ' // IP.BIN fields are space-padded
	}
	copy(u[0x00:], "SEGA SEGAKATANA ") // hardware identifier (format magic)
	copy(u[0x40:], "T-000DC")          // synthetic product number
	copy(u[0x80:], "DREAMCAST TEST")   // synthetic title

	d := &fakeDisc{
		sectorSize: 2352,
		tracks: []fakeTrack{
			{number: 1, typ: "MODE1_RAW", frames: 100, startLBA: 0, control: 0x41},
			{number: 2, typ: "AUDIO", frames: 100, startLBA: 100, control: 0x01},
			{number: 3, typ: "MODE1_RAW", frames: 100, startLBA: 45000, control: 0x41},
		},
		rawSectors: map[int][]byte{45000: sec}, // IP.BIN on the high-density data track
	}

	id, name, ok := identifyDreamcast(d)
	if !ok || id != "T-000DC" || name != "DREAMCAST TEST" {
		t.Errorf("identifyDreamcast = (%q, %q, %v), want T-000DC / DREAMCAST TEST / true", id, name, ok)
	}
	if sys, gotID, gotName := identifyDisc(d); sys != "Dreamcast" || gotID != "T-000DC" || gotName != "DREAMCAST TEST" {
		t.Errorf("identifyDisc = (%q, %q, %q), want Dreamcast / T-000DC / DREAMCAST TEST", sys, gotID, gotName)
	}
}

func TestIdentifyPS1(t *testing.T) {
	d := newISODisc("BOOT = cdrom:\\SLUS_012.34;1\nTCB = 4\n")
	id, name, ok := identifyPS1(d)
	if !ok || id != "SLUS-01234" || name != "" {
		t.Errorf("identifyPS1 = (%q, %q, %v), want SLUS-01234 / empty / true", id, name, ok)
	}
	if sys, gotID, _ := identifyDisc(d); sys != "PS1" || gotID != "SLUS-01234" {
		t.Errorf("identifyDisc = (%q, %q), want PS1 / SLUS-01234", sys, gotID)
	}
}

func TestIdentifyPS1Negatives(t *testing.T) {
	// PS2 (BOOT2) must not match the PS1 detector.
	if _, _, ok := identifyPS1(newISODisc("BOOT2 = cdrom0:\\SLUS_200.00;1\n")); ok {
		t.Error("identifyPS1 matched a PS2 (BOOT2) disc")
	}
	// ISO with no SYSTEM.CNF.
	noCnf := &fakeDisc{sectorSize: 2352, isoSectors: map[int][]byte{
		16: func() []byte {
			p := make([]byte, 2048)
			p[0] = 1
			copy(p[1:], "CD001")
			copy(p[156:], dirRecord("\x00", 20, 2048))
			return p
		}(),
		20: make([]byte, 2048),
	}}
	if _, _, ok := identifyPS1(noCnf); ok {
		t.Error("identifyPS1 matched a disc with no SYSTEM.CNF")
	}
	// Non-ISO disc (no CD001).
	if _, _, ok := identifyPS1(newFakeDisc()); ok {
		t.Error("identifyPS1 matched a non-ISO disc")
	}
}

func TestWriteSummaryColumns(t *testing.T) {
	// PS1-only: no titles, so the NAME column must not appear.
	var ps1 strings.Builder
	writeSummary(&ps1, []summaryRow{
		{crc: "11111111", id: "SLUS-00001", system: "PS1", file: "a.chd"},
	})
	if strings.Contains(ps1.String(), "NAME") {
		t.Errorf("PS1-only summary should omit NAME column:\n%s", ps1.String())
	}
	for _, want := range []string{"CRC32", "ID", "SYSTEM", "FILE", "SLUS-00001"} {
		if !strings.Contains(ps1.String(), want) {
			t.Errorf("PS1 summary missing %q:\n%s", want, ps1.String())
		}
	}

	// A row with a title keeps the NAME column.
	var sat strings.Builder
	writeSummary(&sat, []summaryRow{
		{crc: "22222222", id: "T-000SAT", name: "SATURN TEST", system: "Saturn", file: "b.chd"},
	})
	if !strings.Contains(sat.String(), "NAME") || !strings.Contains(sat.String(), "SATURN TEST") {
		t.Errorf("Saturn summary should include NAME column:\n%s", sat.String())
	}
}

func TestHashDiscNoTracks(t *testing.T) {
	d := &fakeDisc{sectorSize: 2352} // no tracks
	if _, err := hashDisc(d, false); !errors.Is(err, errUnreadable) {
		t.Errorf("hashDisc 0-track err = %v, want errUnreadable", err)
	}
}

func TestWriteSummaryErrorRow(t *testing.T) {
	var b strings.Builder
	writeSummary(&b, []summaryRow{
		{crc: "ERROR", file: "a.chd"},
		{crc: "22222222", id: "T-000SAT", name: "SATURN TEST", system: "Saturn", file: "b.chd"},
	})
	out := b.String()
	if !strings.Contains(out, "ERROR") {
		t.Errorf("summary missing ERROR row:\n%s", out)
	}
	if strings.Contains(out, "00000000") {
		t.Errorf("summary should not show a zero hash:\n%s", out)
	}
}

func TestParallelMapOrder(t *testing.T) {
	for _, workers := range []int{0, 1, 3, 8} {
		got := parallelMap(100, workers, func(i int) int { return i * 2 })
		if len(got) != 100 {
			t.Fatalf("workers=%d: len=%d, want 100", workers, len(got))
		}
		for i, v := range got {
			if v != i*2 {
				t.Fatalf("workers=%d: got[%d]=%d, want %d", workers, i, v, i*2)
			}
		}
	}
}

func TestParseBootSerial(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"BOOT = cdrom:\\SLPS_000.99;1", "SLPS-00099", true},
		{"BOOT=cdrom:\\SLUS_000.12;1", "SLUS-00012", true},
		{"BOOT2 = cdrom0:\\SLUS_000.34;1", "", false},
		{"TCB = 4", "", false},
	}
	for _, c := range cases {
		got, ok := parseBootSerial(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("parseBootSerial(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestHashDiscOverall(t *testing.T) {
	d := newFakeDisc()

	var all []byte
	totalSectors := 0
	for _, tr := range d.tracks {
		all = append(all, d.concatTrack(tr)...)
		totalSectors += tr.frames
	}
	wantCRC := crc32.ChecksumIEEE(all)

	res, err := hashDisc(d, false)
	if err != nil {
		t.Fatalf("hashDisc: %v", err)
	}
	if res.overallCRC != wantCRC {
		t.Errorf("overallCRC = %08x, want %08x", res.overallCRC, wantCRC)
	}
	if res.totalSectors != totalSectors {
		t.Errorf("totalSectors = %d, want %d", res.totalSectors, totalSectors)
	}
	if want := int64(totalSectors * d.sectorSize); res.totalBytes != want {
		t.Errorf("totalBytes = %d, want %d", res.totalBytes, want)
	}
	// Default mode does not populate per-track results.
	if len(res.tracks) != 0 {
		t.Errorf("non-verbose tracks = %d, want 0", len(res.tracks))
	}
}

func TestHashDiscVerboseTracks(t *testing.T) {
	d := newFakeDisc()

	res, err := hashDisc(d, true)
	if err != nil {
		t.Fatalf("hashDisc: %v", err)
	}
	if len(res.tracks) != len(d.tracks) {
		t.Fatalf("tracks = %d, want %d", len(res.tracks), len(d.tracks))
	}

	for i, tr := range d.tracks {
		body := d.concatTrack(tr)
		wantCRC := crc32.ChecksumIEEE(body)
		wantMD5 := md5.Sum(body)
		wantSHA1 := sha1.Sum(body)

		got := res.tracks[i]
		if got.number != tr.number {
			t.Errorf("track %d number = %d, want %d", i, got.number, tr.number)
		}
		if got.crc32 != wantCRC {
			t.Errorf("track %d crc32 = %08x, want %08x", tr.number, got.crc32, wantCRC)
		}
		if fmt.Sprintf("%x", got.md5) != fmt.Sprintf("%x", wantMD5[:]) {
			t.Errorf("track %d md5 = %x, want %x", tr.number, got.md5, wantMD5)
		}
		if fmt.Sprintf("%x", got.sha1) != fmt.Sprintf("%x", wantSHA1[:]) {
			t.Errorf("track %d sha1 = %x, want %x", tr.number, got.sha1, wantSHA1)
		}
		if want := int64(tr.frames * d.sectorSize); got.bytes != want {
			t.Errorf("track %d bytes = %d, want %d", tr.number, got.bytes, want)
		}
	}
}

func TestHashDiscReadError(t *testing.T) {
	d := &errDisc{}
	if _, err := hashDisc(d, false); err == nil {
		t.Fatal("hashDisc on a failing disc returned nil error")
	}
}

// errDisc returns an error from ReadSector for any non-zero LBA, so hashDisc's
// per-sector error path is exercised.
type errDisc struct{}

func (e *errDisc) NumTracks() int { return 1 }
func (e *errDisc) Track(i int) (int, string, int, int, int, uint8) {
	return 1, "MODE1", 5, 0, 0, 0x41
}
func (e *errDisc) ReadSector(lba int) ([]byte, error) {
	if lba == 0 {
		return make([]byte, 2352), nil
	}
	return nil, fmt.Errorf("read failure at lba %d", lba)
}
