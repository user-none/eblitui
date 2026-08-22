package main

import (
	"strings"
	"testing"

	"github.com/user-none/eblitui/rumble"
)

func TestParseRegion(t *testing.T) {
	r, err := parseRegion("wramh:0x06000000:0x100000")
	if err != nil {
		t.Fatalf("parseRegion: %v", err)
	}
	if r.Name != "wramh" || r.Start != 0x06000000 || r.Size != 0x100000 {
		t.Fatalf("region = %+v", r)
	}

	r, err = parseRegion("ram:1024:4096")
	if err != nil {
		t.Fatalf("parseRegion decimal: %v", err)
	}
	if r.Name != "ram" || r.Start != 1024 || r.Size != 4096 {
		t.Fatalf("region = %+v", r)
	}

	// A leading zero is decimal, not an octal prefix.
	r, err = parseRegion("ram:010:0x20")
	if err != nil {
		t.Fatalf("parseRegion leading zero: %v", err)
	}
	if r.Start != 10 || r.Size != 0x20 {
		t.Fatalf("region = %+v", r)
	}
}

func TestParseRegionRejects(t *testing.T) {
	bad := []string{
		"",
		"wramh",
		"wramh:0x06000000",
		"wramh:0x06000000:0x100000:extra",
		":0x06000000:0x100000",
		"wramh:nothex:0x100000",
		"wramh:0x06000000:nothex",
		"wramh:0x06000000:0",
		"wramh:0xFFFFFF00:0x200",
	}
	for _, s := range bad {
		if _, err := parseRegion(s); err == nil {
			t.Errorf("parseRegion(%q) accepted", s)
		}
	}
}

const goodSource = `
watch health byte 0x06000010
on health decreased: pulse 0.75 200ms
`

func TestValidateSourceParseOnly(t *testing.T) {
	if err := validateSource([]byte(goodSource), nil, false); err != nil {
		t.Fatalf("validateSource: %v", err)
	}
	if err := validateSource([]byte("watch health byte"), nil, false); err == nil {
		t.Fatal("malformed source accepted")
	}
}

func TestValidateSourceStrict(t *testing.T) {
	complete := "game: Sky Runner\ngameid: T-99901G\nsystem: saturn\n---\n" + goodSource
	if err := validateSource([]byte(complete), nil, true); err != nil {
		t.Fatalf("complete metadata rejected: %v", err)
	}
	if err := validateSource([]byte(goodSource), nil, true); err == nil {
		t.Fatal("missing metadata accepted under -strict")
	} else if !strings.Contains(err.Error(), "missing game, gameid, system") {
		t.Fatalf("err = %v, want the missing fields named", err)
	}
	partial := "gameid: T-99901G\n---\n" + goodSource
	if err := validateSource([]byte(partial), nil, true); err == nil {
		t.Fatal("partial metadata accepted under -strict")
	} else if !strings.Contains(err.Error(), "missing game, system") {
		t.Fatalf("err = %v, want only the absent fields named", err)
	}
	if err := validateSource([]byte(goodSource), nil, false); err != nil {
		t.Fatalf("missing metadata rejected without -strict: %v", err)
	}
}

func TestValidateSourceRegions(t *testing.T) {
	inside, err := parseRegion("wramh:0x06000000:0x100000")
	if err != nil {
		t.Fatalf("parseRegion: %v", err)
	}
	outside, err := parseRegion("wraml:0x00200000:0x100000")
	if err != nil {
		t.Fatalf("parseRegion: %v", err)
	}

	if err := validateSource([]byte(goodSource), []rumble.Region{inside}, false); err != nil {
		t.Fatalf("address inside region rejected: %v", err)
	}
	err = validateSource([]byte(goodSource), []rumble.Region{outside}, false)
	if err == nil || !strings.Contains(err.Error(), "not in a known memory region") {
		t.Fatalf("err = %v, want address rejection", err)
	}
}
