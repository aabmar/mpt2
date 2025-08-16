// Package printing provides high-level printing operations and formatting
package printing

import (
	"strings"

	"github.com/aabmar/mpt2/go/pkg/escpos"
	"github.com/aabmar/mpt2/go/pkg/printer"
)

// TextOptions holds formatting options for text printing
type TextOptions struct {
	Bold         bool
	Underline    bool
	DoubleWidth  bool
	DoubleHeight bool
	Align        string // "left", "center", "right"
	Lines        int    // Number of lines to feed after printing
	Cut          bool   // Cut paper after printing
}

// DefaultTextOptions returns sensible defaults for text printing
func DefaultTextOptions() TextOptions {
	return TextOptions{
		Bold:         false,
		Underline:    false,
		DoubleWidth:  false,
		DoubleHeight: false,
		Align:        "left",
		Lines:        2,
		Cut:          false,
	}
}

// PrintFormattedText prints text with specified formatting options
func PrintFormattedText(p *printer.ThermalPrinter, text string, opts TextOptions) error {
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

// PrintSeparator prints a separator line with specified character
func PrintSeparator(p *printer.ThermalPrinter, char string, width int) error {
	if char == "" {
		char = "-"
	}
	// Use first character only
	if len(char) > 1 {
		char = string(char[0])
	}
	if width <= 0 {
		width = 32 // Default for 58mm paper
	}

	return p.PrintLine("", char, width)
}

// PrintTestReceipt prints a standard test receipt for printer verification
func PrintTestReceipt(p *printer.ThermalPrinter) error {
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

// PrintSimpleText is a convenience function for basic text printing
func PrintSimpleText(p *printer.ThermalPrinter, text string) error {
	opts := DefaultTextOptions()
	return PrintFormattedText(p, text, opts)
}

// PrintCenteredTitle prints a centered, bold title
func PrintCenteredTitle(p *printer.ThermalPrinter, title string) error {
	opts := TextOptions{
		Bold:        true,
		Align:       "center",
		Lines:       1,
		DoubleWidth: true,
	}
	return PrintFormattedText(p, title, opts)
}

// PrintBoldText prints bold text with default options
func PrintBoldText(p *printer.ThermalPrinter, text string) error {
	opts := DefaultTextOptions()
	opts.Bold = true
	return PrintFormattedText(p, text, opts)
}

// PrintWithCut prints text and cuts the paper
func PrintWithCut(p *printer.ThermalPrinter, text string) error {
	opts := DefaultTextOptions()
	opts.Cut = true
	return PrintFormattedText(p, text, opts)
}
