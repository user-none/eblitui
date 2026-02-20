//go:build !libretro

package screens

import (
	"bytes"
	goimage "image"
	"os"
	"strings"

	_ "image/png"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
	"github.com/user-none/eblitui/standalone/types"
)

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

	// Widget references for scroll preservation (dual view mode)
	iconScrollContainer *widget.ScrollContainer
	iconVSlider         *widget.Slider
	listScrollContainer *widget.ScrollContainer
	listVSlider         *widget.Slider

	// Artwork cache: key = "crc32", value = scaled ebiten.Image
	artworkCache      map[string]*ebiten.Image
	cachedWindowWidth int // Track window width to detect resize

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
		artworkCache:  make(map[string]*ebiten.Image),
	}
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

// ClearArtworkCache clears the cached artwork images.
// Should be called after library scan or when library locations change.
func (s *LibraryScreen) ClearArtworkCache() {
	for _, img := range s.artworkCache {
		if img != nil {
			img.Deallocate()
		}
	}
	s.artworkCache = make(map[string]*ebiten.Image)
	s.cachedWindowWidth = 0
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
	rootContainer := style.ScreenContainer()

	// Track grid columns for navigation
	gridColumns := 1

	if totalGames == 0 {
		// Library is truly empty - single row that stretches to fill
		innerContainer := style.ScreenContentContainer([]bool{true})
		innerContainer.AddChild(s.buildEmptyState())
		rootContainer.AddChild(innerContainer)
		return rootContainer
	}

	innerContainer := style.ScreenContentContainer([]bool{false, true}) // toolbar=fixed, content=stretch

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
	button := style.TextButton("Open Settings", style.ButtonPaddingMedium, func(args *widget.ButtonClickedEventArgs) {
		s.callback.SwitchToSettings()
	})
	return style.EmptyState("No games in library", "Add a ROM folder in Settings", button)
}

// buildFilteredEmptyState creates the display when filters hide all games
func (s *LibraryScreen) buildFilteredEmptyState() *widget.Container {
	return style.EmptyState("No favorites yet", "Turn off the favorites filter to see all games", nil)
}

// buildSearchEmptyState creates the display when search returns no results
func (s *LibraryScreen) buildSearchEmptyState() *widget.Container {
	return style.EmptyState("No matches found", "Try a different search term or press ESC to clear", nil)
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

	iconViewBtn := style.ToggleButton("Icon", s.config.Library.ViewMode == "icon", func(args *widget.ButtonClickedEventArgs) {
		s.config.Library.ViewMode = "icon"
		storage.SaveConfig(s.config)
		s.SetPendingFocus("toolbar-icon")
		s.callback.RequestRebuild()
	})
	s.RegisterFocusButton("toolbar-icon", iconViewBtn)
	leftSection.AddChild(iconViewBtn)

	listViewBtn := style.ToggleButton("List", s.config.Library.ViewMode == "list", func(args *widget.ButtonClickedEventArgs) {
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
	favButton := style.ToggleButton(favText, s.config.Library.FavoritesFilter, func(args *widget.ButtonClickedEventArgs) {
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
	header.AddChild(style.TableHeaderCell("", favW, style.ListHeaderHeight))
	header.AddChild(style.TableHeaderCell("Title", 0, style.ListHeaderHeight))
	header.AddChild(style.TableHeaderCell("Genre", genreW, style.ListHeaderHeight))
	header.AddChild(style.TableHeaderCell("Region", regionW, style.ListHeaderHeight))
	header.AddChild(style.TableHeaderCell("Play Time", playTimeW, style.ListHeaderHeight))
	header.AddChild(style.TableHeaderCell("Last Played", lastPlayedW, style.ListHeaderHeight))

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
		rowIdleBg := style.AlternatingRowColor(idx)

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
		row.AddChild(style.TableCell(fav, favW, style.ListRowHeight, style.Accent))
		row.AddChild(style.TableCell(displayName, 0, style.ListRowHeight, style.Text))
		row.AddChild(style.TableCell(truncGenre, genreW, style.ListRowHeight, style.TextSecondary))
		row.AddChild(style.TableCell(truncRegion, regionW, style.ListRowHeight, style.TextSecondary))
		row.AddChild(style.TableCell(truncPlayTime, playTimeW, style.ListRowHeight, style.TextSecondary))
		row.AddChild(style.TableCell(truncLastPlayed, lastPlayedW, style.ListRowHeight, style.TextSecondary))

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
	scrollContainer, vSlider, scrollRow := style.ScrollableContainer(style.ScrollableOpts{
		Content: listContent,
		BgColor: style.Background,
		Spacing: style.TinySpacing,
	})

	// Store references for scroll preservation
	s.listScrollContainer = scrollContainer
	s.listVSlider = vSlider

	// Restore or calculate scroll position
	if s.listScrollTop > 0 {
		scrollContainer.ScrollTop = s.listScrollTop
		vSlider.Current = int(s.listScrollTop * 1000)
	} else if selectedIndex >= 0 && len(s.games) > 0 {
		totalHeight := len(s.games) * style.ListRowHeight
		selectedY := selectedIndex * style.ListRowHeight
		viewportHeight := style.EstimatedViewportHeight
		targetScrollY := selectedY - (viewportHeight / 2) + (style.ListRowHeight / 2)
		if targetScrollY < 0 {
			targetScrollY = 0
		}
		if totalHeight > viewportHeight && targetScrollY > totalHeight-viewportHeight {
			targetScrollY = totalHeight - viewportHeight
		}
		if totalHeight > 0 {
			scrollTop := float64(targetScrollY) / float64(totalHeight)
			if scrollTop > 1 {
				scrollTop = 1
			}
			if scrollTop < 0 {
				scrollTop = 0
			}
			scrollContainer.ScrollTop = scrollTop
			vSlider.Current = int(scrollTop * 1000)
		}
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
	// Calculate responsive grid dimensions
	windowWidth := s.callback.GetWindowWidth()
	if windowWidth < 400 {
		windowWidth = style.IconDefaultWindowWidth
	}

	// Clear cache if window width changed (artwork needs re-scaling)
	if s.cachedWindowWidth != 0 && s.cachedWindowWidth != windowWidth {
		s.ClearArtworkCache()
	}
	s.cachedWindowWidth = windowWidth

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
	scrollContainer, vSlider, wrapper := style.ScrollableContainer(style.ScrollableOpts{
		Content: gridContainer,
		BgColor: style.Background,
		Spacing: 4,
	})

	// Store references for scroll preservation
	s.iconScrollContainer = scrollContainer
	s.iconVSlider = vSlider

	// Restore icon view scroll position if we have one
	if s.iconScrollTop > 0 {
		scrollContainer.ScrollTop = s.iconScrollTop
		vSlider.Current = int(s.iconScrollTop * 1000)
	}

	container.AddChild(wrapper)
	return columns
}

// buildGameCardSized creates a game card with specific dimensions
func (s *LibraryScreen) buildGameCardSized(game *storage.GameEntry, cardWidth, cardHeight, artHeight int) *widget.Container {
	// Load artwork scaled to fit
	artwork := s.loadGameArtworkSized(game.CRC32, cardWidth, artHeight)

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

	// Artwork button (clickable)
	gameCRC := game.CRC32 // Capture for closure
	artButton := widget.NewButton(
		widget.ButtonOpts.Image(&widget.ButtonImage{
			Idle:    image.NewNineSliceColor(style.Surface),
			Hover:   image.NewNineSliceColor(style.PrimaryHover),
			Pressed: image.NewNineSliceColor(style.Primary),
		}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(cardWidth, artHeight),
		),
		widget.ButtonOpts.Graphic(&widget.GraphicImage{Idle: artwork}),
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

	// Store button reference for focus restoration
	s.RegisterFocusButton("game-"+gameCRC, artButton)

	cardContent.AddChild(artButton)

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
	cardContent.AddChild(titleLabel)

	// Wrapper with AnchorLayout to center the card content in the grid cell
	card := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	card.AddChild(cardContent)

	return card
}

// loadGameArtworkSized loads artwork scaled to specific dimensions
func (s *LibraryScreen) loadGameArtworkSized(gameCRC string, maxWidth, maxHeight int) *ebiten.Image {
	// Check cache first
	if cached, ok := s.artworkCache[gameCRC]; ok {
		return cached
	}

	artPath, err := storage.GetGameArtworkPath(gameCRC)
	if err != nil {
		return s.getPlaceholderImageSized(maxWidth, maxHeight)
	}

	data, err := os.ReadFile(artPath)
	if err != nil {
		return s.getPlaceholderImageSized(maxWidth, maxHeight)
	}

	img, _, err := goimage.Decode(bytes.NewReader(data))
	if err != nil {
		return s.getPlaceholderImageSized(maxWidth, maxHeight)
	}

	scaled := style.ScaleImage(img, maxWidth, maxHeight)
	s.artworkCache[gameCRC] = scaled
	return scaled
}

// getPlaceholderImageSized returns the placeholder image scaled to the specified size
func (s *LibraryScreen) getPlaceholderImageSized(width, height int) *ebiten.Image {
	const placeholderKey = "placeholder"
	if cached, ok := s.artworkCache[placeholderKey]; ok {
		return cached
	}

	data := s.callback.GetPlaceholderImageData()
	if data == nil {
		// Fallback to solid color if no placeholder data
		img := ebiten.NewImage(width, height)
		img.Fill(style.Surface)
		s.artworkCache[placeholderKey] = img
		return img
	}

	img, _, err := goimage.Decode(bytes.NewReader(data))
	if err != nil {
		// Fallback to solid color on decode error
		fallback := ebiten.NewImage(width, height)
		fallback.Fill(style.Surface)
		s.artworkCache[placeholderKey] = fallback
		return fallback
	}

	scaled := style.ScaleImage(img, width, height)
	s.artworkCache[placeholderKey] = scaled
	return scaled
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

		// Set up transitions
		s.SetNavTransition("toolbar", types.DirDown, "content", types.NavIndexPreserve)
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
	var scrollContainer *widget.ScrollContainer
	var vSlider *widget.Slider
	if s.config.Library.ViewMode == "icon" {
		scrollContainer = s.iconScrollContainer
		vSlider = s.iconVSlider
	} else {
		scrollContainer = s.listScrollContainer
		vSlider = s.listVSlider
	}

	if scrollContainer == nil {
		return
	}

	// Get the focused widget's rectangle
	focusWidget := focused.GetWidget()
	if focusWidget == nil {
		return
	}
	focusRect := focusWidget.Rect

	// Get the scroll container's view rect (visible area on screen)
	viewRect := scrollContainer.ViewRect()
	contentRect := scrollContainer.ContentRect()

	// If content fits in view, no scrolling needed
	if contentRect.Dy() <= viewRect.Dy() {
		return
	}

	// Current scroll offset in pixels
	maxScroll := contentRect.Dy() - viewRect.Dy()
	scrollOffset := int(scrollContainer.ScrollTop * float64(maxScroll))

	// Widget's position relative to view top
	widgetTopInView := focusRect.Min.Y - viewRect.Min.Y
	widgetBottomInView := focusRect.Max.Y - viewRect.Min.Y
	viewHeight := viewRect.Dy()

	// Check if widget top is above the visible area
	if widgetTopInView < 0 {
		// Scroll up: align widget top with view top
		newScrollOffset := scrollOffset + widgetTopInView
		if newScrollOffset < 0 {
			newScrollOffset = 0
		}
		scrollContainer.ScrollTop = float64(newScrollOffset) / float64(maxScroll)
		if vSlider != nil {
			vSlider.Current = int(scrollContainer.ScrollTop * 1000)
		}
	} else if widgetBottomInView > viewHeight {
		// Scroll down: align widget bottom with view bottom (minimal scroll)
		newScrollOffset := scrollOffset + (widgetBottomInView - viewHeight)
		if newScrollOffset > maxScroll {
			newScrollOffset = maxScroll
		}
		scrollContainer.ScrollTop = float64(newScrollOffset) / float64(maxScroll)
		if vSlider != nil {
			vSlider.Current = int(scrollContainer.ScrollTop * 1000)
		}
	}
}
