// Bluetooth connection implementation (currently a stub)
package connections

import (
	"context"
	"fmt"
)

// BluetoothConnection implements Connection interface for Bluetooth LE printers
// Note: This is currently a stub implementation
type BluetoothConnection struct {
	address   string
	connected bool
}

// NewBluetoothConnection creates a new Bluetooth connection
func NewBluetoothConnection(address string) (*BluetoothConnection, error) {
	return &BluetoothConnection{
		address:   address,
		connected: false,
	}, nil
}

// Connect establishes the Bluetooth connection
func (b *BluetoothConnection) Connect(ctx context.Context) error {
	return fmt.Errorf("Bluetooth support is not yet implemented - use USB connection instead")
}

// Write sends data to the Bluetooth printer
func (b *BluetoothConnection) Write(data []byte) (int, error) {
	return 0, fmt.Errorf("Bluetooth support is not yet implemented - use USB connection instead")
}

// Close closes the Bluetooth connection
func (b *BluetoothConnection) Close() error {
	b.connected = false
	return nil
}

// IsConnected returns true if the Bluetooth connection is active
func (b *BluetoothConnection) IsConnected() bool {
	return b.connected
}

// GetInfo returns information about the Bluetooth connection
func (b *BluetoothConnection) GetInfo() ConnectionInfo {
	return ConnectionInfo{
		Type:        string(ConnectionTypeBluetooth),
		Address:     b.address,
		Description: fmt.Sprintf("Bluetooth LE Printer (%s) - Not Implemented", b.address),
		Properties: map[string]interface{}{
			"address":     b.address,
			"connected":   b.connected,
			"implemented": false,
		},
	}
}
