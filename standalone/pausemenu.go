//go:build !libretro

package standalone

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/user-none/eblitui/standalone/style"
)

// PauseMenuOption represents a menu option
type PauseMenuOption int

const (
	PauseMenuResume PauseMenuOption = iota
	PauseMenuLibrary
	PauseMenuExit
	PauseMenuOptionCount
)

// PauseMenu handles the in-game pause menu
type PauseMenu struct {
	visible       bool
	selectedIndex int
	onResume      func()
	onLibrary     func()
	onExit        func()

	// Cached layout info for mouse hit testing
	buttonRects []image.Rectangle

	// Cached images to avoid per-frame allocations
	cache struct {
		screenW, screenH int
		themeName        string
		panelW, panelH   int
		buttonW, buttonH int
		dimOverlay       *ebiten.Image
		panelBg          *ebiten.Image
		buttonBgs        [PauseMenuOptionCount]*ebiten.Image
		buttonBgSelected *ebiten.Image
	}

	// Pre-allocated draw options (reset each use)
	drawOpts ebiten.DrawImageOptions
	textOpts text.DrawOptions
}

// NewPauseMenu creates a new pause menu
func NewPauseMenu(onResume, onLibrary, onExit func()) *PauseMenu {
	return &PauseMenu{
		visible:       false,
		selectedIndex: 0,
		onResume:      onResume,
		onLibrary:     onLibrary,
		onExit:        onExit,
		buttonRects:   make([]image.Rectangle, PauseMenuOptionCount),
	}
}

// Show displays the pause menu
func (m *PauseMenu) Show() {
	m.visible = true
	m.selectedIndex = 0
}

// Hide hides the pause menu
func (m *PauseMenu) Hide() {
	m.visible = false
}

// IsVisible returns whether the menu is visible
func (m *PauseMenu) IsVisible() bool {
	return m.visible
}

// Update handles input for the pause menu
func (m *PauseMenu) Update() {
	if !m.visible {
		return
	}

	// ESC closes menu (always Resume regardless of selection)
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		m.selectedIndex = int(PauseMenuResume)
		m.handleSelect()
		return
	}

	// Keyboard navigation
	if inpututil.IsKeyJustPressed(ebiten.KeyUp) || inpututil.IsKeyJustPressed(ebiten.KeyW) {
		m.selectedIndex--
		if m.selectedIndex < 0 {
			m.selectedIndex = int(PauseMenuOptionCount) - 1
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) || inpututil.IsKeyJustPressed(ebiten.KeyS) {
		m.selectedIndex++
		if m.selectedIndex >= int(PauseMenuOptionCount) {
			m.selectedIndex = 0
		}
	}

	// Keyboard selection
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		m.handleSelect()
		return
	}

	// Mouse click
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		for i, rect := range m.buttonRects {
			if image.Pt(mx, my).In(rect) {
				m.selectedIndex = i
				m.handleSelect()
				return
			}
		}
	}

	// Mouse hover for selection highlight
	mx, my := ebiten.CursorPosition()
	for i, rect := range m.buttonRects {
		if image.Pt(mx, my).In(rect) {
			m.selectedIndex = i
			break
		}
	}

	// Gamepad support
	gamepadIDs := ebiten.AppendGamepadIDs(nil)
	for _, id := range gamepadIDs {
		// D-pad navigation
		if inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonLeftTop) {
			m.selectedIndex--
			if m.selectedIndex < 0 {
				m.selectedIndex = int(PauseMenuOptionCount) - 1
			}
		}
		if inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonLeftBottom) {
			m.selectedIndex++
			if m.selectedIndex >= int(PauseMenuOptionCount) {
				m.selectedIndex = 0
			}
		}

		// A/Cross button selects
		if inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonRightBottom) {
			m.handleSelect()
			return
		}

		// B/Circle button acts as Resume
		if inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonRightRight) {
			m.selectedIndex = int(PauseMenuResume)
			m.handleSelect()
			return
		}

		// Start button acts as Resume
		if inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonCenterRight) {
			m.selectedIndex = int(PauseMenuResume)
			m.handleSelect()
			return
		}
	}
}

// handleSelect processes the current selection
func (m *PauseMenu) handleSelect() {
	switch PauseMenuOption(m.selectedIndex) {
	case PauseMenuResume:
		m.Hide()
		if m.onResume != nil {
			m.onResume()
		}
	case PauseMenuLibrary:
		m.Hide()
		if m.onLibrary != nil {
			m.onLibrary()
		}
	case PauseMenuExit:
		m.Hide()
		if m.onExit != nil {
			m.onExit()
		}
	}
}

// rebuildCache recreates cached images when screen dimensions change
func (m *PauseMenu) rebuildCache(screenW, screenH int) {
	// Deallocate old images if they exist
	if m.cache.dimOverlay != nil {
		m.cache.dimOverlay.Deallocate()
	}
	if m.cache.panelBg != nil {
		m.cache.panelBg.Deallocate()
	}
	if m.cache.buttonBgSelected != nil {
		m.cache.buttonBgSelected.Deallocate()
	}
	for i := range m.cache.buttonBgs {
		if m.cache.buttonBgs[i] != nil {
			m.cache.buttonBgs[i].Deallocate()
			m.cache.buttonBgs[i] = nil
		}
	}

	m.cache.screenW = screenW
	m.cache.screenH = screenH
	m.cache.themeName = style.CurrentThemeName

	// Create dim overlay
	m.cache.dimOverlay = ebiten.NewImage(screenW, screenH)
	dimColor := style.DimOverlay
	dimColor.A = 128
	m.cache.dimOverlay.Fill(dimColor)

	// Calculate panel dimensions
	panelWidth := screenW * 40 / 100
	if panelWidth < style.PauseMenuMinWidth {
		panelWidth = style.PauseMenuMinWidth
	}
	if panelWidth > style.PauseMenuMaxWidth {
		panelWidth = style.PauseMenuMaxWidth
	}

	// Calculate button dimensions
	buttonWidth := panelWidth * 80 / 100
	buttonHeight := screenH * 8 / 100
	if buttonHeight < style.PauseMenuMinBtnHeight {
		buttonHeight = style.PauseMenuMinBtnHeight
	}
	if buttonHeight > style.PauseMenuMaxBtnHeight {
		buttonHeight = style.PauseMenuMaxBtnHeight
	}

	buttonSpacing := buttonHeight / 4
	padding := buttonHeight / 2

	// Calculate panel height based on content
	numOptions := int(PauseMenuOptionCount)
	panelHeight := padding*2 + numOptions*buttonHeight + (numOptions-1)*buttonSpacing

	m.cache.panelW = panelWidth
	m.cache.panelH = panelHeight
	m.cache.buttonW = buttonWidth
	m.cache.buttonH = buttonHeight

	// Create panel background
	m.cache.panelBg = ebiten.NewImage(panelWidth, panelHeight)
	m.cache.panelBg.Fill(style.Surface)

	// Draw panel border
	for x := 0; x < panelWidth; x++ {
		m.cache.panelBg.Set(x, 0, style.Border)
		m.cache.panelBg.Set(x, panelHeight-1, style.Border)
	}
	for y := 0; y < panelHeight; y++ {
		m.cache.panelBg.Set(0, y, style.Border)
		m.cache.panelBg.Set(panelWidth-1, y, style.Border)
	}

	// Create selected button background
	m.cache.buttonBgSelected = ebiten.NewImage(buttonWidth, buttonHeight)
	m.cache.buttonBgSelected.Fill(style.Primary)

	// Create unselected button backgrounds
	for i := range m.cache.buttonBgs {
		m.cache.buttonBgs[i] = ebiten.NewImage(buttonWidth, buttonHeight)
		m.cache.buttonBgs[i].Fill(style.Surface)

		// Draw border
		for x := 0; x < buttonWidth; x++ {
			m.cache.buttonBgs[i].Set(x, 0, style.Border)
			m.cache.buttonBgs[i].Set(x, buttonHeight-1, style.Border)
		}
		for y := 0; y < buttonHeight; y++ {
			m.cache.buttonBgs[i].Set(0, y, style.Border)
			m.cache.buttonBgs[i].Set(buttonWidth-1, y, style.Border)
		}
	}
}

// Draw renders the pause menu
func (m *PauseMenu) Draw(screen *ebiten.Image) {
	if !m.visible {
		return
	}

	bounds := screen.Bounds()
	screenW := bounds.Dx()
	screenH := bounds.Dy()

	// Rebuild cache if screen dimensions or theme changed
	if m.cache.screenW != screenW || m.cache.screenH != screenH || m.cache.themeName != style.CurrentThemeName {
		m.rebuildCache(screenW, screenH)
	}

	// Draw dim overlay (reuse cached image)
	screen.DrawImage(m.cache.dimOverlay, nil)

	// Calculate positions using cached dimensions
	panelX := (screenW - m.cache.panelW) / 2
	panelY := (screenH - m.cache.panelH) / 2

	// Draw panel background (reuse cached image and draw options)
	m.drawOpts.GeoM.Reset()
	m.drawOpts.GeoM.Translate(float64(panelX), float64(panelY))
	screen.DrawImage(m.cache.panelBg, &m.drawOpts)

	// Draw menu options
	options := []string{"Resume", "Library", "Exit"}
	buttonSpacing := m.cache.buttonH / 4
	padding := m.cache.buttonH / 2
	startY := panelY + padding

	for i, optionText := range options {
		buttonX := panelX + (m.cache.panelW-m.cache.buttonW)/2
		buttonY := startY + i*(m.cache.buttonH+buttonSpacing)

		// Cache button rect for mouse hit testing
		m.buttonRects[i] = image.Rect(buttonX, buttonY, buttonX+m.cache.buttonW, buttonY+m.cache.buttonH)

		// Select appropriate cached button image
		var btnImg *ebiten.Image
		if i == m.selectedIndex {
			btnImg = m.cache.buttonBgSelected
		} else {
			btnImg = m.cache.buttonBgs[i]
		}

		m.drawOpts.GeoM.Reset()
		m.drawOpts.GeoM.Translate(float64(buttonX), float64(buttonY))
		screen.DrawImage(btnImg, &m.drawOpts)

		// Draw text centered (reuse text options)
		m.textOpts = text.DrawOptions{}
		m.textOpts.GeoM.Translate(float64(buttonX+m.cache.buttonW/2), float64(buttonY+m.cache.buttonH/2))
		m.textOpts.PrimaryAlign = text.AlignCenter
		m.textOpts.SecondaryAlign = text.AlignCenter
		m.textOpts.ColorScale.ScaleWithColor(style.Text)
		text.Draw(screen, optionText, *style.FontFace(), &m.textOpts)
	}
}
