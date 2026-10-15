// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package cheats

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// maxCheats caps the cheat count a file may declare.
const maxCheats = 10000

// maxFileOps caps the decoded ops across all cheats in a file, so repeats
// in many entries cannot add up to an unbounded allocation.
const maxFileOps = 65536

// entry holds one cheat's fields from a CHT file, keyed without the
// "cheatN_" prefix.
type entry map[string]string

// ReadFile reads a CHT file. See Read.
func ReadFile(path string, sys System, coreBigEndian bool) ([]Cheat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f, sys, coreBigEndian)
}

// Read reads a CHT file and returns the cheats that can be applied, in
// file order. sys selects the decoders for code string entries.
// coreBigEndian is the byte order of the core's memory; every op is
// converted to it. Entries that fail to parse or convert, master codes,
// and rumble-only entries are dropped, and cheats sharing an ID are kept
// once, as the first. The file's enable values are ignored.
func Read(r io.Reader, sys System, coreBigEndian bool) ([]Cheat, error) {
	entries, err := readEntries(r)
	if err != nil {
		return nil, err
	}

	var out []Cheat
	seen := make(map[string]bool)
	totalOps := 0
	for _, e := range entries {
		var c Cheat
		if handler, _ := strconv.ParseUint(e["handler"], 0, 32); handler == 1 {
			c, err = parseRetroEntry(e)
			if err == nil {
				err = toByteOrder(c.Ops, retroBigEndian, coreBigEndian)
			}
		} else {
			c, err = ParseCode(e["code"], sys, coreBigEndian)
			c.Name = e["desc"]
		}
		if err != nil || seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		totalOps += len(c.Ops)
		if totalOps > maxFileOps {
			return nil, fmt.Errorf("cheats decode to more than %d operations", maxFileOps)
		}
		out = append(out, c)
	}
	return out, nil
}

// readEntries reads a CHT file's "key = value" lines into one entry per
// declared cheat, in file order. Fields for cheat numbers outside the
// declared count are ignored.
func readEntries(r io.Reader) ([]entry, error) {
	kv := make(map[string]string)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		kv[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	countStr, ok := kv["cheats"]
	if !ok {
		return nil, errors.New("missing cheats count")
	}
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 0 {
		return nil, fmt.Errorf("invalid cheats count %q", countStr)
	}
	if count > maxCheats {
		return nil, fmt.Errorf("cheats count %d exceeds %d", count, maxCheats)
	}

	entries := make([]entry, count)
	for i := range entries {
		entries[i] = make(entry)
	}
	for key, value := range kv {
		rest, ok := strings.CutPrefix(key, "cheat")
		if !ok {
			continue
		}
		numStr, field, ok := strings.Cut(rest, "_")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(numStr)
		if err != nil || n < 0 || n >= count {
			continue
		}
		entries[n][field] = value
	}
	return entries, nil
}
