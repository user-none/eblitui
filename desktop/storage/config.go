// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
)

// LoadConfig loads the configuration from config.json.
// If the file doesn't exist, it returns default configuration.
// If the file is corrupted, it returns an error.
// Missing fields (absent from JSON) are silently defaulted.
func LoadConfig() (*Config, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	// Check if file exists
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		// File doesn't exist, return defaults
		return DefaultConfig(), nil
	}

	// Read raw bytes for both parsing and key detection
	jsonBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Parse into Config struct
	config := &Config{}
	if err := json.Unmarshal(jsonBytes, config); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Detect which keys are actually present in the JSON
	presentKeys := detectPresentKeys(jsonBytes)

	// Apply defaults only for fields that are absent from the file
	ApplyMissingDefaults(config, presentKeys)

	// Apply any migration for older config versions, saving the result so
	// the migration runs once
	if migrateConfig(config) {
		if err := SaveConfig(config); err != nil {
			log.Printf("Failed to save migrated config: %v", err)
		}
	}

	return config, nil
}

// configVersion is the current config.json format version. Version 2 added
// the automatic update check setting.
const configVersion = 2

// migrateConfig updates a config from an older version to the current one
// and reports whether it changed anything. Version 0 (absent from the file)
// is treated as version 1. A version newer than the current one is left for
// validation to report.
func migrateConfig(config *Config) bool {
	migrated := false
	if config.Version == 0 {
		config.Version = 1
		migrated = true
	}
	if config.Version == 1 {
		config.Updates.AutoCheck = true
		config.Version = 2
		migrated = true
	}
	return migrated
}

// SaveConfig saves the configuration to config.json atomically
func SaveConfig(config *Config) error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	return AtomicWriteJSON(path, config)
}

// CreateConfigIfMissing creates a default config.json if it doesn't exist
func CreateConfigIfMissing() error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	// Check if file exists
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		// Create default config
		return SaveConfig(DefaultConfig())
	}

	return nil
}

// DeleteConfig removes the config.json file
func DeleteConfig() error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}
