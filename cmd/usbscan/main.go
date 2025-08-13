// USB device scanner to help debug printer connections
package main

import (
	"fmt"
	"log"

	"github.com/google/gousb"
)

func main() {
	fmt.Println("MPT-II USB Device Scanner")
	fmt.Println("========================")

	// Initialize libusb context
	ctx := gousb.NewContext()
	defer ctx.Close()

	// List all USB devices
	devices, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return true // Return all devices
	})
	if err != nil {
		log.Fatalf("Failed to enumerate USB devices: %v", err)
	}
	defer func() {
		for _, dev := range devices {
			dev.Close()
		}
	}()

	fmt.Printf("Found %d USB devices:\n\n", len(devices))

	targetFound := false
	for i, dev := range devices {
		desc := dev.Desc

		// Get string descriptors if available
		var manufacturer, product, serial string
		if desc.Manufacturer != 0 {
			if m, err := dev.GetStringDescriptor(desc.Manufacturer); err == nil {
				manufacturer = m
			}
		}
		if desc.Product != 0 {
			if p, err := dev.GetStringDescriptor(desc.Product); err == nil {
				product = p
			}
		}
		if desc.SerialNumber != 0 {
			if s, err := dev.GetStringDescriptor(desc.SerialNumber); err == nil {
				serial = s
			}
		}

		isTarget := desc.Vendor == 0x0483 && desc.Product == 0x5840
		if isTarget {
			targetFound = true
			fmt.Printf(">>> TARGET DEVICE FOUND! <<<\n")
		}

		fmt.Printf("Device %d:\n", i+1)
		fmt.Printf("  VID:PID     = %04X:%04X\n", desc.Vendor, desc.Product)
		fmt.Printf("  Class       = %02X (subclass: %02X, protocol: %02X)\n",
			desc.Class, desc.SubClass, desc.Protocol)
		fmt.Printf("  Manufacturer= %s\n", manufacturer)
		fmt.Printf("  Product     = %s\n", product)
		fmt.Printf("  Serial      = %s\n", serial)
		fmt.Printf("  USB Version = %s\n", desc.USB)
		fmt.Printf("  Device Ver  = %s\n", desc.Device)

		if isTarget {
			fmt.Printf("  ** This is your MPT-II printer! **\n")
		}
		fmt.Println()
	}

	fmt.Println("========================")
	if targetFound {
		fmt.Println("✓ MPT-II printer (0483:5840) found!")
		fmt.Println("If you're getting 'not supported' errors, install WinUSB driver using Zadig.")
	} else {
		fmt.Println("✗ MPT-II printer (0483:5840) not found.")
		fmt.Println("Make sure your printer is connected and powered on.")
	}

	// Test opening the target device specifically
	fmt.Println("\nTesting direct connection to MPT-II...")
	device, err := ctx.OpenDeviceWithVIDPID(0x0483, 0x5840)
	if err != nil {
		fmt.Printf("✗ Failed to open MPT-II device: %v\n", err)
		fmt.Println("This confirms you need to install WinUSB driver using Zadig.")
	} else if device == nil {
		fmt.Println("✗ MPT-II device not found (no device returned)")
	} else {
		fmt.Println("✓ Successfully opened MPT-II device!")
		device.Close()
	}
}
