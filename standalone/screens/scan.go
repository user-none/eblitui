//go:build !libretro

package screens

import (
	"fmt"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/standalone/style"
)

// ScanProgress represents progress updates from the scanner
// This mirrors the ui.ScanProgress type
type ScanProgress struct {
	Phase           int
	Progress        float64
	GamesFound      int
	ArtworkTotal    int
	ArtworkComplete int
	StatusText      string
}

// Scanner interface for decoupling
type Scanner interface {
	Cancel()
}

// ScanProgressScreen displays ROM scanning progress
type ScanProgressScreen struct {
	callback        ScreenCallback
	scanner         Scanner
	phase           int
	progress        float64
	gamesFound      int
	artworkTotal    int
	artworkComplete int
	statusText      string
	cancelPending   bool
	cancelled       bool
}

// NewScanProgressScreen creates a new scan progress screen
func NewScanProgressScreen(callback ScreenCallback) *ScanProgressScreen {
	return &ScanProgressScreen{
		callback:   callback,
		statusText: "Initializing...",
	}
}

// SetScanner sets the active scanner
func (s *ScanProgressScreen) SetScanner(scanner Scanner) {
	s.scanner = scanner
}

// UpdateProgress updates the screen with new progress information
func (s *ScanProgressScreen) UpdateProgress(p ScanProgress) {
	s.phase = p.Phase
	s.progress = p.Progress
	s.gamesFound = p.GamesFound
	s.artworkTotal = p.ArtworkTotal
	s.artworkComplete = p.ArtworkComplete
	s.statusText = p.StatusText
}

// IsCancelled returns true if cancel was requested
func (s *ScanProgressScreen) IsCancelled() bool {
	return s.cancelled
}

// Build creates the scan progress screen UI
func (s *ScanProgressScreen) Build() *widget.Container {
	rootContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Background)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)

	centerContent := style.CenteredContainer(style.DefaultSpacing)

	// Status text
	statusText := s.statusText
	if statusText == "" {
		statusText = "Scanning..."
	}
	statusLabel := widget.NewText(
		widget.TextOpts.Text(statusText, style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	centerContent.AddChild(statusLabel)

	// Progress bar background
	progressBg := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Border)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(style.ProgressBarWidth, style.ProgressBarHeight),
		),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
		)),
	)

	// Progress bar fill (width based on progress)
	fillWidth := int(float64(style.ProgressBarWidth) * s.progress)
	if fillWidth < 1 {
		fillWidth = 1
	}
	progressFill := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Primary)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(fillWidth, style.ProgressBarHeight),
		),
	)
	progressBg.AddChild(progressFill)

	centerContent.AddChild(progressBg)

	// Percentage text
	percentText := fmt.Sprintf("%.0f%%", s.progress*100)
	percentLabel := widget.NewText(
		widget.TextOpts.Text(percentText, style.FontFace(), style.TextSecondary),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	centerContent.AddChild(percentLabel)

	// Found count
	foundText := fmt.Sprintf("Found: %d games", s.gamesFound)
	foundLabel := widget.NewText(
		widget.TextOpts.Text(foundText, style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	centerContent.AddChild(foundLabel)

	// Download status (during download phase - phase 2)
	if s.phase == 2 && s.artworkTotal > 0 {
		artworkText := fmt.Sprintf("Checking: %d/%d", s.artworkComplete, s.artworkTotal)
		artworkLabel := widget.NewText(
			widget.TextOpts.Text(artworkText, style.FontFace(), style.Text),
			widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		)
		centerContent.AddChild(artworkLabel)
	}

	// Cancel button
	cancelBtnImage := style.ButtonImage()
	if s.cancelPending {
		cancelBtnImage = style.DisabledButtonImage()
	}

	cancelButton := widget.NewButton(
		widget.ButtonOpts.Image(cancelBtnImage),
		widget.ButtonOpts.Text("Cancel", style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingMedium)),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			if !s.cancelPending {
				s.cancelPending = true
				s.cancelled = true
				if s.scanner != nil {
					s.scanner.Cancel()
				}
			}
		}),
	)
	centerContent.AddChild(cancelButton)

	rootContainer.AddChild(centerContent)

	return rootContainer
}

// OnEnter is called when entering the scan progress screen
func (s *ScanProgressScreen) OnEnter() {
	s.cancelPending = false
	s.cancelled = false
	s.progress = 0
	s.gamesFound = 0
	s.artworkTotal = 0
	s.artworkComplete = 0
	s.statusText = "Initializing..."
}

// OnExit is called when leaving the scan progress screen
func (s *ScanProgressScreen) OnExit() {
	s.scanner = nil
}
