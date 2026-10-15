// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/desktop/achievements"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/style"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/widgets"
	rcheevos "github.com/user-none/go-rcheevos"
)

// RetroAchievementsSection manages RetroAchievements settings
type RetroAchievementsSection struct {
	callback     types.ScreenCallback
	config       *storage.Config
	achievements *achievements.Manager
	contentWidth func() int

	// Input handling
	textInputs    *widgets.TextInputGroup
	usernameInput *widget.TextInput
	passwordInput *widget.TextInput
	errorMessage  string
	loggingIn     bool
}

// NewRetroAchievementsSection creates a new RetroAchievements section.
// contentWidth returns the width of the section content.
func NewRetroAchievementsSection(
	callback types.ScreenCallback,
	config *storage.Config,
	achievementMgr *achievements.Manager,
	contentWidth func() int,
) *RetroAchievementsSection {
	return &RetroAchievementsSection{
		callback:     callback,
		config:       config,
		achievements: achievementMgr,
		contentWidth: contentWidth,
		textInputs:   widgets.NewTextInputGroup(),
	}
}

// SetConfig updates the config reference
func (r *RetroAchievementsSection) SetConfig(config *storage.Config) {
	r.config = config
}

// SetAchievements updates the achievement manager reference
func (r *RetroAchievementsSection) SetAchievements(mgr *achievements.Manager) {
	r.achievements = mgr
}

// Update handles keyboard shortcuts for text inputs (Ctrl+A, Ctrl+V, Ctrl+C)
func (r *RetroAchievementsSection) Update() {
	r.textInputs.Update()
}

// hasStoredCredentials returns true if the user has stored login credentials
func (r *RetroAchievementsSection) hasStoredCredentials() bool {
	return r.config.RetroAchievements.Username != "" && r.config.RetroAchievements.Token != ""
}

// isLoggedIn returns true if the user is logged in (live session or stored credentials)
func (r *RetroAchievementsSection) isLoggedIn() bool {
	return (r.achievements != nil && r.achievements.IsLoggedIn()) || r.hasStoredCredentials()
}

// Build creates the RetroAchievements section UI
func (r *RetroAchievementsSection) Build(focus types.FocusManager) *widget.Container {
	// Outer container so scrollable content can stretch
	outer := widgets.FillContainer()

	// Content container
	section := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
	)

	// Account section
	section.AddChild(widgets.SectionHeader("Account"))
	if r.isLoggedIn() {
		section.AddChild(r.buildLoggedInSection(focus))

		// General section
		section.AddChild(widgets.SectionHeader("General"))
		section.AddChild(r.buildToggleRow(focus, "ra-enable", "Enable RetroAchievements", "",
			r.config.RetroAchievements.Enabled,
			func() {
				r.config.RetroAchievements.Enabled = !r.config.RetroAchievements.Enabled

				if r.config.RetroAchievements.Enabled {
					// Toggling ON: auto-login if we have stored credentials
					if r.achievements != nil && !r.achievements.IsLoggedIn() && r.hasStoredCredentials() {
						username := r.config.RetroAchievements.Username
						token := r.config.RetroAchievements.Token
						r.achievements.LoginWithToken(username, token, func(success bool, result int, err error) {
							if !success {
								// Only clear token for credential errors, not transient failures
								if result == rcheevos.InvalidCredentials ||
									result == rcheevos.ExpiredToken ||
									result == rcheevos.AccessDenied {
									r.config.RetroAchievements.Token = ""
									storage.SaveConfig(r.config)
								}
							}
						})
					}
				} else {
					// Toggling OFF: logout but preserve stored credentials
					if r.achievements != nil && r.achievements.IsLoggedIn() {
						r.achievements.Logout()
					}
				}
			}))

		// Notifications section
		section.AddChild(widgets.SectionHeader("Notifications"))
		section.AddChild(r.buildToggleRow(focus, "ra-notification", "Show Notification", "Display popup on unlock",
			r.config.RetroAchievements.ShowNotification,
			func() {
				r.config.RetroAchievements.ShowNotification = !r.config.RetroAchievements.ShowNotification
			}))
		section.AddChild(r.buildToggleRow(focus, "ra-sound", "Unlock Sound", "Play chime on achievement",
			r.config.RetroAchievements.UnlockSound,
			func() {
				r.config.RetroAchievements.UnlockSound = !r.config.RetroAchievements.UnlockSound
			}))
		section.AddChild(r.buildToggleRow(focus, "ra-screenshot", "Auto Screenshot", "Capture screen on unlock",
			r.config.RetroAchievements.AutoScreenshot,
			func() {
				r.config.RetroAchievements.AutoScreenshot = !r.config.RetroAchievements.AutoScreenshot
			}))

		// Advanced section
		section.AddChild(widgets.SectionHeader("Advanced"))
		section.AddChild(r.buildToggleRow(focus, "ra-encore", "Encore Mode", "Re-trigger unlocked achievements",
			r.config.RetroAchievements.EncoreMode,
			func() {
				r.config.RetroAchievements.EncoreMode = !r.config.RetroAchievements.EncoreMode
			}))
		section.AddChild(r.buildToggleRow(focus, "ra-spectator", "Spectator Mode", "Watch achievements without submitting unlocks",
			r.config.RetroAchievements.SpectatorMode,
			func() {
				r.config.RetroAchievements.SpectatorMode = !r.config.RetroAchievements.SpectatorMode
			}))
	} else {
		section.AddChild(r.buildLoginSection(focus))
	}

	r.setupNavigation(focus)

	// Wrap in scrollable container
	scrollContainer, _, scrollWrapper := widgets.ScrollableContainer(widgets.PageScrollOpts(section))
	focus.SetScrollContainer(scrollContainer)
	focus.RestoreScrollPosition()
	outer.AddChild(scrollWrapper)
	return outer
}

// maxLabelWidth returns the width of the label column of a row with one
// control column controlWidth wide
func (r *RetroAchievementsSection) maxLabelWidth(controlWidth int) float64 {
	return float64(widgets.SettingsRowLabelWidth(widgets.PageRowWidth(r.contentWidth()), controlWidth))
}

// buildToggleRow creates a toggle row with background, label, description, and right-aligned button
func (r *RetroAchievementsSection) buildToggleRow(focus types.FocusManager, key, label, description string, value bool, toggle func()) *widget.Container {
	// Truncate text to prevent pushing buttons off-screen at large font sizes
	maxW := r.maxLabelWidth(widgets.OnOffButtonWidth())
	face := *style.FontFace()
	displayLabel, _ := style.TruncateToWidth(label, face, maxW)
	displayDesc := description
	if description != "" {
		displayDesc, _ = style.TruncateToWidth(description, face, maxW)
	}

	row := widgets.SettingsRow(2)

	// Info column (label + optional description)
	row.AddChild(widgets.LabeledText(displayLabel, displayDesc))

	// Toggle button (right-aligned via grid)
	toggleBtn := widgets.OnOffButton(value, func(args *widget.ButtonClickedEventArgs) {
		toggle()
		storage.SaveConfig(r.config)
		focus.SetPendingFocus(key)
		r.callback.RequestRebuild()
	})
	focus.RegisterFocusButton(key, toggleBtn)
	row.AddChild(toggleBtn)

	return row
}

// setupNavigation registers navigation zones for the section
func (r *RetroAchievementsSection) setupNavigation(focus types.FocusManager) {
	keys := []string{}

	if r.isLoggedIn() {
		keys = append(keys, "ra-logout", "ra-enable",
			"ra-notification", "ra-sound", "ra-screenshot", "ra-encore", "ra-spectator")
	} else {
		keys = append(keys, "ra-login")
	}

	focus.RegisterNavZone("ra-settings", types.NavZoneVertical, keys, 0)
}

// buildLoggedInSection creates the logged-in status section
func (r *RetroAchievementsSection) buildLoggedInSection(focus types.FocusManager) *widget.Container {
	row := widgets.SettingsRow(2)

	// Get username from manager if available, otherwise from config
	username := r.config.RetroAchievements.Username
	if r.achievements != nil && r.achievements.IsLoggedIn() {
		username = r.achievements.GetUsername()
	}

	// Status text (truncated to prevent pushing button off-screen)
	statusStr := "Logged in as: " + username
	displayStatus, _ := style.TruncateToWidth(statusStr, *style.FontFace(), r.maxLabelWidth(widgets.ButtonWidth("Logout")))
	statusText := widget.NewText(
		widget.TextOpts.Text(displayStatus, style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	)
	row.AddChild(statusText)

	// Logout button
	logoutBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ButtonImage()),
		widget.ButtonOpts.Text("Logout", style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			if r.achievements != nil {
				r.achievements.Logout()
			}
			// Clear stored credentials
			r.config.RetroAchievements.Username = ""
			r.config.RetroAchievements.Token = ""
			storage.SaveConfig(r.config)
			focus.SetPendingFocus("ra-enable")
			r.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton("ra-logout", logoutBtn)
	row.AddChild(logoutBtn)

	return row
}

// buildLoginSection creates the login form section
func (r *RetroAchievementsSection) buildLoginSection(focus types.FocusManager) *widget.Container {
	section := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Surface)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(style.SmallSpacing)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	// Username row using grid for alignment
	usernameRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	usernameLabel := widget.NewText(
		widget.TextOpts.Text("Username", style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(style.Px(80), 0),
		),
	)
	usernameRow.AddChild(usernameLabel)

	r.usernameInput = widgets.StyledTextInput("Enter username", false, style.Px(200))
	r.textInputs.Add(r.usernameInput)
	if r.config.RetroAchievements.Username != "" {
		r.usernameInput.SetText(r.config.RetroAchievements.Username)
	}
	usernameRow.AddChild(r.usernameInput)
	section.AddChild(usernameRow)

	// Password row using grid for alignment
	passwordRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	passwordLabel := widget.NewText(
		widget.TextOpts.Text("Password", style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(style.Px(80), 0),
		),
	)
	passwordRow.AddChild(passwordLabel)

	r.passwordInput = widgets.StyledTextInput("Enter password", true, style.Px(200))
	r.textInputs.Add(r.passwordInput)
	passwordRow.AddChild(r.passwordInput)
	section.AddChild(passwordRow)

	// Error message (if any)
	if r.errorMessage != "" {
		errorText := widget.NewText(
			widget.TextOpts.Text(r.errorMessage, style.FontFace(), style.Accent),
		)
		section.AddChild(errorText)
	}

	// Login button
	loginText := "Login"
	if r.loggingIn {
		loginText = "Logging in..."
	}

	loginBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ButtonImage()),
		widget.ButtonOpts.Text(loginText, style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			if r.loggingIn || r.achievements == nil {
				return
			}

			username := r.usernameInput.GetText()
			password := r.passwordInput.GetText()

			if username == "" || password == "" {
				r.errorMessage = "Username and password required"
				r.callback.RequestRebuild()
				return
			}

			r.loggingIn = true
			r.errorMessage = ""
			r.callback.RequestRebuild()

			r.achievements.Login(username, password, func(success bool, token string, err error) {
				r.loggingIn = false
				if success {
					r.config.RetroAchievements.Username = username
					r.config.RetroAchievements.Token = token
					r.config.RetroAchievements.Enabled = true
					storage.SaveConfig(r.config)
					r.errorMessage = ""
				} else {
					if err != nil {
						r.errorMessage = err.Error()
					} else {
						r.errorMessage = "Login failed"
					}
				}
				focus.SetPendingFocus("ra-enable")
				r.callback.RequestRebuild()
			})
		}),
	)
	focus.RegisterFocusButton("ra-login", loginBtn)
	section.AddChild(loginBtn)

	return section
}
