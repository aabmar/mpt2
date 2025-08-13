// Package printer provides the main thermal printer interface and implementation
package printer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aabmar/mpt2/go/pkg/connections"
	"github.com/aabmar/mpt2/go/pkg/escpos"
)

// ThermalPrinter represents a thermal printer with ESC/POS command support
type ThermalPrinter struct {
	connection connections.Connection
	connected  bool
}

// NewThermalPrinter creates a new thermal printer instance
func NewThermalPrinter(conn connections.Connection) *ThermalPrinter {
	return &ThermalPrinter{
		connection: conn,
		connected:  false,
	}
}

// Connect establishes connection to the printer
func (p *ThermalPrinter) Connect(ctx context.Context) error {
	if p.connection == nil {
		return fmt.Errorf("no connection provided")
	}

	err := p.connection.Connect(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to printer: %w", err)
	}

	p.connected = true
	return nil
}

// Disconnect closes the connection to the printer
func (p *ThermalPrinter) Disconnect() error {
	if p.connection != nil {
		err := p.connection.Close()
		p.connected = false
		return err
	}
	return nil
}

// IsConnected returns true if the printer is connected
func (p *ThermalPrinter) IsConnected() bool {
	return p.connected && p.connection != nil && p.connection.IsConnected()
}

// Send sends raw data to the printer
func (p *ThermalPrinter) Send(data []byte) error {
	if !p.IsConnected() {
		return fmt.Errorf("printer not connected")
	}

	_, err := p.connection.Write(data)
	return err
}

// Initialize initializes the printer (resets to default state)
func (p *ThermalPrinter) Initialize() error {
	return p.Send(escpos.INIT)
}

// PrintText prints text with a line feed
func (p *ThermalPrinter) PrintText(text string) error {
	// Convert text to bytes (assuming UTF-8, could add encoding support later)
	data := []byte(text)
	data = append(data, escpos.LINEFEED...)
	return p.Send(data)
}

// PrintLine prints a line of characters
func (p *ThermalPrinter) PrintLine(text string, char string, length int) error {
	if text != "" {
		if err := p.PrintText(text); err != nil {
			return err
		}
	}

	if length <= 0 {
		length = 32 // Default for 58mm paper
	}

	line := strings.Repeat(char, length)
	return p.PrintText(line)
}

// Feed feeds the specified number of lines
func (p *ThermalPrinter) Feed(lines int) error {
	if lines <= 0 {
		lines = 1
	}

	data := make([]byte, 0, lines)
	for i := 0; i < lines; i++ {
		data = append(data, escpos.LINEFEED...)
	}

	return p.Send(data)
}

// Cut cuts the paper (if supported by the printer)
func (p *ThermalPrinter) Cut() error {
	return p.Send(escpos.CUT)
}

// SetBold sets bold text mode
func (p *ThermalPrinter) SetBold(enabled bool) error {
	if enabled {
		return p.Send(escpos.BOLD_ON)
	}
	return p.Send(escpos.BOLD_OFF)
}

// SetUnderline sets underline text mode
func (p *ThermalPrinter) SetUnderline(enabled bool) error {
	if enabled {
		return p.Send(escpos.UNDERLINE_ON)
	}
	return p.Send(escpos.UNDERLINE_OFF)
}

// SetAlign sets text alignment
func (p *ThermalPrinter) SetAlign(align escpos.TextAlign) error {
	return p.Send(escpos.GetAlignCommand(align))
}

// SetDoubleWidth sets double width text mode
func (p *ThermalPrinter) SetDoubleWidth(enabled bool) error {
	if enabled {
		return p.Send(escpos.DOUBLE_WIDTH)
	}
	return p.Send(escpos.INIT) // Reset to normal
}

// SetDoubleHeight sets double height text mode
func (p *ThermalPrinter) SetDoubleHeight(enabled bool) error {
	if enabled {
		return p.Send(escpos.DOUBLE_HEIGHT)
	}
	return p.Send(escpos.INIT) // Reset to normal
}

// ApplyFormat applies multiple formatting options at once
func (p *ThermalPrinter) ApplyFormat(format escpos.FormatMode) error {
	commands := format.GetFormatCommands()
	for _, cmd := range commands {
		if err := p.Send(cmd); err != nil {
			return err
		}
	}
	return nil
}

// PrintReceipt prints a formatted receipt
func (p *ThermalPrinter) PrintReceipt(title string, items []escpos.ReceiptItem, total float64, footer string) error {
	// Initialize printer
	if err := p.Initialize(); err != nil {
		return err
	}

	// Print title (centered, bold)
	if err := p.SetAlign(escpos.AlignCenter); err != nil {
		return err
	}
	if err := p.SetBold(true); err != nil {
		return err
	}
	if err := p.PrintText(title); err != nil {
		return err
	}
	if err := p.SetBold(false); err != nil {
		return err
	}

	// Print timestamp
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	if err := p.PrintText(timestamp); err != nil {
		return err
	}

	// Reset alignment
	if err := p.SetAlign(escpos.AlignLeft); err != nil {
		return err
	}

	// Print separator line
	if err := p.PrintLine("", "-", 32); err != nil {
		return err
	}

	// Print items
	for _, item := range items {
		priceStr := fmt.Sprintf("%.2f", item.Price)
		line := escpos.FormatReceiptLine(item.Name, priceStr, 32)
		if err := p.Send(append(line, escpos.LINEFEED...)); err != nil {
			return err
		}
	}

	// Print separator line
	if err := p.PrintLine("", "-", 32); err != nil {
		return err
	}

	// Print total (bold)
	if err := p.SetBold(true); err != nil {
		return err
	}
	totalStr := fmt.Sprintf("%.2f", total)
	totalLine := escpos.FormatReceiptLine("TOTAL:", totalStr, 32)
	if err := p.Send(append(totalLine, escpos.LINEFEED...)); err != nil {
		return err
	}
	if err := p.SetBold(false); err != nil {
		return err
	}

	// Print footer if provided
	if footer != "" {
		if err := p.PrintLine("", "-", 32); err != nil {
			return err
		}
		if err := p.SetAlign(escpos.AlignCenter); err != nil {
			return err
		}
		if err := p.PrintText(footer); err != nil {
			return err
		}
		if err := p.SetAlign(escpos.AlignLeft); err != nil {
			return err
		}
	}

	// Feed paper and cut
	if err := p.Feed(2); err != nil {
		return err
	}
	if err := p.Cut(); err != nil {
		return err
	}

	return nil
}

// GetConnectionInfo returns information about the current connection
func (p *ThermalPrinter) GetConnectionInfo() connections.ConnectionInfo {
	if p.connection == nil {
		return connections.ConnectionInfo{
			Type:        "none",
			Address:     "",
			Description: "No connection",
			Properties:  map[string]interface{}{},
		}
	}
	return p.connection.GetInfo()
}
