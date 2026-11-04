package storage

import (
	"encoding/json"
	"strings"
	"testing"
)

// validTestThemes is the list of theme names used in tests
var validTestThemes = []string{"Default", "Dark", "Light", "Retro", "Pink", "Hot Pink", "Green LCD", "High Contrast"}

// validTestRules are the rules used in tests: the test themes, the keys
// J, K, W and ArrowUp, and the pad buttons A, B and DpadUp.
var validTestRules = ConfigRules{
	Themes: validTestThemes,
	ValidKey: func(name string) bool {
		return name == "J" || name == "K" || name == "W" || name == "ArrowUp"
	},
	ValidPad: func(name string) bool {
		return name == "A" || name == "B" || name == "DpadUp"
	},
}

// newTestConfigData returns the default configuration values to change
func newTestConfigData() *ConfigData {
	d := defaultConfigData()
	return &d
}

func TestDetectPresentKeys(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected map[string]bool
	}{
		{
			name: "all keys present",
			json: `{
				"version": 1,
				"theme": "Default",
				"video": {"aspectRatio": "dar"},
				"audio": {"volume": 1.0, "fastForwardMute": true},
				"window": {"width": 900, "height": 650},
				"library": {"viewMode": "icon", "sortBy": "title", "artworkDownload": "covers"},
				"rewind": {"bufferSizeMB": 40, "frameStep": 1}
			}`,
			expected: map[string]bool{
				"theme":             true,
				"video.aspectRatio": true,
				"audio.volume":      true, "audio.fastForwardMute": true,
				"window.width": true, "window.height": true,
				"library.viewMode": true, "library.sortBy": true, "library.artworkDownload": true,
				"rewind.bufferSizeMB": true, "rewind.frameStep": true,
			},
		},
		{
			name:     "empty object",
			json:     `{}`,
			expected: map[string]bool{},
		},
		{
			name: "partial keys - missing video and rewind",
			json: `{
				"version": 1,
				"theme": "Dark",
				"audio": {"volume": 0.5},
				"window": {"width": 1024, "height": 768},
				"library": {"viewMode": "list", "sortBy": "lastPlayed"}
			}`,
			expected: map[string]bool{
				"theme":        true,
				"audio.volume": true, "window.width": true, "window.height": true,
				"library.viewMode": true, "library.sortBy": true,
			},
		},
		{
			name: "zero values are still present",
			json: `{
				"audio": {"volume": 0},
				"window": {"width": 0, "height": 0}
			}`,
			expected: map[string]bool{
				"audio.volume": true,
				"window.width": true, "window.height": true,
			},
		},
		{
			name:     "invalid JSON returns empty",
			json:     `{not valid json`,
			expected: map[string]bool{},
		},
		{
			name: "nested object present but empty",
			json: `{
				"audio": {},
				"window": {}
			}`,
			expected: map[string]bool{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectPresentKeys([]byte(tc.json))
			// Check expected keys are present
			for k := range tc.expected {
				if !got[k] {
					t.Errorf("expected key %q to be present", k)
				}
			}
			// Check no extra keys
			for k := range got {
				if !tc.expected[k] {
					t.Errorf("unexpected key %q detected", k)
				}
			}
		})
	}
}

func TestApplyMissingDefaults(t *testing.T) {
	t.Run("all missing gets all defaults", func(t *testing.T) {
		config := &ConfigData{}
		presentKeys := map[string]bool{}

		applyMissingDefaults(config, presentKeys)

		defaults := newTestConfigData()
		if config.Version != 0 {
			t.Errorf("version: got %d, want 0 (left for migration)", config.Version)
		}
		if config.Theme != defaults.Theme {
			t.Errorf("theme: got %q, want %q", config.Theme, defaults.Theme)
		}
		if config.Audio.Volume != defaults.Audio.Volume {
			t.Errorf("audio.volume: got %f, want %f", config.Audio.Volume, defaults.Audio.Volume)
		}
		if config.Window.Width != defaults.Window.Width {
			t.Errorf("window.width: got %d, want %d", config.Window.Width, defaults.Window.Width)
		}
		if config.Window.Height != defaults.Window.Height {
			t.Errorf("window.height: got %d, want %d", config.Window.Height, defaults.Window.Height)
		}
		if config.Library.ViewMode != defaults.Library.ViewMode {
			t.Errorf("library.viewMode: got %q, want %q", config.Library.ViewMode, defaults.Library.ViewMode)
		}
		if config.Library.SortBy != defaults.Library.SortBy {
			t.Errorf("library.sortBy: got %q, want %q", config.Library.SortBy, defaults.Library.SortBy)
		}
		if config.Library.ArtworkDownload != defaults.Library.ArtworkDownload {
			t.Errorf("library.artworkDownload: got %q, want %q", config.Library.ArtworkDownload, defaults.Library.ArtworkDownload)
		}
		if config.Rewind.BufferSizeMB != defaults.Rewind.BufferSizeMB {
			t.Errorf("rewind.bufferSizeMB: got %d, want %d", config.Rewind.BufferSizeMB, defaults.Rewind.BufferSizeMB)
		}
		if config.Rewind.FrameStep != defaults.Rewind.FrameStep {
			t.Errorf("rewind.frameStep: got %d, want %d", config.Rewind.FrameStep, defaults.Rewind.FrameStep)
		}
		if config.Audio.FastForwardMute != defaults.Audio.FastForwardMute {
			t.Errorf("audio.fastForwardMute: got %v, want %v", config.Audio.FastForwardMute, defaults.Audio.FastForwardMute)
		}
		if config.Video.AspectRatio != defaults.Video.AspectRatio {
			t.Errorf("video.aspectRatio: got %q, want %q", config.Video.AspectRatio, defaults.Video.AspectRatio)
		}
	})

	t.Run("present keys preserved even when zero", func(t *testing.T) {
		config := &ConfigData{
			Audio:  AudioConfig{Volume: 0.0},
			Window: WindowConfig{Width: 0, Height: 0},
		}
		presentKeys := map[string]bool{
			"audio.volume":  true,
			"window.width":  true,
			"window.height": true,
		}

		applyMissingDefaults(config, presentKeys)

		// These should NOT be overwritten since they're present
		if config.Audio.Volume != 0.0 {
			t.Errorf("audio.volume should remain 0.0, got %f", config.Audio.Volume)
		}
		if config.Window.Width != 0 {
			t.Errorf("window.width should remain 0, got %d", config.Window.Width)
		}
		if config.Window.Height != 0 {
			t.Errorf("window.height should remain 0, got %d", config.Window.Height)
		}

		// Missing fields should get defaults
		defaults := newTestConfigData()
		if config.Theme != defaults.Theme {
			t.Errorf("theme should default to %q, got %q", defaults.Theme, config.Theme)
		}
	})

	t.Run("all present keeps values", func(t *testing.T) {
		config := &ConfigData{
			Version: 1,
			Theme:   "Dark",
			Video:   VideoConfig{AspectRatio: "4:3"},
			Audio:   AudioConfig{Volume: 0.5, FastForwardMute: false},
			Window:  WindowConfig{Width: 1024, Height: 768},
			Library: LibraryView{ViewMode: "list", SortBy: "lastPlayed"},
			Rewind:  RewindConfig{BufferSizeMB: 100, FrameStep: 5},
		}
		presentKeys := map[string]bool{
			"theme":             true,
			"video.aspectRatio": true,
			"audio.volume":      true, "audio.fastForwardMute": true,
			"window.width": true, "window.height": true,
			"library.viewMode": true, "library.sortBy": true,
			"rewind.bufferSizeMB": true, "rewind.frameStep": true,
		}

		applyMissingDefaults(config, presentKeys)

		if config.Version != 1 {
			t.Errorf("version should remain 1, got %d", config.Version)
		}
		if config.Theme != "Dark" {
			t.Errorf("theme should remain Dark, got %q", config.Theme)
		}
		if config.Audio.Volume != 0.5 {
			t.Errorf("audio.volume should remain 0.5, got %f", config.Audio.Volume)
		}
		if config.Window.Width != 1024 {
			t.Errorf("window.width should remain 1024, got %d", config.Window.Width)
		}
		if config.Audio.FastForwardMute != false {
			t.Errorf("audio.fastForwardMute should remain false, got %v", config.Audio.FastForwardMute)
		}
		if config.Video.AspectRatio != "4:3" {
			t.Errorf("video.aspectRatio should remain 4:3, got %q", config.Video.AspectRatio)
		}
	})

	t.Run("absent aspect ratio defaults to dar", func(t *testing.T) {
		config := &ConfigData{}
		presentKeys := map[string]bool{}

		applyMissingDefaults(config, presentKeys)

		if config.Video.AspectRatio != "dar" {
			t.Errorf("video.aspectRatio should default to dar, got %q", config.Video.AspectRatio)
		}
	})

	t.Run("present aspect ratio preserved", func(t *testing.T) {
		config := &ConfigData{
			Video: VideoConfig{AspectRatio: "stretch"},
		}
		presentKeys := map[string]bool{
			"video.aspectRatio": true,
		}

		applyMissingDefaults(config, presentKeys)

		if config.Video.AspectRatio != "stretch" {
			t.Errorf("video.aspectRatio should remain stretch, got %q", config.Video.AspectRatio)
		}
	})
}

func TestValidateConfig(t *testing.T) {
	t.Run("valid config has no errors", func(t *testing.T) {
		config := newTestConfigData()
		errs := validateConfig(config, validTestRules)
		if len(errs) != 0 {
			t.Errorf("expected no errors for default config, got: %v", errs)
		}
	})

	t.Run("invalid version", func(t *testing.T) {
		for _, version := range []int{0, 1, configVersion + 1, 99} {
			config := newTestConfigData()
			config.Version = version
			errs := validateConfig(config, validTestRules)
			if len(errs) == 0 {
				t.Errorf("expected error for version %d", version)
			}
		}
	})

	t.Run("invalid theme", func(t *testing.T) {
		config := newTestConfigData()
		config.Theme = "NonexistentTheme"
		errs := validateConfig(config, validTestRules)
		if len(errs) == 0 {
			t.Error("expected error for invalid theme")
		}
	})

	t.Run("valid aspect ratios", func(t *testing.T) {
		for _, ar := range ValidAspectRatios {
			config := newTestConfigData()
			config.Video.AspectRatio = ar
			errs := validateConfig(config, validTestRules)
			if len(errs) != 0 {
				t.Errorf("aspect ratio %q should be valid, got errors: %v", ar, errs)
			}
		}
	})

	t.Run("invalid aspect ratio", func(t *testing.T) {
		config := newTestConfigData()
		config.Video.AspectRatio = "16:9"
		errs := validateConfig(config, validTestRules)
		if len(errs) == 0 {
			t.Error("expected error for invalid aspect ratio 16:9")
		}
	})

	t.Run("negative volume", func(t *testing.T) {
		config := newTestConfigData()
		config.Audio.Volume = -0.1
		errs := validateConfig(config, validTestRules)
		if len(errs) == 0 {
			t.Error("expected error for negative volume")
		}
	})

	t.Run("volume too high", func(t *testing.T) {
		config := newTestConfigData()
		config.Audio.Volume = 2.1
		errs := validateConfig(config, validTestRules)
		if len(errs) == 0 {
			t.Error("expected error for volume > 2.0")
		}
	})

	t.Run("window width too small", func(t *testing.T) {
		config := newTestConfigData()
		config.Window.Width = 800
		errs := validateConfig(config, validTestRules)
		if len(errs) == 0 {
			t.Error("expected error for width < 900")
		}
	})

	t.Run("window height too small", func(t *testing.T) {
		config := newTestConfigData()
		config.Window.Height = 400
		errs := validateConfig(config, validTestRules)
		if len(errs) == 0 {
			t.Error("expected error for height < 650")
		}
	})

	t.Run("invalid viewMode", func(t *testing.T) {
		config := newTestConfigData()
		config.Library.ViewMode = "grid"
		errs := validateConfig(config, validTestRules)
		if len(errs) == 0 {
			t.Error("expected error for invalid viewMode")
		}
	})

	t.Run("artworkDownload values", func(t *testing.T) {
		for _, v := range ValidArtworkDownloads {
			config := newTestConfigData()
			config.Library.ArtworkDownload = v
			if errs := validateConfig(config, validTestRules); len(errs) != 0 {
				t.Errorf("%q: %v", v, errs)
			}
		}
		config := newTestConfigData()
		config.Library.ArtworkDownload = "everything"
		if errs := validateConfig(config, validTestRules); len(errs) == 0 {
			t.Error("expected error for invalid artworkDownload")
		}
	})

	t.Run("invalid sortBy", func(t *testing.T) {
		config := newTestConfigData()
		config.Library.SortBy = "date"
		errs := validateConfig(config, validTestRules)
		if len(errs) == 0 {
			t.Error("expected error for invalid sortBy")
		}
	})

	t.Run("bufferSizeMB too small", func(t *testing.T) {
		config := newTestConfigData()
		config.Rewind.BufferSizeMB = 5
		errs := validateConfig(config, validTestRules)
		if len(errs) == 0 {
			t.Error("expected error for bufferSizeMB < 10")
		}
	})

	t.Run("bufferSizeMB too large", func(t *testing.T) {
		config := newTestConfigData()
		config.Rewind.BufferSizeMB = 500
		errs := validateConfig(config, validTestRules)
		if len(errs) == 0 {
			t.Error("expected error for bufferSizeMB > 400")
		}
	})

	t.Run("frameStep too small", func(t *testing.T) {
		config := newTestConfigData()
		config.Rewind.FrameStep = 0
		errs := validateConfig(config, validTestRules)
		if len(errs) == 0 {
			t.Error("expected error for frameStep < 1")
		}
	})

	t.Run("frameStep too large", func(t *testing.T) {
		config := newTestConfigData()
		config.Rewind.FrameStep = 31
		errs := validateConfig(config, validTestRules)
		if len(errs) == 0 {
			t.Error("expected error for frameStep > 30")
		}
	})

	t.Run("multiple errors at once", func(t *testing.T) {
		config := &ConfigData{
			Version: 99,
			Theme:   "BadTheme",
			Video:   VideoConfig{AspectRatio: "16:9"},
			Audio:   AudioConfig{Volume: 999},
			Window:  WindowConfig{Width: 0, Height: 0},
			Library: LibraryView{ViewMode: "grid", SortBy: "date", ArtworkDownload: "everything"},
			Rewind:  RewindConfig{BufferSizeMB: 0, FrameStep: 0},
		}
		errs := validateConfig(config, validTestRules)
		if len(errs) != 11 {
			t.Errorf("expected 11 errors, got %d: %v", len(errs), errs)
		}
	})

	// Boundary values (valid)
	t.Run("boundary: volume 0.0 is valid", func(t *testing.T) {
		config := newTestConfigData()
		config.Audio.Volume = 0.0
		errs := validateConfig(config, validTestRules)
		if len(errs) != 0 {
			t.Errorf("volume 0.0 should be valid, got errors: %v", errs)
		}
	})

	t.Run("boundary: volume 2.0 is valid", func(t *testing.T) {
		config := newTestConfigData()
		config.Audio.Volume = 2.0
		errs := validateConfig(config, validTestRules)
		if len(errs) != 0 {
			t.Errorf("volume 2.0 should be valid, got errors: %v", errs)
		}
	})

	t.Run("boundary: width 900 is valid", func(t *testing.T) {
		config := newTestConfigData()
		config.Window.Width = 900
		errs := validateConfig(config, validTestRules)
		if len(errs) != 0 {
			t.Errorf("width 900 should be valid, got errors: %v", errs)
		}
	})

	t.Run("boundary: height 650 is valid", func(t *testing.T) {
		config := newTestConfigData()
		config.Window.Height = 650
		errs := validateConfig(config, validTestRules)
		if len(errs) != 0 {
			t.Errorf("height 650 should be valid, got errors: %v", errs)
		}
	})

	t.Run("boundary: bufferSizeMB 10 is valid", func(t *testing.T) {
		config := newTestConfigData()
		config.Rewind.BufferSizeMB = 10
		errs := validateConfig(config, validTestRules)
		if len(errs) != 0 {
			t.Errorf("bufferSizeMB 10 should be valid, got errors: %v", errs)
		}
	})

	t.Run("boundary: bufferSizeMB 400 is valid", func(t *testing.T) {
		config := newTestConfigData()
		config.Rewind.BufferSizeMB = 400
		errs := validateConfig(config, validTestRules)
		if len(errs) != 0 {
			t.Errorf("bufferSizeMB 400 should be valid, got errors: %v", errs)
		}
	})

	t.Run("boundary: frameStep 1 is valid", func(t *testing.T) {
		config := newTestConfigData()
		config.Rewind.FrameStep = 1
		errs := validateConfig(config, validTestRules)
		if len(errs) != 0 {
			t.Errorf("frameStep 1 should be valid, got errors: %v", errs)
		}
	})

	t.Run("boundary: frameStep 30 is valid", func(t *testing.T) {
		config := newTestConfigData()
		config.Rewind.FrameStep = 30
		errs := validateConfig(config, validTestRules)
		if len(errs) != 0 {
			t.Errorf("frameStep 30 should be valid, got errors: %v", errs)
		}
	})
}

func TestCorrectConfig(t *testing.T) {
	t.Run("corrects only invalid fields", func(t *testing.T) {
		config := &ConfigData{
			Version: 1,
			Theme:   "Dark",                                            // valid
			Audio:   AudioConfig{Volume: 999},                          // invalid
			Window:  WindowConfig{Width: 1024, Height: 768},            // valid
			Library: LibraryView{ViewMode: "list", SortBy: "playTime"}, // valid
			Rewind:  RewindConfig{BufferSizeMB: 100, FrameStep: 5},     // valid
		}

		corrected := correctConfig(config, validTestRules)
		defaults := newTestConfigData()

		// Valid fields preserved
		if corrected.Theme != "Dark" {
			t.Errorf("theme should remain Dark, got %q", corrected.Theme)
		}
		if corrected.Window.Width != 1024 {
			t.Errorf("window.width should remain 1024, got %d", corrected.Window.Width)
		}
		if corrected.Window.Height != 768 {
			t.Errorf("window.height should remain 768, got %d", corrected.Window.Height)
		}
		if corrected.Library.ViewMode != "list" {
			t.Errorf("viewMode should remain list, got %q", corrected.Library.ViewMode)
		}
		if corrected.Library.SortBy != "playTime" {
			t.Errorf("sortBy should remain playTime, got %q", corrected.Library.SortBy)
		}
		if corrected.Rewind.BufferSizeMB != 100 {
			t.Errorf("bufferSizeMB should remain 100, got %d", corrected.Rewind.BufferSizeMB)
		}
		if corrected.Rewind.FrameStep != 5 {
			t.Errorf("frameStep should remain 5, got %d", corrected.Rewind.FrameStep)
		}

		// Invalid fields corrected
		if corrected.Audio.Volume != defaults.Audio.Volume {
			t.Errorf("volume should be corrected to %f, got %f", defaults.Audio.Volume, corrected.Audio.Volume)
		}
	})

	t.Run("valid config unchanged", func(t *testing.T) {
		config := newTestConfigData()
		original, _ := json.Marshal(config)

		corrected := correctConfig(config, validTestRules)
		after, _ := json.Marshal(corrected)

		if string(original) != string(after) {
			t.Error("valid config should not be modified")
		}
	})

	t.Run("invalid aspect ratio corrected to dar", func(t *testing.T) {
		config := newTestConfigData()
		config.Video.AspectRatio = "16:9"

		corrected := correctConfig(config, validTestRules)

		if corrected.Video.AspectRatio != "dar" {
			t.Errorf("video.aspectRatio should be corrected to dar, got %q", corrected.Video.AspectRatio)
		}
	})

	t.Run("valid aspect ratio preserved", func(t *testing.T) {
		config := newTestConfigData()
		config.Video.AspectRatio = "4:3"

		corrected := correctConfig(config, validTestRules)

		if corrected.Video.AspectRatio != "4:3" {
			t.Errorf("video.aspectRatio should remain 4:3, got %q", corrected.Video.AspectRatio)
		}
	})

	t.Run("all invalid resets all to defaults", func(t *testing.T) {
		config := &ConfigData{
			Version: 99,
			Theme:   "BadTheme",
			Video:   VideoConfig{AspectRatio: "bad"},
			Audio:   AudioConfig{Volume: 999},
			Window:  WindowConfig{Width: 0, Height: 0},
			Library: LibraryView{ViewMode: "grid", SortBy: "date", ArtworkDownload: "everything"},
			Rewind:  RewindConfig{BufferSizeMB: 0, FrameStep: 0},
		}

		corrected := correctConfig(config, validTestRules)
		defaults := newTestConfigData()

		if corrected.Library.ArtworkDownload != defaults.Library.ArtworkDownload {
			t.Errorf("artworkDownload: got %q, want %q", corrected.Library.ArtworkDownload, defaults.Library.ArtworkDownload)
		}
		if corrected.Version != defaults.Version {
			t.Errorf("version: got %d, want %d", corrected.Version, defaults.Version)
		}
		if corrected.Theme != defaults.Theme {
			t.Errorf("theme: got %q, want %q", corrected.Theme, defaults.Theme)
		}
		if corrected.Audio.Volume != defaults.Audio.Volume {
			t.Errorf("volume: got %f, want %f", corrected.Audio.Volume, defaults.Audio.Volume)
		}
		if corrected.Window.Width != defaults.Window.Width {
			t.Errorf("width: got %d, want %d", corrected.Window.Width, defaults.Window.Width)
		}
		if corrected.Window.Height != defaults.Window.Height {
			t.Errorf("height: got %d, want %d", corrected.Window.Height, defaults.Window.Height)
		}
		if corrected.Library.ViewMode != defaults.Library.ViewMode {
			t.Errorf("viewMode: got %q, want %q", corrected.Library.ViewMode, defaults.Library.ViewMode)
		}
		if corrected.Library.SortBy != defaults.Library.SortBy {
			t.Errorf("sortBy: got %q, want %q", corrected.Library.SortBy, defaults.Library.SortBy)
		}
		if corrected.Rewind.BufferSizeMB != defaults.Rewind.BufferSizeMB {
			t.Errorf("bufferSizeMB: got %d, want %d", corrected.Rewind.BufferSizeMB, defaults.Rewind.BufferSizeMB)
		}
		if corrected.Rewind.FrameStep != defaults.Rewind.FrameStep {
			t.Errorf("frameStep: got %d, want %d", corrected.Rewind.FrameStep, defaults.Rewind.FrameStep)
		}
	})
}

func TestValidateInputConfig(t *testing.T) {
	validKey := func(name string) bool {
		return name == "J" || name == "K" || name == "W" || name == "ArrowUp"
	}
	validPad := func(name string) bool {
		return name == "A" || name == "B" || name == "DpadUp"
	}

	t.Run("empty config has no errors", func(t *testing.T) {
		config := newTestConfigData()
		errs := validateInputConfig(config, validKey, validPad)
		if len(errs) != 0 {
			t.Errorf("expected no errors, got: %v", errs)
		}
	})

	t.Run("valid overrides have no errors", func(t *testing.T) {
		config := newTestConfigData()
		config.Input.P1Keyboard = map[string]string{"Up": "ArrowUp", "A": "J"}
		config.Input.Profiles = []ControllerProfile{
			{ID: "p1", SDLID: "sdl1", Controller: "Pad", Name: "default", Bindings: map[string]string{"A": "DpadUp"}},
		}
		config.Input.Players = []PlayerConfig{{Profile: "p1"}, {Profile: ""}}
		errs := validateInputConfig(config, validKey, validPad)
		if len(errs) != 0 {
			t.Errorf("expected no errors, got: %v", errs)
		}
	})

	t.Run("invalid key name detected", func(t *testing.T) {
		config := newTestConfigData()
		config.Input.P1Keyboard = map[string]string{"A": "BadKey"}
		errs := validateInputConfig(config, validKey, validPad)
		if len(errs) != 1 {
			t.Errorf("expected 1 error, got %d: %v", len(errs), errs)
		}
	})

	t.Run("invalid pad name detected", func(t *testing.T) {
		config := newTestConfigData()
		config.Input.Profiles = []ControllerProfile{
			{ID: "p1", SDLID: "sdl1", Controller: "Pad", Name: "default", Bindings: map[string]string{"A": "BadPad"}},
		}
		errs := validateInputConfig(config, validKey, validPad)
		if len(errs) != 1 {
			t.Errorf("expected 1 error, got %d: %v", len(errs), errs)
		}
	})

	t.Run("profile identity problems detected", func(t *testing.T) {
		config := newTestConfigData()
		config.Input.Profiles = []ControllerProfile{
			{ID: "", SDLID: "sdl1", Controller: "Pad", Name: "default"},
			{ID: "p1", SDLID: "sdl1", Controller: "Pad", Name: "one"},
			{ID: "p1", SDLID: "sdl1", Controller: "Pad", Name: "two"},
			{ID: "p2", SDLID: "sdl1", Controller: "Pad", Name: "one"},
		}
		errs := validateInputConfig(config, validKey, validPad)
		// missing id, duplicate id, duplicate name per model
		if len(errs) != 3 {
			t.Errorf("expected 3 errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("player problems detected", func(t *testing.T) {
		config := newTestConfigData()
		config.Input.Players = []PlayerConfig{
			{Profile: "missing"}, {Profile: ""}, {Profile: ""},
		}
		errs := validateInputConfig(config, validKey, validPad)
		// too many players, unknown profile reference
		if len(errs) != 2 {
			t.Errorf("expected 2 errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("profile rumbleScale out of range detected", func(t *testing.T) {
		config := newTestConfigData()
		config.Input.Profiles = []ControllerProfile{
			{ID: "p1", SDLID: "sdl1", Controller: "Pad", Name: "one", RumbleScale: -0.05},
			{ID: "p2", SDLID: "sdl1", Controller: "Pad", Name: "two", RumbleScale: 3.05},
			{ID: "p3", SDLID: "sdl1", Controller: "Pad", Name: "three", RumbleScale: 3.0},
			{ID: "p4", SDLID: "sdl1", Controller: "Pad", Name: "four", RumbleScale: 0.0},
		}
		errs := validateInputConfig(config, validKey, validPad)
		if len(errs) != 2 {
			t.Errorf("expected 2 errors, got %d: %v", len(errs), errs)
		}
	})
}

func TestCorrectInputConfig(t *testing.T) {
	validKey := func(name string) bool {
		return name == "J" || name == "K"
	}
	validPad := func(name string) bool {
		return name == "A" || name == "B"
	}

	t.Run("removes invalid entries", func(t *testing.T) {
		config := newTestConfigData()
		config.Input.P1Keyboard = map[string]string{"A": "J", "B": "BadKey"}
		config.Input.Profiles = []ControllerProfile{
			{ID: "p1", SDLID: "sdl1", Controller: "Pad", Name: "default", Bindings: map[string]string{"A": "A", "B": "BadPad"}},
		}

		correctInputConfig(config, validKey, validPad)

		if len(config.Input.P1Keyboard) != 1 {
			t.Errorf("expected 1 keyboard entry, got %d", len(config.Input.P1Keyboard))
		}
		if config.Input.P1Keyboard["A"] != "J" {
			t.Error("valid keyboard entry should be preserved")
		}
		if len(config.Input.Profiles[0].Bindings) != 1 {
			t.Errorf("expected 1 binding, got %d", len(config.Input.Profiles[0].Bindings))
		}
		if config.Input.Profiles[0].Bindings["A"] != "A" {
			t.Error("valid binding should be preserved")
		}
	})

	t.Run("drops invalid and duplicate profiles", func(t *testing.T) {
		config := newTestConfigData()
		config.Input.Profiles = []ControllerProfile{
			{ID: "", SDLID: "sdl1", Controller: "Pad", Name: "default"},
			{ID: "p1", SDLID: "", Controller: "Pad", Name: "default"},
			{ID: "p1", SDLID: "sdl1", Controller: "Pad", Name: "one"},
			{ID: "p1", SDLID: "sdl1", Controller: "Pad", Name: "two"},
			{ID: "p2", SDLID: "sdl1", Controller: "Pad", Name: "one"},
			{ID: "p3", SDLID: "sdl2", Controller: "Pad", Name: "one"},
		}

		correctInputConfig(config, validKey, validPad)

		if len(config.Input.Profiles) != 2 {
			t.Fatalf("expected 2 profiles, got %d: %v", len(config.Input.Profiles), config.Input.Profiles)
		}
		if config.Input.Profiles[0].ID != "p1" || config.Input.Profiles[0].Name != "one" {
			t.Errorf("unexpected first profile: %v", config.Input.Profiles[0])
		}
		if config.Input.Profiles[1].ID != "p3" {
			t.Errorf("unexpected second profile: %v", config.Input.Profiles[1])
		}
	})

	t.Run("clamps players and clears dangling references", func(t *testing.T) {
		config := newTestConfigData()
		config.Input.Profiles = []ControllerProfile{
			{ID: "p1", SDLID: "sdl1", Controller: "Pad", Name: "default"},
		}
		config.Input.Players = []PlayerConfig{
			{Profile: "p1"}, {Profile: "missing"}, {Profile: "p1"},
		}

		correctInputConfig(config, validKey, validPad)

		if len(config.Input.Players) != 2 {
			t.Fatalf("expected 2 players, got %d", len(config.Input.Players))
		}
		if config.Input.Players[0].Profile != "p1" {
			t.Error("valid player reference should be preserved")
		}
		if config.Input.Players[1].Profile != "" {
			t.Error("dangling player reference should be cleared")
		}
	})

	t.Run("resets out-of-range rumbleScale to default", func(t *testing.T) {
		config := newTestConfigData()
		config.Input.Profiles = []ControllerProfile{
			{ID: "p1", SDLID: "sdl1", Controller: "Pad", Name: "one", RumbleScale: 5.0},
			{ID: "p2", SDLID: "sdl1", Controller: "Pad", Name: "two", RumbleScale: 0.5},
			{ID: "p3", SDLID: "sdl1", Controller: "Pad", Name: "three"},
		}

		correctInputConfig(config, validKey, validPad)

		if config.Input.Profiles[0].RumbleScale != DefaultRumbleScale {
			t.Errorf("out-of-range rumbleScale should reset to default, got %f", config.Input.Profiles[0].RumbleScale)
		}
		if config.Input.Profiles[1].RumbleScale != 0.5 {
			t.Errorf("valid rumbleScale should be preserved, got %f", config.Input.Profiles[1].RumbleScale)
		}
		if config.Input.Profiles[2].RumbleScale != 0 {
			t.Errorf("zero (off) rumbleScale should be preserved, got %f", config.Input.Profiles[2].RumbleScale)
		}
	})

	t.Run("nil maps are safe", func(t *testing.T) {
		config := newTestConfigData()
		correctInputConfig(config, validKey, validPad) // Should not panic
	})
}

func TestProfileRumbleScaleUnmarshal(t *testing.T) {
	t.Run("absent key defaults", func(t *testing.T) {
		jsonBytes := []byte(`{"input": {"profiles": [
			{"id": "p1", "sdlId": "sdl1", "controller": "Pad", "name": "one"}
		]}}`)
		config := &ConfigData{}
		if err := json.Unmarshal(jsonBytes, config); err != nil {
			t.Fatal(err)
		}
		if got := config.Input.Profiles[0].RumbleScale; got != DefaultRumbleScale {
			t.Errorf("absent rumbleScale should default to %f, got %f", DefaultRumbleScale, got)
		}
	})

	t.Run("explicit 0 preserved", func(t *testing.T) {
		jsonBytes := []byte(`{"input": {"profiles": [
			{"id": "p1", "sdlId": "sdl1", "controller": "Pad", "name": "one", "rumbleScale": 0}
		]}}`)
		config := &ConfigData{}
		if err := json.Unmarshal(jsonBytes, config); err != nil {
			t.Fatal(err)
		}
		if got := config.Input.Profiles[0].RumbleScale; got != 0.0 {
			t.Errorf("explicit rumbleScale=0 should be preserved, got %f", got)
		}
	})

	t.Run("explicit value preserved", func(t *testing.T) {
		jsonBytes := []byte(`{"input": {"profiles": [
			{"id": "p1", "sdlId": "sdl1", "controller": "Pad", "name": "one", "rumbleScale": 0.5}
		]}}`)
		config := &ConfigData{}
		if err := json.Unmarshal(jsonBytes, config); err != nil {
			t.Fatal(err)
		}
		if got := config.Input.Profiles[0].RumbleScale; got != 0.5 {
			t.Errorf("explicit rumbleScale should be preserved, got %f", got)
		}
	})
}

func TestPlayerRumbleScale(t *testing.T) {
	input := &InputConfig{
		Profiles: []ControllerProfile{
			{ID: "p1", SDLID: "sdl1", Controller: "Pad", Name: "one", RumbleScale: 0.5},
			{ID: "p2", SDLID: "sdl1", Controller: "Pad", Name: "two", RumbleScale: 0.0},
		},
		Players: []PlayerConfig{{Profile: "p1"}, {Profile: "p2"}},
	}

	if got := input.PlayerRumbleScale(0); got != 0.5 {
		t.Errorf("player 0 scale: got %f, want 0.5", got)
	}
	if got := input.PlayerRumbleScale(1); got != 0.0 {
		t.Errorf("player 1 scale: got %f, want 0.0", got)
	}
	if got := input.PlayerRumbleScale(2); got != 0.0 {
		t.Errorf("unassigned slot scale: got %f, want 0.0", got)
	}
	if got := input.PlayerRumbleScale(-1); got != 0.0 {
		t.Errorf("negative slot scale: got %f, want 0.0", got)
	}
}

func TestRumbleEnabled(t *testing.T) {
	t.Run("enabled when any assigned profile has non-zero scale", func(t *testing.T) {
		input := &InputConfig{
			Profiles: []ControllerProfile{
				{ID: "p1", SDLID: "sdl1", Controller: "Pad", Name: "one", RumbleScale: 0.0},
				{ID: "p2", SDLID: "sdl1", Controller: "Pad", Name: "two", RumbleScale: 1.0},
			},
			Players: []PlayerConfig{{Profile: "p1"}, {Profile: "p2"}},
		}
		if !input.RumbleEnabled() {
			t.Error("expected rumble enabled")
		}
	})

	t.Run("disabled when assigned profiles are all off", func(t *testing.T) {
		input := &InputConfig{
			Profiles: []ControllerProfile{
				{ID: "p1", SDLID: "sdl1", Controller: "Pad", Name: "one", RumbleScale: 0.0},
			},
			Players: []PlayerConfig{{Profile: "p1"}},
		}
		if input.RumbleEnabled() {
			t.Error("expected rumble disabled")
		}
	})

	t.Run("disabled when unassigned profile has non-zero scale", func(t *testing.T) {
		input := &InputConfig{
			Profiles: []ControllerProfile{
				{ID: "p1", SDLID: "sdl1", Controller: "Pad", Name: "one", RumbleScale: 1.0},
			},
		}
		if input.RumbleEnabled() {
			t.Error("expected rumble disabled with no player assignment")
		}
	})
}

func TestInputConfigSerialization(t *testing.T) {
	t.Run("empty maps omitted from JSON", func(t *testing.T) {
		config := newTestConfigData()
		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		s := string(data)
		// p1Keyboard should not appear since it's nil (omitempty)
		if contains(s, "p1Keyboard") {
			t.Error("empty p1Keyboard should be omitted from JSON")
		}
	})

	t.Run("non-empty maps included in JSON", func(t *testing.T) {
		config := newTestConfigData()
		config.Input.P1Keyboard = map[string]string{"Up": "ArrowUp"}
		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		s := string(data)
		if !contains(s, "p1Keyboard") {
			t.Error("non-empty p1Keyboard should be in JSON")
		}
		if !contains(s, "ArrowUp") {
			t.Error("ArrowUp value should be in JSON")
		}
	})

	t.Run("roundtrip preserves overrides", func(t *testing.T) {
		config := newTestConfigData()
		config.Input.P1Keyboard = map[string]string{"Up": "ArrowUp", "A": "Z"}
		config.Input.Profiles = []ControllerProfile{
			{ID: "p1", SDLID: "sdl1", Controller: "Pad", Name: "default", Bindings: map[string]string{"A": "Y"}},
		}
		config.Input.Players = []PlayerConfig{{Profile: "p1"}}
		config.CoreOptions = map[string]string{"sixbutton": "true"}

		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}

		var restored ConfigData
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}

		if restored.Input.P1Keyboard["Up"] != "ArrowUp" {
			t.Error("P1Keyboard Up override not preserved")
		}
		if restored.Input.P1Keyboard["A"] != "Z" {
			t.Error("P1Keyboard A override not preserved")
		}
		if len(restored.Input.Profiles) != 1 || restored.Input.Profiles[0].Bindings["A"] != "Y" {
			t.Error("profile binding not preserved")
		}
		if len(restored.Input.Players) != 1 || restored.Input.Players[0].Profile != "p1" {
			t.Error("player assignment not preserved")
		}
		if restored.CoreOptions["sixbutton"] != "true" {
			t.Error("CoreOptions not preserved")
		}
	})

	t.Run("legacy p1Controller key is ignored on load", func(t *testing.T) {
		var restored ConfigData
		legacy := `{"input":{"p1Keyboard":{"Up":"ArrowUp"},"p1Controller":{"A":"Y"}}}`
		if err := json.Unmarshal([]byte(legacy), &restored); err != nil {
			t.Fatal(err)
		}
		if restored.Input.P1Keyboard["Up"] != "ArrowUp" {
			t.Error("p1Keyboard should still load")
		}
		data, err := json.Marshal(&restored)
		if err != nil {
			t.Fatal(err)
		}
		if contains(string(data), "p1Controller") {
			t.Error("legacy p1Controller should not survive a save")
		}
	})

	t.Run("absent profiles and players stay empty", func(t *testing.T) {
		jsonBytes := []byte(`{"version":1}`)
		var config ConfigData
		if err := json.Unmarshal(jsonBytes, &config); err != nil {
			t.Fatal(err)
		}
		applyMissingDefaults(&config, detectPresentKeys(jsonBytes))
		if len(config.Input.Profiles) != 0 {
			t.Error("profiles should default to empty")
		}
		if len(config.Input.Players) != 0 {
			t.Error("players should default to empty")
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestLegacyDisableAnalogStickIgnored(t *testing.T) {
	var restored ConfigData
	legacy := `{"input":{"p1Keyboard":{"Up":"ArrowUp"},"disableAnalogStick":true}}`
	if err := json.Unmarshal([]byte(legacy), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Input.P1Keyboard["Up"] != "ArrowUp" {
		t.Error("p1Keyboard should still load")
	}
	data, err := json.Marshal(&restored)
	if err != nil {
		t.Fatal(err)
	}
	if contains(string(data), "disableAnalogStick") {
		t.Error("legacy disableAnalogStick should not survive a save")
	}
}

func TestPresentButZeroVsMissing(t *testing.T) {
	// A key present with a zero value is kept by applyMissingDefaults and
	// left for validateConfig to judge; an absent key gets its default.
	t.Run("present volume=0 preserved", func(t *testing.T) {
		jsonBytes := []byte(`{"audio": {"volume": 0}}`)
		config := &ConfigData{}
		json.Unmarshal(jsonBytes, config)

		presentKeys := detectPresentKeys(jsonBytes)
		applyMissingDefaults(config, presentKeys)

		if config.Audio.Volume != 0.0 {
			t.Errorf("present volume=0 should be preserved, got %f", config.Audio.Volume)
		}

		// volume=0 is valid (within 0.0-2.0 range)
		errs := validateConfig(config, validTestRules)
		for _, e := range errs {
			if len(e) > 12 && e[:12] == "audio.volume" {
				t.Errorf("volume 0.0 should be valid, got error: %s", e)
			}
		}
	})
}

func TestValidPadMapping(t *testing.T) {
	const guid = "030000007e0500000920000000020000"
	tests := []struct {
		name string
		guid string
		line string
		want bool
	}{
		{"valid", guid, guid + ",Pad,a:b0,dpup:h0.1,", true},
		{"empty guid", "", ",Pad,a:b0,", false},
		{"short guid", "abcd", "abcd,Pad,a:b0,", false},
		{"uppercase guid", strings.ToUpper(guid), strings.ToUpper(guid) + ",Pad,a:b0,", false},
		{"non-hex guid", "zz0000007e0500000920000000020000", "zz0000007e0500000920000000020000,Pad,", false},
		{"line missing guid prefix", guid, "ffffffffffffffffffffffffffffffff,Pad,a:b0,", false},
		{"guid only no comma", guid, guid, false},
		{"newline injection", guid, guid + ",Pad,a:b0,\nffffffffffffffffffffffffffffffff,Other,b:b1,", false},
		{"carriage return", guid, guid + ",Pad,a:b0,\r", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidPadMapping(tt.guid, tt.line); got != tt.want {
				t.Errorf("ValidPadMapping(%q, %q) = %v, want %v", tt.guid, tt.line, got, tt.want)
			}
		})
	}
}

func TestValidateAndCorrectPadMappings(t *testing.T) {
	const guid = "030000007e0500000920000000020000"
	validKey := func(string) bool { return true }
	validPad := func(string) bool { return true }

	config := newTestConfigData()
	config.Input.PadMappings = map[string]string{
		guid:  guid + ",Pad,a:b0,dpup:h0.1,",
		"bad": "bad,Pad,a:b0,",
	}

	errs := validateInputConfig(config, validKey, validPad)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got: %v", errs)
	}

	correctInputConfig(config, validKey, validPad)
	if len(config.Input.PadMappings) != 1 {
		t.Fatalf("expected 1 mapping after correction, got: %v", config.Input.PadMappings)
	}
	if _, ok := config.Input.PadMappings[guid]; !ok {
		t.Error("valid mapping was removed by correction")
	}

	config.Input.PadMappings = map[string]string{"bad": "bad"}
	correctInputConfig(config, validKey, validPad)
	if config.Input.PadMappings != nil {
		t.Errorf("expected nil map after removing all entries, got: %v", config.Input.PadMappings)
	}
}

// TestConfigValidateCorrectInput tests that Config.Validate reports invalid
// input entries along with other fields, and Config.Correct removes them
// while keeping the valid ones.
func TestConfigValidateCorrectInput(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetKeyboardBinding("Up", "W")
	cfg.SetKeyboardBinding("A", "NoSuchKey")
	id := cfg.AddProfile(ControllerProfile{
		SDLID: "guid", Controller: "Pad", Name: "Mine", RumbleScale: 1,
		Bindings: map[string]string{"A": "B", "B": "NoSuchButton"},
	})
	cfg.SetPlayerProfile(0, id)
	cfg.SetTheme("NoSuchTheme")

	errs := cfg.Validate(validTestRules)
	for _, want := range []string{"theme", `input.p1Keyboard["A"]`, `bindings["B"]`} {
		found := false
		for _, e := range errs {
			if strings.Contains(e, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("no error for %s in %v", want, errs)
		}
	}
	if len(errs) != 3 {
		t.Errorf("got %d errors, want 3: %v", len(errs), errs)
	}

	cfg.Correct(validTestRules)
	if errs := cfg.Validate(validTestRules); len(errs) != 0 {
		t.Fatalf("errors after Correct: %v", errs)
	}
	v := cfg.Values()
	if len(v.Input.P1Keyboard) != 1 || v.Input.P1Keyboard["Up"] != "W" {
		t.Errorf("keyboard = %v, want only Up=W", v.Input.P1Keyboard)
	}
	prof := v.Input.PlayerProfile(0)
	if prof == nil || prof.ID != id || len(prof.Bindings) != 1 || prof.Bindings["A"] != "B" {
		t.Errorf("player 1 profile = %+v, want %s with only A=B", prof, id)
	}
}
