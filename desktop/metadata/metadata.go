// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package metadata

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/user-none/eblitui/desktop/content"
	"github.com/user-none/eblitui/desktop/region"
	"github.com/user-none/eblitui/desktop/storage"
)

// MetadataManager holds the emulator's game metadata.
type MetadataManager struct {
	games map[string]content.GameMetadata
}

// NewMetadataManager creates a metadata manager. When source is not nil,
// all of its game metadata is loaded once here; the map is read-only
// afterwards, so lookups are safe from any goroutine.
func NewMetadataManager(source content.MetadataSource) *MetadataManager {
	m := &MetadataManager{}
	if source != nil {
		m.games = source.AllGameMetadata()
	}
	return m
}

// Game returns the metadata for a game ID and true, or false when the
// emulator has no entry for it.
func (m *MetadataManager) Game(gameID string) (content.GameMetadata, bool) {
	g, ok := m.games[gameID]
	return g, ok
}

// Title returns the title to show for a library entry: the metadata title
// when there is one, else the entry's file name without its extension and
// parenthesized tags.
func (m *MetadataManager) Title(game *storage.GameEntry) string {
	if g, ok := m.games[game.GameID]; ok && g.Title != "" {
		return g.Title
	}
	return FallbackName(game)
}

// Region returns the region code for a library entry: the first of its
// supported regions in region.All order, or "" when it has none or the
// core has no metadata for it.
func (m *MetadataManager) Region(game *storage.GameEntry) string {
	if g, ok := m.games[game.GameID]; ok {
		if codes := region.Codes(g.Regions); len(codes) > 0 {
			return codes[0]
		}
	}
	return ""
}

// Info returns the title and region code for a library entry, for sorting
// and searching the library.
func (m *MetadataManager) Info(game *storage.GameEntry) (string, string) {
	return m.Title(game), m.Region(game)
}

// FallbackName returns a name for a library entry without metadata: the
// file name of its first disc, or its cartridge file, without the extension
// and anything from the first " (" on. Returns "" when the entry has no
// file.
func FallbackName(game *storage.GameEntry) string {
	file := game.File
	if len(game.Discs) > 0 {
		file = game.Discs[0].File
	}
	if file == "" {
		return ""
	}
	name := filepath.Base(file)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if idx := strings.Index(name, " ("); idx > 0 {
		name = strings.TrimSpace(name[:idx])
	}
	return name
}

var monthNames = [...]string{"January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December"}

// FormatReleaseDate formats a metadata release date for display: YYYY-MM-DD
// becomes "Month YYYY" and YYYY is returned as is. Any other value is
// returned unchanged.
func FormatReleaseDate(date string) string {
	var y, mo, d int
	if n, _ := fmt.Sscanf(date, "%4d-%2d-%2d", &y, &mo, &d); n == 3 && len(date) == 10 && mo >= 1 && mo <= 12 {
		return fmt.Sprintf("%s %d", monthNames[mo-1], y)
	}
	return date
}
