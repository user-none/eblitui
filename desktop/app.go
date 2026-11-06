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
	"github.com/user-none/eblitui/desktop/content"
	"github.com/user-none/eblitui/desktop/display"
	"github.com/user-none/eblitui/desktop/metadata"
	"github.com/user-none/eblitui/desktop/netutil"
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
	emu        Emulator // the application built around the core
	hasMemory  bool     // the core's emulator implements coreif.Memory

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

	// Game metadata from the emulator's metadata source
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
// or nil. Used so the scanner can key disc games by product number.
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
// factory supplies the core; emu describes the application built around it.
func Run(factory coreif.CoreFactory, emu Emulator) error {
	info := factory.SystemInfo()

	// oto allows a single context rate per process. Use the core's rate
	// so emulator audio plays at the correct pitch and the audio-demand
	// pacing stays in lockstep; synthesized UI sounds are generated at
	// this same rate. Must be set before any audio/oto use.
	if info.SampleRate > 0 {
		audioSampleRate = info.SampleRate
	}

	// Initialize storage with the emulator's data directory
	storage.Init(emu.DataDirName)

	// Configure window
	ebiten.SetWindowTitle(emu.Name)
	applyWindowIcon(emu.Icon)
	// Remove the macOS About menu item; the in-app About (settings) is the only
	// About. One-time call; on darwin the menu work is deferred until the menu
	// exists. No-op on other platforms.
	removeNativeAboutMenuItem()
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSizeLimits(style.MinWindowWidth, style.MinWindowHeight, -1, -1)

	app, err := newApp(factory, info, emu)
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
func newApp(factory coreif.CoreFactory, info coreif.SystemInfo, emu Emulator) (*App, error) {
	// Rumble and cheats both need memory access
	memory := supportsMemory(factory)
	app := &App{
		state:           StateLibrary,
		factory:         factory,
		systemInfo:      info,
		emu:             emu,
		hasMemory:       memory,
		padMappingTried: make(map[string]bool),
	}

	// Artwork and update downloads identify the application as the
	// RetroAchievements requests do: AppName/Version
	netutil.SetUserAgent(fmt.Sprintf("%s/%s", emu.Name, emu.Version))

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

// initMetadata creates the metadata manager, loading all of the core's game
// metadata once for the session.
func (a *App) initMetadata() {
	a.metadata = metadata.NewMetadataManager(a.emu.Metadata)
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

	if errs := a.config.Validate(configRules()); len(errs) > 0 {
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
	cfg := a.config.Values()
	style.ApplyThemeByName(cfg.Theme)
	applySavedPadMappings(cfg.Input.PadMappings)
}

// newAchievementManager creates the achievement manager for the current
// config. A previous manager is destroyed so none keeps a replaced config.
func (a *App) newAchievementManager() {
	if a.achievementManager != nil {
		a.achievementManager.Destroy()
	}
	a.achievementManager = achievements.NewManager(a.notification, a.config, a.emu.Name, a.emu.Version, uint32(a.emu.RetroAchievementsConsoleID), a.systemInfo.BigEndianMemory, audioSampleRate)
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

	if errs := library.Validate(); len(errs) > 0 {
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
		a.cheatSystem(),
		a.emu.Cheats,
		a.emu.Rumble,
		a.emu.RetroAchievementsConsoleID,
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
	a.updateManager = NewUpdateManager(a.emu.Update, a.emu.Name, a.emu.Version, a.config, a.RequestRebuild)
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
	ra := a.config.Values().RetroAchievements
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
						a.config.ClearRetroAchievementsToken()
					}
				}
			},
		)
	}()
}

// scanSources returns where a scan downloads artwork from, the emulator's
// artwork config, and which artwork, the library's artwork download
// setting
func (a *App) scanSources() scanner.Sources {
	var src scanner.Sources
	if a.emu.Artwork != nil {
		src.ArtworkURL = a.emu.Artwork.URL
		src.ArtworkKeys = append([]string(nil), a.emu.Artwork.Keys...)
	}
	if a.config != nil {
		src.ArtworkDownload = a.config.Values().Library.ArtworkDownload
	}
	return src
}

// newScanManager creates the scan manager. It needs the scan screen, so it
// is created after initScreens.
func (a *App) newScanManager() *ScanManager {
	return NewScanManager(
		a.library,
		a.scanScreen,
		contentExtensions(a.systemInfo),
		a.emu.RetroAchievementsConsoleID,
		a.systemInfo.Disc,
		discIdentifier(a.factory),
		a.scanSources,
		nil, // onProgress: the scan screen updates its widgets in place
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
		nil, // onProgress: the scan screen updates its widgets in place
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
	w := a.config.Values().Window
	return w.Width, w.Height, w.X, w.Y, w.Fullscreen
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
	a.config.SetWindow(storage.WindowConfig{
		Width:      a.lastWindowedWidth,
		Height:     a.lastWindowedHeight,
		X:          &a.windowX,
		Y:          &a.windowY,
		Fullscreen: a.lastFullscreenState,
	})
}

// toggleFullscreen toggles between fullscreen and windowed mode
func (a *App) toggleFullscreen() {
	ebiten.SetFullscreen(!ebiten.IsFullscreen())
	a.lastFullscreenState = ebiten.IsFullscreen()
	a.config.SetFullscreen(a.lastFullscreenState)
}

// initScreens creates all screen instances
func (a *App) initScreens() {
	a.libraryScreen = screens.NewLibraryScreen(a, a.library, a.config, a.updates())
	a.detailScreen = screens.NewDetailScreen(a, a.library, a.config, a.achievementManager, a.emu.RetroAchievementsConsoleID, a.cheatSystem(), a.emu.Cheats, a.systemInfo.BigEndianMemory, a.systemInfo.CoreOptions)
	a.settingsScreen = screens.NewSettingsScreen(a, a.library, a.config, a.achievementManager, a.systemInfo.SerializeSize, a.systemInfo, a.emu.Name, a.emu.Version, a.emu.Icon, a.updates())
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
		a.detailScreen.SetConfig(a.config)
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

		// Save a running game the way leaving through the pause menu does:
		// battery save, resume state and play time. Exit does nothing when
		// no game is loaded.
		if a.gameplay != nil {
			a.gameplay.Exit(true)
		}

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
		// Finish scrolling the selected game into place once real layout rects
		// are available (one frame after a rebuild).
		a.libraryScreen.ApplyPendingScroll()
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
		ScrollBy(widget.Focuser, int) bool
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

	// A focused scroll bar scrolls with Up and Down until the end of its
	// view; past that, and for the other directions, focus moves on
	if screen != nil && screen.ScrollBy(focused, direction) {
		return
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
	a.ensurePadMappings(pads)

	// One-time bootstrap: first ever controller gets a profile and player 1.
	input := a.config.Input()
	if !a.configLoadFailed && AutoCreateFirstProfile(&input, pads) {
		id := a.config.AddProfile(input.Profiles[0])
		a.config.SetPlayerProfile(0, id)
		input = a.config.Input()
		if a.state == StateSettings {
			a.RequestRebuild()
		}
	}

	events := a.assignment.Update(pads, &input)

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
	case StateScanProgress, StateCleanProgress:
		// Cancel the scan or clean, as the Cancel button does; settings
		// show once it has stopped
		a.scanScreen.Cancel()
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
	a.screenshotManager.Flush(screen, picture.Rect(), a.gameplay.CurrentGameID())
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
func (a *App) SwitchToDetail(gameID string) {
	a.notification.Clear()
	a.previousState = a.state
	a.state = StateDetail
	a.detailScreen.SetGame(gameID)
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
func (a *App) LaunchGame(gameID string, resume bool, discIndex int) {
	// Reset shader buffers to avoid stale data from previous game
	a.shaderManager.ResetBuffers()
	a.shaderManager.SetAspectRatioMode(a.config.Values().Video.AspectRatio)

	if a.gameplay.Launch(gameID, resume, discIndex) {
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

// GameMetadata returns the emulator's metadata for a game ID and true, or
// false when it has none for it.
func (a *App) GameMetadata(gameID string) (content.GameMetadata, bool) {
	return a.metadata.Game(gameID)
}

// GameTitle returns the title to show for a library entry.
func (a *App) GameTitle(game *storage.GameEntry) string {
	return a.metadata.Title(game)
}

// GameInfo returns the keys a library entry is sorted and searched by: its
// title and supported region codes.
func (a *App) GameInfo(game *storage.GameEntry) storage.GameKeys {
	return a.metadata.Info(game)
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

// cheatSystem returns the emulator's cheat system, or 0 when cheats are off.
// Cheats need memory access and an Emulator.CheatFormat the cheats package
// supports.
func (a *App) cheatSystem() cheats.System {
	if !a.hasMemory || a.emu.CheatFormat == "" {
		return 0
	}
	sys, err := cheats.SystemFromName(a.emu.CheatFormat)
	if err != nil {
		log.Printf("Cheats disabled: %v", err)
		return 0
	}
	return sys
}

// handleDeleteAndContinue handles the delete and continue button
func (a *App) handleDeleteAndContinue() {
	switch a.errorFile {
	case "config.json":
		config, err := storage.ResetConfig()
		if err != nil {
			log.Printf("Failed to reset config: %v", err)
			config = storage.DefaultConfig()
		}
		a.config = config
		a.applyConfig()
		if !a.loadLibrary() {
			return
		}
	case "library.json":
		library, err := storage.ResetLibrary()
		if err != nil {
			log.Printf("Failed to reset library: %v", err)
			return
		}
		a.library = library
	}

	a.finishStartup()
}

// configRules returns the rules the configuration is validated and
// corrected against: the known themes and the bindable key and button
// names.
func configRules() storage.ConfigRules {
	return storage.ConfigRules{
		Themes:   style.ThemeNames(),
		ValidKey: validKeyName,
		ValidPad: validPadName,
	}
}

// handleResetAndContinue handles the reset and continue button for validation errors.
// It corrects invalid config fields to defaults, saves, and proceeds to the library.
func (a *App) handleResetAndContinue() {
	// Correct invalid fields, which saves the corrected config
	a.config.Correct(configRules())

	a.applyConfig()
	if !a.loadLibrary() {
		return
	}
	a.finishStartup()
}

// handleLibraryResetAndContinue handles the reset and continue button for
// library validation errors. It corrects invalid library-level fields,
// which saves them, and proceeds to the library screen.
func (a *App) handleLibraryResetAndContinue() {
	a.library.Correct()
	a.finishStartup()
}

// SaveAndClose saves config and library before exit
func (a *App) SaveAndClose() {
	// Capture current window state before saving
	a.saveWindowState()

	// The config saves itself after each change; wait for those writes.
	// A config.json that failed to load is replaced by a default that is
	// never written.
	a.config.Flush()

	// The library saves itself after each change; wait for those writes.
	// A library that failed to validate, or the error screen's placeholder,
	// is never changed and so never written.
	a.library.Flush()
}

// preloadConfiguredShaders loads all shaders referenced in config
func (a *App) preloadConfiguredShaders() {
	shaders := a.config.Values().Shaders
	allShaders := make(map[string]bool)
	for _, id := range shaders.UIShaders {
		allShaders[id] = true
	}
	for _, id := range shaders.GameShaders {
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
		return a.config.Shaders().UIShaders
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
