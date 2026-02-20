package storage

import (
	"encoding/json"
	"errors"
	"fmt"
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

	return config, nil
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
