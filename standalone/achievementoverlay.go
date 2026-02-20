//go:build !libretro

package standalone

import (
	"fmt"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/user-none/eblitui/standalone/achievements"
	"github.com/user-none/eblitui/standalone/style"
	"github.com/user-none/go-rcheevos"
)

// AchievementOverlay shows achievements during gameplay
type AchievementOverlay struct {
	visible bool
	manager *achievements.Manager

	// Badge loading state (actual cache is in manager)
	mu            sync.Mutex
	badgesPending map[uint32]bool
	// Local grayscale badge cache (cleared on close)
	grayscaleBadges map[uint32]*ebiten.Image

	// Scroll state
	scrollOffset float64
	scrollMax    float64
	visibleItems int

	// Cached images
	cache struct {
		screenW, screenH   int
		themeName          string
		fontScale          float64
		achCount           int
		panelW, panelH     int
		dimOverlay         *ebiten.Image
		panelBg            *ebiten.Image
		rowBg              *ebiten.Image
		rowBgWidth         int
		placeholderBadge   *ebiten.Image
		placeholderBadgeSz int
	}

	// Pre-allocated draw options
	drawOpts ebiten.DrawImageOptions
	textOpts text.DrawOptions
}

// NewAchievementOverlay creates a new achievement overlay
func NewAchievementOverlay(manager *achievements.Manager) *AchievementOverlay {
	o := &AchievementOverlay{
		manager:         manager,
		badgesPending:   make(map[uint32]bool),
		grayscaleBadges: make(map[uint32]*ebiten.Image),
	}
	// Register callback to clear grayscale cache when achievements unlock
	if manager != nil {
		manager.SetOnUnlockCallback(o.handleUnlock)
	}
	return o
}

// Show displays the achievement overlay
func (o *AchievementOverlay) Show() {
	o.visible = true
	o.scrollOffset = 0
	o.updateScrollMax()
}

// InitForGame prepares the overlay for a new game session.
// The manager already caches achievements on LoadGame, so this just resets overlay state.
func (o *AchievementOverlay) InitForGame() {
	o.mu.Lock()
	o.grayscaleBadges = make(map[uint32]*ebiten.Image)
	o.badgesPending = make(map[uint32]bool)
	o.mu.Unlock()
	o.scrollOffset = 0
}

// Hide hides the achievement overlay
func (o *AchievementOverlay) Hide() {
	o.visible = false
	// Clear grayscale cache
	o.mu.Lock()
	o.grayscaleBadges = make(map[uint32]*ebiten.Image)
	o.mu.Unlock()
}

// handleUnlock is called when an achievement is unlocked during gameplay.
// The manager updates its cached list; we just need to clear our grayscale cache.
func (o *AchievementOverlay) handleUnlock(achievementID uint32) {
	o.mu.Lock()
	delete(o.grayscaleBadges, achievementID)
	o.mu.Unlock()
}

// IsVisible returns whether the overlay is visible
func (o *AchievementOverlay) IsVisible() bool {
	return o.visible
}

// Reset clears session state when the game ends
func (o *AchievementOverlay) Reset() {
	o.mu.Lock()
	o.grayscaleBadges = make(map[uint32]*ebiten.Image)
	o.badgesPending = make(map[uint32]bool)
	o.mu.Unlock()
	o.scrollOffset = 0
}

// getAchievements returns the cached achievements from the manager
func (o *AchievementOverlay) getAchievements() []*rcheevos.Achievement {
	if o.manager == nil {
		return nil
	}
	return o.manager.GetCachedAchievements()
}

// getGameTitle returns the cached game title from the manager
func (o *AchievementOverlay) getGameTitle() string {
	if o.manager == nil {
		return ""
	}
	return o.manager.GetCachedGameTitle()
}

// computeSummary calculates summary stats from the cached achievements
func (o *AchievementOverlay) computeSummary(achievements []*rcheevos.Achievement) (numTotal, numUnlocked, pointsTotal, pointsUnlocked uint32) {
	for _, ach := range achievements {
		numTotal++
		pointsTotal += ach.Points
		if ach.Unlocked != rcheevos.AchievementUnlockedNone {
			numUnlocked++
			pointsUnlocked += ach.Points
		}
	}
	return
}

// updateScrollMax calculates the maximum scroll offset
func (o *AchievementOverlay) updateScrollMax() {
	achievements := o.getAchievements()
	count := len(achievements)

	if o.visibleItems > 0 && count > o.visibleItems {
		o.scrollMax = float64(count - o.visibleItems)
	} else {
		o.scrollMax = 0
	}
}

// Update handles input for the overlay
func (o *AchievementOverlay) Update() {
	if !o.visible {
		return
	}

	// ESC closes overlay
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		o.Hide()
		return
	}

	// Keyboard navigation
	scrollAmount := 0.0
	if ebiten.IsKeyPressed(ebiten.KeyUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
		scrollAmount = -1
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) || ebiten.IsKeyPressed(ebiten.KeyS) {
		scrollAmount = 1
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) {
		scrollAmount = -float64(o.visibleItems)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) {
		scrollAmount = float64(o.visibleItems)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) {
		o.scrollOffset = 0
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) {
		o.scrollOffset = o.scrollMax
		return
	}

	// Mouse wheel
	_, wheelY := ebiten.Wheel()
	if wheelY != 0 {
		scrollAmount = -wheelY * 2
	}

	// Gamepad support
	gamepadIDs := ebiten.AppendGamepadIDs(nil)
	for _, id := range gamepadIDs {
		// D-pad navigation
		if inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonLeftTop) {
			scrollAmount = -1
		}
		if inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonLeftBottom) {
			scrollAmount = 1
		}
		// B button closes
		if inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonRightRight) {
			o.Hide()
			return
		}
		// Start button closes
		if inpututil.IsStandardGamepadButtonJustPressed(id, ebiten.StandardGamepadButtonCenterRight) {
			o.Hide()
			return
		}
	}

	// Apply scroll
	if scrollAmount != 0 {
		o.scrollOffset += scrollAmount
		if o.scrollOffset < 0 {
			o.scrollOffset = 0
		}
		if o.scrollOffset > o.scrollMax {
			o.scrollOffset = o.scrollMax
		}
	}
}

// rebuildCache recreates cached images when screen dimensions or content change
func (o *AchievementOverlay) rebuildCache(screenW, screenH, achCount int) {
	// Deallocate old images
	if o.cache.dimOverlay != nil {
		o.cache.dimOverlay.Deallocate()
	}
	if o.cache.panelBg != nil {
		o.cache.panelBg.Deallocate()
	}
	if o.cache.rowBg != nil {
		o.cache.rowBg.Deallocate()
		o.cache.rowBg = nil
		o.cache.rowBgWidth = 0
	}
	if o.cache.placeholderBadge != nil {
		o.cache.placeholderBadge.Deallocate()
		o.cache.placeholderBadge = nil
		o.cache.placeholderBadgeSz = 0
	}

	o.cache.screenW = screenW
	o.cache.screenH = screenH
	o.cache.themeName = style.CurrentThemeName
	o.cache.fontScale = style.FontScale()
	o.cache.achCount = achCount

	// Create dim overlay
	o.cache.dimOverlay = ebiten.NewImage(screenW, screenH)
	dimColor := style.DimOverlay
	dimColor.A = 160
	o.cache.dimOverlay.Fill(dimColor)

	// Calculate panel dimensions (centered, scaled width)
	panelWidth := style.AchievementOverlayWidth
	if panelWidth > screenW-style.AchievementPanelMargin {
		panelWidth = screenW - style.AchievementPanelMargin
	}

	padding := style.AchievementOverlayPadding

	// Calculate header and footer heights from font measurements
	_, fontH := text.Measure("M", *style.FontFace(), 0)
	lineGap := int(fontH * 0.4)
	// Reserve space for: title + spectator line + summary + separator gap
	headerHeight := (int(fontH)+lineGap)*3 + lineGap
	// Footer: gap + close hint text + bottom margin
	footerHeight := lineGap + int(fontH) + lineGap

	// Size panel to content: header + items + footer + padding
	itemsHeight := achCount * style.AchievementRowHeight
	if achCount == 0 {
		// Reserve space for "not available" message
		itemsHeight = int(fontH) + lineGap*2
	}
	panelHeight := padding*2 + headerHeight + itemsHeight + footerHeight

	// Cap at 70% of screen height
	maxHeight := screenH * 70 / 100
	if panelHeight > maxHeight {
		panelHeight = maxHeight
	}
	if panelHeight < style.AchievementMinPanelHeight {
		panelHeight = style.AchievementMinPanelHeight
	}

	o.cache.panelW = panelWidth
	o.cache.panelH = panelHeight

	// Calculate visible items from available content space
	contentHeight := panelHeight - padding*2 - headerHeight - footerHeight
	o.visibleItems = contentHeight / style.AchievementRowHeight
	if o.visibleItems < 1 {
		o.visibleItems = 1
	}
	o.updateScrollMax()

	// Create panel background
	o.cache.panelBg = ebiten.NewImage(panelWidth, panelHeight)
	o.cache.panelBg.Fill(style.Surface)

	// Draw panel border
	for x := 0; x < panelWidth; x++ {
		o.cache.panelBg.Set(x, 0, style.Border)
		o.cache.panelBg.Set(x, panelHeight-1, style.Border)
	}
	for y := 0; y < panelHeight; y++ {
		o.cache.panelBg.Set(0, y, style.Border)
		o.cache.panelBg.Set(panelWidth-1, y, style.Border)
	}
}

// Draw renders the overlay
func (o *AchievementOverlay) Draw(screen *ebiten.Image) {
	if !o.visible {
		return
	}

	bounds := screen.Bounds()
	screenW := bounds.Dx()
	screenH := bounds.Dy()

	// Rebuild cache if screen dimensions, theme, font size, or achievement count changed
	achCount := len(o.getAchievements())
	if o.cache.screenW != screenW || o.cache.screenH != screenH || o.cache.themeName != style.CurrentThemeName || o.cache.fontScale != style.FontScale() || o.cache.achCount != achCount {
		o.rebuildCache(screenW, screenH, achCount)
	}

	// Draw dim overlay
	screen.DrawImage(o.cache.dimOverlay, nil)

	// Calculate panel position (centered)
	panelX := (screenW - o.cache.panelW) / 2
	panelY := (screenH - o.cache.panelH) / 2

	// Draw panel background
	o.drawOpts.GeoM.Reset()
	o.drawOpts.GeoM.Translate(float64(panelX), float64(panelY))
	screen.DrawImage(o.cache.panelBg, &o.drawOpts)

	padding := style.AchievementOverlayPadding
	contentX := panelX + padding
	contentW := o.cache.panelW - padding*2

	// Font measurements for layout
	_, fontH := text.Measure("M", *style.FontFace(), 0)
	lineGap := int(fontH * 0.4)

	// Get data from manager's cache
	achievements := o.getAchievements()
	gameTitle := o.getGameTitle()

	// Draw title
	title := "Achievements"
	if gameTitle != "" {
		title = gameTitle
	}

	// Truncate title if wider than content area
	titleW, _ := text.Measure(title, *style.FontFace(), 0)
	if titleW > float64(contentW) {
		title, _ = style.TruncateToWidth(title, *style.FontFace(), float64(contentW))
	}

	curY := panelY + padding

	o.textOpts = text.DrawOptions{}
	o.textOpts.GeoM.Translate(float64(contentX+contentW/2), float64(curY))
	o.textOpts.PrimaryAlign = text.AlignCenter
	o.textOpts.ColorScale.ScaleWithColor(style.Text)
	text.Draw(screen, title, *style.FontFace(), &o.textOpts)
	curY += int(fontH) + lineGap

	// Draw spectator mode indicator if enabled
	spectatorMode := o.manager.IsSpectatorMode()
	if spectatorMode {
		o.textOpts = text.DrawOptions{}
		o.textOpts.GeoM.Translate(float64(contentX+contentW/2), float64(curY))
		o.textOpts.PrimaryAlign = text.AlignCenter
		o.textOpts.ColorScale.ScaleWithColor(style.Accent)
		text.Draw(screen, "[SPECTATOR MODE]", *style.FontFace(), &o.textOpts)
		curY += int(fontH) + lineGap
	}

	// Draw summary
	if len(achievements) > 0 {
		numTotal, numUnlocked, pointsTotal, pointsUnlocked := o.computeSummary(achievements)
		pct := 0
		if numTotal > 0 {
			pct = int(numUnlocked * 100 / numTotal)
		}
		summaryText := fmt.Sprintf("Progress: %d/%d (%d%%)    Points: %d/%d",
			numUnlocked, numTotal, pct, pointsUnlocked, pointsTotal)

		// Truncate summary if wider than content area
		summaryW, _ := text.Measure(summaryText, *style.FontFace(), 0)
		if summaryW > float64(contentW) {
			summaryText, _ = style.TruncateToWidth(summaryText, *style.FontFace(), float64(contentW))
		}

		o.textOpts = text.DrawOptions{}
		o.textOpts.GeoM.Translate(float64(contentX+contentW/2), float64(curY))
		o.textOpts.PrimaryAlign = text.AlignCenter
		o.textOpts.ColorScale.ScaleWithColor(style.TextSecondary)
		text.Draw(screen, summaryText, *style.FontFace(), &o.textOpts)
		curY += int(fontH) + lineGap
	}

	// Draw separator line
	for x := contentX; x < contentX+contentW; x++ {
		screen.Set(x, curY, style.Border)
	}
	curY += lineGap

	// Draw achievement list or "not available" message
	listY := curY

	if len(achievements) == 0 {
		// Show message when no achievements are available
		centerY := (panelY + o.cache.panelH) / 2
		o.textOpts = text.DrawOptions{}
		o.textOpts.GeoM.Translate(float64(contentX+contentW/2), float64(centerY))
		o.textOpts.PrimaryAlign = text.AlignCenter
		o.textOpts.ColorScale.ScaleWithColor(style.TextSecondary)
		text.Draw(screen, "Achievements not available", *style.FontFace(), &o.textOpts)
	} else {
		startIdx := int(o.scrollOffset)
		endIdx := startIdx + o.visibleItems
		if endIdx > len(achievements) {
			endIdx = len(achievements)
		}

		for i := startIdx; i < endIdx; i++ {
			ach := achievements[i]
			rowY := listY + (i-startIdx)*style.AchievementRowHeight

			// Skip if row would be below panel
			if rowY+style.AchievementRowHeight > panelY+o.cache.panelH-padding {
				break
			}

			o.drawAchievementRow(screen, ach, contentX, rowY, contentW)
		}
	}

	// Draw close hint at bottom
	hintY := panelY + o.cache.panelH - padding - int(fontH)
	o.textOpts = text.DrawOptions{}
	o.textOpts.GeoM.Translate(float64(contentX+contentW/2), float64(hintY))
	o.textOpts.PrimaryAlign = text.AlignCenter
	o.textOpts.ColorScale.ScaleWithColor(style.TextSecondary)
	text.Draw(screen, "[ESC] to close", *style.FontFace(), &o.textOpts)
}

// drawAchievementRow draws a single achievement row
func (o *AchievementOverlay) drawAchievementRow(screen *ebiten.Image, ach *rcheevos.Achievement, x, y, width int) {
	if y < 0 || y > o.cache.screenH {
		return
	}

	isUnlocked := ach.Unlocked != rcheevos.AchievementUnlockedNone
	badgeSize := style.AchievementBadgeSize

	// Font measurements for row layout
	_, fontH := text.Measure("M", *style.FontFace(), 0)
	rowPad := int(fontH * 0.3)
	rowBgHeight := style.AchievementRowHeight - style.AchievementRowSpacing

	// Text area starts after badge + padding
	textX := x + rowPad + badgeSize + rowPad
	// Points text width for title truncation
	pointsText := fmt.Sprintf("%d pts", ach.Points)
	pointsW, _ := text.Measure(pointsText, *style.FontFace(), 0)
	// Description gets full text width, title reserves space for points
	descWidth := float64(x+width-rowPad) - float64(textX)
	titleWidth := descWidth - pointsW - float64(rowPad)

	// Draw row background - use cached image
	if o.cache.rowBg == nil || o.cache.rowBgWidth != width {
		if o.cache.rowBg != nil {
			o.cache.rowBg.Deallocate()
		}
		o.cache.rowBg = ebiten.NewImage(width, rowBgHeight)
		o.cache.rowBg.Fill(style.Background)
		o.cache.rowBgWidth = width
	}

	o.drawOpts.GeoM.Reset()
	o.drawOpts.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(o.cache.rowBg, &o.drawOpts)

	// Draw badge - vertically centered in row
	badgeY := y + (rowBgHeight-badgeSize)/2
	o.drawBadge(screen, ach, x+rowPad, badgeY, badgeSize)

	// Title - unlocked uses primary text, locked uses secondary
	titleColor := style.Text
	if !isUnlocked {
		titleColor = style.TextSecondary
	}
	titleText, _ := style.TruncateToWidth(ach.Title, *style.FontFace(), titleWidth)

	// Vertically center the text block (title + gap + description) within the row
	textGap := rowPad / 2
	textBlockH := int(fontH)*2 + textGap
	titleY := y + (rowBgHeight-textBlockH)/2

	o.textOpts = text.DrawOptions{}
	o.textOpts.GeoM.Translate(float64(textX), float64(titleY))
	o.textOpts.ColorScale.ScaleWithColor(titleColor)
	text.Draw(screen, titleText, *style.FontFace(), &o.textOpts)

	// Description (truncate if needed)
	desc, _ := style.TruncateToWidth(ach.Description, *style.FontFace(), descWidth)
	descY := titleY + int(fontH) + textGap
	o.textOpts = text.DrawOptions{}
	o.textOpts.GeoM.Translate(float64(textX), float64(descY))
	o.textOpts.ColorScale.ScaleWithColor(style.TextSecondary)
	text.Draw(screen, desc, *style.FontFace(), &o.textOpts)

	// Points (right-aligned) - same Y as title
	pointsColor := style.TextSecondary
	if isUnlocked {
		pointsColor = style.Primary
	}
	o.textOpts = text.DrawOptions{}
	o.textOpts.GeoM.Translate(float64(x+width-rowPad), float64(titleY))
	o.textOpts.PrimaryAlign = text.AlignEnd
	o.textOpts.ColorScale.ScaleWithColor(pointsColor)
	text.Draw(screen, pointsText, *style.FontFace(), &o.textOpts)
}

// drawBadge draws an achievement badge, fetching it async if not cached in manager
// Applies grayscale effect for locked achievements
func (o *AchievementOverlay) drawBadge(screen *ebiten.Image, ach *rcheevos.Achievement, x, y, size int) {
	isUnlocked := ach.Unlocked != rcheevos.AchievementUnlockedNone

	// Check manager's badge cache (always stores colored version)
	if o.manager != nil {
		badge := o.manager.GetBadgeImage(ach.ID)
		if badge != nil {
			// For locked achievements, use cached grayscale version
			if !isUnlocked {
				o.mu.Lock()
				grayBadge, exists := o.grayscaleBadges[ach.ID]
				o.mu.Unlock()

				if !exists {
					// Create and cache grayscale version
					grayBadge = style.ApplyGrayscale(badge)
					o.mu.Lock()
					o.grayscaleBadges[ach.ID] = grayBadge
					o.mu.Unlock()
				}
				badge = grayBadge
			}

			// Scale from 64x64 (badge size) to target size using GeoM
			bounds := badge.Bounds()
			scaleX := float64(size) / float64(bounds.Dx())
			scaleY := float64(size) / float64(bounds.Dy())

			o.drawOpts.GeoM.Reset()
			o.drawOpts.GeoM.Scale(scaleX, scaleY)
			o.drawOpts.GeoM.Translate(float64(x), float64(y))
			screen.DrawImage(badge, &o.drawOpts)
			return
		}
	}

	// Draw placeholder while loading - use cached image
	if o.cache.placeholderBadge == nil || o.cache.placeholderBadgeSz != size {
		if o.cache.placeholderBadge != nil {
			o.cache.placeholderBadge.Deallocate()
		}
		o.cache.placeholderBadge = ebiten.NewImage(size, size)
		o.cache.placeholderBadge.Fill(style.Border)
		o.cache.placeholderBadgeSz = size
	}
	o.drawOpts.GeoM.Reset()
	o.drawOpts.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(o.cache.placeholderBadge, &o.drawOpts)

	// Start async fetch if not already pending - stores in manager's cache
	o.mu.Lock()
	pending := o.badgesPending[ach.ID]
	o.mu.Unlock()

	if o.manager != nil && !pending {
		o.mu.Lock()
		o.badgesPending[ach.ID] = true
		o.mu.Unlock()

		achID := ach.ID
		o.manager.GetBadgeImageAsync(achID, func(img *ebiten.Image) {
			// Badge is now in manager's cache, clear pending flag
			o.mu.Lock()
			delete(o.badgesPending, achID)
			o.mu.Unlock()
		})
	}
}
