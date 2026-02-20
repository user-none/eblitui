//go:build !libretro

package settings

import (
	"fmt"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
	"github.com/user-none/eblitui/standalone/types"
)

const (
	rewindBufferMin  = 10
	rewindBufferMax  = 200
	rewindBufferStep = 10
	rewindFrameMin   = 1
	rewindFrameMax   = 10
	rewindFrameStep  = 1
)

// RewindSection manages rewind settings
type RewindSection struct {
	callback      types.ScreenCallback
	config        *storage.Config
	serializeSize int // Bytes per save state (from SystemInfo)

	// Live-updated text widgets (avoid rebuild on +/- to preserve focus)
	bufferValueText *widget.Text
	stepValueText   *widget.Text
	infoText        *widget.Text
}

// NewRewindSection creates a new rewind section.
// serializeSize is the bytes per save state from SystemInfo.SerializeSize.
func NewRewindSection(callback types.ScreenCallback, config *storage.Config, serializeSize int) *RewindSection {
	return &RewindSection{
		callback:      callback,
		config:        config,
		serializeSize: serializeSize,
	}
}

// SetConfig updates the config reference
func (r *RewindSection) SetConfig(config *storage.Config) {
	r.config = config
}

// Build creates the rewind section UI
func (r *RewindSection) Build(focus types.FocusManager) *widget.Container {
	// Outer container with grid layout so scrollable content can stretch
	outer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
		)),
	)

	// Content container
	section := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
	)

	// Enabled toggle
	section.AddChild(r.buildToggleRow(focus, "rewind-enable", "Enabled",
		r.config.Rewind.Enabled,
		func() {
			r.config.Rewind.Enabled = !r.config.Rewind.Enabled
		}))

	// Buffer Size row (only shown when enabled)
	if r.config.Rewind.Enabled {
		section.AddChild(r.buildValueRow(focus, "Buffer Size (MB)",
			"rewind-buf-dec", "rewind-buf-inc",
			&r.bufferValueText,
			r.config.Rewind.BufferSizeMB,
			func() {
				if r.config.Rewind.BufferSizeMB > rewindBufferMin {
					r.config.Rewind.BufferSizeMB -= rewindBufferStep
				}
			},
			func() {
				if r.config.Rewind.BufferSizeMB < rewindBufferMax {
					r.config.Rewind.BufferSizeMB += rewindBufferStep
				}
			}))

		// Frame Step row
		section.AddChild(r.buildValueRow(focus, "Frame Step",
			"rewind-step-dec", "rewind-step-inc",
			&r.stepValueText,
			r.config.Rewind.FrameStep,
			func() {
				if r.config.Rewind.FrameStep > rewindFrameMin {
					r.config.Rewind.FrameStep -= rewindFrameStep
				}
			},
			func() {
				if r.config.Rewind.FrameStep < rewindFrameMax {
					r.config.Rewind.FrameStep += rewindFrameStep
				}
			}))

		// Info text showing estimated rewind duration
		r.infoText = widget.NewText(
			widget.TextOpts.Text(r.estimateRewindDuration(), style.FontFace(), style.TextSecondary),
		)
		section.AddChild(r.infoText)
	}

	r.setupNavigation(focus)

	// Wrap in scrollable container
	scrollContainer, vSlider, scrollWrapper := style.ScrollableContainer(style.ScrollableOpts{
		Content:     section,
		BgColor:     style.Background,
		BorderColor: style.Border,
		Spacing:     0,
		Padding:     style.SmallSpacing,
	})
	focus.SetScrollWidgets(scrollContainer, vSlider)
	focus.RestoreScrollPosition()
	outer.AddChild(scrollWrapper)
	return outer
}

// setupNavigation registers navigation zones for the rewind section
func (r *RewindSection) setupNavigation(focus types.FocusManager) {
	focus.RegisterNavZone("rewind-enable", types.NavZoneHorizontal, []string{"rewind-enable"}, 0)

	if r.config.Rewind.Enabled {
		// Grid zone: 2 columns ([-] [+]), 2 rows (buffer, step)
		// Grid handles left/right within rows and up/down between rows with column preservation
		focus.RegisterNavZone("rewind-values", types.NavZoneGrid, []string{
			"rewind-buf-dec", "rewind-buf-inc",
			"rewind-step-dec", "rewind-step-inc",
		}, 2)

		focus.SetNavTransition("rewind-enable", types.DirDown, "rewind-values", types.NavIndexFirst)
		focus.SetNavTransition("rewind-values", types.DirUp, "rewind-enable", types.NavIndexFirst)
	}
}

// buildToggleRow creates an on/off toggle row
func (r *RewindSection) buildToggleRow(focus types.FocusManager, key, label string, value bool, toggle func()) *widget.Container {
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
		widget.TextOpts.Text(label, style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		),
	)
	row.AddChild(labelText)

	toggleBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(value)),
		widget.ButtonOpts.Text(boolToOnOff(value), style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(50), 0),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			toggle()
			storage.SaveConfig(r.config)
			focus.SetPendingFocus(key)
			r.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton(key, toggleBtn)
	row.AddChild(toggleBtn)

	return row
}

// buildValueRow creates a row with label, value display, and [-] [+] buttons.
// The valueRef stores a reference to the value text widget so it can be updated
// in-place without a full UI rebuild (which would lose keyboard/gamepad focus).
func (r *RewindSection) buildValueRow(focus types.FocusManager, label, decKey, incKey string, valueRef **widget.Text, value int, onDec, onInc func()) *widget.Container {
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
		widget.TextOpts.Text(label, style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		),
	)
	row.AddChild(labelText)

	// Controls group: [-] value [+] in a horizontal row with center alignment
	controls := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
	)

	// Value display (created first so click handlers can reference it)
	valueText := widget.NewText(
		widget.TextOpts.Text(fmt.Sprintf("%d", value), style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(40), 0),
		),
	)
	*valueRef = valueText

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
			onDec()
			storage.SaveConfig(r.config)
			r.updateValueLabels()
		}),
	)
	focus.RegisterFocusButton(decKey, decBtn)
	controls.AddChild(decBtn)

	controls.AddChild(valueText)

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
			onInc()
			storage.SaveConfig(r.config)
			r.updateValueLabels()
		}),
	)
	focus.RegisterFocusButton(incKey, incBtn)
	controls.AddChild(incBtn)

	row.AddChild(controls)

	return row
}

// updateValueLabels updates the value text and info text labels in-place
// without triggering a full UI rebuild, preserving keyboard/gamepad focus.
func (r *RewindSection) updateValueLabels() {
	if r.bufferValueText != nil {
		r.bufferValueText.Label = fmt.Sprintf("%d", r.config.Rewind.BufferSizeMB)
	}
	if r.stepValueText != nil {
		r.stepValueText.Label = fmt.Sprintf("%d", r.config.Rewind.FrameStep)
	}
	if r.infoText != nil {
		r.infoText.Label = r.estimateRewindDuration()
	}
}

// estimateRewindDuration calculates estimated rewind time based on settings
func (r *RewindSection) estimateRewindDuration() string {
	stateSize := r.serializeSize
	entries := (r.config.Rewind.BufferSizeMB * 1024 * 1024) / stateSize
	// At 60fps with frameStep, each entry covers frameStep frames
	totalFrames := entries * r.config.Rewind.FrameStep
	seconds := totalFrames / 60
	if seconds < 60 {
		return fmt.Sprintf("~%d seconds of rewind at 60fps", seconds)
	}
	minutes := seconds / 60
	remainSec := seconds % 60
	if remainSec > 0 {
		return fmt.Sprintf("~%dm %ds of rewind at 60fps", minutes, remainSec)
	}
	return fmt.Sprintf("~%d minutes of rewind at 60fps", minutes)
}
