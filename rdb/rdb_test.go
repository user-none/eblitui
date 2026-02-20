package rdb

import (
	"testing"
)

func TestGetDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"USA region", "Sonic the Hedgehog (USA)", "Sonic the Hedgehog"},
		{"Europe region", "Alex Kidd in Miracle World (Europe)", "Alex Kidd in Miracle World"},
		{"Japan region", "Phantasy Star (Japan)", "Phantasy Star"},
		{"Multi-region", "Sonic the Hedgehog (USA, Europe)", "Sonic the Hedgehog"},
		{"With revision", "Zillion (Japan) (Rev 2)", "Zillion"},
		{"No parentheses", "Wonder Boy", "Wonder Boy"},
		{"Empty string", "", ""},
		{"Only parentheses", "(USA)", "(USA)"},
		{"Leading space paren", "Game (USA)", "Game"},
		{"Multiple parens groups", "Game (USA) (Rev 1)", "Game"},
		{"World region", "Black Belt (World)", "Black Belt"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GetDisplayName(tc.input)
			if got != tc.expected {
				t.Errorf("GetDisplayName(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestGetRegionFromName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// US detection
		{"USA in parens", "Sonic (USA)", "us"},
		{"US in parens", "Sonic (US)", "us"},
		{"USA comma suffix", "Sonic (Europe, USA)", "us"},
		{"USA Europe combo", "Sonic (USA, Europe)", "us"},

		// Europe detection
		{"Europe in parens", "Alex Kidd (Europe)", "eu"},
		{"EU in parens", "Alex Kidd (EU)", "eu"},
		{"Europe comma suffix", "Alex Kidd (Japan, Europe)", "eu"},

		// Japan detection
		{"Japan in parens", "Phantasy Star (Japan)", "jp"},
		{"JP in parens", "Phantasy Star (JP)", "jp"},
		{"Japan comma suffix", "Phantasy Star (Europe, Japan)", "eu"}, // Europe checked before Japan

		// World/multi-region defaults to US
		{"World", "Game (World)", "us"},

		// Unknown
		{"No region info", "Wonder Boy", ""},
		{"Empty string", "", ""},
		{"Unknown region", "Game (Brazil)", ""},

		// Case insensitivity
		{"Lowercase usa", "Game (usa)", "us"},
		{"Mixed case Europe", "Game (EUROPE)", "eu"},
		{"Mixed case Japan", "Game (JAPAN)", "jp"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GetRegionFromName(tc.input)
			if got != tc.expected {
				t.Errorf("GetRegionFromName(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestRDBLookups(t *testing.T) {
	// Build an RDB manually to test lookup methods
	rdb := &RDB{
		games: []Game{
			{Name: "Sonic", CRC32: 0x12345678, MD5: "abcdef0123456789"},
			{Name: "Alex Kidd", CRC32: 0xAABBCCDD, MD5: "1234567890abcdef"},
			{Name: "No Hash", CRC32: 0, MD5: ""},
		},
		byCRC32: make(map[uint32]*Game),
		byMD5:   make(map[string]*Game),
	}
	// Build indexes like Parse does
	for i := range rdb.games {
		if rdb.games[i].CRC32 != 0 {
			rdb.byCRC32[rdb.games[i].CRC32] = &rdb.games[i]
		}
		if rdb.games[i].MD5 != "" {
			rdb.byMD5[rdb.games[i].MD5] = &rdb.games[i]
		}
	}

	t.Run("FindByCRC32 found", func(t *testing.T) {
		g := rdb.FindByCRC32(0x12345678)
		if g == nil {
			t.Fatal("expected to find game")
		}
		if g.Name != "Sonic" {
			t.Errorf("expected Sonic, got %s", g.Name)
		}
	})

	t.Run("FindByCRC32 not found", func(t *testing.T) {
		g := rdb.FindByCRC32(0x00000000)
		if g != nil {
			t.Errorf("expected nil, got %+v", g)
		}
	})

	t.Run("FindByMD5 found", func(t *testing.T) {
		g := rdb.FindByMD5("abcdef0123456789")
		if g == nil {
			t.Fatal("expected to find game")
		}
		if g.Name != "Sonic" {
			t.Errorf("expected Sonic, got %s", g.Name)
		}
	})

	t.Run("FindByMD5 not found", func(t *testing.T) {
		g := rdb.FindByMD5("nonexistent")
		if g != nil {
			t.Errorf("expected nil, got %+v", g)
		}
	})

	t.Run("GetMD5ByCRC32 found", func(t *testing.T) {
		md5 := rdb.GetMD5ByCRC32(0x12345678)
		if md5 != "abcdef0123456789" {
			t.Errorf("expected abcdef0123456789, got %s", md5)
		}
	})

	t.Run("GetMD5ByCRC32 not found", func(t *testing.T) {
		md5 := rdb.GetMD5ByCRC32(0x00000000)
		if md5 != "" {
			t.Errorf("expected empty string, got %s", md5)
		}
	})

	t.Run("GameCount", func(t *testing.T) {
		if rdb.GameCount() != 3 {
			t.Errorf("expected 3, got %d", rdb.GameCount())
		}
	})
}

func TestEmptyRDB(t *testing.T) {
	rdb := &RDB{
		games:   nil,
		byCRC32: make(map[uint32]*Game),
		byMD5:   make(map[string]*Game),
	}

	if rdb.GameCount() != 0 {
		t.Errorf("expected 0, got %d", rdb.GameCount())
	}
	if g := rdb.FindByCRC32(0x12345678); g != nil {
		t.Errorf("expected nil, got %+v", g)
	}
	if g := rdb.FindByMD5("test"); g != nil {
		t.Errorf("expected nil, got %+v", g)
	}
	if md5 := rdb.GetMD5ByCRC32(0x12345678); md5 != "" {
		t.Errorf("expected empty, got %s", md5)
	}
}

func TestSetGameField(t *testing.T) {
	tests := []struct {
		key     string
		value   string
		checkFn func(g *Game) bool
		desc    string
	}{
		{"name", "Test Game", func(g *Game) bool { return g.Name == "Test Game" }, "name field"},
		{"description", "A test", func(g *Game) bool { return g.Description == "A test" }, "description field"},
		{"genre", "Action", func(g *Game) bool { return g.Genre == "Action" }, "genre field"},
		{"developer", "Sega", func(g *Game) bool { return g.Developer == "Sega" }, "developer field"},
		{"publisher", "Sega", func(g *Game) bool { return g.Publisher == "Sega" }, "publisher field"},
		{"franchise", "Sonic", func(g *Game) bool { return g.Franchise == "Sonic" }, "franchise field"},
		{"esrb_rating", "E", func(g *Game) bool { return g.ESRBRating == "E" }, "esrb_rating field"},
		{"serial", "MK-27000", func(g *Game) bool { return g.Serial == "MK-27000" }, "serial field"},
		{"rom_name", "sonic.sms", func(g *Game) bool { return g.ROMName == "sonic.sms" }, "rom_name field"},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			g := &Game{}
			setGameField(g, tc.key, tc.value)
			if !tc.checkFn(g) {
				t.Errorf("setGameField(%q, %q) did not set field correctly", tc.key, tc.value)
			}
		})
	}

	t.Run("unknown key is ignored", func(t *testing.T) {
		g := &Game{}
		setGameField(g, "unknown_field", "value")
		// Should not panic and game should be zero-valued
		if g.Name != "" {
			t.Errorf("unexpected field modified")
		}
	})
}

func TestParseEmptyData(t *testing.T) {
	// Data too short (< 0x11 bytes)
	rdb := Parse([]byte{})
	if rdb.GameCount() != 0 {
		t.Errorf("expected 0 games for empty data, got %d", rdb.GameCount())
	}

	// Exactly at boundary
	shortData := make([]byte, 0x10)
	rdb = Parse(shortData)
	if rdb.GameCount() != 0 {
		t.Errorf("expected 0 games for short data, got %d", rdb.GameCount())
	}
}

func TestParseNilTerminated(t *testing.T) {
	// 0x10 header bytes + mpfNil byte should yield 0 games
	data := make([]byte, 0x11)
	data[0x10] = mpfNil
	rdb := Parse(data)
	if rdb.GameCount() != 0 {
		t.Errorf("expected 0 games for nil-terminated data, got %d", rdb.GameCount())
	}
}
