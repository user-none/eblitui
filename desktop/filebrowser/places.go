// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package filebrowser

import (
	"os"
	"path/filepath"
)

// PlaceKind groups sidebar places.
type PlaceKind int

const (
	// PlaceFolder is a standard user folder (Home, Desktop, ...).
	PlaceFolder PlaceKind = iota
	// PlaceVolume is a filesystem root, drive or mounted volume.
	PlaceVolume
)

// Place is one sidebar entry. Label is for display only; Path is the real
// filesystem path.
type Place struct {
	Label string
	Path  string
	Kind  PlaceKind
}

// Places returns the sidebar entries: Home, the standard folders that exist,
// then the platform's roots and volumes.
func Places() []Place {
	var places []Place
	if home, err := os.UserHomeDir(); err == nil {
		places = append(places, Place{Label: "Home", Path: home, Kind: PlaceFolder})
		for _, sf := range standardFolders(home) {
			// user-dirs.dirs points a disabled folder at the home directory
			if filepath.Clean(sf.Path) == filepath.Clean(home) || !isDir(sf.Path) {
				continue
			}
			places = append(places, sf)
		}
	}
	return append(places, volumes()...)
}

// HomeDir returns the user's home directory, or the filesystem root of the
// working directory when it cannot be determined.
func HomeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	wd, err := os.Getwd()
	if err != nil {
		return string(filepath.Separator)
	}
	return filepath.VolumeName(wd) + string(filepath.Separator)
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
