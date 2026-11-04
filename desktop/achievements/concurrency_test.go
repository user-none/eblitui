package achievements

import (
	"sync"
	"testing"

	"github.com/user-none/go-rcheevos"
)

// fakeMemory is emulator memory that returns its fill byte for every
// address.
type fakeMemory struct {
	fill byte
}

func (f fakeMemory) ReadMemoryFlat(off uint32, buf []byte) uint32 {
	for i := range buf {
		buf[i] = f.fill
	}
	return uint32(len(buf))
}

// TestGetCachedAchievementsIsCopy tests that the returned list and the
// achievements in it are copies: an unlock recorded afterwards changes
// neither, and a change to the copy does not reach the cache.
func TestGetCachedAchievementsIsCopy(t *testing.T) {
	m := &Manager{cachedAchievements: []*rcheevos.Achievement{
		{ID: 1, Title: "one"},
		{ID: 2, Title: "two"},
	}}

	list := m.GetCachedAchievements()
	m.mu.Lock()
	m.updateCachedAchievementUnlocked(1)
	m.mu.Unlock()

	if list[0].ID != 1 || list[0].Unlocked != 0 {
		t.Fatalf("copy changed by unlock: %+v", list[0])
	}
	list[1].Title = "changed"
	if got := m.GetCachedAchievements(); got[0].ID != 2 || got[0].Title != "two" ||
		got[1].ID != 1 || got[1].Unlocked != rcheevos.AchievementUnlockedSoftcore {
		t.Fatalf("cache = %+v, %+v", got[0], got[1])
	}
}

// TestGetCachedAchievementsNoGame tests that no cached list returns nil.
func TestGetCachedAchievementsNoGame(t *testing.T) {
	m := &Manager{}
	if got := m.GetCachedAchievements(); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

// TestCachedAchievementsConcurrentUnlock tests that reading the list while
// unlocks are recorded on another goroutine is safe. Run with -race.
func TestCachedAchievementsConcurrentUnlock(t *testing.T) {
	const count = 50
	m := &Manager{}
	for i := range count {
		m.cachedAchievements = append(m.cachedAchievements, &rcheevos.Achievement{ID: uint32(i)})
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range count {
			m.mu.Lock()
			m.updateCachedAchievementUnlocked(uint32(i))
			m.mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		for range count {
			for _, ach := range m.GetCachedAchievements() {
				_ = ach.Unlocked
			}
		}
	}()
	wg.Wait()

	for _, ach := range m.GetCachedAchievements() {
		if ach.Unlocked != rcheevos.AchievementUnlockedSoftcore {
			t.Fatalf("achievement %d not unlocked", ach.ID)
		}
	}
}

// TestReadMemoryEmulatorChanges tests that memory reads while the emulator
// is set and cleared on another goroutine are safe, and read either the
// emulator's memory or nothing. Run with -race.
func TestReadMemoryEmulatorChanges(t *testing.T) {
	m := &Manager{}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 200 {
			m.SetEmulator(fakeMemory{fill: 0xAB})
			m.UnloadGame()
		}
	}()
	go func() {
		defer wg.Done()
		buf := make([]byte, 4)
		for range 200 {
			n := m.readMemory(0, buf)
			if n != 0 && (n != 4 || buf[0] != 0xAB) {
				t.Errorf("read %d bytes %x", n, buf)
				return
			}
		}
	}()
	wg.Wait()

	if n := m.readMemory(0, make([]byte, 4)); n != 0 {
		t.Fatalf("read %d bytes after unload, want 0", n)
	}
}

// TestReadMemoryBigEndian tests that a big-endian core's memory is read
// from address^1, one byte at a time.
func TestReadMemoryBigEndian(t *testing.T) {
	m := &Manager{bigEndian: true}
	m.SetEmulator(fakeMemory{fill: 0x5A})
	buf := make([]byte, 3)
	if n := m.readMemory(0, buf); n != 3 || buf[0] != 0x5A || buf[2] != 0x5A {
		t.Fatalf("read %d bytes %x", n, buf)
	}
}
