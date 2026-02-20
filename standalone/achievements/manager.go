//go:build !libretro && !ios

package achievements

import (
	"fmt"
	"image"
	_ "image/png" // PNG decoder for badge images
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/go-rcheevos"
)

// Notification interface for showing achievement popups
type Notification interface {
	ShowDefault(message string)
	ShowAchievementWithBadge(title, description string, badge *ebiten.Image)
	SetBadge(badge *ebiten.Image) // Update badge after async fetch
	PlaySound(soundData []byte)   // Play sound through notification audio stream
}

// ScreenshotFunc is called when an achievement triggers to capture a screenshot
type ScreenshotFunc func()

// Manager wraps the rcheevos client for RetroAchievements integration
type Manager struct {
	client       *rcheevos.Client
	httpClient   *http.Client
	notification Notification
	userAgent    string // Cached User-Agent string
	config       *storage.Config
	consoleID    uint32

	// State
	mu         sync.Mutex
	emulator   EmulatorInterface
	loggedIn   bool
	username   string
	token      string
	gameLoaded bool

	// Callbacks for unlock events
	screenshotFunc ScreenshotFunc
	onUnlockFunc   func(achievementID uint32) // Called when achievement unlocks

	// Progress dirty flag (set when achievements unlock, cleared when detail screen refreshes)
	progressDirty bool

	// Unlock sound
	unlockSoundData []byte

	// Badge cache (gameID<<32 | achievementID -> image)
	badgeCache map[uint64]*ebiten.Image
	// Game image cache (gameID -> image)
	gameImageCache map[uint32]*ebiten.Image

	// Cached achievements for the current game session (populated on LoadGame)
	cachedAchievements []*rcheevos.Achievement
	cachedGameTitle    string

	// Library data for achievement viewing (pre-fetched)
	hashLibraryMap   map[string]uint32                      // MD5 hash -> gameID lookup
	userProgressMap  map[uint32]*rcheevos.UserProgressEntry // gameID -> progress
	librariesLoaded  bool
	librariesLoading bool
}

// NewManager creates a new achievement manager
func NewManager(notification Notification, config *storage.Config, appName, appVersion string, consoleID uint32) *Manager {
	m := &Manager{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		notification:    notification,
		config:          config,
		consoleID:       consoleID,
		unlockSoundData: generateUnlockSound(),
		badgeCache:      make(map[uint64]*ebiten.Image),
		gameImageCache:  make(map[uint32]*ebiten.Image),
	}

	// Create rcheevos client with memory and server callbacks
	m.client = rcheevos.NewClient(m.readMemory, m.serverCall)

	// Build User-Agent string: "AppName/Version rcheevos/X.X.X"
	rcheevosUA := m.client.GetUserAgentClause()
	m.userAgent = fmt.Sprintf("%s/%s %s", appName, appVersion, rcheevosUA)

	// Set up event handler
	m.client.SetEventHandler(m.handleEvent)

	return m
}

// IsLoggedIn returns whether a user is currently logged in
func (m *Manager) IsLoggedIn() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loggedIn
}

// IsEnabled returns whether achievements are enabled in config
func (m *Manager) IsEnabled() bool {
	return m.config.RetroAchievements.Enabled
}

// IsSpectatorMode returns whether spectator mode is enabled in config
func (m *Manager) IsSpectatorMode() bool {
	return m.config.RetroAchievements.SpectatorMode
}

// SetScreenshotFunc sets the callback for taking screenshots on achievement unlock
func (m *Manager) SetScreenshotFunc(fn ScreenshotFunc) {
	m.screenshotFunc = fn
}

// SetOnUnlockCallback sets a callback that's called when an achievement is unlocked.
// The callback receives the achievement ID. Used by the overlay to update its display.
func (m *Manager) SetOnUnlockCallback(fn func(achievementID uint32)) {
	m.mu.Lock()
	m.onUnlockFunc = fn
	m.mu.Unlock()
}

// IsProgressDirty returns true if achievements were unlocked since the last check.
// Used by the detail screen to know when to refresh cached progress data.
func (m *Manager) IsProgressDirty() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.progressDirty
}

// ClearProgressDirty clears the dirty flag after the detail screen refreshes.
func (m *Manager) ClearProgressDirty() {
	m.mu.Lock()
	m.progressDirty = false
	m.mu.Unlock()
}

// GetUsername returns the logged in username
func (m *Manager) GetUsername() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.username
}

// Login authenticates with RetroAchievements using username and password
func (m *Manager) Login(username, password string, callback func(success bool, token string, err error)) {
	m.client.LoginWithPassword(username, password, func(result int, errorMessage string) {
		if result != rcheevos.OK {
			callback(false, "", fmt.Errorf("login failed: %s", errorMessage))
			return
		}

		user := m.client.GetUser()
		if user == nil {
			callback(false, "", fmt.Errorf("login succeeded but user info unavailable"))
			return
		}

		m.mu.Lock()
		m.loggedIn = true
		m.username = user.Username
		m.token = user.Token
		m.mu.Unlock()

		callback(true, user.Token, nil)
	})
}

// LoginWithToken authenticates with RetroAchievements using a stored token.
// The result code is passed through so callers can distinguish credential
// errors (e.g. ExpiredToken) from network errors (e.g. NoResponse).
func (m *Manager) LoginWithToken(username, token string, callback func(success bool, result int, err error)) {
	m.client.LoginWithToken(username, token, func(result int, errorMessage string) {
		if result != rcheevos.OK {
			callback(false, result, fmt.Errorf("token login failed: %s", errorMessage))
			return
		}

		user := m.client.GetUser()
		if user == nil {
			callback(false, rcheevos.OK, fmt.Errorf("login succeeded but user info unavailable"))
			return
		}

		m.mu.Lock()
		m.loggedIn = true
		m.username = user.Username
		m.token = user.Token
		m.mu.Unlock()

		callback(true, rcheevos.OK, nil)
	})
}

// Logout logs out the current user and clears all caches
func (m *Manager) Logout() {
	m.client.Logout()

	m.mu.Lock()
	m.loggedIn = false
	m.username = ""
	m.token = ""
	m.gameLoaded = false
	m.cachedAchievements = nil
	m.cachedGameTitle = ""
	m.progressDirty = false
	m.badgeCache = make(map[uint64]*ebiten.Image)
	m.gameImageCache = make(map[uint32]*ebiten.Image)
	m.hashLibraryMap = nil
	m.userProgressMap = nil
	m.librariesLoaded = false
	m.mu.Unlock()
}

// SetEmulator sets the emulator for memory access
func (m *Manager) SetEmulator(emu EmulatorInterface) {
	m.emulator = emu
}

// LoadGame identifies and loads a game for achievement tracking.
// If md5Hash is provided and non-empty, it will be used directly (fast path).
// Otherwise, the hash will be computed from romData (fallback).
func (m *Manager) LoadGame(romData []byte, filePath string, md5Hash string) error {
	m.mu.Lock()
	if !m.loggedIn {
		m.mu.Unlock()
		return fmt.Errorf("not logged in")
	}
	m.mu.Unlock()

	// Apply client settings from config before loading
	m.client.SetEncoreModeEnabled(m.config.RetroAchievements.EncoreMode)
	if m.config.RetroAchievements.SpectatorMode {
		m.client.SetSpectatorModeEnabled(true)
	}

	// Use a channel to capture the async result
	done := make(chan error, 1)

	loadCallback := func(result int, errorMessage string) {
		if result != rcheevos.OK {
			done <- fmt.Errorf("failed to load game: %s", errorMessage)
			return
		}

		m.mu.Lock()
		m.gameLoaded = true
		m.mu.Unlock()

		// Cache achievements for this session
		m.cacheAchievements()

		done <- nil
	}

	// Use hash directly if provided, otherwise identify from ROM
	if md5Hash != "" {
		m.client.LoadGame(md5Hash, loadCallback)
	} else {
		m.client.IdentifyAndLoadGame(m.consoleID, filePath, romData, loadCallback)
	}

	// Wait for the callback with a timeout
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		return fmt.Errorf("game load timed out")
	}
}

// badgeCacheKey creates a composite cache key from game ID and achievement ID
func badgeCacheKey(gameID, achievementID uint32) uint64 {
	return (uint64(gameID) << 32) | uint64(achievementID)
}

// getBadge returns a cached badge or fetches it on-demand
func (m *Manager) getBadge(achievementID uint32, url string) *ebiten.Image {
	if url == "" {
		return nil
	}

	game := m.client.GetGame()
	if game == nil {
		return nil
	}
	cacheKey := badgeCacheKey(game.ID, achievementID)

	// Check cache first
	m.mu.Lock()
	if img, ok := m.badgeCache[cacheKey]; ok {
		m.mu.Unlock()
		return img
	}
	m.mu.Unlock()

	// Fetch on-demand
	img := m.fetchImage(url)
	if img != nil {
		m.mu.Lock()
		m.badgeCache[cacheKey] = img
		m.mu.Unlock()
	}
	return img
}

// getGameImage returns the cached game image or fetches it on-demand
func (m *Manager) getGameImage() *ebiten.Image {
	game := m.client.GetGame()
	if game == nil {
		return nil
	}

	// Check cache first
	m.mu.Lock()
	if img, ok := m.gameImageCache[game.ID]; ok {
		m.mu.Unlock()
		return img
	}
	m.mu.Unlock()

	url := m.client.GetGameImageURL()
	if url == "" {
		return nil
	}

	img := m.fetchImage(url)
	if img != nil {
		m.mu.Lock()
		m.gameImageCache[game.ID] = img
		m.mu.Unlock()
	}
	return img
}

// fetchImage downloads an image from a URL and returns an ebiten.Image
func (m *Manager) fetchImage(url string) *ebiten.Image {
	resp, err := m.httpClient.Get(url)
	if err != nil {
		log.Printf("[RetroAchievements] Failed to fetch image: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[RetroAchievements] Image fetch returned status %d", resp.StatusCode)
		return nil
	}

	// Limit read to 1MB
	limitedReader := io.LimitReader(resp.Body, 1024*1024)
	img, _, err := image.Decode(limitedReader)
	if err != nil {
		log.Printf("[RetroAchievements] Failed to decode image: %v", err)
		return nil
	}

	return ebiten.NewImageFromImage(img)
}

// DoFrame processes achievements for the current frame
func (m *Manager) DoFrame() {
	if !m.config.RetroAchievements.Enabled {
		return
	}

	m.mu.Lock()
	loggedIn := m.loggedIn
	gameLoaded := m.gameLoaded
	m.mu.Unlock()

	if !loggedIn || !gameLoaded {
		return
	}

	m.client.DoFrame()
}

// Idle processes periodic tasks when paused
func (m *Manager) Idle() {
	m.mu.Lock()
	loggedIn := m.loggedIn
	m.mu.Unlock()

	if !loggedIn {
		return
	}

	m.client.Idle()
}

// UnloadGame unloads the current game
func (m *Manager) UnloadGame() {
	m.mu.Lock()
	wasLoaded := m.gameLoaded
	m.gameLoaded = false
	m.emulator = nil
	m.cachedAchievements = nil
	m.cachedGameTitle = ""
	m.mu.Unlock()

	if wasLoaded {
		m.client.UnloadGame()
		// Disable spectator mode after unload so it doesn't persist to next session
		m.client.SetSpectatorModeEnabled(false)
	}
}

// Destroy cleans up the client resources
func (m *Manager) Destroy() {
	m.client.Destroy()
}

// readMemory is the memory callback for rcheevos.
// Delegates to the core adapter's ReadMemory which handles mapping
// flat addresses to internal memory regions.
func (m *Manager) readMemory(address uint32, buffer []byte) uint32 {
	if m.emulator == nil {
		return 0
	}
	return m.emulator.ReadMemory(address, buffer)
}

// serverCall handles HTTP requests to the RetroAchievements API
func (m *Manager) serverCall(request *rcheevos.ServerRequest) {
	go func() {
		var resp *http.Response
		var err error

		// Create request with User-Agent header
		var req *http.Request
		if request.PostData != "" {
			// POST request
			req, err = http.NewRequest("POST", request.URL, strings.NewReader(request.PostData))
			if err == nil {
				req.Header.Set("Content-Type", request.ContentType)
			}
		} else {
			// GET request
			req, err = http.NewRequest("GET", request.URL, nil)
		}

		if err != nil {
			log.Printf("[RetroAchievements] Failed to create request: %v", err)
			request.Respond(nil, 0)
			return
		}

		req.Header.Set("User-Agent", m.userAgent)
		resp, err = m.httpClient.Do(req)

		if err != nil {
			log.Printf("[RetroAchievements] HTTP error: %v", err)
			request.Respond(nil, 0)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[RetroAchievements] Read error: %v", err)
			request.Respond(nil, resp.StatusCode)
			return
		}

		request.Respond(body, resp.StatusCode)
	}()
}

// handleEvent processes achievement events
func (m *Manager) handleEvent(event *rcheevos.Event) {
	switch event.Type {
	case rcheevos.EventAchievementTriggered:
		if event.Achievement == nil || m.notification == nil {
			return
		}

		// Copy values we need (event may become invalid after handler returns)
		title := event.Achievement.Title
		description := event.Achievement.Description
		achievementID := event.Achievement.ID
		badgeURL := m.client.GetAchievementImageURL(event.Achievement, rcheevos.AchievementStateUnlocked)

		// Check if this is the hardcore warning and should be suppressed
		isHardcoreWarning := strings.Contains(title, "Unknown Emulator") ||
			strings.Contains(description, "Hardcore unlocks cannot be earned")
		if m.config.RetroAchievements.SuppressHardcoreWarning && isHardcoreWarning {
			return
		}

		// Mark progress as dirty, update cache, and notify callback (for real achievements only)
		if !isHardcoreWarning {
			m.mu.Lock()
			m.progressDirty = true
			onUnlock := m.onUnlockFunc

			// Update cached achievement to mark as unlocked and move to end of list
			m.updateCachedAchievementUnlocked(achievementID)
			m.mu.Unlock()

			if onUnlock != nil {
				onUnlock(achievementID)
			}
		}

		// Get cached badge
		game := m.client.GetGame()
		var cachedBadge *ebiten.Image
		if game != nil {
			m.mu.Lock()
			cachedBadge = m.badgeCache[badgeCacheKey(game.ID, achievementID)]
			m.mu.Unlock()
		}

		// Play unlock sound
		if m.config.RetroAchievements.UnlockSound && len(m.unlockSoundData) > 0 {
			m.notification.PlaySound(m.unlockSoundData)
		}

		// Take screenshot
		if m.config.RetroAchievements.AutoScreenshot && m.screenshotFunc != nil {
			m.screenshotFunc()
		}

		// Show notification
		if m.config.RetroAchievements.ShowNotification {
			if cachedBadge != nil {
				m.notification.ShowAchievementWithBadge(title, description, cachedBadge)
			} else {
				// Show notification immediately without badge, fetch async
				m.notification.ShowAchievementWithBadge(title, description, nil)

				// Fetch badge in background and update notification
				go func() {
					badge := m.getBadge(achievementID, badgeURL)
					if badge != nil {
						m.notification.SetBadge(badge)
					}
				}()
			}
		}
	case rcheevos.EventGameCompleted:
		if m.notification != nil && m.config.RetroAchievements.ShowNotification {
			// Check cache first
			game := m.client.GetGame()
			var cachedImg *ebiten.Image
			if game != nil {
				m.mu.Lock()
				cachedImg = m.gameImageCache[game.ID]
				m.mu.Unlock()
			}

			if cachedImg != nil {
				m.notification.ShowAchievementWithBadge("Game Mastered!", "All achievements unlocked", cachedImg)
			} else {
				// Show immediately, fetch image async
				m.notification.ShowAchievementWithBadge("Game Mastered!", "All achievements unlocked", nil)
				go func() {
					gameImg := m.getGameImage()
					if gameImg != nil {
						m.notification.SetBadge(gameImg)
					}
				}()
			}
		}
	case rcheevos.EventServerError:
		if event.ServerError != nil {
			log.Printf("[RetroAchievements] Server error: %s", event.ServerError.ErrorMessage)
		}
	}
}

// --- Achievement Viewing API ---

// IsGameLoaded returns whether a game is currently loaded for achievements
func (m *Manager) IsGameLoaded() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.gameLoaded
}

// cacheAchievements fetches and caches achievements for the current game session.
// Called internally after LoadGame succeeds.
func (m *Manager) cacheAchievements() {
	// Clear old cache first
	m.mu.Lock()
	m.cachedAchievements = nil
	m.cachedGameTitle = ""
	m.mu.Unlock()

	list := m.client.CreateAchievementList(rcheevos.AchievementCategoryCore, rcheevos.AchievementListGroupingLockState)
	if list == nil {
		return
	}
	defer list.Destroy()

	allAchievements := list.GetAllAchievements()

	// Filter out 0-point achievements (warnings like "Unknown Emulator")
	// and sort: locked first, unlocked at bottom
	var locked []*rcheevos.Achievement
	var unlocked []*rcheevos.Achievement
	for _, ach := range allAchievements {
		if ach.Points == 0 {
			continue
		}
		if ach.Unlocked != rcheevos.AchievementUnlockedNone {
			unlocked = append(unlocked, ach)
		} else {
			locked = append(locked, ach)
		}
	}

	m.mu.Lock()
	m.cachedAchievements = append(locked, unlocked...)
	if game := m.client.GetGame(); game != nil {
		m.cachedGameTitle = game.Title
	}
	m.mu.Unlock()
}

// GetCachedAchievements returns the cached achievement list for the current game.
// Returns nil if no game is loaded. The slice is safe to read; do not modify.
func (m *Manager) GetCachedAchievements() []*rcheevos.Achievement {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cachedAchievements
}

// GetCachedGameTitle returns the cached game title.
func (m *Manager) GetCachedGameTitle() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cachedGameTitle
}

// updateCachedAchievementUnlocked marks an achievement as unlocked in the cache
// and moves it to the end of the list (unlocked section).
// Must be called while holding m.mu.
func (m *Manager) updateCachedAchievementUnlocked(achievementID uint32) {
	for i, ach := range m.cachedAchievements {
		if ach.ID == achievementID {
			// Mark as unlocked
			ach.Unlocked = rcheevos.AchievementUnlockedSoftcore

			// Move to end of list (unlocked section)
			m.cachedAchievements = append(m.cachedAchievements[:i], m.cachedAchievements[i+1:]...)
			m.cachedAchievements = append(m.cachedAchievements, ach)
			return
		}
	}
}

// GetGame returns information about the currently loaded game.
// Returns nil if no game is loaded.
func (m *Manager) GetGame() *rcheevos.Game {
	return m.client.GetGame()
}

// GetBadgeImage returns a cached badge image for an achievement.
// Always returns the colored (unlocked) version - caller applies grayscale if needed.
// Returns nil if the badge is not yet cached.
func (m *Manager) GetBadgeImage(achievementID uint32) *ebiten.Image {
	game := m.client.GetGame()
	if game == nil {
		return nil
	}

	cacheKey := badgeCacheKey(game.ID, achievementID)
	m.mu.Lock()
	img := m.badgeCache[cacheKey]
	m.mu.Unlock()
	return img
}

// GetBadgeImageAsync fetches a badge image asynchronously.
// Always fetches the colored (unlocked) version - caller applies grayscale if needed.
// The callback is called when the image is ready (may be nil on error).
func (m *Manager) GetBadgeImageAsync(achievementID uint32, callback func(*ebiten.Image)) {
	ach := m.client.GetAchievement(achievementID)
	if ach == nil {
		if callback != nil {
			callback(nil)
		}
		return
	}

	// Always fetch the colored (unlocked) badge - grayscale is applied at display time
	url := m.client.GetAchievementImageURL(ach, rcheevos.AchievementStateUnlocked)

	go func() {
		badge := m.getBadge(achievementID, url)
		if callback != nil {
			callback(badge)
		}
	}()
}

// --- Library Pre-loading API ---

// EnsureLibrariesLoaded fetches the hash library and user progress for SMS if not already cached.
// Call this when logged in to pre-fetch data for the detail screen.
func (m *Manager) EnsureLibrariesLoaded(callback func(success bool)) {
	m.mu.Lock()
	if m.librariesLoaded {
		m.mu.Unlock()
		if callback != nil {
			callback(true)
		}
		return
	}
	if m.librariesLoading {
		m.mu.Unlock()
		// Another goroutine is loading - wait for it
		go func() {
			for {
				m.mu.Lock()
				if !m.librariesLoading {
					loaded := m.librariesLoaded
					m.mu.Unlock()
					if callback != nil {
						callback(loaded)
					}
					return
				}
				m.mu.Unlock()
				time.Sleep(50 * time.Millisecond)
			}
		}()
		return
	}
	m.librariesLoading = true
	m.mu.Unlock()

	var wg sync.WaitGroup
	var hashErr, progressErr error

	wg.Add(2)

	// Fetch hash library
	m.client.FetchHashLibrary(m.consoleID, func(result int, errorMessage string, library *rcheevos.HashLibrary) {
		defer wg.Done()
		if result != rcheevos.OK {
			hashErr = fmt.Errorf("fetch hash library: %s", errorMessage)
			return
		}
		if library != nil {
			m.mu.Lock()
			m.hashLibraryMap = make(map[string]uint32, len(library.Entries))
			for _, entry := range library.Entries {
				m.hashLibraryMap[entry.Hash] = entry.GameID
			}
			m.mu.Unlock()
		}
	})

	// Fetch user progress
	m.client.FetchAllUserProgress(m.consoleID, func(result int, errorMessage string, progress *rcheevos.AllUserProgress) {
		defer wg.Done()
		if result != rcheevos.OK {
			progressErr = fmt.Errorf("fetch user progress: %s", errorMessage)
			return
		}
		if progress != nil {
			m.mu.Lock()
			m.userProgressMap = make(map[uint32]*rcheevos.UserProgressEntry, len(progress.Entries))
			for _, entry := range progress.Entries {
				m.userProgressMap[entry.GameID] = entry
			}
			m.mu.Unlock()
		}
	})

	// Wait for both to complete
	go func() {
		wg.Wait()

		m.mu.Lock()
		m.librariesLoading = false
		if hashErr == nil && progressErr == nil {
			m.librariesLoaded = true
		} else {
			if hashErr != nil {
				log.Printf("[RetroAchievements] %v", hashErr)
			}
			if progressErr != nil {
				log.Printf("[RetroAchievements] %v", progressErr)
			}
		}
		loaded := m.librariesLoaded
		m.mu.Unlock()

		if callback != nil {
			callback(loaded)
		}
	}()
}

// RefreshUserProgress re-fetches user progress data from the server.
// Call this after achievements are unlocked to update cached progress.
func (m *Manager) RefreshUserProgress(callback func(success bool)) {
	m.client.FetchAllUserProgress(m.consoleID, func(result int, errorMessage string, progress *rcheevos.AllUserProgress) {
		if result != rcheevos.OK {
			log.Printf("[RetroAchievements] refresh user progress: %s", errorMessage)
			if callback != nil {
				callback(false)
			}
			return
		}
		if progress != nil {
			m.mu.Lock()
			m.userProgressMap = make(map[uint32]*rcheevos.UserProgressEntry, len(progress.Entries))
			for _, entry := range progress.Entries {
				m.userProgressMap[entry.GameID] = entry
			}
			m.mu.Unlock()
		}
		if callback != nil {
			callback(true)
		}
	})
}

// LookupGameProgress returns user progress for a game by MD5 hash.
// Returns (false, nil) if the game is not in the RetroAchievements database.
// Returns (true, nil) if the game exists but user has no progress.
// Returns (true, progress) if the game exists and user has progress.
func (m *Manager) LookupGameProgress(md5Hash string) (found bool, progress *rcheevos.UserProgressEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.hashLibraryMap == nil {
		return false, nil
	}

	gameID, ok := m.hashLibraryMap[strings.ToLower(md5Hash)]
	if !ok {
		return false, nil
	}

	if m.userProgressMap == nil {
		return true, nil
	}

	return true, m.userProgressMap[gameID]
}

// ComputeGameHash generates the MD5 hash for ROM data in rcheevos format.
// Use this as a fallback when the MD5 is not in the RDB.
func (m *Manager) ComputeGameHash(romData []byte) string {
	return rcheevos.HashFromBuffer(m.consoleID, romData)
}
