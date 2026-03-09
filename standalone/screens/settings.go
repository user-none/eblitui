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
		video:             settings.NewVideoSection(callback, config, systemInfo),
		audio:             settings.NewAudioSection(callback, config, systemInfo),
		rewind:            settings.NewRewindSection(callback, config, serializeSize),
		retroAchievements: settings.NewRetroAchievementsSection(callback, config, achievementMgr),
		input:             settings.NewInputSection(callback, config, systemInfo),
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
		if opt.Category == emucore.CoreOptionCategoryCore {
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
	s.RegisterNavZone("sidebar", types.NavZoneVertical, sidebarKeys, 0)

	if s.selectedSection >= 0 && s.selectedSection < len(s.sections) {
		s.sections[s.selectedSection].setupNav()
	}
}

func (s *SettingsScreen) setupVideoNav() {
	firstVideoZone := "video-shaders"
	for _, opt := range s.video.SystemInfo().CoreOptions {
		if opt.Category == emucore.CoreOptionCategoryVideo {
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
		if opt.Category == emucore.CoreOptionCategoryAudio {
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
	firstZone := "input-bindings"
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
	if s.selectedSection >= 0 && s.selectedSection < len(s.sections) {
		switch s.sections[s.selectedSection].focusKey {
		case "section-input":
			s.input.Update()
		case "section-achievements":
			s.retroAchievements.Update()
		}
	}
}

// IsInputCaptureActive returns true when the input section is waiting for a key/button press
func (s *SettingsScreen) IsInputCaptureActive() bool {
	return s.input != nil && s.input.IsCapturing()
}
