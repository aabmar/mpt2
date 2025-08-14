// Bluetooth connection implementation using tinygo.org/x/bluetooth
package connections

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

// BluetoothConnection implements Connection interface for Bluetooth LE printers
type BluetoothConnection struct {
	address   string
	connected bool
	device    *bluetooth.Device
	service   *bluetooth.DeviceService
	writeChar *bluetooth.DeviceCharacteristic
	// candidates holds all matched writable printer characteristics in priority order
	candidates []*bluetooth.DeviceCharacteristic
	mutex      sync.RWMutex
}

// Common service UUIDs for thermal printers
var (
	// Generic Access Profile
	printerServiceUUID = bluetooth.ServiceUUIDGenericAccess
	// Many thermal printers use a custom service or Nordic UART service
	nordicUARTServiceUUID, _ = bluetooth.ParseUUID("6E400001-B5A3-F393-E0A9-E50E24DCCA9E")
	nordicUARTTxUUID, _      = bluetooth.ParseUUID("6E400002-B5A3-F393-E0A9-E50E24DCCA9E") // Write characteristic
)

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
func NewBluetoothConnection(address string) (*BluetoothConnection, error) {
	// Validate MAC address format
	if !isValidMACAddress(address) {
		return nil, fmt.Errorf("invalid MAC address format: %s (expected XX:XX:XX:XX:XX:XX)", address)
	}

	return &BluetoothConnection{
		address:   strings.ToUpper(address),
		connected: false,
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

	// Use scan-based approach only (more reliable on Windows)
	return b.connectViaScan(ctx, adapter)
}

// connectViaScan attempts connection via device discovery
func (b *BluetoothConnection) connectViaScan(ctx context.Context, adapter *bluetooth.Adapter) error {
	fmt.Printf("Scanning for Bluetooth device %s...\n", b.address)
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
		deviceName := "Unknown"
		if scanResult.LocalName() != "" {
			deviceName = scanResult.LocalName()
		}

		fmt.Printf("Found device: %s (%s)\n", deviceAddr, deviceName)

		// Try both exact match and case-insensitive match
		if deviceAddr == b.address || strings.EqualFold(deviceAddr, b.address) {
			fmt.Printf("Target device found! Stopping scan and connecting...\n")
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

	fmt.Printf("Attempting to connect to %s...\n", b.address)

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
		if device != nil {
			device.Disconnect()
		}
		return fmt.Errorf("connection to device %s timed out", b.address)
	default:
		// Connection successful
	}

	fmt.Printf("Successfully connected to device %s\n", b.address)
	b.device = device
	return b.setupCharacteristics()
}

// setupCharacteristics discovers services and sets up the write characteristic
func (b *BluetoothConnection) setupCharacteristics() error {
	fmt.Printf("Discovering services on device %s...\n", b.address)

	// Discover services
	services, err := b.device.DiscoverServices(nil)
	if err != nil {
		b.device.Disconnect()
		return fmt.Errorf("device found but failed to discover services: %w", err)
	}

	fmt.Printf("Found %d services on device\n", len(services))

	// Try each service to find a suitable characteristic
	var allCharacteristics []bluetooth.DeviceCharacteristic

	for i, svc := range services {
		fmt.Printf("Service %d: %s\n", i+1, svc.UUID().String())

		// Discover characteristics for this service
		chars, err := svc.DiscoverCharacteristics(nil)
		if err != nil {
			fmt.Printf("  Failed to discover characteristics: %v\n", err)
			continue
		}

		fmt.Printf("  Found %d characteristics:\n", len(chars))
		for j, char := range chars {
			fmt.Printf("    Characteristic %d: %s\n", j+1, char.UUID().String())
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
				fmt.Printf("Found matching printer characteristic: %s\n", knownUUID)
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

	fmt.Printf("Successfully connected to device %s using characteristic %s\n", b.address, matchedUUID)
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

		fmt.Printf("Writing %d bytes to characteristic...\n", len(chunk))

		// Try WriteWithoutResponse first, then fall back to regular Write
		_, err := b.writeChar.WriteWithoutResponse(chunk)
		if err != nil {
			fmt.Printf("WriteWithoutResponse failed (%v), trying regular Write...\n", err)
			// Try regular Write method
			if _, werr := b.writeChar.Write(chunk); werr != nil {
				// If write not supported, try next candidate characteristic if available
				// We match on error string due to tinygo API
				if strings.Contains(strings.ToLower(werr.Error()), "write not supported") && len(b.candidates) > 1 {
					fmt.Printf("Current characteristic %s rejected writes; trying alternate characteristic...\n", b.writeChar.UUID().String())
					for idx := 1; idx < len(b.candidates); idx++ {
						alt := b.candidates[idx]
						fmt.Printf("Switching to characteristic %s...\n", alt.UUID().String())
						b.writeChar = alt
						if _, aerr := b.writeChar.WriteWithoutResponse(chunk); aerr == nil {
							fmt.Printf("Alternate characteristic WriteWithoutResponse succeeded for %d bytes\n", len(chunk))
							werr = nil
							break
						}
						if _, aerr := b.writeChar.Write(chunk); aerr == nil {
							fmt.Printf("Alternate characteristic Write succeeded for %d bytes\n", len(chunk))
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
				fmt.Printf("Regular Write succeeded for %d bytes\n", len(chunk))
			}
		} else {
			fmt.Printf("WriteWithoutResponse succeeded for %d bytes\n", len(chunk))
		}

		totalWritten += chunkSize

		// Small delay between chunks to avoid overwhelming the device
		if len(data) > 0 {
			time.Sleep(50 * time.Millisecond) // Slightly longer delay for regular Write
		}
	}

	fmt.Printf("Successfully wrote %d bytes total to printer\n", totalWritten)
	return totalWritten, nil
}

// Close closes the Bluetooth connection
func (b *BluetoothConnection) Close() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.device != nil && b.connected {
		err := b.device.Disconnect()
		b.connected = false
		b.device = nil
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
