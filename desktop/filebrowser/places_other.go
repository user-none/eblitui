// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux && !darwin && !windows

package filebrowser

import "path/filepath"

// standardFolders returns Desktop, Documents and Downloads under home.
func standardFolders(home string) []Place {
	return []Place{
		{Label: "Desktop", Path: filepath.Join(home, "Desktop"), Kind: PlaceFolder},
		{Label: "Documents", Path: filepath.Join(home, "Documents"), Kind: PlaceFolder},
		{Label: "Downloads", Path: filepath.Join(home, "Downloads"), Kind: PlaceFolder},
	}
}

// volumes returns the filesystem root.
func volumes() []Place {
	return []Place{{Label: "/", Path: "/", Kind: PlaceVolume}}
}
