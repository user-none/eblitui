// Package rdb is a parser for RDB files, a binary database of games with
// metadata used by RetroArch/libretro.
//
// Adapted from github.com/libretro/ludo/rdb
// Original Copyright (c) libretro team
package rdb

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// Game represents a game entry in the RDB
type Game struct {
	Name         string // Full No-Intro name (e.g., "Sonic the Hedgehog (USA, Europe)")
	Description  string
	Genre        string
	Developer    string
	Publisher    string
	Franchise    string
	ESRBRating   string
	ROMName      string // ROM filename
	ReleaseMonth uint
	ReleaseYear  uint
	Size         uint64
	CRC32        uint32
	Serial       string
	MD5          string // MD5 hash for RetroAchievements lookup
}

// RDB contains all game entries from a parsed RDB file
type RDB struct {
	games   []Game
	byCRC32 map[uint32]*Game // Index for fast CRC32 lookups
	byMD5   map[string]*Game // Index for fast MD5 lookups
}

// MessagePack format constants
const (
	mpfFixMap   = 0x80
	mpfMap16    = 0xde
	mpfMap32    = 0xdf
	mpfFixArray = 0x90
	mpfFixStr   = 0xa0
	mpfStr8     = 0xd9
	mpfStr16    = 0xda
	mpfStr32    = 0xdb
	mpfBin8     = 0xc4
	mpfBin16    = 0xc5
	mpfBin32    = 0xc6
	mpfUint8    = 0xcc
	mpfUint16   = 0xcd
	mpfUint32   = 0xce
	mpfUint64   = 0xcf
	mpfNil      = 0xc0
)

// LoadRDB loads and parses an RDB file from disk
func LoadRDB(path string) (*RDB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read RDB file: %w", err)
	}
	return Parse(data), nil
}

// Parse parses RDB file content and returns an RDB database
func Parse(data []byte) *RDB {
	games := parseGames(data)

	rdb := &RDB{
		games:   games,
		byCRC32: make(map[uint32]*Game, len(games)),
		byMD5:   make(map[string]*Game, len(games)),
	}

	// Build CRC32 and MD5 indexes
	for i := range rdb.games {
		if rdb.games[i].CRC32 != 0 {
			rdb.byCRC32[rdb.games[i].CRC32] = &rdb.games[i]
		}
		if rdb.games[i].MD5 != "" {
			rdb.byMD5[rdb.games[i].MD5] = &rdb.games[i]
		}
	}

	return rdb
}

// FindByCRC32 looks up a game by its CRC32 checksum
func (rdb *RDB) FindByCRC32(crc32 uint32) *Game {
	return rdb.byCRC32[crc32]
}

// FindByMD5 looks up a game by its MD5 hash
func (rdb *RDB) FindByMD5(md5 string) *Game {
	return rdb.byMD5[md5]
}

// GetMD5ByCRC32 returns the MD5 hash for a game found by CRC32
func (rdb *RDB) GetMD5ByCRC32(crc32 uint32) string {
	if g := rdb.byCRC32[crc32]; g != nil {
		return g.MD5
	}
	return ""
}

// GameCount returns the number of games in the database
func (rdb *RDB) GameCount() int {
	return len(rdb.games)
}

// GetDisplayName extracts a clean display name from a No-Intro name
// by removing region/version information in parentheses
func GetDisplayName(name string) string {
	// Find the first parenthesis
	idx := strings.Index(name, " (")
	if idx > 0 {
		return strings.TrimSpace(name[:idx])
	}
	return name
}

// GetRegionFromName extracts region information from a No-Intro name
// Returns "us", "eu", "jp", or "" if unknown
func GetRegionFromName(name string) string {
	nameLower := strings.ToLower(name)

	// Check for region indicators in parentheses
	if strings.Contains(nameLower, "(usa") ||
		strings.Contains(nameLower, "(us)") ||
		strings.Contains(nameLower, ", usa)") {
		return "us"
	}
	if strings.Contains(nameLower, "(europe") ||
		strings.Contains(nameLower, "(eu)") ||
		strings.Contains(nameLower, ", europe)") {
		return "eu"
	}
	if strings.Contains(nameLower, "(japan") ||
		strings.Contains(nameLower, "(jp)") ||
		strings.Contains(nameLower, ", japan)") {
		return "jp"
	}

	// Check for combined regions - default to US for multi-region
	if strings.Contains(nameLower, "(usa, europe)") ||
		strings.Contains(nameLower, "(world)") {
		return "us"
	}

	return ""
}

// parseGames parses the MessagePack-encoded RDB data
func parseGames(data []byte) []Game {
	if len(data) < 0x11 {
		return nil
	}

	var output []Game
	pos := 0x10
	iskey := false
	key := ""
	g := Game{}

	for pos < len(data) && int(data[pos]) != mpfNil {
		fieldtype := int(data[pos])
		var value []byte

		if fieldtype < mpfFixMap {
			// Positive fixint - skip
		} else if fieldtype < mpfFixArray {
			// fixmap - new game entry
			if g.Name != "" || g.CRC32 != 0 {
				output = append(output, g)
			}
			g = Game{}
			pos++
			iskey = true
			continue
		} else if fieldtype < mpfNil {
			// fixstr
			length := int(data[pos]) - mpfFixStr
			pos++
			if pos+length > len(data) {
				break
			}
			value = data[pos : pos+length]
			pos += length
		}

		switch fieldtype {
		case mpfStr8, mpfStr16, mpfStr32:
			pos++
			lenlen := fieldtype - mpfStr8 + 1
			if pos+lenlen > len(data) {
				break
			}
			lenhex := fmt.Sprintf("%x", string(data[pos:pos+lenlen]))
			i64, _ := strconv.ParseInt(lenhex, 16, 32)
			length := int(i64)
			pos += lenlen
			if pos+length > len(data) {
				break
			}
			value = data[pos : pos+length]
			pos += length

		case mpfUint8, mpfUint16, mpfUint32, mpfUint64:
			pow := float64(data[pos]) - 0xC9
			length := int(math.Pow(2, pow)) / 8
			pos++
			if pos+length > len(data) {
				break
			}
			value = data[pos : pos+length]
			pos += length

		case mpfBin8, mpfBin16, mpfBin32:
			pos++
			if pos >= len(data) {
				break
			}
			length := int(data[pos])
			pos++
			if pos+length > len(data) {
				break
			}
			value = data[pos : pos+length]
			pos += length

		case mpfMap16, mpfMap32:
			// Map16/Map32 mark a new game entry (same as fixmap but for 16+ fields)
			if g.Name != "" || g.CRC32 != 0 {
				output = append(output, g)
			}
			g = Game{}
			length := 2
			if int(data[pos]) == mpfMap32 {
				length = 4
			}
			pos++
			if pos+length > len(data) {
				break
			}
			pos += length
			iskey = true
			continue
		}

		if iskey {
			key = string(value)
		} else {
			setGameField(&g, key, string(value))
		}
		iskey = !iskey
	}

	// Don't forget the last entry
	if g.Name != "" || g.CRC32 != 0 {
		output = append(output, g)
	}

	return output
}

// setGameField sets a field in the game entry
func setGameField(g *Game, key string, value string) {
	switch key {
	case "name":
		g.Name = value
	case "description":
		g.Description = value
	case "genre":
		g.Genre = value
	case "developer":
		g.Developer = value
	case "publisher":
		g.Publisher = value
	case "franchise":
		g.Franchise = value
	case "esrb_rating":
		g.ESRBRating = value
	case "serial":
		g.Serial = value
	case "rom_name":
		g.ROMName = value
	case "size":
		v := fmt.Sprintf("%x", value)
		u64, _ := strconv.ParseUint(v, 16, 64)
		g.Size = u64
	case "releasemonth":
		v := fmt.Sprintf("%x", value)
		u64, _ := strconv.ParseUint(v, 16, 32)
		g.ReleaseMonth = uint(u64)
	case "releaseyear":
		v := fmt.Sprintf("%x", value)
		u64, _ := strconv.ParseUint(v, 16, 32)
		g.ReleaseYear = uint(u64)
	case "crc":
		v := fmt.Sprintf("%x", value)
		u64, _ := strconv.ParseUint(v, 16, 32)
		g.CRC32 = uint32(u64)
	case "md5":
		// MD5 is stored as raw 16 bytes in RDB, convert to hex string
		g.MD5 = fmt.Sprintf("%x", value)
	}
}
