package chd

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	metaTagCHTR = 0x43485452 // "CHTR"
	metaTagCHT2 = 0x43485432 // "CHT2"
	metaTagCHGD = 0x43484744 // "CHGD" - GD-ROM track info (same text format + PAD)
)

func (rd *Reader) parseMetadata(metaOffset uint64) error {
	if metaOffset == 0 {
		return nil
	}

	var rawTracks []Track
	offset := metaOffset

	for offset != 0 {
		var node [16]byte
		if _, err := rd.r.ReadAt(node[:], int64(offset)); err != nil {
			return fmt.Errorf("reading metadata node at %d: %w", offset, err)
		}

		tag := binary.BigEndian.Uint32(node[0:4])
		flagsAndLen := binary.BigEndian.Uint32(node[4:8])
		payloadLen := flagsAndLen & 0x00FFFFFF
		nextOffset := binary.BigEndian.Uint64(node[8:16])

		if (tag == metaTagCHTR || tag == metaTagCHT2 || tag == metaTagCHGD) && payloadLen > 0 {
			payload := make([]byte, payloadLen)
			if _, err := rd.r.ReadAt(payload, int64(offset)+16); err != nil {
				return fmt.Errorf("reading track metadata payload: %w", err)
			}
			t, err := parseTrackLine(payload)
			if err != nil {
				return fmt.Errorf("parsing track metadata: %w", err)
			}
			rawTracks = append(rawTracks, t)
		}

		offset = nextOffset
	}

	sort.Slice(rawTracks, func(i, j int) bool {
		return rawTracks[i].Number < rawTracks[j].Number
	})

	// Compute layout offsets. CHDMAN pads each track in the CHD file
	// to a multiple of 4 frames; the padding lives only in the CHD
	// layout, not the logical LBA layout. CHTR/CHT2 Frames already
	// includes the pregap frames at the start of the track.
	const trackPadding = 4
	chdOffset := 0
	lbaOffset := 0
	for i := range rawTracks {
		t := &rawTracks[i]
		t.chdStart = chdOffset
		t.startLBA = lbaOffset
		extra := ((t.Frames+trackPadding-1)/trackPadding)*trackPadding - t.Frames
		chdOffset += t.Frames + extra
		lbaOffset += t.Frames
	}

	rd.tracks = rawTracks
	return nil
}

func parseTrackLine(data []byte) (Track, error) {
	line := strings.TrimSpace(string(data))
	fields := strings.Fields(line)

	var t Track
	for _, f := range fields {
		parts := strings.SplitN(f, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], parts[1]
		switch key {
		case "TRACK":
			n, err := strconv.Atoi(val)
			if err != nil {
				return t, fmt.Errorf("bad TRACK number %q: %w", val, err)
			}
			t.Number = n
		case "TYPE":
			t.Type = val
		case "FRAMES":
			n, err := strconv.Atoi(val)
			if err != nil {
				return t, fmt.Errorf("bad FRAMES %q: %w", val, err)
			}
			t.Frames = n
		case "PREFRAMES", "PREGAP":
			n, err := strconv.Atoi(val)
			if err != nil {
				return t, fmt.Errorf("bad %s %q: %w", key, val, err)
			}
			t.Pregap = n
		}
	}

	if t.Number == 0 {
		return t, fmt.Errorf("missing TRACK number in %q", line)
	}

	if t.Type == "AUDIO" {
		t.Control = 0x01
	} else {
		t.Control = 0x41
	}
	return t, nil
}
