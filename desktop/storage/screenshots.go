// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ScreenshotAchievementTag starts the filename tag of a screenshot taken
// on an achievement unlock. The achievement title follows it.
const ScreenshotAchievementTag = "achievement_"

// screenshotExt is the extension of screenshot files
const screenshotExt = ".png"

// MaxScreenshotTitleLen is the most runes of an achievement title kept in
// a screenshot filename
const MaxScreenshotTitleLen = 64

// MaxScreenshotTitleBytes is the most UTF-8 bytes of an achievement title
// kept in a screenshot filename. It keeps the whole filename under the 255
// byte component limit of common filesystems when the title is made of
// wide runes.
const MaxScreenshotTitleBytes = 192

// screenshotTitlePunctuation is the punctuation allowed in a screenshot
// title besides letters, digits, and spaces
const screenshotTitlePunctuation = "-.!()"

// Screenshot is one saved screenshot of a game
type Screenshot struct {
	Path        string    // full path of the PNG file
	Time        time.Time // capture time
	Achievement string    // achievement title the capture was taken for, or "" for a manual capture
}

// ScreenshotFilename returns the name of a screenshot taken at ts: the
// millisecond Unix timestamp, "-n" when n is above zero, "_" and the tag
// when one is set, and the .png extension. ParseScreenshotName reads the
// name back.
func ScreenshotFilename(ts time.Time, n int, tag string) string {
	var b strings.Builder
	b.WriteString(strconv.FormatInt(ts.UnixMilli(), 10))
	if n > 0 {
		b.WriteByte('-')
		b.WriteString(strconv.Itoa(n))
	}
	if tag != "" {
		b.WriteByte('_')
		b.WriteString(tag)
	}
	b.WriteString(screenshotExt)
	return b.String()
}

// SanitizeScreenshotTitle reduces an achievement title to text allowed in
// a screenshot filename: letters, digits, marks, spaces, and the
// punctuation in screenshotTitlePunctuation. Everything else is dropped,
// runs of whitespace become one space, and the result is trimmed and
// capped at MaxScreenshotTitleLen runes and MaxScreenshotTitleBytes bytes.
// The allow-list keeps every shell, path, and command line metacharacter
// out of the name, and ParseScreenshotName accepts only titles it would
// produce.
func SanitizeScreenshotTitle(s string) string {
	var b strings.Builder
	pendingSpace := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			pendingSpace = true
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.IsMark(r),
			strings.ContainsRune(screenshotTitlePunctuation, r):
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			pendingSpace = false
			b.WriteRune(r)
		}
	}
	runes := []rune(b.String())
	if len(runes) > MaxScreenshotTitleLen {
		runes = runes[:MaxScreenshotTitleLen]
	}
	for len(string(runes)) > MaxScreenshotTitleBytes {
		runes = runes[:len(runes)-1]
	}
	return strings.TrimSpace(string(runes))
}

// ParseScreenshotName reads a screenshot file name written by
// ScreenshotFilename: the capture time and the achievement title, or ""
// for a manual capture. Names from before millisecond timestamps, with ten
// or fewer digits, are read as seconds. ok is false for any other name,
// including an achievement title that SanitizeScreenshotTitle would not
// produce, so a file planted with a crafted name is not a screenshot.
func ParseScreenshotName(name string) (ts time.Time, achievement string, ok bool) {
	base, found := strings.CutSuffix(name, screenshotExt)
	if !found {
		return time.Time{}, "", false
	}

	digits := leadingDigits(base)
	if digits == "" {
		return time.Time{}, "", false
	}
	rest := base[len(digits):]

	// Optional collision suffix
	if after, found := strings.CutPrefix(rest, "-"); found {
		n := leadingDigits(after)
		if n == "" {
			return time.Time{}, "", false
		}
		rest = after[len(n):]
	}

	if rest != "" {
		achievement, found = strings.CutPrefix(rest, "_"+ScreenshotAchievementTag)
		if !found || achievement == "" || SanitizeScreenshotTitle(achievement) != achievement {
			return time.Time{}, "", false
		}
	}

	v, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return time.Time{}, "", false
	}
	if len(digits) <= 10 {
		ts = time.Unix(v, 0)
	} else {
		ts = time.UnixMilli(v)
	}
	return ts, achievement, true
}

// leadingDigits returns the run of ASCII digits at the start of s
func leadingDigits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

// ListScreenshots returns the screenshots in dir, newest first. Entries
// that are not regular files named as screenshots are skipped, so a
// symlink or a planted file is never previewed, deleted, or revealed. A
// missing dir lists nothing.
func ListScreenshots(dir string) ([]Screenshot, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var shots []Screenshot
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		ts, achievement, ok := ParseScreenshotName(e.Name())
		if !ok {
			continue
		}
		shots = append(shots, Screenshot{
			Path:        filepath.Join(dir, e.Name()),
			Time:        ts,
			Achievement: achievement,
		})
	}

	// Captures with the same time sort by name length, then name, so a
	// collision suffix follows the name it was added to
	sort.Slice(shots, func(i, j int) bool {
		if !shots[i].Time.Equal(shots[j].Time) {
			return shots[i].Time.After(shots[j].Time)
		}
		ni, nj := filepath.Base(shots[i].Path), filepath.Base(shots[j].Path)
		if len(ni) != len(nj) {
			return len(ni) < len(nj)
		}
		return ni < nj
	})
	return shots, nil
}

// DeleteScreenshot removes the screenshot file at path. path must name a
// screenshot directly inside dir, the game's screenshot directory, so a
// bad path cannot remove anything else.
func DeleteScreenshot(dir, path string) error {
	if filepath.Dir(filepath.Clean(path)) != filepath.Clean(dir) {
		return fmt.Errorf("screenshot %q is not in %q", path, dir)
	}
	if _, _, ok := ParseScreenshotName(filepath.Base(path)); !ok {
		return fmt.Errorf("%q is not a screenshot", path)
	}
	return os.Remove(path)
}
