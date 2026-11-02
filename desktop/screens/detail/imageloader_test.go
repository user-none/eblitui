// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package detail

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// testLoader is an ImageLoader whose decodes record their paths and block
// until released through gate, then fail. Load completions are counted.
type testLoader struct {
	*ImageLoader
	mu      sync.Mutex
	decoded []string
	loads   int
	gate    chan struct{}
}

func newTestLoader() *testLoader {
	tl := &testLoader{gate: make(chan struct{}, 16)}
	tl.ImageLoader = NewImageLoader(func() {
		tl.mu.Lock()
		tl.loads++
		tl.mu.Unlock()
	})
	tl.decode = func(path string) (*ebiten.Image, error) {
		<-tl.gate
		tl.mu.Lock()
		tl.decoded = append(tl.decoded, path)
		tl.mu.Unlock()
		return nil, errTestDecode
	}
	return tl
}

// release lets n decodes finish
func (tl *testLoader) release(n int) {
	for range n {
		tl.gate <- struct{}{}
	}
}

func (tl *testLoader) counts() (loads int, decoded []string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	return tl.loads, append([]string(nil), tl.decoded...)
}

// waitLoads waits until n loads have completed
func (tl *testLoader) waitLoads(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if loads, _ := tl.counts(); loads >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("loads did not reach %d", n)
		}
		time.Sleep(time.Millisecond)
	}
}

// files creates the named files in a temp directory and returns their
// paths in order
func files(t *testing.T, names ...string) []string {
	t.Helper()
	dir := t.TempDir()
	var out []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func TestImageLoaderLoadsInBackground(t *testing.T) {
	tl := newTestLoader()
	paths := files(t, "a.webp", "b.webp")
	tl.Start(func() []string { return paths })

	if _, loading := tl.Images(); !loading {
		t.Fatal("not loading after Start")
	}
	tl.release(2)
	tl.waitLoads(t, 1)

	imgs, loading := tl.Images()
	if loading || len(imgs) != 2 {
		t.Fatalf("after load: %d images, loading %v, want 2 and done", len(imgs), loading)
	}
	if _, decoded := tl.counts(); len(decoded) != 2 || decoded[0] != paths[0] || decoded[1] != paths[1] {
		t.Errorf("decoded %v, want %v in order", decoded, paths)
	}
}

func TestImageLoaderSkipsMissingFiles(t *testing.T) {
	tl := newTestLoader()
	present := files(t, "a.webp")[0]
	missing := filepath.Join(filepath.Dir(present), "none.webp")
	tl.Start(func() []string { return []string{missing, present} })
	tl.release(1)
	tl.waitLoads(t, 1)

	imgs, _ := tl.Images()
	if len(imgs) != 2 || imgs[0] != nil {
		t.Errorf("images = %v, want two with the missing one nil", imgs)
	}
	if _, decoded := tl.counts(); len(decoded) != 1 || decoded[0] != present {
		t.Errorf("decoded %v, want only the present file", decoded)
	}
}

func TestImageLoaderStartsOnceUntilRelease(t *testing.T) {
	tl := newTestLoader()
	paths := files(t, "a.webp")
	resolve := func() []string { return paths }
	tl.Start(resolve)
	tl.Start(resolve)
	tl.release(1)
	tl.waitLoads(t, 1)
	tl.Start(resolve)
	time.Sleep(10 * time.Millisecond)
	if _, decoded := tl.counts(); len(decoded) != 1 {
		t.Fatalf("decoded %d times, want 1 until Release", len(decoded))
	}

	tl.Release()
	if imgs, loading := tl.Images(); imgs != nil || loading {
		t.Errorf("after Release: %v, loading %v, want nothing", imgs, loading)
	}
	tl.Start(resolve)
	tl.release(1)
	tl.waitLoads(t, 2)
	if _, decoded := tl.counts(); len(decoded) != 2 {
		t.Errorf("decoded %d times after Release, want 2", len(decoded))
	}
}

func TestImageLoaderDropsStaleLoad(t *testing.T) {
	tl := newTestLoader()
	paths := files(t, "a.webp")
	tl.Start(func() []string { return paths })
	// Released while the decode is in progress: its result is dropped
	tl.Release()
	tl.release(1)
	time.Sleep(20 * time.Millisecond)
	if loads, _ := tl.counts(); loads != 0 {
		t.Errorf("stale load completed %d times, want dropped", loads)
	}
	if imgs, loading := tl.Images(); imgs != nil || loading {
		t.Errorf("stale load stored %v, loading %v", imgs, loading)
	}
}
