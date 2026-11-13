package romloader

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"errors"
	"hash/crc32"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// zipTestEntry is one entry of a test zip. When raw is set the entry is
// written exactly as given: header describes it and data is the bytes
// stored in the zip. Otherwise data is the file content, compressed with
// method.
type zipTestEntry struct {
	name   string
	data   []byte
	method uint16
	raw    bool
	header zip.FileHeader
}

// writeTestZip writes the entries to game.zip in a new directory.
func writeTestZip(t *testing.T, entries []zipTestEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "game.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for _, e := range entries {
		if e.raw {
			h := e.header
			h.Name = e.name
			fw, err := w.CreateRaw(&h)
			if err != nil {
				t.Fatalf("Failed to create %s in zip: %v", e.name, err)
			}
			if _, err := fw.Write(e.data); err != nil {
				t.Fatalf("Failed to write %s to zip: %v", e.name, err)
			}
			continue
		}
		fw, err := w.CreateHeader(&zip.FileHeader{Name: e.name, Method: e.method})
		if err != nil {
			t.Fatalf("Failed to create %s in zip: %v", e.name, err)
		}
		if _, err := fw.Write(e.data); err != nil {
			t.Fatalf("Failed to write %s to zip: %v", e.name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Failed to close zip: %v", err)
	}
	return path
}

// rawDeflateEntry returns a raw entry holding data deflated, described by a
// header the caller may then alter.
func rawDeflateEntry(t *testing.T, name string, data []byte) zipTestEntry {
	t.Helper()
	comp := testDeflate(t, data, flate.DefaultCompression)
	return zipTestEntry{
		name: name,
		data: comp,
		raw:  true,
		header: zip.FileHeader{
			Method:             zip.Deflate,
			CRC32:              crc32.ChecksumIEEE(data),
			CompressedSize64:   uint64(len(comp)),
			UncompressedSize64: uint64(len(data)),
		},
	}
}

const testZipCue = "FILE \"game (Track 1).bin\" BINARY\n" +
	"  TRACK 01 MODE1/2352\n" +
	"    INDEX 01 00:00:00\n" +
	"FILE \"game (Track 2).bin\" BINARY\n" +
	"  TRACK 02 AUDIO\n" +
	"    INDEX 00 00:00:00\n" +
	"    INDEX 01 00:02:00\n" +
	"FILE \"game (Track 3).bin\" BINARY\n" +
	"  TRACK 03 AUDIO\n" +
	"    INDEX 00 00:00:00\n" +
	"    INDEX 01 00:02:00\n"

// TestOpenDisc_ZipMatchesCue tests that a zip of a cue/bin set reads the
// same table of contents and the same sectors as the set on disk, for
// sectors read in order and out of order.
func TestOpenDisc_ZipMatchesCue(t *testing.T) {
	bins := []struct {
		name   string
		data   []byte
		method uint16
	}{
		// Large enough to be indexed with several checkpoints.
		{"game (Track 1).bin", testMixedData(2400*cueSectorBytes, 10), zip.Deflate},
		{"game (Track 2).bin", testRandomData(400*cueSectorBytes, 11), zip.Deflate},
		{"game (Track 3).bin", testMixedData(300*cueSectorBytes, 12), zip.Store},
	}

	dir := t.TempDir()
	cuePath := filepath.Join(dir, "game.cue")
	if err := os.WriteFile(cuePath, []byte(testZipCue), 0644); err != nil {
		t.Fatalf("Failed to create cue file: %v", err)
	}
	entries := []zipTestEntry{{name: "game.cue", data: []byte(testZipCue), method: zip.Deflate}}
	for _, b := range bins {
		if err := os.WriteFile(filepath.Join(dir, b.name), b.data, 0644); err != nil {
			t.Fatalf("Failed to create bin file: %v", err)
		}
		entries = append(entries, zipTestEntry{name: b.name, data: b.data, method: b.method})
	}

	want, err := OpenDisc(cuePath)
	if err != nil {
		t.Fatalf("OpenDisc cue: %v", err)
	}
	defer want.Close()
	got, err := OpenDisc(writeTestZip(t, entries))
	if err != nil {
		t.Fatalf("OpenDisc zip: %v", err)
	}
	defer got.Close()

	zb := got.backend.(*zipCueBackend)
	track1 := zb.bins["game (Track 1).bin"].r.(*zipEntry)
	if len(track1.ckpts) < 3 {
		t.Fatalf("track 1 has %d checkpoints, the test needs several", len(track1.ckpts))
	}

	if got.NumTracks() != want.NumTracks() {
		t.Fatalf("NumTracks = %d, want %d", got.NumTracks(), want.NumTracks())
	}
	total := 0
	for i := 0; i < want.NumTracks(); i++ {
		gn, gt, gf, gp, gs, gc := got.Track(i)
		wn, wt, wf, wp, ws, wc := want.Track(i)
		if gn != wn || gt != wt || gf != wf || gp != wp || gs != ws || gc != wc {
			t.Fatalf("Track(%d) = %d %s %d %d %d %#x, want %d %s %d %d %d %#x",
				i, gn, gt, gf, gp, gs, gc, wn, wt, wf, wp, ws, wc)
		}
		if got.NumTrackIndexes(i) != want.NumTrackIndexes(i) {
			t.Fatalf("NumTrackIndexes(%d) = %d, want %d", i, got.NumTrackIndexes(i), want.NumTrackIndexes(i))
		}
		for n := 0; n < want.NumTrackIndexes(i); n++ {
			gi, gl := got.TrackIndex(i, n)
			wi, wl := want.TrackIndex(i, n)
			if gi != wi || gl != wl {
				t.Fatalf("TrackIndex(%d, %d) = %d %d, want %d %d", i, n, gi, gl, wi, wl)
			}
		}
		total += wf
	}

	// Every sector in order, then every sector in a random order so reads
	// jump backward and forward across checkpoints and tracks.
	lbas := make([]int, 0, 2*total)
	for lba := 0; lba < total; lba++ {
		lbas = append(lbas, lba)
	}
	lbas = append(lbas, rand.New(rand.NewSource(13)).Perm(total)...)
	for _, lba := range lbas {
		g, err := got.ReadSector(lba)
		if err != nil {
			t.Fatalf("zip ReadSector(%d): %v", lba, err)
		}
		w, err := want.ReadSector(lba)
		if err != nil {
			t.Fatalf("cue ReadSector(%d): %v", lba, err)
		}
		if !bytes.Equal(g, w) {
			t.Fatalf("sector %d differs between the zip and the cue", lba)
		}
	}

	if _, err := got.ReadSector(total); err == nil {
		t.Fatal("expected an error reading past the last sector")
	}
}

// testLazyZip writes a zip of a three track disc, the first track large
// enough for several regions, and returns its path with each bin's data.
func testLazyZip(t *testing.T) (string, [][]byte) {
	t.Helper()
	bins := [][]byte{
		testMixedData(2400*cueSectorBytes, 80),
		testRandomData(700*cueSectorBytes, 81),
		testMixedData(300*cueSectorBytes, 82),
	}
	cue := "FILE \"a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n" +
		"FILE \"b.bin\" BINARY\nTRACK 02 AUDIO\nINDEX 01 00:00:00\n" +
		"FILE \"c.bin\" BINARY\nTRACK 03 AUDIO\nINDEX 01 00:00:00\n"
	return writeTestZip(t, []zipTestEntry{
		{name: "game.cue", data: []byte(cue), method: zip.Deflate},
		{name: "a.bin", data: bins[0], method: zip.Deflate},
		{name: "b.bin", data: bins[1], method: zip.Deflate},
		{name: "c.bin", data: bins[2], method: zip.Deflate},
	}), bins
}

// TestOpenDiscLazy_IndexesAsRead tests that a lazily opened zip decodes
// nothing at open, indexes a bin only as far as reads reach, and reads the
// right data at every stage: ahead of the index, behind it, and from other
// bins in between.
func TestOpenDiscLazy_IndexesAsRead(t *testing.T) {
	path, bins := testLazyZip(t)
	d, err := OpenDiscLazy(path)
	if err != nil {
		t.Fatalf("OpenDiscLazy: %v", err)
	}
	defer d.Close()

	zb := d.backend.(*zipCueBackend)
	a := zb.bins["a.bin"].r.(*zipEntry)
	b := zb.bins["b.bin"].r.(*zipEntry)
	regions := func(e *zipEntry) int { return len(e.ckpts) }
	if regions(a) != 1 || a.indexed || regions(b) != 1 || b.indexed {
		t.Fatal("bins were indexed at open")
	}

	// Each track starts where the one before ends.
	starts := []int{0, 2400, 3100}
	check := func(lba int) {
		t.Helper()
		track := 2
		for track > 0 && lba < starts[track] {
			track--
		}
		got, err := d.ReadSector(lba)
		if err != nil {
			t.Fatalf("ReadSector(%d): %v", lba, err)
		}
		off := (lba - starts[track]) * cueSectorBytes
		if !bytes.Equal(got, bins[track][off:off+cueSectorBytes]) {
			t.Fatalf("sector %d differs from its bin", lba)
		}
	}

	// The first sector indexes one region of the first bin and nothing
	// of the others.
	check(0)
	if regions(a) != 2 || a.indexed || regions(b) != 1 {
		t.Fatalf("after sector 0: bin a has %d checkpoints, indexed %v, bin b has %d", regions(a), a.indexed, regions(b))
	}

	// A jump ahead indexes up to its region, not to the end.
	check(1500)
	if n := regions(a); n < 4 || a.indexed {
		t.Fatalf("after sector 1500: bin a has %d checkpoints, indexed %v", n, a.indexed)
	}
	reached := regions(a)

	// Reads behind the index, and reads of another bin, leave it alone.
	check(700)
	check(3)
	check(2400 + 650)
	check(1499)
	if regions(a) != reached {
		t.Fatalf("reads behind the index changed bin a from %d to %d checkpoints", reached, regions(a))
	}
	if !b.indexed {
		t.Fatal("bin b was read to its last region but is not indexed")
	}

	// Reading to the end finishes the bin.
	check(2399)
	if !a.indexed {
		t.Fatal("bin a was read to its end but is not indexed")
	}

	// Every sector in a random order.
	for _, lba := range rand.New(rand.NewSource(83)).Perm(3400) {
		check(lba)
	}
}

// TestOpenDiscLazy_MatchesOpenDisc tests that a zip opened lazily and read
// to the end has the same index as one indexed at open.
func TestOpenDiscLazy_MatchesOpenDisc(t *testing.T) {
	path, _ := testLazyZip(t)
	eager, err := OpenDisc(path)
	if err != nil {
		t.Fatalf("OpenDisc: %v", err)
	}
	defer eager.Close()
	lazy, err := OpenDiscLazy(path)
	if err != nil {
		t.Fatalf("OpenDiscLazy: %v", err)
	}
	defer lazy.Close()

	for lba := 0; lba < 3400; lba++ {
		got, err := lazy.ReadSector(lba)
		if err != nil {
			t.Fatalf("lazy ReadSector(%d): %v", lba, err)
		}
		want, err := eager.ReadSector(lba)
		if err != nil {
			t.Fatalf("ReadSector(%d): %v", lba, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("sector %d differs between the lazy and the indexed disc", lba)
		}
	}

	for _, name := range []string{"a.bin", "b.bin", "c.bin"} {
		e := eager.backend.(*zipCueBackend).bins[name].r.(*zipEntry)
		l := lazy.backend.(*zipCueBackend).bins[name].r.(*zipEntry)
		if !l.indexed || len(l.ckpts) != len(e.ckpts) {
			t.Fatalf("%s: lazy index has %d checkpoints, indexed %v, want %d", name, len(l.ckpts), l.indexed, len(e.ckpts))
		}
		for i := range e.ckpts {
			if l.ckpts[i].out != e.ckpts[i].out || l.ckpts[i].at.bit != e.ckpts[i].at.bit {
				t.Fatalf("%s: checkpoint %d differs between the lazy and the indexed disc", name, i)
			}
		}
	}
}

// TestOpenDiscLazy_ChecksAtEnd tests that a bin whose data does not match
// the zip's checksum opens and reads lazily, and fails when a read reaches
// its end.
func TestOpenDiscLazy_ChecksAtEnd(t *testing.T) {
	bin := testMixedData(1200*cueSectorBytes, 84)
	bad := rawDeflateEntry(t, "a.bin", bin)
	bad.header.CRC32 ^= 1
	cue := "FILE \"a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n"
	path := writeTestZip(t, []zipTestEntry{
		{name: "game.cue", data: []byte(cue), method: zip.Deflate},
		bad,
	})

	if d, err := OpenDisc(path); err == nil {
		d.Close()
		t.Fatal("OpenDisc: expected an error")
	}

	d, err := OpenDiscLazy(path)
	if err != nil {
		t.Fatalf("OpenDiscLazy: %v", err)
	}
	defer d.Close()
	sector, err := d.ReadSector(0)
	if err != nil {
		t.Fatalf("ReadSector(0): %v", err)
	}
	if !bytes.Equal(sector, bin[:cueSectorBytes]) {
		t.Fatal("sector 0 differs from the bin")
	}
	_, err = d.ReadSector(1199)
	if err == nil {
		t.Fatal("expected an error reading the last sector")
	}
	for _, want := range []string{`"a.bin"`, "CRC-32"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected an error containing %s, got %v", want, err)
		}
	}
}

// TestOpenDiscLazy_OtherFormats tests that the lazy open accepts the same
// formats as OpenDisc and rejects the same paths.
func TestOpenDiscLazy_OtherFormats(t *testing.T) {
	dir := t.TempDir()
	writeTestBin(t, filepath.Join(dir, "a.bin"), 0x33)
	cuePath := filepath.Join(dir, "game.cue")
	cue := "FILE \"a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n"
	if err := os.WriteFile(cuePath, []byte(cue), 0644); err != nil {
		t.Fatalf("Failed to create cue file: %v", err)
	}
	d, err := OpenDiscLazy(cuePath)
	if err != nil {
		t.Fatalf("OpenDiscLazy cue: %v", err)
	}
	sector, err := d.ReadSector(1)
	d.Close()
	if err != nil || sector[0] != 0x33 {
		t.Fatalf("ReadSector = %#x, %v, want 0x33", sector[0], err)
	}

	if _, err := OpenDiscLazy(filepath.Join(dir, "game.iso")); !errors.Is(err, ErrUnsupportedDisc) {
		t.Fatalf("expected ErrUnsupportedDisc, got %v", err)
	}
}

// TestOpenDisc_ZipCacheEviction tests reads over more regions than the
// cache holds, so regions are evicted, their buffers reused for regions of
// other sizes, and evicted regions decoded again.
func TestOpenDisc_ZipCacheEviction(t *testing.T) {
	const sectors = 9200
	bin := testMixedData(sectors*cueSectorBytes, 22)
	cue := "FILE \"a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n"
	path := writeTestZip(t, []zipTestEntry{
		{name: "game.cue", data: []byte(cue), method: zip.Deflate},
		{name: "a.bin", data: bin, method: zip.Deflate},
	})
	t.Run("indexed at open", func(t *testing.T) {
		d, err := OpenDisc(path)
		if err != nil {
			t.Fatalf("OpenDisc: %v", err)
		}
		defer d.Close()
		testZipCacheEviction(t, d, bin)
	})
	t.Run("indexed by a read", func(t *testing.T) {
		d, err := OpenDiscLazy(path)
		if err != nil {
			t.Fatalf("OpenDiscLazy: %v", err)
		}
		defer d.Close()
		// Reading the last sector indexes every region on the way to
		// it, each decoded into the cache and most evicted again.
		if _, err := d.ReadSector(sectors - 1); err != nil {
			t.Fatalf("ReadSector(%d): %v", sectors-1, err)
		}
		testZipCacheEviction(t, d, bin)
	})
}

// testZipCacheEviction checks the regions of the indexed bin a.bin of d,
// which holds bin, and reads across all of them forward and backward.
func testZipCacheEviction(t *testing.T, d *Disc, bin []byte) {
	t.Helper()
	e := d.backend.(*zipCueBackend).bins["a.bin"].r.(*zipEntry)
	if len(e.ckpts) <= zipCacheSlots {
		t.Fatalf("bin has %d regions, the test needs more than the %d cache slots", len(e.ckpts), zipCacheSlots)
	}

	// For each region, the sector holding its first byte, which for all but
	// the first also holds the end of the region before, and the sector
	// after it.
	var lbas []int
	for i, ck := range e.ckpts {
		if i > 0 {
			if n := ck.out - e.ckpts[i-1].out; n < zipCheckpointSpacing || n > zipChunkMax {
				t.Fatalf("region %d is %d bytes, want %d to %d", i-1, n, zipCheckpointSpacing, zipChunkMax)
			}
		}
		lba := int(ck.out / cueSectorBytes)
		lbas = append(lbas, lba, lba+1)
	}
	// Forward, backward, then forward again: every region is decoded, then
	// evicted by later ones, then decoded into a reused buffer.
	order := slices.Clone(lbas)
	slices.Reverse(lbas)
	order = append(order, lbas...)
	slices.Reverse(lbas)
	order = append(order, lbas...)

	for _, lba := range order {
		sector, err := d.ReadSector(lba)
		if err != nil {
			t.Fatalf("ReadSector(%d): %v", lba, err)
		}
		if !bytes.Equal(sector, bin[lba*cueSectorBytes:(lba+1)*cueSectorBytes]) {
			t.Fatalf("sector %d differs from the bin", lba)
		}
	}
}

// TestOpenDisc_ZipCueInDirectory tests that bin names resolve against the
// cue sheet's directory inside the zip.
func TestOpenDisc_ZipCueInDirectory(t *testing.T) {
	bin := testMixedData(4*cueSectorBytes, 20)
	cue := "FILE \"a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n"
	d, err := OpenDisc(writeTestZip(t, []zipTestEntry{
		{name: "disc/", method: zip.Store},
		{name: "disc/game.cue", data: []byte(cue), method: zip.Deflate},
		{name: "disc/a.bin", data: bin, method: zip.Deflate},
		{name: "a.bin", data: make([]byte, 4*cueSectorBytes), method: zip.Deflate},
	}))
	if err != nil {
		t.Fatalf("OpenDisc: %v", err)
	}
	defer d.Close()

	sector, err := d.ReadSector(3)
	if err != nil {
		t.Fatalf("ReadSector: %v", err)
	}
	if !bytes.Equal(sector, bin[3*cueSectorBytes:]) {
		t.Fatal("sector was not read from the bin beside the cue sheet")
	}
}

// TestOpenDisc_ZipIgnoresMacOSXEntries tests that entries under the
// __MACOSX directory are not seen: a cue sheet there does not count as a
// second cue sheet, and a bin there is not used.
func TestOpenDisc_ZipIgnoresMacOSXEntries(t *testing.T) {
	bin := testMixedData(4*cueSectorBytes, 21)
	cue := "FILE \"a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n"
	d, err := OpenDisc(writeTestZip(t, []zipTestEntry{
		{name: "game.cue", data: []byte(cue), method: zip.Deflate},
		{name: "a.bin", data: bin, method: zip.Deflate},
		{name: "__MACOSX/._game.cue", data: []byte("resource fork"), method: zip.Deflate},
		{name: "__MACOSX/._a.bin", data: []byte("resource fork"), method: zip.Deflate},
		{name: `__MACOSX\other.cue`, data: []byte("resource fork"), method: zip.Deflate},
	}))
	if err != nil {
		t.Fatalf("OpenDisc: %v", err)
	}
	defer d.Close()

	sector, err := d.ReadSector(2)
	if err != nil {
		t.Fatalf("ReadSector: %v", err)
	}
	if !bytes.Equal(sector, bin[2*cueSectorBytes:3*cueSectorBytes]) {
		t.Fatal("sector differs from the bin")
	}

	// With only a __MACOSX cue sheet the zip has no cue sheet.
	_, err = OpenDisc(writeTestZip(t, []zipTestEntry{
		{name: "__MACOSX/game.cue", data: []byte(cue), method: zip.Deflate},
		{name: "__MACOSX/a.bin", data: bin, method: zip.Deflate},
	}))
	if err == nil || !strings.Contains(err.Error(), "no cue sheet") {
		t.Fatalf("expected a no cue sheet error, got %v", err)
	}
}

// TestOpenDisc_ZipIgnoresAppleDouble tests that a file named with the
// AppleDouble "._" prefix is not seen wherever it is in the zip: one beside
// the cue sheet does not count as a second cue sheet, and one is never used
// as a bin.
func TestOpenDisc_ZipIgnoresAppleDouble(t *testing.T) {
	bin := testMixedData(4*cueSectorBytes, 23)
	cue := "FILE \"a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n"
	d, err := OpenDisc(writeTestZip(t, []zipTestEntry{
		{name: "disc/._game.cue", data: []byte("metadata"), method: zip.Deflate},
		{name: "disc/game.cue", data: []byte(cue), method: zip.Deflate},
		{name: "disc/._a.bin", data: []byte("metadata"), method: zip.Deflate},
		{name: "disc/a.bin", data: bin, method: zip.Deflate},
		{name: "._other.cue", data: []byte("metadata"), method: zip.Deflate},
	}))
	if err != nil {
		t.Fatalf("OpenDisc: %v", err)
	}
	defer d.Close()

	sector, err := d.ReadSector(1)
	if err != nil {
		t.Fatalf("ReadSector: %v", err)
	}
	if !bytes.Equal(sector, bin[cueSectorBytes:2*cueSectorBytes]) {
		t.Fatal("sector differs from the bin")
	}

	// A bin that exists only under an AppleDouble name is missing.
	_, err = OpenDisc(writeTestZip(t, []zipTestEntry{
		{name: "game.cue", data: []byte("FILE \"._a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n"), method: zip.Deflate},
		{name: "._a.bin", data: bin, method: zip.Deflate},
	}))
	if err == nil || !strings.Contains(err.Error(), "opening track file") {
		t.Fatalf("expected a missing track file error, got %v", err)
	}
}

// TestOpenDisc_ZipBackslashNames tests that entry names written with
// backslashes are read as paths, so a cue sheet in a directory finds the
// bins beside it.
func TestOpenDisc_ZipBackslashNames(t *testing.T) {
	bin := testMixedData(4*cueSectorBytes, 24)
	cue := "FILE \"sub\\a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n"
	d, err := OpenDisc(writeTestZip(t, []zipTestEntry{
		{name: `disc\`, method: zip.Store},
		{name: `disc\game.cue`, data: []byte(cue), method: zip.Deflate},
		{name: `disc\sub\a.bin`, data: bin, method: zip.Deflate},
		{name: `sub\a.bin`, data: make([]byte, 4*cueSectorBytes), method: zip.Deflate},
	}))
	if err != nil {
		t.Fatalf("OpenDisc: %v", err)
	}
	defer d.Close()

	sector, err := d.ReadSector(3)
	if err != nil {
		t.Fatalf("ReadSector: %v", err)
	}
	if !bytes.Equal(sector, bin[3*cueSectorBytes:]) {
		t.Fatal("sector was not read from the bin beside the cue sheet")
	}
}

// TestOpenDisc_ZipUncleanNames tests that entry names with a leading "./"
// or "/" or a doubled separator are found by the cleaned names the cue
// sheet's FILE directives resolve to.
func TestOpenDisc_ZipUncleanNames(t *testing.T) {
	bin := testMixedData(4*cueSectorBytes, 25)
	cue := "FILE \"a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n"
	for _, names := range [][2]string{
		{"./game.cue", "./a.bin"},
		{"/game.cue", "/a.bin"},
		{"disc//game.cue", "disc/./a.bin"},
	} {
		d, err := OpenDisc(writeTestZip(t, []zipTestEntry{
			{name: names[0], data: []byte(cue), method: zip.Deflate},
			{name: names[1], data: bin, method: zip.Deflate},
		}))
		if err != nil {
			t.Fatalf("%q and %q: OpenDisc: %v", names[0], names[1], err)
		}
		sector, err := d.ReadSector(2)
		d.Close()
		if err != nil {
			t.Fatalf("%q and %q: ReadSector: %v", names[0], names[1], err)
		}
		if !bytes.Equal(sector, bin[2*cueSectorBytes:3*cueSectorBytes]) {
			t.Fatalf("%q and %q: sector differs from the bin", names[0], names[1])
		}
	}
}

// TestOpenDisc_ZipRejected tests the zips OpenDisc refuses.
func TestOpenDisc_ZipRejected(t *testing.T) {
	bin := testMixedData(8*cueSectorBytes, 30)
	cue := zipTestEntry{
		name:   "game.cue",
		data:   []byte("FILE \"a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n"),
		method: zip.Deflate,
	}
	altered := func(change func(h *zip.FileHeader)) zipTestEntry {
		e := rawDeflateEntry(t, "a.bin", bin)
		change(&e.header)
		return e
	}

	cases := []struct {
		name    string
		entries []zipTestEntry
		wantErr string
		wantIs  error
	}{
		{
			name:    "no cue sheet",
			entries: []zipTestEntry{{name: "a.bin", data: bin, method: zip.Deflate}},
			wantErr: "no cue sheet",
		},
		{
			name: "two cue sheets",
			entries: []zipTestEntry{
				cue,
				{name: "other.CUE", data: cue.data, method: zip.Deflate},
				{name: "a.bin", data: bin, method: zip.Deflate},
			},
			wantErr: "more than one cue sheet",
		},
		{
			name:    "missing bin",
			entries: []zipTestEntry{cue, {name: "b.bin", data: bin, method: zip.Deflate}},
			wantErr: "opening track file",
		},
		{
			name:    "bin name matches no entry",
			entries: []zipTestEntry{{name: "game.cue", data: []byte("FILE \"../a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n"), method: zip.Deflate}},
			wantErr: "opening track file",
		},
		{
			name:    "bin size not whole sectors",
			entries: []zipTestEntry{cue, {name: "a.bin", data: bin[:len(bin)-1], method: zip.Deflate}},
			wantErr: "not a multiple",
		},
		{
			name:    "unsupported method",
			entries: []zipTestEntry{cue, altered(func(h *zip.FileHeader) { h.Method = 93 })},
			wantErr: "unsupported compression method",
		},
		{
			name:    "encrypted",
			entries: []zipTestEntry{cue, altered(func(h *zip.FileHeader) { h.Flags |= zipEncrypted })},
			wantErr: "encrypted",
		},
		{
			name:    "wrong CRC",
			entries: []zipTestEntry{cue, altered(func(h *zip.FileHeader) { h.CRC32 ^= 1 })},
			wantErr: "CRC-32",
		},
		{
			name:    "data longer than the directory size",
			entries: []zipTestEntry{cue, altered(func(h *zip.FileHeader) { h.UncompressedSize64 -= cueSectorBytes })},
			wantErr: "longer than",
		},
		{
			name:    "data shorter than the directory size",
			entries: []zipTestEntry{cue, altered(func(h *zip.FileHeader) { h.UncompressedSize64 += cueSectorBytes })},
			wantErr: "bytes, the zip directory gives",
		},
		{
			// One final block whose last decoder run passes the first
			// checkpoint spacing, so the stream ends where a checkpoint
			// would otherwise be taken.
			name: "data ending past a checkpoint spacing, shorter than the directory size",
			entries: []zipTestEntry{cue, func() zipTestEntry {
				data := make([]byte, (zipCheckpointSpacing/cueSectorBytes+1)*cueSectorBytes)
				for i := range data {
					data[i] = byte(i % 144)
				}
				comp := testFixedBlock(data)
				return zipTestEntry{
					name: "a.bin",
					data: comp,
					raw:  true,
					header: zip.FileHeader{
						Method:             zip.Deflate,
						CRC32:              crc32.ChecksumIEEE(data),
						CompressedSize64:   uint64(len(comp)),
						UncompressedSize64: uint64(len(data)) + cueSectorBytes,
					},
				}
			}()},
			wantErr: "bytes, the zip directory gives",
		},
		{
			name:    "compressed data cut short",
			entries: []zipTestEntry{cue, altered(func(h *zip.FileHeader) { h.CompressedSize64 /= 2 })},
			wantErr: "stream ends",
		},
		{
			name:    "bin larger than a disc",
			entries: []zipTestEntry{cue, altered(func(h *zip.FileHeader) { h.UncompressedSize64 = maxZipDiscBytes + cueSectorBytes })},
			wantIs:  ErrFileTooLarge,
		},
		{
			name: "stored size mismatch",
			entries: []zipTestEntry{cue, {
				name: "a.bin",
				data: bin,
				raw:  true,
				header: zip.FileHeader{
					Method:             zip.Store,
					CRC32:              crc32.ChecksumIEEE(bin),
					CompressedSize64:   uint64(len(bin)),
					UncompressedSize64: uint64(len(bin)) + cueSectorBytes,
				},
			}},
			wantErr: "stored entry",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := OpenDisc(writeTestZip(t, tc.entries))
			if err == nil {
				d.Close()
				t.Fatal("expected an error")
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Fatalf("expected %v, got %v", tc.wantIs, err)
			}
			if tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected an error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestOpenDisc_ZipOneBinFailsOfSeveral tests that when one of several
// deflated bins fails its index build, the open fails and names that bin.
func TestOpenDisc_ZipOneBinFailsOfSeveral(t *testing.T) {
	cue := "FILE \"a.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n" +
		"FILE \"b.bin\" BINARY\nTRACK 02 AUDIO\nINDEX 01 00:00:00\n" +
		"FILE \"c.bin\" BINARY\nTRACK 03 AUDIO\nINDEX 01 00:00:00\n" +
		"FILE \"d.bin\" BINARY\nTRACK 04 AUDIO\nINDEX 01 00:00:00\n"
	bad := rawDeflateEntry(t, "c.bin", testMixedData(300*cueSectorBytes, 62))
	bad.header.CRC32 ^= 1
	d, err := OpenDisc(writeTestZip(t, []zipTestEntry{
		{name: "game.cue", data: []byte(cue), method: zip.Deflate},
		{name: "a.bin", data: testMixedData(600*cueSectorBytes, 60), method: zip.Deflate},
		{name: "b.bin", data: testMixedData(400*cueSectorBytes, 61), method: zip.Deflate},
		bad,
		{name: "d.bin", data: testMixedData(200*cueSectorBytes, 63), method: zip.Deflate},
	}))
	if err == nil {
		d.Close()
		t.Fatal("expected an error")
	}
	for _, want := range []string{`"c.bin"`, "CRC-32"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected an error containing %s, got %v", want, err)
		}
	}
}

// TestZipIndexWorkers tests the number of entries indexed at a time: enough
// for the smaller entries to finish alongside the largest, and no more.
func TestZipIndexWorkers(t *testing.T) {
	const mb = 1 << 20
	cases := []struct {
		name  string
		sizes []int64 // largest first
		cpus  int
		want  int
	}{
		{"no entries", nil, 8, 0},
		{"one entry", []int64{500 * mb}, 8, 1},
		{"empty entries", []int64{0, 0, 0}, 8, 1},
		{"one entry holds most of the disc", []int64{490 * mb, 80 * mb, 25 * mb, 2 * mb}, 10, 3},
		{"one large entry and many small", append([]int64{184 * mb}, slices.Repeat([]int64{18 * mb}, 20)...), 10, 4},
		{"equal entries", slices.Repeat([]int64{30 * mb}, 20), 10, 10},
		{"fewer entries than wanted", []int64{30 * mb, 30 * mb}, 10, 2},
		{"fewer cpus than wanted", slices.Repeat([]int64{30 * mb}, 6), 2, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			order := make([]*zipEntry, len(tc.sizes))
			for i, size := range tc.sizes {
				order[i] = &zipEntry{size: size}
			}
			if got := zipIndexWorkers(order, tc.cpus); got != tc.want {
				t.Fatalf("zipIndexWorkers = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestOpenDisc_NotAZip tests that a file that is not a zip is rejected.
func TestOpenDisc_NotAZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.zip")
	if err := os.WriteFile(path, testRandomData(4096, 40), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	d, err := OpenDisc(path)
	if err == nil {
		d.Close()
		t.Fatal("expected an error")
	}
}
