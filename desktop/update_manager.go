// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package desktop

import (
	"log"
	"sync"
	"time"

	"github.com/user-none/eblitui/desktop/launch"
	"github.com/user-none/eblitui/desktop/storage"
	"github.com/user-none/eblitui/desktop/types"
	"github.com/user-none/eblitui/desktop/update"
)

// UpdateConfig describes where an application's releases are published and
// how they are verified. See SetUpdateConfig.
type UpdateConfig struct {
	ManifestURL    string   // The latest release's manifest, an HTTPS URL
	SignatureURL   string   // The latest release's manifest signature file, an HTTPS URL
	ReleasePageURL string   // A release's page, an HTTPS URL; {version} is replaced by the version
	AppName        string   // Application name bound into the signed manifest
	Version        string   // Running version
	Keys           []string // Update signing public keys, each algorithm:base64
}

// updateConfig holds the update configuration supplied by the core via
// SetUpdateConfig. The zero value leaves the update system off.
var updateConfig UpdateConfig

// SetUpdateConfig turns on checking for new releases. A core calls this
// before Run with where its releases are published, its name and running
// version, and the update signing public keys stamped into its release
// builds. If never called, called without usable keys, or called with a URL
// that is not HTTPS, the update system is off: no network requests, no
// update bar, and no Updates area in About.
func SetUpdateConfig(cfg UpdateConfig) {
	updateConfig = cfg
}

// updateCheckInterval is the least time between automatic checks
const updateCheckInterval = 24 * time.Hour

// UpdateManager checks for new releases and holds the update state shown by
// the update bar and Settings > About. Checks run in the background; the
// methods are safe to call from the UI thread while one runs.
type UpdateManager struct {
	source   *update.Source
	target   update.Target
	config   *storage.Config
	onChange func() // Called from the check goroutine when a check finishes

	// saveMu serializes saves so they reach disk in the order their state
	// was taken. It is held during disk writes; mu never is, so reading the
	// state never waits on the disk. A save takes saveMu before mu.
	saveMu sync.Mutex

	mu        sync.Mutex
	state     *storage.UpdateState
	available *update.Manifest // Verified manifest offering a newer version, nil when none
	checking  bool
	failed    bool // The last check this session failed
	dismissed bool // The update bar was dismissed this session
}

// NewUpdateManager returns the update manager for cfg, or nil when the update
// system is off. It loads the stored update state and manifest.
func NewUpdateManager(cfg UpdateConfig, config *storage.Config, onChange func()) *UpdateManager {
	if len(cfg.Keys) == 0 {
		return nil
	}
	keys, errs := update.ParseKeys(cfg.Keys)
	for _, err := range errs {
		log.Printf("Ignoring %v", err)
	}
	if keys.Empty() {
		log.Printf("Update check disabled: no usable update keys")
		return nil
	}
	if cfg.AppName == "" {
		log.Printf("Update check disabled: no application name")
		return nil
	}
	source, err := update.NewSource(cfg.ManifestURL, cfg.SignatureURL, cfg.ReleasePageURL)
	if err != nil {
		log.Printf("Update check disabled: %v", err)
		return nil
	}

	m := &UpdateManager{
		source: source,
		target: update.Target{
			App:      cfg.AppName,
			Version:  update.RunningVersion(cfg.Version),
			Platform: update.Platform(),
			Keys:     keys,
		},
		config:   config,
		onChange: onChange,
	}
	m.load()
	return m
}

// load reads the stored update state and manifest. The stored manifest is
// verified again since the update directory is writable by the user.
func (m *UpdateManager) load() {
	state, err := storage.LoadUpdateState()
	if err != nil {
		log.Printf("Failed to load update state: %v", err)
		state = &storage.UpdateState{}
	}
	m.state = state

	manifest, sig, err := storage.LoadUpdateManifest(update.MaxManifestSize, update.MaxSignatureSize)
	if err != nil {
		return
	}
	if status, found, _ := m.target.Check(manifest, sig); status == update.Available {
		m.available = found
	}
}

// StartAutomaticCheck starts a background check when automatic checks are
// on and the last check is at least updateCheckInterval old.
func (m *UpdateManager) StartAutomaticCheck() {
	if !m.config.Updates.AutoCheck {
		return
	}
	m.mu.Lock()
	due := checkDue(m.state.LastCheck, time.Now())
	m.mu.Unlock()
	if due {
		m.startCheck(false)
	}
}

// CheckNow starts a manual background check unless one is running. An
// update it finds is treated as ignored for the update bar.
func (m *UpdateManager) CheckNow() {
	m.startCheck(true)
}

// checkDue reports whether a check is due at now given the last check time.
// A last check in the future (the clock changed) makes a check due.
func checkDue(lastCheck int64, now time.Time) bool {
	if lastCheck == 0 {
		return true
	}
	last := time.Unix(lastCheck, 0)
	return now.Before(last) || now.Sub(last) >= updateCheckInterval
}

// startCheck runs a check in the background unless one is running
func (m *UpdateManager) startCheck(manual bool) {
	m.mu.Lock()
	if m.checking {
		m.mu.Unlock()
		return
	}
	m.checking = true
	m.mu.Unlock()

	go func() {
		manifest, sig, err := m.source.FetchLatest()
		m.finishCheck(manifest, sig, err, manual, time.Now())
		if m.onChange != nil {
			m.onChange()
		}
	}()
}

// finishCheck records the result of a check. A network failure or a
// manifest that fails verification is a failed check; the stored manifest
// is kept. A manifest that offers an update replaces the stored one.
func (m *UpdateManager) finishCheck(manifest, sig []byte, fetchErr error, manual bool, now time.Time) {
	status := update.Invalid
	var found *update.Manifest
	err := fetchErr
	if err == nil {
		status, found, err = m.target.Check(manifest, sig)
	}
	if err != nil {
		log.Printf("Update check failed: %v", err)
	}

	m.saveMu.Lock()
	defer m.saveMu.Unlock()

	m.mu.Lock()
	m.checking = false
	m.failed = err != nil
	m.state.LastCheck = now.Unix()
	if status == update.Available {
		m.available = found
		if manual {
			m.state.IgnoredVersion = found.Version
		}
	}
	state := *m.state
	m.mu.Unlock()

	if status == update.Available {
		if err := storage.SaveUpdateManifest(manifest, sig); err != nil {
			log.Printf("Failed to save update manifest: %v", err)
		}
	}
	saveUpdateState(state)
}

// saveUpdateState saves a copy of the update state. The caller holds saveMu
// and not mu.
func saveUpdateState(state storage.UpdateState) {
	if err := storage.SaveUpdateState(&state); err != nil {
		log.Printf("Failed to save update state: %v", err)
	}
}

// AppName returns the application name shown in the update bar
func (m *UpdateManager) AppName() string {
	return m.target.App
}

// Status returns the update state for Settings > About
func (m *UpdateManager) Status() types.UpdateStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	status := types.UpdateStatus{
		Checking:  m.checking,
		Failed:    m.failed,
		LastCheck: m.state.LastCheck,
	}
	if m.available != nil {
		status.Available = m.available.Version
	}
	return status
}

// BarVersion returns the version the update bar offers, or "" when the bar
// is not shown. The bar is shown while automatic checks are on and an update
// is available that is newer than the ignored version and was not dismissed
// this session. It is not shown while a check runs, since the check may find
// a newer version.
func (m *UpdateManager) BarVersion() string {
	if !m.config.Updates.AutoCheck {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.checking || m.dismissed || m.available == nil {
		return ""
	}
	version, _ := update.ParseVersion(m.available.Version)
	ignored, _ := update.ParseVersion(m.state.IgnoredVersion)
	if !ignored.Less(version) {
		return ""
	}
	return m.available.Version
}

// IgnoreVersion stops the update bar from showing the available version.
// A newer version shows it again.
func (m *UpdateManager) IgnoreVersion() {
	m.saveMu.Lock()
	defer m.saveMu.Unlock()

	m.mu.Lock()
	if m.available == nil {
		m.mu.Unlock()
		return
	}
	m.state.IgnoredVersion = m.available.Version
	state := *m.state
	m.mu.Unlock()

	saveUpdateState(state)
}

// Dismiss hides the update bar for the rest of this session
func (m *UpdateManager) Dismiss() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dismissed = true
}

// Download opens the available version's release page in the browser and
// stops the update bar from showing that version.
func (m *UpdateManager) Download() {
	m.mu.Lock()
	available := m.available
	m.mu.Unlock()
	if available == nil {
		return
	}

	if err := launch.OpenURL(m.source.ReleasePage(available.Version)); err != nil {
		log.Printf("Failed to open release page: %v", err)
	}
	m.IgnoreVersion()
}
