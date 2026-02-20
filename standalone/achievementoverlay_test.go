//go:build !libretro

package standalone

import (
	"testing"

	"github.com/user-none/go-rcheevos"
)

func TestNewAchievementOverlay(t *testing.T) {
	o := NewAchievementOverlay(nil)

	if o.IsVisible() {
		t.Error("should not be visible initially")
	}
}

func TestAchievementOverlayShowHide(t *testing.T) {
	o := NewAchievementOverlay(nil)

	o.Show()
	if !o.IsVisible() {
		t.Error("should be visible after Show()")
	}

	o.Hide()
	if o.IsVisible() {
		t.Error("should not be visible after Hide()")
	}
}

func TestAchievementOverlayReset(t *testing.T) {
	o := NewAchievementOverlay(nil)
	o.scrollOffset = 5.0
	o.badgesPending[1] = true

	o.Reset()

	if o.scrollOffset != 0 {
		t.Errorf("scrollOffset should be 0 after Reset, got %f", o.scrollOffset)
	}
	if len(o.badgesPending) != 0 {
		t.Errorf("badgesPending should be empty after Reset, got %d entries", len(o.badgesPending))
	}
	if len(o.grayscaleBadges) != 0 {
		t.Errorf("grayscaleBadges should be empty after Reset, got %d entries", len(o.grayscaleBadges))
	}
}

func TestComputeSummaryEmpty(t *testing.T) {
	o := NewAchievementOverlay(nil)

	numTotal, numUnlocked, pointsTotal, pointsUnlocked := o.computeSummary(nil)

	if numTotal != 0 || numUnlocked != 0 || pointsTotal != 0 || pointsUnlocked != 0 {
		t.Errorf("empty achievements should return all zeros, got total=%d unlocked=%d ptsTotal=%d ptsUnlocked=%d",
			numTotal, numUnlocked, pointsTotal, pointsUnlocked)
	}
}

func TestComputeSummaryAllUnlocked(t *testing.T) {
	o := NewAchievementOverlay(nil)

	achievements := []*rcheevos.Achievement{
		{ID: 1, Points: 10, Unlocked: rcheevos.AchievementUnlockedSoftcore},
		{ID: 2, Points: 25, Unlocked: rcheevos.AchievementUnlockedHardcore},
		{ID: 3, Points: 50, Unlocked: rcheevos.AchievementUnlockedBoth},
	}

	numTotal, numUnlocked, pointsTotal, pointsUnlocked := o.computeSummary(achievements)

	if numTotal != 3 {
		t.Errorf("numTotal = %d, want 3", numTotal)
	}
	if numUnlocked != 3 {
		t.Errorf("numUnlocked = %d, want 3", numUnlocked)
	}
	if pointsTotal != 85 {
		t.Errorf("pointsTotal = %d, want 85", pointsTotal)
	}
	if pointsUnlocked != 85 {
		t.Errorf("pointsUnlocked = %d, want 85", pointsUnlocked)
	}
}

func TestComputeSummaryMixed(t *testing.T) {
	o := NewAchievementOverlay(nil)

	achievements := []*rcheevos.Achievement{
		{ID: 1, Points: 10, Unlocked: rcheevos.AchievementUnlockedSoftcore},
		{ID: 2, Points: 25, Unlocked: rcheevos.AchievementUnlockedNone},
		{ID: 3, Points: 50, Unlocked: rcheevos.AchievementUnlockedHardcore},
		{ID: 4, Points: 15, Unlocked: rcheevos.AchievementUnlockedNone},
	}

	numTotal, numUnlocked, pointsTotal, pointsUnlocked := o.computeSummary(achievements)

	if numTotal != 4 {
		t.Errorf("numTotal = %d, want 4", numTotal)
	}
	if numUnlocked != 2 {
		t.Errorf("numUnlocked = %d, want 2", numUnlocked)
	}
	if pointsTotal != 100 {
		t.Errorf("pointsTotal = %d, want 100", pointsTotal)
	}
	if pointsUnlocked != 60 {
		t.Errorf("pointsUnlocked = %d, want 60", pointsUnlocked)
	}
}

func TestComputeSummaryAllLocked(t *testing.T) {
	o := NewAchievementOverlay(nil)

	achievements := []*rcheevos.Achievement{
		{ID: 1, Points: 10, Unlocked: rcheevos.AchievementUnlockedNone},
		{ID: 2, Points: 25, Unlocked: rcheevos.AchievementUnlockedNone},
	}

	numTotal, numUnlocked, pointsTotal, pointsUnlocked := o.computeSummary(achievements)

	if numTotal != 2 {
		t.Errorf("numTotal = %d, want 2", numTotal)
	}
	if numUnlocked != 0 {
		t.Errorf("numUnlocked = %d, want 0", numUnlocked)
	}
	if pointsTotal != 35 {
		t.Errorf("pointsTotal = %d, want 35", pointsTotal)
	}
	if pointsUnlocked != 0 {
		t.Errorf("pointsUnlocked = %d, want 0", pointsUnlocked)
	}
}

func TestGetAchievementsNilManager(t *testing.T) {
	o := NewAchievementOverlay(nil)

	achievements := o.getAchievements()
	if achievements != nil {
		t.Errorf("expected nil, got %v", achievements)
	}
}

func TestGetGameTitleNilManager(t *testing.T) {
	o := NewAchievementOverlay(nil)

	title := o.getGameTitle()
	if title != "" {
		t.Errorf("expected empty string, got %q", title)
	}
}

func TestInitForGameResetsState(t *testing.T) {
	o := NewAchievementOverlay(nil)
	o.scrollOffset = 10.0
	o.badgesPending[1] = true

	o.InitForGame()

	if o.scrollOffset != 0 {
		t.Errorf("scrollOffset should be 0 after InitForGame, got %f", o.scrollOffset)
	}
	if len(o.badgesPending) != 0 {
		t.Errorf("badgesPending should be empty after InitForGame, got %d", len(o.badgesPending))
	}
	if len(o.grayscaleBadges) != 0 {
		t.Errorf("grayscaleBadges should be empty after InitForGame, got %d", len(o.grayscaleBadges))
	}
}

func TestHandleUnlock(t *testing.T) {
	o := NewAchievementOverlay(nil)

	// Simulate a cached grayscale badge
	o.grayscaleBadges[42] = nil // value doesn't matter for this test

	o.handleUnlock(42)

	if _, exists := o.grayscaleBadges[42]; exists {
		t.Error("grayscale badge should be removed after unlock")
	}
}
