package configstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LastDevice represents the last successfully used device for quick reconnects.
type LastDevice struct {
	Type      string    `json:"type"` // "bluetooth"
	BLE       *BLEInfo  `json:"ble,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type BLEInfo struct {
	Address string `json:"address"`
}

// path returns the full path to the cache file in the user's config directory.
func path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve user config dir: %w", err)
	}
	appDir := filepath.Join(dir, "mpt2")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create app config dir: %w", err)
	}
	return filepath.Join(appDir, "device_cache.json"), nil
}

// LoadLastDevice loads the cached last device information.
func LoadLastDevice() (*LastDevice, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("failed to read cache: %w", err)
	}
	var d LastDevice
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("failed to parse cache: %w", err)
	}
	return &d, nil
}

// SaveLastDevice writes the given device information to cache.
func SaveLastDevice(d *LastDevice) error {
	if d == nil {
		return nil
	}
	p, err := path()
	if err != nil {
		return err
	}
	d.UpdatedAt = time.Now().UTC()
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize cache: %w", err)
	}
	// Write atomically: write to temp then rename
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("failed to write temp cache: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("failed to commit cache: %w", err)
	}
	return nil
}

// ClearLastDevice removes the cache file, if present.
func ClearLastDevice() error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to remove cache: %w", err)
	}
	return nil
}
