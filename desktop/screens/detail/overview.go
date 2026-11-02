// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package detail

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/achievements"
	"github.com/user-none/eblitui/desktop/metadata"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
	"github.com/user-none/eblitui/romloader"
	"github.com/user-none/go-rcheevos"
)

// discsZone is the navigation zone of the Overview disc buttons
const discsZone = "discs"

// OverviewSection is the detail Overview page: the game metadata, the
// disc selection for multi-disc games, and achievement progress
type OverviewSection struct {
	callback           types.ScreenCallback
	library            *storage.Library
	achievementManager *achievements.Manager
	defaultConsoleID   int
	contentWidth       func() int
	game               *storage.GameEntry
	files              storage.FileStatus // Which of the game's files are missing

	// Achievement loading state. loadGen counts SetGame calls; a lookup
	// started for an earlier game carries an older generation and its
	// result is dropped.
	achMu       sync.Mutex
	loadGen     int
	achLoading  bool
	achLoadErr  error
	achFound    bool
	achProgress *rcheevos.UserProgressEntry

	discKeys []string // Focus keys of the disc buttons
}

// progressRequest is what one achievement progress lookup needs, copied
// from the game when it starts so the lookup never reads the game while
// the UI thread may change it.
type progressRequest struct {
	gen       int    // loadGen when the lookup started
	consoleID uint32 // console used to hash the game
	discFile  string // selected disc to hash, for disc games; empty when missing
	romFile   string // ROM to hash, for cartridge games; empty when missing
}

// NewOverviewSection creates the Overview page. contentWidth returns the
// width of the page content area.
func NewOverviewSection(callback types.ScreenCallback, library *storage.Library, achievementManager *achievements.Manager, defaultConsoleID int, contentWidth func() int) *OverviewSection {
	return &OverviewSection{
		callback:           callback,
		library:            library,
		achievementManager: achievementManager,
		defaultConsoleID:   defaultConsoleID,
		contentWidth:       contentWidth,
	}
}

// SetGame sets the game to show, with which of its files are missing, and
// starts loading its achievement progress when logged in
func (o *OverviewSection) SetGame(game *storage.GameEntry, files storage.FileStatus) {
	o.game = game
	o.files = files

	// Reset achievement state and retire any lookup still running
	o.achMu.Lock()
	o.loadGen++
	req := progressRequest{gen: o.loadGen}
	o.achLoading = false
	o.achLoadErr = nil
	o.achFound = false
	o.achProgress = nil
	o.achMu.Unlock()

	if game == nil || o.achievementManager == nil || !o.achievementManager.IsLoggedIn() {
		return
	}

	// Copy what the lookup needs before handing it to the goroutine
	req.consoleID = uint32(o.defaultConsoleID)
	if game.ConsoleID != 0 {
		req.consoleID = uint32(game.ConsoleID)
	}
	// A missing file is left empty so the lookup does not try to hash it
	sel := game.SelectedDiscIndex()
	switch {
	case files.MissingAt(sel):
	case len(game.Discs) > 0:
		req.discFile = game.Discs[sel].File
	default:
		req.romFile = game.File
	}

	o.achMu.Lock()
	o.achLoading = true
	o.achMu.Unlock()
	go o.loadAchievementProgress(req)
}

// finishLoad stores a lookup's outcome and requests a rebuild, unless a
// later SetGame has retired the lookup's generation.
func (o *OverviewSection) finishLoad(gen int, found bool, progress *rcheevos.UserProgressEntry, err error) {
	o.achMu.Lock()
	if gen != o.loadGen {
		o.achMu.Unlock()
		return
	}
	o.achLoading = false
	o.achLoadErr = err
	o.achFound = found
	o.achProgress = progress
	o.achMu.Unlock()
	o.callback.RequestRebuild()
}

// NavZones returns the page's navigation zones from the last Build
func (o *OverviewSection) NavZones() []string {
	if len(o.discKeys) == 0 {
		return nil
	}
	return []string{discsZone}
}

// loadAchievementProgress loads the achievement progress described by
// req on a background goroutine and stores it through finishLoad
func (o *OverviewSection) loadAchievementProgress(req progressRequest) {
	// Ensure libraries are loaded first
	done := make(chan bool, 1)
	o.achievementManager.EnsureLibrariesLoaded(func(success bool) {
		done <- success
	})
	if !<-done {
		o.finishLoad(req.gen, false, nil, fmt.Errorf("failed to load achievement data"))
		return
	}

	// If achievements were unlocked during gameplay, refresh the cached progress
	if o.achievementManager.IsProgressDirty() {
		refreshDone := make(chan bool, 1)
		o.achievementManager.RefreshUserProgress(func(success bool) {
			refreshDone <- success
		})
		<-refreshDone
		o.achievementManager.ClearProgressDirty()
	}

	// Compute the game's hash. Disc games hash the selected disc (rcheevos
	// reads sectors through it); cartridge games hash the ROM file. A game
	// whose file is missing cannot be hashed.
	var md5Hash string
	switch {
	case req.discFile != "":
		disc, err := romloader.OpenDisc(req.discFile)
		if err != nil {
			o.finishLoad(req.gen, false, nil, err)
			return
		}
		md5Hash = o.achievementManager.ComputeGameHashFromDisc(disc, req.consoleID)
		disc.Close()
	case req.romFile != "":
		romData, _, err := romloader.Load(req.romFile, o.callback.GetExtensions())
		if err != nil {
			o.finishLoad(req.gen, false, nil, err)
			return
		}
		md5Hash = o.achievementManager.ComputeGameHash(romData, req.consoleID)
	default:
		o.finishLoad(req.gen, false, nil, nil)
		return
	}

	// Look up progress using MD5
	found, progress := o.achievementManager.LookupGameProgress(md5Hash)
	o.finishLoad(req.gen, found, progress, nil)
}

// Build creates the Overview page: the game metadata and, for multi-disc
// games, the disc selection
func (o *OverviewSection) Build(focus types.FocusManager) *widget.Container {
	o.discKeys = o.discKeys[:0]

	// Outer metadata container that anchors content to top-left
	metadataOuter := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Stretch: true,
			}),
		),
	)

	// Metadata uses the content area less the scrollbar
	metadataWidth := widgets.PageRowWidth(o.contentWidth())
	minMetadata := style.Px(200)
	if metadataWidth < minMetadata {
		metadataWidth = minMetadata
	}

	// Calculate pixel width for value text column
	// value column = metadataWidth - label - grid spacing - grid padding
	labelWidth := style.Px(80)
	valueWidth := metadataWidth - labelWidth - style.DefaultSpacing - style.SmallSpacing*2

	// Inner metadata container with fixed width
	metadataContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionStart,
				VerticalPosition:   widget.AnchorLayoutPositionStart,
				StretchHorizontal:  true,
			}),
			widget.WidgetOpts.MinSize(metadataWidth, 0),
		),
	)

	// Game Info section
	metadataContainer.AddChild(widgets.SectionHeader("Game Info"))

	md, hasMetadata := o.callback.GameMetadata(o.game.GameID)

	// Title (with warning icon if missing)
	titleText := o.callback.GameTitle(o.game)
	if o.files.Missing {
		titleText = "[!] " + titleText
	}
	metadataContainer.AddChild(buildMetadataRow("Title", titleText, valueWidth))
	if md.NativeTitle != "" {
		metadataContainer.AddChild(buildMetadataRow("Native Title", md.NativeTitle, valueWidth))
	}

	// Game ID: the metadata serial, else the product number of a disc game
	switch {
	case hasMetadata && md.Serial != "":
		metadataContainer.AddChild(buildMetadataRow("Game ID", md.Serial, valueWidth))
	case len(o.game.Discs) > 0:
		metadataContainer.AddChild(buildMetadataRow("Game ID", o.game.GameID, valueWidth))
	}
	region := strings.ToUpper(metadata.RegionCode(md.Regions))
	if region == "" {
		region = "Unknown"
	}
	metadataContainer.AddChild(buildMetadataRow("Region", region, valueWidth))

	// Discs section (multi-disc disc-based games only). Each disc is a
	// focusable selection button; activating it only marks that disc as
	// the selected disc (persisted) - it does not launch. The toolbar
	// Play button launches the selected disc, or removes it when its file
	// is missing. With no prior selection the first disc is selected by
	// default.
	if len(o.game.Discs) > 1 && !o.files.Missing {
		metadataContainer.AddChild(widgets.SectionHeader("Discs"))
		sel := o.game.SelectedDiscIndex()
		for i, d := range o.game.Discs {
			i, d := i, d
			label := d.Name
			if label == "" {
				label = fmt.Sprintf("Disc %d", d.Index+1)
			}
			if o.files.MissingAt(i) {
				label = "[!] " + label
			}
			if i == sel {
				label = "> " + label
			}
			handler := func(args *widget.ButtonClickedEventArgs) {
				o.game.SelectedDisc = i
				storage.SaveLibrary(o.library)
				o.callback.RequestRebuild()
			}
			var btn *widget.Button
			if i == sel {
				btn = widgets.PrimaryTextButton(label, style.ButtonPaddingMedium, handler)
			} else {
				btn = widgets.TextButton(label, style.ButtonPaddingMedium, handler)
			}
			key := fmt.Sprintf("disc-%d", i)
			focus.RegisterFocusButton(key, btn)
			o.discKeys = append(o.discKeys, key)
			metadataContainer.AddChild(btn)
		}
	}

	// Production section
	genre := strings.Join(md.Genres, ", ")
	players := playersText(md.MaxPlayers, md.Cooperative)
	released := metadata.FormatReleaseDate(md.ReleaseDate)
	hasProduction := md.Developer != "" || md.Publisher != "" ||
		genre != "" || players != "" || released != "" || md.ESRB != ""
	if hasProduction {
		metadataContainer.AddChild(widgets.SectionHeader("Production"))
		if md.Developer != "" {
			metadataContainer.AddChild(buildMetadataRow("Developer", md.Developer, valueWidth))
		}
		if md.Publisher != "" {
			metadataContainer.AddChild(buildMetadataRow("Publisher", md.Publisher, valueWidth))
		}
		if genre != "" {
			metadataContainer.AddChild(buildMetadataRow("Genre", genre, valueWidth))
		}
		if players != "" {
			metadataContainer.AddChild(buildMetadataRow("Players", players, valueWidth))
		}
		if released != "" {
			metadataContainer.AddChild(buildMetadataRow("Released", released, valueWidth))
		}
		if md.ESRB != "" {
			metadataContainer.AddChild(buildMetadataRow("ESRB", md.ESRB, valueWidth))
		}
	}

	// Activity section
	metadataContainer.AddChild(widgets.SectionHeader("Activity"))
	metadataContainer.AddChild(buildMetadataRow("Play Time", style.FormatPlayTime(o.game.PlayTimeSeconds), valueWidth))
	metadataContainer.AddChild(buildMetadataRow("Last Played", style.FormatLastPlayed(o.game.LastPlayed), valueWidth))
	metadataContainer.AddChild(buildMetadataRow("Added", style.FormatDate(o.game.Added), valueWidth))

	// Achievements section (only if logged in)
	if o.achievementManager != nil && o.achievementManager.IsLoggedIn() {
		metadataContainer.AddChild(widgets.SectionHeader("Achievements"))
		metadataContainer.AddChild(o.buildAchievementSection(valueWidth))
	}

	// Missing file warning: the whole game, or the selected disc
	warning := ""
	if o.files.Missing {
		warning = "ROM file not found"
	} else if o.files.MissingAt(o.game.SelectedDiscIndex()) {
		warning = "Disc file not found"
	}
	if warning != "" {
		metadataContainer.AddChild(widgets.SectionHeader("Warning"))
		warningRow := widget.NewContainer(
			widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Surface)),
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(style.SmallSpacing)),
			)),
			widget.ContainerOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
		)
		warningLabel := widget.NewText(
			widget.TextOpts.Text(warning, style.FontFace(), style.Accent),
		)
		warningRow.AddChild(warningLabel)
		metadataContainer.AddChild(warningRow)
	}

	// Overview: the game's description, wrapped, last on the page. Each
	// line is its own text so the lines can be spaced apart; an empty line
	// holds a space to keep its height.
	if md.Overview != "" {
		metadataContainer.AddChild(widgets.SectionHeader("Overview"))
		overviewRow := widget.NewContainer(
			widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Surface)),
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionVertical),
				widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(style.SmallSpacing)),
				widget.RowLayoutOpts.Spacing(style.Px(2)),
			)),
			widget.ContainerOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
		)
		for _, line := range style.WrapLines(md.Overview, *style.FontFace(), float64(metadataWidth-style.SmallSpacing*2)) {
			if line == "" {
				line = " "
			}
			overviewRow.AddChild(widget.NewText(
				widget.TextOpts.Text(line, style.FontFace(), style.Text),
			))
		}
		metadataContainer.AddChild(overviewRow)
	}

	metadataOuter.AddChild(metadataContainer)

	// Wrap content in a scrollable container so metadata scrolls when window is small
	scrollContent := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.DefaultSpacing),
		)),
	)
	scrollContent.AddChild(metadataOuter)

	scrollContainer, _, scrollWrapper := widgets.ScrollableContainer(widgets.PageScrollOpts(scrollContent))
	focus.SetScrollContainer(scrollContainer)
	focus.RestoreScrollPosition()

	page := widgets.FillContainer()
	page.AddChild(scrollWrapper)

	o.setupNavigation(focus)
	return page
}

// setupNavigation registers the disc list zone when discs are shown
func (o *OverviewSection) setupNavigation(focus types.FocusManager) {
	if len(o.discKeys) > 0 {
		focus.RegisterNavZone(discsZone, types.NavZoneVertical, o.discKeys, 0)
	}
}

// playersText returns the Players value: "1" or "1-n" for up to n players,
// with ", co-op" when the game is cooperative, or "" when neither is known
func playersText(maxPlayers int, coop bool) string {
	var s string
	switch {
	case maxPlayers == 1:
		s = "1"
	case maxPlayers > 1:
		s = fmt.Sprintf("1-%d", maxPlayers)
	}
	if coop {
		if s == "" {
			return "Co-op"
		}
		s += ", co-op"
	}
	return s
}

// buildMetadataRow creates a metadata row with background, label on left, value on right
// valuePixelWidth specifies the maximum pixel width for the value before truncation
func buildMetadataRow(label, value string, valuePixelWidth int) *widget.Container {
	row := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Surface)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(style.SmallSpacing)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	// Label (fixed width for alignment, scales with font size)
	labelText := widget.NewText(
		widget.TextOpts.Text(label, style.FontFace(), style.TextSecondary),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(80), 0),
		),
	)
	row.AddChild(labelText)

	// Value (truncated if necessary using pixel-based measurement)
	displayValue, wasTruncated := style.TruncateToWidth(value, *style.FontFace(), float64(valuePixelWidth))

	valueOpts := []widget.TextOpt{
		widget.TextOpts.Text(displayValue, style.FontFace(), style.Text),
	}

	// Add tooltip with full value if truncated
	if wasTruncated {
		valueOpts = append(valueOpts, widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.ToolTip(
				widget.NewToolTip(
					widget.ToolTipOpts.Content(widgets.TooltipContent(value)),
				),
			),
		))
	} else {
		valueOpts = append(valueOpts, widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		))
	}

	valueText := widget.NewText(valueOpts...)
	row.AddChild(valueText)

	return row
}

// buildAchievementSection creates the achievements section content
func (o *OverviewSection) buildAchievementSection(valuePixelWidth int) *widget.Container {
	container := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	o.achMu.Lock()
	loading := o.achLoading
	loadErr := o.achLoadErr
	found := o.achFound
	progress := o.achProgress
	o.achMu.Unlock()

	if loading {
		container.AddChild(buildMetadataRow("Status", "Loading...", valuePixelWidth))
		return container
	}

	if loadErr != nil {
		container.AddChild(buildMetadataRow("Status", "Unable to load", valuePixelWidth))
		return container
	}

	if !found {
		container.AddChild(buildMetadataRow("Status", "Not found", valuePixelWidth))
		return container
	}

	if progress == nil || progress.NumAchievements == 0 {
		container.AddChild(buildMetadataRow("Status", "No achievements", valuePixelWidth))
		return container
	}

	// Has achievements - show progress
	pct := 0
	if progress.NumAchievements > 0 {
		pct = int(progress.NumUnlockedAchievements * 100 / progress.NumAchievements)
	}
	progressText := fmt.Sprintf("%d / %d (%d%%)",
		progress.NumUnlockedAchievements, progress.NumAchievements, pct)
	container.AddChild(buildMetadataRow("Progress", progressText, valuePixelWidth))

	return container
}
