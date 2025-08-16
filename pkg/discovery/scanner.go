// Package discovery provides device scanning and discovery functionality
package discovery

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"tinygo.org/x/bluetooth"
)

// DeviceInfo represents a discovered device
type DeviceInfo struct {
	Address     string
	Name        string
	RSSI        int16
	Type        string // "bluetooth"
	Connectable bool
}

// ScanOptions configures device scanning
type ScanOptions struct {
	Timeout        time.Duration
	ShowDuplicates bool
	ShowRSSI       bool
	ShowProgress   bool
	FilterPrinters bool // Only return devices that look like printers
}

// DefaultScanOptions returns sensible default scan options
func DefaultScanOptions() ScanOptions {
	return ScanOptions{
		Timeout:        8 * time.Second,
		ShowDuplicates: false,
		ShowRSSI:       true,
		ShowProgress:   true,
		FilterPrinters: false,
	}
}

// BluetoothScanner handles BLE device discovery
type BluetoothScanner struct {
	adapter *bluetooth.Adapter
}

// NewBluetoothScanner creates a new Bluetooth scanner
func NewBluetoothScanner() (*BluetoothScanner, error) {
	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		return nil, fmt.Errorf("failed to enable Bluetooth adapter: %w", err)
	}

	return &BluetoothScanner{adapter: adapter}, nil
}

// Scan discovers Bluetooth devices
func (s *BluetoothScanner) Scan(ctx context.Context, opts ScanOptions) ([]DeviceInfo, error) {
	seen := make(map[string]DeviceInfo)
	var mu sync.RWMutex

	// Set up progress reporting
	var progressTicker *time.Ticker
	var progressDone chan struct{}

	if opts.ShowProgress {
		progressTicker = time.NewTicker(1 * time.Second)
		progressDone = make(chan struct{})

		go func() {
			start := time.Now()
			for {
				select {
				case <-progressTicker.C:
					mu.RLock()
					count := len(seen)
					mu.RUnlock()
					elapsed := time.Since(start).Truncate(time.Second)
					log.Infof("Scanning... %s elapsed, %d device(s) found", elapsed, count)
				case <-progressDone:
					return
				}
			}
		}()
	}

	// Start scanning
	if err := s.adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
		addr := result.Address.String()
		name := result.LocalName()
		if name == "" {
			name = "(no name)"
		}

		// Check for duplicates
		if !opts.ShowDuplicates {
			mu.RLock()
			_, exists := seen[addr]
			mu.RUnlock()
			if exists {
				return
			}
		}

		device := DeviceInfo{
			Address:     addr,
			Name:        name,
			RSSI:        result.RSSI,
			Type:        "bluetooth",
			Connectable: true, // Assume connectable for now
		}

		mu.Lock()
		seen[addr] = device
		mu.Unlock()

		if opts.ShowDuplicates {
			if opts.ShowRSSI {
				log.Infof("%s\t%s\t(RSSI %d)", addr, name, result.RSSI)
			} else {
				log.Infof("%s\t%s", addr, name)
			}
		}
	}); err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	// Wait for timeout or context cancellation
	select {
	case <-time.After(opts.Timeout):
		// Normal timeout
	case <-ctx.Done():
		// Context cancelled
	}

	// Stop scanning and cleanup
	s.adapter.StopScan()

	if opts.ShowProgress {
		close(progressDone)
		progressTicker.Stop()

		mu.RLock()
		total := len(seen)
		mu.RUnlock()
		log.Infof("Scan complete: %d device(s) found", total)
	}

	// Convert to slice and sort
	mu.RLock()
	devices := make([]DeviceInfo, 0, len(seen))
	for _, device := range seen {
		devices = append(devices, device)
	}
	mu.RUnlock()

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Address < devices[j].Address
	})

	// Filter for printers if requested
	if opts.FilterPrinters {
		filtered := make([]DeviceInfo, 0, len(devices))
		for _, device := range devices {
			if isPotentialPrinter(device) {
				filtered = append(filtered, device)
			}
		}
		devices = filtered
	}

	return devices, nil
}

// FindPrinters scans for devices that look like thermal printers
func (s *BluetoothScanner) FindPrinters(ctx context.Context, timeout time.Duration) ([]DeviceInfo, error) {
	opts := ScanOptions{
		Timeout:        timeout,
		ShowDuplicates: false,
		ShowRSSI:       false,
		ShowProgress:   true,
		FilterPrinters: true,
	}

	return s.Scan(ctx, opts)
}

// isPotentialPrinter determines if a device might be a thermal printer based on name
func isPotentialPrinter(device DeviceInfo) bool {
	name := strings.ToLower(device.Name)

	// Common printer name patterns
	printerPatterns := []string{
		"printer", "print", "thermal", "receipt", "pos", "escpos",
		"mpt", "bluetooth printer", "bt printer", "mini printer",
		"portable printer", "ticket printer",
	}

	for _, pattern := range printerPatterns {
		if strings.Contains(name, pattern) {
			return true
		}
	}

	return false
}

// USBScanner handles USB device discovery (stub for future implementation)
type USBScanner struct{}

// NewUSBScanner creates a new USB scanner
func NewUSBScanner() *USBScanner {
	return &USBScanner{}
}

// Scan discovers USB printers
func (s *USBScanner) Scan(ctx context.Context) ([]DeviceInfo, error) {
	// TODO: Implement USB device enumeration using gousb
	// For now, return common MPT-II device
	devices := []DeviceInfo{
		{
			Address:     "0483:5840",
			Name:        "MPT-II Thermal Printer",
			Type:        "usb",
			Connectable: true,
		},
	}

	return devices, nil
}
