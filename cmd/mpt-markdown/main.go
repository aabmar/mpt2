// mpt-markdown: Print a Markdown file to the thermal printer with simple formatting
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aabmar/mpt2/go/pkg/discovery"
	"github.com/aabmar/mpt2/go/pkg/printing"
)

// Options for printing (keeping for backwards compatibility)
type Options struct {
	LineWidth int
	FeedLines int
	Cut       bool
}

func main() {
	// Flags kept minimal; default to USB like mptprint
	width := flag.Int("width", 32, "Line width in characters (e.g., 32 for 58mm)")
	feed := flag.Int("feed", 2, "Number of lines to feed after printing")
	cut := flag.Bool("cut", false, "Cut paper after printing")
	codepage := flag.Int("codepage", -1, "ESC/POS code page (0=PC437,2=PC850,5=PC865,16=WPC1252,19=PC858)")
	help := flag.Bool("help", false, "Show help")
	flag.Parse()

	if *help || flag.NArg() < 1 {
		usage()
		return
	}

	path := flag.Arg(0)
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file %s: %v\n", path, err)
		os.Exit(1)
	}

	// Connect via USB using the discovery manager
	manager := discovery.NewConnectionManager()
	
	p, err := manager.ConnectUSB(context.Background(), 0x0483, 0x5840, *codepage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to connect to printer: %v\n", err)
		os.Exit(1)
	}
	defer p.Disconnect()

	// Convert legacy options to library options
	markdownOpts := printing.MarkdownOptions{
		LineWidth: *width,
		FeedLines: *feed,
		Cut:       *cut,
	}
	
	if err := printing.PrintMarkdown(p, string(data), markdownOpts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	exe := filepath.Base(os.Args[0])
	fmt.Println("mpt-markdown - Print a Markdown file on a thermal printer")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Printf("  %s [options] <file.md>\n", exe)
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  -width N   Line width (characters), default 32")
	fmt.Println("  -feed N    Lines to feed after print, default 2")
	fmt.Println("  -cut       Cut paper after printing")
	fmt.Println("  -codepage  ESC/POS code page (0=PC437,2=PC850,5=PC865,16=WPC1252,19=PC858)")
}
