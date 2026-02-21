//go:build !libretro

package screens

import (
	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	emucore "github.com/user-none/eblitui/api"
	"github.com/user-none/eblitui/standalone/achievements"
	"github.com/user-none/eblitui/standalone/screens/settings"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
	"github.com/user-none/eblitui/standalone/types"
)

// SettingsScreen displays application settings
type SettingsScreen struct {
	BaseScreen // Embedded for focus restoration

	callback        ScreenCallback
	selectedSection int

	// Encapsulated sections
	library           *settings.LibrarySection
	appearance        *settings.AppearanceSection
	video             *settings.VideoSection
	audio             *settings.AudioSection
	rewind            *settings.RewindSection
	retroAchievements *settings.RetroAchievementsSection
	input             *settings.InputSection
}

// NewSettingsScreen creates a new settings screen.
// serializeSize is the bytes per save state for rewind duration estimates.
// systemInfo provides button definitions and core options for the input section.
func NewSettingsScreen(callback ScreenCallback, library *storage.Library, config *storage.Config, achievementMgr *achievements.Manager, serializeSize int, systemInfo emucore.SystemInfo) *SettingsScreen {
	s := &SettingsScreen{
		callback:          callback,
		selectedSection:   0,
		library:           settings.NewLibrarySection(callback, library),
		appearance:        settings.NewAppearanceSection(callback, config),
		video:             settings.NewVideoSection(callback, config),
		audio:             settings.NewAudioSection(callback, config),
		rewind:            settings.NewRewindSection(callback, config, serializeSize),
		retroAchievements: settings.NewRetroAchievementsSection(callback, config, achievementMgr),
		input:             settings.NewInputSection(callback, config, systemInfo),
	}
	s.InitBase()
	return s
}

// HasPendingScan delegates to library section
func (s *SettingsScreen) HasPendingScan() bool {
	return s.library.HasPendingScan()
}

// ClearPendingScan delegates to library section
func (s *SettingsScreen) ClearPendingScan() {
	s.library.ClearPendingScan()
}

// SetLibrary updates the library reference in the library section
func (s *SettingsScreen) SetLibrary(library *storage.Library) {
	s.library.SetLibrary(library)
}

// SetConfig updates the config reference in all config-dependent sections
func (s *SettingsScreen) SetConfig(config *storage.Config) {
	s.appearance.SetConfig(config)
	s.video.SetConfig(config)
	s.audio.SetConfig(config)
	s.rewind.SetConfig(config)
	s.retroAchievements.SetConfig(config)
	s.input.SetConfig(config)
}

// SetAchievements updates the achievement manager reference
func (s *SettingsScreen) SetAchievements(mgr *achievements.Manager) {
	s.retroAchievements.SetAchievements(mgr)
}

// Build creates the settings screen UI
func (s *SettingsScreen) Build() *widget.Container {
	// Clear button references for fresh build
	s.ClearFocusButtons()

	// Use GridLayout for the root to properly constrain sizes
	rootContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Background)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			// Row 0 (header) = fixed, Row 1 (main content) = stretch
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true}),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(style.DefaultPadding)),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, style.DefaultSpacing),
		)),
	)

	// Header with back button and title
	header := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(style.DefaultSpacing),
		)),
	)

	backButton := style.TextButton("Back", style.ButtonPaddingSmall, func(args *widget.ButtonClickedEventArgs) {
		s.callback.SwitchToLibrary()
	})
	header.AddChild(backButton)

	rootContainer.AddChild(header)

	// Main content area with sidebar and content - use GridLayout for proper sizing
	mainContent := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			// Col 0 (sidebar) = fixed, Col 1 (content) = stretch
			// Row stretches vertically
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
		)),
	)

	// Sidebar
	sidebar := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Surface)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(style.SmallSpacing)),
			widget.RowLayoutOpts.Spacing(style.TinySpacing),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(style.SettingsSidebarMinWidth, 0),
		),
	)

	// Library section button
	libraryBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(s.selectedSection == 0)),
		widget.ButtonOpts.Text("Library", style.FontFace(), &widget.ButtonTextColor{
			Idle:     style.Text,
			Disabled: style.TextSecondary,
		}),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			s.selectedSection = 0
			s.SetPendingFocus("section-library")
			s.callback.RequestRebuild()
		}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	s.RegisterFocusButton("section-library", libraryBtn)
	sidebar.AddChild(libraryBtn)

	// Appearance section button
	appearanceBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(s.selectedSection == 1)),
		widget.ButtonOpts.Text("Appearance", style.FontFace(), &widget.ButtonTextColor{
			Idle:     style.Text,
			Disabled: style.TextSecondary,
		}),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			s.selectedSection = 1
			s.SetPendingFocus("section-appearance")
			s.callback.RequestRebuild()
		}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	s.RegisterFocusButton("section-appearance", appearanceBtn)
	sidebar.AddChild(appearanceBtn)

	// Video section button
	videoBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(s.selectedSection == 2)),
		widget.ButtonOpts.Text("Video", style.FontFace(), &widget.ButtonTextColor{
			Idle:     style.Text,
			Disabled: style.TextSecondary,
		}),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			s.selectedSection = 2
			s.SetPendingFocus("section-video")
			s.callback.RequestRebuild()
		}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	s.RegisterFocusButton("section-video", videoBtn)
	sidebar.AddChild(videoBtn)

	// Audio section button
	audioBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(s.selectedSection == 3)),
		widget.ButtonOpts.Text("Audio", style.FontFace(), &widget.ButtonTextColor{
			Idle:     style.Text,
			Disabled: style.TextSecondary,
		}),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			s.selectedSection = 3
			s.SetPendingFocus("section-audio")
			s.callback.RequestRebuild()
		}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	s.RegisterFocusButton("section-audio", audioBtn)
	sidebar.AddChild(audioBtn)

	// Rewind section button
	rewindBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(s.selectedSection == 4)),
		widget.ButtonOpts.Text("Rewind", style.FontFace(), &widget.ButtonTextColor{
			Idle:     style.Text,
			Disabled: style.TextSecondary,
		}),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			s.selectedSection = 4
			s.SetPendingFocus("section-rewind")
			s.callback.RequestRebuild()
		}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	s.RegisterFocusButton("section-rewind", rewindBtn)
	sidebar.AddChild(rewindBtn)

	// RetroAchievements section button
	raBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(s.selectedSection == 5)),
		widget.ButtonOpts.Text("Achievements", style.FontFace(), &widget.ButtonTextColor{
			Idle:     style.Text,
			Disabled: style.TextSecondary,
		}),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			s.selectedSection = 5
			s.SetPendingFocus("section-achievements")
			s.callback.RequestRebuild()
		}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	s.RegisterFocusButton("section-achievements", raBtn)
	sidebar.AddChild(raBtn)

	// Input section button
	inputBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(s.selectedSection == 6)),
		widget.ButtonOpts.Text("Input", style.FontFace(), &widget.ButtonTextColor{
			Idle:     style.Text,
			Disabled: style.TextSecondary,
		}),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			s.selectedSection = 6
			s.SetPendingFocus("section-input")
			s.callback.RequestRebuild()
		}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	s.RegisterFocusButton("section-input", inputBtn)
	sidebar.AddChild(inputBtn)

	mainContent.AddChild(sidebar)

	// Content area - use GridLayout to constrain the library section
	contentArea := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(style.DefaultPadding)),
		)),
	)

	// Section content - delegate to encapsulated sections
	switch s.selectedSection {
	case 0:
		contentArea.AddChild(s.library.Build(s))
	case 1:
		contentArea.AddChild(s.appearance.Build(s))
	case 2:
		contentArea.AddChild(s.video.Build(s))
	case 3:
		contentArea.AddChild(s.audio.Build(s))
	case 4:
		contentArea.AddChild(s.rewind.Build(s))
	case 5:
		contentArea.AddChild(s.retroAchievements.Build(s))
	case 6:
		contentArea.AddChild(s.input.Build(s))
	}

	mainContent.AddChild(contentArea)
	rootContainer.AddChild(mainContent)

	// Set up navigation zones
	s.setupNavigation()

	return rootContainer
}

// setupNavigation registers navigation zones for settings screen
func (s *SettingsScreen) setupNavigation() {
	// Sidebar zone (vertical)
	sidebarKeys := []string{"section-library", "section-appearance", "section-video", "section-audio", "section-rewind", "section-achievements", "section-input"}
	s.RegisterNavZone("sidebar", types.NavZoneVertical, sidebarKeys, 0)

	// Set up transitions from sidebar to content
	// The content zone names are set by the sections
	switch s.selectedSection {
	case 0: // Library
		s.SetNavTransition("sidebar", types.DirRight, "lib-folders", types.NavIndexFirst)
		s.SetNavTransition("lib-folders", types.DirLeft, "sidebar", types.NavIndexFirst)
		s.SetNavTransition("lib-buttons", types.DirLeft, "sidebar", types.NavIndexFirst)
	case 1: // Appearance - uses theme-list zone
		s.SetNavTransition("sidebar", types.DirRight, "theme-list", types.NavIndexFirst)
		s.SetNavTransition("theme-list", types.DirLeft, "sidebar", types.NavIndexFirst)
	case 2: // Video
		s.SetNavTransition("sidebar", types.DirRight, "video-shaders", types.NavIndexFirst)
		s.SetNavTransition("video-shaders", types.DirLeft, "sidebar", types.NavIndexFirst)
	case 3: // Audio
		s.SetNavTransition("sidebar", types.DirRight, "audio-mute", types.NavIndexFirst)
		s.SetNavTransition("audio-mute", types.DirLeft, "sidebar", types.NavIndexFirst)
		s.SetNavTransition("audio-volume", types.DirLeft, "sidebar", types.NavIndexFirst)
		s.SetNavTransition("audio-ff-mute", types.DirLeft, "sidebar", types.NavIndexFirst)
	case 4: // Rewind
		s.SetNavTransition("sidebar", types.DirRight, "rewind-enable", types.NavIndexFirst)
		s.SetNavTransition("rewind-enable", types.DirLeft, "sidebar", types.NavIndexFirst)
	case 5: // RetroAchievements
		s.SetNavTransition("sidebar", types.DirRight, "ra-settings", types.NavIndexFirst)
		s.SetNavTransition("ra-settings", types.DirLeft, "sidebar", types.NavIndexFirst)
	case 6: // Input
		firstZone := "input-bindings"
		// Use core options zone as first target if it has entries
		for _, opt := range s.input.SystemInfo().CoreOptions {
			if opt.Category == emucore.CoreOptionCategoryInput {
				firstZone = "input-core-opts"
				break
			}
		}
		s.SetNavTransition("sidebar", types.DirRight, firstZone, types.NavIndexFirst)
		s.SetNavTransition("input-core-opts", types.DirLeft, "sidebar", types.NavIndexFirst)
		s.SetNavTransition("input-bindings", types.DirLeft, "sidebar", types.NavIndexFirst)
		s.SetNavTransition("input-reset", types.DirLeft, "sidebar", types.NavIndexFirst)
	}
}

// OnEnter is called when entering the settings screen
func (s *SettingsScreen) OnEnter() {
	s.SetPendingFocus("section-library") // Always defaults to Library section when entering
}

// EnsureFocusedVisible scrolls the theme list to keep the focused widget visible
func (s *SettingsScreen) EnsureFocusedVisible(focused widget.Focuser) {
	// Use the base implementation - all theme buttons should trigger scrolling
	s.BaseScreen.EnsureFocusedVisible(focused, nil)
}

// Update handles per-frame updates for settings sections
func (s *SettingsScreen) Update() {
	switch s.selectedSection {
	case 5:
		s.retroAchievements.Update()
	case 6:
		s.input.Update()
	}
}

// IsInputCaptureActive returns true when the input section is waiting for a key/button press
func (s *SettingsScreen) IsInputCaptureActive() bool {
	return s.input != nil && s.input.IsCapturing()
}
