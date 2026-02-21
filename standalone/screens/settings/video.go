//go:build !libretro

package settings

import (
	"github.com/ebitenui/ebitenui/widget"
	emucore "github.com/user-none/eblitui/api"
	"github.com/user-none/eblitui/standalone/shader"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
	"github.com/user-none/eblitui/standalone/types"
)

// VideoSection manages video settings including shaders
type VideoSection struct {
	callback   types.ScreenCallback
	config     *storage.Config
	systemInfo emucore.SystemInfo
}

// NewVideoSection creates a new video section
func NewVideoSection(callback types.ScreenCallback, config *storage.Config, systemInfo emucore.SystemInfo) *VideoSection {
	return &VideoSection{
		callback:   callback,
		config:     config,
		systemInfo: systemInfo,
	}
}

// SystemInfo returns the system info for navigation setup
func (v *VideoSection) SystemInfo() emucore.SystemInfo {
	return v.systemInfo
}

// SetConfig updates the config reference
func (v *VideoSection) SetConfig(config *storage.Config) {
	v.config = config
}

// Build creates the video section UI
func (v *VideoSection) Build(focus types.FocusManager) *widget.Container {
	outer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
		)),
	)

	// Shaders list in scrollable container (includes core options and header)
	outer.AddChild(v.buildShadersList(focus))

	// Set up navigation zones
	v.setupNavigation(focus)

	return outer
}

// setupNavigation registers navigation zones for the video section
func (v *VideoSection) setupNavigation(focus types.FocusManager) {
	// Core options zone
	coreOptKeys := make([]string, 0)
	for _, opt := range v.systemInfo.CoreOptions {
		if opt.Category == emucore.CoreOptionCategoryVideo {
			coreOptKeys = append(coreOptKeys, "video-opt-"+opt.Key)
		}
	}
	if len(coreOptKeys) > 0 {
		focus.RegisterNavZone("video-core-opts", types.NavZoneVertical, coreOptKeys, 0)
	}

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

	// Transitions from core options to first shader zone
	if len(coreOptKeys) > 0 {
		firstShaderZone := "video-shaders"
		if len(preprocessKeys) > 0 {
			firstShaderZone = "video-preprocess"
		}
		focus.SetNavTransition("video-core-opts", types.DirDown, firstShaderZone, types.NavIndexFirst)
		if len(preprocessKeys) > 0 {
			focus.SetNavTransition("video-preprocess", types.DirUp, "video-core-opts", types.NavIndexFirst)
		} else if len(shaderKeys) > 0 {
			focus.SetNavTransition("video-shaders", types.DirUp, "video-core-opts", types.NavIndexFirst)
		}
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
	hasVideoOptions := false
	for _, opt := range v.systemInfo.CoreOptions {
		if opt.Category == emucore.CoreOptionCategoryVideo {
			hasVideoOptions = true
			listContent.AddChild(buildCoreOptionRow(focus, v.callback, v.config, opt, "video"))
		}
	}

	if hasVideoOptions {
		listContent.AddChild(widget.NewText(
			widget.TextOpts.Text("", style.FontFace(), style.TextSecondary),
		))
	}

	// Shaders header
	shadersLabel := widget.NewText(
		widget.TextOpts.Text("Shader Effects", style.FontFace(), style.Accent),
	)
	listContent.AddChild(shadersLabel)

	for _, shaderInfo := range shader.AvailableShaders {
		listContent.AddChild(v.buildShaderRow(shaderInfo, focus))
	}

	// Wrap in scrollable container
	scrollContainer, vSlider, scrollWrapper := style.ScrollableContainer(style.ScrollableOpts{
		Content:     listContent,
		BgColor:     style.Background,
		BorderColor: style.Border,
		Spacing:     0,
		Padding:     style.SmallSpacing,
	})
	focus.SetScrollWidgets(scrollContainer, vSlider)
	focus.RestoreScrollPosition()

	return scrollWrapper
}

// maxShaderLabelWidth calculates the maximum pixel width for text labels in shader rows,
// based on the current window width and font-dependent sidebar size.
func (v *VideoSection) maxShaderLabelWidth() float64 {
	windowWidth := v.callback.GetWindowWidth()
	if windowWidth == 0 {
		windowWidth = 1100
	}

	// Estimate sidebar width: max of min size or measured widest label + padding
	sidebarWidth := style.SettingsSidebarMinWidth
	measuredSidebar := int(style.MeasureWidth("Achievements")) +
		style.SmallSpacing*2 + style.ButtonPaddingSmall*2
	if measuredSidebar > sidebarWidth {
		sidebarWidth = measuredSidebar
	}

	// Measure button column widths
	uiBtnW := int(style.MeasureWidth("UI")) + style.ButtonPaddingSmall*2
	gameBtnW := int(style.MeasureWidth("Game")) + style.ButtonPaddingSmall*2

	// Layout overhead: root padding + sidebar + main spacing + content area padding +
	// scroll wrapper padding + scrollbar + 2 grid spacings + UI button + Game button
	overhead := style.DefaultPadding*2 + sidebarWidth + style.DefaultSpacing +
		style.DefaultPadding*2 + style.SmallSpacing*2 + style.ScrollbarWidth +
		style.DefaultSpacing*2 + uiBtnW + gameBtnW

	available := windowWidth - overhead
	if available < 150 {
		available = 150
	}
	return float64(available)
}

// buildShaderRow creates a row for a single shader with UI and Game toggle buttons
func (v *VideoSection) buildShaderRow(info shader.ShaderInfo, focus types.FocusManager) *widget.Container {
	gameEnabled := v.isShaderEnabledForGame(info.ID)

	// Truncate text to prevent pushing buttons off-screen at large font sizes
	maxW := v.maxShaderLabelWidth()
	face := *style.FontFace()
	displayName, _ := style.TruncateToWidth(info.Name, face, maxW)
	displayDesc, _ := style.TruncateToWidth(info.Description, face, maxW)

	// Use grid layout: [Info (stretch)] [UI toggle] [Game toggle]
	row := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			widget.GridLayoutOpts.Stretch([]bool{true, false, false}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	// Info column
	infoContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.TinySpacing),
		)),
	)

	nameLabel := widget.NewText(
		widget.TextOpts.Text(displayName, style.FontFace(), style.Text),
	)
	infoContainer.AddChild(nameLabel)

	descLabel := widget.NewText(
		widget.TextOpts.Text(displayDesc, style.FontFace(), style.TextSecondary),
	)
	infoContainer.AddChild(descLabel)

	row.AddChild(infoContainer)

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
		// Empty placeholder to maintain 3-column grid layout
		placeholder := widget.NewContainer(
			widget.ContainerOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.GridLayoutData{
					VerticalPosition: widget.GridLayoutPositionCenter,
				}),
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

// boolToOnOff converts a boolean to "On" or "Off" string
func boolToOnOff(b bool) string {
	if b {
		return "On"
	}
	return "Off"
}
