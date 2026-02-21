//go:build !libretro

package settings

import (
	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	emucore "github.com/user-none/eblitui/api"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
	"github.com/user-none/eblitui/standalone/types"
)

// buildCoreOptionRow creates a settings row for a core option.
// prefix is used for focus key namespacing (e.g. "audio", "video", "input").
func buildCoreOptionRow(focus types.FocusManager, callback types.ScreenCallback, config *storage.Config, opt emucore.CoreOption, prefix string) *widget.Container {
	row := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Surface)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{true, false}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(style.SmallSpacing)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	label := widget.NewText(
		widget.TextOpts.Text(opt.Label, style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		),
	)
	row.AddChild(label)

	focusKey := prefix + "-opt-" + opt.Key

	switch opt.Type {
	case emucore.CoreOptionBool:
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

	case emucore.CoreOptionSelect:
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

	case emucore.CoreOptionRange:
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

// getCoreOptionValue returns the current value for a core option,
// checking config overrides first, then falling back to the option's default.
func getCoreOptionValue(config *storage.Config, opt emucore.CoreOption) string {
	if config.Input.CoreOptions != nil {
		if v, ok := config.Input.CoreOptions[opt.Key]; ok {
			return v
		}
	}
	return opt.Default
}

// setCoreOptionValue saves a core option value to config.
func setCoreOptionValue(config *storage.Config, key, value string) {
	if config.Input.CoreOptions == nil {
		config.Input.CoreOptions = make(map[string]string)
	}
	config.Input.CoreOptions[key] = value
	storage.SaveConfig(config)
}
