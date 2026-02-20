//go:build !libretro && !ios

package settings

import (
	"github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/user-none/eblitui/standalone/achievements"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
	"github.com/user-none/eblitui/standalone/types"
	rcheevos "github.com/user-none/go-rcheevos"
)

// Max width for the settings content to keep toggles closer to labels
const settingsMaxWidth = 600

// RetroAchievementsSection manages RetroAchievements settings
type RetroAchievementsSection struct {
	callback     types.ScreenCallback
	config       *storage.Config
	achievements *achievements.Manager

	// Input handling
	textInputs    *style.TextInputGroup
	usernameInput *widget.TextInput
	passwordInput *widget.TextInput
	errorMessage  string
	loggingIn     bool
}

// NewRetroAchievementsSection creates a new RetroAchievements section
func NewRetroAchievementsSection(
	callback types.ScreenCallback,
	config *storage.Config,
	achievementMgr *achievements.Manager,
) *RetroAchievementsSection {
	return &RetroAchievementsSection{
		callback:     callback,
		config:       config,
		achievements: achievementMgr,
		textInputs:   style.NewTextInputGroup(),
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
	// Outer container with grid layout so scrollable content can stretch
	outer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
		)),
	)

	// Content container
	section := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
	)

	// Account section
	section.AddChild(r.buildSectionHeader("Account"))
	if r.isLoggedIn() {
		section.AddChild(r.buildLoggedInSection(focus))
	} else {
		section.AddChild(r.buildLoginSection(focus))
	}

	// General section
	section.AddChild(r.buildSectionHeader("General"))
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

	// Options (only shown when enabled)
	if r.config.RetroAchievements.Enabled {
		// Notifications section
		section.AddChild(r.buildSectionHeader("Notifications"))
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
		section.AddChild(r.buildToggleRow(focus, "ra-suppress", "Suppress Hardcore Warning", "Hide 'Unknown Emulator' notice",
			r.config.RetroAchievements.SuppressHardcoreWarning,
			func() {
				r.config.RetroAchievements.SuppressHardcoreWarning = !r.config.RetroAchievements.SuppressHardcoreWarning
			}))

		// Advanced section
		section.AddChild(r.buildSectionHeader("Advanced"))
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
	}

	r.setupNavigation(focus)

	// Wrap in scrollable container
	scrollContainer, vSlider, scrollWrapper := style.ScrollableContainer(style.ScrollableOpts{
		Content:     section,
		BgColor:     style.Background,
		BorderColor: style.Border,
		Spacing:     0,
		Padding:     style.SmallSpacing,
	})
	focus.SetScrollWidgets(scrollContainer, vSlider)
	focus.RestoreScrollPosition()
	outer.AddChild(scrollWrapper)
	return outer
}

// maxLabelWidth calculates the maximum pixel width for text labels in toggle rows,
// based on the current window width and font-dependent sidebar size.
func (r *RetroAchievementsSection) maxLabelWidth() float64 {
	windowWidth := r.callback.GetWindowWidth()
	if windowWidth == 0 {
		windowWidth = 1100
	}

	// Estimate sidebar width: max of min size or measured widest label + padding
	sidebarWidth := style.SettingsSidebarMinWidth
	measuredSidebar := int(style.MeasureWidth("Achievements")) +
		style.SmallSpacing*2 + style.ButtonPaddingSmall*2
	if measuredSidebar > sidebarWidth {
		sidebarWidth = measuredSidebar
	}

	// Layout overhead: root padding + sidebar + main spacing + content area padding +
	// scroll wrapper padding + scrollbar + toggle row padding + grid spacing + button column
	overhead := style.DefaultPadding*2 + sidebarWidth + style.DefaultSpacing +
		style.DefaultPadding*2 + style.SmallSpacing*2 + style.ScrollbarWidth +
		style.SmallSpacing*2 + style.DefaultSpacing + 70

	available := windowWidth - overhead
	if available < 150 {
		available = 150
	}
	return float64(available)
}

// buildSectionHeader creates a section header label
func (r *RetroAchievementsSection) buildSectionHeader(title string) *widget.Container {
	container := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.TinySpacing),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	label := widget.NewText(
		widget.TextOpts.Text(title, style.FontFace(), style.Accent),
	)
	container.AddChild(label)

	return container
}

// buildToggleRow creates a toggle row with background, label, description, and right-aligned button
func (r *RetroAchievementsSection) buildToggleRow(focus types.FocusManager, key, label, description string, value bool, toggle func()) *widget.Container {
	// Truncate text to prevent pushing buttons off-screen at large font sizes
	maxW := r.maxLabelWidth()
	face := *style.FontFace()
	displayLabel, _ := style.TruncateToWidth(label, face, maxW)
	displayDesc := description
	if description != "" {
		displayDesc, _ = style.TruncateToWidth(description, face, maxW)
	}

	// Outer container with background color
	row := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Surface)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{true, false}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(style.SmallSpacing)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	// Info column (label + optional description)
	infoContainer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.TinySpacing),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		),
	)

	labelText := widget.NewText(
		widget.TextOpts.Text(displayLabel, style.FontFace(), style.Text),
	)
	infoContainer.AddChild(labelText)

	if displayDesc != "" {
		descText := widget.NewText(
			widget.TextOpts.Text(displayDesc, style.FontFace(), style.TextSecondary),
		)
		infoContainer.AddChild(descText)
	}

	row.AddChild(infoContainer)

	// Toggle button (right-aligned via grid)
	toggleBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ActiveButtonImage(value)),
		widget.ButtonOpts.Text(boolToOnOff(value), style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(50), 0),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			toggle()
			storage.SaveConfig(r.config)
			focus.SetPendingFocus(key)
			r.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton(key, toggleBtn)
	row.AddChild(toggleBtn)

	return row
}

// setupNavigation registers navigation zones for the section
func (r *RetroAchievementsSection) setupNavigation(focus types.FocusManager) {
	keys := []string{}

	if r.isLoggedIn() {
		keys = append(keys, "ra-logout")
	} else {
		keys = append(keys, "ra-login")
	}

	keys = append(keys, "ra-enable")

	if r.config.RetroAchievements.Enabled {
		keys = append(keys, "ra-sound", "ra-screenshot", "ra-suppress", "ra-encore")
	}

	focus.RegisterNavZone("ra-settings", types.NavZoneVertical, keys, 0)
}

// buildLoggedInSection creates the logged-in status section
func (r *RetroAchievementsSection) buildLoggedInSection(focus types.FocusManager) *widget.Container {
	// Row with background
	row := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(image.NewNineSliceColor(style.Surface)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{true, false}, []bool{true}),
			widget.GridLayoutOpts.Spacing(style.DefaultSpacing, 0),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(style.SmallSpacing)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)

	// Get username from manager if available, otherwise from config
	username := r.config.RetroAchievements.Username
	if r.achievements != nil && r.achievements.IsLoggedIn() {
		username = r.achievements.GetUsername()
	}

	// Status text (truncated to prevent pushing button off-screen)
	statusStr := "Logged in as: " + username
	displayStatus, _ := style.TruncateToWidth(statusStr, *style.FontFace(), r.maxLabelWidth())
	statusText := widget.NewText(
		widget.TextOpts.Text(displayStatus, style.FontFace(), style.Text),
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
		),
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
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(80), 0),
		),
	)
	usernameRow.AddChild(usernameLabel)

	r.usernameInput = style.StyledTextInput("Enter username", false, style.Px(200))
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
		widget.TextOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(80), 0),
		),
	)
	passwordRow.AddChild(passwordLabel)

	r.passwordInput = style.StyledTextInput("Enter password", true, style.Px(200))
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
