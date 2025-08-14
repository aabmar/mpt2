// Simple command-line tool for quick thermal printing
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aabmar/mpt2/go/pkg/connections"
	"github.com/aabmar/mpt2/go/pkg/printer"
)

func main() {
	// Optional: allow code page via env or simple flag-like env var for minimal tool
	cp := -1
	if v := os.Getenv("MPT_CODEPAGE"); v != "" {
		var n int
		_, _ = fmt.Sscanf(v, "%d", &n)
		cp = n
	}
	// Simple usage: mptprint "text to print"
	if len(os.Args) < 2 {
		fmt.Println("Usage: mptprint \"text to print\"")
		fmt.Println("       echo \"text\" | mptprint")
		fmt.Println("")
		fmt.Println("Simple thermal printer tool. Uses default USB connection (VID:0483 PID:5840)")
		fmt.Println("For advanced options, use: mptprinter-cli")
		os.Exit(1)
	}

	// Simple tool outputs user messages to stderr/stdout via fmt only

	// Get text to print
	var textToPrint string
	if len(os.Args) >= 2 {
		textToPrint = os.Args[1]
	}

	// Use default USB connection (MPT-II)
	factory := connections.NewConnectionFactory()
	conn, err := factory.CreateUSBConnection(connections.USBConnectionParams{
		VendorID:  0x0483,
		ProductID: 0x5840,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create USB connection: %v\n", err)
		os.Exit(1)
	}

	// Create printer
	thermalPrinter := printer.NewThermalPrinter(conn)
	if cp >= 0 && cp <= 255 {
		thermalPrinter.SetCodePage(byte(cp))
	}

	// Connect with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := thermalPrinter.Connect(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to connect to printer: %v\n", err)
		fmt.Fprintf(os.Stderr, "Make sure your MPT-II printer is connected via USB\n")
		os.Exit(1)
	}
	defer thermalPrinter.Disconnect()

	// Initialize and print
	if err := thermalPrinter.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize printer: %v\n", err)
		os.Exit(1)
	}

	if err := thermalPrinter.PrintText(textToPrint); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to print text: %v\n", err)
		os.Exit(1)
	}

	// Feed 2 lines
	if err := thermalPrinter.Feed(2); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to feed paper: %v\n", err)
	}

	fmt.Println("✓ Printed successfully")
}
