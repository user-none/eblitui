package settings

import (
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
)

// buildKeyboardView creates the player 1 keyboard binding editor view.
func (s *InputSection) buildKeyboardView(focus types.FocusManager, section *widget.Container) {
	section.AddChild(sectionHeader("Keyboard Bindings (Player 1)"))
	section.AddChild(s.buildHeaderRow("Keyboard"))

	for _, dp := range dpadEntries {
		section.AddChild(s.buildKeyBindingRow(focus, dp.Name, dp.DefaultKey))
	}
	for _, btn := range s.systemInfo.Buttons {
		section.AddChild(s.buildKeyBindingRow(focus, btn.Name, btn.DefaultKey))
	}

	section.AddChild(s.buildActionRow(focus,
		actionButton{"Reset Keyboard", "input-reset-kb", func() {
			s.config.Input.P1Keyboard = nil
			storage.SaveConfig(s.config)
			s.focus.SetPendingFocus("input-reset-kb")
			s.callback.RequestRebuild()
		}},
		actionButton{"Done", "input-kb-done", func() {
			s.switchView(inputViewMain, "input-kb-edit")
		}},
	))

	bindingKeys := s.bindingFocusKeys("input-kb-")
	focus.RegisterNavZone("input-kb-bindings", types.NavZoneVertical, bindingKeys, 0)
	focus.RegisterNavZone("input-kb-actions", types.NavZoneHorizontal, []string{"input-reset-kb", "input-kb-done"}, 0)
	chainZones(focus, []string{"input-kb-bindings", "input-kb-actions"})
}

// updateKeyboardCapture reads the next valid key press and applies it to
// the captured button.
func (s *InputSection) updateKeyboardCapture() {
	keys := inpututil.AppendJustPressedKeys(nil)
	for _, k := range keys {
		if IsReservedFunc != nil && IsReservedFunc(k) {
			continue
		}
		if KeyToNameFunc == nil {
			continue
		}
		name, ok := KeyToNameFunc(k)
		if !ok {
			continue
		}
		s.applyKeyboardBinding(s.captureBtn, name)
		return
	}
}

// applyKeyboardBinding saves a keyboard binding and exits capture mode
func (s *InputSection) applyKeyboardBinding(buttonName, keyName string) {
	if s.config.Input.P1Keyboard == nil {
		s.config.Input.P1Keyboard = make(map[string]string)
	}

	// Check if this is the default - if so, remove the override
	defaultKey := s.defaultKeyForButton(buttonName)
	if keyName == defaultKey {
		delete(s.config.Input.P1Keyboard, buttonName)
		if len(s.config.Input.P1Keyboard) == 0 {
			s.config.Input.P1Keyboard = nil
		}
	} else {
		s.config.Input.P1Keyboard[buttonName] = keyName
	}

	storage.SaveConfig(s.config)
	s.capturing = false
	s.focus.SetPendingFocus("input-kb-" + buttonName)
	s.callback.RequestRebuild()
}

// buildKeyBindingRow creates a row for a single keyboard binding
func (s *InputSection) buildKeyBindingRow(focus types.FocusManager, buttonName, defaultKey string) *widget.Container {
	row := widgets.SettingsRow(2)

	row.AddChild(widget.NewText(
		widget.TextOpts.Text(buttonName, style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	))

	display := s.resolveKeyDisplay(buttonName, defaultKey)
	focusKey := "input-kb-" + buttonName

	if s.capturing && s.captureBtn == buttonName {
		display = "Press a key..."
	}

	btn := widget.NewButton(
		widget.ButtonOpts.Image(s.bindingButtonImage(buttonName, defaultKey, s.config.Input.P1Keyboard)),
		widget.ButtonOpts.Text(display, style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(90), 0),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			s.capturing = true
			s.captureBtn = buttonName
			focus.SetPendingFocus(focusKey)
			s.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton(focusKey, btn)
	row.AddChild(btn)

	return row
}
