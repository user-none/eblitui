// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build darwin

package filebrowser

import (
	"os"
	"path/filepath"
	"strings"
)

// standardFolders returns Desktop, Documents and Downloads under home.
func standardFolders(home string) []Place {
	return []Place{
		{Label: "Desktop", Path: filepath.Join(home, "Desktop"), Kind: PlaceFolder},
		{Label: "Documents", Path: filepath.Join(home, "Documents"), Kind: PlaceFolder},
		{Label: "Downloads", Path: filepath.Join(home, "Downloads"), Kind: PlaceFolder},
	}
}

// volumes lists /Volumes. The entry that links to / is the boot volume; it is
// shown as the root using the volume's name as the label. Other directories
// are mounted volumes.
func volumes() []Place {
	root := Place{Label: "/", Path: "/", Kind: PlaceVolume}
	des, err := os.ReadDir("/Volumes")
	if err != nil {
		return []Place{root}
	}
	var vols []Place
	for _, de := range des {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		p := filepath.Join("/Volumes", name)
		if de.Type()&os.ModeSymlink != 0 {
			if target, err := filepath.EvalSymlinks(p); err == nil && target == "/" {
				root.Label = name
			}
			continue
		}
		if de.IsDir() {
			vols = append(vols, Place{Label: name, Path: p, Kind: PlaceVolume})
		}
	}
	return append([]Place{root}, vols...)
}
