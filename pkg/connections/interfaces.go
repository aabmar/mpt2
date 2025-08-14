// Package connections provides connection interfaces and implementations for printer communication
package connections

import (
	"context"
	"io"
)

// Connection represents a generic connection to a printer
type Connection interface {
	io.Writer
	io.Closer
	Connect(ctx context.Context) error
	IsConnected() bool
	GetInfo() ConnectionInfo
}

// ConnectionInfo provides information about a connection
type ConnectionInfo struct {
	Type        string
	Address     string
	Description string
	Properties  map[string]interface{}
}

// ConnectionType represents the type of connection
type ConnectionType string

const (
	ConnectionTypeUSB       ConnectionType = "usb"
	ConnectionTypeBluetooth ConnectionType = "bluetooth"
)

// USBConnectionParams holds parameters for USB connections
type USBConnectionParams struct {
	VendorID  uint16
	ProductID uint16
}

// BluetoothConnectionParams holds parameters for Bluetooth connections
type BluetoothConnectionParams struct {
	Address string // MAC address like "XX:XX:XX:XX:XX:XX"
	Verbose bool   // Enable verbose BLE logging
}

// ConnectionFactory creates connections based on type and parameters
type ConnectionFactory struct{}

// NewConnectionFactory creates a new connection factory
func NewConnectionFactory() *ConnectionFactory {
	return &ConnectionFactory{}
}

// CreateUSBConnection creates a USB connection with the given parameters
func (f *ConnectionFactory) CreateUSBConnection(params USBConnectionParams) (Connection, error) {
	return NewUSBConnection(params.VendorID, params.ProductID)
}

// CreateBluetoothConnection creates a Bluetooth connection with the given parameters
func (f *ConnectionFactory) CreateBluetoothConnection(params BluetoothConnectionParams) (Connection, error) {
	return NewBluetoothConnection(params.Address, params.Verbose)
}
