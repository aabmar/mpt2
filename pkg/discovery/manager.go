// Package discovery provides connection management with auto-discovery and fallback
package discovery

import (
	"context"
	"fmt"
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
	BluetoothAddress string

	// Behavior options
	UseCache bool // Try cached device first
	Verbose  bool // Enable verbose logging
	Timeout  time.Duration

	// Printer options
	CodePage int // ESC/POS code page (-1 for default)
}

// DefaultConnectOptions returns sensible defaults
func DefaultConnectOptions() ConnectOptions {
	return ConnectOptions{
		BluetoothAddress: "", // Auto-scan
		UseCache:         true,
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

	// 1. Cache first (if enabled)
	if opts.UseCache {
		if cacheCandidate := m.buildCacheCandidate(opts); cacheCandidate != nil {
			candidates = append(candidates, *cacheCandidate)
		}
	}

	// 2. Bluetooth connection
	candidates = append(candidates, m.buildBluetoothCandidate(opts))

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

	if info.Type == "bluetooth" {
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

// ConnectBluetooth is a convenience method for direct Bluetooth connection
func (m *ConnectionManager) ConnectBluetooth(ctx context.Context, address string, verbose bool, codePage int) (*printer.ThermalPrinter, error) {
	opts := ConnectOptions{
		BluetoothAddress: address,
		UseCache:         false,
		Verbose:          verbose,
		Timeout:          30 * time.Second,
		CodePage:         codePage,
	}

	return m.ConnectAuto(ctx, opts)
}
