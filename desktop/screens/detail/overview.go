// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package detail

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/achievements"
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
	crc32     string // game key, for the RDB MD5 lookup
	consoleID uint32 // console used to hash the game when not in the RDB
	discFile  string // selected disc to hash, for disc games
	romFile   string // ROM to hash, for cartridge games
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

// SetGame sets the game to show and starts loading its achievement
// progress when logged in
func (o *OverviewSection) SetGame(game *storage.GameEntry) {
	o.game = game

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
	req.crc32 = game.CRC32
	req.consoleID = uint32(o.defaultConsoleID)
	if game.ConsoleID != 0 {
		req.consoleID = uint32(game.ConsoleID)
	}
	if len(game.Discs) > 0 {
		idx := game.SelectedDisc
		if idx < 0 || idx >= len(game.Discs) {
			idx = 0
		}
		req.discFile = game.Discs[idx].File
	} else {
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

	// Get MD5 from RDB (fast path - no ROM loading needed)
	crc32, _ := strconv.ParseUint(req.crc32, 16, 32)
	md5Hash := o.callback.GetMD5ByCRC32(uint32(crc32))

	// Fallback: compute the hash directly when not in the RDB. Disc games hash
	// the selected disc (rcheevos reads sectors through it); cartridge games
	// hash the ROM file.
	if md5Hash == "" {
		if req.discFile != "" {
			disc, err := romloader.OpenDisc(req.discFile)
			if err != nil {
				o.finishLoad(req.gen, false, nil, err)
				return
			}
			md5Hash = o.achievementManager.ComputeGameHashFromDisc(disc, req.consoleID)
			disc.Close()
		} else {
			romData, _, err := romloader.Load(req.romFile, o.callback.GetExtensions())
			if err != nil {
				o.finishLoad(req.gen, false, nil, err)
				return
			}
			md5Hash = o.achievementManager.ComputeGameHash(romData, req.consoleID)
		}
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
	minMetadata := style.PxFont(200)
	if metadataWidth < minMetadata {
		metadataWidth = minMetadata
	}

	// Calculate pixel width for value text column
	// Label width scales with font size; value column = metadataWidth - label - grid spacing - grid padding
	labelWidth := style.PxFont(80)
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

	// Title (with warning icon if missing)
	titleText := o.game.DisplayName
	if o.game.Missing {
		titleText = "[!] " + titleText
	}
	metadataContainer.AddChild(buildMetadataRow("Title", titleText, valueWidth))

	if o.game.Name != "" && o.game.Name != o.game.DisplayName {
		metadataContainer.AddChild(buildMetadataRow("Name", o.game.Name, valueWidth))
	}
	if o.game.Serial != "" {
		metadataContainer.AddChild(buildMetadataRow("Game ID", o.game.Serial, valueWidth))
	}
	region := strings.ToUpper(o.game.Region)
	if region == "" {
		region = "Unknown"
	}
	metadataContainer.AddChild(buildMetadataRow("Region", region, valueWidth))

	if o.game.System != "" {
		metadataContainer.AddChild(buildMetadataRow("System", o.game.System, valueWidth))
	}

	// Discs section (multi-disc disc-based games only). Each disc is a
	// focusable selection button; activating it only marks that disc as
	// the selected disc (persisted) - it does not launch. The toolbar
	// Play button launches the selected disc. With no prior selection the
	// first disc is selected by default.
	if len(o.game.Discs) > 1 && !o.game.Missing {
		metadataContainer.AddChild(widgets.SectionHeader("Discs"))
		sel := o.game.SelectedDisc
		if sel < 0 || sel >= len(o.game.Discs) {
			sel = 0
		}
		for i, d := range o.game.Discs {
			i, d := i, d
			label := d.Name
			if label == "" {
				label = fmt.Sprintf("Disc %d", d.Index+1)
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
	hasProduction := o.game.Developer != "" || o.game.Publisher != "" ||
		o.game.Genre != "" || o.game.Franchise != "" ||
		o.game.ReleaseDate != "" || o.game.ESRBRating != ""
	if hasProduction {
		metadataContainer.AddChild(widgets.SectionHeader("Production"))
		if o.game.Developer != "" {
			metadataContainer.AddChild(buildMetadataRow("Developer", o.game.Developer, valueWidth))
		}
		if o.game.Publisher != "" {
			metadataContainer.AddChild(buildMetadataRow("Publisher", o.game.Publisher, valueWidth))
		}
		if o.game.Genre != "" {
			metadataContainer.AddChild(buildMetadataRow("Genre", o.game.Genre, valueWidth))
		}
		if o.game.Franchise != "" {
			metadataContainer.AddChild(buildMetadataRow("Franchise", o.game.Franchise, valueWidth))
		}
		if o.game.ReleaseDate != "" {
			metadataContainer.AddChild(buildMetadataRow("Released", o.game.ReleaseDate, valueWidth))
		}
		if o.game.ESRBRating != "" {
			metadataContainer.AddChild(buildMetadataRow("ESRB", o.game.ESRBRating, valueWidth))
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

	// Missing ROM warning
	if o.game.Missing {
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
			widget.TextOpts.Text("ROM file not found", style.FontFace(), style.Accent),
		)
		warningRow.AddChild(warningLabel)
		metadataContainer.AddChild(warningRow)
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
			widget.WidgetOpts.MinSize(style.PxFont(80), 0),
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
