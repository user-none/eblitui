// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"fmt"
	"slices"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/filebrowser"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// LibrarySection manages ROM folder settings and the library's artwork
// settings
type LibrarySection struct {
	callback     types.ScreenCallback
	library      *storage.Library
	config       *storage.Config
	selectedDirs map[int]bool
	pendingScan  bool
	contentWidth func() int
}

// NewLibrarySection creates a new library section. contentWidth returns the
// width of the section content, used to fit the setting descriptions.
func NewLibrarySection(callback types.ScreenCallback, library *storage.Library, config *storage.Config, contentWidth func() int) *LibrarySection {
	return &LibrarySection{
		callback:     callback,
		library:      library,
		config:       config,
		selectedDirs: make(map[int]bool),
		contentWidth: contentWidth,
	}
}

// SetConfig updates the config reference
func (l *LibrarySection) SetConfig(config *storage.Config) {
	l.config = config
}

// HasPendingScan returns true if a scan should be triggered
func (l *LibrarySection) HasPendingScan() bool {
	return l.pendingScan
}

// ClearPendingScan clears the pending scan flag
func (l *LibrarySection) ClearPendingScan() {
	l.pendingScan = false
}

// SetLibrary updates the library reference
func (l *LibrarySection) SetLibrary(library *storage.Library) {
	l.library = library
}

// Build creates the library section UI
func (l *LibrarySection) Build(focus types.FocusManager) *widget.Container {
	// Use GridLayout so we can make the list stretch to fill available space
	section := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			// Row stretch: artwork header=no, artwork download=no, box art
			// region=no, folders header=no, list=YES, buttons=no, count=no
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, false, false, false, true, false, false}),
			widget.GridLayoutOpts.Spacing(0, style.ButtonPaddingMedium),
		)),
	)

	// Artwork settings
	section.AddChild(widgets.SectionHeader("Artwork"))
	section.AddChild(l.buildArtworkDownloadRow(focus))
	section.AddChild(l.buildBoxArtRegionRow(focus))

	section.AddChild(widgets.SectionHeader("ROM Folders"))

	// Create the folder list
	section.AddChild(l.buildFolderList(focus))

	// Track folder count for navigation setup
	folderCount := len(l.library.ScanDirectories())

	// Button row across the full width: the actions that add to the library
	// (Scan Library, Add Folder) on the left, the ones that take away from
	// it (Remove Folder, Clean Library) on the right, with the space between
	// them stretching
	buttonRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			widget.GridLayoutOpts.Stretch([]bool{false, true, false}, []bool{false}),
			widget.GridLayoutOpts.Spacing(style.ButtonPaddingMedium, 0),
		)),
	)
	addButtons := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(style.ButtonPaddingMedium),
		)),
	)
	removeButtons := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(style.ButtonPaddingMedium),
		)),
	)
	buttonRow.AddChild(addButtons)
	buttonRow.AddChild(widget.NewContainer())
	buttonRow.AddChild(removeButtons)

	// Scan Library button
	scanBtn := widgets.PrimaryTextButton("Scan Library", style.ButtonPaddingMedium, func(args *widget.ButtonClickedEventArgs) {
		l.callback.SwitchToScanProgress()
	})
	focus.RegisterFocusButton("lib-scan", scanBtn)
	addButtons.AddChild(scanBtn)

	// Add Folder button
	addDirBtn := widgets.TextButton("Add Folder...", style.ButtonPaddingMedium, func(args *widget.ButtonClickedEventArgs) {
		l.onAddDirectoryClick(focus)
	})
	focus.RegisterFocusButton("lib-add", addDirBtn)
	addButtons.AddChild(addDirBtn)

	// Remove Folder button - disabled when nothing selected, removes all selected folders
	removeBtn := widgets.EnabledTextButton("Remove Folder", style.ButtonPaddingMedium, len(l.selectedDirs) > 0, func(args *widget.ButtonClickedEventArgs) {
		// The selection indexes the list as shown, so remove by path
		dirs := l.library.ScanDirectories()
		for idx, dir := range dirs {
			if l.selectedDirs[idx] {
				l.library.RemoveScanDirectory(dir.Path)
			}
		}
		l.selectedDirs = make(map[int]bool) // Clear selection
		l.callback.RequestRebuild()
	})
	focus.RegisterFocusButton("lib-remove", removeBtn)
	removeButtons.AddChild(removeBtn)

	// Clean Library button - removes missing discs and games
	cleanBtn := widgets.TextButton("Clean Library", style.ButtonPaddingMedium, func(args *widget.ButtonClickedEventArgs) {
		l.callback.SwitchToCleanProgress()
	})
	focus.RegisterFocusButton("lib-clean", cleanBtn)
	removeButtons.AddChild(cleanBtn)

	section.AddChild(buttonRow)

	// Game count, the last row
	gameCount := l.library.GameCount()
	countText := "No games in library"
	if gameCount == 1 {
		countText = "1 game in library"
	} else if gameCount > 1 {
		countText = fmt.Sprintf("%d games in library", gameCount)
	}

	countLabel := widget.NewText(
		widget.TextOpts.Text(countText, style.FontFace(), style.TextSecondary),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				HorizontalPosition: widget.GridLayoutPositionCenter,
			}),
		),
	)
	section.AddChild(countLabel)

	// Set up navigation zones
	l.setupNavigation(focus, folderCount)

	return section
}

// buildArtworkDownloadRow creates the row choosing which artwork a library
// scan downloads. It only limits downloads; artwork already on disk is
// kept and shown.
func (l *LibrarySection) buildArtworkDownloadRow(focus types.FocusManager) *widget.Container {
	return l.buildCycleRow(focus, "lib-artwork-download", "Artwork Download", "Downloaded during library scan",
		storage.ValidArtworkDownloads, l.config.Values().Library.ArtworkDownload, storage.ArtworkDownloadDisplayName, l.config.SetArtworkDownload)
}

// buildBoxArtRegionRow creates the row choosing the region whose artwork is
// shown first for a game released in more than one region
func (l *LibrarySection) buildBoxArtRegionRow(focus types.FocusManager) *widget.Container {
	return l.buildCycleRow(focus, "lib-boxart-region", "Preferred Box Art Region", "Which region's cover the library shows when a game has more than one",
		storage.ValidBoxArtRegions, l.config.Values().Library.BoxArtRegion, storage.BoxArtRegionDisplayName, l.config.SetBoxArtRegion)
}

// buildCycleRow creates a settings row with a label, a description cut to
// fit the row, and a cycle button over options starting at current.
// displayName labels an option and set stores the option chosen.
func (l *LibrarySection) buildCycleRow(focus types.FocusManager, key, label, description string, options []string, current string, displayName func(string) string, set func(string)) *widget.Container {
	row := widgets.SettingsRow(2)

	minWidth := 0
	for _, o := range options {
		minWidth = max(minWidth, widgets.ButtonWidth(displayName(o)))
	}
	maxW := float64(widgets.SettingsRowLabelWidth(l.contentWidth(), minWidth))
	face := *style.FontFace()
	displayLabel, _ := style.TruncateToWidth(label, face, maxW)
	displayDesc, _ := style.TruncateToWidth(description, face, maxW)
	row.AddChild(widgets.LabeledText(displayLabel, displayDesc))

	cycleBtn := widgets.CycleButton(displayName(current), slices.Index(options, current), len(options), minWidth, func(next int) {
		set(options[next])
		focus.SetPendingFocus(key)
		l.callback.RequestRebuild()
	})
	focus.RegisterFocusButton(key, cycleBtn)
	row.AddChild(cycleBtn)

	return row
}

// setupNavigation registers navigation zones for the library section
func (l *LibrarySection) setupNavigation(focus types.FocusManager, folderCount int) {
	// Artwork rows, the first zone
	focus.RegisterNavZone("lib-artwork", types.NavZoneVertical, []string{"lib-artwork-download", "lib-boxart-region"}, 0)

	// Folder list zone (vertical)
	if folderCount > 0 {
		folderKeys := make([]string, folderCount)
		for i := 0; i < folderCount; i++ {
			folderKeys[i] = fmt.Sprintf("folder-%d", i)
		}
		focus.RegisterNavZone("lib-folders", types.NavZoneVertical, folderKeys, 0)
	}

	// Button row zone (horizontal), in screen order left to right
	buttonKeys := []string{"lib-scan", "lib-add", "lib-remove", "lib-clean"}
	focus.RegisterNavZone("lib-buttons", types.NavZoneHorizontal, buttonKeys, 0)

	// Transitions. The folder list sits between the artwork rows and the
	// buttons; with no folders the artwork rows lead to the buttons.
	if folderCount > 0 {
		focus.SetNavTransition("lib-artwork", types.DirDown, "lib-folders", types.NavIndexFirst)
		focus.SetNavTransition("lib-folders", types.DirUp, "lib-artwork", types.NavIndexLast)
		focus.SetNavTransition("lib-folders", types.DirDown, "lib-buttons", types.NavIndexFirst)
		focus.SetNavTransition("lib-buttons", types.DirUp, "lib-folders", types.NavIndexLast)
	} else {
		focus.SetNavTransition("lib-artwork", types.DirDown, "lib-buttons", types.NavIndexFirst)
		focus.SetNavTransition("lib-buttons", types.DirUp, "lib-artwork", types.NavIndexLast)
	}
}

// buildFolderList creates a selectable folder list with scrolling
func (l *LibrarySection) buildFolderList(focus types.FocusManager) widget.PreferredSizeLocateableWidget {
	maxPathChars := 70

	// Create list content container
	listContent := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(0),
		)),
	)

	dirs := l.library.ScanDirectories()
	if len(dirs) == 0 {
		// Empty state - centered text
		emptyContainer := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
			widget.ContainerOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, style.SettingsFolderListMinHeight),
			),
		)
		emptyLabel := widget.NewText(
			widget.TextOpts.Text("No folders added", style.FontFace(), style.TextSecondary),
			widget.TextOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
					HorizontalPosition: widget.AnchorLayoutPositionCenter,
					VerticalPosition:   widget.AnchorLayoutPositionCenter,
				}),
			),
		)
		emptyContainer.AddChild(emptyLabel)
		listContent.AddChild(emptyContainer)
	} else {
		for i, dir := range dirs {
			idx := i
			dirPath := dir.Path
			displayPath, wasTruncated := style.TruncateStart(dirPath, maxPathChars)

			// Determine row background based on selection state
			var rowBg = style.Background
			if l.selectedDirs[idx] {
				rowBg = style.Primary // Selected items show primary color
			} else if idx%2 == 1 {
				rowBg = style.Surface // Alternating colors for unselected
			}

			// Create row content with path label (no background - button handles colors for focus states)
			rowContent := widget.NewContainer(
				widget.ContainerOpts.Layout(widget.NewAnchorLayout(
					widget.AnchorLayoutOpts.Padding(&widget.Insets{Left: style.ButtonPaddingMedium, Right: style.ButtonPaddingMedium}),
				)),
				widget.ContainerOpts.WidgetOpts(
					widget.WidgetOpts.MinSize(0, style.SettingsRowHeight),
				),
			)

			// Build path label widget options
			pathWidgetOpts := []widget.WidgetOpt{
				widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
					HorizontalPosition: widget.AnchorLayoutPositionStart,
					VerticalPosition:   widget.AnchorLayoutPositionCenter,
				}),
			}

			// Add tooltip if path was truncated
			if wasTruncated {
				pathWidgetOpts = append(pathWidgetOpts, widget.WidgetOpts.ToolTip(
					widget.NewToolTip(
						widget.ToolTipOpts.Content(widgets.TooltipContent(dirPath)),
					),
				))
			}

			pathLabel := widget.NewText(
				widget.TextOpts.Text(displayPath, style.FontFace(), style.Text),
				widget.TextOpts.WidgetOpts(pathWidgetOpts...),
			)
			rowContent.AddChild(pathLabel)

			// Wrap in a button for click handling (selection)
			rowButton := widget.NewButton(
				widget.ButtonOpts.Image(&widget.ButtonImage{
					Idle:    image.NewNineSliceColor(rowBg),
					Hover:   image.NewNineSliceColor(style.PrimaryHover),
					Pressed: image.NewNineSliceColor(style.Primary),
				}),
				widget.ButtonOpts.WidgetOpts(
					widget.WidgetOpts.LayoutData(widget.RowLayoutData{
						Stretch: true,
					}),
					widget.WidgetOpts.MinSize(0, style.SettingsRowHeight),
				),
				widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
					// Toggle selection - click to select, click again to deselect
					if l.selectedDirs[idx] {
						delete(l.selectedDirs, idx)
					} else {
						l.selectedDirs[idx] = true
					}
					focus.SetPendingFocus(fmt.Sprintf("folder-%d", idx))
					l.callback.RequestRebuild()
				}),
			)

			// Store button reference for focus restoration
			focus.RegisterFocusButton(fmt.Sprintf("folder-%d", idx), rowButton)

			// Stack button and content
			rowWrapper := widget.NewContainer(
				widget.ContainerOpts.Layout(widget.NewStackedLayout()),
				widget.ContainerOpts.WidgetOpts(
					widget.WidgetOpts.LayoutData(widget.RowLayoutData{
						Stretch: true,
					}),
					widget.WidgetOpts.MinSize(0, style.SettingsRowHeight),
				),
			)
			rowWrapper.AddChild(rowButton)
			rowWrapper.AddChild(rowContent)

			listContent.AddChild(rowWrapper)
		}
	}

	// Create scrollable container with border
	scroll, bar, wrapper := widgets.ScrollableContainer(widgets.ScrollableOpts{
		Content:     listContent,
		BgColor:     style.Surface,
		BorderColor: style.Border,
		Spacing:     0,
		Padding:     2,
	})
	focus.RegisterScrollBar("scrollbar", scroll, bar)

	return wrapper
}

// onAddDirectoryClick opens the folder picker and adds the chosen folder
func (l *LibrarySection) onAddDirectoryClick(focus types.FocusManager) {
	// Return focus to the Add Folder button when the picker closes
	focus.SetPendingFocus("lib-add")
	l.callback.OpenFilePicker(types.FilePickerRequest{
		Title: "Select ROM Folder",
		Mode:  filebrowser.ModeFolder,
		OnSelect: func(path string) {
			l.library.AddScanDirectory(path, true) // recursive=true by default
			// Trigger auto-scan after adding directory
			l.pendingScan = true
		},
	})
}
