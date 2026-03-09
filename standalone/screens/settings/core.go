package settings

import (
	"github.com/ebitenui/ebitenui/widget"
	emucore "github.com/user-none/eblitui/api"
	"github.com/user-none/eblitui/standalone/storage"
	"github.com/user-none/eblitui/standalone/style"
	"github.com/user-none/eblitui/standalone/types"
)

// CoreSection manages core-category option settings
type CoreSection struct {
	callback   types.ScreenCallback
	config     *storage.Config
	systemInfo emucore.SystemInfo
}

// NewCoreSection creates a new core options section
func NewCoreSection(callback types.ScreenCallback, config *storage.Config, systemInfo emucore.SystemInfo) *CoreSection {
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
		if opt.Category == emucore.CoreOptionCategoryCore {
			section.AddChild(buildCoreOptionRow(focus, c.callback, c.config, opt, "core"))
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

// setupNavigation registers navigation zones for the core options section
func (c *CoreSection) setupNavigation(focus types.FocusManager) {
	coreOptKeys := make([]string, 0)
	for _, opt := range c.systemInfo.CoreOptions {
		if opt.Category == emucore.CoreOptionCategoryCore {
			coreOptKeys = append(coreOptKeys, "core-opt-"+opt.Key)
		}
	}
	if len(coreOptKeys) > 0 {
		focus.RegisterNavZone("core-core-opts", types.NavZoneVertical, coreOptKeys, 0)
	}
}
