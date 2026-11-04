package storage

import (
	"fmt"
	"sync"
	"testing"
)

// TestConfigConcurrentUse tests that changes, reads and background saves
// from many goroutines at once are safe and none is lost. Run with -race.
func TestConfigConcurrentUse(t *testing.T) {
	useTempDataDir(t)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}

	const writers = 8
	const changes = 25

	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range changes {
				cfg.SetCoreOption(fmt.Sprintf("w%d-%d", w, i), "on")
				cfg.SetKeyboardBinding(fmt.Sprintf("b%d-%d", w, i), "KeyA")
				cfg.SetBIOSFile("bios", fmt.Sprintf("v%d-%d", w, i), "/bios.bin")
				cfg.ToggleMuted()
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range changes {
				v := cfg.Values()
				if v.CoreOptions != nil {
					v.CoreOptions["x"] = "changed"
				}
				cfg.Input()
				cfg.Shaders()
				cfg.RetroAchievements()
			}
		}()
	}
	wg.Wait()
	cfg.Flush()

	reloaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	v := reloaded.Values()
	want := writers * changes
	if len(v.CoreOptions) != want || len(v.Input.P1Keyboard) != want || len(v.BIOS["bios"].Files) != want {
		t.Fatalf("reloaded %d core options, %d bindings, %d BIOS files; want %d each",
			len(v.CoreOptions), len(v.Input.P1Keyboard), len(v.BIOS["bios"].Files), want)
	}
	if _, ok := v.CoreOptions["x"]; ok {
		t.Fatal("a change to a Values copy reached the configuration")
	}
	if v.Audio.Muted != (want%2 == 1) {
		t.Fatalf("muted = %v after %d toggles", v.Audio.Muted, want)
	}
}
