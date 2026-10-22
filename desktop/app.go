// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/ebitenui/ebitenui"
	ebitenuiInput "github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/achievements"
	"github.com/user-none/eblitui/desktop/cheats"
	"github.com/user-none/eblitui/desktop/display"
	"github.com/user-none/eblitui/desktop/metadata"
	"github.com/user-none/eblitui/desktop/scanner"
	"github.com/user-none/eblitui/desktop/screens"
	"github.com/user-none/eblitui/desktop/shader"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
	"github.com/user-none/eblitui/romloader"
	rcheevos "github.com/user-none/go-rcheevos"
)

// thumbnailCacheClearThreshold is the library size above which the icon-view
// artwork cache is freed when a game launches. Small libraries keep their
// thumbnails cached (fast return, low memory); large libraries can hold
// hundreds of MB of textures, which is freed while playing and rebuilt on
// return to the icon view.
const thumbnailCacheClearThreshold = 75

// App is the main application struct that implements ebiten.Game
type App struct {
	ui *ebitenui.UI

	// Core factory and system info (set by Run)
	factory    coreif.CoreFactory
	systemInfo coreif.SystemInfo
	downloads  scanner.Downloads // Optional scan downloads the core supports

	// State management
	state         AppState
	previousState AppState

	// Data
	config  *storage.Config
	library *storage.Library

	// Screens
	libraryScreen  *screens.LibraryScreen
	detailScreen   *screens.DetailScreen
	settingsScreen *screens.SettingsScreen
	scanScreen     *screens.ScanProgressScreen
	errorScreen    *screens.ErrorScreen
	filePicker     *screens.FilePickerScreen

	// State to return to when the file picker closes
	pickerReturnState AppState

	// Metadata for RDB lookups
	metadata *metadata.MetadataManager

	// Gameplay (emulation, input, save states, pause menu)
	gameplay *GameplayManager

	// UI managers
	notification      *Notification
	saveStateManager  *SaveStateManager
	screenshotManager *ScreenshotManager
	searchOverlay     *SearchOverlay

	// Scan and clean managers
	scanManager  *ScanManager
	cleanManager *CleanManager

	// Update check manager, nil when the update system is off
	updateManager *UpdateManager

	// Error state
	errorFile        string
	errorPath        string
	configLoadFailed bool // True if config.json failed to load (don't overwrite on exit)
	libraryLoaded    bool // True once library.json loaded and validated (safe to save on exit)

	// SDL GUIDs already given a baseline mapping attempt this session
	padMappingTried map[string]bool

	// Window tracking for persistence and responsive layouts
	windowX, windowY   int
	windowWidth        int
	windowHeight       int
	lastWindowedWidth  int // Last non-fullscreen width (logical pixels)
	lastWindowedHeight int // Last non-fullscreen height (logical pixels)
	lastBuildWidth     int // Track width used for last UI build

	// Input manager for UI navigation
	inputManager *InputManager

	// Player-to-controller assignment (updated every frame)
	assignment *PlayerAssignment

	// Shader manager for visual effects
	shaderManager *shader.Manager
	shaderBuffer  *ebiten.Image // Intermediate buffer for shader rendering

	// Achievement manager for RetroAchievements integration
	achievementManager *achievements.Manager

	// Rebuild pending flag, set from goroutines and cleared on the Ebiten
	// thread each frame
	rebuildPending atomic.Bool

	// Guard to prevent input leaking from gameplay to UI screens.
	// When the pause menu triggers a screen transition, the activation input
	// (Enter/Space/mouse click) may still be held. ebitenui's fresh widget
	// state would interpret the held input as a new press, causing phantom
	// clicks on the library screen. The guard suppresses UI input processing
	// until all activation inputs are released.
	gameplayTransitionGuard bool

	// HiDPI: current device scale factor tracked across Layout calls
	currentDPIScale float64

	// Window scale for the current window size, computed in Layout and
	// applied once the size settles. See settleResize.
	targetWindowScale  float64
	windowScaleApplied bool

	// Resize settling: the window size last seen, when it changed to that,
	// and whether the UI has been rebuilt for it
	lastSeenWidth   int
	lastSeenHeight  int
	resizeChangedAt time.Time
	resizeSettled   bool

	// Fullscreen: track state so it can be saved on exit even if macOS
	// has already left native fullscreen by the time saveWindowState runs.
	lastFullscreenState bool
}

// discIdentifier returns the factory's DiscIdentifier if it implements one,
// or nil. Used so the scanner can resolve disc-based metadata by serial.
func discIdentifier(f coreif.CoreFactory) coreif.DiscIdentifier {
	d, _ := f.(coreif.DiscIdentifier)
	return d
}

// supportsMemory reports whether the core's emulator implements
// coreif.Memory. Memory is an emulator interface, so a temporary emulator
// is created to check it and closed without being started.
func supportsMemory(f coreif.CoreFactory) bool {
	emu := f.CreateEmulator()
	defer emu.Close()
	_, ok := emu.(coreif.Memory)
	return ok
}

// Run is the public entry point for the standalone UI. It initializes storage,
// configures the window, creates the app, and starts the Ebiten game loop.
func Run(factory coreif.CoreFactory) error {
	info := factory.SystemInfo()

	// oto allows a single context rate per process. Use the core's rate
	// so emulator audio plays at the correct pitch and the audio-demand
	// pacing stays in lockstep; synthesized UI sounds are generated at
	// this same rate. Must be set before any audio/oto use.
	if info.SampleRate > 0 {
		audioSampleRate = info.SampleRate
	}

	// Initialize storage with core-specific data directory
	storage.Init(info.DataDirName)

	// Configure window
	ebiten.SetWindowTitle(info.CoreName)
	applyWindowIcon()
	// Remove the macOS About menu item; the in-app About (settings) is the only
	// About. One-time call; on darwin the menu work is deferred until the menu
	// exists. No-op on other platforms.
	removeNativeAboutMenuItem()
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSizeLimits(style.MinWindowWidth, style.MinWindowHeight, -1, -1)

	app, err := newApp(factory, info)
	if err != nil {
		return err
	}

	// Restore window size from saved config (before RunGame to avoid resize flash)
	width, height, x, y, fullscreen := app.GetWindowConfig()
	width = max(width, style.MinWindowWidth)
	height = max(height, style.MinWindowHeight)
	ebiten.SetWindowSize(width, height)

	// Restore window position if previously saved
	if x != nil && y != nil {
		ebiten.SetWindowPosition(*x, *y)
	}

	// Restore fullscreen state
	if fullscreen {
		ebiten.SetFullscreen(true)
	}

	// Handle window close manually so background goroutines can be
	// stopped before RunGame returns (ebiten panics on NewImage after that).
	ebiten.SetWindowClosingHandled(true)

	if err := ebiten.RunGame(app); err != nil {
		return err
	}

	app.SaveAndClose()
	return nil
}

// newApp creates and initializes the application with the given core factory and system info.
func newApp(factory coreif.CoreFactory, info coreif.SystemInfo) (*App, error) {
	// Rumble and cheats both need memory access
	memory := supportsMemory(factory)
	app := &App{
		state:           StateLibrary,
		factory:         factory,
		systemInfo:      info,
		downloads:       scanner.Downloads{Rumble: memory, Cheats: memory},
		padMappingTried: make(map[string]bool),
	}

	if err := app.initStorage(); err != nil {
		return nil, err
	}
	app.initMetadata()
	app.initManagers()

	// A config or library file that fails to load or validate shows the
	// error screen; its handlers continue startup once the file is fixed.
	if !app.loadConfig() {
		return app, nil
	}
	app.applyConfig()

	if !app.loadLibrary() {
		return app, nil
	}
	app.finishStartup()

	return app, nil
}

// initStorage creates the data directories and any missing config or
// library file.
func (a *App) initStorage() error {
	if err := storage.EnsureDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}
	if err := storage.CreateConfigIfMissing(); err != nil {
		log.Printf("Warning: failed to create config: %v", err)
	}
	if err := storage.CreateLibraryIfMissing(); err != nil {
		log.Printf("Warning: failed to create library: %v", err)
	}
	return nil
}

// initMetadata creates the metadata manager, starts a background RDB
// download when the RDB is missing, and loads the RDB for achievement MD5
// lookups.
func (a *App) initMetadata() {
	a.metadata = metadata.NewMetadataManager(a.systemInfo.MetadataVariants)
	if !a.metadata.RDBExists() {
		go func() {
			if err := a.metadata.DownloadRDB(); err != nil {
				log.Printf("Background RDB download failed: %v", err)
			}
		}()
	}
	if err := a.metadata.LoadRDB(); err != nil {
		log.Printf("Failed to load RDB: %v", err)
	}
}

// initManagers creates the managers that do not depend on config or
// library.
func (a *App) initManagers() {
	a.notification = NewNotification()
	a.saveStateManager = NewSaveStateManager(a.notification)
	a.screenshotManager = NewScreenshotManager()
	a.inputManager = NewInputManager()
	a.assignment = NewPlayerAssignment(a.systemInfo.Players)
	a.shaderManager = shader.NewManager(a.systemInfo.PixelAspectRatio)
	a.searchOverlay = NewSearchOverlay(func(text string) {
		if a.state == StateLibrary {
			a.libraryScreen.SetSearchText(text)
			a.rebuildCurrentScreen()
		}
	})
}

// loadConfig loads and validates config.json. On failure it shows the
// error screen and returns false.
func (a *App) loadConfig() bool {
	configPath, _ := storage.GetConfigPath()
	config, err := storage.LoadConfig()
	if err != nil {
		a.config = storage.DefaultConfig()
		a.showStartupError("config.json", configPath, nil)
		return false
	}
	a.config = config

	if errs := storage.ValidateConfig(a.config, style.ThemeNames()); len(errs) > 0 {
		a.showStartupError("config.json", configPath, errs)
		return false
	}
	return true
}

// applyConfig applies a loaded, valid config: the achievement manager,
// theme, and saved controller mappings.
func (a *App) applyConfig() {
	a.configLoadFailed = false
	a.newAchievementManager()
	style.ApplyThemeByName(a.config.Theme)
	applySavedPadMappings(a.config.Input.PadMappings)
}

// newAchievementManager creates the achievement manager for the current
// config. A previous manager is destroyed so none keeps a replaced config.
func (a *App) newAchievementManager() {
	if a.achievementManager != nil {
		a.achievementManager.Destroy()
	}
	a.achievementManager = achievements.NewManager(a.notification, a.config, a.systemInfo.Name, Version, uint32(a.systemInfo.ConsoleID), a.systemInfo.BigEndianMemory, audioSampleRate)
}

// loadLibrary loads and validates library.json. On failure it shows the
// error screen and returns false.
func (a *App) loadLibrary() bool {
	libraryPath, _ := storage.GetLibraryPath()
	library, err := storage.LoadLibrary()
	if err != nil {
		a.showStartupError("library.json", libraryPath, nil)
		return false
	}
	a.library = library

	if errs := storage.ValidateLibrary(library); len(errs) > 0 {
		a.showStartupError("library.json", libraryPath, errs)
		return false
	}
	return true
}

// showStartupError shows the error screen for a config or library file
// that failed to load (details nil) or validate. Screens are built on the
// first error so the error screen can show before startup finishes.
func (a *App) showStartupError(file, path string, details []string) {
	a.state = StateError
	a.errorFile = file
	a.errorPath = path
	if file == "config.json" {
		a.configLoadFailed = true // Don't overwrite the file on exit
	}

	if a.errorScreen == nil {
		if a.achievementManager == nil {
			a.newAchievementManager()
		}
		if a.library == nil {
			a.library = storage.DefaultLibrary()
		}
		a.preloadConfiguredShaders()
		a.initScreens()
	}

	switch {
	case details == nil:
		a.errorScreen.SetError(file, path)
	case file == "config.json":
		a.errorScreen.SetValidationError(file, path, details, a.handleResetAndContinue)
	default:
		a.errorScreen.SetValidationError(file, path, details, a.handleLibraryResetAndContinue)
	}
	a.rebuildCurrentScreen()
}

// finishStartup completes startup once config and library are loaded and
// valid. It runs once, from newApp or from an error screen handler.
func (a *App) finishStartup() {
	a.libraryLoaded = true

	a.gameplay = NewGameplayManager(
		a.factory,
		a.systemInfo,
		a.inputManager,
		a.assignment,
		a.saveStateManager,
		a.screenshotManager,
		a.notification,
		a.library,
		a.config,
		a.achievementManager,
		a.metadata,
		a.cheatSystem(),
		func() { // onExitToLibrary
			a.shaderManager.ReleaseGameBuffers()
			a.SwitchToLibrary()
		},
		func() { a.Exit() }, // onExitApp
	)

	a.startAchievementAutoLogin()
	a.preloadConfiguredShaders()

	// A due automatic check starts before the library is first built, so the
	// update bar is not shown from the stored manifest while it runs
	a.updateManager = NewUpdateManager(updateConfig, a.config, a.RequestRebuild)
	if a.updateManager != nil {
		a.updateManager.StartAutomaticCheck()
	}

	// Window dimensions are set by main before RunGame, so they're already correct
	a.initScreens()
	a.scanManager = a.newScanManager()
	a.cleanManager = a.newCleanManager()

	a.state = StateLibrary
	a.libraryScreen.OnEnter()
	a.rebuildCurrentScreen()
}

// updates returns the update system for the screens, nil when it is off.
// A nil *UpdateManager must not be returned as a non-nil interface.
func (a *App) updates() types.Updates {
	if a.updateManager == nil {
		return nil
	}
	return a.updateManager
}

// startAchievementAutoLogin logs in to RetroAchievements in the background
// with the stored token, when achievements are enabled and a token exists.
func (a *App) startAchievementAutoLogin() {
	ra := a.config.RetroAchievements
	if !ra.Enabled || ra.Username == "" || ra.Token == "" {
		return
	}
	go func() {
		a.achievementManager.LoginWithToken(ra.Username, ra.Token,
			func(success bool, result int, err error) {
				if !success {
					log.Printf("RetroAchievements auto-login failed: %v", err)
					// Only clear token for credential errors, not transient failures
					if result == rcheevos.InvalidCredentials ||
						result == rcheevos.ExpiredToken ||
						result == rcheevos.AccessDenied {
						a.config.RetroAchievements.Token = ""
						storage.SaveConfig(a.config)
					}
				}
			},
		)
	}()
}

// newScanManager creates the scan manager. It needs the scan screen, so it
// is created after initScreens.
func (a *App) newScanManager() *ScanManager {
	return NewScanManager(
		a.library,
		a.scanScreen,
		contentExtensions(a.systemInfo),
		a.metadata,
		a.systemInfo.ConsoleID,
		a.systemInfo.Disc,
		discIdentifier(a.factory),
		a.downloads,
		func() { a.rebuildCurrentScreen() }, // onProgress
		func(msg string) { // onComplete
			a.libraryScreen.ClearArtworkCache()
			a.state = StateSettings
			a.rebuildCurrentScreen()
			if msg != "" {
				a.notification.ShowDefault(msg)
			}
		},
	)
}

// newCleanManager creates the clean manager. It shares the scan screen, so
// it is created after initScreens.
func (a *App) newCleanManager() *CleanManager {
	return NewCleanManager(
		a.library,
		a.scanScreen,
		func() { a.rebuildCurrentScreen() }, // onProgress
		func(msg string) { // onComplete
			a.state = StateSettings
			a.rebuildCurrentScreen()
			if msg != "" {
				a.notification.ShowDefault(msg)
			}
		},
	)
}

// GetWindowConfig returns the saved window dimensions, position, and fullscreen state from config.
// This should be called before RunGame to set the initial window size.
func (a *App) GetWindowConfig() (width, height int, x, y *int, fullscreen bool) {
	return a.config.Window.Width, a.config.Window.Height, a.config.Window.X, a.config.Window.Y, a.config.Window.Fullscreen
}

// saveWindowState saves current window position and size to config
func (a *App) saveWindowState() {
	// Don't overwrite config if it failed to load (user may want to fix it manually)
	if a.configLoadFailed {
		return
	}

	// Don't save if we never got valid windowed dimensions.
	// lastWindowedWidth/Height are only set when not in fullscreen, so if the
	// app was fullscreen for its entire lifetime they remain 0.
	if a.lastWindowedWidth == 0 || a.lastWindowedHeight == 0 {
		return
	}

	// Use lastFullscreenState instead of IsFullscreen() because macOS exits
	// native fullscreen before this handler runs on Cmd+Q.
	// Use lastWindowedWidth/Height (not windowWidth/Height) so that quitting
	// in fullscreen saves the windowed size, not the fullscreen resolution.
	// ebiten.WindowSize() cannot be used here because it returns 0,0 during
	// window close on some platforms.
	a.config.Window.Width = a.lastWindowedWidth
	a.config.Window.Height = a.lastWindowedHeight
	a.config.Window.X = &a.windowX
	a.config.Window.Y = &a.windowY
	a.config.Window.Fullscreen = a.lastFullscreenState

	// Save to disk
	storage.SaveConfig(a.config)
}

// toggleFullscreen toggles between fullscreen and windowed mode
func (a *App) toggleFullscreen() {
	ebiten.SetFullscreen(!ebiten.IsFullscreen())
	a.lastFullscreenState = ebiten.IsFullscreen()
	a.config.Window.Fullscreen = a.lastFullscreenState
	storage.SaveConfig(a.config)
}

// initScreens creates all screen instances
func (a *App) initScreens() {
	a.libraryScreen = screens.NewLibraryScreen(a, a.library, a.config, a.updates())
	a.detailScreen = screens.NewDetailScreen(a, a.library, a.achievementManager, a.systemInfo.ConsoleID, a.cheatSystem(), a.systemInfo.BigEndianMemory, a.systemInfo.CoreOptions)
	a.settingsScreen = screens.NewSettingsScreen(a, a.library, a.config, a.achievementManager, a.systemInfo.SerializeSize, a.systemInfo, appIcon, a.updates())
	a.scanScreen = screens.NewScanProgressScreen()
	a.errorScreen = screens.NewErrorScreen(a, a.errorFile, a.errorPath, a.handleDeleteAndContinue)
	a.filePicker = screens.NewFilePickerScreen(a, a.closeFilePicker)
}

// rebuildCurrentScreen rebuilds the UI for the current state
func (a *App) rebuildCurrentScreen() {
	var container *widget.Container

	switch a.state {
	case StateLibrary:
		// Save scroll position and focused widget before rebuilding so
		// async rebuilds (e.g., an update check finishing) restore both
		a.libraryScreen.SaveScrollPosition()
		if a.ui != nil {
			a.libraryScreen.SaveFocusState(a.ui.GetFocusedWidget())
		}
		a.libraryScreen.SetLibrary(a.library)
		a.libraryScreen.SetConfig(a.config)
		container = a.libraryScreen.Build()
	case StateDetail:
		// Save scroll position and focused button before rebuild so async
		// rebuilds (e.g., achievement loading) restore both.
		a.detailScreen.SaveScrollPosition()
		if a.ui != nil {
			a.detailScreen.SaveFocusState(a.ui.GetFocusedWidget())
		}
		container = a.detailScreen.Build()
	case StateSettings:
		// Save scroll position and focused button before rebuilding
		a.settingsScreen.SaveScrollPosition()
		if a.ui != nil {
			a.settingsScreen.SaveFocusState(a.ui.GetFocusedWidget())
		}
		container = a.settingsScreen.Build()
	case StateScanProgress, StateCleanProgress:
		container = a.scanScreen.Build()
	case StateError:
		container = a.errorScreen.Build()
	case StateFilePicker:
		a.filePicker.SaveScrollPosition()
		if a.ui != nil {
			a.filePicker.SaveFocusState(a.ui.GetFocusedWidget())
		}
		container = a.filePicker.Build()
	default:
		// For StatePlaying, no UI container needed
		return
	}

	a.ui = &ebitenui.UI{Container: container}
	a.lastBuildWidth = a.windowWidth // Track width for responsive rebuild detection
}

// mouseClickInProgress reports whether the left mouse button is held or
// was released this frame. A button clicks on release, and only when it
// also saw the press, so a rebuild in between replaces it and loses the
// click. ebitenui takes focus from the focused widget on the press, which
// is one way a rebuild gets requested mid-click.
func mouseClickInProgress() bool {
	return ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) ||
		inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)
}

// Update implements ebiten.Game
func (a *App) Update() error {
	// Handle manual window close: halt background goroutines while still
	// inside RunGame so ebiten image APIs cannot be called after exit.
	if ebiten.IsWindowBeingClosed() {
		// Track window position and fullscreen state for save on exit.
		// Layout() handles width/height, but position must be queried here.
		// Fullscreen is tracked because macOS exits native fullscreen before
		// the save handler runs on Cmd+Q, so we can't rely on IsFullscreen() at exit.
		a.windowX, a.windowY = ebiten.WindowPosition()
		a.lastFullscreenState = ebiten.IsFullscreen()

		a.libraryScreen.HaltArtworkLoader()
		a.screenshotManager.Close()
		return ebiten.Termination
	}

	// Process any pending rebuild request (set from goroutines). Swap
	// reads and clears the flag in one step so a request made in between
	// is not lost. A rebuild waits for a mouse click to finish.
	if !mouseClickInProgress() && a.rebuildPending.Swap(false) {
		a.rebuildCurrentScreen()

		// Restore focus immediately after rebuild, before processUIInput
		// runs. This ensures navigation sees the correct focused widget
		// instead of falling back to ChangeFocus (which would pick the
		// first focusable widget, typically the back button).
		if a.ui != nil {
			switch a.state {
			case StateSettings:
				a.restorePendingFocus(a.settingsScreen)
			case StateLibrary:
				a.restorePendingFocus(a.libraryScreen)
			case StateDetail:
				a.restorePendingFocus(a.detailScreen)
			case StateFilePicker:
				a.restoreFilePickerFocus()
			}
		}
	}

	// Poll input manager for global keys (F12 screenshot, F11 fullscreen).
	// Screenshots are of gameplay only, paused or not.
	screenshotRequested, fullscreenToggle := a.inputManager.Update()
	if screenshotRequested && a.state == StatePlaying {
		a.screenshotManager.Queue("")
	}
	if fullscreenToggle {
		a.toggleFullscreen()
	}

	a.updatePlayerAssignment()

	a.settleResize()

	switch a.state {
	case StatePlaying:
		// Keep ebitenui's global input handler in sync during gameplay.
		// Without this, the handler's LastLeftMouseButtonPressed goes stale
		// (never updated while UI.Update is not called), causing phantom
		// "just pressed" events on the first UI frame after exiting gameplay.
		ebitenuiInput.Update()
		ebitenuiInput.AfterUpdate()
		_, err := a.gameplay.Update()
		return err
	case StateScanProgress:
		nav := a.processUIInput()
		a.ui.Update()
		a.scanManager.Update()
		// No scroll containers in scan screen
		_ = nav
		return nil
	case StateCleanProgress:
		a.processUIInput()
		a.ui.Update()
		a.cleanManager.Update()
		return nil
	case StateSettings:
		// Skip UI navigation during input capture mode
		if a.settingsScreen.IsInputCaptureActive() {
			a.settingsScreen.Update()
			// Poll navigation to keep InputManager direction state synced.
			// Without this, a d-pad press used for binding leaves im.direction
			// stale, causing an immediate navigation on the rebuild frame.
			a.inputManager.GetUINavigation()
			a.ui.Update()
			return nil
		}
		nav := a.processUIInput()
		a.settingsScreen.Update() // Handle section-specific updates (e.g., clipboard shortcuts)
		a.ui.Update()
		// Check if state changed during ui.Update (e.g., user navigated away)
		if a.state != StateSettings {
			return nil
		}
		if !a.rebuildPending.Load() {
			a.restorePendingFocus(a.settingsScreen)
		}
		if nav.FocusChanged {
			a.ensureFocusedVisible()
		}
		// Check if settings screen triggered a scan (after adding directory)
		if a.settingsScreen.HasPendingScan() {
			a.settingsScreen.ClearPendingScan()
			a.SwitchToScanProgress()
		}
	case StateLibrary:
		// Guard: suppress UI processing until activation inputs from the
		// gameplay transition are released. This prevents ebitenui's
		// handleSubmit (Enter/Space) and mouse click detection from
		// triggering phantom activations on library buttons.
		if a.gameplayTransitionGuard {
			ebitenuiInput.Update()
			ebitenuiInput.AfterUpdate()
			if !ebiten.IsKeyPressed(ebiten.KeyEnter) &&
				!ebiten.IsKeyPressed(ebiten.KeySpace) &&
				!ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
				a.gameplayTransitionGuard = false
			}
			return nil
		}

		// Handle search overlay input first
		if a.searchOverlay.IsActive() {
			a.searchOverlay.HandleInput()
		}

		// Check for '/' to activate search (when not already active)
		if inpututil.IsKeyJustPressed(ebiten.KeySlash) && !a.searchOverlay.IsActive() {
			a.searchOverlay.Activate()
		}

		// ESC clears search if visible or active (before normal back handling)
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) && (a.searchOverlay.IsVisible() || a.searchOverlay.IsActive()) {
			a.searchOverlay.Clear()
			return nil // Don't process as normal back
		}

		// Skip normal UI input if search is capturing
		var nav UINavigation
		if !a.searchOverlay.IsActive() {
			nav = a.processUIInput()
		}
		a.ui.Update()
		// Check if state changed during ui.Update (e.g., user clicked a game)
		if a.state != StateLibrary {
			return nil
		}
		// Update artwork graphics in place when background loader has new entries
		a.libraryScreen.UpdateArtwork()
		if !a.rebuildPending.Load() {
			a.restorePendingFocus(a.libraryScreen)
		}
		// Finish top-aligning the selected icon card once real layout rects
		// are available (one frame after a rebuild).
		a.libraryScreen.ApplyPendingIconScroll()
		if nav.FocusChanged {
			a.ensureFocusedVisible()
		}
	case StateDetail:
		nav := a.processUIInput()
		a.detailScreen.Update()
		a.ui.Update()
		if a.state != StateDetail {
			return nil
		}
		if !a.rebuildPending.Load() {
			a.restorePendingFocus(a.detailScreen)
		}
		if nav.FocusChanged {
			a.ensureFocusedVisible()
		}
	case StateFilePicker:
		nav := a.processUIInput()
		a.filePicker.Update()
		a.ui.Update()
		// Check if state changed during ui.Update (Accept or Cancel)
		if a.state != StateFilePicker {
			return nil
		}
		if !a.rebuildPending.Load() {
			a.restoreFilePickerFocus()
		}
		if nav.FocusChanged {
			a.ensureFocusedVisible()
		}
	default:
		// StateError only
		nav := a.processUIInput()
		prevState := a.state
		a.ui.Update()
		if a.state != prevState {
			return nil
		}
		if nav.FocusChanged {
			a.ensureFocusedVisible()
		}
	}
	return nil
}

// restoreFilePickerFocus focuses the picker's path field when requested,
// otherwise restores the pending focus button.
func (a *App) restoreFilePickerFocus() {
	if input, ok := a.filePicker.TakePathFocusRequest(); ok {
		a.ui.SetFocusedWidget(input)
		return
	}
	a.restorePendingFocus(a.filePicker)
}

// restorePendingFocus restores focus to a pending target if one exists.
// Uses SetFocusedWidget so ebitenui's focusedWidget is updated immediately
// rather than via a deferred event. This prevents processUIInput from seeing
// nil focus and falling back to ChangeFocus (which would focus the wrong widget).
func (a *App) restorePendingFocus(screen screens.FocusRestorer) {
	if target := screen.GetPendingFocus(); target != nil {
		a.ui.SetFocusedWidget(target)
		screen.ClearPendingFocus()
	}
}

// processUIInput polls gamepad input via InputManager and applies UI actions.
// Returns the navigation result for focus scroll handling.
func (a *App) processUIInput() UINavigation {
	if a.ui == nil {
		return UINavigation{}
	}

	nav := a.inputManager.GetUINavigation()

	// Apply navigation direction using spatial navigation if supported
	if nav.Direction != types.DirNone {
		a.applySpatialNavigation(nav.Direction)
	}

	// A/Cross button activates focused widget
	if nav.Activate {
		if focused := a.ui.GetFocusedWidget(); focused != nil {
			if btn, ok := focused.(*widget.Button); ok {
				btn.Click()
			}
		}
	}

	// B/Circle button for back navigation
	if nav.Back {
		a.handleGamepadBack()
	}

	// Start button opens settings from library
	if nav.OpenSettings && a.state == StateLibrary {
		a.SwitchToSettings()
	}

	return nav
}

// applySpatialNavigation uses 2D spatial navigation to find the next focus target.
// Falls back to linear navigation for screens that don't support spatial nav.
func (a *App) applySpatialNavigation(direction int) {
	// Get the current focused widget
	focused := a.ui.GetFocusedWidget()

	// A multi-line field takes directions as caret moves. Up on its first
	// line and Down on its last fall through to move focus out; Left and
	// Right stay in the field.
	if field, ok := focused.(*widgets.MultiLineInput); ok {
		switch direction {
		case types.DirUp:
			if field.MoveCaretUp() {
				return
			}
		case types.DirDown:
			if field.MoveCaretDown() {
				return
			}
		case types.DirLeft:
			field.MoveCaretLeft()
			return
		case types.DirRight:
			field.MoveCaretRight()
			return
		}
	}
	// A single-line field moves its own caret with Left and Right, so
	// only Up and Down move focus out of it.
	if _, ok := focused.(*widget.TextInput); ok && (direction == types.DirLeft || direction == types.DirRight) {
		return
	}

	// The screen whose zones handle navigation. StateError,
	// StateScanProgress, and StateCleanProgress have none and use
	// ebitenui's focus order.
	var screen interface {
		FindFocusInDirection(widget.Focuser, int) widget.Focuser
		FindFocusLinear(widget.Focuser, bool) widget.Focuser
		IsFocusTarget(widget.Focuser) bool
	}
	switch a.state {
	case StateLibrary:
		screen = a.libraryScreen
	case StateDetail:
		screen = a.detailScreen
	case StateSettings:
		screen = a.settingsScreen
	case StateFilePicker:
		if target, handled := a.filePicker.NavigatePath(focused, direction); handled {
			if target != nil {
				a.ui.SetFocusedWidget(target)
			}
			return
		}
		screen = a.filePicker
	}

	forward := !(direction == types.DirUp || direction == types.DirLeft)

	var next widget.Focuser
	if screen != nil {
		next = screen.FindFocusInDirection(focused, direction)
		// Linear fallback when zone navigation has no target. The custom
		// scroll view hides content from ebitenui's GetFocusers, so
		// ChangeFocus cannot reach scrolled widgets; the screen's own
		// ordered list can.
		if next == nil {
			next = screen.FindFocusLinear(focused, forward)
		}
	}

	switch {
	case next != nil:
		// Found a target - unfocus current first
		if focused != nil {
			focused.Focus(false)
		}
		next.Focus(true)
	case screen != nil && screen.IsFocusTarget(focused):
		// At the edge of the screen's navigation: focus stays put
	case forward:
		// Last resort for screens without a registered focus order
		a.ui.ChangeFocus(widget.FOCUS_NEXT)
	default:
		a.ui.ChangeFocus(widget.FOCUS_PREVIOUS)
	}
}

// updatePlayerAssignment reconciles player-to-controller bindings with the
// currently connected gamepads. Runs every frame in all states so bindings
// are current before gameplay polls input and before settings render.
func (a *App) updatePlayerAssignment() {
	if a.state == StateError || a.config == nil {
		return
	}

	pads := SnapshotPads()

	// Give controllers without a standard layout a generated baseline
	// mapping so they work everywhere standard buttons are polled.
	if a.ensurePadMappings(pads) && !a.configLoadFailed {
		storage.SaveConfig(a.config)
	}

	// One-time bootstrap: first ever controller gets a profile and player 1.
	if !a.configLoadFailed && AutoCreateFirstProfile(&a.config.Input, pads) {
		storage.SaveConfig(a.config)
		if a.state == StateSettings {
			a.RequestRebuild()
		}
	}

	events := a.assignment.Update(pads, &a.config.Input)

	// Surface controller changes only during gameplay.
	if a.state != StatePlaying {
		return
	}
	for _, ev := range events {
		state := "connected"
		if !ev.Bound {
			state = "disconnected"
		}
		name := ev.Controller
		if name == "" {
			name = "Controller"
		}
		a.notification.ShowShort(fmt.Sprintf("Player %d: %s %s", ev.Player+1, name, state))
	}
}

// handleGamepadBack handles B button press for back navigation
func (a *App) handleGamepadBack() {
	switch a.state {
	case StateLibrary:
		// Focus the first toolbar button for quick navigation to top
		a.libraryScreen.SetPendingFocus("toolbar-icon")
	case StateDetail:
		if a.detailScreen.HandleBack() {
			return
		}
		a.SwitchToLibrary()
	case StateSettings:
		if a.settingsScreen.HandleBack() {
			return
		}
		a.SwitchToLibrary()
	case StateScanProgress:
		// Cancel scan and return to settings
		a.scanManager.Cancel()
	case StateCleanProgress:
		// Cancel clean and return to settings
		a.cleanManager.Cancel()
	case StateFilePicker:
		a.filePicker.Cancel()
	}
}

// ensureFocusedVisible scrolls the current screen to keep the focused widget visible
func (a *App) ensureFocusedVisible() {
	focused := a.ui.GetFocusedWidget()
	if focused == nil {
		return
	}

	// Call the appropriate screen's scroll method
	switch a.state {
	case StateLibrary:
		a.libraryScreen.EnsureFocusedVisible(focused)
	case StateDetail:
		a.detailScreen.EnsureFocusedVisible(focused)
	case StateSettings:
		a.settingsScreen.EnsureFocusedVisible(focused)
	case StateFilePicker:
		a.filePicker.EnsureFocusedVisible(focused)
	}
}

// Draw implements ebiten.Game
func (a *App) Draw(screen *ebiten.Image) {
	// Advance frame counter for animated shaders
	a.shaderManager.IncrementFrame()

	// Determine which shaders to apply based on state and application mode
	shaderIDs := a.getActiveShaders()

	// Placement of the game picture in screen, for the screenshot crop
	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	picture := shader.FullPicture(sw, sh)

	if len(shaderIDs) == 0 {
		// No shaders/effects - direct draw
		switch a.state {
		case StatePlaying:
			picture = a.gameplay.Draw(screen)
			a.gameplay.DrawPauseMenu(screen)
			a.gameplay.DrawAchievementOverlay(screen)
			a.gameplay.DrawPerfOverlay(screen)
		default:
			a.ui.Draw(screen)
		}
		a.notification.Draw(screen)
		if a.state == StateLibrary {
			a.searchOverlay.Draw(screen)
		}
	} else {
		// With effects/shaders - use preprocessing pipeline
		var processed *ebiten.Image
		switch a.state {
		case StatePlaying:
			// Run the full native-frame pipeline: overscan crop, xBR/scale,
			// ghosting. The frame carries its own pixel aspect ratio so
			// variable-resolution cores scale correctly.
			native, par := a.gameplay.DrawFramebuffer()
			processed, picture = a.shaderManager.ApplyPreprocessEffects(native, par, shaderIDs, sw, sh)
		default:
			// UI: preprocessors are game-only, so draw straight to the buffer.
			buffer := a.getOrCreateShaderBuffer(sw, sh)
			buffer.Clear()
			a.ui.Draw(buffer)
			processed = buffer
		}

		// No frame yet (e.g. before the first emulated frame); skip this draw.
		if processed == nil {
			return
		}

		// For StatePlaying: draw pause menu and achievement overlay after effects (so shaders apply to them)
		if a.state == StatePlaying {
			a.gameplay.DrawPauseMenu(processed)
			a.gameplay.DrawAchievementOverlay(processed)
			a.gameplay.DrawPerfOverlay(processed)
		}

		// Notification drawn after effects, before shaders
		a.notification.Draw(processed)
		if a.state == StateLibrary {
			a.searchOverlay.Draw(processed)
		}

		// Apply shader chain to final screen (preprocess effects are skipped internally)
		a.shaderManager.ApplyShaders(screen, processed, shaderIDs, picture)
	}

	// Save queued screenshots of the final frame's picture, after everything
	// is drawn
	a.screenshotManager.Flush(screen, picture.Rect(), a.gameplay.CurrentGameCRC())
}

// resizeSettleDelay is how long the window size must hold before the UI
// is rebuilt for it, so a drag does not rebuild every frame
const resizeSettleDelay = 170 * time.Millisecond

// settleResize rebuilds the UI for a new window size once the size has
// held for resizeSettleDelay. The window scale changes first so the
// rebuild uses it. Screens with a responsive layout also rebuild for a
// width change alone.
func (a *App) settleResize() {
	if a.windowWidth != a.lastSeenWidth || a.windowHeight != a.lastSeenHeight {
		a.lastSeenWidth = a.windowWidth
		a.lastSeenHeight = a.windowHeight
		a.resizeChangedAt = time.Now()
		a.resizeSettled = false
		return
	}
	if a.resizeSettled || time.Since(a.resizeChangedAt) < resizeSettleDelay {
		return
	}
	a.resizeSettled = true

	scaleChanged := a.targetWindowScale != style.FontScale()
	if scaleChanged {
		style.SetWindowScale(a.targetWindowScale)
	}
	responsive := a.state == StateLibrary || a.state == StateDetail || a.state == StateSettings || a.state == StateFilePicker
	if scaleChanged || (responsive && a.windowWidth != a.lastBuildWidth) {
		a.rebuildCurrentScreen()
	}
}

// Layout implements ebiten.Game
func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	// Query the device scale factor for HiDPI/Retina rendering
	s := display.DPIScale()
	if s != a.currentDPIScale {
		a.currentDPIScale = s
		style.SetDPIScale(s)
		a.rebuildPending.Store(true)
	}

	// The window scale follows the logical size. It is applied at once the
	// first time so the first frame is drawn at the right scale, and after
	// that once the size settles.
	a.targetWindowScale = style.WindowScaleFor(outsideWidth, outsideHeight)
	if !a.windowScaleApplied {
		a.windowScaleApplied = true
		style.SetWindowScale(a.targetWindowScale)
		a.rebuildPending.Store(true)
	}

	// Return physical pixel dimensions so the game renders at full resolution
	w := int(float64(outsideWidth) * s)
	h := int(float64(outsideHeight) * s)
	a.windowWidth = w
	a.windowHeight = h
	// Track windowed dimensions separately so fullscreen doesn't overwrite them.
	// These are used by saveWindowState to persist the pre-fullscreen size.
	// Store the logical size as given so it round-trips exactly through
	// SetWindowSize; converting the physical size back truncates at
	// fractional scale factors.
	if !ebiten.IsFullscreen() {
		a.lastWindowedWidth = outsideWidth
		a.lastWindowedHeight = outsideHeight
	}
	return w, h
}

// ScreenCallback implementations

// SwitchToLibrary transitions to the library screen
func (a *App) SwitchToLibrary() {
	a.notification.Clear()
	// When exiting gameplay, the pause menu processes activation input on
	// press (not release), so Enter/Space/mouse may still be held when the
	// library screen first runs. Set a guard to suppress UI input processing
	// until all activation inputs are released.
	if a.state == StatePlaying {
		a.gameplayTransitionGuard = true
	}
	a.leaveDetail()
	a.previousState = a.state
	a.state = StateLibrary
	a.libraryScreen.OnEnter()
	a.rebuildCurrentScreen()
	// Focus restoration is handled by the Update loop on the next frame
}

// leaveDetail lets the detail screen release what it holds for display
// when the state is about to move away from it
func (a *App) leaveDetail() {
	if a.state == StateDetail {
		a.detailScreen.OnExit()
	}
}

// SwitchToDetail transitions to the detail screen
func (a *App) SwitchToDetail(gameCRC string) {
	a.notification.Clear()
	a.previousState = a.state
	a.state = StateDetail
	a.detailScreen.SetGame(gameCRC)
	a.detailScreen.OnEnter()
	a.rebuildCurrentScreen()
}

// SwitchToSettings transitions to the settings screen
func (a *App) SwitchToSettings() {
	a.notification.Clear()
	a.leaveDetail()
	a.previousState = a.state
	a.state = StateSettings
	a.settingsScreen.OnEnter()
	a.rebuildCurrentScreen()
}

// SwitchToSettingsSection transitions to the settings screen with the
// given section selected
func (a *App) SwitchToSettingsSection(focusKey string) {
	a.notification.Clear()
	a.leaveDetail()
	a.previousState = a.state
	a.state = StateSettings
	a.settingsScreen.OnEnter()
	a.settingsScreen.SelectSection(focusKey)
	a.rebuildCurrentScreen()
}

// OpenFilePicker shows the full-screen file/folder picker. The current state
// is restored when the picker closes.
func (a *App) OpenFilePicker(req types.FilePickerRequest) {
	a.notification.Clear()
	a.pickerReturnState = a.state
	a.previousState = a.state
	a.state = StateFilePicker
	a.filePicker.Open(req)
	a.rebuildCurrentScreen()
}

// closeFilePicker returns to the screen that opened the picker.
func (a *App) closeFilePicker() {
	a.previousState = a.state
	a.state = a.pickerReturnState
	a.rebuildCurrentScreen()
}

// SwitchToScanProgress transitions to the scan progress screen
func (a *App) SwitchToScanProgress() {
	a.notification.Clear()
	a.previousState = a.state
	a.state = StateScanProgress

	// Start scan via manager
	a.scanManager.Start()
	a.scanScreen.OnEnter()
	a.rebuildCurrentScreen()
}

// SwitchToCleanProgress transitions to the progress screen and starts
// Clean Library
func (a *App) SwitchToCleanProgress() {
	a.notification.Clear()
	a.previousState = a.state
	a.state = StateCleanProgress

	a.cleanManager.Start()
	a.scanScreen.OnEnter()
	a.rebuildCurrentScreen()
}

// LaunchGame starts the emulator with the specified game. discIndex
// selects the disc of a multi-disc game; -1 uses the remembered
// SelectedDisc. Ignored for single-disc and cartridge games.
func (a *App) LaunchGame(gameCRC string, resume bool, discIndex int) {
	// Reset shader buffers to avoid stale data from previous game
	a.shaderManager.ResetBuffers()
	a.shaderManager.SetAspectRatioMode(a.config.Video.AspectRatio)

	if a.gameplay.Launch(gameCRC, resume, discIndex) {
		a.leaveDetail()
		a.previousState = a.state
		a.state = StatePlaying

		// Free the library cover textures while playing; they are rebuilt when
		// the icon view is shown again on return. Only done for large libraries,
		// where the thumbnail cache is large enough to be worth reclaiming.
		if a.library.GameCount() > thumbnailCacheClearThreshold {
			a.libraryScreen.ClearArtworkCache()
		}
	}
}

// Exit closes the application
func (a *App) Exit() {
	// Save window state before exiting
	a.windowX, a.windowY = ebiten.WindowPosition()
	a.lastFullscreenState = ebiten.IsFullscreen()
	a.saveWindowState()

	// Clean up achievement manager resources
	if a.achievementManager != nil {
		a.achievementManager.Destroy()
	}

	a.libraryScreen.HaltArtworkLoader()
	a.screenshotManager.Close()

	// Clean exit using os.Exit to avoid log.Fatal's stack trace
	os.Exit(0)
}

// GetWindowWidth returns the current window width for responsive layouts
func (a *App) GetWindowWidth() int {
	return a.windowWidth
}

// RequestRebuild triggers a UI rebuild for the current screen.
// This is safe to call from goroutines - the rebuild happens on the main thread.
// Focus restoration is handled in the Update loop after ui.Update()
func (a *App) RequestRebuild() {
	a.rebuildPending.Store(true)
}

// GetMD5ByCRC32 returns the MD5 hash for a game by CRC32 from loaded RDBs
func (a *App) GetMD5ByCRC32(crc32 uint32) string {
	return a.metadata.GetMD5ByCRC32(crc32)
}

// GetExtensions returns the file extensions for the core's content: the disc
// image formats for a disc core, or the declared ROM extensions otherwise.
func (a *App) GetExtensions() []string {
	return contentExtensions(a.systemInfo)
}

// contentExtensions returns the file extensions that identify a core's content
// on disk. A disc core reads its content through a format-agnostic interface and
// does not declare extensions itself, so the openable disc-image formats come
// from romloader. A cartridge core uses the ROM extensions it declares.
func contentExtensions(info coreif.SystemInfo) []string {
	if info.Disc {
		return romloader.DiscExtensions()
	}
	return info.Extensions
}

// ShowNotification shows a brief notification message
func (a *App) ShowNotification(msg string) {
	a.notification.ShowDefault(msg)
}

// cheatSystem returns the core's cheat system, or 0 when the core has no
// cheat support. Cheats need memory access (downloads.Cheats) and a system
// the cheats package supports; the first metadata variant whose RDB name
// it supports is used.
func (a *App) cheatSystem() cheats.System {
	if !a.downloads.Cheats {
		return 0
	}
	for i := 0; i < a.metadata.VariantCount(); i++ {
		if sys, err := cheats.SystemFromName(a.metadata.VariantRDBName(i)); err == nil {
			return sys
		}
	}
	return 0
}

// handleDeleteAndContinue handles the delete and continue button
func (a *App) handleDeleteAndContinue() {
	switch a.errorFile {
	case "config.json":
		if err := storage.DeleteConfig(); err != nil {
			log.Printf("Failed to delete config: %v", err)
		}
		a.config = storage.DefaultConfig()
		if err := storage.SaveConfig(a.config); err != nil {
			log.Printf("Failed to save config: %v", err)
		}
		a.applyConfig()
		if !a.loadLibrary() {
			return
		}
	case "library.json":
		if err := storage.DeleteLibrary(); err != nil {
			log.Printf("Failed to delete library: %v", err)
		}
		a.library = storage.DefaultLibrary()
		if err := storage.SaveLibrary(a.library); err != nil {
			log.Printf("Failed to save library: %v", err)
		}
	}

	a.finishStartup()
}

// handleResetAndContinue handles the reset and continue button for validation errors.
// It corrects invalid config fields to defaults, saves, and proceeds to the library.
func (a *App) handleResetAndContinue() {
	// Correct invalid fields
	themeNames := style.ThemeNames()
	storage.CorrectConfig(a.config, themeNames)

	// Save corrected config
	if err := storage.SaveConfig(a.config); err != nil {
		log.Printf("Failed to save corrected config: %v", err)
	}

	a.applyConfig()
	if !a.loadLibrary() {
		return
	}
	a.finishStartup()
}

// handleLibraryResetAndContinue handles the reset and continue button for
// library validation errors. It corrects invalid library-level fields, saves,
// and proceeds to the library screen.
func (a *App) handleLibraryResetAndContinue() {
	storage.CorrectLibrary(a.library)

	if err := storage.SaveLibrary(a.library); err != nil {
		log.Printf("Failed to save corrected library: %v", err)
	}

	a.finishStartup()
}

// SaveAndClose saves config and library before exit
func (a *App) SaveAndClose() {
	// Capture current window state before saving
	a.saveWindowState()

	// Don't overwrite a library that failed to load or validate, or replace
	// it with the placeholder used by the error screen
	if !a.libraryLoaded {
		return
	}
	if err := storage.SaveLibrary(a.library); err != nil {
		log.Printf("Failed to save library: %v", err)
	}
}

// preloadConfiguredShaders loads all shaders referenced in config
func (a *App) preloadConfiguredShaders() {
	allShaders := make(map[string]bool)
	for _, id := range a.config.Shaders.UIShaders {
		allShaders[id] = true
	}
	for _, id := range a.config.Shaders.GameShaders {
		allShaders[id] = true
	}

	ids := make([]string, 0, len(allShaders))
	for id := range allShaders {
		ids = append(ids, id)
	}
	a.shaderManager.PreloadShaders(ids)
}

// getActiveShaders returns the shader IDs to apply for the current state
func (a *App) getActiveShaders() []string {
	switch a.state {
	case StatePlaying:
		return a.gameplay.GameShaders()
	default:
		return a.config.Shaders.UIShaders
	}
}

// getOrCreateShaderBuffer returns a buffer matching the given dimensions
func (a *App) getOrCreateShaderBuffer(width, height int) *ebiten.Image {
	if a.shaderBuffer != nil {
		bw, bh := a.shaderBuffer.Bounds().Dx(), a.shaderBuffer.Bounds().Dy()
		if bw == width && bh == height {
			return a.shaderBuffer
		}
		a.shaderBuffer.Deallocate()
	}
	a.shaderBuffer = display.NewUnmanagedImage(width, height)
	return a.shaderBuffer
}
