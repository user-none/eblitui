// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"log"
	"os"
	"sync"
)

// saver writes a file in the background whenever its owner changes. Each
// change marks it dirty; one goroutine at a time encodes the owner's
// current contents and writes them atomically, repeating while further
// changes arrive, so a burst of changes becomes as few writes as possible
// and the newest contents always end up on disk. The caller never waits
// for the disk.
type saver struct {
	path   string
	perm   os.FileMode
	encode func() ([]byte, error) // returns the owner's current contents; safe on any goroutine

	mu      sync.Mutex
	idle    *sync.Cond
	dirty   bool
	running bool
}

// newSaver returns a saver writing path with mode perm from encode.
func newSaver(path string, perm os.FileMode, encode func() ([]byte, error)) *saver {
	s := &saver{path: path, perm: perm, encode: encode}
	s.idle = sync.NewCond(&s.mu)
	return s
}

// request schedules a write of the current contents.
func (s *saver) request() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirty = true
	if !s.running {
		s.running = true
		go s.run()
	}
}

// run writes until no change is pending. Errors are logged; the next
// change tries again.
func (s *saver) run() {
	for {
		s.mu.Lock()
		if !s.dirty {
			s.running = false
			s.idle.Broadcast()
			s.mu.Unlock()
			return
		}
		s.dirty = false
		s.mu.Unlock()

		data, err := s.encode()
		if err == nil {
			err = atomicWriteFile(s.path, data, s.perm)
		}
		if err != nil {
			log.Printf("Failed to save %s: %v", s.path, err)
		}
	}
}

// flush waits until every requested write has finished.
func (s *saver) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.running || s.dirty {
		s.idle.Wait()
	}
}
