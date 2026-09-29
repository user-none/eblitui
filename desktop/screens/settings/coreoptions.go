// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
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

		toggleBtn := widget.NewButton(
			widget.ButtonOpts.Image(style.ActiveButtonImage(isOn)),
			widget.ButtonOpts.Text(boolToOnOff(isOn), style.FontFace(), style.ButtonTextColor()),
			widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.GridLayoutData{
					VerticalPosition: widget.GridLayoutPositionCenter,
				}),
				widget.WidgetOpts.MinSize(style.Px(50), 0),
			),
			widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
				if isOn {
					setCoreOptionValue(config, opt.Key, "false")
				} else {
					setCoreOptionValue(config, opt.Key, "true")
				}
				focus.SetPendingFocus(focusKey)
				callback.RequestRebuild()
			}),
		)
		focus.RegisterFocusButton(focusKey, toggleBtn)
		row.AddChild(toggleBtn)

	case coreif.CoreOptionSelect:
		current := getCoreOptionValue(config, opt)
		nextIdx := 0
		for i, v := range opt.Values {
			if v == current {
				nextIdx = (i + 1) % len(opt.Values)
				break
			}
		}

		cycleBtn := widget.NewButton(
			widget.ButtonOpts.Image(style.ButtonImage()),
			widget.ButtonOpts.Text(current, style.FontFace(), style.ButtonTextColor()),
			widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.GridLayoutData{
					VerticalPosition: widget.GridLayoutPositionCenter,
				}),
				widget.WidgetOpts.MinSize(style.Px(60), 0),
			),
			widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
				setCoreOptionValue(config, opt.Key, opt.Values[nextIdx])
				focus.SetPendingFocus(focusKey)
				callback.RequestRebuild()
			}),
		)
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

// categoryOptions returns the core options of the category in display
// order: the ungrouped options first, in declared order, then each
// group in order of its first declared option, with its options in
// declared order.
func categoryOptions(opts []coreif.CoreOption, category coreif.CoreOptionCategory) []coreif.CoreOption {
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
	for _, opt := range categoryOptions(opts, category) {
		keys = append(keys, prefix+"-opt-"+opt.Key)
	}
	return keys
}

// addCoreOptionRows adds the category's option rows to container in
// display order, a section header before each group's rows, and
// reports whether any rows were added.
func addCoreOptionRows(container *widget.Container, focus types.FocusManager, callback types.ScreenCallback, config *storage.Config, opts []coreif.CoreOption, category coreif.CoreOptionCategory, prefix string) bool {
	ordered := categoryOptions(opts, category)
	group := ""
	for _, opt := range ordered {
		if opt.Group != "" && opt.Group != group {
			container.AddChild(buildSectionHeader(opt.Group))
		}
		group = opt.Group
		container.AddChild(buildCoreOptionRow(focus, callback, config, opt, prefix))
	}
	return len(ordered) > 0
}

// buildSectionHeader creates a section header label with accent color.
func buildSectionHeader(title string) *widget.Container {
	container := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.TinySpacing),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	label := widget.NewText(
		widget.TextOpts.Text(title, style.FontFace(), style.Accent),
	)
	container.AddChild(label)
	return container
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
