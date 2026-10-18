// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"slices"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// buildCoreOptionRow creates a settings row for a core option.
// prefix is used for focus key namespacing (e.g. "audio", "video", "input").
func buildCoreOptionRow(focus types.FocusManager, callback types.ScreenCallback, config *storage.Config, opt coreif.CoreOption, prefix string) *widget.Container {
	row := widgets.SettingsRow(2)

	row.AddChild(widgets.LabeledText(opt.Label, opt.Description))

	focusKey := prefix + "-opt-" + opt.Key

	switch opt.Type {
	case coreif.CoreOptionBool:
		current := getCoreOptionValue(config, opt)
		isOn := current == "true" || current == "1" || current == "on"

		toggleBtn := widgets.OnOffButton(isOn, func(args *widget.ButtonClickedEventArgs) {
			if isOn {
				setCoreOptionValue(config, opt.Key, "false")
			} else {
				setCoreOptionValue(config, opt.Key, "true")
			}
			focus.SetPendingFocus(focusKey)
			callback.RequestRebuild()
		})
		focus.RegisterFocusButton(focusKey, toggleBtn)
		row.AddChild(toggleBtn)

	case coreif.CoreOptionSelect:
		current := getCoreOptionValue(config, opt)
		cycleBtn := widgets.CycleButton(current, slices.Index(opt.Values, current), len(opt.Values), style.Px(60), func(next int) {
			setCoreOptionValue(config, opt.Key, opt.Values[next])
			focus.SetPendingFocus(focusKey)
			callback.RequestRebuild()
		})
		focus.RegisterFocusButton(focusKey, cycleBtn)
		row.AddChild(cycleBtn)

	case coreif.CoreOptionRange:
		current := getCoreOptionValue(config, opt)
		displayBtn := widget.NewButton(
			widget.ButtonOpts.Image(style.ButtonImage()),
			widget.ButtonOpts.Text(current, style.FontFace(), style.ButtonTextColor()),
			widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.GridLayoutData{
					VerticalPosition: widget.GridLayoutPositionCenter,
				}),
				widget.WidgetOpts.MinSize(style.Px(50), 0),
			),
		)
		focus.RegisterFocusButton(focusKey, displayBtn)
		row.AddChild(displayBtn)
	}

	return row
}

// CategoryOptions returns the core options of the category in display
// order: the ungrouped options first, in declared order, then each
// group in order of its first declared option, with its options in
// declared order.
func CategoryOptions(opts []coreif.CoreOption, category coreif.CoreOptionCategory) []coreif.CoreOption {
	var out []coreif.CoreOption
	for _, o := range opts {
		if o.Category == category && o.Group == "" {
			out = append(out, o)
		}
	}
	seen := map[string]bool{}
	for _, o := range opts {
		if o.Category != category || o.Group == "" || seen[o.Group] {
			continue
		}
		seen[o.Group] = true
		for _, g := range opts {
			if g.Category == category && g.Group == o.Group {
				out = append(out, g)
			}
		}
	}
	return out
}

// coreOptionFocusKeys returns the focus keys of the category's options
// in display order, prefix-opt-key, matching the rows addCoreOptionRows
// adds so the navigation zone follows the rows.
func coreOptionFocusKeys(opts []coreif.CoreOption, category coreif.CoreOptionCategory, prefix string) []string {
	var keys []string
	for _, opt := range CategoryOptions(opts, category) {
		keys = append(keys, prefix+"-opt-"+opt.Key)
	}
	return keys
}

// addCoreOptionRows adds the category's option rows to container in
// display order, a section header before each group's rows.
func addCoreOptionRows(container *widget.Container, focus types.FocusManager, callback types.ScreenCallback, config *storage.Config, opts []coreif.CoreOption, category coreif.CoreOptionCategory, prefix string) {
	group := ""
	for _, opt := range CategoryOptions(opts, category) {
		if opt.Group != "" && opt.Group != group {
			container.AddChild(widgets.SectionHeader(opt.Group))
		}
		group = opt.Group
		container.AddChild(buildCoreOptionRow(focus, callback, config, opt, prefix))
	}
}

// getCoreOptionValue returns the current value for a core option,
// checking config overrides first, then falling back to the option's default.
func getCoreOptionValue(config *storage.Config, opt coreif.CoreOption) string {
	if config.CoreOptions != nil {
		if v, ok := config.CoreOptions[opt.Key]; ok {
			return v
		}
	}
	return opt.Default
}

// setCoreOptionValue saves a core option value to config.
func setCoreOptionValue(config *storage.Config, key, value string) {
	if config.CoreOptions == nil {
		config.CoreOptions = make(map[string]string)
	}
	config.CoreOptions[key] = value
	storage.SaveConfig(config)
}
