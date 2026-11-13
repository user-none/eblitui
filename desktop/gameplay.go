// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/achievements"
	"github.com/user-none/eblitui/desktop/cheats"
	"github.com/user-none/eblitui/desktop/content"
	"github.com/user-none/eblitui/desktop/shader"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/romloader"
	"github.com/user-none/eblitui/rumble"
)

// GameplayManager handles all gameplay-related state and logic.
// This includes emulator control, input handling, save states,
// play time tracking, and the pause menu.
//
// The emulator runs on a dedicated goroutine paced by the framePacer:
// an absolute-deadline timer at the frame interval, slowly corrected from
// the audio ring fill so long-term rate stays locked to the device. The
// Ebiten thread handles UI, input polling, and reads the shared framebuffer.
type GameplayManager struct {
	// Core factory and system info
	factory    coreif.CoreFactory
	systemInfo coreif.SystemInfo
	mappings   [maxPlayers]InputMapping // per-player input mappings

	// Emulation state
	emulator        coreif.Emulator
	saveStater      coreif.SaveStater         // Detected at launch (may be nil)
	batterySaver    coreif.BatterySaver       // Detected at launch (may be nil)
	aspectProvider  coreif.AspectProvider     // Detected at launch (may be nil)
	renderInfo      coreif.RenderInfoProvider // Detected at launch (may be nil)
	memoryInspector coreif.Memory             // Detected at launch (may be nil)
	renderer        *FramebufferRenderer
	audioPlayer     *AudioPlayer
	framePacer      *framePacer
	currentGame     *storage.GameEntry
	disc            *romloader.Disc // open streaming disc (disc-based systems)
	gameShaders     []string        // game shaders of the running game, its overrides applied

	// The launch in progress, nil once the game runs or the launch has
	// failed. Ebiten thread only.
	pending *gameLaunch

	// Emulation goroutine control
	emuControl        *EmuControl
	sharedInput       *SharedInput
	sharedFramebuffer *SharedFramebuffer
	emuDone           chan struct{}

	// Cached auto-save state (written by emu goroutine, read by Ebiten thread)
	autoSaveState   []byte
	autoSaveStateMu sync.Mutex
	autoSaveReady   bool

	// Rewind
	rewindBuffer *RewindBuffer

	// Pause menu
	pauseMenu *PauseMenu

	// Set when resuming from the pause menu; suppresses all game input until
	// every held input is released, so nothing from the menu interaction leaks
	// into the game on the following frames.
	resumeInputGuard bool

	// Achievement overlay
	achievementOverlay *AchievementOverlay

	// Play time tracking
	playTime PlayTimeTracker

	// Auto-save state
	autoSaveInterval time.Duration
	autoSaving       atomic.Bool // set while the auto-save goroutine writes
	autoSaveWg       sync.WaitGroup

	// Rumble. rumbleSource supplies the .erumble files (nil when none);
	// rumbleEngine serves the game's file.
	rumbleSource     content.RumbleSource
	rumbleEngine     *rumble.Engine
	pendingMotor     []rumble.MotorState
	pendingMotorSet  bool
	pendingRumbleMu  sync.Mutex
	rumbleActivePads map[int]bool // Ebiten thread only: players vibrating last frame

	// Cheats. cheatSystem is the emulator's cheat system (0 when cheats are
	// off); cheatsSource supplies the database cheats (nil when none);
	// cheatEngine applies the game's enabled cheats each frame, nil when
	// none are on.
	cheatSystem  cheats.System
	cheatsSource content.CheatsSource
	cheatEngine  *CheatEngine

	// raConsoleID is the emulator's RetroAchievements console ID, used for
	// games whose library entry has none
	raConsoleID int

	// Turbo (fast-forward)
	turboState    *TurboState
	turboAudioBuf []int16 // Pre-allocated buffer for collecting multi-frame audio

	// Performance overlay: mode persists across games, statistics
	// reset per game and are fed by the emulation goroutine.
	perfOverlay *PerfOverlay

	// External dependencies (not owned by GameplayManager)
	inputManager       *InputManager
	assignment         *PlayerAssignment
	saveStateManager   *SaveStateManager
	screenshotManager  *ScreenshotManager
	notification       *Notification
	library            *storage.Library
	config             *storage.Config
	achievementManager *achievements.Manager

	// Callbacks to App
	onExitToLibrary func()
	onExitApp       func()
}

// PlayTimeTracker tracks play time during gameplay
type PlayTimeTracker struct {
	sessionSeconds int64
	trackStart     int64
	tracking       bool
}

// NewGameplayManager creates a new gameplay manager
func NewGameplayManager(
	factory coreif.CoreFactory,
	systemInfo coreif.SystemInfo,
	inputManager *InputManager,
	assignment *PlayerAssignment,
	saveStateManager *SaveStateManager,
	screenshotManager *ScreenshotManager,
	notification *Notification,
	library *storage.Library,
	config *storage.Config,
	achievementManager *achievements.Manager,
	cheatSystem cheats.System,
	cheatsSource content.CheatsSource,
	rumbleSource content.RumbleSource,
	raConsoleID int,
	onExitToLibrary func(),
	onExitApp func(),
) *GameplayManager {
	input := config.Values().Input
	gm := &GameplayManager{
		factory:            factory,
		systemInfo:         systemInfo,
		mappings:           buildPlayerMappings(systemInfo, &input),
		autoSaveInterval:   style.AutoSaveInterval,
		turboState:         &TurboState{multiplier: 1},
		perfOverlay:        &PerfOverlay{},
		inputManager:       inputManager,
		assignment:         assignment,
		saveStateManager:   saveStateManager,
		screenshotManager:  screenshotManager,
		notification:       notification,
		library:            library,
		config:             config,
		achievementManager: achievementManager,
		cheatSystem:        cheatSystem,
		cheatsSource:       cheatsSource,
		rumbleSource:       rumbleSource,
		raConsoleID:        raConsoleID,
		onExitToLibrary:    onExitToLibrary,
		onExitApp:          onExitApp,
	}

	// Initialize pause menu with callbacks
	gm.pauseMenu = NewPauseMenu(
		func() { // onResume
			gm.Resume()
		},
		func() { // onLibrary
			gm.Exit(true)
			if gm.onExitToLibrary != nil {
				gm.onExitToLibrary()
			}
		},
		func() { // onExit
			gm.Exit(true)
			if gm.onExitApp != nil {
				gm.onExitApp()
			}
		},
	)

	// Initialize achievement overlay
	gm.achievementOverlay = NewAchievementOverlay(achievementManager)

	return gm
}

// IsPlaying returns true if a game is currently being played
func (gm *GameplayManager) IsPlaying() bool {
	return gm.emulator != nil
}

// CurrentGameID returns the game ID of the currently loaded game, or empty string if none
func (gm *GameplayManager) CurrentGameID() string {
	if gm.currentGame != nil {
		return gm.currentGame.GameID
	}
	return ""
}

// GameShaders returns the game shaders of the running game: the global
// game shaders with the game's overrides applied
func (gm *GameplayManager) GameShaders() []string {
	return gm.gameShaders
}

// gameLaunch is a game start in progress. Opening the game's content,
// starting the emulator and loading the game's achievements can take
// seconds, so they run on the emulation goroutine while the Ebiten thread
// keeps drawing. The Ebiten thread fills in what the goroutine needs and
// starts it. The goroutine prepares the emulator and sends the outcome on
// result, then waits for the answer on begin before it runs any frame. The
// goroutine owns emu, disc, romData and achievements until it has sent on
// result, and the Ebiten thread owns them after.
type gameLaunch struct {
	gameID    string
	game      *storage.GameEntry // gameplay's own copy of the entry
	resume    bool
	discIndex int // index into game.Discs, for a disc game
	cfg       storage.ConfigData
	overrides *storage.OverrideSettings

	emu     coreif.Emulator
	disc    *romloader.Disc // the open disc, for a disc game
	romData []byte          // the ROM, for a cartridge game

	achievements bool // the game is loaded in the achievements manager

	result chan *launchFailure // nil once the emulator is started
	begin  chan bool           // true to run frames, false to exit
}

// launchFailure is why a launch stopped: the message to show and the error
// behind it, when there is one.
type launchFailure struct {
	message string
	err     error
}

// Launch begins starting the emulator with the specified game and reports
// whether it began. The game is not running when it returns: PollLaunch
// reports when it is. discIndex selects which disc of a multi-disc game to
// launch; -1, or any index out of range, uses the entry's remembered
// SelectedDisc. Ignored for single-disc and cartridge games.
func (gm *GameplayManager) Launch(gameID string, resume bool, discIndex int) bool {
	cfg := gm.config.Values()

	// Rebuild input mappings from current config (settings may have changed)
	gm.mappings = buildPlayerMappings(gm.systemInfo, &cfg.Input)

	// The launch keeps gameplay's own copy of the entry; changes to the
	// stored entry go through the library
	entry, ok := gm.library.Game(gameID)
	if !ok {
		gm.notification.ShowDefault("Game not found")
		return false
	}

	l := &gameLaunch{
		gameID:    gameID,
		game:      &entry,
		resume:    resume,
		cfg:       cfg,
		overrides: loadOverrides(gameID),
		result:    make(chan *launchFailure, 1),
		begin:     make(chan bool, 1),
	}
	if gm.systemInfo.Disc {
		// Resolve which disc to load. Disc games carry a Discs list and
		// SelectedDisc is the slice index into it; a discIndex out of
		// range, such as -1, uses the remembered SelectedDisc.
		if len(entry.Discs) == 0 {
			gm.notification.ShowDefault("Failed to load disc")
			return false
		}
		l.discIndex = entry.SelectedDiscIndex()
		if discIndex >= 0 && discIndex < len(entry.Discs) {
			l.discIndex = discIndex
		}
	}

	l.emu = gm.factory.CreateEmulator()
	gm.pending = l
	gm.emuDone = make(chan struct{})
	go gm.emulationLoop(l)
	return true
}

// PollLaunch reports on the launch Launch began. done is set once the launch
// has ended, and ok when it ended with the game running. A failed launch has
// shown its message and released everything it made. Called from the Ebiten
// thread every update until done.
func (gm *GameplayManager) PollLaunch() (done, ok bool) {
	l := gm.pending
	if l == nil {
		return true, false
	}
	select {
	case failure := <-l.result:
		if failure == nil {
			gm.finishLaunch(l)
			return true, true
		}
		// The goroutine exits after reporting a failure.
		<-gm.emuDone
		gm.releaseLaunch(l)
		if failure.err != nil {
			log.Printf("%s: %v", failure.message, failure.err)
		}
		gm.notification.ShowDefault(failure.message)
		return true, false
	default:
		return false, false
	}
}

// releaseLaunch frees what a launch that will not run made. The emulation
// goroutine must have exited.
func (gm *GameplayManager) releaseLaunch(l *gameLaunch) {
	// Unloading also drops the emulator a failed achievements load was
	// given. It does nothing more when no game was loaded.
	if gm.achievementManager != nil {
		gm.achievementManager.UnloadGame()
	}
	if l.disc != nil {
		l.disc.Close()
	}
	l.emu.Close()
	gm.pending = nil
	gm.emuDone = nil
}

// prepareLaunch gives the emulator its content, options and BIOS files,
// starts it, and loads the game's achievements. It runs on the emulation
// goroutine and touches only the launch, the achievements manager, and
// values that do not change while the application runs.
func (gm *GameplayManager) prepareLaunch(l *gameLaunch) *launchFailure {
	emu := l.emu
	cfg := l.cfg

	// Provide content: disc-based systems stream from disk via SetDisc;
	// cartridge systems load the ROM bytes via SetRom.
	if gm.systemInfo.Disc {
		disc, err := romloader.OpenDisc(l.game.Discs[l.discIndex].File)
		if err != nil {
			return &launchFailure{"Failed to load disc", err}
		}
		l.disc = disc
		emu.SetDisc(disc)
	} else {
		data, _, err := romloader.Load(l.game.File, gm.systemInfo.Extensions)
		if err != nil {
			return &launchFailure{"Failed to load ROM", err}
		}
		l.romData = data
		emu.SetRom(data)
	}

	// Apply core options, with the game's overrides of the global settings
	for _, opt := range gm.systemInfo.CoreOptions {
		emu.SetOption(opt.Key, launchCoreOption(opt, cfg.CoreOptions, l.overrides))
	}

	// Load BIOS files
	for _, opt := range gm.systemInfo.BIOSOptions {
		bc, hasCfg := cfg.BIOS[opt.Key]
		if !hasCfg || bc.Active == "" {
			if opt.Required {
				return &launchFailure{"BIOS required: " + opt.Label, nil}
			}
			continue
		}
		filePath := ""
		if bc.Files != nil {
			filePath = bc.Files[bc.Active]
		}
		if filePath == "" {
			if opt.Required {
				return &launchFailure{"BIOS file not set: " + opt.Label, nil}
			}
			continue
		}
		data, err := romloader.LoadBIOS(filePath)
		if err != nil {
			if opt.Required {
				return &launchFailure{"Failed to read BIOS: " + opt.Label, err}
			}
			continue
		}
		// Validate hash against the active variant
		hashValid := true
		for _, v := range opt.Variants {
			if v.Label == bc.Active && v.SHA256 != "" {
				h := sha256.Sum256(data)
				hash := hex.EncodeToString(h[:])
				if hash != v.SHA256 {
					hashValid = false
				}
				break
			}
		}
		if !hashValid {
			if opt.Required {
				return &launchFailure{"BIOS hash mismatch: " + opt.Label, nil}
			}
			continue
		}
		if err := emu.SetBIOS(opt.Key, data); err != nil {
			if opt.Required {
				return &launchFailure{"BIOS rejected: " + opt.Label, err}
			}
			continue
		}
	}

	// Starting the emulator and loading the game's achievements can be the
	// longest steps, so each is skipped when the launch is no longer wanted.
	if l.stopRequested() {
		return &launchFailure{message: "Launch stopped"}
	}
	emu.Start()

	if l.stopRequested() {
		return &launchFailure{message: "Launch stopped"}
	}
	l.achievements = gm.loadAchievements(l)
	return nil
}

// stopRequested reports whether Exit has asked the launch to stop. Exit's
// answer is the only one sent on begin before the outcome is reported.
func (l *gameLaunch) stopRequested() bool {
	select {
	case <-l.begin:
		return true
	default:
		return false
	}
}

// loadAchievements loads the game of a launch into the achievements manager
// when achievements are enabled and the user is logged in, and reports
// whether it loaded. Loading hashes the game's content and waits on the
// server. It runs on the emulation goroutine, after the emulator is started
// and before any frame.
func (gm *GameplayManager) loadAchievements(l *gameLaunch) bool {
	am := gm.achievementManager
	if am == nil || !am.IsEnabled() || !am.IsLoggedIn() {
		return false
	}

	// The screenshot is taken from the final drawn frame, so it carries
	// the unlock notification.
	am.SetScreenshotFunc(func(title string, id uint32) {
		gm.screenshotManager.Queue(achievementTag(title, id))
	})
	if mem, ok := l.emu.(coreif.Memory); ok {
		am.SetEmulator(mem)
	}

	// The game's own console ID, when it has one, overrides the system's
	consoleID := uint32(gm.raConsoleID)
	if l.game.ConsoleID != 0 {
		consoleID = uint32(l.game.ConsoleID)
	}

	// Disc cores hash by reading sectors through the open disc (rcheevos
	// handles the system-specific hashing); cartridge cores hash the ROM.
	var err error
	if gm.systemInfo.Disc {
		err = am.LoadGameFromDisc(l.disc, consoleID)
	} else {
		err = am.LoadGame(l.romData, l.game.File, "", consoleID)
	}
	if err != nil {
		log.Printf("Failed to load achievements: %v", err)
		return false
	}
	return true
}

// finishLaunch takes over the started emulator of a launch, sets up what
// runs the game around it, and lets the emulation goroutine begin running
// frames. It runs on the Ebiten thread.
func (gm *GameplayManager) finishLaunch(l *gameLaunch) {
	emu := l.emu
	game := l.game
	gameID := l.gameID
	cfg := l.cfg
	resume := l.resume

	gm.emulator = emu
	gm.currentGame = game
	gm.saveStateManager.SetGame(gameID)
	gm.disc = l.disc
	if gm.systemInfo.Disc {
		// Persist the chosen disc so it is remembered next launch.
		gm.library.SetSelectedDisc(gameID, l.discIndex)
		game.SelectedDisc = l.discIndex
	}

	// Resolve the game shaders, with the game's overrides of the global
	// settings
	gm.gameShaders = launchGameShaders(cfg.Shaders.GameShaders, l.overrides)

	// Detect optional interfaces
	gm.saveStater, _ = emu.(coreif.SaveStater)
	gm.batterySaver, _ = emu.(coreif.BatterySaver)
	gm.aspectProvider, _ = emu.(coreif.AspectProvider)
	gm.renderInfo, _ = emu.(coreif.RenderInfoProvider)
	gm.memoryInspector, _ = emu.(coreif.Memory)
	gm.perfOverlay.Stats().Reset(gm.renderInfo)

	// Create renderer and shared structures for the emulation goroutine
	gm.renderer = NewFramebufferRenderer(gm.systemInfo.PixelAspectRatio)
	gm.renderer.SetAspectRatioMode(cfg.Video.AspectRatio)
	gm.sharedInput = &SharedInput{}
	gm.sharedFramebuffer = NewSharedFramebuffer(gm.systemInfo.ScreenWidth, gm.systemInfo.ScreenHeight)
	gm.emuControl = NewEmuControl()

	// The emulation loop is paced by the framePacer (an absolute-deadline
	// timer locked to the audio ring fill), not by audio drain. The pacer
	// owns the rate-derived ring/frame sizing; the audio player just outputs.
	// When muted, volume is set to 0 (silent output, loop still paced). If no
	// audio device is available, NewAudioPlayer falls back to a timer-driven
	// drain so the ring still empties.
	volume := cfg.Audio.Volume
	if cfg.Audio.Muted {
		volume = 0
	}
	gm.framePacer = newFramePacer(audioSampleRate, emu.GetTiming().FPS)
	gm.audioPlayer = NewAudioPlayer(volume, gm.framePacer.RingCapacity(), gm.framePacer.FrameBytes())

	// Load SRAM if exists
	if gm.batterySaver != nil {
		if err := gm.saveStateManager.LoadSRAM(gm.batterySaver); err != nil {
			log.Printf("Failed to load SRAM: %v", err)
		}
	}

	// Load resume state if requested
	if resume && gm.saveStater != nil {
		if err := gm.saveStateManager.LoadResume(gm.saveStater); err != nil {
			gm.notification.ShowShort("Failed to resume, starting fresh")
		}
	}

	// Update library entry
	gm.library.SetLastPlayed(gameID, time.Now().Unix())

	// Create rewind buffer if enabled
	if cfg.Rewind.Enabled && gm.saveStater != nil && gm.systemInfo.SerializeSize > 0 {
		gm.rewindBuffer = NewRewindBuffer(cfg.Rewind.BufferSizeMB, cfg.Rewind.FrameStep, gm.systemInfo.SerializeSize)
	}

	// Set TPS to 60 for all regions; emu goroutine handles its own timing
	ebiten.SetTPS(60)

	// Start play time tracking
	gm.playTime.sessionSeconds = 0
	gm.playTime.trackStart = time.Now().Unix()
	gm.playTime.tracking = true

	// Initialize pause menu
	gm.pauseMenu.Hide()

	// Initialize the overlay for the achievements loaded with the game
	if l.achievements {
		gm.achievementOverlay.InitForGame()
	}

	// Initialize rumble if any assigned controller profile has a non-zero
	// rumble intensity and the emulator exposes memory.
	if cfg.Input.RumbleEnabled() && gm.memoryInspector != nil {
		gm.rumbleEngine = gm.loadRumbleRuleset(game.GameID, gm.memoryInspector)
	}

	// Load the game's enabled cheats when the emulator exposes memory
	if gm.memoryInspector != nil {
		gm.cheatEngine = gm.loadCheatEngine(game.GameID)
	}

	// Let the emulation goroutine run frames
	gm.pending = nil
	l.begin <- true
}

// currentPAR returns the pixel aspect ratio to deliver with the frame:
// the core's cached per-frame value when it implements AspectProvider,
// otherwise the static SystemInfo value.
func (gm *GameplayManager) currentPAR() float64 {
	if gm.aspectProvider != nil {
		return gm.aspectProvider.PixelAspectRatio()
	}
	return gm.systemInfo.PixelAspectRatio
}

// runEmulatorFrame advances the emulator by one frame, records the
// frame for the performance overlay, and processes achievements.
// Called from the emulation goroutine.
func (gm *GameplayManager) runEmulatorFrame() {
	start := time.Now()
	gm.emulator.RunFrame()
	now := time.Now()
	var gameFrames uint64
	if gm.renderInfo != nil {
		gameFrames = gm.renderInfo.GameFrameCount()
	}
	gm.perfOverlay.Stats().AddFrame(now.Sub(start), gameFrames, now)
	if gm.achievementManager != nil {
		gm.achievementManager.DoFrame()
	}
	// Cheats write memory between frames, while the emulation is stopped
	if gm.cheatEngine != nil {
		gm.cheatEngine.Apply(gm.memoryInspector)
	}
}

// emulationLoop runs on a dedicated goroutine. It first prepares the
// emulator of the launch, reports the outcome, and waits to be told to
// begin. It then executes emulator frames, queues audio, and updates the
// shared framebuffer. Pacing comes
// from the framePacer: each iteration sleeps to an absolute frame deadline,
// with the interval slowly corrected from the audio ring fill so long-term
// rate stays locked to the device. Turbo runs several emulated frames per
// pacer step, so audio plays at 1x while emulation runs faster.
func (gm *GameplayManager) emulationLoop(l *gameLaunch) {
	defer close(gm.emuDone)

	failure := gm.prepareLaunch(l)
	l.result <- failure
	if failure != nil || !<-l.begin {
		return
	}

	autoSaveTimer := time.Now().Add(time.Second) // First serialize after 1 second

	// Settings cannot change while a game runs, so the fast-forward mute
	// setting is read once rather than copied from the config every frame
	fastForwardMute := gm.config.Values().Audio.FastForwardMute

	for {
		// Check pause/stop. Shutdown exits here (Stop + ring Close); the
		// pacer only ever parks in a bounded time.Sleep.
		if !gm.emuControl.CheckPause() {
			return
		}
		gm.framePacer.wait(gm.audioPlayer.Buffered())

		// Read input from shared state
		buttons := gm.sharedInput.Read()
		for player := 0; player < maxPlayers; player++ {
			gm.emulator.SetInput(player, buttons[player])
		}

		// Read turbo state
		multiplier := gm.turboState.Read()

		// Run extra turbo frames. Always drain GetAudioSamples (even
		// when muted) so the core's internal audio buffer doesn't
		// accumulate across RunFrames.
		for i := 1; i < multiplier; i++ {
			gm.runEmulatorFrame()
			samples := gm.emulator.GetAudioSamples()
			if !fastForwardMute {
				gm.turboAudioBuf = append(gm.turboAudioBuf, samples...)
			}
		}

		// Run the primary frame
		gm.runEmulatorFrame()

		// Evaluate rumble conditions (fire on Ebiten thread via Update)
		if gm.rumbleEngine != nil {
			states := gm.rumbleEngine.Evaluate(gm.memoryInspector, time.Now())
			gm.pendingRumbleMu.Lock()
			gm.pendingMotor = states
			gm.pendingMotorSet = true
			gm.pendingRumbleMu.Unlock()
		}

		// Queue one frame of audio. QueueSamples auto-pads empty input
		// to one frame of silence so oto always has samples to drain.
		switch {
		case multiplier == 1:
			gm.audioPlayer.QueueSamples(gm.emulator.GetAudioSamples())
		case !fastForwardMute:
			gm.turboAudioBuf = append(gm.turboAudioBuf, gm.emulator.GetAudioSamples()...)
			averaged := averageAudio(gm.turboAudioBuf, multiplier)
			gm.audioPlayer.QueueSamples(averaged)
			gm.turboAudioBuf = gm.turboAudioBuf[:0]
		default:
			// Muted fast-forward: drain the core's samples to keep its
			// internal audio buffer from accumulating, then queue an
			// equal-length run of zeros (or the auto-padded silent
			// frame when the core produced nothing). Zero data plays
			// silently regardless of volume.
			samples := gm.emulator.GetAudioSamples()
			gm.audioPlayer.QueueSamples(make([]int16, len(samples)))
		}

		// Update shared framebuffer for Draw thread
		gm.sharedFramebuffer.Update(
			gm.emulator.GetFramebuffer(),
			gm.emulator.GetFramebufferStride(),
			gm.emulator.GetActiveHeight(),
			gm.currentPAR(),
		)

		// Capture rewind state (only when not rewinding)
		if gm.rewindBuffer != nil && gm.saveStater != nil {
			gm.rewindBuffer.Capture(gm.saveStater)
		}

		// Periodic auto-save: serialize state and cache for Ebiten thread to write to disk
		now := time.Now()
		if now.After(autoSaveTimer) && gm.saveStater != nil {
			state, err := gm.saveStater.Serialize()
			if err == nil {
				gm.autoSaveStateMu.Lock()
				gm.autoSaveState = state
				gm.autoSaveReady = true
				gm.autoSaveStateMu.Unlock()
			}
			autoSaveTimer = now.Add(gm.autoSaveInterval)
		}
	}
}

// Update handles the gameplay update loop. Returns true if pause menu was opened.
// This runs on the Ebiten thread — it polls input and manages UI state.
// The emulator itself runs on a separate goroutine.
func (gm *GameplayManager) Update() (pauseMenuOpened bool, err error) {
	if gm.emulator == nil {
		return false, nil
	}

	// Check for Tab key to toggle achievement overlay
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) && !gm.pauseMenu.IsVisible() {
		if gm.achievementOverlay.IsVisible() {
			gm.resumeFromAchievementOverlay()
		} else if gm.achievementManager != nil && gm.achievementManager.IsGameLoaded() {
			gm.emuControl.RequestPause()
			gm.achievementOverlay.Show()
			gm.pausePlayTimeTracking()
		}
	}

	// Handle achievement overlay if visible. Navigation comes from the shared
	// InputManager (keyboard, D-pad, and left analog stick), polled here only
	// while the overlay is open so gameplay inputs are not consumed as UI input.
	if gm.achievementOverlay.IsVisible() {
		gm.achievementOverlay.Update(gm.inputManager.GetUINavigation())
		if !gm.achievementOverlay.IsVisible() {
			// Overlay closed itself (back/close input) - resume gameplay.
			gm.resumeFromAchievementOverlay()
		} else if gm.achievementManager != nil {
			// Process achievement idle tasks while the overlay is shown.
			gm.achievementManager.Idle()
		}
		return false, nil
	}

	// Check for pause menu toggle (ESC or Select button)
	openPauseMenu := inpututil.IsKeyJustPressed(ebiten.KeyEscape)

	// Check for Select button on gamepad
	gamepadIDs := ebiten.AppendGamepadIDs(nil)
	for _, id := range gamepadIDs {
		if inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonCenterLeft) {
			openPauseMenu = true
			break
		}
	}

	if openPauseMenu && !gm.pauseMenu.IsVisible() {
		// Pause emulation goroutine, then open pause menu
		gm.emuControl.RequestPause()
		gm.triggerAutoSave()
		gm.pauseMenu.Show()
		gm.pausePlayTimeTracking()
		return true, nil
	}

	// Handle pause menu if visible. Navigation comes from the shared
	// InputManager (keyboard, D-pad, and left analog stick), polled here only
	// while the menu is open so gameplay inputs are not consumed as UI input.
	if gm.pauseMenu.IsVisible() {
		gm.pauseMenu.Update(gm.inputManager.GetUINavigation())
		// Process achievement idle tasks while paused
		if gm.achievementManager != nil {
			gm.achievementManager.Idle()
		}
		return false, nil
	}

	// Fire pending rumble output (must be on Ebiten thread)
	gm.pendingRumbleMu.Lock()
	motorStates := gm.pendingMotor
	motorSet := gm.pendingMotorSet
	gm.pendingMotor = nil
	gm.pendingMotorSet = false
	gm.pendingRumbleMu.Unlock()
	if motorSet {
		gm.rumbleActivePads = FireMotorStates(motorStates, gm.playerRumbleScales(), gm.rumbleActivePads)
	}

	// Poll input and write to shared state (emu goroutine reads it). After
	// resuming from an overlay (pause menu or achievement overlay), hold all
	// game input neutral until every held input is released, so nothing from
	// the overlay interaction leaks into the game.
	if gm.resumeInputGuard {
		p1, p2 := gm.pollButtons()
		gm.sharedInput.Set(0, 0)
		gm.sharedInput.Set(1, 0)
		if p1 == 0 && p2 == 0 {
			gm.resumeInputGuard = false
		}
	} else {
		gm.pollInputToShared()
	}

	// Handle turbo key input (F4 cycles speed)
	gm.handleTurboKey()

	// Handle the performance overlay key (grave accent cycles the mode)
	gm.handlePerfOverlayKey()

	// Check rewind input (R key)
	if gm.rewindBuffer != nil {
		holdDuration := inpututil.KeyPressDuration(ebiten.KeyR)
		if holdDuration > 0 {
			items := rewindItemsForHoldDuration(holdDuration)
			if items > 0 {
				if !gm.rewindBuffer.IsRewinding() {
					// Pause goroutine for rewind — we access emulator directly
					gm.emuControl.RequestPause()
					gm.rewindBuffer.SetRewinding(true)
					gm.audioPlayer.ClearQueue()
				}
				gm.rewindBuffer.Rewind(gm.emulator, gm.saveStater, items)
				// Update shared framebuffer after rewind step
				gm.sharedFramebuffer.Update(
					gm.emulator.GetFramebuffer(),
					gm.emulator.GetFramebufferStride(),
					gm.emulator.GetActiveHeight(),
					gm.currentPAR(),
				)
				return false, nil
			}
			// items == 0 means we're in a hold gap frame; skip normal execution
			return false, nil
		} else if gm.rewindBuffer.IsRewinding() {
			// R released - resume emulation goroutine
			gm.rewindBuffer.SetRewinding(false)
			gm.emuControl.RequestResume()
		}
	}

	// Handle save state keys (pauses goroutine as needed)
	gm.handleSaveStateKeys()

	// Check for cached auto-save state from emu goroutine → write to disk
	gm.autoSaveStateMu.Lock()
	if gm.autoSaveReady && !gm.autoSaving.Load() {
		state := gm.autoSaveState
		gm.autoSaveReady = false
		gm.autoSaveStateMu.Unlock()
		gm.writeAutoSave(state)
	} else {
		gm.autoSaveStateMu.Unlock()
	}

	return false, nil
}

// Draw renders the gameplay screen from the shared framebuffer and returns
// the picture's placement in screen, zero when nothing was drawn.
func (gm *GameplayManager) Draw(screen *ebiten.Image) shader.Picture {
	if gm.emulator == nil || gm.sharedFramebuffer == nil || gm.renderer == nil {
		return shader.Picture{}
	}

	pixels, stride, height, par := gm.sharedFramebuffer.Read()
	if height == 0 {
		return shader.Picture{}
	}
	gm.renderer.SetPAR(par)
	return gm.renderer.DrawFramebuffer(screen, pixels, stride, height)
}

// DrawFramebuffer returns the native-resolution framebuffer and the pixel
// aspect ratio delivered with that frame, for the effects pipeline. Reads from
// the shared framebuffer rather than directly from the emulator.
func (gm *GameplayManager) DrawFramebuffer() (*ebiten.Image, float64) {
	if gm.emulator == nil || gm.sharedFramebuffer == nil || gm.renderer == nil {
		return nil, 0
	}
	pixels, stride, height, par := gm.sharedFramebuffer.Read()
	if height == 0 {
		return nil, 0
	}
	return gm.renderer.GetFramebufferImage(pixels, stride, height), par
}

// DrawPauseMenu draws the pause menu overlay
func (gm *GameplayManager) DrawPauseMenu(screen *ebiten.Image) {
	gm.pauseMenu.Draw(screen)
}

// DrawAchievementOverlay draws the achievement overlay
func (gm *GameplayManager) DrawAchievementOverlay(screen *ebiten.Image) {
	gm.achievementOverlay.Draw(screen)
}

// DrawPerfOverlay draws the performance overlay when it is on.
func (gm *GameplayManager) DrawPerfOverlay(screen *ebiten.Image) {
	if gm.emulator == nil {
		return
	}
	gm.perfOverlay.Draw(screen)
}

// IsPaused returns whether the pause menu is visible
func (gm *GameplayManager) IsPaused() bool {
	return gm.pauseMenu.IsVisible()
}

// Resume resumes gameplay after the pause menu.
func (gm *GameplayManager) Resume() {
	gm.pauseMenu.Hide()
	gm.emuControl.RequestResume()
	gm.playTime.trackStart = time.Now().Unix()
	gm.playTime.tracking = true
	gm.resumeInputGuard = true
}

// resumeFromAchievementOverlay resumes gameplay after the achievement overlay,
// whether it was closed by the Tab toggle or by its own back/close input.
func (gm *GameplayManager) resumeFromAchievementOverlay() {
	gm.achievementOverlay.Hide()
	gm.emuControl.RequestResume()
	gm.playTime.trackStart = time.Now().Unix()
	gm.playTime.tracking = true
	gm.resumeInputGuard = true
}

// Exit cleans up when exiting gameplay
func (gm *GameplayManager) Exit(saveResume bool) {
	if l := gm.pending; l != nil {
		// A launch is still in progress. Tell the goroutine not to run
		// frames, wait for it to finish the step it is on, and release
		// what it made. Nothing of the game has run, so nothing is saved.
		l.begin <- false
		<-gm.emuDone
		gm.releaseLaunch(l)
		return
	}
	if gm.emulator == nil {
		return
	}

	// Stop emulation goroutine and wait for it to exit. The audio player is
	// closed before waiting because the goroutine may be blocked in ring.Write
	// on a full ring; AudioPlayer.Close wakes that path. Otherwise the pacer
	// parks only in a bounded time.Sleep, so the goroutine reaches its next
	// CheckPause and exits. The goroutine may still be mid-frame and using
	// the player, whose writes return at once after Close, so the player is
	// cleared only once the goroutine is done.
	if gm.emuControl != nil {
		gm.emuControl.Stop()
		gm.audioPlayer.Close()
		<-gm.emuDone
		gm.audioPlayer = nil
	}

	// Wait for any pending auto-save disk write to complete (max 2 seconds)
	done := make(chan struct{})
	go func() {
		gm.autoSaveWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		// Auto-save completed
	case <-time.After(2 * time.Second):
		log.Printf("Warning: auto-save timed out on exit")
	}

	// Stop play time tracking and update
	gm.pausePlayTimeTracking()
	gm.updatePlayTime()

	// Save SRAM (goroutine is stopped, safe to access emulator directly)
	if gm.batterySaver != nil {
		if err := gm.saveStateManager.SaveSRAM(gm.batterySaver); err != nil {
			log.Printf("SRAM save failed: %v", err)
		}
	}

	// Save resume state if requested
	if saveResume && gm.saveStater != nil {
		if err := gm.saveStateManager.SaveResume(gm.saveStater); err != nil {
			log.Printf("Resume save failed: %v", err)
		}
	}

	// Free shared state
	gm.rewindBuffer = nil
	gm.sharedInput = nil
	gm.sharedFramebuffer = nil
	gm.emuControl = nil
	gm.autoSaveState = nil
	gm.autoSaveReady = false
	gm.turboAudioBuf = nil

	// Reset achievement overlay and unload achievements
	gm.achievementOverlay.Reset()
	if gm.achievementManager != nil {
		gm.achievementManager.UnloadGame()
	}

	// Close emulator
	gm.emulator.Close()
	if gm.disc != nil {
		gm.disc.Close()
		gm.disc = nil
	}

	// Clear emulator and optional interfaces
	gm.emulator = nil
	gm.saveStater = nil
	gm.batterySaver = nil
	gm.memoryInspector = nil
	if gm.renderer != nil {
		gm.renderer.Release()
		gm.renderer = nil
	}
	gm.currentGame = nil
	gm.rumbleEngine = nil
	gm.cheatEngine = nil
	gm.pendingMotor = nil
	gm.pendingMotorSet = false
	gm.rumbleActivePads = nil

	// Reset TPS to 60 for UI
	ebiten.SetTPS(60)
}

// pollButtons reads the current button bitmasks for both players using the
// player-to-controller assignment. Player 1 always includes the keyboard;
// a player with no bound controller reads 0 from the pad.
func (gm *GameplayManager) pollButtons() (p1, p2 uint32) {
	p1Pad, p1HasPad := gm.assignment.PadFor(0)
	p1 = PollButtons(gm.mappings[0], p1Pad, p1HasPad)

	if p2Pad, ok := gm.assignment.PadFor(1); ok {
		p2 = PollGamepadButtons(gm.mappings[1], p2Pad)
	}
	return p1, p2
}

// pollInputToShared reads input and writes it to the shared input state
// for the emulation goroutine to consume. Both players are always written
// so a disconnected controller reads as no buttons held.
func (gm *GameplayManager) pollInputToShared() {
	p1, p2 := gm.pollButtons()
	gm.sharedInput.Set(0, p1)
	gm.sharedInput.Set(1, p2)
}

// handleTurboKey checks F4 to cycle turbo speed: Off → 2x → 3x → Off.
func (gm *GameplayManager) handleTurboKey() {
	if inpututil.IsKeyJustPressed(ebiten.KeyF4) {
		multiplier := gm.turboState.CycleMultiplier()
		switch multiplier {
		case 1:
			gm.notification.ShowShort("Turbo: Off")
		case 2:
			gm.notification.ShowShort("Turbo: 2x")
		case 3:
			gm.notification.ShowShort("Turbo: 3x")
		}
	}
}

// handlePerfOverlayKey checks the grave accent key to cycle the
// performance overlay: off, basic, extended, off.
func (gm *GameplayManager) handlePerfOverlayKey() {
	if inpututil.IsKeyJustPressed(ebiten.KeyGraveAccent) {
		gm.perfOverlay.Cycle()
	}
}

// handleSaveStateKeys handles F1/F2/F3 for save states.
// Pauses the emulation goroutine for Save/Load operations.
func (gm *GameplayManager) handleSaveStateKeys() {
	if gm.saveStater == nil {
		return
	}

	// F1 - Save to current slot
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		gm.emuControl.RequestPause()
		if err := gm.saveStateManager.Save(gm.saveStater); err != nil {
			log.Printf("Save state failed: %v", err)
		}
		gm.emuControl.RequestResume()
	}

	// F2 - Next slot (Shift+F2 - Previous slot) — no pause needed
	if inpututil.IsKeyJustPressed(ebiten.KeyF2) {
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			gm.saveStateManager.PreviousSlot()
		} else {
			gm.saveStateManager.NextSlot()
		}
	}

	// F3 - Load from current slot
	if inpututil.IsKeyJustPressed(ebiten.KeyF3) {
		gm.emuControl.RequestPause()
		if err := gm.saveStateManager.Load(gm.saveStater); err != nil {
			log.Printf("Load state failed: %v", err)
		} else {
			if gm.rewindBuffer != nil {
				gm.rewindBuffer.Reset()
			}
			if gm.rumbleEngine != nil {
				gm.rumbleEngine.Reset()
			}
			// Update shared framebuffer after load
			gm.sharedFramebuffer.Update(
				gm.emulator.GetFramebuffer(),
				gm.emulator.GetFramebufferStride(),
				gm.emulator.GetActiveHeight(),
				gm.currentPAR(),
			)
			gm.audioPlayer.ClearQueue()
		}
		gm.emuControl.RequestResume()
	}
}

// triggerAutoSave saves the emulator state to disk.
// When the goroutine is paused, serializes fresh. Otherwise uses cached state.
func (gm *GameplayManager) triggerAutoSave() {
	if gm.emulator == nil || gm.currentGame == nil || gm.autoSaving.Load() {
		return
	}

	var state []byte
	if gm.emuControl != nil && gm.emuControl.IsPaused() && gm.saveStater != nil {
		// Goroutine is paused — safe to serialize fresh and save SRAM
		var err error
		state, err = gm.saveStater.Serialize()
		if err != nil {
			log.Printf("Auto-save serialize failed: %v", err)
			return
		}
		// Also save SRAM since goroutine is paused
		if gm.batterySaver != nil {
			if err := gm.saveStateManager.SaveSRAM(gm.batterySaver); err != nil {
				log.Printf("SRAM save failed: %v", err)
			}
		}
	} else {
		// Use cached state from emu goroutine (no SRAM save — goroutine running)
		gm.autoSaveStateMu.Lock()
		state = gm.autoSaveState
		gm.autoSaveStateMu.Unlock()
	}

	if state == nil {
		return
	}

	gm.updatePlayTime()
	gm.writeAutoSave(state)
}

// writeAutoSave writes pre-serialized state data to disk asynchronously.
// The goroutine only writes the file: the path is resolved before it
// starts, so a game launched while it is still writing cannot change
// where it writes.
func (gm *GameplayManager) writeAutoSave(state []byte) {
	path, err := gm.saveStateManager.ResumePath()
	if err != nil {
		log.Printf("Auto-save failed: %v", err)
		return
	}
	gm.autoSaving.Store(true)
	gm.autoSaveWg.Add(1)
	go func() {
		defer gm.autoSaveWg.Done()
		defer gm.autoSaving.Store(false)

		if err := storage.AtomicWriteFile(path, state); err != nil {
			log.Printf("Auto-save failed: %v", err)
		}
	}()
}

// pausePlayTimeTracking pauses the play time tracker
func (gm *GameplayManager) pausePlayTimeTracking() {
	if gm.playTime.tracking {
		elapsed := time.Now().Unix() - gm.playTime.trackStart
		gm.playTime.sessionSeconds += elapsed
		gm.playTime.tracking = false
	}
}

// updatePlayTime updates the play time in the library
func (gm *GameplayManager) updatePlayTime() {
	if gm.currentGame == nil {
		return
	}

	var totalSession int64
	if gm.playTime.tracking {
		elapsed := time.Now().Unix() - gm.playTime.trackStart
		totalSession = gm.playTime.sessionSeconds + elapsed
	} else {
		totalSession = gm.playTime.sessionSeconds
	}

	// Only update if there's actual play time
	if totalSession > 0 {
		gm.library.AddPlayTime(gm.currentGame.GameID, totalSession)
		gm.currentGame.PlayTimeSeconds += totalSession
		gm.playTime.sessionSeconds = 0
		if gm.playTime.tracking {
			gm.playTime.trackStart = time.Now().Unix()
		}
	}
}
