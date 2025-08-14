// Command-line interface for the Go thermal printer driver
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aabmar/mpt2/go/pkg/connections"
	"github.com/aabmar/mpt2/go/pkg/escpos"
	"github.com/aabmar/mpt2/go/pkg/printer"
)

func main() {
	// Define command-line flags
	var (
		connectionType = flag.String("type", "usb", "Connection type: usb or bluetooth")
		usbVID         = flag.String("vid", "0483", "USB Vendor ID (hex)")
		usbPID         = flag.String("pid", "5840", "USB Product ID (hex)")
		btAddress      = flag.String("address", "", "Bluetooth MAC address (XX:XX:XX:XX:XX:XX)")
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
		codepage       = flag.Int("codepage", -1, "ESC/POS code page number (e.g., 0=PC437,2=PC850,5=PC865,16=WPC1252,19=PC858)")
		help           = flag.Bool("help", false, "Show help")
	)

	flag.Parse()

	if *help {
		showHelp()
		return
	}

	// Set up logging
	if *quiet {
		log.SetOutput(io.Discard)
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

	// Create connection based on type
	factory := connections.NewConnectionFactory()
	var conn connections.Connection
	var err error

	switch strings.ToLower(*connectionType) {
	case "usb":
		vid, err := parseHex(*usbVID)
		if err != nil {
			log.Fatalf("Invalid USB VID '%s': %v", *usbVID, err)
		}
		pid, err := parseHex(*usbPID)
		if err != nil {
			log.Fatalf("Invalid USB PID '%s': %v", *usbPID, err)
		}

		params := connections.USBConnectionParams{
			VendorID:  uint16(vid),
			ProductID: uint16(pid),
		}
		conn, err = factory.CreateUSBConnection(params)

	case "bluetooth", "ble":
		if *btAddress == "" {
			log.Fatal("Bluetooth address must be specified with -address")
		}

		params := connections.BluetoothConnectionParams{
			Address: *btAddress,
		}
		conn, err = factory.CreateBluetoothConnection(params)

	default:
		log.Fatalf("Unknown connection type: %s", *connectionType)
	}

	if err != nil {
		log.Fatalf("Failed to create connection: %v", err)
	}

	// Create printer instance
	thermalPrinter := printer.NewThermalPrinter(conn)
	if *codepage >= 0 && *codepage <= 255 {
		thermalPrinter.SetCodePage(byte(*codepage))
	}

	// Connect to printer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Printf("Connecting to printer via %s...", *connectionType)
	if err := thermalPrinter.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to printer: %v", err)
	}
	defer thermalPrinter.Disconnect()

	log.Println("Connected successfully!")

	// Initialize printer
	if err := thermalPrinter.Initialize(); err != nil {
		log.Fatalf("Failed to initialize printer: %v", err)
	}

	if *testReceipt {
		// Print test receipt
		if err := printTestReceipt(thermalPrinter); err != nil {
			log.Fatalf("Failed to print test receipt: %v", err)
		}
		log.Println("Test receipt printed successfully!")
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
		log.Println("Text printed successfully!")
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
