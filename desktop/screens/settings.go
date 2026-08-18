// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/achievements"
	"github.com/user-none/eblitui/desktop/screens/settings"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// sectionDescriptor describes a settings sidebar section
type sectionDescriptor struct {
	label    string
	focusKey string
	build    func(types.FocusManager) *widget.Container
	setupNav func()
}

// SettingsScreen displays application settings
type SettingsScreen struct {
	BaseScreen // Embedded for focus restoration

	callback        ScreenCallback
	selectedSection int
	sections        []sectionDescriptor

	// Encapsulated sections
	library           *settings.LibrarySection
	appearance        *settings.AppearanceSection
	video             *settings.VideoSection
	audio             *settings.AudioSection
	rewind            *settings.RewindSection
	retroAchievements *settings.RetroAchievementsSection
	input             *settings.InputSection
	coreOptions       *settings.CoreSection
	about             *settings.AboutSection
}

// NewSettingsScreen creates a new settings screen.
// serializeSize is the bytes per save state for rewind duration estimates.
// systemInfo provides button definitions and core options for the input section.
func NewSettingsScreen(callback ScreenCallback, library *storage.Library, config *storage.Config, achievementMgr *achievements.Manager, serializeSize int, systemInfo coreif.SystemInfo, appIcon []byte) *SettingsScreen {
	s := &SettingsScreen{
		callback:          callback,
		selectedSection:   0,
		library:           settings.NewLibrarySection(callback, library),
		appearance:        settings.NewAppearanceSection(callback, config),
		video:             settings.NewVideoSection(callback, config, systemInfo),
		audio:             settings.NewAudioSection(callback, config, systemInfo),
		rewind:            settings.NewRewindSection(callback, config, serializeSize),
		retroAchievements: settings.NewRetroAchievementsSection(callback, config, achievementMgr),
		input:             settings.NewInputSection(callback, config, systemInfo),
		about:             settings.NewAboutSection(systemInfo.CoreName, systemInfo.CoreVersion, appIcon),
	}
	s.InitBase()

	s.sections = []sectionDescriptor{
		{label: "Video", focusKey: "section-video", build: s.video.Build, setupNav: s.setupVideoNav},
		{label: "Audio", focusKey: "section-audio", build: s.audio.Build, setupNav: s.setupAudioNav},
		{label: "Input", focusKey: "section-input", build: s.input.Build, setupNav: s.setupInputNav},
		{label: "Library", focusKey: "section-library", build: s.library.Build, setupNav: s.setupLibraryNav},
		{label: "Appearance", focusKey: "section-appearance", build: s.appearance.Build, setupNav: s.setupAppearanceNav},
		{label: "Rewind", focusKey: "section-rewind", build: s.rewind.Build, setupNav: s.setupRewindNav},
		{label: "Achievements", focusKey: "section-achievements", build: s.retroAchievements.Build, setupNav: s.setupAchievementsNav},
	}

	hasCoreOpts := false
	for _, opt := range systemInfo.CoreOptions {
		if opt.Category == coreif.CoreOptionCategoryCore {
			hasCoreOpts = true
			break
		}
	}
	if hasCoreOpts || len(systemInfo.BIOSOptions) > 0 {
		s.coreOptions = settings.NewCoreSection(callback, config, systemInfo)
		s.sections = append(s.sections, sectionDescriptor{
			label:    "Core Options",
			focusKey: "section-core",
			build:    s.coreOptions.Build,
			setupNav: s.setupCoreNav,
		})
	}

	// About is always the last section.
	s.sections = append(s.sections, sectionDescriptor{
		label:    "About",
		focusKey: "section-about",
		build:    s.about.Build,
		setupNav: s.setupAboutNav,
	})

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
	if s.coreOptions != nil {
		s.coreOptions.SetConfig(config)
	}
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

	backButton := widgets.TextButton("Back", style.ButtonPaddingSmall, func(args *widget.ButtonClickedEventArgs) {
		s.callback.SwitchToLibrary()
	})
	s.RegisterFocusButton("settings-back", backButton)
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

	for i, sec := range s.sections {
		idx := i
		key := sec.focusKey
		btn := widget.NewButton(
			widget.ButtonOpts.Image(style.ActiveButtonImage(s.selectedSection == idx)),
			widget.ButtonOpts.Text(sec.label, style.FontFace(), &widget.ButtonTextColor{
				Idle:     style.Text,
				Disabled: style.TextSecondary,
			}),
			widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
			widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
				s.selectedSection = idx
				s.SetPendingFocus(key)
				s.callback.RequestRebuild()
			}),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
		)
		s.RegisterFocusButton(key, btn)
		sidebar.AddChild(btn)
	}

	mainContent.AddChild(sidebar)

	// Content area - use GridLayout to constrain the library section
	contentArea := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(style.DefaultPadding)),
		)),
	)

	if s.selectedSection >= 0 && s.selectedSection < len(s.sections) {
		contentArea.AddChild(s.sections[s.selectedSection].build(s))
	}

	mainContent.AddChild(contentArea)
	rootContainer.AddChild(mainContent)

	// Set up navigation zones
	s.setupNavigation()

	return rootContainer
}

// setupNavigation registers navigation zones for settings screen
func (s *SettingsScreen) setupNavigation() {
	sidebarKeys := make([]string, len(s.sections))
	for i, sec := range s.sections {
		sidebarKeys[i] = sec.focusKey
	}

	// Header zone holds the Back button above the sidebar/content. Up from the
	// top of the sidebar reaches it; Down from it returns to the sidebar.
	s.RegisterNavZone("header", types.NavZoneHorizontal, []string{"settings-back"}, 0)
	s.RegisterNavZone("sidebar", types.NavZoneVertical, sidebarKeys, 0)
	s.SetNavTransition("header", types.DirDown, "sidebar", types.NavIndexFirst)
	s.SetNavTransition("sidebar", types.DirUp, "header", types.NavIndexFirst)

	if s.selectedSection >= 0 && s.selectedSection < len(s.sections) {
		s.sections[s.selectedSection].setupNav()
	}
}

func (s *SettingsScreen) setupVideoNav() {
	firstVideoZone := "video-shaders"
	for _, opt := range s.video.SystemInfo().CoreOptions {
		if opt.Category == coreif.CoreOptionCategoryVideo {
			firstVideoZone = "video-core-opts"
			break
		}
	}
	s.SetNavTransition("sidebar", types.DirRight, firstVideoZone, types.NavIndexFirst)
	s.SetNavTransition("video-core-opts", types.DirLeft, "sidebar", types.NavIndexFirst)
	s.SetNavTransition("video-preprocess", types.DirLeft, "sidebar", types.NavIndexFirst)
	s.SetNavTransition("video-shaders", types.DirLeft, "sidebar", types.NavIndexFirst)
}

func (s *SettingsScreen) setupAudioNav() {
	firstAudioZone := "audio-mute"
	for _, opt := range s.audio.SystemInfo().CoreOptions {
		if opt.Category == coreif.CoreOptionCategoryAudio {
			firstAudioZone = "audio-core-opts"
			break
		}
	}
	s.SetNavTransition("sidebar", types.DirRight, firstAudioZone, types.NavIndexFirst)
	s.SetNavTransition("audio-core-opts", types.DirLeft, "sidebar", types.NavIndexFirst)
	s.SetNavTransition("audio-mute", types.DirLeft, "sidebar", types.NavIndexFirst)
	s.SetNavTransition("audio-volume", types.DirLeft, "sidebar", types.NavIndexFirst)
	s.SetNavTransition("audio-ff-mute", types.DirLeft, "sidebar", types.NavIndexFirst)
}

func (s *SettingsScreen) setupInputNav() {
	s.SetNavTransition("sidebar", types.DirRight, s.input.FirstNavZone(), types.NavIndexFirst)
	for _, zone := range s.input.NavZones() {
		s.SetNavTransition(zone, types.DirLeft, "sidebar", types.NavIndexFirst)
	}
}

func (s *SettingsScreen) setupLibraryNav() {
	s.SetNavTransition("sidebar", types.DirRight, "lib-folders", types.NavIndexFirst)
	s.SetNavTransition("lib-folders", types.DirLeft, "sidebar", types.NavIndexFirst)
	s.SetNavTransition("lib-buttons", types.DirLeft, "sidebar", types.NavIndexFirst)
}

func (s *SettingsScreen) setupAppearanceNav() {
	s.SetNavTransition("sidebar", types.DirRight, "theme-list", types.NavIndexFirst)
	s.SetNavTransition("theme-list", types.DirLeft, "sidebar", types.NavIndexFirst)
}

func (s *SettingsScreen) setupRewindNav() {
	s.SetNavTransition("sidebar", types.DirRight, "rewind-enable", types.NavIndexFirst)
	s.SetNavTransition("rewind-enable", types.DirLeft, "sidebar", types.NavIndexFirst)
}

func (s *SettingsScreen) setupAchievementsNav() {
	s.SetNavTransition("sidebar", types.DirRight, "ra-settings", types.NavIndexFirst)
	s.SetNavTransition("ra-settings", types.DirLeft, "sidebar", types.NavIndexFirst)
}

// setupAboutNav is a no-op: the About section has no focusable widgets, so
// there is no sidebar-to-content navigation transition to register.
func (s *SettingsScreen) setupAboutNav() {}

func (s *SettingsScreen) setupCoreNav() {
	firstZone := "core-core-opts"
	if s.coreOptions != nil && !s.coreOptions.HasCoreOpts() && s.coreOptions.HasBIOS() {
		firstZone = "core-bios"
	}
	s.SetNavTransition("sidebar", types.DirRight, firstZone, types.NavIndexFirst)
	s.SetNavTransition("core-core-opts", types.DirLeft, "sidebar", types.NavIndexFirst)
	s.SetNavTransition("core-bios", types.DirLeft, "sidebar", types.NavIndexFirst)
}

// OnEnter is called when entering the settings screen
func (s *SettingsScreen) OnEnter() {
	if len(s.sections) > 0 {
		s.SetPendingFocus(s.sections[0].focusKey)
	}
}

// EnsureFocusedVisible scrolls the theme list to keep the focused widget visible
func (s *SettingsScreen) EnsureFocusedVisible(focused widget.Focuser) {
	// Use the base implementation - all theme buttons should trigger scrolling
	s.BaseScreen.EnsureFocusedVisible(focused, nil)
}

// Update handles per-frame updates for settings sections
func (s *SettingsScreen) Update() {
	// Input capture must keep updating even if the visible section changed
	// while capturing (e.g. a mouse click on the sidebar), so ESC can cancel.
	inputUpdated := false
	if s.input != nil && s.input.IsCapturing() {
		s.input.Update()
		inputUpdated = true
	}

	if s.selectedSection >= 0 && s.selectedSection < len(s.sections) {
		switch s.sections[s.selectedSection].focusKey {
		case "section-input":
			if !inputUpdated {
				s.input.Update()
			}
		case "section-achievements":
			s.retroAchievements.Update()
		}
	}
}

// IsInputCaptureActive returns true when the input section is waiting for a key/button press
func (s *SettingsScreen) IsInputCaptureActive() bool {
	return s.input != nil && s.input.IsCapturing()
}

// HandleBack gives the selected section a chance to consume a back action
// (e.g. leaving an input sub-view). Returns true when consumed.
func (s *SettingsScreen) HandleBack() bool {
	if s.selectedSection >= 0 && s.selectedSection < len(s.sections) &&
		s.sections[s.selectedSection].focusKey == "section-input" {
		return s.input.HandleBack()
	}
	return false
}
