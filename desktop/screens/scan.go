// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"fmt"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/widgets"
)

// ScanProgress represents progress updates from a library scan or clean
type ScanProgress struct {
	Phase            int
	Progress         float64
	CountText        string // Games counted so far, such as "Found: 3 games"
	DownloadTotal    int
	DownloadComplete int
	StatusText       string
}

// Scanner is the running scan or clean the Cancel button stops
type Scanner interface {
	Cancel()
}

// ScanProgressScreen displays library scan and clean progress
type ScanProgressScreen struct {
	scanner          Scanner
	phase            int
	progress         float64
	countText        string
	downloadTotal    int
	downloadComplete int
	statusText       string
	cancelPending    bool
}

// NewScanProgressScreen creates a new scan progress screen
func NewScanProgressScreen() *ScanProgressScreen {
	return &ScanProgressScreen{
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
	s.countText = p.CountText
	s.downloadTotal = p.DownloadTotal
	s.downloadComplete = p.DownloadComplete
	s.statusText = p.StatusText
}

// Build creates the scan progress screen UI
func (s *ScanProgressScreen) Build() *widget.Container {
	rootContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Background)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)

	centerContent := widgets.CenteredContainer(style.DefaultSpacing)

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

	// Game count
	countLabel := widget.NewText(
		widget.TextOpts.Text(s.countText, style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	centerContent.AddChild(countLabel)

	// Download status (during the artwork and rumble download phases)
	if s.phase >= 2 && s.downloadTotal > 0 {
		downloadText := fmt.Sprintf("Checking: %d/%d", s.downloadComplete, s.downloadTotal)
		downloadLabel := widget.NewText(
			widget.TextOpts.Text(downloadText, style.FontFace(), style.Text),
			widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		)
		centerContent.AddChild(downloadLabel)
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
	s.progress = 0
	s.countText = ""
	s.downloadTotal = 0
	s.downloadComplete = 0
	s.statusText = "Initializing..."
}
