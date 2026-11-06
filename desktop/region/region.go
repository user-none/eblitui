// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package region defines the game regions the desktop supports: USA,
// Europe and Japan. Each has a lower case code used by the library, the
// artwork and the settings. Every other region is not supported and is
// ignored wherever regions are shown or chosen.
package region

import (
	"slices"
	"strings"
)

// The supported regions
const (
	US = "us" // USA
	EU = "eu" // Europe
	JP = "jp" // Japan
)

// All lists the supported regions in the order they are shown and
// preferred
var All = []string{US, EU, JP}

// codes maps a metadata region name to its supported region. The European
// countries and Australia, the PAL releases, belong to EU.
var codes = map[string]string{
	"USA":       US,
	"Japan":     JP,
	"Europe":    EU,
	"UK":        EU,
	"Germany":   EU,
	"France":    EU,
	"Spain":     EU,
	"Italy":     EU,
	"Finland":   EU,
	"Sweden":    EU,
	"Australia": EU,
}

// Code returns the supported region of a metadata region name, such as EU
// for "Germany", or "" when the region is not supported.
func Code(name string) string {
	return codes[name]
}

// Codes returns the supported regions that a list of metadata region names
// belongs to, each once, in the order the names first reach them. USA,
// Germany and France give US, EU.
func Codes(names []string) []string {
	var out []string
	for _, n := range names {
		if c := codes[n]; c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// Abbreviations returns region codes upper case and comma separated, such
// as "US, EU", or "" when there are none
func Abbreviations(codes []string) string {
	return strings.ToUpper(strings.Join(codes, ", "))
}

// Names returns the display names of region codes, comma separated, such
// as "USA, Europe", or "" when there are none
func Names(codes []string) string {
	names := make([]string, 0, len(codes))
	for _, c := range codes {
		names = append(names, DisplayName(c))
	}
	return strings.Join(names, ", ")
}

// Supported reports whether code is one of All
func Supported(code string) bool {
	return slices.Contains(All, code)
}

// DisplayName returns the user-facing name of a supported region, such as
// "Europe" for EU, or "" for any other code.
func DisplayName(code string) string {
	switch code {
	case US:
		return "USA"
	case EU:
		return "Europe"
	case JP:
		return "Japan"
	}
	return ""
}
