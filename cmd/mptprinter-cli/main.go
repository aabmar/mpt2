// Command-line interface for the Go thermal printer driver
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aabmar/mpt2/go/pkg/configstore"
	"github.com/aabmar/mpt2/go/pkg/connections"
	"github.com/aabmar/mpt2/go/pkg/escpos"
	"github.com/aabmar/mpt2/go/pkg/printer"
	log "github.com/sirupsen/logrus"
)

func main() {
	// Define command-line flags
	var (
		connectionType = flag.String("type", "usb", "Connection type: usb or bluetooth")
		usbVID         = flag.String("vid", "0483", "USB Vendor ID (hex)")
		usbPID         = flag.String("pid", "5840", "USB Product ID (hex)")
		btAddress      = flag.String("address", "", "Bluetooth MAC address (XX:XX:XX:XX:XX:XX). If omitted for bluetooth, auto-scan for a compatible printer.")
		text           = flag.String("text", "", "Text to print")
		testReceipt    = flag.Bool("test", false, "Print a test receipt")
		bold           = flag.Bool("bold", false, "Use bold text")
		underline      = flag.Bool("underline", false, "Use underlined text")
		doubleWidth    = flag.Bool("double-width", false, "Use double width text")
		doubleHeight   = flag.Bool("double-height", false, "Use double height text")
		align          = flag.String("align", "left", "Text alignment: left, center, right")
		lines          = flag.Int("lines", 2, "Number of lines to feed after printing")
		cut            = flag.Bool("cut", false, "Cut paper after printing")
		fromStdin      = flag.Bool("stdin", false, "Read text from stdin instead of -text")
		separator      = flag.String("separator", "", "Print a separator line with this character")
		quiet          = flag.Bool("quiet", false, "Suppress log output")
		verbose        = flag.Bool("verbose", false, "Enable verbose BLE logging (scan/services/characteristics)")
		codepage       = flag.Int("codepage", -1, "ESC/POS code page number (e.g., 0=PC437,2=PC850,5=PC865,16=WPC1252,19=PC858)")
		help           = flag.Bool("help", false, "Show help")
		clearCache     = flag.Bool("clear-cache", false, "Clear cached device selection and exit")
	)

	flag.Parse()

	if *help {
		showHelp()
		return
	}

	// Handle cache clearing early and exit
	if *clearCache {
		if err := configstore.ClearLastDevice(); err != nil {
			log.Fatalf("Failed to clear cache: %v", err)
		}
		log.Info("Device cache cleared.")
		return
	}

	// Configure logging (logrus)
	// -quiet: suppress all output
	// -verbose: enable debug-level logs (BLE scan/services/characteristics)
	if *quiet {
		log.SetOutput(io.Discard)
	} else {
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
		if *verbose {
			log.SetLevel(log.DebugLevel)
		} else {
			log.SetLevel(log.InfoLevel)
		}
	}

	// Get text to print
	var textToPrint string
	if *fromStdin {
		// Read from stdin
		content, err := readFromStdin()
		if err != nil {
			log.Fatalf("Error reading from stdin: %v", err)
		}
		textToPrint = content
	} else if *text != "" {
		textToPrint = *text
	} else if !*testReceipt {
		fmt.Println("Error: Either -text, -stdin, or -test must be specified")
		showHelp()
		os.Exit(1)
	}

	// Create connection based on type with cache and auto-detection & fallback
	factory := connections.NewConnectionFactory()
	var thermalPrinter *printer.ThermalPrinter
	selectedType := strings.ToLower(*connectionType)

	// Determine if -type flag was explicitly set
	flagWasProvided := false
	flag.CommandLine.Visit(func(f *flag.Flag) {
		if f.Name == "type" {
			flagWasProvided = true
		}
	})

	// Build candidate list
	type candidate struct {
		typ   string
		build func() (connections.Connection, error)
	}
	var candidates []candidate

	if !flagWasProvided {
		// 1) Cache (if present)
		if last, cerr := configstore.LoadLastDevice(); cerr == nil && last != nil {
			if last.Type == "usb" && last.USB != nil {
				u := *last.USB
				candidates = append(candidates, candidate{
					typ: "usb",
					build: func() (connections.Connection, error) {
						return factory.CreateUSBConnection(connections.USBConnectionParams{VendorID: u.VendorID, ProductID: u.ProductID})
					},
				})
			} else if last.Type == "bluetooth" && last.BLE != nil {
				b := *last.BLE
				candidates = append(candidates, candidate{
					typ: "bluetooth",
					build: func() (connections.Connection, error) {
						return factory.CreateBluetoothConnection(connections.BluetoothConnectionParams{Address: b.Address, Verbose: *verbose})
					},
				})
			}
		}
		// 2) USB default (flags default to 0483:5840)
		candidates = append(candidates, candidate{
			typ: "usb",
			build: func() (connections.Connection, error) {
				vid, vErr := parseHex(*usbVID)
				if vErr != nil {
					return nil, vErr
				}
				pid, pErr := parseHex(*usbPID)
				if pErr != nil {
					return nil, pErr
				}
				return factory.CreateUSBConnection(connections.USBConnectionParams{VendorID: uint16(vid), ProductID: uint16(pid)})
			},
		})
		// 3) BLE (address optional; auto-scan if empty)
		candidates = append(candidates, candidate{
			typ: "bluetooth",
			build: func() (connections.Connection, error) {
				return factory.CreateBluetoothConnection(connections.BluetoothConnectionParams{Address: *btAddress, Verbose: *verbose})
			},
		})
	} else {
		// Respect explicit -type
		switch selectedType {
		case "usb":
			candidates = append(candidates, candidate{
				typ: "usb",
				build: func() (connections.Connection, error) {
					vid, vErr := parseHex(*usbVID)
					if vErr != nil {
						return nil, vErr
					}
					pid, pErr := parseHex(*usbPID)
					if pErr != nil {
						return nil, pErr
					}
					return factory.CreateUSBConnection(connections.USBConnectionParams{VendorID: uint16(vid), ProductID: uint16(pid)})
				},
			})
		case "bluetooth", "ble":
			candidates = append(candidates, candidate{
				typ: "bluetooth",
				build: func() (connections.Connection, error) {
					return factory.CreateBluetoothConnection(connections.BluetoothConnectionParams{Address: *btAddress, Verbose: *verbose})
				},
			})
		default:
			log.Fatalf("Unknown connection type: %s", *connectionType)
		}
	}

	// Try candidates in order until one connects
	var connectErr error
	for _, c := range candidates {
		conn, berr := c.build()
		if berr != nil {
			log.WithError(berr).Debugf("Skipping %s candidate (build error)", c.typ)
			connectErr = berr
			continue
		}
		tp := printer.NewThermalPrinter(conn)
		if *codepage >= 0 && *codepage <= 255 {
			tp.SetCodePage(byte(*codepage))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		log.Infof("Connecting to printer via %s...", c.typ)
		if err := tp.Connect(ctx); err != nil {
			cancel()
			log.WithError(err).Warnf("Connection via %s failed; trying next option", c.typ)
			_ = conn.Close()
			connectErr = err
			continue
		}
		cancel()
		thermalPrinter = tp
		selectedType = c.typ
		break
	}

	if thermalPrinter == nil {
		log.Fatalf("Failed to connect to any printer candidate: %v", connectErr)
	}
	defer thermalPrinter.Disconnect()

	log.Info("Connected successfully!")

	// Initialize printer
	if err := thermalPrinter.Initialize(); err != nil {
		log.Fatalf("Failed to initialize printer: %v", err)
	}

	// On successful connection, save cache
	info := thermalPrinter.GetConnectionInfo()
	if info.Type == "usb" {
		_ = configstore.SaveLastDevice(&configstore.LastDevice{Type: "usb", USB: &configstore.USBInfo{VendorID: uint16FromProps(info.Properties["vendor_id"]), ProductID: uint16FromProps(info.Properties["product_id"])}})
	} else if info.Type == "bluetooth" {
		addr := ""
		if v, ok := info.Properties["address"].(string); ok {
			addr = v
		} else {
			addr = info.Address
		}
		_ = configstore.SaveLastDevice(&configstore.LastDevice{Type: "bluetooth", BLE: &configstore.BLEInfo{Address: strings.ToUpper(addr)}})
	}

	if *testReceipt {
		// Print test receipt
		if err := printTestReceipt(thermalPrinter); err != nil {
			log.Fatalf("Failed to print test receipt: %v", err)
		}
		log.Info("Test receipt printed successfully!")
	} else {
		// Print separator line if requested
		if *separator != "" {
			if err := printSeparator(thermalPrinter, *separator); err != nil {
				log.Fatalf("Failed to print separator: %v", err)
			}
		}

		// Print specified text
		if err := printFormattedText(thermalPrinter, textToPrint, PrintOptions{
			Bold:         *bold,
			Underline:    *underline,
			DoubleWidth:  *doubleWidth,
			DoubleHeight: *doubleHeight,
			Align:        *align,
			Lines:        *lines,
			Cut:          *cut,
		}); err != nil {
			log.Fatalf("Failed to print text: %v", err)
		}
		log.Info("Text printed successfully!")
	}
}

func parseHex(s string) (uint64, error) {
	// Remove 0x prefix if present
	s = strings.TrimPrefix(strings.ToLower(s), "0x")
	return strconv.ParseUint(s, 16, 16)
}

// PrintOptions holds formatting options for text printing
type PrintOptions struct {
	Bold         bool
	Underline    bool
	DoubleWidth  bool
	DoubleHeight bool
	Align        string
	Lines        int
	Cut          bool
}

// readFromStdin reads all content from stdin
func readFromStdin() (string, error) {
	var content strings.Builder
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		content.WriteString(scanner.Text())
		content.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return strings.TrimSuffix(content.String(), "\n"), nil
}

// printSeparator prints a separator line
func printSeparator(p *printer.ThermalPrinter, char string) error {
	if char == "" {
		char = "-"
	}
	// Use first character only
	if len(char) > 1 {
		char = string(char[0])
	}
	return p.PrintLine("", char, 32)
}

// printFormattedText prints text with specified formatting options
func printFormattedText(p *printer.ThermalPrinter, text string, opts PrintOptions) error {
	// Set alignment
	var align escpos.TextAlign
	switch strings.ToLower(opts.Align) {
	case "center":
		align = escpos.AlignCenter
	case "right":
		align = escpos.AlignRight
	default:
		align = escpos.AlignLeft
	}

	if err := p.SetAlign(align); err != nil {
		return err
	}

	// Apply formatting
	if err := p.SetBold(opts.Bold); err != nil {
		return err
	}

	if err := p.SetUnderline(opts.Underline); err != nil {
		return err
	}

	if err := p.SetDoubleWidth(opts.DoubleWidth); err != nil {
		return err
	}

	if err := p.SetDoubleHeight(opts.DoubleHeight); err != nil {
		return err
	}

	// Print text line by line
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if err := p.PrintText(line); err != nil {
			return err
		}
	}

	// Feed lines
	if opts.Lines > 0 {
		if err := p.Feed(opts.Lines); err != nil {
			return err
		}
	}

	// Cut paper if requested
	if opts.Cut {
		if err := p.Cut(); err != nil {
			return err
		}
	}

	return nil
}

func printText(p *printer.ThermalPrinter, text string, bold bool, alignStr string) error {
	// Set alignment
	var align escpos.TextAlign
	switch strings.ToLower(alignStr) {
	case "center":
		align = escpos.AlignCenter
	case "right":
		align = escpos.AlignRight
	default:
		align = escpos.AlignLeft
	}

	if err := p.SetAlign(align); err != nil {
		return err
	}

	// Set bold if requested
	if err := p.SetBold(bold); err != nil {
		return err
	}

	// Print text
	if err := p.PrintText(text); err != nil {
		return err
	}

	// Feed some lines
	return p.Feed(2)
}

func printTestReceipt(p *printer.ThermalPrinter) error {
	title := "MPT-II GO DRIVER TEST"
	items := []escpos.ReceiptItem{
		{Name: "Item 1", Price: 10.00},
		{Name: "Item 2", Price: 15.50},
		{Name: "Item 3", Price: 7.25},
	}

	var total float64
	for _, item := range items {
		total += item.Price
	}

	footer := "Thank you for testing!"

	return p.PrintReceipt(title, items, total, footer)
}

// uint16FromProps attempts to convert an interface{} to uint16 for vendor_id/product_id
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
		// try hex first
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

func showHelp() {
	fmt.Println("MPT-II Thermal Printer CLI (Go Version)")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  mptprinter-cli [options]")
	fmt.Println("")
	fmt.Println("Connection Options:")
	fmt.Println("  -type string")
	fmt.Println("        Connection type: usb or bluetooth (default \"usb\")")
	fmt.Println("  -vid string")
	fmt.Println("        USB Vendor ID in hex (default \"0483\")")
	fmt.Println("  -pid string")
	fmt.Println("        USB Product ID in hex (default \"5840\")")
	fmt.Println("  -address string")
	fmt.Println("        Bluetooth MAC address (XX:XX:XX:XX:XX:XX)")
	fmt.Println("")
	fmt.Println("Print Options:")
	fmt.Println("  -text string")
	fmt.Println("        Text to print")
	fmt.Println("  -stdin")
	fmt.Println("        Read text from stdin instead of -text")
	fmt.Println("  -test")
	fmt.Println("        Print a test receipt")
	fmt.Println("")
	fmt.Println("Formatting Options:")
	fmt.Println("  -bold")
	fmt.Println("        Use bold text")
	fmt.Println("  -underline")
	fmt.Println("        Use underlined text")
	fmt.Println("  -double-width")
	fmt.Println("        Use double width text")
	fmt.Println("  -double-height")
	fmt.Println("        Use double height text")
	fmt.Println("  -align string")
	fmt.Println("        Text alignment: left, center, right (default \"left\")")
	fmt.Println("  -separator string")
	fmt.Println("        Print a separator line with this character")
	fmt.Println("")
	fmt.Println("Output Options:")
	fmt.Println("  -lines int")
	fmt.Println("        Number of lines to feed after printing (default 2)")
	fmt.Println("  -cut")
	fmt.Println("        Cut paper after printing")
	fmt.Println("  -quiet")
	fmt.Println("        Suppress log output")
	fmt.Println("  -verbose")
	fmt.Println("        Enable verbose BLE logging (scan/services/characteristics)")
	fmt.Println("  -clear-cache")
	fmt.Println("        Clear cached device selection and exit")
	fmt.Println("  -codepage int")
	fmt.Println("        ESC/POS code page (0=PC437,2=PC850,5=PC865,16=WPC1252,19=PC858)")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  # Print simple text via USB")
	fmt.Println("  mptprinter-cli -text \"Hello, World!\"")
	fmt.Println("")
	fmt.Println("  # Print from stdin with formatting")
	fmt.Println("  echo \"Hello from stdin\" | mptprinter-cli -stdin -bold -center")
	fmt.Println("")
	fmt.Println("  # Print test receipt via Bluetooth")
	fmt.Println("  mptprinter-cli -type bluetooth -address XX:XX:XX:XX:XX:XX -test")
	fmt.Println("")
	fmt.Println("  # Print bold, centered title with separator and cut")
	fmt.Println("  mptprinter-cli -text \"RECEIPT\" -bold -align center -separator \"=\" -cut")
	fmt.Println("")
	fmt.Println("  # Print multiline from file")
	fmt.Println("  cat receipt.txt | mptprinter-cli -stdin -lines 3")
	fmt.Println("")
	fmt.Println("  # Quiet mode for scripts")
	fmt.Println("  mptprinter-cli -text \"Silent print\" -quiet")
}
