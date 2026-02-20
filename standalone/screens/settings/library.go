//go:build !libretro

package settings

import (
	"fmt"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/sqweek/dialog"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
	"github.com/user-none/eblitui/standalone/types"
)

// LibrarySection manages ROM folder settings
type LibrarySection struct {
	callback     types.ScreenCallback
	library      *storage.Library
	selectedDirs map[int]bool
	pendingScan  bool
}

// NewLibrarySection creates a new library section
func NewLibrarySection(callback types.ScreenCallback, library *storage.Library) *LibrarySection {
	return &LibrarySection{
		callback:     callback,
		library:      library,
		selectedDirs: make(map[int]bool),
	}
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
			// Row stretch: label=no, list=YES, buttons=no, count=no
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true, false, false}),
			widget.GridLayoutOpts.Spacing(0, style.ButtonPaddingMedium),
		)),
	)

	// ROM Folders label
	dirLabel := widget.NewText(
		widget.TextOpts.Text("ROM Folders", style.FontFace(), style.Accent),
	)
	section.AddChild(dirLabel)

	// Create the folder list
	section.AddChild(l.buildFolderList(focus))

	// Track folder count for navigation setup
	folderCount := len(l.library.ScanDirectories)

	// Button row: Add Folder | Scan Library | Remove (centered)
	buttonRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(style.ButtonPaddingMedium),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				HorizontalPosition: widget.GridLayoutPositionCenter,
			}),
		),
	)

	// Add Folder button
	addDirBtn := style.TextButton("Add Folder...", style.ButtonPaddingMedium, func(args *widget.ButtonClickedEventArgs) {
		l.onAddDirectoryClick()
	})
	focus.RegisterFocusButton("lib-add", addDirBtn)
	buttonRow.AddChild(addDirBtn)

	// Scan Library button
	scanBtn := style.PrimaryTextButton("Scan Library", style.ButtonPaddingMedium, func(args *widget.ButtonClickedEventArgs) {
		l.callback.SwitchToScanProgress(true)
	})
	focus.RegisterFocusButton("lib-scan", scanBtn)
	buttonRow.AddChild(scanBtn)

	// Remove button - disabled when nothing selected, removes all selected folders
	removeButtonImage := style.ButtonImage()
	if len(l.selectedDirs) == 0 {
		removeButtonImage = style.DisabledButtonImage()
	}
	removeBtn := widget.NewButton(
		widget.ButtonOpts.Image(removeButtonImage),
		widget.ButtonOpts.Text("Remove", style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingMedium)),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			if len(l.selectedDirs) > 0 {
				// Collect paths to remove (iterate in reverse to avoid index shifting issues)
				for idx := len(l.library.ScanDirectories) - 1; idx >= 0; idx-- {
					if l.selectedDirs[idx] {
						path := l.library.ScanDirectories[idx].Path
						l.library.RemoveScanDirectory(path)
					}
				}
				l.selectedDirs = make(map[int]bool) // Clear selection
				storage.SaveLibrary(l.library)
				l.callback.RequestRebuild()
			}
		}),
	)
	focus.RegisterFocusButton("lib-remove", removeBtn)
	buttonRow.AddChild(removeBtn)

	section.AddChild(buttonRow)

	// Game count
	gameCount := len(l.library.Games)
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

// setupNavigation registers navigation zones for the library section
func (l *LibrarySection) setupNavigation(focus types.FocusManager, folderCount int) {
	// Folder list zone (vertical)
	if folderCount > 0 {
		folderKeys := make([]string, folderCount)
		for i := 0; i < folderCount; i++ {
			folderKeys[i] = fmt.Sprintf("folder-%d", i)
		}
		focus.RegisterNavZone("lib-folders", types.NavZoneVertical, folderKeys, 0)
	}

	// Button row zone (horizontal)
	buttonKeys := []string{"lib-add", "lib-scan", "lib-remove"}
	focus.RegisterNavZone("lib-buttons", types.NavZoneHorizontal, buttonKeys, 0)

	// Transitions
	if folderCount > 0 {
		focus.SetNavTransition("lib-folders", types.DirDown, "lib-buttons", types.NavIndexFirst)
		focus.SetNavTransition("lib-buttons", types.DirUp, "lib-folders", types.NavIndexLast)
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

	if len(l.library.ScanDirectories) == 0 {
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
		for i, dir := range l.library.ScanDirectories {
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
						widget.ToolTipOpts.Content(style.TooltipContent(dirPath)),
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
	_, _, wrapper := style.ScrollableContainer(style.ScrollableOpts{
		Content:     listContent,
		BgColor:     style.Surface,
		BorderColor: style.Border,
		Spacing:     0,
		Padding:     2,
	})

	return wrapper
}

// onAddDirectoryClick handles adding a search directory
func (l *LibrarySection) onAddDirectoryClick() {
	// Run dialog in goroutine to avoid blocking Ebiten's main thread
	go func() {
		path, err := dialog.Directory().
			Title("Select ROM Folder").
			Browse()
		if err != nil {
			return // User cancelled or error
		}
		l.library.AddScanDirectory(path, true) // recursive=true by default
		storage.SaveLibrary(l.library)
		// Trigger auto-scan after adding directory
		l.pendingScan = true
	}()
}
