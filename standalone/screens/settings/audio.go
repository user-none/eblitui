//go:build !libretro

package settings

import (
	"fmt"
	"math"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
	"github.com/user-none/eblitui/standalone/types"
)

const (
	volumeMin  = 0.0
	volumeMax  = 2.0
	volumeStep = 0.1
)

// AudioSection manages audio settings
type AudioSection struct {
	callback types.ScreenCallback
	config   *storage.Config

	// Live-updated text widget (avoid rebuild on +/- to preserve focus)
	volumeValueText *widget.Text
}

// NewAudioSection creates a new audio section
func NewAudioSection(callback types.ScreenCallback, config *storage.Config) *AudioSection {
	return &AudioSection{
		callback: callback,
		config:   config,
	}
}

// SetConfig updates the config reference
func (a *AudioSection) SetConfig(config *storage.Config) {
	a.config = config
}

// Build creates the audio section UI
func (a *AudioSection) Build(focus types.FocusManager) *widget.Container {
	section := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.DefaultSpacing),
		)),
	)

	// Mute Game Audio toggle
	section.AddChild(a.buildMuteRow(focus))

	// Volume control row
	section.AddChild(a.buildVolumeRow(focus))

	// Fast-forward mute toggle
	section.AddChild(a.buildFastForwardMuteRow(focus))

	a.setupNavigation(focus)

	return section
}

// setupNavigation registers navigation zones for the audio section
func (a *AudioSection) setupNavigation(focus types.FocusManager) {
	focus.RegisterNavZone("audio-mute", types.NavZoneHorizontal, []string{"audio-mute"}, 0)
	focus.RegisterNavZone("audio-volume", types.NavZoneGrid, []string{"audio-vol-dec", "audio-vol-inc"}, 2)
	focus.RegisterNavZone("audio-ff-mute", types.NavZoneHorizontal, []string{"audio-ff-mute"}, 0)

	focus.SetNavTransition("audio-mute", types.DirDown, "audio-volume", types.NavIndexFirst)
	focus.SetNavTransition("audio-volume", types.DirUp, "audio-mute", types.NavIndexFirst)
	focus.SetNavTransition("audio-volume", types.DirDown, "audio-ff-mute", types.NavIndexFirst)
	focus.SetNavTransition("audio-ff-mute", types.DirUp, "audio-volume", types.NavIndexFirst)
}

// buildVolumeRow creates the volume control row with [-] value [+] buttons
func (a *AudioSection) buildVolumeRow(focus types.FocusManager) *widget.Container {
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

	labelText := widget.NewText(
		widget.TextOpts.Text("Volume", style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		),
	)
	row.AddChild(labelText)

	// Controls group: [-] value [+]
	controls := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
	)

	// Value display (created first so click handlers can reference it)
	a.volumeValueText = widget.NewText(
		widget.TextOpts.Text(a.volumeLabel(), style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(50), 0),
		),
	)

	// [-] button
	decBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ButtonImage()),
		widget.ButtonOpts.Text("-", style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			if a.config.Audio.Volume > volumeMin {
				a.config.Audio.Volume = math.Round((a.config.Audio.Volume-volumeStep)*10) / 10
				if a.config.Audio.Volume < volumeMin {
					a.config.Audio.Volume = volumeMin
				}
			}
			storage.SaveConfig(a.config)
			a.updateVolumeLabel()
		}),
	)
	focus.RegisterFocusButton("audio-vol-dec", decBtn)
	controls.AddChild(decBtn)

	controls.AddChild(a.volumeValueText)

	// [+] button
	incBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ButtonImage()),
		widget.ButtonOpts.Text("+", style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			if a.config.Audio.Volume < volumeMax {
				a.config.Audio.Volume = math.Round((a.config.Audio.Volume+volumeStep)*10) / 10
				if a.config.Audio.Volume > volumeMax {
					a.config.Audio.Volume = volumeMax
				}
			}
			storage.SaveConfig(a.config)
			a.updateVolumeLabel()
		}),
	)
	focus.RegisterFocusButton("audio-vol-inc", incBtn)
	controls.AddChild(incBtn)

	row.AddChild(controls)

	return row
}

// volumeLabel returns the current volume as a percentage string
func (a *AudioSection) volumeLabel() string {
	return fmt.Sprintf("%d%%", int(math.Round(a.config.Audio.Volume*100)))
}

// updateVolumeLabel updates the volume text label in-place
// without triggering a full UI rebuild, preserving keyboard/gamepad focus.
func (a *AudioSection) updateVolumeLabel() {
	if a.volumeValueText != nil {
		a.volumeValueText.Label = a.volumeLabel()
	}
}

// buildMuteRow creates the mute toggle row
func (a *AudioSection) buildMuteRow(focus types.FocusManager) *widget.Container {
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
		widget.TextOpts.Text("Mute Game Audio", style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		),
	)
	row.AddChild(label)

	toggleBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(a.config.Audio.Muted)),
		widget.ButtonOpts.Text(boolToOnOff(a.config.Audio.Muted), style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(50), 0),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			a.config.Audio.Muted = !a.config.Audio.Muted
			storage.SaveConfig(a.config)
			focus.SetPendingFocus("audio-mute")
			a.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton("audio-mute", toggleBtn)
	row.AddChild(toggleBtn)

	return row
}

// buildFastForwardMuteRow creates the fast-forward audio mute toggle row
func (a *AudioSection) buildFastForwardMuteRow(focus types.FocusManager) *widget.Container {
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
		widget.TextOpts.Text("Mute Fast-Forward Audio", style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		),
	)
	row.AddChild(label)

	toggleBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(a.config.Audio.FastForwardMute)),
		widget.ButtonOpts.Text(boolToOnOff(a.config.Audio.FastForwardMute), style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(50), 0),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			a.config.Audio.FastForwardMute = !a.config.Audio.FastForwardMute
			storage.SaveConfig(a.config)
			focus.SetPendingFocus("audio-ff-mute")
			a.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton("audio-ff-mute", toggleBtn)
	row.AddChild(toggleBtn)

	return row
}
