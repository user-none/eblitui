// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package filebrowser

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func mkdirs(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func mkfiles(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func names(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReadDirFolderMode(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "beta", "Alpha", ".hidden")
	mkfiles(t, root, "game.bin", ".dotfile")

	entries, err := ReadDir(root, ModeFolder)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Alpha", "beta"}
	if got := names(entries); !equalStrings(got, want) {
		t.Errorf("names = %v, want %v", got, want)
	}
	for _, e := range entries {
		if !e.IsDir {
			t.Errorf("%s: IsDir = false in folder mode", e.Name)
		}
		if e.Path != filepath.Join(root, e.Name) {
			t.Errorf("%s: Path = %q", e.Name, e.Path)
		}
	}
}

func TestReadDirFileMode(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "zeta", "Bios")
	mkfiles(t, root, "b.rom", "A.rom", "c.rom", ".dotfile")

	entries, err := ReadDir(root, ModeFile)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Bios", "zeta", "A.rom", "b.rom", "c.rom"}
	if got := names(entries); !equalStrings(got, want) {
		t.Errorf("names = %v, want %v", got, want)
	}
}

func TestReadDirCaseTieBreak(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("case-insensitive filesystem")
	}
	root := t.TempDir()
	mkfiles(t, root, "rom", "ROM")

	entries, err := ReadDir(root, ModeFile)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ROM", "rom"}
	if got := names(entries); !equalStrings(got, want) {
		t.Errorf("names = %v, want %v", got, want)
	}
}

func TestReadDirSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	root := t.TempDir()
	target := t.TempDir()
	mkdirs(t, target, "real")
	mkfiles(t, target, "real.bin")
	links := map[string]string{
		"dirlink":  filepath.Join(target, "real"),
		"filelink": filepath.Join(target, "real.bin"),
		"broken":   filepath.Join(target, "missing"),
	}
	for name, dst := range links {
		if err := os.Symlink(dst, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}

	folders, err := ReadDir(root, ModeFolder)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(folders); !equalStrings(got, []string{"dirlink"}) {
		t.Errorf("folder mode names = %v, want [dirlink]", got)
	}
	if !folders[0].IsDir {
		t.Error("dirlink should list as a directory")
	}

	files, err := ReadDir(root, ModeFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(files); !equalStrings(got, []string{"dirlink", "filelink"}) {
		t.Errorf("file mode names = %v, want [dirlink filelink]", got)
	}
}

func TestReadDirMissing(t *testing.T) {
	if _, err := ReadDir(filepath.Join(t.TempDir(), "missing"), ModeFolder); err == nil {
		t.Error("expected error for missing directory")
	}
}

func TestFilter(t *testing.T) {
	entries := []Entry{{Name: "Genesis"}, {Name: "Master System"}, {Name: "game gear"}}

	tests := []struct {
		text string
		want []string
	}{
		{"", []string{"Genesis", "Master System", "game gear"}},
		{"GE", []string{"Genesis", "game gear"}},
		{"sys", []string{"Master System"}},
		{"zzz", []string{}},
	}
	for _, tc := range tests {
		if got := names(Filter(entries, tc.text)); !equalStrings(got, tc.want) {
			t.Errorf("Filter(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestSplitInput(t *testing.T) {
	sep := string(filepath.Separator)
	tests := []struct {
		text, dir, filter string
	}{
		{"", "", ""},
		{"roms", "", "roms"},
		{sep, sep, ""},
		{sep + "ge", sep, "ge"},
		{sep + "home" + sep + "user" + sep, sep + "home" + sep + "user" + sep, ""},
		{sep + "home" + sep + "user" + sep + "ro", sep + "home" + sep + "user" + sep, "ro"},
	}
	for _, tc := range tests {
		dir, filter := SplitInput(tc.text)
		if dir != tc.dir || filter != tc.filter {
			t.Errorf("SplitInput(%q) = (%q, %q), want (%q, %q)", tc.text, dir, filter, tc.dir, tc.filter)
		}
	}
}

func TestParent(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")

	if p, ok := Parent(sub); !ok || p != root {
		t.Errorf("Parent(%q) = (%q, %v), want (%q, true)", sub, p, ok, root)
	}
	if p, ok := Parent(sub + string(filepath.Separator)); !ok || p != root {
		t.Errorf("Parent with trailing separator = (%q, %v), want (%q, true)", p, ok, root)
	}

	fsRoot := filepath.VolumeName(root) + string(filepath.Separator)
	if _, ok := Parent(fsRoot); ok {
		t.Errorf("Parent(%q) should report root", fsRoot)
	}
}

func TestDirText(t *testing.T) {
	sep := string(filepath.Separator)
	root := t.TempDir()
	fsRoot := filepath.VolumeName(root) + sep

	tests := []struct {
		dir, want string
	}{
		{fsRoot, fsRoot},
		{root, root + sep},
		{root + sep, root + sep},
		{root + sep + "a" + sep + ".." + sep + "b", root + sep + "b" + sep},
	}
	for _, tc := range tests {
		if got := DirText(tc.dir); got != tc.want {
			t.Errorf("DirText(%q) = %q, want %q", tc.dir, got, tc.want)
		}
	}
}

func TestResolve(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "dir")
	mkfiles(t, root, "file.bin")
	dir := filepath.Join(root, "dir")
	file := filepath.Join(root, "file.bin")
	sep := string(filepath.Separator)

	tests := []struct {
		name    string
		text    string
		mode    Mode
		want    string
		wantErr bool
	}{
		{"empty", "", ModeFolder, "", true},
		{"relative", "dir", ModeFolder, "", true},
		{"missing", filepath.Join(root, "missing"), ModeFolder, "", true},
		{"folder", dir, ModeFolder, dir, false},
		{"folder trailing sep", dir + sep, ModeFolder, dir, false},
		{"file in folder mode", file, ModeFolder, "", true},
		{"file", file, ModeFile, file, false},
		{"folder in file mode", dir, ModeFile, "", true},
		{"uncleaned", dir + sep + ".." + sep + "file.bin", ModeFile, file, false},
	}
	for _, tc := range tests {
		got, err := Resolve(tc.text, tc.mode)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tc.name, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: Resolve = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestPlacesStartsWithHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	places := Places()
	if len(places) == 0 || places[0].Path != home || places[0].Label != "Home" {
		t.Fatalf("first place = %+v, want Home at %q", places, home)
	}
	sawVolume := false
	for _, p := range places {
		if p.Kind == PlaceVolume {
			sawVolume = true
		} else if sawVolume {
			t.Errorf("folder %+v listed after volumes", p)
		}
	}
}
