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

// ScanProgressScreen displays library scan and clean progress. Progress
// updates change the built widgets in place rather than rebuilding the
// screen, so a press on Cancel is never lost to a rebuild between the
// button's press and release.
type ScanProgressScreen struct {
	scanner          Scanner
	phase            int
	progress         float64
	countText        string
	downloadTotal    int
	downloadComplete int
	statusText       string
	cancelPending    bool

	// Widgets of the last Build, updated in place by refresh
	root          *widget.Container
	statusLabel   *widget.Text
	progressFill  *widget.Container
	percentLabel  *widget.Text
	countLabel    *widget.Text
	downloadLabel *widget.Text
	cancelButton  *widget.Button
	cancelingText *widget.Text
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

// UpdateProgress updates the screen with new progress information, in
// place when the screen is built
func (s *ScanProgressScreen) UpdateProgress(p ScanProgress) {
	s.phase = p.Phase
	s.progress = p.Progress
	s.countText = p.CountText
	s.downloadTotal = p.DownloadTotal
	s.downloadComplete = p.DownloadComplete
	s.statusText = p.StatusText
	s.refresh()
}

// Cancel stops the running scan or clean once: the Cancel button is
// replaced by "Canceling..." right away, and the scan or clean ends at its
// next check. Further calls do nothing.
func (s *ScanProgressScreen) Cancel() {
	if s.cancelPending {
		return
	}
	s.cancelPending = true
	if s.scanner != nil {
		s.scanner.Cancel()
	}
	s.refresh()
}

// Build creates the scan progress screen UI
func (s *ScanProgressScreen) Build() *widget.Container {
	rootContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Background)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)

	centerContent := widgets.CenteredContainer(style.DefaultSpacing)

	// Status text
	statusLabel := widget.NewText(
		widget.TextOpts.Text("", style.FontFace(), style.Text),
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

	// Progress bar fill (width set by refresh)
	progressFill := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Primary)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(1, style.ProgressBarHeight),
		),
	)
	progressBg.AddChild(progressFill)

	centerContent.AddChild(progressBg)

	// Percentage text
	percentLabel := widget.NewText(
		widget.TextOpts.Text("", style.FontFace(), style.TextSecondary),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	centerContent.AddChild(percentLabel)

	// Game count
	countLabel := widget.NewText(
		widget.TextOpts.Text("", style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	centerContent.AddChild(countLabel)

	// Download status (shown during the artwork and rumble download phases)
	downloadLabel := widget.NewText(
		widget.TextOpts.Text("", style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	centerContent.AddChild(downloadLabel)

	// Cancel button, replaced by "Canceling..." once pressed
	cancelButton := widgets.TextButton("Cancel", style.ButtonPaddingMedium, func(args *widget.ButtonClickedEventArgs) {
		s.Cancel()
	})
	centerContent.AddChild(cancelButton)
	cancelingText := widget.NewText(
		widget.TextOpts.Text("Canceling...", style.FontFace(), style.TextSecondary),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	centerContent.AddChild(cancelingText)

	rootContainer.AddChild(centerContent)

	s.root = rootContainer
	s.statusLabel = statusLabel
	s.progressFill = progressFill
	s.percentLabel = percentLabel
	s.countLabel = countLabel
	s.downloadLabel = downloadLabel
	s.cancelButton = cancelButton
	s.cancelingText = cancelingText
	s.refresh()

	return rootContainer
}

// refresh shows the current progress and cancel state in the widgets of
// the last Build. Before the first Build it does nothing.
func (s *ScanProgressScreen) refresh() {
	if s.root == nil {
		return
	}
	statusText := s.statusText
	if statusText == "" {
		statusText = "Scanning..."
	}
	s.statusLabel.Label = statusText

	s.progressFill.GetWidget().MinWidth = max(1, int(float64(style.ProgressBarWidth)*s.progress))
	s.percentLabel.Label = fmt.Sprintf("%.0f%%", s.progress*100)
	s.countLabel.Label = s.countText

	if s.phase >= 2 && s.downloadTotal > 0 {
		s.downloadLabel.Label = fmt.Sprintf("Checking: %d/%d", s.downloadComplete, s.downloadTotal)
		s.downloadLabel.GetWidget().SetVisibility(widget.Visibility_Show)
	} else {
		s.downloadLabel.GetWidget().SetVisibility(widget.Visibility_Hide)
	}

	if s.cancelPending {
		s.cancelButton.GetWidget().SetVisibility(widget.Visibility_Hide)
		s.cancelingText.GetWidget().SetVisibility(widget.Visibility_Show)
	} else {
		s.cancelButton.GetWidget().SetVisibility(widget.Visibility_Show)
		s.cancelingText.GetWidget().SetVisibility(widget.Visibility_Hide)
	}

	s.root.RequestRelayout()
}

// OnEnter is called when entering the scan progress screen
func (s *ScanProgressScreen) OnEnter() {
	s.cancelPending = false
	s.progress = 0
	s.countText = ""
	s.downloadTotal = 0
	s.downloadComplete = 0
	s.statusText = "Initializing..."
	s.refresh()
}
