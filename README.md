# MPT-II Thermal Printer - Go Driver

This directory contains a Go implementation of the MPT-II thermal printer driver, providing a fast, native alternative to the Python version.

## Features

- ✅ **USB Connection Support** - Using `gousb` library (libusb wrapper)
- ✅ **Bluetooth LE Support** - Using `tinygo.org/x/bluetooth` library  
- ✅ **Complete ESC/POS Implementation** - All basic thermal printer commands
- ✅ **Cross-platform** - Windows, macOS, Linux support
- ✅ **Static Binary** - No runtime dependencies
- ✅ **High Performance** - Native Go performance
- ✅ **Structured Logging** - Uses logrus; -verbose for debug, -quiet to suppress
- ✅ **Clean Architecture** - Modular, testable design

## Architecture

```
go/
├── pkg/
│   ├── escpos/          # ESC/POS command constants and utilities
│   ├── connections/     # Connection interfaces (USB, Bluetooth)
│   └── printer/         # Core thermal printer logic
├── cmd/
│   ├── mptprinter-cli/  # Full-featured command-line interface
│   ├── mptprint/        # Simple print tool for basic usage
│   └── mpt-markdown/    # Print Markdown files with simple formatting
├── bin/                 # Built executables (after running make)
└── go.mod               # Go module definition
```

## Prerequisites

### For USB Support:
- **Windows**: libusb drivers (can use Zadig tool, if you use MSYS install with pacman)
- **Linux**: libusb-1.0-dev package
- **macOS**: libusb (via Homebrew)

### For Bluetooth Support:
- **Windows**: Built-in Windows BLE support
- **Linux**: BlueZ stack
- **macOS**: Built-in CoreBluetooth support

## Building

```bash
# Download dependencies
go mod tidy

# Build all tools using Makefile (builds mptprinter-cli, mptprint, mpt-markdown)
make            

# Or build manually
go build -o bin/mptprinter-cli ./cmd/mptprinter-cli  # Full-featured CLI
go build -o bin/mptprint ./cmd/mptprint              # Simple print tool
go build -o bin/mpt-markdown ./cmd/mpt-markdown      # Markdown printer
```

## Usage

### 🚀 Simple Print Tool (`mptprint`)

For quick, simple printing tasks:

```bash
# Print simple text
./bin/mptprint "Hello, World!"

# Print from command line
./bin/mptprint "Store Receipt #1234"

# Works great for simple scripts
echo "Order completed at $(date)" | ./bin/mptprint
```

### 🔧 Advanced CLI Tool (`mptprinter-cli`)

For full control and formatting:

```bash
# Print text via USB (default MPT-II VID:PID)
./bin/mptprinter-cli -text "Hello from Go!"

# Print test receipt via USB with custom VID:PID
./bin/mptprinter-cli -vid 0483 -pid 5840 -test

# Print via Bluetooth LE
./bin/mptprinter-cli -type bluetooth -address "XX:XX:XX:XX:XX:XX" -text "Hello BLE!"

# Print bold, centered text with separator
./bin/mptprinter-cli -text "RECEIPT" -bold -align center -separator "=" -cut

# Print from stdin with formatting
echo -e "Line 1\nLine 2\nLine 3" | ./bin/mptprinter-cli -stdin -underline

# Advanced formatting example
./bin/mptprinter-cli -text "BIG TITLE" -double-width -double-height -align center -lines 3

# Quiet mode for scripts
./bin/mptprinter-cli -text "Silent print" -quiet
```

### As a Go Library

```go
package main

import (
    "context"
    log "github.com/sirupsen/logrus"
    
    "github.com/aabmar/mpt2/go/pkg/connections"
    "github.com/aabmar/mpt2/go/pkg/printer"
    "github.com/aabmar/mpt2/go/pkg/escpos"
)

func main() {
    // Create USB connection
    factory := connections.NewConnectionFactory()
    conn, err := factory.CreateUSBConnection(connections.USBConnectionParams{
        VendorID:  0x0483,
        ProductID: 0x5840,
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Create printer
    thermalPrinter := printer.NewThermalPrinter(conn)
    
    // Connect and print
    ctx := context.Background()
    if err := thermalPrinter.Connect(ctx); err != nil {
        log.Fatal(err)
    }
    defer thermalPrinter.Disconnect()
    
    // Print formatted text
    thermalPrinter.Initialize()
    thermalPrinter.SetAlign(escpos.AlignCenter)
    thermalPrinter.SetBold(true)
    thermalPrinter.PrintText("Hello from Go!")
    thermalPrinter.Feed(2)
}
```

### 📝 Markdown Printer (`mpt-markdown`)

Render and print a Markdown file using ESC/POS formatting:

```bash
./bin/mpt-markdown README.md         # default 32 columns (58mm)
./bin/mpt-markdown -width 42 doc.md  # wider paper (80mm printers)
./bin/mpt-markdown -cut doc.md       # cut after printing
```
Supported Markdown subset:
- Headings (#, ##, ### and setext underlines) mapped to bold/size and centered
- Bold/italic/combined (uses bold + underline)
- Horizontal rules (---, ***, ___)
- Lists (unordered -, +, * and ordered 1.) with basic wrapping
- Blockquotes (prefixed with | and underlined)
- Code blocks (indented)
- Links and images printed as text with URL

## Library Comparison

| Feature | Python Version | Go Version |
|---------|---------------|------------|
| **Performance** | Interpreted | Native binary |
| **Dependencies** | pyusb, bleak, tkinter | Static binary |
| **Memory Usage** | Higher | Lower |
| **Startup Time** | Slower | Instant |
| **Distribution** | Requires Python | Single executable |
| **Cross-compilation** | No | Yes |

## Dependencies

- **USB**: `github.com/google/gousb` - Mature libusb wrapper
- **Bluetooth**: `tinygo.org/x/bluetooth` - Cross-platform BLE library
- **Standard Library**: Only Go standard library for core functionality

## Known Printer Compatibility

- ✅ MPT-II (VID: 0x0483, PID: 0x5840)
- ✅ Generic 58mm thermal printers
- ✅ ESC/POS compatible printers

## Platform Support

| Platform | USB | Bluetooth LE |
|----------|-----|--------------|
| **Windows** | ✅ | ✅ |
| **macOS** | ✅ | ✅ |
| **Linux** | ✅ | ✅ |

## Troubleshooting

### USB Issues
- Ensure libusb is installed
- On Windows, use Zadig to install WinUSB driver
- Run with administrator privileges if needed

### Bluetooth Issues  
- Ensure printer is in pairing/discoverable mode
- Check Bluetooth is enabled on host system
- Verify MAC address format (XX:XX:XX:XX:XX:XX)

## Future Enhancements

- [ ] Device discovery functionality
- [ ] GUI application using Fyne
- [ ] Advanced ESC/POS commands (images, barcodes)
- [ ] Configuration file support
- [ ] Logging and debugging improvements
