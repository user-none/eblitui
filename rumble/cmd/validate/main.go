// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Command validate checks rumble files
//
// Every file is parsed and validated against FORMAT.md. With one or
// more -region flags the watch addresses are also validated against
// those regions, the same check NewEngine performs when a host binds
// a file to a running system. Without -region flags the address check
// is skipped, since regions come from an emulator's memory map.
//
// With -strict a file must also carry the game, gameid, and system
// metadata expected of a distributed file. The format leaves all of it
// optional, so this check belongs here rather than in the parser.
//
// Usage:
//
//	validate [-strict] [-region name:start:size]... <file>...
//
// Exit status is 0 when every file passes and 1 when any file fails.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/user-none/eblitui/rumble"
)

// parseUint32 parses a region number as decimal or 0x hex.
func parseUint32(s string) (uint64, error) {
	base := 10
	if strings.HasPrefix(s, "0x") {
		base, s = 16, s[2:]
	}
	return strconv.ParseUint(s, base, 32)
}

// parseRegion parses a -region flag value of the form name:start:size.
// Start and size accept decimal or 0x hex.
func parseRegion(s string) (rumble.Region, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return rumble.Region{}, fmt.Errorf("region %q is not name:start:size", s)
	}
	name := parts[0]
	if name == "" {
		return rumble.Region{}, fmt.Errorf("region %q has an empty name", s)
	}
	start, err := parseUint32(parts[1])
	if err != nil {
		return rumble.Region{}, fmt.Errorf("region %q start: %v", s, err)
	}
	size, err := parseUint32(parts[2])
	if err != nil {
		return rumble.Region{}, fmt.Errorf("region %q size: %v", s, err)
	}
	if size == 0 {
		return rumble.Region{}, fmt.Errorf("region %q size must be positive", s)
	}
	if start+size > 1<<32 {
		return rumble.Region{}, fmt.Errorf("region %q extends past the 32-bit address space", s)
	}
	return rumble.Region{Name: name, Start: uint32(start), Size: uint32(size)}, nil
}

// validateSource runs the same validation a host performs at load:
// Parse always, and the NewEngine address binding when regions are
// given. Native byte order only affects value decoding, never
// validation, so any endianness serves the address check.
func validateSource(src []byte, regions []rumble.Region, strict bool) error {
	rs, err := rumble.Parse(src)
	if err != nil {
		return err
	}
	if strict {
		if err := checkMetadata(rs.Metadata); err != nil {
			return err
		}
	}
	if len(regions) == 0 {
		return nil
	}
	sys := rumble.System{BigEndian: true, Regions: regions}
	_, err = rumble.NewEngine(rs, sys, 1)
	return err
}

// checkMetadata requires the metadata a distributed file carries.
func checkMetadata(md rumble.Metadata) error {
	var missing []string
	if md.Game == "" {
		missing = append(missing, "game")
	}
	if md.GameID == "" {
		missing = append(missing, "gameid")
	}
	if md.System == "" {
		missing = append(missing, "system")
	}
	if len(missing) > 0 {
		return fmt.Errorf("metadata is missing %s", strings.Join(missing, ", "))
	}
	return nil
}

func main() {
	var regions []rumble.Region
	flag.Func("region", "memory region as name:start:size, repeatable (e.g. wramh:0x06000000:0x100000)", func(s string) error {
		r, err := parseRegion(s)
		if err != nil {
			return err
		}
		regions = append(regions, r)
		return nil
	})
	strict := flag.Bool("strict", false, "also require the game, gameid, and system metadata a distributed file carries")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: validate [-strict] [-region name:start:size]... <file>...\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	failed := false
	for _, path := range flag.Args() {
		src, err := os.ReadFile(path)
		if err == nil {
			err = validateSource(src, regions, *strict)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			failed = true
			continue
		}
		fmt.Printf("%s: ok\n", path)
	}
	if failed {
		os.Exit(1)
	}
}
