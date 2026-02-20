//go:build !ios && !libretro

package standalone

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	emucore "github.com/user-none/eblitui/api"
	"github.com/user-none/eblitui/romloader"
)

// directRunner implements ebiten.Game for minimal direct ROM execution.
// It skips the full UI (library, settings, save states, achievements, etc.)
// and runs the emulator with just input, audio, and rendering.
type directRunner struct {
	emulator     emucore.Emulator
	systemInfo   emucore.SystemInfo
	inputMapping InputMapping
	renderer     *FramebufferRenderer
	audioPlayer  *AudioPlayer
	emuControl   *EmuControl
	sharedInput  *SharedInput
	sharedFB     *SharedFramebuffer
	emuDone      chan struct{}
}

// RunDirect loads a ROM and runs it directly without the full UI.
// The regionStr parameter accepts "auto", "ntsc", or "pal".
// The options map is applied to the emulator via SetOption.
func RunDirect(factory emucore.CoreFactory, romPath, regionStr string, options map[string]string) error {
	systemInfo := factory.SystemInfo()

	romData, _, err := romloader.Load(romPath, systemInfo.Extensions)
	if err != nil {
		return fmt.Errorf("failed to load ROM: %w", err)
	}

	region, err := parseRegion(regionStr, factory, romData)
	if err != nil {
		return err
	}

	emulator, err := factory.CreateEmulator(romData, region)
	if err != nil {
		return fmt.Errorf("failed to create emulator: %w", err)
	}

	for key, value := range options {
		emulator.SetOption(key, value)
	}

	ebiten.SetWindowTitle(systemInfo.CoreName)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(60)

	windowW := systemInfo.ScreenWidth * 3
	windowH := int(float64(windowW) / systemInfo.AspectRatio)
	minW := systemInfo.ScreenWidth * 2
	minH := int(float64(minW) / systemInfo.AspectRatio)
	ebiten.SetWindowSize(windowW, windowH)
	ebiten.SetWindowSizeLimits(minW, minH, -1, -1)

	audioPlayer, err := NewAudioPlayer(1.0)
	if err != nil {
		log.Printf("Warning: audio initialization failed: %v", err)
	}

	dr := &directRunner{
		emulator:     emulator,
		systemInfo:   systemInfo,
		inputMapping: BuildDefaultMapping(systemInfo.Buttons),
		renderer:     NewFramebufferRenderer(systemInfo.ScreenWidth),
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

// emulationLoop runs on a dedicated goroutine with audio-driven timing.
func (dr *directRunner) emulationLoop() {
	defer close(dr.emuDone)

	timing := dr.emulator.GetTiming()
	frameTime := time.Duration(float64(time.Second) / float64(timing.FPS))
	lastFrameTime := time.Now()

	for {
		if !dr.emuControl.CheckPause() {
			return
		}

		buttons := dr.sharedInput.Read()
		for player := 0; player < maxPlayers; player++ {
			dr.emulator.SetInput(player, buttons[player])
		}

		dr.emulator.RunFrame()

		if dr.audioPlayer != nil {
			dr.audioPlayer.QueueSamples(dr.emulator.GetAudioSamples())
		}

		dr.sharedFB.Update(
			dr.emulator.GetFramebuffer(),
			dr.emulator.GetFramebufferStride(),
			dr.emulator.GetActiveHeight(),
		)

		elapsed := time.Since(lastFrameTime)
		sleepTime := frameTime - elapsed

		if dr.audioPlayer != nil {
			bufferLevel := dr.audioPlayer.GetBufferLevel()
			if bufferLevel < adtMinBuffer {
				sleepTime = time.Duration(float64(sleepTime) * 0.9)
			} else if bufferLevel > adtMaxBuffer {
				sleepTime = time.Duration(float64(sleepTime) * 1.1)
			}
		}

		if sleepTime > time.Millisecond {
			time.Sleep(sleepTime)
		}

		lastFrameTime = time.Now()
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
	s := 1.0
	if m := ebiten.Monitor(); m != nil {
		s = m.DeviceScaleFactor()
	}
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
	buttons := PollButtons(dr.inputMapping, gamepadID, hasGamepad)
	dr.sharedInput.Set(0, buttons)

	// Player 2: second gamepad only
	if len(gamepadIDs) > 1 {
		p2buttons := PollGamepadButtons(dr.inputMapping, gamepadIDs[1])
		dr.sharedInput.Set(1, p2buttons)
	}
}

// Close cleans up resources.
func (dr *directRunner) Close() {
	dr.emuControl.Stop()
	<-dr.emuDone

	if dr.audioPlayer != nil {
		dr.audioPlayer.Close()
	}
	dr.emulator.Close()
}

// parseRegion converts a region string to emucore.Region.
// "auto" uses the factory's DetectRegion, "ntsc" and "pal" map directly.
func parseRegion(regionStr string, factory emucore.CoreFactory, romData []byte) (emucore.Region, error) {
	switch strings.ToLower(regionStr) {
	case "auto":
		region, _ := factory.DetectRegion(romData)
		return region, nil
	case "ntsc":
		return emucore.RegionNTSC, nil
	case "pal":
		return emucore.RegionPAL, nil
	default:
		return 0, fmt.Errorf("unknown region %q: use auto, ntsc, or pal", regionStr)
	}
}
