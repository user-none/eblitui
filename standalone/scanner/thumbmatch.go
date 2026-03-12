package scanner

import (
	"strings"
	"unicode"
)

// thumbEntry holds a parsed thumbnail listing entry.
type thumbEntry struct {
	original string   // original full filename (no .png)
	groups   []string // normalized paren groups: ["usa", "enja", ...]
}

// ThumbnailListing holds the directory listing for a single variant's
// thumbnail repository. Each artwork type has indexed entries for fast
// lookup by base title.
type ThumbnailListing struct {
	// Maps keyed by artwork type ("Named_Boxarts", etc.)
	Exact  map[string]map[string]string       // artType -> exactKey -> original
	ByBase map[string]map[string][]thumbEntry // artType -> normBase -> candidates
}

// newThumbnailListing creates an empty ThumbnailListing.
func newThumbnailListing() *ThumbnailListing {
	return &ThumbnailListing{
		Exact:  make(map[string]map[string]string),
		ByBase: make(map[string]map[string][]thumbEntry),
	}
}

// addEntry adds a single filename entry for an artwork type.
// name should be the filename without the .png extension.
func (tl *ThumbnailListing) addEntry(artType string, name string) {
	if tl.Exact[artType] == nil {
		tl.Exact[artType] = make(map[string]string)
		tl.ByBase[artType] = make(map[string][]thumbEntry)
	}

	// Exact map: & replaced with _ (matches libretro convention)
	exactKey := strings.ReplaceAll(name, "&", "_")
	tl.Exact[artType][exactKey] = name

	// Parse and index by normalized base title
	base, groups := parseName(name)
	normBase := normalizeName(base)
	tl.ByBase[artType][normBase] = append(tl.ByBase[artType][normBase], thumbEntry{
		original: name,
		groups:   groups,
	})
}

// parseName splits a name into a base title and normalized parenthetical
// groups. Square-bracketed sections (dump tags like [!], [b1]) are stripped
// first. The base is the trimmed text before the first '('. Groups are the
// normalized contents of each (...) pair.
func parseName(s string) (string, []string) {
	// Strip [...] sections
	var stripped strings.Builder
	inBracket := false
	for _, r := range s {
		if r == '[' {
			inBracket = true
			continue
		}
		if r == ']' {
			inBracket = false
			continue
		}
		if !inBracket {
			stripped.WriteRune(r)
		}
	}
	s = stripped.String()

	// Find first '('
	idx := strings.IndexByte(s, '(')
	if idx < 0 {
		return strings.TrimSpace(s), nil
	}

	base := strings.TrimSpace(s[:idx])

	// Extract parenthetical groups
	var groups []string
	rest := s[idx:]
	for len(rest) > 0 {
		open := strings.IndexByte(rest, '(')
		if open < 0 {
			break
		}
		close := strings.IndexByte(rest[open:], ')')
		if close < 0 {
			break
		}
		content := rest[open+1 : open+close]
		norm := normalizeName(content)
		if norm != "" {
			groups = append(groups, norm)
		}
		rest = rest[open+close+1:]
	}

	return base, groups
}

// scoreMatch scores how well candidate groups match game groups.
// Exact normalized match = 2 points, prefix match = 1 point.
// Score = totalPoints * 1000 + len(candidateGroups) for tiebreaking.
func scoreMatch(gameGroups []string, candidateGroups []string) int {
	if len(candidateGroups) == 0 {
		return 0
	}

	totalPoints := 0
	for _, cg := range candidateGroups {
		best := 0
		for _, gg := range gameGroups {
			if cg == gg {
				best = 2
				break
			}
			// Prefix match: shorter is prefix of longer
			if best < 1 && (strings.HasPrefix(cg, gg) || strings.HasPrefix(gg, cg)) {
				best = 1
			}
		}
		totalPoints += best
	}

	return totalPoints*1000 + len(candidateGroups)
}

// normalizeName lowercases, replaces & with _, strips non-alphanumeric
// characters (keeping spaces), collapses multiple spaces, and trims.
func normalizeName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "&", "_")

	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			b.WriteRune(r)
		}
	}

	// Collapse multiple spaces and trim
	fields := strings.Fields(b.String())
	return strings.Join(fields, " ")
}

// resolveArtworkNameForType tries to find a matching artwork filename in the
// listing for a single artwork type. Tries exact match, base+group scoring,
// then fuzzy matching. Returns the original filename and whether a match was found.
func resolveArtworkNameForType(listing *ThumbnailListing, artType string, gameName string) (string, bool) {
	if listing == nil || gameName == "" {
		return "", false
	}

	exactName := strings.ReplaceAll(gameName, "&", "_")
	gameBase, gameGroups := parseName(gameName)
	normBase := normalizeName(gameBase)

	if normBase == "" {
		return "", false
	}

	exactMap := listing.Exact[artType]
	baseMap := listing.ByBase[artType]

	if exactMap == nil {
		return "", false
	}

	// 1. Exact match on full name
	if orig, ok := exactMap[exactName]; ok {
		return orig, true
	}

	// 2. Base title lookup with group scoring
	if candidates, ok := baseMap[normBase]; ok {
		if orig, found := bestCandidate(candidates, gameGroups); found {
			return orig, true
		}
	}

	// 3. Fuzzy base title match
	firstWord := normBase
	if idx := strings.IndexByte(normBase, ' '); idx > 0 {
		firstWord = normBase[:idx]
	}

	bestScore := 0.0
	var bestCandidates []thumbEntry
	for key, candidates := range baseMap {
		entryFirst := key
		if idx := strings.IndexByte(key, ' '); idx > 0 {
			entryFirst = key[:idx]
		}
		if entryFirst != firstWord {
			continue
		}

		sim := similarityScore(normBase, key)
		if sim >= 0.85 && sim > bestScore {
			bestScore = sim
			bestCandidates = candidates
		}
	}
	if bestCandidates != nil {
		if orig, found := bestCandidate(bestCandidates, gameGroups); found {
			return orig, true
		}
	}

	return "", false
}

// bestCandidate picks the best candidate from a slice using group scoring.
// Returns the original name and true, or empty and false if no candidates.
func bestCandidate(candidates []thumbEntry, gameGroups []string) (string, bool) {
	if len(candidates) == 0 {
		return "", false
	}

	bestScore := -1
	bestOrig := ""
	for _, c := range candidates {
		s := scoreMatch(gameGroups, c.groups)
		if s > bestScore {
			bestScore = s
			bestOrig = c.original
		}
	}

	return bestOrig, true
}

// levenshtein computes the Levenshtein edit distance between two strings.
func levenshtein(a string, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	la := len(ra)
	lb := len(rb)

	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use single row optimization
	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost

			min := del
			if ins < min {
				min = ins
			}
			if sub < min {
				min = sub
			}
			curr[j] = min
		}
		prev = curr
	}

	return prev[lb]
}

// similarityScore returns a similarity score between 0 and 1 based on
// Levenshtein distance. Inputs should already be normalized.
// 1.0 means identical, 0.0 means completely different.
func similarityScore(a string, b string) float64 {
	maxLen := len([]rune(a))
	if l := len([]rune(b)); l > maxLen {
		maxLen = l
	}
	if maxLen == 0 {
		return 1.0
	}

	dist := levenshtein(a, b)
	return 1.0 - float64(dist)/float64(maxLen)
}
