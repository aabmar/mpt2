// Command-line interface for the Go thermal printer driver
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aabmar/mpt2/go/pkg/configstore"
	"github.com/aabmar/mpt2/go/pkg/discovery"
	"github.com/aabmar/mpt2/go/pkg/printing"
	log "github.com/sirupsen/logrus"
)

func main() {
	// Define command-line flags
	var (
		btAddress      = flag.String("address", "", "Bluetooth MAC address (XX:XX:XX:XX:XX:XX). If omitted, auto-scan for a compatible printer.")
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

	// Create connection manager
	manager := discovery.NewConnectionManager()

	// Build connection options
	opts := discovery.DefaultConnectOptions()
	opts.Verbose = *verbose
	opts.CodePage = *codepage

	// Set Bluetooth address if provided
	if *btAddress != "" {
		opts.BluetoothAddress = *btAddress
	}

	// Connect to printer
	thermalPrinter, err := manager.ConnectAuto(context.Background(), opts)
	if err != nil {
		log.Fatalf("Failed to connect to printer: %v", err)
	}
	defer thermalPrinter.Disconnect()

	if *testReceipt {
		// Print test receipt
		if err := printing.PrintTestReceipt(thermalPrinter); err != nil {
			log.Fatalf("Failed to print test receipt: %v", err)
		}
		log.Info("Test receipt printed successfully!")
	} else {
		// Print separator line if requested
		if *separator != "" {
			if err := printing.PrintSeparator(thermalPrinter, *separator, 32); err != nil {
				log.Fatalf("Failed to print separator: %v", err)
			}
		}

		// Print specified text
		printOpts := printing.TextOptions{
			Bold:         *bold,
			Underline:    *underline,
			DoubleWidth:  *doubleWidth,
			DoubleHeight: *doubleHeight,
			Align:        *align,
			Lines:        *lines,
			Cut:          *cut,
		}

		if err := printing.PrintFormattedText(thermalPrinter, textToPrint, printOpts); err != nil {
			log.Fatalf("Failed to print text: %v", err)
		}
		log.Info("Text printed successfully!")
	}
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

func showHelp() {
	fmt.Println("MPT-II Thermal Printer CLI (Go Version)")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  mptprinter-cli [options]")
	fmt.Println("")
	fmt.Println("Connection Options:")
	fmt.Println("  -address string")
	fmt.Println("        Bluetooth MAC address (XX:XX:XX:XX:XX:XX). If omitted, auto-scan.")
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
	fmt.Println("  # Print simple text (auto-scan for printer)")
	fmt.Println("  mptprinter-cli -text \"Hello, World!\"")
	fmt.Println("")
	fmt.Println("  # Print from stdin with formatting")
	fmt.Println("  echo \"Hello from stdin\" | mptprinter-cli -stdin -bold -align center")
	fmt.Println("")
	fmt.Println("  # Print test receipt via Bluetooth")
	fmt.Println("  mptprinter-cli -address XX:XX:XX:XX:XX:XX -test")
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
