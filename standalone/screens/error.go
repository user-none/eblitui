//go:build !libretro

package screens

import (
	"fmt"

	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/standalone/style"
)

// ErrorMode distinguishes between types of config errors
type ErrorMode int

const (
	// ErrorModeCorrupted indicates the JSON file could not be parsed
	ErrorModeCorrupted ErrorMode = iota
	// ErrorModeInvalid indicates the JSON parsed but contains invalid values
	ErrorModeInvalid
)

// ErrorScreen displays startup errors for corrupted config/library files
type ErrorScreen struct {
	callback ScreenCallback
	filename string // "config.json" or "library.json"
	filepath string // Full path to the file
	onDelete func() // Callback for delete and continue (corrupted mode)
	mode     ErrorMode
	details  []string // Validation error details (invalid mode)
	onReset  func()   // Callback for reset and continue (invalid mode)
}

// NewErrorScreen creates a new error screen
func NewErrorScreen(callback ScreenCallback, filename, filepath string, onDelete func()) *ErrorScreen {
	return &ErrorScreen{
		callback: callback,
		filename: filename,
		filepath: filepath,
		onDelete: onDelete,
		mode:     ErrorModeCorrupted,
	}
}

// SetError updates the error details (used when transitioning between errors)
func (s *ErrorScreen) SetError(filename, filepath string) {
	s.filename = filename
	s.filepath = filepath
	s.mode = ErrorModeCorrupted
	s.details = nil
}

// SetValidationError configures the screen for validation error display
func (s *ErrorScreen) SetValidationError(filename, filepath string, details []string, onReset func()) {
	s.filename = filename
	s.filepath = filepath
	s.mode = ErrorModeInvalid
	s.details = details
	s.onReset = onReset
}

// Build creates the error screen UI
func (s *ErrorScreen) Build() *widget.Container {
	rootContainer := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Background)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)

	centerContent := style.CenteredContainer(style.DefaultSpacing)

	switch s.mode {
	case ErrorModeInvalid:
		s.buildInvalidMode(centerContent)
	default:
		s.buildCorruptedMode(centerContent)
	}

	rootContainer.AddChild(centerContent)
	return rootContainer
}

// buildCorruptedMode builds the UI for corrupted JSON files
func (s *ErrorScreen) buildCorruptedMode(container *widget.Container) {
	// Title
	titleLabel := widget.NewText(
		widget.TextOpts.Text("Configuration Error", style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	container.AddChild(titleLabel)

	// Message
	msgText := fmt.Sprintf("The file \"%s\" is invalid or corrupted.", s.filename)
	msgLabel := widget.NewText(
		widget.TextOpts.Text(msgText, style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	container.AddChild(msgLabel)

	// Help text
	helpLabel := widget.NewText(
		widget.TextOpts.Text("You can delete the file and start fresh, or exit to manually fix the file.", style.FontFace(), style.TextSecondary),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	container.AddChild(helpLabel)

	// Buttons container
	buttonsContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(style.DefaultSpacing),
		)),
	)

	// Delete and Continue button
	deleteButton := style.TextButton("Delete and Continue", style.ButtonPaddingMedium, func(args *widget.ButtonClickedEventArgs) {
		if s.onDelete != nil {
			s.onDelete()
		}
	})
	buttonsContainer.AddChild(deleteButton)

	// Exit button
	exitButton := style.TextButton("Exit", style.ButtonPaddingMedium, func(args *widget.ButtonClickedEventArgs) {
		s.callback.Exit()
	})
	buttonsContainer.AddChild(exitButton)

	container.AddChild(buttonsContainer)
}

// buildInvalidMode builds the UI for validation errors
func (s *ErrorScreen) buildInvalidMode(container *widget.Container) {
	// Title
	titleLabel := widget.NewText(
		widget.TextOpts.Text("Invalid Settings", style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	container.AddChild(titleLabel)

	// Message
	msgText := fmt.Sprintf("The file \"%s\" contains invalid settings:", s.filename)
	msgLabel := widget.NewText(
		widget.TextOpts.Text(msgText, style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	container.AddChild(msgLabel)

	// List validation errors (cap at 5 to avoid pushing buttons off screen)
	maxDisplay := 5
	for i, detail := range s.details {
		if i >= maxDisplay {
			moreLabel := widget.NewText(
				widget.TextOpts.Text(fmt.Sprintf("+%d more", len(s.details)-maxDisplay), style.FontFace(), style.TextSecondary),
				widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
			)
			container.AddChild(moreLabel)
			break
		}
		detailLabel := widget.NewText(
			widget.TextOpts.Text(detail, style.FontFace(), style.TextSecondary),
			widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		)
		container.AddChild(detailLabel)
	}

	// Help text
	helpLabel := widget.NewText(
		widget.TextOpts.Text("You can reset invalid settings to defaults, or exit to manually fix the file.", style.FontFace(), style.TextSecondary),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
	)
	container.AddChild(helpLabel)

	// Buttons container
	buttonsContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(style.DefaultSpacing),
		)),
	)

	// Reset and Continue button
	resetButton := style.TextButton("Reset and Continue", style.ButtonPaddingMedium, func(args *widget.ButtonClickedEventArgs) {
		if s.onReset != nil {
			s.onReset()
		}
	})
	buttonsContainer.AddChild(resetButton)

	// Exit button
	exitButton := style.TextButton("Exit", style.ButtonPaddingMedium, func(args *widget.ButtonClickedEventArgs) {
		s.callback.Exit()
	})
	buttonsContainer.AddChild(exitButton)

	container.AddChild(buttonsContainer)
}

// OnEnter is called when entering the error screen
func (s *ErrorScreen) OnEnter() {
	// Nothing to do
}
