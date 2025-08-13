// USB connection implementation using gousb
package connections

import (
	"context"
	"fmt"
	"log"

	"github.com/google/gousb"
)

// USBConnection implements Connection interface for USB printers
type USBConnection struct {
	vendorID  uint16
	productID uint16
	context   *gousb.Context
	device    *gousb.Device
	iface     *gousb.Interface
	endpoint  *gousb.OutEndpoint
	connected bool
}

// NewUSBConnection creates a new USB connection
func NewUSBConnection(vendorID, productID uint16) (*USBConnection, error) {
	return &USBConnection{
		vendorID:  vendorID,
		productID: productID,
		connected: false,
	}, nil
}

// Connect establishes the USB connection
func (u *USBConnection) Connect(ctx context.Context) error {
	// Initialize libusb context
	u.context = gousb.NewContext()

	// Find the device
	device, err := u.context.OpenDeviceWithVIDPID(gousb.ID(u.vendorID), gousb.ID(u.productID))
	if err != nil {
		return fmt.Errorf("failed to open USB device %04x:%04x: %w", u.vendorID, u.productID, err)
	}
	if device == nil {
		return fmt.Errorf("USB device %04x:%04x not found", u.vendorID, u.productID)
	}

	u.device = device

	// Set auto-detach kernel driver (Linux)
	if err := u.device.SetAutoDetach(true); err != nil {
		log.Printf("Warning: could not set auto-detach: %v", err)
	}

	// Claim the interface (usually interface 0)
	iface, done, err := u.device.DefaultInterface()
	if err != nil {
		u.device.Close()
		return fmt.Errorf("failed to claim interface: %w", err)
	}
	u.iface = iface

	// Find the first available bulk OUT endpoint
	var endpoint *gousb.OutEndpoint

	// Try to find any OUT endpoint (some printers use different endpoint numbers)
	for epNum := 1; epNum <= 15; epNum++ {
		endpoint, err = u.iface.OutEndpoint(epNum)
		if err == nil {
			log.Printf("Found OUT endpoint %d for USB printer", epNum)
			break
		}
	}

	if endpoint == nil {
		done()
		u.device.Close()
		return fmt.Errorf("failed to find any OUT endpoint: %w", err)
	}
	u.endpoint = endpoint

	u.connected = true
	log.Printf("Connected to USB printer %04x:%04x", u.vendorID, u.productID)

	return nil
}

// Write sends data to the USB printer
func (u *USBConnection) Write(data []byte) (int, error) {
	if !u.connected || u.endpoint == nil {
		return 0, fmt.Errorf("USB connection not established")
	}

	// Send data in chunks if necessary (some printers have transfer limits)
	const maxChunkSize = 64
	totalWritten := 0

	for len(data) > 0 {
		chunkSize := len(data)
		if chunkSize > maxChunkSize {
			chunkSize = maxChunkSize
		}

		chunk := data[:chunkSize]
		data = data[chunkSize:]

		written, err := u.endpoint.Write(chunk)
		if err != nil {
			return totalWritten, fmt.Errorf("USB write error: %w", err)
		}

		totalWritten += written
	}

	return totalWritten, nil
}

// Close closes the USB connection
func (u *USBConnection) Close() error {
	if u.iface != nil {
		u.iface.Close()
		u.iface = nil
	}

	if u.device != nil {
		u.device.Close()
		u.device = nil
	}

	if u.context != nil {
		u.context.Close()
		u.context = nil
	}

	u.connected = false
	log.Println("USB connection closed")

	return nil
}

// IsConnected returns true if the USB connection is active
func (u *USBConnection) IsConnected() bool {
	return u.connected
}

// GetInfo returns information about the USB connection
func (u *USBConnection) GetInfo() ConnectionInfo {
	return ConnectionInfo{
		Type:        string(ConnectionTypeUSB),
		Address:     fmt.Sprintf("%04x:%04x", u.vendorID, u.productID),
		Description: fmt.Sprintf("USB Printer (VID: 0x%04X, PID: 0x%04X)", u.vendorID, u.productID),
		Properties: map[string]interface{}{
			"vendor_id":  u.vendorID,
			"product_id": u.productID,
			"connected":  u.connected,
		},
	}
}
