// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"bytes"
	"fmt"
	goimage "image"
	"image/color"
	_ "image/png"
	"time"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// aboutIconSize is the logical width/height the app icon is scaled to fit.
const aboutIconSize = 128

// AboutSection displays the application icon, name, and version, and the
// Updates area when the update system is on.
type AboutSection struct {
	callback types.ScreenCallback
	config   *storage.Config
	updates  types.Updates // nil when the update system is off
	name     string
	version  string
	iconPNG  []byte // PNG-encoded icon (may be nil)

	iconImg *ebiten.Image // decoded/scaled icon, cached after first Build
}

// NewAboutSection creates the About section. iconPNG may be nil, in which case
// only the name and version are shown. updates is nil when the update system
// is off, in which case no Updates area is shown.
func NewAboutSection(callback types.ScreenCallback, config *storage.Config, updates types.Updates, name, version string, iconPNG []byte) *AboutSection {
	return &AboutSection{
		callback: callback,
		config:   config,
		updates:  updates,
		name:     name,
		version:  version,
		iconPNG:  iconPNG,
	}
}

// SetConfig updates the config reference
func (a *AboutSection) SetConfig(config *storage.Config) {
	a.config = config
}

// HasUpdates reports whether the Updates area is shown
func (a *AboutSection) HasUpdates() bool {
	return a.updates != nil
}

// Build creates the About section UI
func (a *AboutSection) Build(focus types.FocusManager) *widget.Container {
	outer := widgets.FillContainer()

	section := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.DefaultSpacing),
		)),
	)

	if img := a.icon(); img != nil {
		section.AddChild(widget.NewGraphic(
			widget.GraphicOpts.Image(img),
			widget.GraphicOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{
					Position: widget.RowLayoutPositionCenter,
				}),
			),
		))
	}
	section.AddChild(centeredText(a.name, style.Text))
	section.AddChild(centeredText("Version "+a.version, style.TextSecondary))

	if a.updates != nil {
		a.buildUpdates(section, focus)
	}

	scrollContainer, _, scrollWrapper := widgets.ScrollableContainer(widgets.PageScrollOpts(section))
	focus.SetScrollContainer(scrollContainer)
	focus.RestoreScrollPosition()
	outer.AddChild(scrollWrapper)
	return outer
}

// centeredText creates a line of text centered in a vertical row layout
func centeredText(text string, textColor color.Color) *widget.Text {
	return widget.NewText(
		widget.TextOpts.Text(text, style.FontFace(), textColor),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
		),
	)
}

// buildUpdates adds the Updates area: the automatic check toggle, the
// status, and the Download and Check Now buttons where they apply
func (a *AboutSection) buildUpdates(section *widget.Container, focus types.FocusManager) {
	status := a.updates.Status()
	autoCheck := a.config.Updates.AutoCheck

	section.AddChild(widgets.SectionHeader("Updates"))

	row := widgets.SettingsRow(2)
	row.AddChild(widget.NewText(
		widget.TextOpts.Text("Check for updates automatically", style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	))
	toggle := widgets.OnOffButton(autoCheck, func(args *widget.ButtonClickedEventArgs) {
		a.config.Updates.AutoCheck = !a.config.Updates.AutoCheck
		storage.SaveConfig(a.config)
		focus.SetPendingFocus("about-auto-check")
		a.callback.RequestRebuild()
	})
	focus.RegisterFocusButton("about-auto-check", toggle)
	row.AddChild(toggle)
	section.AddChild(row)

	if line := updateStatusText(status); line != "" {
		section.AddChild(widget.NewText(
			widget.TextOpts.Text(line, style.FontFace(), style.Text),
		))
	}
	if !autoCheck {
		section.AddChild(widgets.NoticeText(lastCheckedText(status.LastCheck, time.Now())))
	}

	var keys []string
	buttons := widgets.ButtonRow()
	// No Download while a check runs: it may find a newer version
	if status.Available != "" && !status.Checking {
		download := widgets.TextButton("Download", style.ButtonPaddingSmall, func(args *widget.ButtonClickedEventArgs) {
			a.updates.Download()
		})
		focus.RegisterFocusButton("about-download", download)
		buttons.AddChild(download)
		keys = append(keys, "about-download")
	}
	if !autoCheck {
		checkNow := widgets.EnabledTextButton("Check Now", style.ButtonPaddingSmall, !status.Checking, func(args *widget.ButtonClickedEventArgs) {
			a.updates.CheckNow()
			focus.SetPendingFocus("about-check-now")
			a.callback.RequestRebuild()
		})
		focus.RegisterFocusButton("about-check-now", checkNow)
		buttons.AddChild(checkNow)
		keys = append(keys, "about-check-now")
	}
	if len(keys) > 0 {
		section.AddChild(buttons)
	}

	a.setupNavigation(focus, keys)
}

// setupNavigation registers the Updates area zones: the toggle, then the
// button row when it has buttons
func (a *AboutSection) setupNavigation(focus types.FocusManager, buttonKeys []string) {
	zones := []string{"about-auto-check"}
	focus.RegisterNavZone("about-auto-check", types.NavZoneHorizontal, []string{"about-auto-check"}, 0)
	if len(buttonKeys) > 0 {
		focus.RegisterNavZone("about-buttons", types.NavZoneHorizontal, buttonKeys, 0)
		zones = append(zones, "about-buttons")
	}
	types.ChainZones(focus, zones)
}

// NavZones returns the navigation zones of the Updates area
func (a *AboutSection) NavZones() []string {
	return []string{"about-auto-check", "about-buttons"}
}

// updateStatusText returns the update status line, or "" when there is
// nothing to show (no check has run and no update is known). An update found
// earlier stays shown when a later check fails.
func updateStatusText(status types.UpdateStatus) string {
	switch {
	case status.Checking:
		return "Checking for updates..."
	case status.Available != "":
		return "Update available: " + status.Available
	case status.Failed:
		return "Couldn't check for updates"
	case status.LastCheck != 0:
		return "Up to date"
	}
	return ""
}

// lastCheckedText returns when the last check ran, relative to now
func lastCheckedText(lastCheck int64, now time.Time) string {
	if lastCheck == 0 {
		return "Last checked: never"
	}
	ago := now.Sub(time.Unix(lastCheck, 0))
	switch {
	case ago < time.Minute:
		return "Last checked just now"
	case ago < time.Hour:
		return "Last checked " + plural(int(ago/time.Minute), "minute") + " ago"
	case ago < 24*time.Hour:
		return "Last checked " + plural(int(ago/time.Hour), "hour") + " ago"
	}
	return "Last checked " + plural(int(ago/(24*time.Hour)), "day") + " ago"
}

// plural returns n and unit, with unit made plural when n is not 1
func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// icon decodes and scales the icon once, caching the result. Returns nil when
// no icon is set or the bytes cannot be decoded.
func (a *AboutSection) icon() *ebiten.Image {
	if a.iconImg != nil {
		return a.iconImg
	}
	if len(a.iconPNG) == 0 {
		return nil
	}
	img, _, err := goimage.Decode(bytes.NewReader(a.iconPNG))
	if err != nil {
		return nil
	}
	a.iconImg = style.ScaleImage(img, style.Px(aboutIconSize), style.Px(aboutIconSize))
	return a.iconImg
}
