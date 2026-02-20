//go:build !libretro

package style

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

func TestTruncateStart(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		maxLen      int
		expected    string
		shouldTrunc bool
	}{
		{"shorter than max", "hello", 10, "hello", false},
		{"exact length", "hello", 5, "hello", false},
		{"truncated with ellipsis", "/Users/john/very/long/path/to/file.sms", 20, ".../path/to/file.sms", true},
		{"maxLen 3", "abcdef", 3, "def", true},
		{"maxLen 2", "abcdef", 2, "ef", true},
		{"maxLen 1", "abcdef", 1, "f", true},
		{"empty string", "", 5, "", false},
		{"single char no trunc", "a", 5, "a", false},
		{"truncate to 4", "abcdef", 4, "...f", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, truncated := TruncateStart(tc.input, tc.maxLen)
			if got != tc.expected {
				t.Errorf("TruncateStart(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.expected)
			}
			if truncated != tc.shouldTrunc {
				t.Errorf("TruncateStart(%q, %d) truncated = %v, want %v", tc.input, tc.maxLen, truncated, tc.shouldTrunc)
			}
		})
	}
}

func TestTruncateEnd(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		maxLen      int
		expected    string
		shouldTrunc bool
	}{
		{"shorter than max", "hello", 10, "hello", false},
		{"exact length", "hello", 5, "hello", false},
		{"truncated with ellipsis", "Sonic the Hedgehog in Very Long Title", 20, "Sonic the Hedgeho...", true},
		{"maxLen 3", "abcdef", 3, "abc", true},
		{"maxLen 2", "abcdef", 2, "ab", true},
		{"maxLen 1", "abcdef", 1, "a", true},
		{"empty string", "", 5, "", false},
		{"single char no trunc", "a", 5, "a", false},
		{"truncate to 4", "abcdef", 4, "a...", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, truncated := TruncateEnd(tc.input, tc.maxLen)
			if got != tc.expected {
				t.Errorf("TruncateEnd(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.expected)
			}
			if truncated != tc.shouldTrunc {
				t.Errorf("TruncateEnd(%q, %d) truncated = %v, want %v", tc.input, tc.maxLen, truncated, tc.shouldTrunc)
			}
		})
	}
}

func TestFormatPlayTime(t *testing.T) {
	tests := []struct {
		name     string
		seconds  int64
		expected string
	}{
		{"zero", 0, "-"},
		{"1 second", 1, "< 1m"},
		{"30 seconds", 30, "< 1m"},
		{"59 seconds", 59, "< 1m"},
		{"exactly 1 minute", 60, "1m"},
		{"5 minutes", 300, "5m"},
		{"59 minutes", 3540, "59m"},
		{"exactly 1 hour", 3600, "1h 0m"},
		{"1 hour 30 minutes", 5400, "1h 30m"},
		{"2 hours 15 minutes", 8100, "2h 15m"},
		{"24 hours", 86400, "24h 0m"},
		{"100 hours 59 minutes", 363540, "100h 59m"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatPlayTime(tc.seconds)
			if got != tc.expected {
				t.Errorf("FormatPlayTime(%d) = %q, want %q", tc.seconds, got, tc.expected)
			}
		})
	}
}

func TestFormatDate(t *testing.T) {
	t.Run("zero timestamp", func(t *testing.T) {
		got := FormatDate(0)
		if got != "Unknown" {
			t.Errorf("FormatDate(0) = %q, want \"Unknown\"", got)
		}
	})

	t.Run("valid timestamp returns date", func(t *testing.T) {
		// Use a mid-day timestamp to avoid timezone boundary issues
		// 1609509600 = 2021-01-01 14:00:00 UTC
		got := FormatDate(1609509600)
		if got == "Unknown" {
			t.Error("FormatDate should not return \"Unknown\" for non-zero timestamp")
		}
		// Should contain year 2021 (or Dec 2020 depending on timezone, but not "Unknown")
		if len(got) < 5 {
			t.Errorf("FormatDate returned unexpectedly short result: %q", got)
		}
	})
}

func TestFormatLastPlayed(t *testing.T) {
	t.Run("zero timestamp", func(t *testing.T) {
		got := FormatLastPlayed(0)
		if got != "Never" {
			t.Errorf("FormatLastPlayed(0) = %q, want %q", got, "Never")
		}
	})

	t.Run("very old timestamp", func(t *testing.T) {
		// 1609459200 = Jan 1, 2021 - should show full date with year
		got := FormatLastPlayed(1609459200)
		if got == "Never" || got == "Today" || got == "Yesterday" {
			t.Errorf("FormatLastPlayed(1609459200) = %q, expected a date with year", got)
		}
	})
}

func TestApplyFontSize(t *testing.T) {
	// Ensure dpiScale is 1.0 for this test
	origDPI := dpiScale
	dpiScale = 1.0
	defer func() {
		dpiScale = origDPI
		ApplyFontSize(14)
	}()

	// Save original values
	origListRowHeight := ListRowHeight
	origAchievementRowHeight := AchievementRowHeight
	origAchievementBadgeSize := AchievementBadgeSize
	origAchievementOverlayWidth := AchievementOverlayWidth
	origAchievementOverlayPadding := AchievementOverlayPadding

	// Apply at default 14pt - values should match defaults
	ApplyFontSize(14)
	if ListRowHeight != 40 {
		t.Errorf("at 14pt, ListRowHeight = %d, want 40", ListRowHeight)
	}
	if AchievementRowHeight != 92 {
		t.Errorf("at 14pt, AchievementRowHeight = %d, want 92", AchievementRowHeight)
	}

	// Apply at 28pt (2x scale)
	ApplyFontSize(28)
	if ListRowHeight != 80 {
		t.Errorf("at 28pt, ListRowHeight = %d, want 80", ListRowHeight)
	}
	if ListHeaderHeight != 76 {
		t.Errorf("at 28pt, ListHeaderHeight = %d, want 76", ListHeaderHeight)
	}
	if AchievementRowHeight != 138 {
		t.Errorf("at 28pt, AchievementRowHeight = %d, want 138", AchievementRowHeight)
	}
	if AchievementBadgeSize != 84 {
		t.Errorf("at 28pt, AchievementBadgeSize = %d, want 84", AchievementBadgeSize)
	}
	if AchievementOverlayWidth != 1000 {
		t.Errorf("at 28pt, AchievementOverlayWidth = %d, want 1000", AchievementOverlayWidth)
	}
	if AchievementOverlayPadding != 32 {
		t.Errorf("at 28pt, AchievementOverlayPadding = %d, want 32", AchievementOverlayPadding)
	}

	// Apply at 10pt (scale = 10/14 ≈ 0.714)
	ApplyFontSize(10)
	// 40 * 10 / 14 = 28.57 -> int truncates to 28
	if ListRowHeight != 28 {
		t.Errorf("at 10pt, ListRowHeight = %d, want 28", ListRowHeight)
	}

	// Restore to 14
	ApplyFontSize(14)
	if ListRowHeight != origListRowHeight {
		t.Errorf("after restore, ListRowHeight = %d, want %d", ListRowHeight, origListRowHeight)
	}
	if AchievementRowHeight != origAchievementRowHeight {
		t.Errorf("after restore, AchievementRowHeight = %d, want %d", AchievementRowHeight, origAchievementRowHeight)
	}
	if AchievementBadgeSize != origAchievementBadgeSize {
		t.Errorf("after restore, AchievementBadgeSize = %d, want %d", AchievementBadgeSize, origAchievementBadgeSize)
	}
	if AchievementOverlayWidth != origAchievementOverlayWidth {
		t.Errorf("after restore, AchievementOverlayWidth = %d, want %d", AchievementOverlayWidth, origAchievementOverlayWidth)
	}
	if AchievementOverlayPadding != origAchievementOverlayPadding {
		t.Errorf("after restore, AchievementOverlayPadding = %d, want %d", AchievementOverlayPadding, origAchievementOverlayPadding)
	}
}

func TestFontScale(t *testing.T) {
	defer ApplyFontSize(14) // Restore

	ApplyFontSize(14)
	if FontScale() != 1.0 {
		t.Errorf("at 14pt, FontScale() = %f, want 1.0", FontScale())
	}

	ApplyFontSize(28)
	if FontScale() != 2.0 {
		t.Errorf("at 28pt, FontScale() = %f, want 2.0", FontScale())
	}

	ApplyFontSize(7)
	if FontScale() != 0.5 {
		t.Errorf("at 7pt, FontScale() = %f, want 0.5", FontScale())
	}
}

func TestTruncateToWidth(t *testing.T) {
	// Initialize font face for testing
	face := FontFace()
	if face == nil || *face == nil {
		t.Fatal("FontFace() returned nil")
	}

	t.Run("string that fits returns unchanged", func(t *testing.T) {
		got, truncated := TruncateToWidth("Hi", *face, 500)
		if truncated {
			t.Errorf("expected no truncation for short string, got truncated=%v result=%q", truncated, got)
		}
		if got != "Hi" {
			t.Errorf("expected %q, got %q", "Hi", got)
		}
	})

	t.Run("long string is truncated with ellipsis", func(t *testing.T) {
		long := "Sonic The Hedgehog (USA, Europe, Brazil) (En,Fr,De,Es,It,Pt)"
		got, truncated := TruncateToWidth(long, *face, 200)
		if !truncated {
			t.Error("expected truncation for long string")
		}
		if len(got) < 4 {
			t.Errorf("truncated result too short: %q", got)
		}
		// Must end with ellipsis
		if got[len(got)-3:] != "..." {
			t.Errorf("expected ellipsis suffix, got %q", got)
		}
		// Must be shorter than original
		if len(got) >= len(long) {
			t.Errorf("truncated result should be shorter than original: %q vs %q", got, long)
		}
		// Verify it actually fits
		w, _ := text.Measure(got, *face, 0)
		if w > 200 {
			t.Errorf("truncated string width %.1f exceeds max 200", w)
		}
	})

	t.Run("empty string returns empty", func(t *testing.T) {
		got, truncated := TruncateToWidth("", *face, 100)
		if truncated {
			t.Error("expected no truncation for empty string")
		}
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("very narrow width returns ellipsis", func(t *testing.T) {
		got, truncated := TruncateToWidth("Hello World", *face, 5)
		if !truncated {
			t.Error("expected truncation for very narrow width")
		}
		if got != "..." {
			t.Errorf("expected %q for very narrow width, got %q", "...", got)
		}
	})
}

func TestPx(t *testing.T) {
	origDPI := dpiScale
	defer func() { dpiScale = origDPI }()

	dpiScale = 1.0
	if got := Px(10); got != 10 {
		t.Errorf("Px(10) at scale 1.0 = %d, want 10", got)
	}

	dpiScale = 2.0
	if got := Px(10); got != 20 {
		t.Errorf("Px(10) at scale 2.0 = %d, want 20", got)
	}

	dpiScale = 1.5
	if got := Px(10); got != 15 {
		t.Errorf("Px(10) at scale 1.5 = %d, want 15", got)
	}
}

func TestSetDPIScale(t *testing.T) {
	origDPI := dpiScale
	defer func() {
		dpiScale = origDPI
		ApplyFontSize(14)
	}()

	// Set to 2.0x and verify spatial vars double
	SetDPIScale(2.0)

	if DPIScale() != 2.0 {
		t.Errorf("DPIScale() = %f, want 2.0", DPIScale())
	}

	// Non-font-dependent vars should be exactly 2x base
	if DefaultPadding != 32 {
		t.Errorf("DefaultPadding at 2x = %d, want 32", DefaultPadding)
	}
	if SmallSpacing != 16 {
		t.Errorf("SmallSpacing at 2x = %d, want 16", SmallSpacing)
	}
	if ScrollbarWidth != 40 {
		t.Errorf("ScrollbarWidth at 2x = %d, want 40", ScrollbarWidth)
	}
	if IconMinCardWidth != 400 {
		t.Errorf("IconMinCardWidth at 2x = %d, want 400", IconMinCardWidth)
	}
	if AchievementRowSpacing != 8 {
		t.Errorf("AchievementRowSpacing at 2x = %d, want 8", AchievementRowSpacing)
	}

	// Font-dependent vars at 14pt with 2x DPI should be 2x their 1x values
	// ListRowHeight = int(40 * 1.0 * 2.0) = 80
	if ListRowHeight != 80 {
		t.Errorf("ListRowHeight at 14pt/2x = %d, want 80", ListRowHeight)
	}

	// Font face should incorporate DPI scale
	face := FontFace()
	if face != nil && *face != nil {
		goFace, ok := (*face).(*text.GoTextFace)
		if ok && goFace.Size != 28.0 {
			t.Errorf("FontFace size at 14pt/2x = %f, want 28.0", goFace.Size)
		}
	}

	// Restore to 1.0
	SetDPIScale(1.0)
	if DefaultPadding != 16 {
		t.Errorf("DefaultPadding after restore = %d, want 16", DefaultPadding)
	}
	if ListRowHeight != 40 {
		t.Errorf("ListRowHeight after restore = %d, want 40", ListRowHeight)
	}
}

func TestPxFont(t *testing.T) {
	origDPI := dpiScale
	defer func() {
		dpiScale = origDPI
		ApplyFontSize(14)
	}()

	// At 14pt / 1x DPI: FontScale()=1.0, dpiScale=1.0 → PxFont(80)=80
	dpiScale = 1.0
	ApplyFontSize(14)
	if got := PxFont(80); got != 80 {
		t.Errorf("PxFont(80) at 14pt/1x = %d, want 80", got)
	}

	// At 28pt / 1x DPI: FontScale()=2.0, dpiScale=1.0 → PxFont(80)=160
	ApplyFontSize(28)
	if got := PxFont(80); got != 160 {
		t.Errorf("PxFont(80) at 28pt/1x = %d, want 160", got)
	}

	// At 14pt / 2x DPI: FontScale()=1.0, dpiScale=2.0 → PxFont(80)=160
	dpiScale = 2.0
	ApplyFontSize(14)
	if got := PxFont(80); got != 160 {
		t.Errorf("PxFont(80) at 14pt/2x = %d, want 160", got)
	}

	// At 28pt / 2x DPI: FontScale()=2.0, dpiScale=2.0 → PxFont(80)=320
	ApplyFontSize(28)
	if got := PxFont(80); got != 320 {
		t.Errorf("PxFont(80) at 28pt/2x = %d, want 320", got)
	}
}

func TestSetDPIScaleNewVars(t *testing.T) {
	origDPI := dpiScale
	defer func() {
		dpiScale = origDPI
		ApplyFontSize(14)
	}()

	SetDPIScale(2.0)

	if OverlayPadding != 24 {
		t.Errorf("OverlayPadding at 2x = %d, want 24", OverlayPadding)
	}
	if OverlayMargin != 16 {
		t.Errorf("OverlayMargin at 2x = %d, want 16", OverlayMargin)
	}
	if PauseMenuMinWidth != 300 {
		t.Errorf("PauseMenuMinWidth at 2x = %d, want 300", PauseMenuMinWidth)
	}
	if PauseMenuMaxWidth != 700 {
		t.Errorf("PauseMenuMaxWidth at 2x = %d, want 700", PauseMenuMaxWidth)
	}
	if PauseMenuMinBtnHeight != 80 {
		t.Errorf("PauseMenuMinBtnHeight at 2x = %d, want 80", PauseMenuMinBtnHeight)
	}
	if PauseMenuMaxBtnHeight != 120 {
		t.Errorf("PauseMenuMaxBtnHeight at 2x = %d, want 120", PauseMenuMaxBtnHeight)
	}
	if AchievementPanelMargin != 80 {
		t.Errorf("AchievementPanelMargin at 2x = %d, want 80", AchievementPanelMargin)
	}
	if AchievementMinPanelHeight != 400 {
		t.Errorf("AchievementMinPanelHeight at 2x = %d, want 400", AchievementMinPanelHeight)
	}
	if AchievementNotifyMargin != 40 {
		t.Errorf("AchievementNotifyMargin at 2x = %d, want 40", AchievementNotifyMargin)
	}
	if AchievementNotifyBadgeSize != 128 {
		t.Errorf("AchievementNotifyBadgeSize at 2x = %d, want 128", AchievementNotifyBadgeSize)
	}
	if ListMinTitleWidth != 300 {
		t.Errorf("ListMinTitleWidth at 2x = %d, want 300", ListMinTitleWidth)
	}

	// Restore and verify
	SetDPIScale(1.0)
	if OverlayPadding != 12 {
		t.Errorf("OverlayPadding after restore = %d, want 12", OverlayPadding)
	}
	if PauseMenuMinWidth != 150 {
		t.Errorf("PauseMenuMinWidth after restore = %d, want 150", PauseMenuMinWidth)
	}
}

func TestSetDPIScaleClampsBelowOne(t *testing.T) {
	origDPI := dpiScale
	defer func() {
		dpiScale = origDPI
		ApplyFontSize(14)
	}()

	SetDPIScale(0.5) // Should be clamped to 1.0
	if DPIScale() != 1.0 {
		t.Errorf("DPIScale() after setting 0.5 = %f, want 1.0", DPIScale())
	}
}
