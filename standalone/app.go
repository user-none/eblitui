//go:build !libretro

package standalone

import (
	"fmt"
	"log"
	"os"

	"github.com/ebitenui/ebitenui"
	ebitenuiInput "github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	emucore "github.com/user-none/eblitui/api"
	"github.com/user-none/eblitui/standalone/achievements"
	"github.com/user-none/eblitui/rdb"
	"github.com/user-none/eblitui/standalone/screens"
	"github.com/user-none/eblitui/standalone/shader"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
	"github.com/user-none/eblitui/standalone/types"
	rcheevos "github.com/user-none/go-rcheevos"
)

// App is the main application struct that implements ebiten.Game
type App struct {
	ui *ebitenui.UI

	// Core factory and system info (set by Run)
	factory    emucore.CoreFactory
	systemInfo emucore.SystemInfo

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

	// Metadata for RDB lookups
	metadata *MetadataManager

	// Gameplay (emulation, input, save states, pause menu)
	gameplay *GameplayManager

	// UI managers
	notification      *Notification
	saveStateManager  *SaveStateManager
	screenshotManager *ScreenshotManager
	searchOverlay     *SearchOverlay

	// Scan manager
	scanManager *ScanManager

	// Error state
	errorFile        string
	errorPath        string
	configLoadFailed bool // True if config.json failed to load (don't overwrite on exit)

	// Window tracking for persistence and responsive layouts
	windowX, windowY   int
	windowWidth        int
	windowHeight       int
	lastWindowedWidth  int // Last non-fullscreen width (physical pixels)
	lastWindowedHeight int // Last non-fullscreen height (physical pixels)
	lastBuildWidth     int // Track width used for last UI build

	// Screenshot pending flag (set in Update, processed in Draw)
	screenshotPending bool

	// Input manager for UI navigation
	inputManager *InputManager

	// Shader manager for visual effects
	shaderManager *shader.Manager
	shaderBuffer  *ebiten.Image // Intermediate buffer for shader rendering

	// Achievement manager for RetroAchievements integration
	achievementManager *achievements.Manager

	// Rebuild pending flag (set from goroutines, processed on main thread)
	rebuildPending bool

	// Guard to prevent input leaking from gameplay to UI screens.
	// When the pause menu triggers a screen transition, the activation input
	// (Enter/Space/mouse click) may still be held. ebitenui's fresh widget
	// state would interpret the held input as a new press, causing phantom
	// clicks on the library screen. The guard suppresses UI input processing
	// until all activation inputs are released.
	gameplayTransitionGuard bool

	// HiDPI: current device scale factor tracked across Layout calls
	currentDPIScale float64

	// Fullscreen: track state so it can be saved on exit even if macOS
	// has already left native fullscreen by the time saveWindowState runs.
	lastFullscreenState bool
}

// Run is the public entry point for the standalone UI. It initializes storage,
// configures the window, creates the app, and starts the Ebiten game loop.
func Run(factory emucore.CoreFactory) error {
	info := factory.SystemInfo()

	// Initialize storage with core-specific data directory
	storage.Init(info.DataDirName)

	// Configure window
	ebiten.SetWindowTitle(info.CoreName)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSizeLimits(900, 650, -1, -1)

	app, err := newApp(factory, info)
	if err != nil {
		return err
	}

	// Restore window size from saved config (before RunGame to avoid resize flash)
	width, height, x, y, fullscreen := app.GetWindowConfig()
	if width < 900 {
		width = 900
	}
	if height < 650 {
		height = 650
	}
	ebiten.SetWindowSize(width, height)

	// Restore window position if previously saved
	if x != nil && y != nil {
		ebiten.SetWindowPosition(*x, *y)
	}

	// Restore fullscreen state
	if fullscreen {
		ebiten.SetFullscreen(true)
	}

	if err := ebiten.RunGame(app); err != nil {
		return err
	}

	app.SaveAndClose()
	return nil
}

// newApp creates and initializes the application with the given core factory and system info.
func newApp(factory emucore.CoreFactory, info emucore.SystemInfo) (*App, error) {
	app := &App{
		state:      StateLibrary,
		factory:    factory,
		systemInfo: info,
	}

	// Ensure directory structure exists
	if err := storage.EnsureDirectories(); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	// Create config/library files if missing
	if err := storage.CreateConfigIfMissing(); err != nil {
		log.Printf("Warning: failed to create config: %v", err)
	}
	if err := storage.CreateLibraryIfMissing(); err != nil {
		log.Printf("Warning: failed to create library: %v", err)
	}

	// Start RDB download in background if missing (non-blocking)
	if !RDBExists() {
		go func() {
			metadata := NewMetadataManager(info.RDBName, info.ThumbnailRepo)
			if err := metadata.DownloadRDB(); err != nil {
				log.Printf("Background RDB download failed: %v", err)
			}
		}()
	}

	// Initialize UI managers
	app.notification = NewNotification()
	app.saveStateManager = NewSaveStateManager(app.notification)
	app.screenshotManager = NewScreenshotManager(app.notification)
	app.inputManager = NewInputManager()
	app.shaderManager = shader.NewManager()
	app.searchOverlay = NewSearchOverlay(func(text string) {
		if app.state == StateLibrary {
			app.libraryScreen.SetSearchText(text)
			app.rebuildCurrentScreen()
		}
	})

	// Initialize metadata manager and load RDB (for achievement MD5 lookups)
	app.metadata = NewMetadataManager(info.RDBName, info.ThumbnailRepo)
	if err := app.metadata.LoadRDB(); err != nil {
		log.Printf("Failed to load RDB: %v", err)
	}

	// Load config
	config, err := storage.LoadConfig()
	if err != nil {
		// JSON parse error - show error screen
		configPath, _ := storage.GetConfigPath()
		app.state = StateError
		app.errorFile = "config.json"
		app.errorPath = configPath
		app.configLoadFailed = true // Don't overwrite the file on exit
		app.config = storage.DefaultConfig()
		app.achievementManager = achievements.NewManager(app.notification, app.config, app.systemInfo.Name, Version, uint32(app.systemInfo.ConsoleID))
		app.library = storage.DefaultLibrary()
		app.preloadConfiguredShaders()
		app.initScreens()
		app.rebuildCurrentScreen()
		return app, nil
	}
	app.config = config

	// Validate config values against allowed ranges
	themeNames := style.ThemeNames()
	validationErrors := storage.ValidateConfig(app.config, themeNames)
	if len(validationErrors) > 0 {
		configPath, _ := storage.GetConfigPath()
		app.state = StateError
		app.errorFile = "config.json"
		app.errorPath = configPath
		app.configLoadFailed = true
		// Use default theme/font for the error screen display
		app.achievementManager = achievements.NewManager(app.notification, app.config, app.systemInfo.Name, Version, uint32(app.systemInfo.ConsoleID))
		app.library = storage.DefaultLibrary()
		app.preloadConfiguredShaders()
		app.initScreens()
		app.errorScreen.SetValidationError("config.json", configPath, validationErrors, app.handleResetAndContinue)
		app.rebuildCurrentScreen()
		return app, nil
	}

	// Create achievement manager with config
	app.achievementManager = achievements.NewManager(app.notification, app.config, app.systemInfo.Name, Version, uint32(app.systemInfo.ConsoleID))

	// Apply theme and font size
	style.ApplyThemeByName(app.config.Theme)
	style.ApplyFontSize(storage.ValidFontSize(app.config.FontSize))

	// Load library
	library, err := storage.LoadLibrary()
	if err != nil {
		// JSON parse error - show error screen
		libraryPath, _ := storage.GetLibraryPath()
		app.state = StateError
		app.errorFile = "library.json"
		app.errorPath = libraryPath
		app.preloadConfiguredShaders()
		app.initScreens()
		app.rebuildCurrentScreen()
		return app, nil
	}

	// Validate library-level fields
	libraryErrors := storage.ValidateLibrary(library)
	if len(libraryErrors) > 0 {
		libraryPath, _ := storage.GetLibraryPath()
		app.state = StateError
		app.errorFile = "library.json"
		app.errorPath = libraryPath
		app.library = library
		app.preloadConfiguredShaders()
		app.initScreens()
		app.errorScreen.SetValidationError("library.json", libraryPath, libraryErrors, app.handleLibraryResetAndContinue)
		app.rebuildCurrentScreen()
		return app, nil
	}
	app.library = library

	// Set library on save state manager for slot persistence
	app.saveStateManager.SetLibrary(library)

	// Initialize gameplay manager with callbacks
	app.gameplay = NewGameplayManager(
		app.factory,
		app.systemInfo,
		app.saveStateManager,
		app.screenshotManager,
		app.notification,
		app.library,
		app.config,
		app.achievementManager,
		app.metadata.GetRDB(),
		func() { app.SwitchToLibrary() }, // onExitToLibrary
		func() { app.Exit() },            // onExitApp
	)

	// Auto-login with stored token if available
	if app.config.RetroAchievements.Enabled &&
		app.config.RetroAchievements.Username != "" &&
		app.config.RetroAchievements.Token != "" {
		go func() {
			app.achievementManager.LoginWithToken(
				app.config.RetroAchievements.Username,
				app.config.RetroAchievements.Token,
				func(success bool, result int, err error) {
					if !success {
						log.Printf("RetroAchievements auto-login failed: %v", err)
						// Only clear token for credential errors, not transient failures
						if result == rcheevos.InvalidCredentials ||
							result == rcheevos.ExpiredToken ||
							result == rcheevos.AccessDenied {
							app.config.RetroAchievements.Token = ""
							storage.SaveConfig(app.config)
						}
					}
				},
			)
		}()
	}

	// Preload configured shaders
	app.preloadConfiguredShaders()

	// Initialize screens and build initial UI
	// Window dimensions are set by main before RunGame, so they're already correct
	app.initScreens()

	// Initialize scan manager (needs scanScreen reference)
	app.scanManager = NewScanManager(
		app.library,
		app.scanScreen,
		app.systemInfo.Extensions,
		app.systemInfo.RDBName,
		app.systemInfo.ThumbnailRepo,
		func() { app.rebuildCurrentScreen() }, // onProgress
		func(msg string) { // onComplete
			app.libraryScreen.ClearArtworkCache()
			app.state = StateSettings
			app.rebuildCurrentScreen()
			if msg != "" {
				app.notification.ShowDefault(msg)
			}
		},
	)

	// Call OnEnter for initial screen (sets default focus)
	app.libraryScreen.OnEnter()
	app.rebuildCurrentScreen()

	return app, nil
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
	s := style.DPIScale()
	a.config.Window.Width = int(float64(a.lastWindowedWidth) / s)
	a.config.Window.Height = int(float64(a.lastWindowedHeight) / s)
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
	a.libraryScreen = screens.NewLibraryScreen(a, a.library, a.config)
	a.detailScreen = screens.NewDetailScreen(a, a.library, a.config, a.achievementManager)
	a.settingsScreen = screens.NewSettingsScreen(a, a.library, a.config, a.achievementManager, a.systemInfo.SerializeSize)
	a.scanScreen = screens.NewScanProgressScreen(a)
	a.errorScreen = screens.NewErrorScreen(a, a.errorFile, a.errorPath, a.handleDeleteAndContinue)
}

// rebuildCurrentScreen rebuilds the UI for the current state
func (a *App) rebuildCurrentScreen() {
	var container *widget.Container

	switch a.state {
	case StateLibrary:
		// Save scroll position before rebuilding
		a.libraryScreen.SaveScrollPosition()
		a.libraryScreen.SetLibrary(a.library)
		a.libraryScreen.SetConfig(a.config)
		container = a.libraryScreen.Build()
	case StateDetail:
		// Save focused button before rebuild so async rebuilds (e.g., achievement
		// loading) restore focus to the previously focused button.
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
	case StateScanProgress:
		container = a.scanScreen.Build()
	case StateError:
		container = a.errorScreen.Build()
	default:
		// For StatePlaying, no UI container needed
		return
	}

	a.ui = &ebitenui.UI{Container: container}
	a.lastBuildWidth = a.windowWidth // Track width for responsive rebuild detection
}

// Update implements ebiten.Game
func (a *App) Update() error {
	// Track window position and fullscreen state for save on exit.
	// Layout() handles width/height, but position must be queried here.
	// Fullscreen is tracked because macOS exits native fullscreen before
	// the save handler runs on Cmd+Q, so we can't rely on IsFullscreen() at exit.
	a.windowX, a.windowY = ebiten.WindowPosition()
	a.lastFullscreenState = ebiten.IsFullscreen()

	// Process any pending rebuild request (set from goroutines)
	if a.rebuildPending {
		a.rebuildPending = false
		a.rebuildCurrentScreen()
	}

	// Poll input manager for global keys (F12 screenshot, F11 fullscreen)
	screenshotRequested, fullscreenToggle := a.inputManager.Update()
	if screenshotRequested {
		a.screenshotPending = true
	}
	if fullscreenToggle {
		a.toggleFullscreen()
	}

	// Check for window resize that needs UI rebuild (for responsive layouts)
	// Rebuild when width changes for screens with responsive layout
	needsResizeRebuild := false
	if a.state == StateLibrary {
		needsResizeRebuild = true
	}
	if a.state == StateDetail {
		needsResizeRebuild = true
	}
	if a.state == StateSettings {
		needsResizeRebuild = true
	}
	if needsResizeRebuild && a.windowWidth > 0 && a.windowWidth != a.lastBuildWidth {
		a.rebuildCurrentScreen()
	}

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
	case StateSettings:
		nav := a.processUIInput()
		a.settingsScreen.Update() // Handle section-specific updates (e.g., clipboard shortcuts)
		a.ui.Update()
		// Check if state changed during ui.Update (e.g., user navigated away)
		if a.state != StateSettings {
			return nil
		}
		if !a.rebuildPending {
			a.restorePendingFocus(a.settingsScreen)
		}
		if nav.FocusChanged {
			a.ensureFocusedVisible()
		}
		// Check if settings screen triggered a scan (after adding directory)
		if a.settingsScreen.HasPendingScan() {
			a.settingsScreen.ClearPendingScan()
			a.SwitchToScanProgress(false)
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
		if !a.rebuildPending {
			a.restorePendingFocus(a.libraryScreen)
		}
		if nav.FocusChanged {
			a.ensureFocusedVisible()
		}
	case StateDetail:
		nav := a.processUIInput()
		a.ui.Update()
		if a.state != StateDetail {
			return nil
		}
		if !a.rebuildPending {
			a.restorePendingFocus(a.detailScreen)
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

// restorePendingFocus restores focus to a pending button if one exists
func (a *App) restorePendingFocus(screen screens.FocusRestorer) {
	btn := screen.GetPendingFocusButton()
	if btn != nil {
		btn.Focus(true)
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

	// Try spatial navigation on the current screen
	var nextBtn *widget.Button

	switch a.state {
	case StateLibrary:
		nextBtn = a.libraryScreen.FindFocusInDirection(focused, direction)
	case StateDetail:
		nextBtn = a.detailScreen.FindFocusInDirection(focused, direction)
	case StateSettings:
		nextBtn = a.settingsScreen.FindFocusInDirection(focused, direction)
		// StateError and StateScanProgress use linear navigation (simple layouts)
	}

	if nextBtn != nil {
		// Spatial navigation found a target - unfocus current first
		if focused != nil {
			focused.Focus(false)
		}
		nextBtn.Focus(true)
	} else {
		// Fallback to linear navigation
		if direction == types.DirUp || direction == types.DirLeft {
			a.ui.ChangeFocus(widget.FOCUS_PREVIOUS)
		} else {
			a.ui.ChangeFocus(widget.FOCUS_NEXT)
		}
	}
}

// handleGamepadBack handles B button press for back navigation
func (a *App) handleGamepadBack() {
	switch a.state {
	case StateLibrary:
		// Focus the first toolbar button for quick navigation to top
		a.libraryScreen.SetPendingFocus("toolbar-icon")
	case StateDetail:
		a.SwitchToLibrary()
	case StateSettings:
		a.SwitchToLibrary()
	case StateScanProgress:
		// Cancel scan and return to settings
		a.scanManager.Cancel()
		// StateError has no back action
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
	case StateSettings:
		a.settingsScreen.EnsureFocusedVisible(focused)
	}
}

// Draw implements ebiten.Game
func (a *App) Draw(screen *ebiten.Image) {
	// Advance frame counter for animated shaders
	a.shaderManager.IncrementFrame()

	// Determine which shaders to apply based on state and application mode
	shaderIDs := a.getActiveShaders()

	if len(shaderIDs) == 0 {
		// No shaders/effects - direct draw
		switch a.state {
		case StatePlaying:
			a.gameplay.Draw(screen)
			a.gameplay.DrawPauseMenu(screen)
			a.gameplay.DrawAchievementOverlay(screen)
		default:
			a.ui.Draw(screen)
		}
		a.notification.Draw(screen)
		if a.state == StateLibrary {
			a.searchOverlay.Draw(screen)
		}
	} else {
		// With effects/shaders - use preprocessing pipeline
		sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
		buffer := a.getOrCreateShaderBuffer(sw, sh)
		buffer.Clear()

		// Determine input for preprocessing based on xBR and state
		var preprocessInput *ebiten.Image
		hasXBR := shader.HasXBR(shaderIDs)

		switch a.state {
		case StatePlaying:
			if hasXBR {
				// xBR path: pass native framebuffer to preprocessing
				preprocessInput = a.gameplay.DrawFramebuffer()
			} else {
				// Non-xBR path: draw scaled game to buffer
				a.gameplay.Draw(buffer)
				preprocessInput = buffer
			}
		default:
			// UI: draw to buffer (xBR has no effect on UI)
			a.ui.Draw(buffer)
			preprocessInput = buffer
		}

		// Apply preprocessing effects (xBR, ghosting)
		// xBR scales native -> screen size; ghosting operates at screen size
		// Returns processed image (screen-sized) and remaining shader IDs
		processed, remainingShaders := a.shaderManager.ApplyPreprocessEffects(
			preprocessInput, shaderIDs, sw, sh)

		// For StatePlaying: draw pause menu and achievement overlay after effects (so shaders apply to them)
		if a.state == StatePlaying {
			a.gameplay.DrawPauseMenu(processed)
			a.gameplay.DrawAchievementOverlay(processed)
		}

		// Notification drawn after effects, before shaders
		a.notification.Draw(processed)
		if a.state == StateLibrary {
			a.searchOverlay.Draw(processed)
		}

		// Apply shader chain to final screen
		a.shaderManager.ApplyShaders(screen, processed, remainingShaders)
	}

	// Take screenshot if pending (after everything is drawn)
	if a.screenshotPending {
		a.screenshotPending = false
		gameCRC := a.gameplay.CurrentGameCRC()
		if err := a.screenshotManager.TakeScreenshot(screen, gameCRC); err != nil {
			log.Printf("Screenshot failed: %v", err)
		}
	}
}

// Layout implements ebiten.Game
func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	// Query the device scale factor for HiDPI/Retina rendering
	s := 1.0
	if m := ebiten.Monitor(); m != nil {
		s = m.DeviceScaleFactor()
	}
	if s != a.currentDPIScale {
		a.currentDPIScale = s
		style.SetDPIScale(s)
		a.rebuildPending = true
	}

	// Return physical pixel dimensions so the game renders at full resolution
	w := int(float64(outsideWidth) * s)
	h := int(float64(outsideHeight) * s)
	a.windowWidth = w
	a.windowHeight = h
	// Track windowed dimensions separately so fullscreen doesn't overwrite them.
	// These are used by saveWindowState to persist the pre-fullscreen size.
	if !ebiten.IsFullscreen() {
		a.lastWindowedWidth = w
		a.lastWindowedHeight = h
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
	a.previousState = a.state
	a.state = StateLibrary
	a.libraryScreen.OnEnter()
	a.rebuildCurrentScreen()
	// Focus restoration is handled by the Update loop on the next frame
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
	a.previousState = a.state
	a.state = StateSettings
	a.settingsScreen.OnEnter()
	a.rebuildCurrentScreen()
}

// SwitchToScanProgress transitions to the scan progress screen
func (a *App) SwitchToScanProgress(rescanAll bool) {
	a.notification.Clear()
	a.previousState = a.state
	a.state = StateScanProgress

	// Start scan via manager
	a.scanManager.Start(rescanAll)
	a.scanScreen.OnEnter()
	a.rebuildCurrentScreen()
}

// LaunchGame starts the emulator with the specified game
func (a *App) LaunchGame(gameCRC string, resume bool) {
	// Reset shader buffers to avoid stale data from previous game
	a.shaderManager.ResetBuffers()

	if a.gameplay.Launch(gameCRC, resume) {
		a.previousState = a.state
		a.state = StatePlaying
	}
}

// Exit closes the application
func (a *App) Exit() {
	// Save window state before exiting
	a.saveWindowState()

	// Clean up achievement manager resources
	if a.achievementManager != nil {
		a.achievementManager.Destroy()
	}

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
	a.rebuildPending = true
}

// GetPlaceholderImageData returns the raw embedded placeholder image data
func (a *App) GetPlaceholderImageData() []byte {
	return placeholderImageData
}

// GetRDB returns the RDB for metadata lookups
func (a *App) GetRDB() *rdb.RDB {
	if a.metadata == nil {
		return nil
	}
	return a.metadata.GetRDB()
}

// GetExtensions returns the supported ROM file extensions
func (a *App) GetExtensions() []string {
	return a.systemInfo.Extensions
}

// handleDeleteAndContinue handles the delete and continue button
func (a *App) handleDeleteAndContinue() {
	var err error

	if a.errorFile == "config.json" {
		if err = storage.DeleteConfig(); err != nil {
			log.Printf("Failed to delete config: %v", err)
		}
		a.config = storage.DefaultConfig()
		if err = storage.SaveConfig(a.config); err != nil {
			log.Printf("Failed to save config: %v", err)
		}

		// Now try loading library
		library, err := storage.LoadLibrary()
		if err != nil {
			// Library is also corrupt
			libraryPath, _ := storage.GetLibraryPath()
			a.errorFile = "library.json"
			a.errorPath = libraryPath
			a.errorScreen.SetError("library.json", libraryPath)
			a.rebuildCurrentScreen()
			return
		}
		// Validate library-level fields
		libraryErrors := storage.ValidateLibrary(library)
		if len(libraryErrors) > 0 {
			libraryPath, _ := storage.GetLibraryPath()
			a.errorFile = "library.json"
			a.errorPath = libraryPath
			a.library = library
			a.errorScreen.SetValidationError("library.json", libraryPath, libraryErrors, a.handleLibraryResetAndContinue)
			a.rebuildCurrentScreen()
			return
		}
		a.library = library
	} else if a.errorFile == "library.json" {
		if err = storage.DeleteLibrary(); err != nil {
			log.Printf("Failed to delete library: %v", err)
		}
		a.library = storage.DefaultLibrary()
		if err = storage.SaveLibrary(a.library); err != nil {
			log.Printf("Failed to save library: %v", err)
		}
	}

	// Update save state manager with new library
	a.saveStateManager.SetLibrary(a.library)

	// Initialize or update gameplay manager
	if a.gameplay == nil {
		a.gameplay = NewGameplayManager(
			a.factory,
			a.systemInfo,
			a.saveStateManager,
			a.screenshotManager,
			a.notification,
			a.library,
			a.config,
			a.achievementManager,
			a.metadata.GetRDB(),
			func() { a.SwitchToLibrary() },
			func() { a.Exit() },
		)
	} else {
		a.gameplay.SetLibrary(a.library)
		a.gameplay.SetConfig(a.config)
	}

	// Reinitialize screens with fresh data
	a.initScreens()

	// Initialize or update scan manager (needs scanScreen reference)
	if a.scanManager == nil {
		a.scanManager = NewScanManager(
			a.library,
			a.scanScreen,
			a.systemInfo.Extensions,
			a.systemInfo.RDBName,
			a.systemInfo.ThumbnailRepo,
			func() { a.rebuildCurrentScreen() },
			func(msg string) {
				a.state = StateSettings
				a.rebuildCurrentScreen()
				if msg != "" {
					a.notification.ShowDefault(msg)
				}
			},
		)
	} else {
		a.scanManager.SetLibrary(a.library)
		a.scanManager.SetScanScreen(a.scanScreen)
	}

	// Proceed to library screen
	a.state = StateLibrary
	a.rebuildCurrentScreen()
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

	// Apply corrected theme and font
	style.ApplyThemeByName(a.config.Theme)
	style.ApplyFontSize(storage.ValidFontSize(a.config.FontSize))

	a.configLoadFailed = false

	// Load library
	library, err := storage.LoadLibrary()
	if err != nil {
		// Library is also corrupt
		libraryPath, _ := storage.GetLibraryPath()
		a.errorFile = "library.json"
		a.errorPath = libraryPath
		a.errorScreen.SetError("library.json", libraryPath)
		a.rebuildCurrentScreen()
		return
	}
	// Validate library-level fields
	libraryErrors := storage.ValidateLibrary(library)
	if len(libraryErrors) > 0 {
		libraryPath, _ := storage.GetLibraryPath()
		a.errorFile = "library.json"
		a.errorPath = libraryPath
		a.library = library
		a.errorScreen.SetValidationError("library.json", libraryPath, libraryErrors, a.handleLibraryResetAndContinue)
		a.rebuildCurrentScreen()
		return
	}
	a.library = library

	// Update save state manager with library
	a.saveStateManager.SetLibrary(a.library)

	// Initialize or update gameplay manager
	if a.gameplay == nil {
		a.gameplay = NewGameplayManager(
			a.factory,
			a.systemInfo,
			a.saveStateManager,
			a.screenshotManager,
			a.notification,
			a.library,
			a.config,
			a.achievementManager,
			a.metadata.GetRDB(),
			func() { a.SwitchToLibrary() },
			func() { a.Exit() },
		)
	} else {
		a.gameplay.SetLibrary(a.library)
		a.gameplay.SetConfig(a.config)
	}

	// Reinitialize screens with corrected config
	a.initScreens()

	// Initialize or update scan manager
	if a.scanManager == nil {
		a.scanManager = NewScanManager(
			a.library,
			a.scanScreen,
			a.systemInfo.Extensions,
			a.systemInfo.RDBName,
			a.systemInfo.ThumbnailRepo,
			func() { a.rebuildCurrentScreen() },
			func(msg string) {
				a.state = StateSettings
				a.rebuildCurrentScreen()
				if msg != "" {
					a.notification.ShowDefault(msg)
				}
			},
		)
	} else {
		a.scanManager.SetLibrary(a.library)
		a.scanManager.SetScanScreen(a.scanScreen)
	}

	// Proceed to library screen
	a.state = StateLibrary
	a.rebuildCurrentScreen()
}

// handleLibraryResetAndContinue handles the reset and continue button for
// library validation errors. It corrects invalid library-level fields, saves,
// and proceeds to the library screen.
func (a *App) handleLibraryResetAndContinue() {
	storage.CorrectLibrary(a.library)

	if err := storage.SaveLibrary(a.library); err != nil {
		log.Printf("Failed to save corrected library: %v", err)
	}

	a.configLoadFailed = false

	// Update save state manager with library
	a.saveStateManager.SetLibrary(a.library)

	// Initialize or update gameplay manager
	if a.gameplay == nil {
		a.gameplay = NewGameplayManager(
			a.factory,
			a.systemInfo,
			a.saveStateManager,
			a.screenshotManager,
			a.notification,
			a.library,
			a.config,
			a.achievementManager,
			a.metadata.GetRDB(),
			func() { a.SwitchToLibrary() },
			func() { a.Exit() },
		)
	} else {
		a.gameplay.SetLibrary(a.library)
	}

	// Reinitialize screens
	a.initScreens()

	// Initialize or update scan manager
	if a.scanManager == nil {
		a.scanManager = NewScanManager(
			a.library,
			a.scanScreen,
			a.systemInfo.Extensions,
			a.systemInfo.RDBName,
			a.systemInfo.ThumbnailRepo,
			func() { a.rebuildCurrentScreen() },
			func(msg string) {
				a.state = StateSettings
				a.rebuildCurrentScreen()
				if msg != "" {
					a.notification.ShowDefault(msg)
				}
			},
		)
	} else {
		a.scanManager.SetLibrary(a.library)
		a.scanManager.SetScanScreen(a.scanScreen)
	}

	// Proceed to library screen
	a.state = StateLibrary
	a.rebuildCurrentScreen()
}

// SaveAndClose saves config and library before exit
func (a *App) SaveAndClose() {
	// Capture current window state before saving
	a.saveWindowState()

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
		return a.config.Shaders.GameShaders
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
	a.shaderBuffer = ebiten.NewImage(width, height)
	return a.shaderBuffer
}
