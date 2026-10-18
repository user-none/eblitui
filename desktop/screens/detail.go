// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"fmt"
	goimage "image"
	"log"
	"os"
	"path/filepath"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/desktop/achievements"
	"github.com/user-none/eblitui/desktop/cheats"
	"github.com/user-none/eblitui/desktop/screens/detail"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// DetailScreen displays game information and launch options
type DetailScreen struct {
	BaseScreen // Embedded for focus restoration

	callback ScreenCallback
	library  *storage.Library
	game     *storage.GameEntry
	files    storage.FileStatus // Which of the game's files are missing, checked in SetGame

	pages sidebarSections // Page sidebar list and selection

	// Encapsulated pages
	overviewPage    *detail.OverviewSection
	cheatsPage      *detail.CheatsSection // nil when the core has no cheat support
	settingsPage    *detail.SettingsSection
	screenshotsPage *detail.ScreenshotsSection
}

// cheatsPageKey, settingsPageKey, and screenshotsPageKey are the focus keys
// of the Cheats, Settings, and Screenshots pages in the sidebar
const (
	cheatsPageKey      = "page-cheats"
	settingsPageKey    = "page-settings"
	screenshotsPageKey = "page-screenshots"
)

// NewDetailScreen creates a new detail screen. cheatSystem is the core's
// cheat system, or 0 when the core has no cheat support; the Cheats page
// is listed only when it is set. cheatBigEndian is the byte order of the
// core's memory, which cheats are converted to. coreOptions are the
// core's options, of which the Settings page lists the per game ones.
func NewDetailScreen(callback ScreenCallback, library *storage.Library, achievementManager *achievements.Manager, defaultConsoleID int, cheatSystem cheats.System, cheatBigEndian bool, coreOptions []coreif.CoreOption) *DetailScreen {
	s := &DetailScreen{
		callback: callback,
		library:  library,
	}
	s.InitBase()
	s.overviewPage = detail.NewOverviewSection(callback, library, achievementManager, defaultConsoleID, s.contentWidth)

	s.pages.sections = []sectionDescriptor{
		{label: "Overview", focusKey: "page-overview", build: s.overviewPage.Build, setupNav: s.setupOverviewNav},
	}
	// Cheats is listed only when the core supports cheats
	if cheatSystem != 0 {
		s.cheatsPage = detail.NewCheatsSection(callback, cheatSystem, cheatBigEndian, s.contentWidth)
		s.pages.sections = append(s.pages.sections, sectionDescriptor{label: "Cheats", focusKey: cheatsPageKey, build: s.cheatsPage.Build, setupNav: s.setupCheatsNav})
	}
	s.settingsPage = detail.NewSettingsSection(callback, coreOptions, s.contentWidth)
	s.pages.sections = append(s.pages.sections, sectionDescriptor{label: "Settings", focusKey: settingsPageKey, build: s.settingsPage.Build, setupNav: s.setupSettingsNav})
	s.screenshotsPage = detail.NewScreenshotsSection(callback, s.contentWidth)
	s.pages.sections = append(s.pages.sections, sectionDescriptor{label: "Screenshots", focusKey: screenshotsPageKey, build: s.screenshotsPage.Build, setupNav: s.setupScreenshotsNav})
	return s
}

// SetGame sets the game to display
func (s *DetailScreen) SetGame(gameCRC string) {
	s.game = s.library.GetGame(gameCRC)
	s.files = storage.FileStatus{}
	if s.game != nil {
		s.files = s.game.CheckFiles()
	}

	// A different game starts on Overview at the top. The scroll container
	// belongs to the previous game's build, so drop it to keep its offset
	// from being saved and restored onto this game.
	s.pages.selected = 0
	s.scrollContainer = nil
	s.scrollTop = 0

	// The Cheats and Settings pages edit the same settings file, so they
	// share one copy and neither saves over the other's changes
	var gameSettings *storage.GameSettingsFile
	if s.game != nil {
		var err error
		gameSettings, err = storage.LoadGameSettings(s.game.CRC32)
		if err != nil {
			log.Printf("Failed to load game settings for %s: %v", s.game.CRC32, err)
			gameSettings = &storage.GameSettingsFile{}
		}
	}

	s.overviewPage.SetGame(s.game, s.files)
	if s.cheatsPage != nil {
		s.cheatsPage.SetGame(s.game, gameSettings)
	}
	s.settingsPage.SetGame(s.game, gameSettings)
	s.screenshotsPage.SetGame(s.game)
}

// Build creates the detail screen UI
func (s *DetailScreen) Build() *widget.Container {
	s.ClearFocusButtons()
	s.updateCheatsLabel()

	rootContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Background)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(style.DefaultPadding)),
			widget.GridLayoutOpts.Spacing(0, style.DefaultSpacing),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true}),
		)),
	)

	// Toolbar: Back button on left, action buttons on right
	toolbar := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{true, false}, nil),
			widget.GridLayoutOpts.Spacing(style.SmallSpacing, 0),
		)),
	)

	toolbarLeft := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
		)),
	)
	backButton := widgets.TextButton("Back", style.ButtonPaddingSmall, func(args *widget.ButtonClickedEventArgs) {
		s.callback.SwitchToLibrary()
	})
	s.RegisterFocusButton("detail-back", backButton)
	toolbarLeft.AddChild(backButton)
	toolbar.AddChild(toolbarLeft)

	toolbarRight := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	toolbar.AddChild(toolbarRight)
	rootContainer.AddChild(toolbar)

	if s.game == nil {
		errorLabel := widget.NewText(
			widget.TextOpts.Text("Game not found", style.FontFace(), style.Text),
		)
		rootContainer.AddChild(errorLabel)
		return rootContainer
	}

	// Action buttons in toolbar right section
	buttonContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(style.DefaultSpacing),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionEnd,
			}),
		),
	)

	hasResume := s.hasResumeState()

	if s.playable() {
		playButton := widgets.PrimaryTextButton("Play", style.ButtonPaddingMedium, func(args *widget.ButtonClickedEventArgs) {
			s.callback.LaunchGame(s.game.CRC32, false, -1)
		})
		s.RegisterFocusButton("play", playButton)
		buttonContainer.AddChild(playButton)

		resumeButton := widgets.EnabledTextButton("Resume", style.ButtonPaddingMedium, hasResume, func(args *widget.ButtonClickedEventArgs) {
			s.callback.LaunchGame(s.game.CRC32, true, -1)
		})
		s.RegisterFocusButton("resume", resumeButton)
		buttonContainer.AddChild(resumeButton)
	} else {
		// No file at all removes the game; a missing selected disc
		// removes that disc
		label, handler := "Remove Disc", s.removeSelectedDisc
		if s.files.Missing {
			label, handler = "Remove from Library", s.removeGame
		}
		removeButton := widgets.TextButton(label, style.ButtonPaddingMedium, func(args *widget.ButtonClickedEventArgs) {
			handler()
		})
		s.RegisterFocusButton("remove", removeButton)
		buttonContainer.AddChild(removeButton)
	}

	favText := "Add to Favorites"
	if s.game.Favorite {
		favText = "Remove from Favorites"
	}
	favButton := widgets.TextButton(favText, style.ButtonPaddingMedium, func(args *widget.ButtonClickedEventArgs) {
		s.game.Favorite = !s.game.Favorite
		storage.SaveLibrary(s.library)
		s.callback.RequestRebuild()
	})
	s.RegisterFocusButton("favorite", favButton)
	buttonContainer.AddChild(favButton)

	toolbarRight.AddChild(buttonContainer)

	// Main area: game column on the left, selected page stretched on the right
	mainContent := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
		)),
	)
	mainContent.AddChild(s.buildGameColumn())
	if content := s.pages.buildContent(s); content != nil {
		mainContent.AddChild(content)
	}
	rootContainer.AddChild(mainContent)

	s.setupNavigation()

	return rootContainer
}

// detailColumnFraction is the share of the window width the game column
// takes, between its minimum and maximum widths
const detailColumnFraction = 5

// detailColumnMaxWidth is the widest the game column grows, in logical
// pixels before scaling. Box art from the thumbnail repositories is about
// 512 pixels on its long side, so at scale 1.0 the column shows it at or
// below its native size. The cap grows with the window scale, so on a
// large display the art upscales along with the rest of the UI.
const detailColumnMaxWidth = 400

// detailSideBySideWidth is the window width, in logical pixels before DPI
// scaling, from which the page sidebar sits beside the box art instead of
// under it. Narrower windows stack them and have no width to spare for a
// wider column. It measures room in the window, so it does not grow with
// the window scale.
const detailSideBySideWidth = 1200

// sideBySide reports whether the sidebar goes beside the art rather than
// under it
func (s *DetailScreen) sideBySide() bool {
	return s.callback.GetWindowWidth() >= style.PxDPI(detailSideBySideWidth)
}

// artWidth returns the box art width: a fifth of the window, kept between
// DetailColumnWidth and detailColumnMaxWidth
func (s *DetailScreen) artWidth() int {
	width := s.callback.GetWindowWidth() / detailColumnFraction
	return min(max(width, style.DetailColumnWidth), style.Px(detailColumnMaxWidth))
}

// columnWidth returns the game column width. Stacked, the art and the
// sidebar share it: the art width, or wider when a page label needs more
// room. Side by side, it is the sidebar at its fit width, the spacing, and
// the art.
func (s *DetailScreen) columnWidth() int {
	if s.sideBySide() {
		return s.pages.sidebarWidth(style.SidebarMinWidth) + style.DefaultSpacing + s.artWidth()
	}
	return s.pages.sidebarWidth(s.artWidth())
}

// contentWidth returns the width of the page content area: the window
// minus the root padding and the game column
func (s *DetailScreen) contentWidth() int {
	return s.callback.GetWindowWidth() - style.DefaultPadding*2 - s.columnWidth() - style.DefaultSpacing
}

// buildGameColumn creates the left column shown on every page: the box art
// and the page sidebar. In a narrow window the sidebar sits directly under
// the art, and on a window too short for the full 3:4 box the art shrinks
// instead of pushing the sidebar off the bottom. From
// detailSideBySideWidth the sidebar sits beside the art on the left, so
// the art no longer limits how many pages fit.
func (s *DetailScreen) buildGameColumn() *widget.Container {
	rebuild := func() { s.callback.RequestRebuild() }

	if !s.sideBySide() {
		width := s.columnWidth()
		column := widget.NewContainer(
			widget.ContainerOpts.Layout(widgets.NewShrinkFirstLayout(style.DefaultSpacing)),
		)
		column.AddChild(s.buildArtBox(width))
		column.AddChild(s.pages.buildSidebar(&s.BaseScreen, width, rebuild))
		return column
	}

	// The sidebar panel runs the full height like the settings sidebar.
	// The art box keeps to the top and takes the height it has, up to 3:4.
	column := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{false, false}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
		)),
	)
	column.AddChild(s.pages.buildSidebar(&s.BaseScreen, s.pages.sidebarWidth(style.SidebarMinWidth), rebuild))

	artWidth := s.artWidth()
	art := s.buildArtBox(artWidth)
	art.GetWidget().MinHeight = 0
	art.GetWidget().LayoutData = widget.GridLayoutData{MaxHeight: artWidth * 4 / 3}
	column.AddChild(art)
	return column
}

// buildArtBox creates the box art at the given width in a 3:4 black box.
// The art is drawn to fit the box, so it keeps its aspect ratio when the
// column layout shrinks the box on a short window. Without artwork the
// game's name is shown in the box instead.
func (s *DetailScreen) buildArtBox(artWidth int) *widget.Container {
	artHeight := artWidth * 4 / 3 // 3:4 aspect ratio for box art

	// Box art container with black background (per design spec)
	artContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Black)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(artWidth, artHeight),
		),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)

	// Try to load box art image (scaled to fit)
	artImage := s.loadBoxArtScaled(artWidth, artHeight)
	if artImage != nil {
		// Display the actual artwork, fit to whatever the box ends up
		artContainer.AddChild(widgets.NewFitGraphic(artImage, widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			StretchHorizontal: true,
			StretchVertical:   true,
		})))
	} else {
		// Show placeholder text if no artwork. Wrap and truncate the
		// title so it fits inside the fixed box instead of stretching
		// it. Inset by a small padding so text does not touch edges.
		pad := style.SmallSpacing
		placeholderText := style.WrapToBox(
			s.game.DisplayName, *style.FontFace(),
			float64(artWidth-pad*2), float64(artHeight-pad*2),
		)
		artPlaceholder := widget.NewText(
			widget.TextOpts.Text(placeholderText, style.FontFace(), style.TextSecondary),
			widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionCenter,
				VerticalPosition:   widget.AnchorLayoutPositionCenter,
			})),
		)
		artContainer.AddChild(artPlaceholder)
	}
	return artContainer
}

// cheatsLabel returns the Cheats page label for n cheats on: "Cheats (n)",
// or "Cheats" when none are
func cheatsLabel(n int) string {
	if n > 0 {
		return fmt.Sprintf("Cheats (%d)", n)
	}
	return "Cheats"
}

// updateCheatsLabel sets the Cheats page label to show how many cheats are
// on. It runs at the start of Build, before the sidebar is measured.
func (s *DetailScreen) updateCheatsLabel() {
	if s.cheatsPage == nil {
		return
	}
	label := cheatsLabel(s.cheatsPage.EnabledCount())
	for i := range s.pages.sections {
		if s.pages.sections[i].focusKey == cheatsPageKey {
			s.pages.sections[i].label = label
		}
	}
}

// EnsureFocusedVisible scrolls the page content to keep the focused widget
// visible
func (s *DetailScreen) EnsureFocusedVisible(focused widget.Focuser) {
	s.BaseScreen.EnsureFocusedVisible(focused, nil)
}

// playable reports whether the selected disc, or the cartridge, has its
// file. When it does not, the toolbar offers Remove in place of Play.
func (s *DetailScreen) playable() bool {
	return !s.files.MissingAt(s.game.SelectedDiscIndex())
}

// actionKey returns the focus key of the toolbar action: Play, or Remove
// when the selected file is missing
func (s *DetailScreen) actionKey() string {
	if s.playable() {
		return "play"
	}
	return "remove"
}

// removeGame moves the game out of the library into the removed games and
// returns to the library
func (s *DetailScreen) removeGame() {
	if err := s.library.RemoveGames(s.game.CRC32); err != nil {
		log.Printf("Failed to remove game: %v", err)
		s.callback.ShowNotification("Failed to remove game")
		return
	}
	s.callback.SwitchToLibrary()
}

// removeSelectedDisc removes the selected disc, whose file is missing, and
// refreshes the screen for the remaining discs
func (s *DetailScreen) removeSelectedDisc() {
	s.game.RemoveDisc(s.game.SelectedDiscIndex())
	if err := storage.SaveLibrary(s.library); err != nil {
		log.Printf("Failed to save library: %v", err)
	}
	s.files = s.game.CheckFiles()
	s.overviewPage.SetGame(s.game, s.files)
	s.SetPendingFocus(s.actionKey())
	s.callback.RequestRebuild()
}

// toolbarActionIndex is the toolbar zone index of Play, or Remove when the
// selected file is missing, directly after Back
const toolbarActionIndex = 1

// setupNavigation registers the detail screen navigation zones. The toolbar
// sits above the game column's page sidebar; the selected page adds its own
// zones.
func (s *DetailScreen) setupNavigation() {
	toolbarKeys := []string{"detail-back", "play", "resume", "favorite"}
	if !s.playable() {
		toolbarKeys = []string{"detail-back", "remove", "favorite"}
	}
	s.RegisterNavZone("toolbar", types.NavZoneHorizontal, toolbarKeys, 0)
	s.pages.setupNav(&s.BaseScreen)
	s.SetNavTransition("toolbar", types.DirDown, sidebarZone, types.NavIndexFirst)
	s.SetNavTransition(sidebarZone, types.DirUp, "toolbar", types.NavIndexFirst)
}

func (s *DetailScreen) setupOverviewNav() {
	s.setupPageNav(s.overviewPage.NavZones())
}

func (s *DetailScreen) setupCheatsNav() {
	s.setupPageNav(s.cheatsPage.NavZones())
}

func (s *DetailScreen) setupSettingsNav() {
	s.setupPageNav(s.settingsPage.NavZones())
}

// setupPageNav links the selected page's zones, listed from the top, to
// the sidebar on the left and the toolbar above the first zone. Right
// from the sidebar enters the first zone. Moves between the zones are the
// page's own.
func (s *DetailScreen) setupPageNav(zones []string) {
	if len(zones) == 0 {
		return
	}
	s.SetNavTransition(sidebarZone, types.DirRight, zones[0], types.NavIndexFirst)
	s.SetNavTransition(zones[0], types.DirUp, "toolbar", toolbarActionIndex)
	for _, zone := range zones {
		s.SetNavTransition(zone, types.DirLeft, sidebarZone, types.NavIndexFirst)
	}
}

// setupScreenshotsNav links the Screenshots page: the list, its first
// zone, is entered from the sidebar at the selected row and leaves up to
// the toolbar or left to the sidebar. The action row beside the list also
// leaves up to the toolbar; its way back to the list is the page's own.
func (s *DetailScreen) setupScreenshotsNav() {
	zones := s.screenshotsPage.NavZones()
	if len(zones) == 0 {
		return
	}
	s.SetNavTransition(sidebarZone, types.DirRight, zones[0], s.screenshotsPage.SelectedRow())
	s.SetNavTransition(zones[0], types.DirUp, "toolbar", toolbarActionIndex)
	s.SetNavTransition(zones[0], types.DirLeft, sidebarZone, types.NavIndexFirst)
	for _, zone := range zones[1:] {
		s.SetNavTransition(zone, types.DirUp, "toolbar", toolbarActionIndex)
	}
}

// Update runs each frame while the detail screen is shown
func (s *DetailScreen) Update() {
	if s.cheatsPage != nil && s.pages.currentKey() == cheatsPageKey {
		s.cheatsPage.Update()
	}
}

// HandleBack handles Back on the selected page, reporting whether it was
// used there. Back closes an open cheat form or cancels an armed cheat or
// screenshot delete.
func (s *DetailScreen) HandleBack() bool {
	switch s.pages.currentKey() {
	case cheatsPageKey:
		return s.cheatsPage != nil && s.cheatsPage.HandleBack()
	case screenshotsPageKey:
		return s.screenshotsPage.HandleBack()
	}
	return false
}

// loadBoxArtScaled loads and scales the box art image for the current game
func (s *DetailScreen) loadBoxArtScaled(maxWidth, maxHeight int) *ebiten.Image {
	if s.game == nil {
		return nil
	}

	artworkPath, err := storage.GetGameArtworkPath(s.game.CRC32)
	if err != nil {
		return nil
	}

	f, err := os.Open(artworkPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	img, _, err := goimage.Decode(f)
	if err != nil {
		return nil
	}

	return style.ScaleImageUnmanaged(img, maxWidth, maxHeight)
}

// hasResumeState checks if a resume state exists for the current game
func (s *DetailScreen) hasResumeState() bool {
	saveDir, err := storage.GetGameSaveDir(s.game.CRC32)
	if err != nil {
		return false
	}
	resumePath := filepath.Join(saveDir, "resume.state")
	_, err = os.Stat(resumePath)
	return err == nil
}

// OnEnter is called when entering the detail screen
func (s *DetailScreen) OnEnter() {
	if s.game != nil {
		s.SetPendingFocus(s.actionKey())
	}
}

// OnExit is called when leaving the detail screen. It frees the screenshot
// preview texture, which SetGame recreates on return.
func (s *DetailScreen) OnExit() {
	s.screenshotsPage.ReleasePreview()
}
