// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/shader"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// VideoSection manages video settings including shaders
type VideoSection struct {
	callback     types.ScreenCallback
	config       *storage.Config
	systemInfo   coreif.SystemInfo
	contentWidth func() int
}

// NewVideoSection creates a new video section. contentWidth returns the
// width of the section content.
func NewVideoSection(callback types.ScreenCallback, config *storage.Config, systemInfo coreif.SystemInfo, contentWidth func() int) *VideoSection {
	return &VideoSection{
		callback:     callback,
		config:       config,
		systemInfo:   systemInfo,
		contentWidth: contentWidth,
	}
}

// SystemInfo returns the system info for navigation setup
func (v *VideoSection) SystemInfo() coreif.SystemInfo {
	return v.systemInfo
}

// SetConfig updates the config reference
func (v *VideoSection) SetConfig(config *storage.Config) {
	v.config = config
}

// Build creates the video section UI
func (v *VideoSection) Build(focus types.FocusManager) *widget.Container {
	outer := widgets.FillContainer()

	// Shaders list in scrollable container (includes core options and header)
	outer.AddChild(v.buildShadersList(focus))

	// Set up navigation zones
	v.setupNavigation(focus)

	return outer
}

// setupNavigation registers navigation zones for the video section
func (v *VideoSection) setupNavigation(focus types.FocusManager) {
	// Core options zone
	coreOptKeys := coreOptionFocusKeys(v.systemInfo.CoreOptions, coreif.CoreOptionCategoryVideo, "video")
	if len(coreOptKeys) > 0 {
		focus.RegisterNavZone("video-core-opts", types.NavZoneVertical, coreOptKeys, 0)
	}

	// Aspect ratio zone (single button)
	focus.RegisterNavZone("video-aspect", types.NavZoneVertical, []string{"video-aspect-ratio"}, 0)

	// Preprocessing effects: game-only, 1-column grid
	preprocessKeys := make([]string, 0)
	for _, info := range shader.AvailableShaders {
		if info.Preprocess {
			preprocessKeys = append(preprocessKeys, "shader-game-"+info.ID)
		}
	}
	if len(preprocessKeys) > 0 {
		focus.RegisterNavZone("video-preprocess", types.NavZoneGrid, preprocessKeys, 1)
	}

	// Regular shaders: UI+Game, 2-column grid
	shaderKeys := make([]string, 0)
	for _, info := range shader.AvailableShaders {
		if !info.Preprocess {
			shaderKeys = append(shaderKeys, "shader-ui-"+info.ID)
			shaderKeys = append(shaderKeys, "shader-game-"+info.ID)
		}
	}
	if len(shaderKeys) > 0 {
		focus.RegisterNavZone("video-shaders", types.NavZoneGrid, shaderKeys, 2)
	}

	// Navigation chain: core-opts <-> aspect <-> preprocess/shaders
	firstShaderZone := "video-shaders"
	if len(preprocessKeys) > 0 {
		firstShaderZone = "video-preprocess"
	}

	if len(coreOptKeys) > 0 {
		focus.SetNavTransition("video-core-opts", types.DirDown, "video-aspect", types.NavIndexFirst)
		focus.SetNavTransition("video-aspect", types.DirUp, "video-core-opts", types.NavIndexFirst)
	}

	if len(shaderKeys) > 0 || len(preprocessKeys) > 0 {
		focus.SetNavTransition("video-aspect", types.DirDown, firstShaderZone, types.NavIndexFirst)
		if len(preprocessKeys) > 0 {
			focus.SetNavTransition("video-preprocess", types.DirUp, "video-aspect", types.NavIndexFirst)
		} else {
			focus.SetNavTransition("video-shaders", types.DirUp, "video-aspect", types.NavIndexFirst)
		}
	}

	// Game-only effects sit above the UI+Game shaders, their Game buttons
	// in the shaders' Game column (index 1 of the first row)
	if len(preprocessKeys) > 0 && len(shaderKeys) > 0 {
		focus.SetNavTransition("video-preprocess", types.DirDown, "video-shaders", 1)
		focus.SetNavTransition("video-shaders", types.DirUp, "video-preprocess", types.NavIndexLast)
	}
}

// buildShadersList creates the scrollable shaders list with core options and header
func (v *VideoSection) buildShadersList(focus types.FocusManager) widget.PreferredSizeLocateableWidget {
	listContent := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
	)

	// Core options filtered by video category
	addCoreOptionRows(listContent, focus, v.callback, v.config, v.systemInfo.CoreOptions, coreif.CoreOptionCategoryVideo, "video")

	// Aspect ratio setting
	listContent.AddChild(v.buildAspectRatioRow(focus))

	// Shaders header
	listContent.AddChild(widgets.SectionHeader("Shader Effects"))

	for _, shaderInfo := range shader.AvailableShaders {
		listContent.AddChild(v.buildShaderRow(shaderInfo, focus))
	}

	// Wrap in scrollable container
	scrollContainer, _, scrollWrapper := widgets.ScrollableContainer(widgets.PageScrollOpts(listContent))
	focus.SetScrollContainer(scrollContainer)
	focus.RestoreScrollPosition()

	return scrollWrapper
}

// maxShaderLabelWidth returns the width of the name column of a shader
// row: the row less the UI and Game buttons
func (v *VideoSection) maxShaderLabelWidth() float64 {
	return float64(widgets.SettingsRowLabelWidth(widgets.PageRowWidth(v.contentWidth()),
		widgets.ButtonWidth("UI"), widgets.ButtonWidth("Game")))
}

// buildAspectRatioRow creates a cycle button row for the aspect ratio setting
func (v *VideoSection) buildAspectRatioRow(focus types.FocusManager) *widget.Container {
	current := v.config.Video.AspectRatio
	displayName := storage.AspectRatioDisplayName(current)

	nextIdx := 0
	for i, ar := range storage.ValidAspectRatios {
		if ar == current {
			nextIdx = (i + 1) % len(storage.ValidAspectRatios)
			break
		}
	}

	row := widgets.SettingsRow(2)

	label := widget.NewText(
		widget.TextOpts.Text("Aspect Ratio", style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	)
	row.AddChild(label)

	cycleBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ButtonImage()),
		widget.ButtonOpts.Text(displayName, style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(60), 0),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			v.config.Video.AspectRatio = storage.ValidAspectRatios[nextIdx]
			storage.SaveConfig(v.config)
			focus.SetPendingFocus("video-aspect-ratio")
			v.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton("video-aspect-ratio", cycleBtn)
	row.AddChild(cycleBtn)

	return row
}

// buildShaderRow creates a row for a single shader with UI and Game toggle buttons
func (v *VideoSection) buildShaderRow(info shader.ShaderInfo, focus types.FocusManager) *widget.Container {
	gameEnabled := v.isShaderEnabledForGame(info.ID)

	// Truncate text to prevent pushing buttons off-screen at large font sizes
	maxW := v.maxShaderLabelWidth()
	face := *style.FontFace()
	displayName, _ := style.TruncateToWidth(info.Name, face, maxW)
	displayDesc, _ := style.TruncateToWidth(info.Description, face, maxW)

	// Columns: [Info (stretch)] [UI toggle] [Game toggle]
	row := widgets.SettingsRow(3)

	// Info column
	row.AddChild(widgets.LabeledText(displayName, displayDesc))

	// UI toggle button (hidden for game-only preprocessing effects)
	supportsUI := info.Context&shader.ContextUI != 0
	if supportsUI {
		uiEnabled := v.isShaderEnabledForUI(info.ID)
		uiBtn := widget.NewButton(
			widget.ButtonOpts.Image(style.ActiveButtonImage(uiEnabled)),
			widget.ButtonOpts.Text("UI", style.FontFace(), style.ButtonTextColor()),
			widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.GridLayoutData{
					VerticalPosition: widget.GridLayoutPositionCenter,
				}),
			),
			widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
				v.toggleShaderUI(info.ID)
				storage.SaveConfig(v.config)
				focus.SetPendingFocus("shader-ui-" + info.ID)
				v.callback.RequestRebuild()
			}),
		)
		focus.RegisterFocusButton("shader-ui-"+info.ID, uiBtn)
		row.AddChild(uiBtn)
	} else {
		// Empty placeholder the width of the UI button, so every row has
		// the same columns
		placeholder := widget.NewContainer(
			widget.ContainerOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.GridLayoutData{
					VerticalPosition: widget.GridLayoutPositionCenter,
				}),
				widget.WidgetOpts.MinSize(widgets.ButtonWidth("UI"), 0),
			),
		)
		row.AddChild(placeholder)
	}

	// Game toggle button
	gameBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(gameEnabled)),
		widget.ButtonOpts.Text("Game", style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			v.toggleShaderGame(info.ID)
			storage.SaveConfig(v.config)
			focus.SetPendingFocus("shader-game-" + info.ID)
			v.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton("shader-game-"+info.ID, gameBtn)
	row.AddChild(gameBtn)

	return row
}

// isShaderEnabledForUI checks if a shader is enabled for UI context
func (v *VideoSection) isShaderEnabledForUI(id string) bool {
	for _, s := range v.config.Shaders.UIShaders {
		if s == id {
			return true
		}
	}
	return false
}

// isShaderEnabledForGame checks if a shader is enabled for Game context
func (v *VideoSection) isShaderEnabledForGame(id string) bool {
	for _, s := range v.config.Shaders.GameShaders {
		if s == id {
			return true
		}
	}
	return false
}

// toggleShaderUI adds or removes a shader from the UI list
func (v *VideoSection) toggleShaderUI(id string) {
	if v.isShaderEnabledForUI(id) {
		v.config.Shaders.UIShaders = removeFromSlice(v.config.Shaders.UIShaders, id)
	} else {
		v.config.Shaders.UIShaders = append(v.config.Shaders.UIShaders, id)
	}
}

// toggleShaderGame adds or removes a shader from the Game list
func (v *VideoSection) toggleShaderGame(id string) {
	if v.isShaderEnabledForGame(id) {
		v.config.Shaders.GameShaders = removeFromSlice(v.config.Shaders.GameShaders, id)
	} else {
		v.config.Shaders.GameShaders = append(v.config.Shaders.GameShaders, id)
	}
}

// removeFromSlice removes all occurrences of value from slice
func removeFromSlice(slice []string, value string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != value {
			result = append(result, s)
		}
	}
	return result
}
