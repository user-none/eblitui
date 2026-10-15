// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package scanner

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const chtExt = ".cht"

// CheatListing holds the cheat CHT files in a variant's libretro-database
// cht directory. Rumble CHT files in the same directory are excluded.
type CheatListing struct {
	urls   map[string]string // file base name (no extension) -> download URL
	byNorm map[string]string // normalized base name -> file base name
}

// newCheatListing creates an empty CheatListing.
func newCheatListing() *CheatListing {
	return &CheatListing{
		urls:   make(map[string]string),
		byNorm: make(map[string]string),
	}
}

// addCheatEntry adds a file from the directory listing. Files without the
// .cht extension and rumble CHT files are skipped.
func (cl *CheatListing) addCheatEntry(name, downloadURL string) {
	lower := strings.ToLower(name)
	if !strings.HasSuffix(lower, chtExt) || strings.HasSuffix(lower, strings.ToLower(rumbleSuffix)) {
		return
	}
	if downloadURL == "" {
		return
	}

	base := name[:len(name)-len(chtExt)]
	cl.urls[base] = downloadURL
	if norm := normalizeName(base); norm != "" {
		cl.byNorm[norm] = base
	}
}

// pressingTag matches the contents of a disc pressing tag group, such as
// "2S", "13M", or "2M, 3M". Pressings of a release hold the same game, so
// a cheat file without the tag applies to them.
var pressingTag = regexp.MustCompile(`^\d+[A-Z](, \d+[A-Z])*$`)

// resolveCheatName finds the cheat file for an RDB game name and returns
// its base name (no extension). Each name from cheatNameCandidates is
// tried in order with matchCheatName; the first match wins.
func resolveCheatName(listing *CheatListing, gameName string) (string, bool) {
	if listing == nil || gameName == "" {
		return "", false
	}
	for _, name := range cheatNameCandidates(gameName) {
		if base, ok := matchCheatName(listing, name); ok {
			return base, true
		}
	}
	return "", false
}

// cheatNameCandidates returns the names tried when matching a game to a
// cheat file, most specific first: the game name, the name without disc
// pressing tags, then each of those with a region list such as
// "(USA, Brazil)" narrowed to each listed region. A name that does not
// split into a title and tag groups is returned alone.
func cheatNameCandidates(gameName string) []string {
	title, groups, ok := splitNameGroups(gameName)
	if !ok || len(groups) == 0 {
		return []string{gameName}
	}

	unpressed := make([]string, 0, len(groups))
	for _, g := range groups {
		if !pressingTag.MatchString(g) {
			unpressed = append(unpressed, g)
		}
	}

	// The range sees only the two sets present when it starts, not the
	// narrowed sets appended during it
	groupSets := [][]string{groups, unpressed}
	for _, set := range groupSets {
		if len(set) == 0 || !strings.Contains(set[0], ", ") {
			continue
		}
		for _, region := range strings.Split(set[0], ", ") {
			narrowed := append([]string{region}, set[1:]...)
			groupSets = append(groupSets, narrowed)
		}
	}

	var out []string
	seen := make(map[string]bool, len(groupSets))
	for _, set := range groupSets {
		name := title
		for _, g := range set {
			name += " (" + g + ")"
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// splitNameGroups splits a name of the form "Title (A) (B)" into the title
// and the contents of each parenthesized group. It returns false when the
// name has text outside the groups after the title, or unbalanced
// parentheses.
func splitNameGroups(name string) (string, []string, bool) {
	idx := strings.Index(name, " (")
	if idx < 0 {
		return name, nil, true
	}
	title := name[:idx]
	rest := name[idx:]

	var groups []string
	for rest != "" {
		if !strings.HasPrefix(rest, " (") {
			return "", nil, false
		}
		end := strings.IndexByte(rest, ')')
		if end < 0 {
			return "", nil, false
		}
		groups = append(groups, rest[2:end])
		rest = rest[end+1:]
	}
	return title, groups, true
}

// matchCheatName matches one name against the listing. It tries an exact
// name match, then a normalized match, then the shortest file whose
// normalized name extends the name's with extra tags, such as
// "Astal (USA) (RetroArch)" for "Astal (USA)".
func matchCheatName(listing *CheatListing, gameName string) (string, bool) {
	if _, ok := listing.urls[gameName]; ok {
		return gameName, true
	}

	norm := normalizeName(gameName)
	if norm == "" {
		return "", false
	}
	if base, ok := listing.byNorm[norm]; ok {
		return base, true
	}

	best := ""
	for n := range listing.byNorm {
		if !strings.HasPrefix(n, norm+" ") {
			continue
		}
		if best == "" || len(n) < len(best) || (len(n) == len(best) && n < best) {
			best = n
		}
	}
	if best == "" {
		return "", false
	}
	return listing.byNorm[best], true
}

// fetchCheatListing fetches the cheat CHT file listing for a variant using
// the GitHub Contents API (e.g. cht/Sega - Saturn). If the directory does
// not exist (404) or any error occurs, returns nil.
func fetchCheatListing(rdbName string) *CheatListing {
	contentsURL := fmt.Sprintf("%s/%s", chtContentsBaseURL, url.PathEscape(rdbName))

	entries, err := fetchContentsListing(contentsURL)
	if err != nil {
		return nil
	}

	listing := newCheatListing()
	for _, entry := range entries {
		if entry.Type != "file" {
			continue
		}
		listing.addCheatEntry(entry.Name, entry.DownloadURL)
	}
	return listing
}
