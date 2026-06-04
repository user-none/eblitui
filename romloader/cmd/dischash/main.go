// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Command dischash hashes a disc image's track data to match it against a
// reference (redump-style) breakdown. It reads the disc through romloader, so
// it supports every container format romloader handles (currently CHD); it is
// not tied to a single file type.
//
// For each disc it prints an overall CRC-32 taken over the concatenation of
// every track's raw sectors in track order (pregaps excluded, matching the
// length/sector counts of a redump-style breakdown). The overall CRC-32 is the
// value used to match a disc against a reference dump. The hashing is
// console-agnostic.
//
// It also prints a product number (disc ID) and title when the disc carries a
// Sega Saturn IP header; other discs leave those blank.
//
// With -v it instead prints a per-track table (number, type, pregap, length,
// sectors, size, CRC-32, MD5, SHA-1) followed by a Total row carrying the
// summed length, sector count, byte size, and the overall CRC-32.
//
// Hashing is a single streaming pass: each sector is read once and fed to the
// active hashers, so memory use is independent of disc size.
package main

import (
	"crypto/md5"
	"crypto/sha1"
	"errors"
	"flag"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/user-none/eblitui/romloader"
)

// discReader is the subset of the romloader disc surface this tool needs,
// declared as an interface so the hashing logic can be unit-tested with a fake
// disc; *romloader.Disc satisfies it structurally.
type discReader interface {
	ReadSector(lba int) ([]byte, error)
	NumTracks() int
	Track(i int) (number int, typ string, frames int, pregap int, startLBA int, control uint8)
}

// trackResult holds the per-track hashes and sizes gathered in verbose mode.
type trackResult struct {
	number int
	typ    string // raw romloader type string
	pregap int    // pregap length in frames
	frames int    // track body length in frames (sectors)
	bytes  int64  // bytes hashed for this track
	crc32  uint32
	md5    []byte
	sha1   []byte
}

// discResult is the outcome of hashing one disc.
type discResult struct {
	system       string
	id           string
	name         string
	overallCRC   uint32
	totalSectors int
	totalBytes   int64
	tracks       []trackResult // populated only in verbose mode
}

func main() {
	filePath := flag.String("file", "", "path to a single disc image")
	dirPath := flag.String("dir", "", "directory to scan for *.chd images")
	verbose := flag.Bool("v", false, "print the full per-track hash table")
	jobs := flag.Int("j", runtime.NumCPU(), "max discs to hash concurrently (-dir)")
	flag.Parse()

	// Exactly one of -file / -dir must be given.
	if (*filePath == "") == (*dirPath == "") {
		fmt.Fprintln(os.Stderr, "specify exactly one of -file or -dir")
		flag.Usage()
		os.Exit(2)
	}

	var paths []string
	if *filePath != "" {
		paths = []string{*filePath}
	} else {
		found, err := chdsInDir(*dirPath)
		if err != nil {
			log.Fatalf("scan %s: %v", *dirPath, err)
		}
		if len(found) == 0 {
			log.Fatalf("no .chd files in %s", *dirPath)
		}
		paths = found
	}

	if *verbose {
		os.Exit(runVerbose(os.Stdout, paths, *jobs))
	}
	os.Exit(runSummary(os.Stdout, paths, *jobs))
}

// hashOutcome pairs a hash result with any error from hashing one disc.
type hashOutcome struct {
	res *discResult
	err error
}

// parallelMap applies fn to indices [0,n) using at most workers goroutines and
// returns the results in index order. A single disc (n==1) runs inline.
func parallelMap[T any](n, workers int, fn func(i int) T) []T {
	out := make([]T, n)
	if workers < 1 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = fn(i)
		}(i)
	}
	wg.Wait()
	return out
}

// hashAll hashes every path, up to workers discs concurrently, returning
// outcomes in path order. Each disc is an independent romloader.Disc, so the
// only shared state is the per-index result slot.
func hashAll(paths []string, verbose bool, workers int) []hashOutcome {
	return parallelMap(len(paths), workers, func(i int) hashOutcome {
		res, err := openAndHash(paths[i], verbose)
		return hashOutcome{res: res, err: err}
	})
}

// summaryRow is one disc's row in the default output.
type summaryRow struct {
	crc, id, name, system, file string
}

// runSummary hashes the discs (up to workers concurrently) and prints the
// default table. Returns the process exit code (1 if any disc failed).
func runSummary(out io.Writer, paths []string, workers int) int {
	failed := false
	var rows []summaryRow
	for i, oc := range hashAll(paths, false, workers) {
		if oc.err != nil {
			// Show the disc with ERROR in the hash column instead of dropping
			// it. A genuine open/read failure is also logged and fails the run;
			// an unreadable disc (no tracks) is expected and stays quiet.
			if !errors.Is(oc.err, errUnreadable) {
				fmt.Fprintf(os.Stderr, "%s: %v\n", paths[i], oc.err)
				failed = true
			}
			rows = append(rows, summaryRow{crc: "ERROR", file: filepath.Base(paths[i])})
			continue
		}
		rows = append(rows, summaryRow{
			crc:    fmt.Sprintf("%08x", oc.res.overallCRC),
			id:     oc.res.id,
			name:   oc.res.name,
			system: oc.res.system,
			file:   filepath.Base(paths[i]),
		})
	}
	writeSummary(out, rows)
	if failed {
		return 1
	}
	return 0
}

// writeSummary prints the rows as an aligned table. CRC32 and FILE are always
// shown; ID, NAME, and SYSTEM appear only when at least one row populates them,
// so a format that doesn't carry a field leaves no empty column behind.
func writeSummary(out io.Writer, rows []summaryRow) {
	var showID, showName, showSystem bool
	for _, r := range rows {
		showID = showID || r.id != ""
		showName = showName || r.name != ""
		showSystem = showSystem || r.system != ""
	}

	line := func(crc, id, name, system, file string) string {
		cols := []string{crc}
		if showID {
			cols = append(cols, id)
		}
		if showName {
			cols = append(cols, name)
		}
		if showSystem {
			cols = append(cols, system)
		}
		return strings.Join(append(cols, file), "\t")
	}

	tw := tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, line("CRC32", "ID", "NAME", "SYSTEM", "FILE"))
	for _, r := range rows {
		fmt.Fprintln(tw, line(r.crc, r.id, r.name, r.system, r.file))
	}
	tw.Flush()
}

// runVerbose hashes the discs (up to workers concurrently) and prints the full
// per-track table for each, in path order. Returns the process exit code (1 if
// any disc failed).
func runVerbose(out io.Writer, paths []string, workers int) int {
	failed := false
	for i, oc := range hashAll(paths, true, workers) {
		if oc.err != nil {
			fmt.Fprintf(out, "FILE:   %s\nERROR:  %v\n\n", filepath.Base(paths[i]), oc.err)
			if !errors.Is(oc.err, errUnreadable) {
				failed = true
			}
			continue
		}
		printVerbose(out, paths[i], oc.res)
	}
	if failed {
		return 1
	}
	return 0
}

// openAndHash opens a disc through romloader and hashes it, always closing the
// disc before returning.
func openAndHash(path string, verbose bool) (*discResult, error) {
	disc, err := romloader.OpenDisc(path)
	if err != nil {
		return nil, err
	}
	defer disc.Close()
	return hashDisc(disc, verbose)
}

// errUnreadable marks a disc romloader opened but can't hash - no tracks, e.g.
// a GD-ROM/DVD CHD whose track format isn't supported. It is shown as an ERROR
// row rather than treated as a hard failure.
var errUnreadable = errors.New("no tracks; unsupported or unreadable disc")

// hashDisc streams every track's sectors through the overall CRC-32 hasher and,
// in verbose mode, through per-track CRC-32/MD5/SHA-1 hashers. Each sector is
// read exactly once.
func hashDisc(d discReader, verbose bool) (*discResult, error) {
	if d.NumTracks() == 0 {
		return nil, errUnreadable
	}

	res := &discResult{}
	res.system, res.id, res.name = identifyDisc(d)

	overall := crc32.NewIEEE()
	n := d.NumTracks()
	for i := 0; i < n; i++ {
		number, typ, frames, pregap, startLBA, _ := d.Track(i)
		if frames < 0 || startLBA < 0 {
			return nil, fmt.Errorf("track %d: invalid TOC (startLBA=%d frames=%d)",
				number, startLBA, frames)
		}

		var tcrc hash.Hash32
		var tmd5, tsha1 hash.Hash
		if verbose {
			tcrc = crc32.NewIEEE()
			tmd5 = md5.New()
			tsha1 = sha1.New()
		}

		var trackBytes int64
		for s := 0; s < frames; s++ {
			lba := startLBA + s
			data, err := d.ReadSector(lba)
			if err != nil {
				return nil, fmt.Errorf("track %d: read sector %d: %w", number, lba, err)
			}
			overall.Write(data)
			if verbose {
				tcrc.Write(data)
				tmd5.Write(data)
				tsha1.Write(data)
			}
			trackBytes += int64(len(data))
		}

		res.totalSectors += frames
		res.totalBytes += trackBytes
		if verbose {
			res.tracks = append(res.tracks, trackResult{
				number: number,
				typ:    typ,
				pregap: pregap,
				frames: frames,
				bytes:  trackBytes,
				crc32:  tcrc.Sum32(),
				md5:    tmd5.Sum(nil),
				sha1:   tsha1.Sum(nil),
			})
		}
	}

	res.overallCRC = overall.Sum32()
	return res, nil
}

// printVerbose writes the identity header, per-track table, and Total row.
func printVerbose(out io.Writer, path string, res *discResult) {
	fmt.Fprintf(out, "FILE:   %s\n", filepath.Base(path))
	if res.id != "" {
		fmt.Fprintf(out, "ID:     %s\n", res.id)
	}
	if res.name != "" {
		fmt.Fprintf(out, "NAME:   %s\n", res.name)
	}
	if res.system != "" {
		fmt.Fprintf(out, "SYSTEM: %s\n", res.system)
	}

	tw := tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tType\tPregap\tLength\tSectors\tSize\tCRC-32\tMD5\tSHA-1")
	for _, t := range res.tracks {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%d\t%d\t%08x\t%x\t%x\n",
			t.number, trackTypeLabel(t.typ), framesToMSF(t.pregap),
			framesToMSF(t.frames), t.frames, t.bytes, t.crc32, t.md5, t.sha1)
	}
	fmt.Fprintf(tw, "Total\t\t\t%s\t%d\t%d\t%08x\t\t\n",
		framesToMSF(res.totalSectors), res.totalSectors, res.totalBytes, res.overallCRC)
	tw.Flush()
	fmt.Fprintln(out)
}

// identifier recognizes one disc format and extracts its serial/ID and title.
// detect returns ok=false when the disc is not that format.
type identifier struct {
	system string
	detect func(d discReader) (id, name string, ok bool)
}

// identifiers are tried in order; the first match wins. Saturn and Dreamcast
// use fixed-offset headers, so they are cheap and specific; PS1 falls through
// to an ISO 9660 filesystem walk.
var identifiers = []identifier{
	{"Saturn", identifySaturn},
	{"Dreamcast", identifyDreamcast},
	{"PS1", identifyPS1},
}

// identifyDisc returns the detected system, ID, and title. Any field a format
// does not provide (or that no format matched) comes back empty.
func identifyDisc(d discReader) (system, id, name string) {
	for _, idf := range identifiers {
		if id, name, ok := idf.detect(d); ok {
			return idf.system, id, name
		}
	}
	return "", "", ""
}

// identifySaturn reads the Sega Saturn IP header from sector 0: hardware
// identifier "SEGA SEGASATURN " at user offset 0x00, product number at 0x20
// (10 bytes), game title at 0x60 (112 bytes); user data starts at byte 16 of
// the raw Mode 1 sector.
func identifySaturn(d discReader) (string, string, bool) {
	data, err := d.ReadSector(0)
	if err != nil || len(data) < 16+0xD0 {
		return "", "", false
	}
	user := data[16:]
	if string(user[0:16]) != "SEGA SEGASATURN " {
		return "", "", false
	}
	id := strings.TrimSpace(string(user[0x20:0x2A]))
	name := strings.TrimSpace(string(user[0x60:0xD0]))
	return id, name, true
}

// identifyDreamcast reads the Sega Dreamcast IP.BIN from the first data track
// that carries it (the high-density data track on a GD-ROM): hardware identifier
// "SEGA SEGAKATANA " at user offset 0x00, product number at 0x40 (10 bytes),
// software title at 0x80 (128 bytes).
func identifyDreamcast(d discReader) (string, string, bool) {
	n := d.NumTracks()
	for i := 0; i < n; i++ {
		_, typ, frames, _, startLBA, _ := d.Track(i)
		if frames <= 0 || startLBA < 0 || !strings.Contains(strings.ToUpper(typ), "MODE") {
			continue
		}
		sec, err := d.ReadSector(startLBA)
		if err != nil {
			continue
		}
		user := isoUserData(sec)
		if user == nil || string(user[0:16]) != "SEGA SEGAKATANA " {
			continue
		}
		id := strings.TrimSpace(string(user[0x40:0x4A]))
		name := strings.TrimSpace(string(user[0x80:0x100]))
		return id, name, true
	}
	return "", "", false
}

// isoSectorSize is the ISO 9660 logical sector size (user data per sector).
const isoSectorSize = 2048

// isoUserData returns the 2048-byte logical sector from a raw CD sector,
// choosing the user-data offset by sector mode: Mode 2 Form 1 (byte 15 == 2)
// starts at byte 24, Mode 1 at byte 16. Returns nil if the sector is short.
func isoUserData(sec []byte) []byte {
	if len(sec) < 16 {
		return nil
	}
	off := 16
	if sec[15] == 2 {
		off = 24
	}
	if off+isoSectorSize > len(sec) {
		return nil
	}
	return sec[off : off+isoSectorSize]
}

// readISO reads one ISO 9660 logical sector at the given LBA.
func readISO(d discReader, lba int) []byte {
	if lba < 0 {
		return nil
	}
	sec, err := d.ReadSector(lba)
	if err != nil {
		return nil
	}
	return isoUserData(sec)
}

func le32(b []byte) int {
	return int(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24)
}

// identifyPS1 reads the PlayStation serial from SYSTEM.CNF via an ISO 9660
// walk: Primary Volume Descriptor at LBA 16, root directory, then the BOOT line
// of SYSTEM.CNF. PS1 discs carry no title, so only the serial is returned. A
// BOOT2 key marks a PS2 disc (out of scope) and yields no match.
func identifyPS1(d discReader) (string, string, bool) {
	pvd := readISO(d, 16)
	if pvd == nil || pvd[0] != 1 || string(pvd[1:6]) != "CD001" {
		return "", "", false
	}
	// Root directory record at PVD offset 156: extent LBA at +2, size at +10.
	rootExtent := le32(pvd[158:162])
	rootSize := le32(pvd[166:170])

	cnfExtent, cnfSize, ok := findInDir(d, rootExtent, rootSize, "SYSTEM.CNF")
	if !ok {
		return "", "", false
	}
	cnf := readISO(d, cnfExtent)
	if cnf == nil {
		return "", "", false
	}
	if cnfSize > len(cnf) {
		cnfSize = len(cnf)
	}
	serial, ok := parseBootSerial(string(cnf[:cnfSize]))
	if !ok {
		return "", "", false
	}
	return serial, "", true
}

// findInDir walks an ISO 9660 directory (extent/size in logical sectors/bytes)
// and returns the extent LBA and byte size of the first entry whose name
// matches want (case-insensitive, ignoring the ";version" suffix).
func findInDir(d discReader, extent, size int, want string) (int, int, bool) {
	if extent <= 0 || size <= 0 {
		return 0, 0, false
	}
	sectors := (size + isoSectorSize - 1) / isoSectorSize
	if sectors > 64 { // sanity cap; root directories are tiny
		sectors = 64
	}
	for s := 0; s < sectors; s++ {
		dir := readISO(d, extent+s)
		if dir == nil {
			break
		}
		for p := 0; p+33 < len(dir); {
			recLen := int(dir[p])
			if recLen == 0 {
				break // remaining bytes in this sector are padding
			}
			if p+recLen > len(dir) || recLen < 34 {
				break
			}
			nameLen := int(dir[p+32])
			if nameLen > 0 && p+33+nameLen <= len(dir) {
				name := string(dir[p+33 : p+33+nameLen])
				if i := strings.IndexByte(name, ';'); i >= 0 {
					name = name[:i]
				}
				if strings.EqualFold(name, want) {
					return le32(dir[p+2 : p+6]), le32(dir[p+10 : p+14]), true
				}
			}
			p += recLen
		}
	}
	return 0, 0, false
}

// parseBootSerial extracts and normalizes the PlayStation serial from a
// SYSTEM.CNF body. The BOOT line looks like "BOOT = cdrom:\SCUS_947.02;1"
// (whitespace optional); the executable name is the serial, normalized to
// redump form (uppercase, '_' -> '-', '.' dropped): SCUS_947.02 -> SCUS-94702.
// A BOOT2 key (PS2) returns ok=false.
func parseBootSerial(cnf string) (string, bool) {
	for _, line := range strings.Split(cnf, "\n") {
		key, val, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(key), "BOOT") {
			continue
		}
		val = strings.TrimSpace(val)
		if i := strings.LastIndexByte(val, '\\'); i >= 0 {
			val = val[i+1:]
		} else if i := strings.LastIndexByte(val, ':'); i >= 0 {
			val = val[i+1:]
		}
		if i := strings.IndexByte(val, ';'); i >= 0 {
			val = val[:i]
		}
		val = strings.TrimSpace(val)
		if val == "" {
			return "", false
		}
		serial := strings.ToUpper(val)
		serial = strings.ReplaceAll(serial, "_", "-")
		serial = strings.ReplaceAll(serial, ".", "")
		return serial, true
	}
	return "", false
}

// framesToMSF formats a frame count as MM:SS:FF at 75 frames per second.
func framesToMSF(frames int) string {
	if frames < 0 {
		frames = 0
	}
	ff := frames % 75
	secs := frames / 75
	return fmt.Sprintf("%02d:%02d:%02d", secs/60, secs%60, ff)
}

// trackTypeLabel maps a romloader track type string to the redump-style label.
func trackTypeLabel(typ string) string {
	u := strings.ToUpper(typ)
	switch {
	case u == "AUDIO":
		return "Audio"
	case strings.Contains(u, "MODE1"):
		return "Data/Mode 1"
	case strings.Contains(u, "MODE2"):
		return "Data/Mode 2"
	default:
		return typ
	}
}

// chdsInDir returns the sorted paths of all *.chd files directly inside dir.
func chdsInDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".chd") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}
