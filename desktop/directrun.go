// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

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
	emulator       coreif.Emulator
	aspectProvider coreif.AspectProvider // nil unless the core implements it
	systemInfo     coreif.SystemInfo
	inputMapping   InputMapping
	renderer       *FramebufferRenderer
	audioPlayer    *AudioPlayer
	framePacer     *framePacer
	emuControl     *EmuControl
	sharedInput    *SharedInput
	sharedFB       *SharedFramebuffer
	emuDone        chan struct{}
}

// RunDirect loads a ROM and runs it directly without the full UI.
// The options map is applied to the emulator via SetOption.
// The bios map provides BIOS data keyed by BIOSOption.Key (may be nil).
func RunDirect(factory coreif.CoreFactory, romPath string, options map[string]string, bios map[string][]byte) error {
	systemInfo := factory.SystemInfo()

	// oto allows a single context rate per process. Use the core's rate
	// so emulator audio plays at the correct pitch and the framePacer's
	// frame/ring sizing matches. Must be set before any audio/oto use.
	if systemInfo.SampleRate > 0 {
		audioSampleRate = systemInfo.SampleRate
	}

	emulator := factory.CreateEmulator()

	// Disc-based systems stream from disk via SetDisc; cartridge systems
	// load the ROM bytes via SetRom.
	if systemInfo.Disc {
		disc, derr := romloader.OpenDisc(romPath)
		if derr != nil {
			emulator.Close()
			return fmt.Errorf("failed to load disc: %w", derr)
		}
		defer disc.Close()
		emulator.SetDisc(disc)
	} else {
		romData, _, lerr := romloader.Load(romPath, systemInfo.Extensions)
		if lerr != nil {
			emulator.Close()
			return fmt.Errorf("failed to load ROM: %w", lerr)
		}
		emulator.SetRom(romData)
	}

	for key, value := range options {
		emulator.SetOption(key, value)
	}

	for key, data := range bios {
		if err := emulator.SetBIOS(key, data); err != nil {
			emulator.Close()
			return fmt.Errorf("failed to set BIOS %q: %w", key, err)
		}
	}

	emulator.Start()

	ebiten.SetWindowTitle(systemInfo.CoreName)
	applyWindowIcon()
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(60)

	// Use 4:3 (standard CRT) for initial window size; the renderer
	// computes the correct DAR per-frame from actual dimensions.
	// ScreenWidth is a core's maximum framebuffer width (e.g. the
	// Saturn's 704 hi-res mode), so ScreenWidth*3 can exceed the
	// monitor. Clamp to the monitor work area, preserving 4:3.
	windowW := systemInfo.ScreenWidth * 3
	windowH := windowW * 3 / 4
	if m := ebiten.Monitor(); m != nil {
		mw, mh := m.Size()
		// Leave a margin so the title bar and taskbar stay on screen.
		maxW := mw * 9 / 10
		maxH := mh * 9 / 10
		if maxW > 0 && maxH > 0 && (windowW > maxW || windowH > maxH) {
			if windowW*maxH > maxW*windowH {
				windowW = maxW
				windowH = windowW * 3 / 4
			} else {
				windowH = maxH
				windowW = windowH * 4 / 3
			}
		}
	}
	minW := systemInfo.ScreenWidth * 2
	minH := minW * 3 / 4
	if minW > windowW || minH > windowH {
		minW = windowW
		minH = windowH
	}
	ebiten.SetWindowSize(windowW, windowH)
	ebiten.SetWindowSizeLimits(minW, minH, -1, -1)

	pacer := newFramePacer(audioSampleRate, emulator.GetTiming().FPS)
	audioPlayer := NewAudioPlayer(1.0, pacer.RingCapacity(), pacer.FrameBytes())

	aspectProvider, _ := emulator.(coreif.AspectProvider)

	dr := &directRunner{
		emulator:       emulator,
		aspectProvider: aspectProvider,
		systemInfo:     systemInfo,
		inputMapping:   BuildDefaultMapping(systemInfo.Buttons),
		renderer:       NewFramebufferRenderer(systemInfo.ScreenWidth, systemInfo.PixelAspectRatio),
		audioPlayer:    audioPlayer,
		framePacer:     pacer,
		emuControl:     NewEmuControl(),
		sharedInput:    &SharedInput{},
		sharedFB:       NewSharedFramebuffer(systemInfo.ScreenWidth, systemInfo.MaxScreenHeight),
		emuDone:        make(chan struct{}),
	}

	go dr.emulationLoop()

	err := ebiten.RunGame(dr)

	dr.Close()

	return err
}

// emulationLoop runs on a dedicated goroutine. Pacing comes from the
// framePacer: each frame sleeps to an absolute deadline, with the interval
// slowly corrected from the audio ring fill so long-term rate stays locked
// to the device.
func (dr *directRunner) emulationLoop() {
	defer close(dr.emuDone)

	for {
		if !dr.emuControl.CheckPause() {
			return
		}
		dr.framePacer.wait(dr.audioPlayer.Buffered())

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
			dr.currentPAR(),
		)
	}
}

// currentPAR returns the core's per-frame pixel aspect ratio when it
// implements AspectProvider, otherwise the static SystemInfo value.
func (dr *directRunner) currentPAR() float64 {
	if dr.aspectProvider != nil {
		return dr.aspectProvider.PixelAspectRatio()
	}
	return dr.systemInfo.PixelAspectRatio
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
	pixels, stride, activeHeight, par := dr.sharedFB.Read()
	if activeHeight == 0 {
		return
	}
	dr.renderer.SetPAR(par)
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
