package desktop

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/display"
	"github.com/user-none/eblitui/romloader"
)

// directRunner implements ebiten.Game for minimal direct ROM execution.
// It skips the full UI (library, settings, save states, achievements, etc.)
// and runs the emulator with just input, audio, and rendering.
type directRunner struct {
	emulator     coreif.Emulator
	systemInfo   coreif.SystemInfo
	inputMapping InputMapping
	renderer     *FramebufferRenderer
	audioPlayer  *AudioPlayer
	emuControl   *EmuControl
	sharedInput  *SharedInput
	sharedFB     *SharedFramebuffer
	emuDone      chan struct{}
}

// RunDirect loads a ROM and runs it directly without the full UI.
// The options map is applied to the emulator via SetOption.
// The bios map provides BIOS data keyed by BIOSOption.Key (may be nil).
func RunDirect(factory coreif.CoreFactory, romPath string, options map[string]string, bios map[string][]byte) error {
	systemInfo := factory.SystemInfo()

	romData, _, err := romloader.Load(romPath, systemInfo.Extensions)
	if err != nil {
		return fmt.Errorf("failed to load ROM: %w", err)
	}

	emulator, err := factory.CreateEmulator(romData)
	if err != nil {
		return fmt.Errorf("failed to create emulator: %w", err)
	}

	for key, value := range options {
		emulator.SetOption(key, value)
	}

	for key, data := range bios {
		emulator.SetBIOS(key, data)
	}

	emulator.Start()

	ebiten.SetWindowTitle(systemInfo.CoreName)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(60)

	// Use 4:3 (standard CRT) for initial window size; the renderer
	// computes the correct DAR per-frame from actual dimensions.
	windowW := systemInfo.ScreenWidth * 3
	windowH := windowW * 3 / 4
	minW := systemInfo.ScreenWidth * 2
	minH := minW * 3 / 4
	ebiten.SetWindowSize(windowW, windowH)
	ebiten.SetWindowSizeLimits(minW, minH, -1, -1)

	audioPlayer := NewAudioPlayer(1.0)

	dr := &directRunner{
		emulator:     emulator,
		systemInfo:   systemInfo,
		inputMapping: BuildDefaultMapping(systemInfo.Buttons),
		renderer:     NewFramebufferRenderer(systemInfo.ScreenWidth, systemInfo.PixelAspectRatio),
		audioPlayer:  audioPlayer,
		emuControl:   NewEmuControl(),
		sharedInput:  &SharedInput{},
		sharedFB:     NewSharedFramebuffer(systemInfo.ScreenWidth, systemInfo.MaxScreenHeight),
		emuDone:      make(chan struct{}),
	}

	go dr.emulationLoop()

	err = ebiten.RunGame(dr)

	dr.Close()

	return err
}

// emulationLoop runs on a dedicated goroutine. Pacing comes from the
// audio player's ring buffer blocking on a full ring; the audio device's
// drain rate is the loop's clock.
func (dr *directRunner) emulationLoop() {
	defer close(dr.emuDone)

	for {
		if !dr.emuControl.CheckPause() {
			return
		}

		buttons := dr.sharedInput.Read()
		for player := 0; player < maxPlayers; player++ {
			dr.emulator.SetInput(player, buttons[player])
		}

		dr.emulator.RunFrame()
		dr.audioPlayer.QueueSamples(dr.emulator.GetAudioSamples())

		dr.sharedFB.Update(
			dr.emulator.GetFramebuffer(),
			dr.emulator.GetFramebufferStride(),
			dr.emulator.GetActiveHeight(),
		)
	}
}

// Update implements ebiten.Game.
func (dr *directRunner) Update() error {
	dr.pollInputToShared()

	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	}

	return nil
}

// Draw implements ebiten.Game.
func (dr *directRunner) Draw(screen *ebiten.Image) {
	pixels, stride, activeHeight := dr.sharedFB.Read()
	if activeHeight == 0 {
		return
	}
	dr.renderer.DrawFramebuffer(screen, pixels, stride, activeHeight)
}

// Layout implements ebiten.Game.
func (dr *directRunner) Layout(outsideWidth, outsideHeight int) (int, int) {
	s := display.DPIScale()
	return int(float64(outsideWidth) * s), int(float64(outsideHeight) * s)
}

// pollInputToShared reads keyboard and gamepad input and writes to shared state.
func (dr *directRunner) pollInputToShared() {
	gamepadIDs := ebiten.AppendGamepadIDs(nil)
	hasGamepad := len(gamepadIDs) > 0

	var gamepadID ebiten.GamepadID
	if hasGamepad {
		gamepadID = gamepadIDs[0]
	}

	// Player 1: keyboard + first gamepad
	buttons := PollButtons(dr.inputMapping, gamepadID, hasGamepad, false)
	dr.sharedInput.Set(0, buttons)

	// Player 2: second gamepad only
	if len(gamepadIDs) > 1 {
		p2buttons := PollGamepadButtons(dr.inputMapping, gamepadIDs[1], false)
		dr.sharedInput.Set(1, p2buttons)
	}
}

// Close cleans up resources.
func (dr *directRunner) Close() {
	dr.emuControl.Stop()

	// Close the audio player before waiting on emuDone. A writer parked
	// inside QueueSamples -> ring.Write cond.Wait needs the ring buffer's
	// Close-broadcast to wake; otherwise the emulation goroutine cannot
	// reach its next CheckPause check and Close would deadlock.
	dr.audioPlayer.Close()

	<-dr.emuDone
	dr.emulator.Close()
}
