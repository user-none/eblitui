package metadata

import (
	"testing"

	"github.com/user-none/eblitui/desktop/content"
	"github.com/user-none/eblitui/desktop/storage"
)

// fakeProvider is a content.MetadataSource over a fixed set of entries.
type fakeProvider struct {
	games map[string]content.GameMetadata
	calls int
}

func (p *fakeProvider) GameMetadata(serial string) (content.GameMetadata, bool) {
	g, ok := p.games[serial]
	return g, ok
}

func (p *fakeProvider) AllGameMetadata() map[string]content.GameMetadata {
	p.calls++
	all := make(map[string]content.GameMetadata, len(p.games))
	for k, v := range p.games {
		all[k] = v
	}
	return all
}

func TestManagerGameAndTitle(t *testing.T) {
	p := &fakeProvider{games: map[string]content.GameMetadata{
		"GS-9001": {Serial: "GS-9001", Title: "Virtua Fighter", Regions: []string{"Japan"}},
		"NOTITLE": {Serial: "NOTITLE", Regions: []string{"USA"}},
	}}
	m := NewMetadataManager(p)
	if p.calls != 1 {
		t.Fatalf("AllGameMetadata called %d times, want 1", p.calls)
	}

	if g, ok := m.Game("GS-9001"); !ok || g.Title != "Virtua Fighter" {
		t.Errorf("Game(GS-9001) = %+v, %v", g, ok)
	}
	if _, ok := m.Game("MISSING"); ok {
		t.Error("Game(MISSING) found an entry")
	}

	vf := &storage.GameEntry{GameID: "GS-9001", Discs: []storage.GameDisc{{File: "/roms/Virtua Fighter (Japan).chd"}}}
	if got := m.Title(vf); got != "Virtua Fighter" {
		t.Errorf("Title(matched) = %q", got)
	}
	if got := m.Region(vf); got != "jp" {
		t.Errorf("Region(matched) = %q", got)
	}

	// An entry with no metadata, or metadata without a title, falls back to
	// its file name
	unknown := &storage.GameEntry{GameID: "MISSING", Discs: []storage.GameDisc{{File: "/roms/Some Game (USA) (Disc 1).chd"}}}
	if title, region := m.Info(unknown); title != "Some Game" || region != "" {
		t.Errorf("Info(unmatched) = %q, %q", title, region)
	}
	noTitle := &storage.GameEntry{GameID: "NOTITLE", File: "/roms/Other (Europe).zip"}
	if got := m.Title(noTitle); got != "Other" {
		t.Errorf("Title(no metadata title) = %q", got)
	}
}

func TestManagerWithoutProvider(t *testing.T) {
	m := NewMetadataManager(nil)
	if _, ok := m.Game("GS-9001"); ok {
		t.Error("Game found an entry without a provider")
	}
	g := &storage.GameEntry{GameID: "abcd1234", File: "/roms/Sonic (USA).sms"}
	if title, region := m.Info(g); title != "Sonic" || region != "" {
		t.Errorf("Info = %q, %q", title, region)
	}
}

func TestFallbackName(t *testing.T) {
	tests := []struct {
		name string
		game storage.GameEntry
		want string
	}{
		{"cartridge with tags", storage.GameEntry{File: "/roms/Alex Kidd (Europe) (Rev 1).sms"}, "Alex Kidd"},
		{"disc uses first disc", storage.GameEntry{Discs: []storage.GameDisc{{File: "/a/Game (Japan) (Disc 1).cue"}, {File: "/a/Other.cue"}}}, "Game"},
		{"no tags", storage.GameEntry{File: "/roms/Plain.sms"}, "Plain"},
		{"dots in name", storage.GameEntry{File: "/roms/Vol. 1 (Japan).chd"}, "Vol. 1"},
		{"no file", storage.GameEntry{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FallbackName(&tc.game); got != tc.want {
				t.Errorf("FallbackName = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRegionCode(t *testing.T) {
	tests := []struct {
		regions []string
		want    string
	}{
		{[]string{"Japan"}, "jp"},
		{[]string{"Germany", "USA", "Europe"}, "us"},
		{[]string{"France", "Japan"}, "jp"},
		{[]string{"Spain"}, "eu"},
		{[]string{"Korea"}, "kr"},
		{[]string{"Taiwan", "Korea"}, "tw"},
		{[]string{"Nowhere"}, ""},
		{nil, ""},
	}
	for _, tc := range tests {
		if got := RegionCode(tc.regions); got != tc.want {
			t.Errorf("RegionCode(%v) = %q, want %q", tc.regions, got, tc.want)
		}
	}
}

func TestRegionCodeFor(t *testing.T) {
	for region, want := range map[string]string{
		"USA": "us", "Germany": "eu", "Australia": "eu", "Japan": "jp", "Korea": "kr", "Nowhere": "", "": "",
	} {
		if got := RegionCodeFor(region); got != want {
			t.Errorf("RegionCodeFor(%q) = %q, want %q", region, got, want)
		}
	}
}

func TestFormatReleaseDate(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"1996-04-19", "April 1996"},
		{"1994-12-02", "December 1994"},
		{"1997", "1997"},
		{"", ""},
		{"1996-13-01", "1996-13-01"},
		{"not a date", "not a date"},
	}
	for _, tc := range tests {
		if got := FormatReleaseDate(tc.in); got != tc.want {
			t.Errorf("FormatReleaseDate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
