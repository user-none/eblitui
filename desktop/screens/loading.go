// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screens

import (
	"math"
	"strings"
	"time"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/widgets"
)

const (
	// loadingText is the status line, followed by up to loadingDots dots.
	loadingText = "Loading"
	loadingDots = 3
	// loadingDotInterval is how long each count of dots shows.
	loadingDotInterval = 400 * time.Millisecond
)

// LoadingScreen shows the title of a game being started over a status line
// whose dots cycle, so the screen is seen to be alive while there is no
// progress to report. It takes no input.
type LoadingScreen struct {
	title   string
	entered time.Time

	// Widgets of the last Build, updated in place by Update
	root        *widget.Container
	statusLabel *widget.Text
}

// NewLoadingScreen creates a new loading screen
func NewLoadingScreen() *LoadingScreen {
	return &LoadingScreen{}
}

// OnEnter is called when entering the loading screen for the game with the
// given title
func (s *LoadingScreen) OnEnter(title string) {
	s.title = title
	s.entered = time.Now()
}

// Build creates the loading screen UI
func (s *LoadingScreen) Build() *widget.Container {
	rootContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Background)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)

	centerContent := widgets.CenteredContainer(style.DefaultSpacing)

	// Game title, in the large face when there is one
	titleFace := style.FontFace()
	if large := style.LargeFontFace(); large != nil {
		titleFace = &large
	}
	centerContent.AddChild(widget.NewText(
		widget.TextOpts.Text(s.title, titleFace, style.Text),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
		),
	))

	// Status line. It is as wide as its longest text and starts at its
	// left edge, so the text does not shift as the dots change.
	width, _ := text.Measure(loadingText+strings.Repeat(".", loadingDots), *style.FontFace(), 0)
	statusLabel := widget.NewText(
		widget.TextOpts.Text("", style.FontFace(), style.TextSecondary),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(int(math.Ceil(width)), 0),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			}),
		),
	)
	centerContent.AddChild(statusLabel)

	rootContainer.AddChild(centerContent)

	s.root = rootContainer
	s.statusLabel = statusLabel
	s.Update()

	return rootContainer
}

// Update shows the status line for the current time. Before the first Build
// it does nothing.
func (s *LoadingScreen) Update() {
	if s.root == nil {
		return
	}
	dots := int(time.Since(s.entered)/loadingDotInterval) % (loadingDots + 1)
	label := loadingText + strings.Repeat(".", dots)
	if s.statusLabel.Label == label {
		return
	}
	s.statusLabel.Label = label
	s.root.RequestRelayout()
}
