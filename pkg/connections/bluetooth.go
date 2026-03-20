// Bluetooth connection implementation using tinygo.org/x/bluetooth
package connections

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"tinygo.org/x/bluetooth"
)

// session-level cache of MAC -> whether device exposed ESC/POS characteristic during this program run
var blePrinterCapabilityCache = struct {
	mu    sync.RWMutex
	known map[string]bool
}{known: make(map[string]bool)}

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
	// If no address provided, we auto-scan and pick the first device that exposes the ESC/POS write characteristic.
	if b.address == "" {
		if b.verbose {
			log.Debug("Scanning for BLE printers (no address provided)...")
		}
		// We will scan and attempt to connect to devices one by one until we find one exposing the target characteristic.
		// Record seen MACs to avoid duplicate attempts in this session.
		seen := map[string]bool{}
		var selectedDevice *bluetooth.Device

		// Scan and for each discovered device, attempt a quick service/characteristic check.
		err := adapter.Scan(func(adapter *bluetooth.Adapter, scanResult bluetooth.ScanResult) {
			mac := scanResult.Address.String()
			if seen[mac] {
				return
			}
			seen[mac] = true

			// Consult cache: skip devices known not to be printers
			blePrinterCapabilityCache.mu.RLock()
			known, ok := blePrinterCapabilityCache.known[mac]
			blePrinterCapabilityCache.mu.RUnlock()
			if ok && !known {
				if b.verbose {
					log.WithField("mac", mac).Debug("Skipping device (cached as non-printer)")
				}
				return
			}
			if b.verbose {
				name := scanResult.LocalName()
				if name == "" {
					name = "Unknown"
				}
				log.WithFields(log.Fields{"mac": mac, "name": name}).Debug("Found device during scan")
			}
			// Try connecting briefly to inspect services
			if dev, err := adapter.Connect(scanResult.Address, bluetooth.ConnectionParams{}); err == nil {
				// Discover services and look for the ESC/POS characteristic 49535343-...-29bb3
				if svcs, err := dev.DiscoverServices(nil); err == nil {
					for _, svc := range svcs {
						if chars, err := svc.DiscoverCharacteristics(nil); err == nil {
							for _, ch := range chars {
								if strings.EqualFold(ch.UUID().String(), "49535343-8841-43f4-a8d4-ecbe34729bb3") {
									// Found a candidate printer device
									adapter.StopScan()
									selectedDevice = &dev
									b.device = dev
									b.writeChar = &ch
									b.candidates = []*bluetooth.DeviceCharacteristic{&ch}
									b.connected = true
									b.address = strings.ToUpper(mac)
									// cache as printer-capable
									blePrinterCapabilityCache.mu.Lock()
									blePrinterCapabilityCache.known[mac] = true
									blePrinterCapabilityCache.mu.Unlock()
									if b.verbose {
										log.WithFields(log.Fields{"mac": mac, "uuid": ch.UUID().String()}).Info("Selected BLE printer (ESC/POS characteristic)")
									}
									return
								}
							}
						}
					}
				}
				// Not a printer; disconnect and continue scanning
				_ = dev.Disconnect()
				blePrinterCapabilityCache.mu.Lock()
				blePrinterCapabilityCache.known[mac] = false
				blePrinterCapabilityCache.mu.Unlock()
			}
		})
		if err != nil {
			return fmt.Errorf("scan failed: %w", err)
		}
		if selectedDevice == nil {
			return fmt.Errorf("no BLE printer exposing ESC/POS characteristic found nearby")
		}
		// If we preselected via characteristic, we can proceed without further discovery.
		return nil
	}

	if b.verbose {
		log.WithField("address", b.address).Debug("Scanning for target Bluetooth device...")
	}
	devicesFound := 0
	targetFound := false
	var targetAddress bluetooth.Address

	// Simple approach: scan until we find the device, then stop and connect
	err := adapter.Scan(func(adapter *bluetooth.Adapter, scanResult bluetooth.ScanResult) {
		if targetFound {
			return // Already found target, ignore other results
		}

		devicesFound++
		deviceAddr := scanResult.Address.String()
		if b.verbose {
			deviceName := "Unknown"
			if scanResult.LocalName() != "" {
				deviceName = scanResult.LocalName()
			}
			log.WithFields(log.Fields{"mac": deviceAddr, "name": deviceName}).Debug("Found device during scan")
		}

		// Try both exact match and case-insensitive match
		if deviceAddr == b.address || strings.EqualFold(deviceAddr, b.address) {
			if b.verbose {
				log.Debug("Target device found; stopping scan and connecting...")
			}
			targetFound = true
			targetAddress = scanResult.Address
			adapter.StopScan()
		}
	})

	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	if !targetFound {
		return fmt.Errorf("scan completed but device %s not found (scanned %d devices)", b.address, devicesFound)
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
