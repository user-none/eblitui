package settings

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/sqweek/dialog"
	"github.com/user-none/eblitui/coreif"
	"github.com/user-none/eblitui/romloader"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
	"github.com/user-none/eblitui/standalone/types"
)

// CoreSection manages core-category option settings
type CoreSection struct {
	callback   types.ScreenCallback
	config     *storage.Config
	systemInfo coreif.SystemInfo
}

// NewCoreSection creates a new core options section
func NewCoreSection(callback types.ScreenCallback, config *storage.Config, systemInfo coreif.SystemInfo) *CoreSection {
	return &CoreSection{
		callback:   callback,
		config:     config,
		systemInfo: systemInfo,
	}
}

// SetConfig updates the config reference
func (c *CoreSection) SetConfig(config *storage.Config) {
	c.config = config
}

// HasCoreOpts returns true if any core-category options exist
func (c *CoreSection) HasCoreOpts() bool {
	for _, opt := range c.systemInfo.CoreOptions {
		if opt.Category == coreif.CoreOptionCategoryCore {
			return true
		}
	}
	return false
}

// HasBIOS returns true if any BIOS options exist
func (c *CoreSection) HasBIOS() bool {
	return len(c.systemInfo.BIOSOptions) > 0
}

// Build creates the core options section UI
func (c *CoreSection) Build(focus types.FocusManager) *widget.Container {
	outer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
		)),
	)

	section := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(style.SmallSpacing),
		)),
	)

	for _, opt := range c.systemInfo.CoreOptions {
		if opt.Category == coreif.CoreOptionCategoryCore {
			section.AddChild(buildCoreOptionRow(focus, c.callback, c.config, opt, "core"))
		}
	}

	// BIOS section
	if len(c.systemInfo.BIOSOptions) > 0 {
		for _, opt := range c.systemInfo.BIOSOptions {
			c.buildBIOSOption(section, focus, opt)
		}
	}

	c.setupNavigation(focus)

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

// buildBIOSOption builds the UI for a single BIOSOption:
// - Section header (opt.Label)
// - Active toggle (None / variant labels with files)
// - One row per variant: label + status, Browse or Remove button
func (c *CoreSection) buildBIOSOption(section *widget.Container, focus types.FocusManager, opt coreif.BIOSOption) {
	bc := c.getBIOSConfig(opt.Key)

	// Section header using the BIOSOption label
	section.AddChild(buildSectionHeader(opt.Label))

	// Active toggle row
	activeKey := "core-bios-active-" + opt.Key
	activeLabel := "None"
	if bc.Active != "" {
		activeLabel = bc.Active
	}

	// Build cycle options: None + each variant that has a file
	cycleOptions := []string{"None"}
	for _, v := range opt.Variants {
		if bc.Files != nil {
			if _, ok := bc.Files[v.Label]; ok {
				cycleOptions = append(cycleOptions, v.Label)
			}
		}
	}

	// Find next cycle value
	currentIdx := 0
	for i, val := range cycleOptions {
		if val == activeLabel {
			currentIdx = i
			break
		}
	}
	nextIdx := (currentIdx + 1) % len(cycleOptions)

	activeRow := style.SettingsRow(2)
	activeRow.AddChild(widget.NewText(
		widget.TextOpts.Text("Active", style.FontFace(), style.Text),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	))

	activeBtn := widget.NewButton(
		widget.ButtonOpts.Image(style.ButtonImage()),
		widget.ButtonOpts.Text(activeLabel, style.FontFace(), style.ButtonTextColor()),
		widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			}),
			widget.WidgetOpts.MinSize(style.Px(60), 0),
		),
		widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
			cfg := c.getBIOSConfig(opt.Key)
			nextVal := cycleOptions[nextIdx]
			if nextVal == "None" {
				cfg.Active = ""
			} else {
				cfg.Active = nextVal
			}
			c.setBIOSConfig(opt.Key, cfg)
			focus.SetPendingFocus(activeKey)
			c.callback.RequestRebuild()
		}),
	)
	focus.RegisterFocusButton(activeKey, activeBtn)
	activeRow.AddChild(activeBtn)
	section.AddChild(activeRow)

	// One row per variant
	for _, v := range opt.Variants {
		c.buildBIOSVariantRow(section, focus, opt, v, bc)
	}
}

// buildBIOSVariantRow builds a single variant row with shader-style layout:
// left: label (primary) + filename or "(Not Found)" (secondary)
// right: Browse or Remove button
func (c *CoreSection) buildBIOSVariantRow(section *widget.Container, focus types.FocusManager, opt coreif.BIOSOption, v coreif.BIOSVariant, bc storage.BIOSConfig) {
	filePath := ""
	if bc.Files != nil {
		filePath = bc.Files[v.Label]
	}
	hasFile := filePath != ""

	row := style.SettingsRow(2)

	// Left: info column (label + status)
	statusText := "(Not Found)"
	if hasFile {
		statusText = filepath.Base(filePath)
	}
	row.AddChild(style.LabeledText(v.Label, statusText))

	// Right: Browse or Remove button
	if hasFile {
		removeKey := "core-bios-remove-" + opt.Key + "-" + v.Label
		removeBtn := widget.NewButton(
			widget.ButtonOpts.Image(style.ButtonImage()),
			widget.ButtonOpts.Text("Remove", style.FontFace(), style.ButtonTextColor()),
			widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.GridLayoutData{
					VerticalPosition: widget.GridLayoutPositionCenter,
				}),
				widget.WidgetOpts.MinSize(style.Px(60), 0),
			),
			widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
				cfg := c.getBIOSConfig(opt.Key)
				if cfg.Files != nil {
					delete(cfg.Files, v.Label)
				}
				// Clear active if it pointed to this variant
				if cfg.Active == v.Label {
					cfg.Active = ""
				}
				c.setBIOSConfig(opt.Key, cfg)
				focus.SetPendingFocus("core-bios-browse-" + opt.Key + "-" + v.Label)
				c.callback.RequestRebuild()
			}),
		)
		focus.RegisterFocusButton(removeKey, removeBtn)
		row.AddChild(removeBtn)
	} else {
		browseKey := "core-bios-browse-" + opt.Key + "-" + v.Label
		browseBtn := widget.NewButton(
			widget.ButtonOpts.Image(style.ButtonImage()),
			widget.ButtonOpts.Text("Browse", style.FontFace(), style.ButtonTextColor()),
			widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(style.ButtonPaddingSmall)),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.GridLayoutData{
					VerticalPosition: widget.GridLayoutPositionCenter,
				}),
				widget.WidgetOpts.MinSize(style.Px(60), 0),
			),
			widget.ButtonOpts.ClickedHandler(func(args *widget.ButtonClickedEventArgs) {
				c.onBrowseBIOS(opt, v, browseKey, focus)
			}),
		)
		focus.RegisterFocusButton(browseKey, browseBtn)
		row.AddChild(browseBtn)
	}

	section.AddChild(row)
}

// onBrowseBIOS opens a file dialog for a specific BIOS variant.
func (c *CoreSection) onBrowseBIOS(opt coreif.BIOSOption, v coreif.BIOSVariant, focusKey string, focus types.FocusManager) {
	go func() {
		path, err := dialog.File().
			Title("Select BIOS: " + v.Label).
			Load()
		if err != nil {
			return
		}

		data, err := romloader.LoadBIOS(path)
		if err != nil {
			return
		}

		// Validate hash if the variant has one
		if v.SHA256 != "" {
			hash := computeSHA256(data)
			if hash != v.SHA256 {
				c.callback.ShowNotification("File does not match expected BIOS hash")
				return
			}
		}

		cfg := c.getBIOSConfig(opt.Key)
		if cfg.Files == nil {
			cfg.Files = make(map[string]string)
		}
		cfg.Files[v.Label] = path
		c.setBIOSConfig(opt.Key, cfg)
		focus.SetPendingFocus(focusKey)
		c.callback.RequestRebuild()
	}()
}

// getBIOSConfig returns the BIOS config for a given key, with safe defaults.
func (c *CoreSection) getBIOSConfig(key string) storage.BIOSConfig {
	if c.config.BIOS != nil {
		if bc, ok := c.config.BIOS[key]; ok {
			return bc
		}
	}
	return storage.BIOSConfig{}
}

// setBIOSConfig saves the BIOS config for a given key.
func (c *CoreSection) setBIOSConfig(key string, bc storage.BIOSConfig) {
	if c.config.BIOS == nil {
		c.config.BIOS = make(map[string]storage.BIOSConfig)
	}
	c.config.BIOS[key] = bc
	storage.SaveConfig(c.config)
}

// setupNavigation registers navigation zones for the core options section
func (c *CoreSection) setupNavigation(focus types.FocusManager) {
	coreOptKeys := make([]string, 0)
	for _, opt := range c.systemInfo.CoreOptions {
		if opt.Category == coreif.CoreOptionCategoryCore {
			coreOptKeys = append(coreOptKeys, "core-opt-"+opt.Key)
		}
	}
	if len(coreOptKeys) > 0 {
		focus.RegisterNavZone("core-core-opts", types.NavZoneVertical, coreOptKeys, 0)
	}

	// BIOS navigation zone
	biosKeys := make([]string, 0)
	for _, opt := range c.systemInfo.BIOSOptions {
		bc := c.getBIOSConfig(opt.Key)
		biosKeys = append(biosKeys, "core-bios-active-"+opt.Key)
		for _, v := range opt.Variants {
			hasFile := bc.Files != nil && bc.Files[v.Label] != ""
			if hasFile {
				biosKeys = append(biosKeys, "core-bios-remove-"+opt.Key+"-"+v.Label)
			} else {
				biosKeys = append(biosKeys, "core-bios-browse-"+opt.Key+"-"+v.Label)
			}
		}
	}
	if len(biosKeys) > 0 {
		focus.RegisterNavZone("core-bios", types.NavZoneVertical, biosKeys, 0)
	}

	// Transitions between zones
	if len(coreOptKeys) > 0 && len(biosKeys) > 0 {
		focus.SetNavTransition("core-core-opts", types.DirDown, "core-bios", types.NavIndexFirst)
		focus.SetNavTransition("core-bios", types.DirUp, "core-core-opts", types.NavIndexFirst)
	}
}

// buildSectionHeader creates a section header label with accent color.
func buildSectionHeader(title string) *widget.Container {
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

// computeSHA256 returns the hex-encoded SHA256 hash of data.
func computeSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
