# MPT-II Thermal Printer - Go Driver

Did you buy a cheap thermal printer from AliExpress or Amazon? And now you want to use it with your own applications? Look no further! The MPT-II Go Driver provides a simple and efficient way to control your thermal printer from Go. 

You can use this is a library, from command line, or via a simple browser-based interface.

Vibe coded with GitHub Copilot Agent using Claude Sonnet 4.

Use at your own risk.

## Features

- ✅ **USB Connection Support** - Using `gousb` library (libusb wrapper)
- ✅ **Bluetooth LE Support** - Using `tinygo.org/x/bluetooth` library  
- ✅ **Complete ESC/POS Implementation** - All basic thermal printer commands
- ✅ **Cross-platform** - Windows, macOS, Linux support
- ✅ **Static Binary** - No runtime dependencies
- ✅ **High Performance** - Native Go performance
- ✅ **Structured Logging** - Uses logrus; -verbose for debug, -quiet to suppress
- ✅ **Clean Architecture** - Modular, testable design
- ✅ **Web Interface** - Built-in web server with GUI and REST API
- ✅ **Markdown Support** - Rich text formatting for documents

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
│   ├── mpt-markdown/    # Print Markdown files with simple formatting
│   └── mpt-web/         # Web server with GUI and API for printing
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

# Build all tools using Makefile (builds mptprinter-cli, mptprint, mpt-markdown, mpt-web)
make            

# Or build manually
go build -o bin/mptprinter-cli ./cmd/mptprinter-cli  # Full-featured CLI
go build -o bin/mptprint ./cmd/mptprint              # Simple print tool
go build -o bin/mpt-markdown ./cmd/mpt-markdown      # Markdown printer
go build -o bin/mpt-web ./cmd/mpt-web                # Web server
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

### 🌐 Web Server (`mpt-web`)

Provides a web interface and REST API for printing with Markdown support:

```bash
# Start web server on default port 8080
./bin/mpt-web

# Start on custom port with options
./bin/mpt-web -port 3000 -width 42 -cut

# Enable verbose logging
./bin/mpt-web -verbose
```

**Web Interface Features:**
- 📱 Responsive web GUI at `http://localhost:8080`
- ✍️ Large text area with Markdown formatting guide
- 🖨️ One-click printing with real-time feedback
- 📖 Built-in help with supported Markdown syntax
- ✅ Success/error messages with auto-hide

**REST API:**
```bash
# Print via JSON API
curl -X POST http://localhost:8080/api/print \
  -H "Content-Type: application/json" \
  -d '{"text":"# Hello World!\nThis is **bold** text."}'

# API Response
{"success": true, "message": "Successfully printed"}
```

**Server Features:**
- 🔄 Persistent printer connection (no reconnection delays)
- 🛡️ Thread-safe concurrent request handling
- 🎯 Graceful shutdown with Ctrl+C
- 📊 Structured logging with configurable levels
- 🌐 Clickable localhost URLs in terminal output

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
