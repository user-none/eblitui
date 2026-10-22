// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package detail

import (
	"errors"
	"fmt"
	"image/color"
	"image/png"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/user-none/eblitui/desktop/launch"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

const (
	// screenshotsZone is the navigation zone of the screenshot list
	screenshotsZone = "screenshots"
	// screenshotActionsZone is the navigation zone of the preview's action
	// row
	screenshotActionsZone = "screenshot-actions"
	// screenshotFolderKey and screenshotDeleteKey are the focus keys of the
	// action row buttons
	screenshotFolderKey = "screenshot-folder"
	screenshotDeleteKey = "screenshot-delete"
	// screenshotTimeFormat lays out a capture time in the list. It is also
	// the sample the list width is measured from.
	screenshotTimeFormat = "2006-01-02 15:04:05"
	// maxScreenshotDim is the widest or tallest screenshot the preview
	// decodes, and maxScreenshotPixels the most pixels, bounding the memory
	// one file can take to about 128 MB. A full window on a 6K display
	// fits.
	maxScreenshotDim    = 8192
	maxScreenshotPixels = 32_000_000
)

// ScreenshotsSection is the detail Screenshots page: the game's
// screenshots listed newest first beside a preview of the selected one,
// with its filename, the achievement it was taken for, and the actions on
// it
type ScreenshotsSection struct {
	callback      types.ScreenCallback
	contentWidth  func() int
	dir           string               // the game's screenshot directory
	shots         []storage.Screenshot // newest first
	listErr       error
	selected      int                   // index into shots of the previewed screenshot, or -1
	confirmDelete widgets.ConfirmDelete // the two-press Delete of the selected screenshot
	zones         []string
	graphic       *widgets.FitGraphic // shows the preview in the last Build, or nil

	// Preview loading state. loadGen counts preview starts; a decode
	// started for an earlier selection carries an older generation and its
	// result is dropped.
	mu      sync.Mutex
	loadGen int
	loading bool
	loadErr error
	preview *ebiten.Image // decoded texture of the selected screenshot

	// gameDir returns a game's screenshot directory:
	// storage.GetGameScreenshotDir
	gameDir func(gameCRC string) (string, error)
	// decode reads a screenshot into a texture: decodeScreenshot
	decode func(path string) (*ebiten.Image, error)
	// showInFileManager shows a screenshot in the file manager:
	// launch.ShowInFileManager
	showInFileManager func(path string) error
}

// NewScreenshotsSection creates the Screenshots page. contentWidth returns
// the width of the page content area.
func NewScreenshotsSection(callback types.ScreenCallback, contentWidth func() int) *ScreenshotsSection {
	return &ScreenshotsSection{
		callback:          callback,
		contentWidth:      contentWidth,
		selected:          -1,
		gameDir:           storage.GetGameScreenshotDir,
		decode:            decodeScreenshot,
		showInFileManager: launch.ShowInFileManager,
	}
}

// SetGame lists the game's screenshots and previews the newest. A nil game
// clears the page. Either way the previous preview is freed.
func (s *ScreenshotsSection) SetGame(game *storage.GameEntry) {
	s.ReleasePreview()
	s.dir = ""
	s.shots = nil
	s.listErr = nil
	s.selected = -1
	s.confirmDelete.Disarm()
	if game == nil {
		return
	}
	dir, err := s.gameDir(game.CRC32)
	if err != nil {
		s.listErr = err
		return
	}
	s.dir = dir
	s.reload()
	s.startPreview()
}

// reload lists the screenshots again. The selection stays at the same
// position, which after a delete is the next older screenshot, and moves
// to the newest when it had none.
func (s *ScreenshotsSection) reload() {
	s.shots, s.listErr = storage.ListScreenshots(s.dir)
	if s.listErr != nil {
		log.Printf("Unable to list screenshots in %s: %v", s.dir, s.listErr)
		s.shots = nil
	}
	switch {
	case len(s.shots) == 0:
		s.selected = -1
	case s.selected < 0:
		s.selected = 0
	case s.selected >= len(s.shots):
		s.selected = len(s.shots) - 1
	}
}

// NavZones returns the page's navigation zones from the last Build
func (s *ScreenshotsSection) NavZones() []string {
	return s.zones
}

// HandleBack cancels a pending delete confirmation, reporting whether there
// was one
func (s *ScreenshotsSection) HandleBack() bool {
	if !s.confirmDelete.Disarm() {
		return false
	}
	s.callback.RequestRebuild()
	return true
}

// Build creates the page: the list on the left and the preview of the
// selected screenshot on the right, or a notice when there is nothing to
// show
func (s *ScreenshotsSection) Build(focus types.FocusManager) *widget.Container {
	s.zones = s.zones[:0]
	page := widgets.FillContainer()

	switch {
	case s.listErr != nil:
		page.AddChild(widgets.EmptyState("Unable to read screenshots", "The screenshots folder could not be read", nil))
		return page
	case len(s.shots) == 0:
		page.AddChild(widgets.EmptyState("No screenshots", "Press F12 during gameplay to take one", nil))
		return page
	}

	body := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
		)),
	)
	listWidth := s.listWidth()
	list, listOuterWidth := s.buildList(focus, listWidth)
	body.AddChild(list)
	body.AddChild(s.buildPreview(focus, s.contentWidth()-listOuterWidth-style.DefaultSpacing))
	page.AddChild(body)

	// The list is left of the actions: Right leaves the list for the first
	// action, Left from the first action returns to the selected row
	focus.SetNavTransition(screenshotsZone, types.DirRight, screenshotActionsZone, types.NavIndexFirst)
	focus.SetNavTransition(screenshotActionsZone, types.DirLeft, screenshotsZone, s.selected)
	return page
}

// SelectedRow returns the index of the selected row in the list zone, or
// -1 when the list is empty. Navigation entering the list lands on it.
func (s *ScreenshotsSection) SelectedRow() int {
	return s.selected
}

// listWidth returns the width of a list row: the sidebar minimum, or wider
// when a capture time needs more room beside the marker column
func (s *ScreenshotsSection) listWidth() int {
	timeWidth := int(math.Ceil(style.MeasureWidth(screenshotTimeFormat)))
	return max(style.SidebarMinWidth, style.ListColFavorite+timeWidth+style.SmallSpacing*3)
}

// shotKey returns the focus key of list row i
func shotKey(i int) string {
	return fmt.Sprintf("screenshot-%d", i)
}

// buildList creates the scrolling list of screenshots with rows rowWidth
// wide, in the style of the library list: a marker column, starred for
// achievement captures, and the capture time on alternating row colors.
// The selected row is highlighted. Returns the list and its outer width
// including the scrollbar.
func (s *ScreenshotsSection) buildList(focus types.FocusManager, rowWidth int) (widget.PreferredSizeLocateableWidget, int) {
	list := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		)),
	)

	keys := make([]string, 0, len(s.shots))
	for i, shot := range s.shots {
		idx := i
		key := shotKey(i)

		marker := ""
		if shot.Achievement != "" {
			marker = "*"
		}
		cells := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewGridLayout(
				widget.GridLayoutOpts.Columns(2),
				widget.GridLayoutOpts.Stretch([]bool{false, true}, nil),
				widget.GridLayoutOpts.Spacing(style.SmallSpacing, 0),
				widget.GridLayoutOpts.Padding(&widget.Insets{Left: style.SmallSpacing, Right: style.SmallSpacing}),
			)),
			widget.ContainerOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(rowWidth, style.ListRowHeight),
			),
		)
		cells.AddChild(widgets.TableCell(marker, style.ListColFavorite, style.ListRowHeight, style.Accent))
		cells.AddChild(widgets.TableCell(shot.Time.Format(screenshotTimeFormat), 0, style.ListRowHeight, style.Text))

		// Rows alternate like the library list; the selected row takes the
		// primary color instead
		idle := widgets.AlternatingRowColor(idx)
		if idx == s.selected {
			idle = style.Primary
		}
		btn := widget.NewButton(
			widget.ButtonOpts.Image(&widget.ButtonImage{
				Idle:    image.NewNineSliceColor(idle),
				Hover:   image.NewNineSliceColor(style.PrimaryHover),
				Pressed: image.NewNineSliceColor(style.Primary),
			}),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
				widget.WidgetOpts.MinSize(rowWidth, style.ListRowHeight),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) {
				s.selectShot(idx, focus)
			}),
		)
		focus.RegisterFocusButton(key, btn)
		keys = append(keys, key)

		// Stack: button below for the background and highlight, cells on
		// top
		row := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewStackedLayout()),
			widget.ContainerOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
				widget.WidgetOpts.MinSize(rowWidth, style.ListRowHeight),
			),
		)
		row.AddChild(btn)
		row.AddChild(cells)
		list.AddChild(row)
	}
	focus.RegisterNavZone(screenshotsZone, types.NavZoneVertical, keys, 0)
	s.zones = append(s.zones, screenshotsZone)

	opts := widgets.PageScrollOpts(list)
	scroll, _, wrapper := widgets.ScrollableContainer(opts)
	outerWidth := rowWidth + opts.Spacing + style.ScrollbarWidth
	wrapper.GetWidget().MinWidth = outerWidth
	focus.SetScrollContainer(scroll)
	focus.RestoreScrollPosition()
	return wrapper, outerWidth
}

// selectShot previews screenshot i and keeps focus on its row
func (s *ScreenshotsSection) selectShot(i int, focus types.FocusManager) {
	if i < 0 || i >= len(s.shots) || i == s.selected {
		return
	}
	s.selected = i
	s.startPreview()
	focus.SetPendingFocus(shotKey(i))
	s.callback.RequestRebuild()
}

// buildPreview creates the preview column, width wide: the selected
// screenshot filling the height left over, then its filename, the
// achievement it was taken for, and the action row
func (s *ScreenshotsSection) buildPreview(focus types.FocusManager, width int) *widget.Container {
	shot := s.shots[s.selected]

	box := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Black)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	s.mu.Lock()
	preview, loading, loadErr := s.preview, s.loading, s.loadErr
	s.mu.Unlock()
	s.graphic = nil
	switch {
	case preview != nil:
		s.graphic = widgets.NewFitGraphic(preview, widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			StretchHorizontal: true,
			StretchVertical:   true,
		}))
		box.AddChild(s.graphic)
	case loading:
		box.AddChild(centeredText("Loading..."))
	case loadErr != nil:
		box.AddChild(centeredText("Unable to read screenshot"))
	}

	rows := []widget.PreferredSizeLocateableWidget{
		box,
		truncatedText(filepath.Base(shot.Path), width, style.TextSecondary),
	}
	if shot.Achievement != "" {
		rows = append(rows, truncatedText(shot.Achievement, width, style.Text))
	}
	rows = append(rows, s.buildActions(focus))

	stretch := make([]bool, len(rows))
	stretch[0] = true // the image takes the height the rows below leave
	column := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, stretch),
			widget.GridLayoutOpts.Spacing(0, style.SmallSpacing),
		)),
	)
	for _, row := range rows {
		column.AddChild(row)
	}
	return column
}

// buildActions creates the action row under the preview: Show in Folder
// and Delete. Delete asks for a second press, shown as Confirm Delete.
func (s *ScreenshotsSection) buildActions(focus types.FocusManager) *widget.Container {
	folder := widgets.TextButton("Show in Folder", style.ButtonPaddingSmall, func(*widget.ButtonClickedEventArgs) {
		s.showSelectedInFileManager()
	})
	focus.RegisterFocusButton(screenshotFolderKey, folder)

	del := s.confirmDelete.Button(screenshotDeleteKey, style.ButtonPaddingSmall, focus, s.callback.RequestRebuild, func() {
		s.deleteSelected(focus)
	})
	focus.RegisterFocusButton(screenshotDeleteKey, del)

	focus.RegisterNavZone(screenshotActionsZone, types.NavZoneHorizontal, []string{screenshotFolderKey, screenshotDeleteKey}, 0)
	s.zones = append(s.zones, screenshotActionsZone)
	return widgets.ActionRow(folder, del)
}

// showSelectedInFileManager shows the selected screenshot in the file manager
func (s *ScreenshotsSection) showSelectedInFileManager() {
	if s.selected < 0 {
		return
	}
	if err := s.showInFileManager(s.shots[s.selected].Path); err != nil {
		log.Printf("Show in Folder failed: %v", err)
		s.callback.ShowNotification("Unable to open the screenshots folder")
	}
}

// deleteSelected deletes the selected screenshot, once its Delete is
// confirmed. The selection moves to the next older screenshot.
func (s *ScreenshotsSection) deleteSelected(focus types.FocusManager) {
	if s.selected < 0 {
		return
	}
	// A file already removed outside the app is the outcome asked for; the
	// entry is dropped by the re-list
	if err := storage.DeleteScreenshot(s.dir, s.shots[s.selected].Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("Delete screenshot failed: %v", err)
		s.callback.ShowNotification("Unable to delete the screenshot")
	}
	s.reload()
	s.startPreview()
	if s.selected >= 0 {
		focus.SetPendingFocus(shotKey(s.selected))
	}
	s.callback.RequestRebuild()
}

// startPreview frees the current preview and decodes the selected
// screenshot in the background. The result arrives through finishPreview.
func (s *ScreenshotsSection) startPreview() {
	s.ReleasePreview()
	if s.selected < 0 {
		return
	}
	path := s.shots[s.selected].Path

	s.mu.Lock()
	gen := s.loadGen
	s.loading = true
	s.mu.Unlock()

	go func() {
		img, err := s.decode(path)
		s.finishPreview(gen, img, err)
	}()
}

// ReleasePreview frees the preview texture and retires any decode in
// flight, whose result is then dropped. The detail screen calls it when it
// is left, so the texture does not stay allocated during gameplay; SetGame
// starts the preview again on return.
func (s *ScreenshotsSection) ReleasePreview() {
	// The widget tree of the last build is drawn once more before the
	// rebuild. Detach the texture from it first, since drawing a
	// deallocated image allocates it again.
	if s.graphic != nil {
		s.graphic.Image = nil
	}
	s.mu.Lock()
	s.loadGen++
	s.loading = false
	s.loadErr = nil
	if s.preview != nil {
		s.preview.Deallocate()
		s.preview = nil
	}
	s.mu.Unlock()
}

// finishPreview stores a decode's outcome and requests a rebuild, unless a
// later selection has retired the decode's generation
func (s *ScreenshotsSection) finishPreview(gen int, img *ebiten.Image, err error) {
	s.mu.Lock()
	if gen != s.loadGen {
		s.mu.Unlock()
		if img != nil {
			img.Deallocate()
		}
		return
	}
	s.loading = false
	s.loadErr = err
	s.preview = img
	s.mu.Unlock()
	s.callback.RequestRebuild()
}

// decodeScreenshot reads the PNG at path into a texture of its own. Files
// over maxScreenshotDim on a side or maxScreenshotPixels in all are
// refused before decoding.
func decodeScreenshot(path string) (*ebiten.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return nil, err
	}
	if cfg.Width > maxScreenshotDim || cfg.Height > maxScreenshotDim || cfg.Width*cfg.Height > maxScreenshotPixels {
		return nil, fmt.Errorf("screenshot is %dx%d, larger than allowed", cfg.Width, cfg.Height)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	return ebiten.NewImageFromImageWithOptions(img, &ebiten.NewImageFromImageOptions{Unmanaged: true}), nil
}

// centeredText creates secondary text centered in an anchor layout
func centeredText(text string) *widget.Text {
	return widget.NewText(
		widget.TextOpts.Text(text, style.FontFace(), style.TextSecondary),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionCenter,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
		})),
	)
}

// truncatedText creates text cut to width pixels, with the full text as a
// tooltip when it was cut
func truncatedText(text string, width int, textColor color.Color) *widget.Text {
	display, truncated := style.TruncateToWidth(text, *style.FontFace(), float64(width))
	opts := []widget.TextOpt{
		widget.TextOpts.Text(display, style.FontFace(), textColor),
	}
	if truncated {
		opts = append(opts, widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.ToolTip(widget.NewToolTip(
				widget.ToolTipOpts.Content(widgets.TooltipContent(text)),
			)),
		))
	}
	return widget.NewText(opts...)
}
