// Package discovery provides connection management with auto-discovery and fallback
package discovery

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aabmar/mpt2/go/pkg/configstore"
	"github.com/aabmar/mpt2/go/pkg/connections"
	"github.com/aabmar/mpt2/go/pkg/printer"
	log "github.com/sirupsen/logrus"
)

// ConnectionManager handles smart device discovery and connection with fallback
type ConnectionManager struct {
	factory *connections.ConnectionFactory
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		factory: connections.NewConnectionFactory(),
	}
}

// ConnectOptions configures how to connect to a printer
type ConnectOptions struct {
	// Connection preferences
	PreferredType    string // "usb", "bluetooth", or "" for auto
	USBVendorID      uint16
	USBProductID     uint16
	BluetoothAddress string

	// Behavior options
	UseCache       bool // Try cached device first
	EnableFallback bool // Try other connection types if preferred fails
	Verbose        bool // Enable verbose logging
	Timeout        time.Duration

	// Printer options
	CodePage int // ESC/POS code page (-1 for default)
}

// DefaultConnectOptions returns sensible defaults
func DefaultConnectOptions() ConnectOptions {
	return ConnectOptions{
		PreferredType:    "",     // Auto-detect
		USBVendorID:      0x0483, // MPT-II default
		USBProductID:     0x5840, // MPT-II default
		BluetoothAddress: "",     // Auto-scan
		UseCache:         true,
		EnableFallback:   true,
		Verbose:          false,
		Timeout:          30 * time.Second,
		CodePage:         -1,
	}
}

// ConnectAuto attempts to connect to a printer using smart discovery and fallback
func (m *ConnectionManager) ConnectAuto(ctx context.Context, opts ConnectOptions) (*printer.ThermalPrinter, error) {
	// Build candidate list
	candidates, err := m.buildCandidates(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build connection candidates: %w", err)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no connection candidates available")
	}

	// Try each candidate until one works
	var lastErr error
	for i, candidate := range candidates {
		if opts.Verbose {
			log.Infof("Trying connection %d/%d: %s", i+1, len(candidates), candidate.Description)
		} else {
			log.Infof("Connecting to printer via %s...", candidate.Type)
		}

		conn, err := candidate.Connect()
		if err != nil {
			if opts.Verbose {
				log.WithError(err).Warnf("Connection candidate %d failed", i+1)
			} else {
				log.WithError(err).Warnf("Connection via %s failed; trying next option", candidate.Type)
			}
			lastErr = err
			continue
		}

		// Create printer and attempt connection
		thermalPrinter := printer.NewThermalPrinter(conn)
		if opts.CodePage >= 0 && opts.CodePage <= 255 {
			thermalPrinter.SetCodePage(byte(opts.CodePage))
		}

		connectCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
		err = thermalPrinter.Connect(connectCtx)
		cancel()

		if err != nil {
			log.WithError(err).Warnf("Printer connection failed; trying next option")
			conn.Close()
			lastErr = err
			continue
		}

		// Success! Initialize and cache the connection
		if err := thermalPrinter.Initialize(); err != nil {
			log.WithError(err).Warn("Printer initialization failed")
			thermalPrinter.Disconnect()
			lastErr = err
			continue
		}

		log.Info("Connected successfully!")

		// Save to cache for next time
		if opts.UseCache {
			m.saveToCache(thermalPrinter, candidate.Type, opts)
		}

		return thermalPrinter, nil
	}

	return nil, fmt.Errorf("failed to connect to any printer: %w", lastErr)
}

// ConnectionCandidate represents a potential printer connection
type ConnectionCandidate struct {
	Type        string
	Description string
	Connect     func() (connections.Connection, error)
}

// buildCandidates creates a prioritized list of connection attempts
func (m *ConnectionManager) buildCandidates(opts ConnectOptions) ([]ConnectionCandidate, error) {
	var candidates []ConnectionCandidate

	// 1. Cache first (if enabled and no specific type preference)
	if opts.UseCache && opts.PreferredType == "" {
		if cacheCandidate := m.buildCacheCandidate(opts); cacheCandidate != nil {
			candidates = append(candidates, *cacheCandidate)
		}
	}

	// 2. Preferred type (if specified)
	if opts.PreferredType != "" {
		switch strings.ToLower(opts.PreferredType) {
		case "usb":
			candidates = append(candidates, m.buildUSBCandidate(opts))
		case "bluetooth", "ble":
			candidates = append(candidates, m.buildBluetoothCandidate(opts))
		default:
			return nil, fmt.Errorf("unknown connection type: %s", opts.PreferredType)
		}
	} else if opts.EnableFallback {
		// 3. Default fallback order: USB first, then Bluetooth
		candidates = append(candidates, m.buildUSBCandidate(opts))
		candidates = append(candidates, m.buildBluetoothCandidate(opts))
	}

	return candidates, nil
}

// buildCacheCandidate creates a candidate from cached device info
func (m *ConnectionManager) buildCacheCandidate(opts ConnectOptions) *ConnectionCandidate {
	last, err := configstore.LoadLastDevice()
	if err != nil {
		if opts.Verbose {
			log.WithError(err).Debug("No cached device found")
		}
		return nil
	}

	if last.Type == "usb" && last.USB != nil {
		usb := *last.USB
		return &ConnectionCandidate{
			Type:        "usb",
			Description: fmt.Sprintf("cached USB device %04x:%04x", usb.VendorID, usb.ProductID),
			Connect: func() (connections.Connection, error) {
				return m.factory.CreateUSBConnection(connections.USBConnectionParams{
					VendorID:  usb.VendorID,
					ProductID: usb.ProductID,
				})
			},
		}
	}

	if last.Type == "bluetooth" && last.BLE != nil {
		ble := *last.BLE
		return &ConnectionCandidate{
			Type:        "bluetooth",
			Description: fmt.Sprintf("cached Bluetooth device %s", ble.Address),
			Connect: func() (connections.Connection, error) {
				return m.factory.CreateBluetoothConnection(connections.BluetoothConnectionParams{
					Address: ble.Address,
					Verbose: opts.Verbose,
				})
			},
		}
	}

	return nil
}

// buildUSBCandidate creates a USB connection candidate
func (m *ConnectionManager) buildUSBCandidate(opts ConnectOptions) ConnectionCandidate {
	return ConnectionCandidate{
		Type:        "usb",
		Description: fmt.Sprintf("USB device %04x:%04x", opts.USBVendorID, opts.USBProductID),
		Connect: func() (connections.Connection, error) {
			return m.factory.CreateUSBConnection(connections.USBConnectionParams{
				VendorID:  opts.USBVendorID,
				ProductID: opts.USBProductID,
			})
		},
	}
}

// buildBluetoothCandidate creates a Bluetooth connection candidate
func (m *ConnectionManager) buildBluetoothCandidate(opts ConnectOptions) ConnectionCandidate {
	desc := "Bluetooth auto-scan"
	if opts.BluetoothAddress != "" {
		desc = fmt.Sprintf("Bluetooth device %s", opts.BluetoothAddress)
	}

	return ConnectionCandidate{
		Type:        "bluetooth",
		Description: desc,
		Connect: func() (connections.Connection, error) {
			return m.factory.CreateBluetoothConnection(connections.BluetoothConnectionParams{
				Address: opts.BluetoothAddress,
				Verbose: opts.Verbose,
			})
		},
	}
}

// saveToCache saves successful connection info to cache
func (m *ConnectionManager) saveToCache(printer *printer.ThermalPrinter, connType string, opts ConnectOptions) {
	info := printer.GetConnectionInfo()

	if info.Type == "usb" {
		usbInfo := &configstore.LastDevice{
			Type: "usb",
			USB: &configstore.USBInfo{
				VendorID:  uint16FromProps(info.Properties["vendor_id"]),
				ProductID: uint16FromProps(info.Properties["product_id"]),
			},
		}
		if err := configstore.SaveLastDevice(usbInfo); err != nil && opts.Verbose {
			log.WithError(err).Debug("Failed to save USB device to cache")
		}
	} else if info.Type == "bluetooth" {
		addr := ""
		if v, ok := info.Properties["address"].(string); ok {
			addr = v
		} else {
			addr = info.Address
		}

		bleInfo := &configstore.LastDevice{
			Type: "bluetooth",
			BLE: &configstore.BLEInfo{
				Address: strings.ToUpper(addr),
			},
		}
		if err := configstore.SaveLastDevice(bleInfo); err != nil && opts.Verbose {
			log.WithError(err).Debug("Failed to save Bluetooth device to cache")
		}
	}
}

// uint16FromProps converts interface{} to uint16 for VID/PID
func uint16FromProps(v interface{}) uint16 {
	switch t := v.(type) {
	case uint16:
		return t
	case int:
		return uint16(t)
	case int32:
		return uint16(t)
	case int64:
		return uint16(t)
	case float64:
		return uint16(t)
	case string:
		// Try hex first
		if strings.HasPrefix(t, "0x") || strings.HasPrefix(t, "0X") {
			if n, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(t), "0x"), 16, 16); err == nil {
				return uint16(n)
			}
		}
		if n, err := strconv.ParseUint(t, 10, 16); err == nil {
			return uint16(n)
		}
	}
	return 0
}

// ConnectUSB is a convenience method for direct USB connection
func (m *ConnectionManager) ConnectUSB(ctx context.Context, vendorID, productID uint16, codePage int) (*printer.ThermalPrinter, error) {
	opts := ConnectOptions{
		PreferredType:  "usb",
		USBVendorID:    vendorID,
		USBProductID:   productID,
		UseCache:       false,
		EnableFallback: false,
		Timeout:        15 * time.Second,
		CodePage:       codePage,
	}

	return m.ConnectAuto(ctx, opts)
}

// ConnectBluetooth is a convenience method for direct Bluetooth connection
func (m *ConnectionManager) ConnectBluetooth(ctx context.Context, address string, verbose bool, codePage int) (*printer.ThermalPrinter, error) {
	opts := ConnectOptions{
		PreferredType:    "bluetooth",
		BluetoothAddress: address,
		UseCache:         false,
		EnableFallback:   false,
		Verbose:          verbose,
		Timeout:          30 * time.Second,
		CodePage:         codePage,
	}

	return m.ConnectAuto(ctx, opts)
}
