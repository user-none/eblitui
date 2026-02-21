package storage

import (
	"encoding/json"
	"testing"
)

// validTestThemes is the list of theme names used in tests
var validTestThemes = []string{"Default", "Dark", "Light", "Retro", "Pink", "Hot Pink", "Green LCD", "High Contrast"}

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
				"fontSize": 14,
				"audio": {"volume": 1.0, "fastForwardMute": true},
				"window": {"width": 900, "height": 650},
				"library": {"viewMode": "icon", "sortBy": "title"},
				"rewind": {"bufferSizeMB": 40, "frameStep": 1}
			}`,
			expected: map[string]bool{
				"version": true, "theme": true, "fontSize": true,
				"audio.volume": true, "audio.fastForwardMute": true,
				"window.width": true, "window.height": true,
				"library.viewMode": true, "library.sortBy": true,
				"rewind.bufferSizeMB": true, "rewind.frameStep": true,
			},
		},
		{
			name:     "empty object",
			json:     `{}`,
			expected: map[string]bool{},
		},
		{
			name: "partial keys - missing fontSize and rewind",
			json: `{
				"version": 1,
				"theme": "Dark",
				"audio": {"volume": 0.5},
				"window": {"width": 1024, "height": 768},
				"library": {"viewMode": "list", "sortBy": "lastPlayed"}
			}`,
			expected: map[string]bool{
				"version": true, "theme": true,
				"audio.volume": true, "window.width": true, "window.height": true,
				"library.viewMode": true, "library.sortBy": true,
			},
		},
		{
			name: "zero values are still present",
			json: `{
				"fontSize": 0,
				"audio": {"volume": 0},
				"window": {"width": 0, "height": 0}
			}`,
			expected: map[string]bool{
				"fontSize": true, "audio.volume": true,
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
		config := &Config{}
		presentKeys := map[string]bool{}

		ApplyMissingDefaults(config, presentKeys)

		defaults := DefaultConfig()
		if config.Version != defaults.Version {
			t.Errorf("version: got %d, want %d", config.Version, defaults.Version)
		}
		if config.Theme != defaults.Theme {
			t.Errorf("theme: got %q, want %q", config.Theme, defaults.Theme)
		}
		if config.FontSize != defaults.FontSize {
			t.Errorf("fontSize: got %d, want %d", config.FontSize, defaults.FontSize)
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
		if config.Rewind.BufferSizeMB != defaults.Rewind.BufferSizeMB {
			t.Errorf("rewind.bufferSizeMB: got %d, want %d", config.Rewind.BufferSizeMB, defaults.Rewind.BufferSizeMB)
		}
		if config.Rewind.FrameStep != defaults.Rewind.FrameStep {
			t.Errorf("rewind.frameStep: got %d, want %d", config.Rewind.FrameStep, defaults.Rewind.FrameStep)
		}
		if config.Audio.FastForwardMute != defaults.Audio.FastForwardMute {
			t.Errorf("audio.fastForwardMute: got %v, want %v", config.Audio.FastForwardMute, defaults.Audio.FastForwardMute)
		}
	})

	t.Run("present keys preserved even when zero", func(t *testing.T) {
		config := &Config{
			Audio:  AudioConfig{Volume: 0.0},
			Window: WindowConfig{Width: 0, Height: 0},
		}
		presentKeys := map[string]bool{
			"audio.volume":  true,
			"window.width":  true,
			"window.height": true,
		}

		ApplyMissingDefaults(config, presentKeys)

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
		defaults := DefaultConfig()
		if config.Version != defaults.Version {
			t.Errorf("version should default to %d, got %d", defaults.Version, config.Version)
		}
		if config.Theme != defaults.Theme {
			t.Errorf("theme should default to %q, got %q", defaults.Theme, config.Theme)
		}
		if config.FontSize != defaults.FontSize {
			t.Errorf("fontSize should default to %d, got %d", defaults.FontSize, config.FontSize)
		}
	})

	t.Run("all present keeps values", func(t *testing.T) {
		config := &Config{
			Version:  1,
			Theme:    "Dark",
			FontSize: 20,
			Audio:    AudioConfig{Volume: 0.5, FastForwardMute: false},
			Window:   WindowConfig{Width: 1024, Height: 768},
			Library:  LibraryView{ViewMode: "list", SortBy: "lastPlayed"},
			Rewind:   RewindConfig{BufferSizeMB: 100, FrameStep: 5},
		}
		presentKeys := map[string]bool{
			"version": true, "theme": true, "fontSize": true,
			"audio.volume": true, "audio.fastForwardMute": true,
			"window.width": true, "window.height": true,
			"library.viewMode": true, "library.sortBy": true,
			"rewind.bufferSizeMB": true, "rewind.frameStep": true,
		}

		ApplyMissingDefaults(config, presentKeys)

		if config.Version != 1 {
			t.Errorf("version should remain 1, got %d", config.Version)
		}
		if config.Theme != "Dark" {
			t.Errorf("theme should remain Dark, got %q", config.Theme)
		}
		if config.FontSize != 20 {
			t.Errorf("fontSize should remain 20, got %d", config.FontSize)
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
	})
}

func TestValidateConfig(t *testing.T) {
	t.Run("valid config has no errors", func(t *testing.T) {
		config := DefaultConfig()
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) != 0 {
			t.Errorf("expected no errors for default config, got: %v", errs)
		}
	})

	t.Run("invalid version", func(t *testing.T) {
		config := DefaultConfig()
		config.Version = 99
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) == 0 {
			t.Error("expected error for invalid version")
		}
	})

	t.Run("invalid theme", func(t *testing.T) {
		config := DefaultConfig()
		config.Theme = "NonexistentTheme"
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) == 0 {
			t.Error("expected error for invalid theme")
		}
	})

	t.Run("invalid fontSize", func(t *testing.T) {
		config := DefaultConfig()
		config.FontSize = -5
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) == 0 {
			t.Error("expected error for negative fontSize")
		}
	})

	t.Run("fontSize zero is invalid", func(t *testing.T) {
		config := DefaultConfig()
		config.FontSize = 0
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) == 0 {
			t.Error("expected error for fontSize=0")
		}
	})

	t.Run("fontSize not in presets", func(t *testing.T) {
		config := DefaultConfig()
		config.FontSize = 15
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) == 0 {
			t.Error("expected error for fontSize=15 (not a preset)")
		}
	})

	t.Run("negative volume", func(t *testing.T) {
		config := DefaultConfig()
		config.Audio.Volume = -0.1
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) == 0 {
			t.Error("expected error for negative volume")
		}
	})

	t.Run("volume too high", func(t *testing.T) {
		config := DefaultConfig()
		config.Audio.Volume = 2.1
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) == 0 {
			t.Error("expected error for volume > 2.0")
		}
	})

	t.Run("window width too small", func(t *testing.T) {
		config := DefaultConfig()
		config.Window.Width = 800
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) == 0 {
			t.Error("expected error for width < 900")
		}
	})

	t.Run("window height too small", func(t *testing.T) {
		config := DefaultConfig()
		config.Window.Height = 400
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) == 0 {
			t.Error("expected error for height < 650")
		}
	})

	t.Run("invalid viewMode", func(t *testing.T) {
		config := DefaultConfig()
		config.Library.ViewMode = "grid"
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) == 0 {
			t.Error("expected error for invalid viewMode")
		}
	})

	t.Run("invalid sortBy", func(t *testing.T) {
		config := DefaultConfig()
		config.Library.SortBy = "date"
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) == 0 {
			t.Error("expected error for invalid sortBy")
		}
	})

	t.Run("bufferSizeMB too small", func(t *testing.T) {
		config := DefaultConfig()
		config.Rewind.BufferSizeMB = 5
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) == 0 {
			t.Error("expected error for bufferSizeMB < 10")
		}
	})

	t.Run("bufferSizeMB too large", func(t *testing.T) {
		config := DefaultConfig()
		config.Rewind.BufferSizeMB = 500
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) == 0 {
			t.Error("expected error for bufferSizeMB > 200")
		}
	})

	t.Run("frameStep too small", func(t *testing.T) {
		config := DefaultConfig()
		config.Rewind.FrameStep = 0
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) == 0 {
			t.Error("expected error for frameStep < 1")
		}
	})

	t.Run("frameStep too large", func(t *testing.T) {
		config := DefaultConfig()
		config.Rewind.FrameStep = 11
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) == 0 {
			t.Error("expected error for frameStep > 10")
		}
	})

	t.Run("multiple errors at once", func(t *testing.T) {
		config := &Config{
			Version:  99,
			Theme:    "BadTheme",
			FontSize: -1,
			Audio:    AudioConfig{Volume: 999},
			Window:   WindowConfig{Width: 0, Height: 0},
			Library:  LibraryView{ViewMode: "grid", SortBy: "date"},
			Rewind:   RewindConfig{BufferSizeMB: 0, FrameStep: 0},
		}
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) != 10 {
			t.Errorf("expected 10 errors, got %d: %v", len(errs), errs)
		}
	})

	// Boundary values (valid)
	t.Run("boundary: volume 0.0 is valid", func(t *testing.T) {
		config := DefaultConfig()
		config.Audio.Volume = 0.0
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) != 0 {
			t.Errorf("volume 0.0 should be valid, got errors: %v", errs)
		}
	})

	t.Run("boundary: volume 2.0 is valid", func(t *testing.T) {
		config := DefaultConfig()
		config.Audio.Volume = 2.0
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) != 0 {
			t.Errorf("volume 2.0 should be valid, got errors: %v", errs)
		}
	})

	t.Run("boundary: width 900 is valid", func(t *testing.T) {
		config := DefaultConfig()
		config.Window.Width = 900
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) != 0 {
			t.Errorf("width 900 should be valid, got errors: %v", errs)
		}
	})

	t.Run("boundary: height 650 is valid", func(t *testing.T) {
		config := DefaultConfig()
		config.Window.Height = 650
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) != 0 {
			t.Errorf("height 650 should be valid, got errors: %v", errs)
		}
	})

	t.Run("boundary: bufferSizeMB 10 is valid", func(t *testing.T) {
		config := DefaultConfig()
		config.Rewind.BufferSizeMB = 10
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) != 0 {
			t.Errorf("bufferSizeMB 10 should be valid, got errors: %v", errs)
		}
	})

	t.Run("boundary: bufferSizeMB 200 is valid", func(t *testing.T) {
		config := DefaultConfig()
		config.Rewind.BufferSizeMB = 200
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) != 0 {
			t.Errorf("bufferSizeMB 200 should be valid, got errors: %v", errs)
		}
	})

	t.Run("boundary: frameStep 1 is valid", func(t *testing.T) {
		config := DefaultConfig()
		config.Rewind.FrameStep = 1
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) != 0 {
			t.Errorf("frameStep 1 should be valid, got errors: %v", errs)
		}
	})

	t.Run("boundary: frameStep 10 is valid", func(t *testing.T) {
		config := DefaultConfig()
		config.Rewind.FrameStep = 10
		errs := ValidateConfig(config, validTestThemes)
		if len(errs) != 0 {
			t.Errorf("frameStep 10 should be valid, got errors: %v", errs)
		}
	})
}

func TestCorrectConfig(t *testing.T) {
	t.Run("corrects only invalid fields", func(t *testing.T) {
		config := &Config{
			Version:  1,
			Theme:    "Dark",                                            // valid
			FontSize: -5,                                                // invalid
			Audio:    AudioConfig{Volume: 999},                          // invalid
			Window:   WindowConfig{Width: 1024, Height: 768},            // valid
			Library:  LibraryView{ViewMode: "list", SortBy: "playTime"}, // valid
			Rewind:   RewindConfig{BufferSizeMB: 100, FrameStep: 5},     // valid
		}

		corrected := CorrectConfig(config, validTestThemes)
		defaults := DefaultConfig()

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
		if corrected.FontSize != defaults.FontSize {
			t.Errorf("fontSize should be corrected to %d, got %d", defaults.FontSize, corrected.FontSize)
		}
		if corrected.Audio.Volume != defaults.Audio.Volume {
			t.Errorf("volume should be corrected to %f, got %f", defaults.Audio.Volume, corrected.Audio.Volume)
		}
	})

	t.Run("valid config unchanged", func(t *testing.T) {
		config := DefaultConfig()
		original, _ := json.Marshal(config)

		corrected := CorrectConfig(config, validTestThemes)
		after, _ := json.Marshal(corrected)

		if string(original) != string(after) {
			t.Error("valid config should not be modified")
		}
	})

	t.Run("all invalid resets all to defaults", func(t *testing.T) {
		config := &Config{
			Version:  99,
			Theme:    "BadTheme",
			FontSize: -1,
			Audio:    AudioConfig{Volume: 999},
			Window:   WindowConfig{Width: 0, Height: 0},
			Library:  LibraryView{ViewMode: "grid", SortBy: "date"},
			Rewind:   RewindConfig{BufferSizeMB: 0, FrameStep: 0},
		}

		corrected := CorrectConfig(config, validTestThemes)
		defaults := DefaultConfig()

		if corrected.Version != defaults.Version {
			t.Errorf("version: got %d, want %d", corrected.Version, defaults.Version)
		}
		if corrected.Theme != defaults.Theme {
			t.Errorf("theme: got %q, want %q", corrected.Theme, defaults.Theme)
		}
		if corrected.FontSize != defaults.FontSize {
			t.Errorf("fontSize: got %d, want %d", corrected.FontSize, defaults.FontSize)
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
		config := DefaultConfig()
		errs := ValidateInputConfig(config, validKey, validPad)
		if len(errs) != 0 {
			t.Errorf("expected no errors, got: %v", errs)
		}
	})

	t.Run("valid overrides have no errors", func(t *testing.T) {
		config := DefaultConfig()
		config.Input.P1Keyboard = map[string]string{"Up": "ArrowUp", "A": "J"}
		config.Input.P1Controller = map[string]string{"A": "DpadUp"}
		errs := ValidateInputConfig(config, validKey, validPad)
		if len(errs) != 0 {
			t.Errorf("expected no errors, got: %v", errs)
		}
	})

	t.Run("invalid key name detected", func(t *testing.T) {
		config := DefaultConfig()
		config.Input.P1Keyboard = map[string]string{"A": "BadKey"}
		errs := ValidateInputConfig(config, validKey, validPad)
		if len(errs) != 1 {
			t.Errorf("expected 1 error, got %d: %v", len(errs), errs)
		}
	})

	t.Run("invalid pad name detected", func(t *testing.T) {
		config := DefaultConfig()
		config.Input.P1Controller = map[string]string{"A": "BadPad"}
		errs := ValidateInputConfig(config, validKey, validPad)
		if len(errs) != 1 {
			t.Errorf("expected 1 error, got %d: %v", len(errs), errs)
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
		config := DefaultConfig()
		config.Input.P1Keyboard = map[string]string{"A": "J", "B": "BadKey"}
		config.Input.P1Controller = map[string]string{"A": "A", "B": "BadPad"}

		CorrectInputConfig(config, validKey, validPad)

		if len(config.Input.P1Keyboard) != 1 {
			t.Errorf("expected 1 keyboard entry, got %d", len(config.Input.P1Keyboard))
		}
		if config.Input.P1Keyboard["A"] != "J" {
			t.Error("valid keyboard entry should be preserved")
		}
		if len(config.Input.P1Controller) != 1 {
			t.Errorf("expected 1 controller entry, got %d", len(config.Input.P1Controller))
		}
		if config.Input.P1Controller["A"] != "A" {
			t.Error("valid controller entry should be preserved")
		}
	})

	t.Run("nil maps are safe", func(t *testing.T) {
		config := DefaultConfig()
		CorrectInputConfig(config, validKey, validPad) // Should not panic
	})
}

func TestInputConfigSerialization(t *testing.T) {
	t.Run("empty maps omitted from JSON", func(t *testing.T) {
		config := DefaultConfig()
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
		config := DefaultConfig()
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
		config := DefaultConfig()
		config.Input.P1Keyboard = map[string]string{"Up": "ArrowUp", "A": "Z"}
		config.Input.P1Controller = map[string]string{"A": "Y"}
		config.Input.CoreOptions = map[string]string{"sixbutton": "true"}

		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}

		var restored Config
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}

		if restored.Input.P1Keyboard["Up"] != "ArrowUp" {
			t.Error("P1Keyboard Up override not preserved")
		}
		if restored.Input.P1Keyboard["A"] != "Z" {
			t.Error("P1Keyboard A override not preserved")
		}
		if restored.Input.P1Controller["A"] != "Y" {
			t.Error("P1Controller A override not preserved")
		}
		if restored.Input.CoreOptions["sixbutton"] != "true" {
			t.Error("CoreOptions not preserved")
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

func TestDisableAnalogStickSerialization(t *testing.T) {
	t.Run("omitted when false", func(t *testing.T) {
		config := DefaultConfig()
		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		s := string(data)
		if contains(s, "disableAnalogStick") {
			t.Error("disableAnalogStick should be omitted when false")
		}
	})

	t.Run("included when true", func(t *testing.T) {
		config := DefaultConfig()
		config.Input.DisableAnalogStick = true
		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		s := string(data)
		if !contains(s, "disableAnalogStick") {
			t.Error("disableAnalogStick should be in JSON when true")
		}
	})

	t.Run("roundtrip preserves true", func(t *testing.T) {
		config := DefaultConfig()
		config.Input.DisableAnalogStick = true

		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}

		var restored Config
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}

		if !restored.Input.DisableAnalogStick {
			t.Error("DisableAnalogStick should be true after roundtrip")
		}
	})

	t.Run("absent defaults to false", func(t *testing.T) {
		jsonBytes := []byte(`{"input": {}}`)
		var config Config
		if err := json.Unmarshal(jsonBytes, &config); err != nil {
			t.Fatal(err)
		}
		if config.Input.DisableAnalogStick {
			t.Error("DisableAnalogStick should default to false when absent")
		}
	})
}

func TestPresentButZeroVsMissing(t *testing.T) {
	// fontSize: 0 is present but invalid — should NOT get defaulted by ApplyMissingDefaults
	// absent fontSize should get defaulted
	t.Run("present fontSize=0 stays zero after ApplyMissingDefaults", func(t *testing.T) {
		jsonBytes := []byte(`{"fontSize": 0}`)
		config := &Config{}
		json.Unmarshal(jsonBytes, config)

		presentKeys := detectPresentKeys(jsonBytes)
		ApplyMissingDefaults(config, presentKeys)

		// fontSize was present (as 0), so ApplyMissingDefaults should NOT override it
		if config.FontSize != 0 {
			t.Errorf("fontSize should remain 0 (present-but-zero), got %d", config.FontSize)
		}

		// But ValidateConfig should catch it as invalid
		errs := ValidateConfig(config, validTestThemes)
		found := false
		for _, e := range errs {
			if len(e) > 8 && e[:8] == "fontSize" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected fontSize validation error for value 0")
		}
	})

	t.Run("absent fontSize gets defaulted to 14", func(t *testing.T) {
		jsonBytes := []byte(`{"version": 1, "theme": "Default"}`)
		config := &Config{}
		json.Unmarshal(jsonBytes, config)

		presentKeys := detectPresentKeys(jsonBytes)
		ApplyMissingDefaults(config, presentKeys)

		if config.FontSize != 14 {
			t.Errorf("absent fontSize should default to 14, got %d", config.FontSize)
		}
	})

	t.Run("present volume=0 preserved", func(t *testing.T) {
		jsonBytes := []byte(`{"audio": {"volume": 0}}`)
		config := &Config{}
		json.Unmarshal(jsonBytes, config)

		presentKeys := detectPresentKeys(jsonBytes)
		ApplyMissingDefaults(config, presentKeys)

		if config.Audio.Volume != 0.0 {
			t.Errorf("present volume=0 should be preserved, got %f", config.Audio.Volume)
		}

		// volume=0 is valid (within 0.0-2.0 range)
		errs := ValidateConfig(config, validTestThemes)
		for _, e := range errs {
			if len(e) > 12 && e[:12] == "audio.volume" {
				t.Errorf("volume 0.0 should be valid, got error: %s", e)
			}
		}
	})
}
