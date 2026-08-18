// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"strings"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// iconArtwork holds the artwork texture for an icon view card. The zoom and
// dim effect is applied at draw time by IconGraphic, so a single texture
// serves both the focused and unfocused states.
type iconArtwork struct {
	image *ebiten.Image // Full size (100%)
}

// artRef holds mutable references to a game card's artwork and widgets.
// Closures in buildGameCardSized reference the artRef fields, so updating
// them here updates what the closures see without rebuilding the widget tree.
type artRef struct {
	image   *ebiten.Image
	graphic *widgets.IconGraphic
	button  *widget.Button
	hovered bool
}

// LibraryScreen displays the game library
type LibraryScreen struct {
	BaseScreen // Embedded for focus restoration

	callback ScreenCallback
	library  *storage.Library
	config   *storage.Config

	// UI state
	selectedIndex int
	games         []*storage.GameEntry

	// Selection and scroll preservation (independent for each view)
	iconSelectedCRC string  // CRC of selected game in icon view
	listSelectedCRC string  // CRC of selected game in list view
	iconScrollTop   float64 // Scroll position for icon view
	listScrollTop   float64 // Scroll position for list view

	// Scroll containers for scroll preservation (dual view mode)
	iconScrollContainer *widgets.ScrollView
	listScrollContainer *widgets.ScrollView

	// Set when the selected icon-view card still needs to be top-aligned once
	// real layout rects are known (one frame after a rebuild). The card is
	// identified by iconSelectedCRC.
	pendingIconScroll bool

	// Async artwork loader
	artLoader *artworkLoader
	artRefs   map[string]*artRef

	// Search filter
	searchText string
}

// NewLibraryScreen creates a new library screen
func NewLibraryScreen(callback ScreenCallback, library *storage.Library, config *storage.Config) *LibraryScreen {
	s := &LibraryScreen{
		callback:      callback,
		library:       library,
		config:        config,
		selectedIndex: 0,
	}
	s.artLoader = newArtworkLoader(callback.GetPlaceholderImageData(), callback.GetMissingArtImageData())
	s.InitBase()
	return s
}

// SetLibrary updates the library reference
func (s *LibraryScreen) SetLibrary(library *storage.Library) {
	s.library = library
}

// SetConfig updates the config reference
func (s *LibraryScreen) SetConfig(config *storage.Config) {
	s.config = config
}

// UpdateArtwork checks if new artwork has been cached by the background loader
// and updates existing widget graphics in place without rebuilding the screen.
func (s *LibraryScreen) UpdateArtwork() {
	if !s.artLoader.HaveNew() {
		return
	}

	for crc, ref := range s.artRefs {
		cached := s.artLoader.Get(crc)
		if cached == nil {
			continue
		}
		ref.image = cached.image
		ref.graphic.Image = cached.image
		ref.graphic.Focused = ref.button.IsFocused() || ref.hovered
		delete(s.artRefs, crc)
	}
}

// HaltArtworkLoader permanently stops the background artwork loading goroutine.
// After halting, Start is a no-op.
func (s *LibraryScreen) HaltArtworkLoader() {
	s.artLoader.Halt()
}

// ClearArtworkCache clears the cached artwork images.
// Should be called after library scan or when library locations change.
func (s *LibraryScreen) ClearArtworkCache() {
	s.artLoader.CancelAndClear()
}

// Build creates the library screen UI
func (s *LibraryScreen) Build() *widget.Container {
	// Clear button references for fresh build
	s.ClearFocusButtons()

	// Get sorted and filtered games
	s.games = s.library.GetGamesSortedFiltered(s.config.Library.SortBy, s.config.Library.FavoritesFilter, s.searchText)

	// Check if library is truly empty vs filtered empty
	totalGames := s.library.GameCount()

	// Use standard screen container pattern
	rootContainer := widgets.ScreenContainer()

	// Track grid columns for navigation
	gridColumns := 1

	if totalGames == 0 {
		// Library is truly empty - single row that stretches to fill
		innerContainer := widgets.ScreenContentContainer([]bool{true})
		innerContainer.AddChild(s.buildEmptyState())
		rootContainer.AddChild(innerContainer)
		return rootContainer
	}

	innerContainer := widgets.ScreenContentContainer([]bool{false, true}) // toolbar=fixed, content=stretch

	if len(s.games) == 0 {
		// Library has games but filter/search shows none
		innerContainer.AddChild(s.buildToolbar())
		if s.searchText != "" {
			innerContainer.AddChild(s.buildSearchEmptyState())
		} else {
			innerContainer.AddChild(s.buildFilteredEmptyState())
		}
		s.setupNavigation(1) // Toolbar only
	} else {
		// Toolbar (row 0 - doesn't stretch)
		innerContainer.AddChild(s.buildToolbar())

		// Game list or grid (row 1 - stretches to fill)
		if s.config.Library.ViewMode == "list" {
			innerContainer.AddChild(s.buildListView())
			gridColumns = 1
		} else {
			gridColumns = s.buildIconView(innerContainer)
		}
		s.setupNavigation(gridColumns)
	}

	rootContainer.AddChild(innerContainer)
	return rootContainer
}

// buildEmptyState creates the empty library display
func (s *LibraryScreen) buildEmptyState() *widget.Container {
	button := widgets.TextButton("Open Settings", style.ButtonPaddingMedium, func(args *widget.ButtonClickedEventArgs) {
		s.callback.SwitchToSettings()
	})
	return widgets.EmptyState("No games in library", "Add a ROM folder in Settings", button)
}

// buildFilteredEmptyState creates the display when filters hide all games
func (s *LibraryScreen) buildFilteredEmptyState() *widget.Container {
	return widgets.EmptyState("No favorites yet", "Turn off the favorites filter to see all games", nil)
}

// buildSearchEmptyState creates the display when search returns no results
func (s *LibraryScreen) buildSearchEmptyState() *widget.Container {
	return widgets.EmptyState("No matches found", "Try a different search term or press ESC to clear", nil)
}

// SetSearchText sets the search filter text and resets scroll position
func (s *LibraryScreen) SetSearchText(text string) {
	s.searchText = text
	// Reset scroll positions when search changes
	s.iconScrollTop = 0
	s.listScrollTop = 0
}

// buildToolbar creates the library toolbar
func (s *LibraryScreen) buildToolbar() *widget.Container {
	// Use GridLayout with 3 columns: left (view toggles), center (sort/favorites), right (settings)
	toolbar := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			widget.GridLayoutOpts.Stretch([]bool{false, true, false}, nil),
			widget.GridLayoutOpts.Spacing(style.SmallSpacing, 0),
		)),
	)

	// LEFT SECTION: View mode toggles
	leftSection := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
	)

	iconViewBtn := widgets.ToggleButton("Icon", s.config.Library.ViewMode == "icon", func(args *widget.ButtonClickedEventArgs) {
		s.config.Library.ViewMode = "icon"
		storage.SaveConfig(s.config)
		s.SetPendingFocus("toolbar-icon")
		s.callback.RequestRebuild()
	})
	s.RegisterFocusButton("toolbar-icon", iconViewBtn)
	leftSection.AddChild(iconViewBtn)

	listViewBtn := widgets.ToggleButton("List", s.config.Library.ViewMode == "list", func(args *widget.ButtonClickedEventArgs) {
		s.config.Library.ViewMode = "list"
		storage.SaveConfig(s.config)
		s.SetPendingFocus("toolbar-list")
		s.callback.RequestRebuild()
	})
	s.RegisterFocusButton("toolbar-list", listViewBtn)
	leftSection.AddChild(listViewBtn)

	toolbar.AddChild(leftSection)

	// CENTER SECTION: Sort and Favorites
	centerSection := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)

	centerContent := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionCenter,
				VerticalPosition:   widget.AnchorLayoutPositionCenter,
			}),
		),
	)

	// Sort label (vertically centered via RowLayout position)
	sortLabel := widget.NewText(
		widget.TextOpts.Text("Sort:", style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
		),
	)
	centerContent.AddChild(sortLabel)

	// Sort button
	sortOptions := []string{"Title", "Last Played", "Play Time"}
	sortValues := []string{"title", "lastPlayed", "playTime"}

	currentSortIdx := 0
	for i, v := range sortValues {
		if v == s.config.Library.SortBy {
			currentSortIdx = i
			break
		}
	}

	sortButton := widget.NewButton(
		widget.ButtonOpts.Image(style.ButtonImage()),
		widget.ButtonOpts.Text(sortOptions[currentSortIdx], style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			currentSortIdx = (currentSortIdx + 1) % len(sortOptions)
			s.config.Library.SortBy = sortValues[currentSortIdx]
			storage.SaveConfig(s.config)
			s.SetPendingFocus("toolbar-sort")
			s.callback.RequestRebuild()
		}),
	)
	s.RegisterFocusButton("toolbar-sort", sortButton)
	centerContent.AddChild(sortButton)

	// Favorites button
	favText := "Favorites"
	if s.config.Library.FavoritesFilter {
		favText = "[*] Favorites"
	}
	favButton := widgets.ToggleButton(favText, s.config.Library.FavoritesFilter, func(args *widget.ButtonClickedEventArgs) {
		s.config.Library.FavoritesFilter = !s.config.Library.FavoritesFilter
		storage.SaveConfig(s.config)
		s.SetPendingFocus("toolbar-favorites")
		s.callback.RequestRebuild()
	})
	s.RegisterFocusButton("toolbar-favorites", favButton)
	centerContent.AddChild(favButton)

	centerSection.AddChild(centerContent)
	toolbar.AddChild(centerSection)

	// RIGHT SECTION: Settings button
	rightSection := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)

	settingsButton := widget.NewButton(
		widget.ButtonOpts.Image(style.ButtonImage()),
		widget.ButtonOpts.Text("Settings", style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			s.SetPendingFocus("toolbar-settings")
			s.callback.SwitchToSettings()
		}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionEnd,
			}),
		),
	)
	s.RegisterFocusButton("toolbar-settings", settingsButton)
	rightSection.AddChild(settingsButton)

	toolbar.AddChild(rightSection)

	return toolbar
}

// buildListView creates the list view of games using custom ScrollContainer for scroll control
func (s *LibraryScreen) buildListView() widget.PreferredSizeLocateableWidget {
	selectedIndex := -1

	// Compute responsive column widths based on available window width
	windowWidth := s.callback.GetWindowWidth()
	if windowWidth < 400 {
		windowWidth = style.IconDefaultWindowWidth
	}

	// Available width for the list content (subtract screen padding, scrollbar, and spacing)
	availableWidth := windowWidth - style.DefaultPadding*2 - style.ScrollbarWidth - style.TinySpacing

	// Grid overhead: 5 column gaps + left/right padding inside the grid
	gridOverhead := 5*style.SmallSpacing + 2*style.SmallSpacing

	// Preferred fixed column widths (from scaled constants)
	prefFav := style.ListColFavorite
	prefGenre := style.ListColGenre
	prefRegion := style.ListColRegion
	prefPlayTime := style.ListColPlayTime
	prefLastPlayed := style.ListColLastPlayed
	totalFixed := prefFav + prefGenre + prefRegion + prefPlayTime + prefLastPlayed

	// Minimum title width to keep usable
	minTitleWidth := style.ListMinTitleWidth

	// Compute minimum widths from header text measurement + padding
	minGenre := int(style.MeasureWidth("Genre")) + style.SmallSpacing
	minRegion := int(style.MeasureWidth("Region")) + style.SmallSpacing
	minPlayTime := int(style.MeasureWidth("Play Time")) + style.SmallSpacing
	minLastPlayed := int(style.MeasureWidth("Last Played")) + style.SmallSpacing
	minFav := prefFav // Favorite column has no text header, keep as-is

	// Compute actual column widths, shrinking if needed
	favW := prefFav
	genreW := prefGenre
	regionW := prefRegion
	playTimeW := prefPlayTime
	lastPlayedW := prefLastPlayed

	maxFixed := availableWidth - gridOverhead - minTitleWidth
	if totalFixed > maxFixed && maxFixed > 0 {
		// First try: use text-measured minimums directly
		totalMin := minFav + minGenre + minRegion + minPlayTime + minLastPlayed
		if totalMin <= maxFixed {
			// Distribute remaining space proportionally above minimums
			extra := maxFixed - totalMin
			prefExtra := totalFixed - totalMin
			if prefExtra > 0 {
				genreW = minGenre + (prefGenre-minGenre)*extra/prefExtra
				regionW = minRegion + (prefRegion-minRegion)*extra/prefExtra
				playTimeW = minPlayTime + (prefPlayTime-minPlayTime)*extra/prefExtra
				lastPlayedW = minLastPlayed + (prefLastPlayed-minLastPlayed)*extra/prefExtra
				favW = minFav
			} else {
				genreW = minGenre
				regionW = minRegion
				playTimeW = minPlayTime
				lastPlayedW = minLastPlayed
				favW = minFav
			}
		} else {
			// Extremely tight: use minimums (title gets minTitleWidth)
			favW = minFav
			genreW = minGenre
			regionW = minRegion
			playTimeW = minPlayTime
			lastPlayedW = minLastPlayed
		}
	}

	// Compute actual title width for truncation
	actualFixed := favW + genreW + regionW + playTimeW + lastPlayedW
	titleWidth := availableWidth - gridOverhead - actualFixed
	if titleWidth < minTitleWidth {
		titleWidth = minTitleWidth
	}

	fontFace := *style.FontFace()

	// Build header row
	header := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(6),
			widget.GridLayoutOpts.Stretch([]bool{false, true, false, false, false, false}, nil),
			widget.GridLayoutOpts.Spacing(style.SmallSpacing, 0),
			widget.GridLayoutOpts.Padding(&widget.Insets{Left: style.SmallSpacing, Right: style.SmallSpacing}),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, style.ListHeaderHeight),
		),
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Surface)),
	)
	header.AddChild(widgets.TableHeaderCell("", favW, style.ListHeaderHeight))
	header.AddChild(widgets.TableHeaderCell("Title", 0, style.ListHeaderHeight))
	header.AddChild(widgets.TableHeaderCell("Genre", genreW, style.ListHeaderHeight))
	header.AddChild(widgets.TableHeaderCell("Region", regionW, style.ListHeaderHeight))
	header.AddChild(widgets.TableHeaderCell("Play Time", playTimeW, style.ListHeaderHeight))
	header.AddChild(widgets.TableHeaderCell("Last Played", lastPlayedW, style.ListHeaderHeight))

	// Create vertical container for all game rows
	listContent := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(0),
		)),
	)

	// Add a row for each game
	for i, game := range s.games {
		idx := i
		g := game

		// Track selected index for scroll centering
		if g.CRC32 == s.listSelectedCRC {
			selectedIndex = idx
		}

		// Format cell values
		fav := ""
		if g.Favorite {
			fav = "*"
		}
		region := strings.ToUpper(g.Region)
		if region == "" {
			region = "-"
		}
		genre := g.Genre
		if genre == "" {
			genre = "-"
		}
		playTime := style.FormatPlayTime(g.PlayTimeSeconds)
		lastPlayed := style.FormatLastPlayed(g.LastPlayed)

		// Determine row background color for alternating rows
		rowIdleBg := widgets.AlternatingRowColor(idx)

		// Create row container with grid layout (transparent background - button handles colors)
		row := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewGridLayout(
				widget.GridLayoutOpts.Columns(6),
				widget.GridLayoutOpts.Stretch([]bool{false, true, false, false, false, false}, nil),
				widget.GridLayoutOpts.Spacing(style.SmallSpacing, 0),
				widget.GridLayoutOpts.Padding(&widget.Insets{Left: style.SmallSpacing, Right: style.SmallSpacing}),
			)),
			widget.ContainerOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, style.ListRowHeight),
			),
		)

		// Truncate cell content to fit computed column widths
		displayName, _ := style.TruncateToWidth(g.DisplayName, fontFace, float64(titleWidth))
		truncGenre, _ := style.TruncateToWidth(genre, fontFace, float64(genreW))
		truncRegion, _ := style.TruncateToWidth(region, fontFace, float64(regionW))
		truncPlayTime, _ := style.TruncateToWidth(playTime, fontFace, float64(playTimeW))
		truncLastPlayed, _ := style.TruncateToWidth(lastPlayed, fontFace, float64(lastPlayedW))

		// Add cells
		row.AddChild(widgets.TableCell(fav, favW, style.ListRowHeight, style.Accent))
		row.AddChild(widgets.TableCell(displayName, 0, style.ListRowHeight, style.Text))
		row.AddChild(widgets.TableCell(truncGenre, genreW, style.ListRowHeight, style.TextSecondary))
		row.AddChild(widgets.TableCell(truncRegion, regionW, style.ListRowHeight, style.TextSecondary))
		row.AddChild(widgets.TableCell(truncPlayTime, playTimeW, style.ListRowHeight, style.TextSecondary))
		row.AddChild(widgets.TableCell(truncLastPlayed, lastPlayedW, style.ListRowHeight, style.TextSecondary))

		// Create button with alternating row color as idle, focus/hover colors for interaction
		gameCRC := g.CRC32 // Capture for closure
		rowButton := widget.NewButton(
			widget.ButtonOpts.Image(&widget.ButtonImage{
				Idle:    image.NewNineSliceColor(rowIdleBg),
				Hover:   image.NewNineSliceColor(style.PrimaryHover),
				Pressed: image.NewNineSliceColor(style.Primary),
			}),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{
					Stretch: true,
				}),
				widget.WidgetOpts.MinSize(0, style.ListRowHeight),
			),
			widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
				if s.listScrollContainer != nil {
					s.listScrollTop = s.listScrollContainer.ScrollTop
				}
				s.listSelectedCRC = gameCRC
				s.SetPendingFocus("game-" + gameCRC)
				s.callback.SwitchToDetail(gameCRC)
			}),
		)

		// Store button reference for focus restoration
		s.RegisterFocusButton("game-"+gameCRC, rowButton)

		// Stack: button at bottom (shows background), row content on top (transparent)
		rowWrapper := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewStackedLayout()),
			widget.ContainerOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{
					Stretch: true,
				}),
				widget.WidgetOpts.MinSize(0, style.ListRowHeight),
			),
		)
		rowWrapper.AddChild(rowButton)
		rowWrapper.AddChild(row)

		listContent.AddChild(rowWrapper)
	}

	// Create scrollable container (we use custom layout for header alignment, so ignore wrapper)
	scrollContainer, _, scrollRow := widgets.ScrollableContainer(widgets.ScrollableOpts{
		Content: listContent,
		BgColor: style.Background,
		Spacing: style.TinySpacing,
	})

	// Store reference for scroll preservation
	s.listScrollContainer = scrollContainer

	// Scroll so the selected row is visible (centered); otherwise restore the
	// preserved scroll position. Keeping the selection visible matters when a
	// sort change (e.g. Last Played) moves the selected game.
	if selectedIndex >= 0 && len(s.games) > 0 {
		scrollContainer.SetScrollTop(centerScrollFraction(selectedIndex*style.ListRowHeight, style.ListRowHeight,
			len(s.games)*style.ListRowHeight, style.EstimatedViewportHeight))
	} else if s.listScrollTop > 0 {
		scrollContainer.SetScrollTop(s.listScrollTop)
	}

	// Header row with spacer for slider alignment
	headerRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Spacing(style.TinySpacing, 0),
			widget.GridLayoutOpts.Stretch([]bool{true, false}, nil),
		)),
	)
	headerRow.AddChild(header)
	// Empty spacer matching slider width
	headerSpacer := widget.NewContainer(
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(style.ScrollbarWidth, 0),
		),
	)
	headerRow.AddChild(headerSpacer)

	// Main container: header row + scroll area
	mainContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true}),
			widget.GridLayoutOpts.Spacing(0, style.TinySpacing),
		)),
	)
	mainContainer.AddChild(headerRow)
	mainContainer.AddChild(scrollRow)

	return mainContainer
}

// buildIconView creates the icon/grid view of games with artwork
// Returns the number of columns for navigation setup
func (s *LibraryScreen) buildIconView(container *widget.Container) int {
	s.artRefs = make(map[string]*artRef)

	// Calculate responsive grid dimensions
	windowWidth := s.callback.GetWindowWidth()
	if windowWidth < 400 {
		windowWidth = style.IconDefaultWindowWidth
	}

	// Available width for cards (subtract padding and scrollbar)
	availableWidth := windowWidth - (style.DefaultPadding * 2) - style.ScrollbarWidth

	// Calculate number of columns that fit with minimum card width
	// Formula: columns = floor((availableWidth + spacing) / (minCardWidth + spacing))
	columns := (availableWidth + style.SmallSpacing) / (style.IconMinCardWidth + style.SmallSpacing)
	if columns < 2 {
		columns = 2
	}

	// Calculate exact card width to fill the available space
	// Formula: cardWidth = (availableWidth - (columns - 1) * spacing) / columns
	cardWidth := (availableWidth - (columns-1)*style.SmallSpacing) / columns

	// Card height maintains ~4:3 aspect ratio for artwork + text
	artHeight := cardWidth * 4 / 3
	cardHeight := artHeight + style.IconCardTextHeight

	// Start async artwork loading, prioritizing the artwork that is visible
	// on return so a scrolled-into-view list does not wait on off-screen
	// covers loading first.
	gameCRCs := orderCRCsVisibleFirst(s.games, columns, s.visibleAnchorIndex())
	s.artLoader.Start(gameCRCs, cardWidth, artHeight)

	// Create stretch array - all columns stretch equally to fill width
	columnStretches := make([]bool, columns)
	for i := range columnStretches {
		columnStretches[i] = true
	}

	// Grid container for the cards - columns stretch to fill available width
	gridContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(columns),
			widget.GridLayoutOpts.Spacing(style.SmallSpacing, style.SmallSpacing),
			widget.GridLayoutOpts.Stretch(columnStretches, nil),
		)),
	)

	// Add game cards with calculated dimensions
	for _, game := range s.games {
		card := s.buildGameCardSized(game, cardWidth, cardHeight, artHeight)
		gridContainer.AddChild(card)
	}

	// Create scrollable container
	scrollContainer, _, wrapper := widgets.ScrollableContainer(widgets.ScrollableOpts{
		Content: gridContainer,
		BgColor: style.Background,
		Spacing: 4,
	})

	// Store reference for scroll preservation
	s.iconScrollContainer = scrollContainer

	// Scroll so the selected card's row is the top visible row; otherwise
	// restore the preserved scroll position. Keeping the selection visible
	// matters when a sort change (e.g. Last Played) moves the selected game.
	// The exact top-alignment needs the real layout rects, which are not known
	// until after the first render, so set an estimate now and refine it next
	// frame in ApplyPendingIconScroll.
	selectedIndex := -1
	if s.iconSelectedCRC != "" {
		for i, g := range s.games {
			if g.CRC32 == s.iconSelectedCRC {
				selectedIndex = i
				break
			}
		}
	}
	if selectedIndex >= 0 {
		rowPitch := cardHeight + style.SmallSpacing
		totalRows := (len(s.games) + columns - 1) / columns
		scrollContainer.SetScrollTop(topAlignScrollFraction((selectedIndex/columns)*rowPitch,
			totalRows*rowPitch, style.EstimatedViewportHeight))
		s.pendingIconScroll = true
	} else if s.iconScrollTop > 0 {
		scrollContainer.SetScrollTop(s.iconScrollTop)
		s.pendingIconScroll = false
		s.iconSelectedCRC = ""
	} else {
		s.pendingIconScroll = false
		s.iconSelectedCRC = ""
	}

	container.AddChild(wrapper)
	return columns
}

// ApplyPendingIconScroll finishes top-aligning the selected icon card using the
// real scroll-container rects. buildIconView can only estimate the position
// because widget layout is unknown until the first render after a rebuild, so
// this runs each frame and applies the exact alignment once the rects are
// available, then clears the request.
func (s *LibraryScreen) ApplyPendingIconScroll() {
	if !s.pendingIconScroll {
		return
	}
	if s.config.Library.ViewMode != "icon" || s.iconScrollContainer == nil {
		s.pendingIconScroll = false
		s.iconSelectedCRC = ""
		return
	}

	btn := s.focusButtons["game-"+s.iconSelectedCRC]
	if btn == nil {
		s.pendingIconScroll = false
		s.iconSelectedCRC = ""
		return
	}

	// Wait until the layout rects are known (the frame after a rebuild).
	if s.iconScrollContainer.ContentRect().Dy() <= 0 || s.iconScrollContainer.ViewRect().Dy() <= 0 {
		return
	}

	s.pendingIconScroll = false
	s.iconSelectedCRC = ""
	s.iconScrollContainer.ScrollRectToTop(btn.GetWidget().Rect)
}

// centerScrollFraction returns the scroll fraction (0..1) that centers an item
// of height itemHeight whose top edge is at itemY within content of height
// totalHeight, for the given viewport height. Returns 0 when content is empty.
func centerScrollFraction(itemY, itemHeight, totalHeight, viewportHeight int) float64 {
	if totalHeight <= 0 {
		return 0
	}
	target := itemY - viewportHeight/2 + itemHeight/2
	if target < 0 {
		target = 0
	}
	if totalHeight > viewportHeight && target > totalHeight-viewportHeight {
		target = totalHeight - viewportHeight
	}
	f := float64(target) / float64(totalHeight)
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	return f
}

// topAlignScrollFraction returns the scroll fraction (0..1) that places the top
// of an item at itemY at the top of the viewport, within content of height
// totalHeight. ebitenui offsets content by (totalHeight-viewportHeight)*fraction,
// so the divisor is the scroll range, not the full content height. Returns 0
// when the content fits.
func topAlignScrollFraction(itemY, totalHeight, viewportHeight int) float64 {
	scrollRange := totalHeight - viewportHeight
	if scrollRange <= 0 {
		return 0
	}
	f := float64(itemY) / float64(scrollRange)
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	return f
}

// visibleAnchorIndex returns the index in s.games whose artwork should load
// first. It prefers the selected game (the icon view scrolls to keep that card
// visible), falls back to the restored scroll position, and otherwise anchors
// at the top of the list.
func (s *LibraryScreen) visibleAnchorIndex() int {
	if s.iconSelectedCRC != "" {
		for i, g := range s.games {
			if g.CRC32 == s.iconSelectedCRC {
				return i
			}
		}
	}
	if s.iconScrollTop > 0 && len(s.games) > 0 {
		idx := int(s.iconScrollTop * float64(len(s.games)))
		if idx >= len(s.games) {
			idx = len(s.games) - 1
		}
		return idx
	}
	return 0
}

// orderCRCsVisibleFirst returns the game CRCs ordered so the row containing
// anchorIdx loads first, then expands outward one row at a time alternating
// below and above (anchor row, +1, -1, +2, -2, ...). This prioritizes the
// on-screen cards when returning to a scrolled position without assuming the
// user will next scroll up or down. Every game is emitted exactly once.
func orderCRCsVisibleFirst(games []*storage.GameEntry, columns, anchorIdx int) []string {
	n := len(games)
	ordered := make([]string, 0, n)
	if n == 0 {
		return ordered
	}
	if columns < 1 {
		columns = 1
	}
	if anchorIdx < 0 || anchorIdx >= n {
		anchorIdx = 0
	}

	totalRows := (n + columns - 1) / columns
	anchorRow := anchorIdx / columns

	emitRow := func(row int) {
		if row < 0 || row >= totalRows {
			return
		}
		start := row * columns
		end := start + columns
		if end > n {
			end = n
		}
		for i := start; i < end; i++ {
			ordered = append(ordered, games[i].CRC32)
		}
	}

	emitRow(anchorRow)
	for d := 1; d < totalRows; d++ {
		emitRow(anchorRow + d)
		emitRow(anchorRow - d)
	}
	return ordered
}

// buildGameCardSized creates a game card with specific dimensions
func (s *LibraryScreen) buildGameCardSized(game *storage.GameEntry, cardWidth, cardHeight, artHeight int) *widget.Container {
	// Load artwork for the card. pending is true when the loading placeholder
	// was returned (artwork not yet processed).
	art, pending := s.loadGameArtwork(game.CRC32)

	// Create mutable ref so closures and UpdateArtwork can swap the image
	ref := &artRef{image: art}

	// Inner card content
	cardContent := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.Px(2)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(cardWidth, cardHeight),
		),
	)

	// Game title (truncated based on card pixel width)
	displayName, _ := style.TruncateToWidth(game.DisplayName, *style.FontFace(), float64(cardWidth-4))
	titleLabel := widget.NewText(
		widget.TextOpts.Text(displayName, style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionStart),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Stretch: true,
			}),
		),
	)

	// Artwork graphic (renders on top of button, scaled/dimmed for zoom effect)
	artGraphic := widgets.NewIconGraphic(ref.image, style.IconUnfocusedScale, float32(style.IconUnfocusedDim))
	ref.graphic = artGraphic

	// Artwork button (handles bg colors, click, focus - no graphic image)
	gameCRC := game.CRC32 // Capture for closure
	var artButton *widget.Button
	artButton = widget.NewButton(
		widget.ButtonOpts.Image(&widget.ButtonImage{
			Idle:    image.NewNineSliceColor(style.Surface),
			Hover:   image.NewNineSliceColor(style.PrimaryHover),
			Pressed: image.NewNineSliceColor(style.Primary),
		}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(cardWidth, artHeight),
			widget.WidgetOpts.CursorEnterHandler(func(args *widget.WidgetCursorEnterEventArgs) {
				ref.hovered = true
				titleLabel.SetColor(style.Accent)
				artGraphic.Focused = true
			}),
			widget.WidgetOpts.CursorExitHandler(func(args *widget.WidgetCursorExitEventArgs) {
				ref.hovered = false
				if !artButton.IsFocused() {
					titleLabel.SetColor(style.Text)
					artGraphic.Focused = false
				}
			}),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			// Save scroll position and selected game before navigating
			s.iconSelectedCRC = gameCRC
			s.SetPendingFocus("game-" + gameCRC)
			if s.iconScrollContainer != nil {
				s.iconScrollTop = s.iconScrollContainer.ScrollTop
			}
			s.callback.SwitchToDetail(gameCRC)
		}),
	)
	ref.button = artButton

	// Update title color and artwork on keyboard/gamepad focus changes
	artButton.GetWidget().FocusEvent.AddHandler(func(args interface{}) {
		if a, ok := args.(*widget.WidgetFocusEventArgs); ok {
			if a.Focused {
				titleLabel.SetColor(style.Accent)
				artGraphic.Focused = true
			} else {
				titleLabel.SetColor(style.Text)
				artGraphic.Focused = false
			}
		}
	})

	// Track cards that were built with the loading placeholder so
	// UpdateArtwork can swap in the real image once the background
	// goroutine finishes. The pending flag comes from the same cache
	// lookup that chose the image, avoiding a race where the goroutine
	// caches a result between loadGameArtwork and this point.
	if pending {
		s.artRefs[gameCRC] = ref
	}

	// Store button reference for focus restoration
	s.RegisterFocusButton("game-"+gameCRC, artButton)

	// Stack: button at bottom (background), graphic on top (artwork)
	artStack := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewStackedLayout()),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Stretch: true,
			}),
			widget.WidgetOpts.MinSize(cardWidth, artHeight),
		),
	)
	artStack.AddChild(artButton)
	artStack.AddChild(artGraphic)

	cardContent.AddChild(artStack)
	cardContent.AddChild(titleLabel)

	// Wrapper with AnchorLayout to center the card content in the grid cell
	card := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	card.AddChild(cardContent)

	return card
}

// loadGameArtwork returns the cached artwork texture for a game card.
// pending is true when the loading placeholder was returned because the
// background goroutine has not yet processed this CRC. If processing
// determined no artwork exists, the missing-art image is returned (stored
// under the CRC by loadOne).
func (s *LibraryScreen) loadGameArtwork(gameCRC string) (art *ebiten.Image, pending bool) {
	// Non-nil means processed: real artwork or missing-art image
	if cached := s.artLoader.Get(gameCRC); cached != nil {
		return cached.image, false
	}

	// Not yet processed - show loading image
	loading := s.artLoader.Get("loading")
	return loading.image, true
}

// SaveScrollPosition saves the current scroll position before a rebuild
// This should be called before rebuildCurrentScreen
func (s *LibraryScreen) SaveScrollPosition() {
	if s.config.Library.ViewMode == "icon" {
		if s.iconScrollContainer != nil {
			s.iconScrollTop = s.iconScrollContainer.ScrollTop
		}
	} else {
		if s.listScrollContainer != nil {
			s.listScrollTop = s.listScrollContainer.ScrollTop
		}
	}
}

// OnEnter is called when entering the library screen
func (s *LibraryScreen) OnEnter() {
	s.games = s.library.GetGamesSortedFiltered(s.config.Library.SortBy, s.config.Library.FavoritesFilter, s.searchText)
	s.SetDefaultFocus("toolbar-icon") // Only sets if no pending focus (preserves game selection when returning)
}

// isGameButton returns true if the button is a game button (not a toolbar button)
func (s *LibraryScreen) isGameButton(btn *widget.Button) bool {
	// Game buttons have keys starting with "game-"
	for key, b := range s.focusButtons {
		if b == btn && len(key) > 5 && key[:5] == "game-" {
			return true
		}
	}
	return false
}

// setupNavigation registers navigation zones and transitions
func (s *LibraryScreen) setupNavigation(gridColumns int) {
	// Toolbar zone (horizontal)
	toolbarKeys := []string{
		"toolbar-icon",
		"toolbar-list",
		"toolbar-sort",
		"toolbar-favorites",
		"toolbar-settings",
	}
	s.RegisterNavZone("toolbar", types.NavZoneHorizontal, toolbarKeys, 0)

	// Content zone (grid or list)
	if len(s.games) > 0 {
		gameKeys := make([]string, len(s.games))
		for i, game := range s.games {
			gameKeys[i] = "game-" + game.CRC32
		}

		zoneType := types.NavZoneGrid
		if s.config.Library.ViewMode == "list" {
			zoneType = types.NavZoneVertical
			gridColumns = 1
		}

		s.RegisterNavZone("content", zoneType, gameKeys, gridColumns)

		// Set up transitions. Up from the toolbar wraps to the bottom of the
		// games; down enters them from the top.
		s.SetNavTransition("toolbar", types.DirDown, "content", types.NavIndexPreserve)
		s.SetNavTransition("toolbar", types.DirUp, "content", types.NavIndexPreserve)
		s.SetNavTransition("content", types.DirUp, "toolbar", types.NavIndexPreserve)
	}
}

// EnsureFocusedVisible scrolls the view to ensure the focused widget is visible
// This is called after gamepad navigation changes focus
func (s *LibraryScreen) EnsureFocusedVisible(focused widget.Focuser) {
	if focused == nil {
		return
	}

	// Check if this is a game button (not toolbar)
	// Only game buttons should trigger scrolling
	btn, ok := focused.(*widget.Button)
	if !ok || !s.isGameButton(btn) {
		return
	}

	// Get the appropriate scroll container based on view mode
	var scrollContainer *widgets.ScrollView
	if s.config.Library.ViewMode == "icon" {
		scrollContainer = s.iconScrollContainer
	} else {
		scrollContainer = s.listScrollContainer
	}

	if scrollContainer == nil {
		return
	}

	focusWidget := focused.GetWidget()
	if focusWidget == nil {
		return
	}
	scrollContainer.ScrollRectIntoView(focusWidget.Rect)
}
