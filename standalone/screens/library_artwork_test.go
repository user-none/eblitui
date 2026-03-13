package screens

import (
	"testing"
	"time"
)

func TestArtworkLoaderNewHasEmptyCache(t *testing.T) {
	loader := newArtworkLoader(nil, nil)
	if loader.cache == nil {
		t.Fatal("cache should be initialized")
	}
	if len(loader.cache) != 0 {
		t.Errorf("cache should be empty, got %d entries", len(loader.cache))
	}
}

func TestArtworkLoaderGetReturnsNilOnMiss(t *testing.T) {
	loader := newArtworkLoader(nil, nil)
	if got := loader.Get("nonexistent"); got != nil {
		t.Error("Get should return nil for missing CRC")
	}
}

func TestArtworkLoaderGetReturnsCachedEntry(t *testing.T) {
	loader := newArtworkLoader(nil, nil)
	art := &iconArtwork{}
	loader.mu.Lock()
	loader.cache["abc123"] = art
	loader.mu.Unlock()

	got := loader.Get("abc123")
	if got != art {
		t.Error("Get should return the cached entry")
	}
}

func TestArtworkLoaderCancelAndClearResetsState(t *testing.T) {
	loader := newArtworkLoader(nil, nil)

	// Manually populate some state
	loader.mu.Lock()
	loader.cache["test"] = &iconArtwork{}
	loader.mu.Unlock()
	loader.cardWidth = 100
	loader.artHeight = 75

	loader.CancelAndClear()

	if loader.cardWidth != 0 {
		t.Errorf("cardWidth should be 0, got %d", loader.cardWidth)
	}
	if loader.artHeight != 0 {
		t.Errorf("artHeight should be 0, got %d", loader.artHeight)
	}
	if len(loader.cache) != 0 {
		t.Errorf("cache should be empty after clear, got %d entries", len(loader.cache))
	}
	if loader.cancel != nil {
		t.Error("cancel should be nil after clear")
	}
	if loader.done != nil {
		t.Error("done should be nil after clear")
	}
}

func TestArtworkLoaderStartSetsChannels(t *testing.T) {
	loader := newArtworkLoader(nil, nil)

	// Start with no games - goroutine completes immediately
	loader.Start(nil, 100, 75)

	// Wait for goroutine to finish
	if loader.done != nil {
		<-loader.done
	}

	if loader.cardWidth != 100 {
		t.Errorf("cardWidth should be 100, got %d", loader.cardWidth)
	}
	if loader.artHeight != 75 {
		t.Errorf("artHeight should be 75, got %d", loader.artHeight)
	}
}

func TestArtworkLoaderStartNoDuplicateWithSameDimensions(t *testing.T) {
	loader := newArtworkLoader(nil, nil)

	// First Start
	loader.Start(nil, 100, 75)
	// Wait for completion
	if loader.done != nil {
		<-loader.done
	}

	// Populate cancel so the no-op check passes (simulate still-running state
	// by setting cancel to a non-nil channel - note: done is closed, but
	// Start checks cancel != nil)
	loader.cancel = make(chan struct{})

	// Second Start with same dimensions should be a no-op
	loader.Start(nil, 100, 75)

	if loader.cardWidth != 100 {
		t.Errorf("cardWidth should still be 100, got %d", loader.cardWidth)
	}
}

func TestArtworkLoaderStartCancelsPrevious(t *testing.T) {
	loader := newArtworkLoader(nil, nil)

	// Start with one set of dimensions
	loader.Start(nil, 100, 75)
	if loader.done != nil {
		<-loader.done
	}

	// Start again with different dimensions - should cancel and restart
	loader.Start(nil, 200, 150)
	if loader.done != nil {
		<-loader.done
	}

	if loader.cardWidth != 200 {
		t.Errorf("cardWidth should be 200, got %d", loader.cardWidth)
	}
	if loader.artHeight != 150 {
		t.Errorf("artHeight should be 150, got %d", loader.artHeight)
	}
}

func TestArtworkLoaderCancelStopsGoroutine(t *testing.T) {
	crcs := make([]string, 1000)
	for i := range crcs {
		crcs[i] = "fakecrc"
	}

	loader := newArtworkLoader(nil, nil)
	loader.Start(crcs, 100, 75)

	// Cancel immediately
	loader.CancelAndClear()

	if loader.cancel != nil {
		t.Error("cancel should be nil after CancelAndClear")
	}
}

func TestArtworkLoaderLoadOneStoresMissingForBadFiles(t *testing.T) {
	loader := newArtworkLoader(nil, nil)

	// Pre-populate the "missing" entry so storeMissing has something to copy
	missing := &iconArtwork{}
	loader.mu.Lock()
	loader.cache["missing"] = missing
	loader.mu.Unlock()

	cancel := make(chan struct{})
	loader.loadOne("nonexistent_crc", 100, 75, cancel)

	got := loader.Get("nonexistent_crc")
	if got != missing {
		t.Error("loadOne should store the missing-art entry for files that don't exist")
	}
}

func TestArtworkLoaderCancelAndClearIdempotent(t *testing.T) {
	loader := newArtworkLoader(nil, nil)

	// Calling CancelAndClear multiple times should not panic
	loader.CancelAndClear()
	loader.CancelAndClear()
	loader.CancelAndClear()
}

func TestArtworkLoaderHaltPreventsStart(t *testing.T) {
	loader := newArtworkLoader(nil, nil)

	loader.Halt()

	if !loader.halted {
		t.Error("halted should be true after Halt")
	}

	// Start after Halt should be a no-op
	loader.Start(nil, 100, 75)

	if loader.cancel != nil {
		t.Error("cancel should be nil - Start should not launch after Halt")
	}
}

func TestArtworkLoaderHaveNew(t *testing.T) {
	loader := newArtworkLoader(nil, nil)

	// Initially false
	if loader.HaveNew() {
		t.Error("HaveNew should be false initially")
	}

	// Set via hasNew directly (simulating loadOne)
	loader.hasNew.Store(true)

	// First call returns true and clears
	if !loader.HaveNew() {
		t.Error("HaveNew should return true after store")
	}

	// Second call returns false
	if loader.HaveNew() {
		t.Error("HaveNew should return false after being read")
	}
}

func TestArtworkLoaderHaveNewClearedByHalt(t *testing.T) {
	loader := newArtworkLoader(nil, nil)
	loader.hasNew.Store(true)

	loader.Halt()

	if loader.HaveNew() {
		t.Error("HaveNew should be false after Halt")
	}
}

func TestArtworkLoaderHaveNewClearedByCancelAndClear(t *testing.T) {
	loader := newArtworkLoader(nil, nil)
	loader.hasNew.Store(true)

	loader.CancelAndClear()

	if loader.HaveNew() {
		t.Error("HaveNew should be false after CancelAndClear")
	}
}

func TestArtworkLoaderConcurrentGet(t *testing.T) {
	loader := newArtworkLoader(nil, nil)

	// Pre-populate cache
	art := &iconArtwork{}
	loader.mu.Lock()
	loader.cache["test"] = art
	loader.mu.Unlock()

	// Concurrent reads should not race
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				_ = loader.Get("test")
			}
		}()
	}

	timeout := time.After(2 * time.Second)
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("concurrent Get timed out")
		}
	}
}
