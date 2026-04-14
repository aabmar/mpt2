// Bluetooth connection implementation using tinygo.org/x/bluetooth
package connections

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

const (
	defaultBLEDiscoveryTimeout = 8 * time.Second
	unknownBLEDeviceName       = "Unknown"
)

// session-level cache of MAC -> whether device exposed a supported printer
// write characteristic during this program run.
var blePrinterCapabilityCache = struct {
	mu    sync.RWMutex
	known map[string]bool
}{known: make(map[string]bool)}

type scannedBLEDevice struct {
	address bluetooth.Address
	mac     string
	name    string
	rssi    int16
}

// BluetoothConnection implements Connection interface for Bluetooth LE printers
type BluetoothConnection struct {
	address   string
	connected bool
	device    bluetooth.Device
	service   *bluetooth.DeviceService
	writeChar *bluetooth.DeviceCharacteristic
	// candidates holds all matched writable printer characteristics in priority order
	candidates []*bluetooth.DeviceCharacteristic
	verbose    bool
	mutex      sync.RWMutex
}

// Note: Some printers expose Nordic UART or custom services, but we rely on
// characteristic discovery (see printerWriteCharacteristics) rather than
// hard-coding service UUIDs. This keeps discovery flexible across vendors.

// Common characteristics used by thermal printers (from Python conversion)
var printerWriteCharacteristics = []string{
	// ESC/POS printer characteristic (often correct for many modules)
	"49535343-8841-43f4-a8d4-ecbe34729bb3",
	// Generic printer characteristics (often FF02 is write, FF01 may be notify)
	"0000ff02-0000-1000-8000-00805f9b34fb",
	"0000ff01-0000-1000-8000-00805f9b34fb",
	// Nordic UART TX (write)
	"6e400002-b5a3-f393-e0a9-e50e24dcca9e",
	// Custom printer UUIDs (these vary by manufacturer)
	"0000ffe1-0000-1000-8000-00805f9b34fb",
	"0000ffe2-0000-1000-8000-00805f9b34fb",
	// Additional common printer characteristics seen in the wild
	"0000fff1-0000-1000-8000-00805f9b34fb",
	"0000fff2-0000-1000-8000-00805f9b34fb",
}

// NewBluetoothConnection creates a new Bluetooth connection
func NewBluetoothConnection(address string, verbose bool) (*BluetoothConnection, error) {
	// Validate MAC address format
	if address != "" && !isValidMACAddress(address) {
		return nil, fmt.Errorf("invalid MAC address format: %s (expected XX:XX:XX:XX:XX:XX)", address)
	}

	return &BluetoothConnection{
		address:   strings.ToUpper(address),
		connected: false,
		verbose:   verbose,
	}, nil
}

// Connect establishes the Bluetooth connection
func (b *BluetoothConnection) Connect(ctx context.Context) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.connected {
		return nil
	}

	// Enable the default adapter
	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		return fmt.Errorf("failed to enable Bluetooth adapter: %w", err)
	}

	// Use scan-based approach (more reliable on Windows)
	return b.connectViaScan(ctx, adapter)
}

// connectViaScan attempts connection via device discovery
func (b *BluetoothConnection) connectViaScan(ctx context.Context, adapter *bluetooth.Adapter) error {
	// If no address provided, scan first, then probe likely candidates in priority order.
	if b.address == "" {
		if b.verbose {
			log.Debug("Scanning for BLE printers (no address provided)...")
		}

		devices, err := scanBLEDevices(ctx, adapter, autoDiscoveryTimeout(ctx), b.verbose)
		if err != nil {
			return err
		}
		if len(devices) == 0 {
			return fmt.Errorf("no BLE devices found during auto-discovery")
		}

		for _, candidate := range devices {
			if err := ctx.Err(); err != nil {
				return err
			}

			blePrinterCapabilityCache.mu.RLock()
			known, ok := blePrinterCapabilityCache.known[candidate.mac]
			blePrinterCapabilityCache.mu.RUnlock()
			if ok && !known {
				if b.verbose {
					log.WithField("mac", candidate.mac).Debug("Skipping device (cached as non-printer)")
				}
				continue
			}

			if b.verbose {
				log.WithFields(log.Fields{
					"mac":  candidate.mac,
					"name": displayBLEName(candidate.name),
					"rssi": candidate.rssi,
				}).Debug("Probing scanned BLE device")
			}

			dev, matchedChar, matchedUUID, err := connectKnownPrinterCandidate(adapter, candidate.address)
			if err != nil {
				if b.verbose {
					log.WithError(err).WithField("mac", candidate.mac).Debug("Candidate probe failed")
				}
				blePrinterCapabilityCache.mu.Lock()
				blePrinterCapabilityCache.known[candidate.mac] = false
				blePrinterCapabilityCache.mu.Unlock()
				continue
			}

			b.device = dev
			b.writeChar = &matchedChar
			b.candidates = []*bluetooth.DeviceCharacteristic{b.writeChar}
			b.connected = true
			b.address = strings.ToUpper(candidate.mac)

			blePrinterCapabilityCache.mu.Lock()
			blePrinterCapabilityCache.known[candidate.mac] = true
			blePrinterCapabilityCache.mu.Unlock()

			if b.verbose {
				log.WithFields(log.Fields{
					"mac":  candidate.mac,
					"name": displayBLEName(candidate.name),
					"uuid": matchedUUID,
				}).Info("Selected BLE printer during auto-discovery")
			}
			return nil
		}

		return fmt.Errorf("no BLE printer exposing a supported write characteristic found among %d scanned devices", len(devices))
	}

	if b.verbose {
		log.WithField("address", b.address).Debug("Scanning for target Bluetooth device...")
	}
	targetAddress, _, err := scanForAddress(ctx, adapter, b.address, autoDiscoveryTimeout(ctx), b.verbose)
	if err != nil {
		return err
	}

	if b.verbose {
		log.WithField("address", b.address).Debug("Attempting to connect to device")
	}

	// Try to connect with a timeout
	connectCtx, connectCancel := context.WithTimeout(ctx, 10*time.Second)
	defer connectCancel()

	// Try connection using the discovered address
	device, err := adapter.Connect(targetAddress, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("failed to connect to device %s: %w", b.address, err)
	}

	// Check if context was cancelled during connection
	select {
	case <-connectCtx.Done():
		device.Disconnect()
		return fmt.Errorf("connection to device %s timed out", b.address)
	default:
		// Connection successful
	}

	if b.verbose {
		log.WithField("address", b.address).Info("Connected to BLE device")
	}
	b.device = device
	return b.setupCharacteristics()
}

func autoDiscoveryTimeout(ctx context.Context) time.Duration {
	timeout := defaultBLEDiscoveryTimeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0
		}
		if remaining < timeout {
			return remaining
		}
	}
	return timeout
}

func scanBLEDevices(ctx context.Context, adapter *bluetooth.Adapter, timeout time.Duration, verbose bool) ([]scannedBLEDevice, error) {
	if timeout <= 0 {
		return nil, context.DeadlineExceeded
	}

	seen := make(map[string]scannedBLEDevice)
	var mu sync.Mutex
	stopScan := make(chan struct{})
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	go func() {
		select {
		case <-ctx.Done():
		case <-timer.C:
		case <-stopScan:
			return
		}
		_ = adapter.StopScan()
	}()

	err := adapter.Scan(func(adapter *bluetooth.Adapter, scanResult bluetooth.ScanResult) {
		mac := scanResult.Address.String()
		name := strings.TrimSpace(scanResult.LocalName())

		mu.Lock()
		existing, ok := seen[mac]
		if ok {
			if name != "" && existing.name == "" {
				existing.name = name
			}
			if scanResult.RSSI > existing.rssi {
				existing.rssi = scanResult.RSSI
			}
			existing.address = scanResult.Address
			seen[mac] = existing
		} else {
			seen[mac] = scannedBLEDevice{
				address: scanResult.Address,
				mac:     mac,
				name:    name,
				rssi:    scanResult.RSSI,
			}
		}
		mu.Unlock()
	})
	close(stopScan)
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	devices := make([]scannedBLEDevice, 0, len(seen))
	for _, device := range seen {
		devices = append(devices, device)
	}

	sort.Slice(devices, func(i, j int) bool {
		leftPrinter := looksLikePrinterName(devices[i].name)
		rightPrinter := looksLikePrinterName(devices[j].name)
		if leftPrinter != rightPrinter {
			return leftPrinter
		}

		leftNamed := devices[i].name != ""
		rightNamed := devices[j].name != ""
		if leftNamed != rightNamed {
			return leftNamed
		}

		if devices[i].rssi != devices[j].rssi {
			return devices[i].rssi > devices[j].rssi
		}

		return devices[i].mac < devices[j].mac
	})

	if verbose {
		log.WithFields(log.Fields{
			"timeout": timeout,
			"count":   len(devices),
		}).Debug("BLE discovery scan complete")
	}

	return devices, nil
}

func scanForAddress(ctx context.Context, adapter *bluetooth.Adapter, target string, timeout time.Duration, verbose bool) (bluetooth.Address, int, error) {
	if timeout <= 0 {
		return bluetooth.Address{}, 0, context.DeadlineExceeded
	}

	stopScan := make(chan struct{})
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var (
		targetAddress bluetooth.Address
		devicesFound  int
		targetFound   bool
	)

	go func() {
		select {
		case <-ctx.Done():
		case <-timer.C:
		case <-stopScan:
			return
		}
		_ = adapter.StopScan()
	}()

	err := adapter.Scan(func(adapter *bluetooth.Adapter, scanResult bluetooth.ScanResult) {
		if targetFound {
			return
		}

		devicesFound++
		deviceAddr := scanResult.Address.String()
		if verbose {
			log.WithFields(log.Fields{
				"mac":  deviceAddr,
				"name": displayBLEName(strings.TrimSpace(scanResult.LocalName())),
			}).Debug("Found device during scan")
		}

		if deviceAddr == target || strings.EqualFold(deviceAddr, target) {
			targetFound = true
			targetAddress = scanResult.Address
			_ = adapter.StopScan()
		}
	})
	close(stopScan)
	if err != nil {
		return bluetooth.Address{}, devicesFound, fmt.Errorf("scan failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return bluetooth.Address{}, devicesFound, err
	}
	if !targetFound {
		return bluetooth.Address{}, devicesFound, fmt.Errorf("scan completed but device %s not found (scanned %d devices)", target, devicesFound)
	}

	return targetAddress, devicesFound, nil
}

func connectKnownPrinterCandidate(adapter *bluetooth.Adapter, address bluetooth.Address) (bluetooth.Device, bluetooth.DeviceCharacteristic, string, error) {
	dev, err := adapter.Connect(address, bluetooth.ConnectionParams{})
	if err != nil {
		return bluetooth.Device{}, bluetooth.DeviceCharacteristic{}, "", err
	}

	matchedChar, matchedUUID, err := findKnownPrinterCharacteristic(dev)
	if err != nil {
		_ = dev.Disconnect()
		return bluetooth.Device{}, bluetooth.DeviceCharacteristic{}, "", err
	}

	return dev, matchedChar, matchedUUID, nil
}

func findKnownPrinterCharacteristic(dev bluetooth.Device) (bluetooth.DeviceCharacteristic, string, error) {
	services, err := dev.DiscoverServices(nil)
	if err != nil {
		return bluetooth.DeviceCharacteristic{}, "", err
	}

	var allCharacteristics []bluetooth.DeviceCharacteristic
	for _, svc := range services {
		chars, err := svc.DiscoverCharacteristics(nil)
		if err != nil {
			continue
		}
		allCharacteristics = append(allCharacteristics, chars...)
	}

	for _, knownUUID := range printerWriteCharacteristics {
		for i := range allCharacteristics {
			if strings.EqualFold(allCharacteristics[i].UUID().String(), knownUUID) {
				return allCharacteristics[i], knownUUID, nil
			}
		}
	}

	return bluetooth.DeviceCharacteristic{}, "", fmt.Errorf("no supported printer write characteristic found")
}

func displayBLEName(name string) string {
	if name == "" {
		return unknownBLEDeviceName
	}
	return name
}

func looksLikePrinterName(name string) bool {
	name = strings.ToLower(name)
	if name == "" {
		return false
	}

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

// setupCharacteristics discovers services and sets up the write characteristic
func (b *BluetoothConnection) setupCharacteristics() error {
	if b.verbose {
		log.WithField("address", b.address).Debug("Discovering services on device")
	}

	// Discover services
	services, err := b.device.DiscoverServices(nil)
	if err != nil {
		b.device.Disconnect()
		return fmt.Errorf("device found but failed to discover services: %w", err)
	}

	if b.verbose {
		log.WithField("count", len(services)).Debug("Services discovered on device")
	}

	// Try each service to find a suitable characteristic
	var allCharacteristics []bluetooth.DeviceCharacteristic

	for i, svc := range services {
		if b.verbose {
			log.WithFields(log.Fields{"index": i + 1, "uuid": svc.UUID().String()}).Debug("Service")
		}

		// Discover characteristics for this service
		chars, err := svc.DiscoverCharacteristics(nil)
		if err != nil {
			log.WithError(err).Debug("Failed to discover characteristics for service")
			continue
		}

		if b.verbose {
			log.WithField("count", len(chars)).Debug("Characteristics discovered for service")
		}
		for j, char := range chars {
			if b.verbose {
				log.WithFields(log.Fields{"index": j + 1, "uuid": char.UUID().String()}).Debug("Characteristic")
			}
			allCharacteristics = append(allCharacteristics, char)
		}
	}

	// Look for known printer write characteristics and collect candidates in priority order
	var writeChar *bluetooth.DeviceCharacteristic
	var matchedUUID string
	var candidates []*bluetooth.DeviceCharacteristic

	for _, knownUUID := range printerWriteCharacteristics {
		for i := range allCharacteristics {
			char := &allCharacteristics[i]
			if strings.EqualFold(char.UUID().String(), knownUUID) {
				if b.verbose {
					log.WithField("uuid", knownUUID).Debug("Matched known printer characteristic")
				}
				candidates = append(candidates, char)
			}
		}
	}

	if len(candidates) > 0 {
		writeChar = candidates[0]
		matchedUUID = writeChar.UUID().String()
	}

	// Do NOT fall back to arbitrary characteristics like Device Name (0x2A00) which are not writable for printing
	if writeChar == nil {
		b.device.Disconnect()
		if len(services) == 0 {
			return fmt.Errorf("device %s found but has no services (device may not be a BLE printer)", b.address)
		}
		// Build a list of discovered characteristic UUIDs for debugging
		var discovered []string
		for i := range allCharacteristics {
			discovered = append(discovered, allCharacteristics[i].UUID().String())
		}
		return fmt.Errorf("device %s found but no known printer write characteristic present. Discovered characteristics: %s", b.address, strings.Join(discovered, ", "))
	}

	b.writeChar = writeChar
	b.candidates = candidates
	b.connected = true

	if b.verbose {
		log.WithFields(log.Fields{"address": b.address, "uuid": matchedUUID}).Info("BLE printer characteristic selected")
	}
	return nil
}

// Write sends data to the Bluetooth printer
func (b *BluetoothConnection) Write(data []byte) (int, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if !b.connected || b.writeChar == nil {
		return 0, fmt.Errorf("not connected to device")
	}

	// BLE has MTU limitations, so we might need to split large writes
	const maxChunkSize = 20 // Conservative BLE chunk size
	totalWritten := 0

	for len(data) > 0 {
		chunkSize := len(data)
		if chunkSize > maxChunkSize {
			chunkSize = maxChunkSize
		}

		chunk := data[:chunkSize]
		data = data[chunkSize:]

		if b.verbose {
			log.WithField("bytes", len(chunk)).Debug("Writing chunk to BLE characteristic")
		}

		// Try WriteWithoutResponse first, then fall back to regular Write
		_, err := b.writeChar.WriteWithoutResponse(chunk)
		if err != nil {
			if b.verbose {
				log.WithError(err).Debug("WriteWithoutResponse failed; trying Write")
			}
			// Try regular Write method
			if _, werr := b.writeChar.Write(chunk); werr != nil {
				// If write not supported, try next candidate characteristic if available
				// We match on error string due to tinygo API
				if strings.Contains(strings.ToLower(werr.Error()), "write not supported") && len(b.candidates) > 1 {
					if b.verbose {
						log.WithField("uuid", b.writeChar.UUID().String()).Debug("Characteristic rejected writes; trying alternate")
					}
					for idx := 1; idx < len(b.candidates); idx++ {
						alt := b.candidates[idx]
						if b.verbose {
							log.WithField("uuid", alt.UUID().String()).Debug("Switching to alternate characteristic")
						}
						b.writeChar = alt
						if _, aerr := b.writeChar.WriteWithoutResponse(chunk); aerr == nil {
							if b.verbose {
								log.WithField("bytes", len(chunk)).Debug("Alternate WriteWithoutResponse succeeded")
							}
							werr = nil
							break
						}
						if _, aerr := b.writeChar.Write(chunk); aerr == nil {
							if b.verbose {
								log.WithField("bytes", len(chunk)).Debug("Alternate Write succeeded")
							}
							werr = nil
							break
						}
					}
					if werr != nil {
						return totalWritten, fmt.Errorf("failed to write to any candidate characteristic: %w", werr)
					}
				} else {
					return totalWritten, fmt.Errorf("failed to write to characteristic (both WriteWithoutResponse and Write failed): %w", werr)
				}
			} else {
				if b.verbose {
					log.WithField("bytes", len(chunk)).Debug("Write succeeded")
				}
			}
		} else {
			if b.verbose {
				log.WithField("bytes", len(chunk)).Debug("WriteWithoutResponse succeeded")
			}
		}

		totalWritten += chunkSize

		// Small delay between chunks to avoid overwhelming the device
		if len(data) > 0 {
			time.Sleep(50 * time.Millisecond) // Slightly longer delay for regular Write
		}
	}

	if b.verbose {
		log.WithField("bytes", totalWritten).Debug("Completed BLE write to printer")
	}
	return totalWritten, nil
}

// Close closes the Bluetooth connection
func (b *BluetoothConnection) Close() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.connected {
		err := b.device.Disconnect()
		b.connected = false
		b.device = bluetooth.Device{}
		b.service = nil
		b.writeChar = nil
		return err
	}

	b.connected = false
	return nil
}

// IsConnected returns true if the Bluetooth connection is active
func (b *BluetoothConnection) IsConnected() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.connected
}

// GetInfo returns information about the Bluetooth connection
func (b *BluetoothConnection) GetInfo() ConnectionInfo {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return ConnectionInfo{
		Type:        string(ConnectionTypeBluetooth),
		Address:     b.address,
		Description: fmt.Sprintf("Bluetooth LE Printer (%s)", b.address),
		Properties: map[string]interface{}{
			"address":     b.address,
			"connected":   b.connected,
			"implemented": true,
		},
	}
}

// isValidMACAddress checks if the address is in the correct MAC format
func isValidMACAddress(addr string) bool {
	if len(addr) != 17 {
		return false
	}

	parts := strings.Split(addr, ":")
	if len(parts) != 6 {
		return false
	}

	for _, part := range parts {
		if len(part) != 2 {
			return false
		}
		for _, c := range part {
			if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f')) {
				return false
			}
		}
	}

	return true
}
