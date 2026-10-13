// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package filebrowser

import (
	"golang.org/x/sys/windows"
)

// standardFolders returns Desktop, Documents and Downloads from the known
// folder API so redirected folders resolve to their real location.
func standardFolders(home string) []Place {
	known := []struct {
		label string
		id    *windows.KNOWNFOLDERID
	}{
		{"Desktop", windows.FOLDERID_Desktop},
		{"Documents", windows.FOLDERID_Documents},
		{"Downloads", windows.FOLDERID_Downloads},
	}
	out := make([]Place, 0, len(known))
	for _, k := range known {
		p, err := windows.KnownFolderPath(k.id, 0)
		if err != nil {
			continue
		}
		out = append(out, Place{Label: k.label, Path: p, Kind: PlaceFolder})
	}
	return out
}

// volumes lists every drive letter, labeled by drive type. The type comes from
// the root path alone so no media is accessed.
func volumes() []Place {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	var out []Place
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		letter := string(rune('A' + i))
		root := letter + `:\`
		rp, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		label := driveTypeLabel(windows.GetDriveType(rp))
		out = append(out, Place{Label: label + " (" + letter + ":)", Path: root, Kind: PlaceVolume})
	}
	return out
}

// driveTypeLabel returns the display name for a GetDriveType result.
func driveTypeLabel(t uint32) string {
	switch t {
	case windows.DRIVE_FIXED:
		return "Local Disk"
	case windows.DRIVE_REMOVABLE:
		return "Removable Disk"
	case windows.DRIVE_CDROM:
		return "CD Drive"
	case windows.DRIVE_REMOTE:
		return "Network Drive"
	case windows.DRIVE_RAMDISK:
		return "RAM Disk"
	default:
		return "Drive"
	}
}
