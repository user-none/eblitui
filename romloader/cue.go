// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package romloader

import (
	"fmt"
	"strconv"
	"strings"
)

const cueSectorBytes = 2352

// cueIndex is one INDEX line: its number and its file-relative position in
// frames (sectors).
type cueIndex struct {
	number int
	frame  int
}

// splitKeyword splits a line into its leading keyword and the remainder.
func splitKeyword(line string) (keyword, rest string) {
	i := strings.IndexAny(line, " \t")
	if i < 0 {
		return line, ""
	}
	return line[:i], strings.TrimSpace(line[i+1:])
}

// parseFileLine extracts the filename and file type from the remainder of a
// FILE directive. The filename may be double-quoted (and contain spaces) or a
// single bare token.
func parseFileLine(rest string) (filename, ftype string) {
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "\"") {
		end := strings.IndexByte(rest[1:], '"')
		if end < 0 {
			return "", ""
		}
		filename = rest[1 : 1+end]
		ftype = strings.TrimSpace(rest[1+end+1:])
		return filename, ftype
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", ""
	}
	if len(fields) == 1 {
		return fields[0], ""
	}
	return fields[0], fields[len(fields)-1]
}

// normalizeTrackType maps a cue track datatype to the CHD type vocabulary and
// its CTRL/ADR control byte. Only raw 2352-byte modes are supported.
func normalizeTrackType(s string) (typ string, control uint8, err error) {
	switch strings.ToUpper(s) {
	case "AUDIO":
		return "AUDIO", 0x01, nil
	case "MODE1/2352":
		return "MODE1_RAW", 0x41, nil
	case "MODE2/2352":
		return "MODE2_RAW", 0x41, nil
	default:
		return "", 0, fmt.Errorf("unsupported track mode %q (only raw 2352 modes supported)", s)
	}
}

// parseMSF parses an "mm:ss:ff" timestamp into a frame (sector) count, where
// there are 75 frames per second.
func parseMSF(s string) (int, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("bad MSF %q", s)
	}
	m, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("bad MSF minutes %q", s)
	}
	sec, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("bad MSF seconds %q", s)
	}
	f, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, fmt.Errorf("bad MSF frames %q", s)
	}
	if m < 0 || sec < 0 || sec >= 60 || f < 0 || f >= 75 {
		return 0, fmt.Errorf("MSF out of range %q", s)
	}
	return (m*60+sec)*75 + f, nil
}
