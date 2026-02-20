//go:build !libretro

package style

import (
	"testing"
)

func TestGetThemeByName(t *testing.T) {
	tests := []struct {
		name         string
		expectedName string
	}{
		{"Default", "Default"},
		{"Dark", "Dark"},
		{"Light", "Light"},
		{"Retro", "Retro"},
		{"Pink", "Pink"},
		{"Hot Pink", "Hot Pink"},
		{"Green LCD", "Green LCD"},
		{"High Contrast", "High Contrast"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			theme := GetThemeByName(tc.name)
			if theme.Name != tc.expectedName {
				t.Errorf("GetThemeByName(%q).Name = %q, want %q", tc.name, theme.Name, tc.expectedName)
			}
		})
	}

	t.Run("unknown returns Default", func(t *testing.T) {
		theme := GetThemeByName("Nonexistent")
		if theme.Name != "Default" {
			t.Errorf("GetThemeByName(\"Nonexistent\").Name = %q, want \"Default\"", theme.Name)
		}
	})

	t.Run("empty returns Default", func(t *testing.T) {
		theme := GetThemeByName("")
		if theme.Name != "Default" {
			t.Errorf("GetThemeByName(\"\").Name = %q, want \"Default\"", theme.Name)
		}
	})
}

func TestIsValidThemeName(t *testing.T) {
	validNames := []string{"Default", "Dark", "Light", "Retro", "Pink", "Hot Pink", "Green LCD", "High Contrast"}
	for _, name := range validNames {
		t.Run("valid_"+name, func(t *testing.T) {
			if !IsValidThemeName(name) {
				t.Errorf("IsValidThemeName(%q) = false, want true", name)
			}
		})
	}

	invalidNames := []string{"", "Nonexistent", "default", "DARK", "light"}
	for _, name := range invalidNames {
		t.Run("invalid_"+name, func(t *testing.T) {
			if IsValidThemeName(name) {
				t.Errorf("IsValidThemeName(%q) = true, want false", name)
			}
		})
	}
}

func TestApplyTheme(t *testing.T) {
	// Save original state to restore after test
	origBg := Background
	origName := CurrentThemeName
	defer func() {
		Background = origBg
		CurrentThemeName = origName
	}()

	ApplyTheme(ThemeDark)

	if Background != ThemeDark.Background {
		t.Errorf("Background not updated after ApplyTheme")
	}
	if Surface != ThemeDark.Surface {
		t.Errorf("Surface not updated after ApplyTheme")
	}
	if Primary != ThemeDark.Primary {
		t.Errorf("Primary not updated after ApplyTheme")
	}
	if PrimaryHover != ThemeDark.PrimaryHover {
		t.Errorf("PrimaryHover not updated after ApplyTheme")
	}
	if Text != ThemeDark.Text {
		t.Errorf("Text not updated after ApplyTheme")
	}
	if TextSecondary != ThemeDark.TextSecondary {
		t.Errorf("TextSecondary not updated after ApplyTheme")
	}
	if Accent != ThemeDark.Accent {
		t.Errorf("Accent not updated after ApplyTheme")
	}
	if Border != ThemeDark.Border {
		t.Errorf("Border not updated after ApplyTheme")
	}
	if Black != ThemeDark.Black {
		t.Errorf("Black not updated after ApplyTheme")
	}
	if DimOverlay != ThemeDark.DimOverlay {
		t.Errorf("DimOverlay not updated after ApplyTheme")
	}
	if OverlayBackground != ThemeDark.OverlayBackground {
		t.Errorf("OverlayBackground not updated after ApplyTheme")
	}
	if CurrentThemeName != "Dark" {
		t.Errorf("CurrentThemeName = %q, want \"Dark\"", CurrentThemeName)
	}
}

func TestApplyThemeByName(t *testing.T) {
	origName := CurrentThemeName
	defer func() {
		ApplyThemeByName(origName)
	}()

	ApplyThemeByName("Light")
	if CurrentThemeName != "Light" {
		t.Errorf("CurrentThemeName = %q, want \"Light\"", CurrentThemeName)
	}
	if Background != ThemeLight.Background {
		t.Errorf("Background not updated for Light theme")
	}

	// Unknown name falls back to Default
	ApplyThemeByName("DoesNotExist")
	if CurrentThemeName != "Default" {
		t.Errorf("CurrentThemeName = %q, want \"Default\" for unknown theme", CurrentThemeName)
	}
}

func TestAvailableThemesCompleteness(t *testing.T) {
	// All defined themes should be in AvailableThemes
	definedThemes := []Theme{ThemeDefault, ThemeDark, ThemeLight, ThemeRetro, ThemePink, ThemeHotPink, ThemeGreenLCD, ThemeHighContrast}

	if len(AvailableThemes) != len(definedThemes) {
		t.Errorf("AvailableThemes has %d themes, expected %d", len(AvailableThemes), len(definedThemes))
	}

	for _, dt := range definedThemes {
		found := false
		for _, at := range AvailableThemes {
			if at.Name == dt.Name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("theme %q not found in AvailableThemes", dt.Name)
		}
	}
}

func TestAvailableThemesNoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, theme := range AvailableThemes {
		if seen[theme.Name] {
			t.Errorf("duplicate theme name in AvailableThemes: %q", theme.Name)
		}
		seen[theme.Name] = true
	}
}

func TestThemeColorsNonZeroAlpha(t *testing.T) {
	// All theme colors should have full alpha (0xff) to be visible
	for _, theme := range AvailableThemes {
		t.Run(theme.Name, func(t *testing.T) {
			colors := map[string]uint8{
				"Background":        theme.Background.A,
				"Surface":           theme.Surface.A,
				"Primary":           theme.Primary.A,
				"PrimaryHover":      theme.PrimaryHover.A,
				"Text":              theme.Text.A,
				"TextSecondary":     theme.TextSecondary.A,
				"Accent":            theme.Accent.A,
				"Border":            theme.Border.A,
				"Black":             theme.Black.A,
				"DimOverlay":        theme.DimOverlay.A,
				"OverlayBackground": theme.OverlayBackground.A,
			}
			for name, alpha := range colors {
				if alpha != 0xff {
					t.Errorf("%s.%s alpha = 0x%02x, want 0xff", theme.Name, name, alpha)
				}
			}
		})
	}
}
