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
	codePage   byte // ESC/POS code page number
}

// NewThermalPrinter creates a new thermal printer instance
func NewThermalPrinter(conn connections.Connection) *ThermalPrinter {
	return &ThermalPrinter{
		connection: conn,
		connected:  false,
		codePage:   0, // default to printer default (often CP437)
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
	if err := p.Send(escpos.INIT); err != nil {
		return err
	}
	if p.codePage != 0xFF { // 0xFF means "leave as printer default"
		if err := p.Send(escpos.CodePageCommand(p.codePage)); err != nil {
			return err
		}
	}
	return nil
}

// PrintText prints text with a line feed
func (p *ThermalPrinter) PrintText(text string) error {
	// Convert UTF-8 text to bytes using current code page best-effort mapping
	data := p.encodeText(text)
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

// SetCodePage selects the ESC/POS character code table (see printer manual)
// Common values: 0=PC437, 2=PC850, 5=PC865 (Nordic), 16=WPC1252, 19=PC858
// Pass 0xFF to keep printer default.
func (p *ThermalPrinter) SetCodePage(n byte) {
	p.codePage = n
}

// encodeText maps a subset of Unicode to the selected code page.
// For Western European languages, useful pages: 2 (850), 16 (1252), 19 (858), 5 (865 for Nordic).
// This is a minimal mapping for common characters; unsupported runes fall back to '?'.
func (p *ThermalPrinter) encodeText(s string) []byte {
	// If no explicit code page set, send as-is and hope the printer expects UTF-8 or similar.
	// Many ESC/POS printers expect single-byte encodings, so mapping helps for Nordic letters.
	cp := p.codePage
	if cp == 0 { // 0 often means CP437; we still map to single-byte approximations
		cp = 0
	}
	out := make([]byte, 0, len(s))
	for _, r := range s {
		b, ok := mapRuneToCP(r, cp)
		if !ok {
			b = '?'
		}
		out = append(out, b)
	}
	return out
}

// mapRuneToCP provides minimal mappings for CP437/850/858/865/1252 for characters likely in Norwegian.
func mapRuneToCP(r rune, cp byte) (byte, bool) {
	// Common ASCII fast-path
	if r >= 0x20 && r <= 0x7E {
		return byte(r), true
	}
	// Norwegian letters: Æ Ø Å æ ø å and common diacritics
	switch r {
	case 'Æ':
		switch cp {
		case 5 /*865*/ :
			return 0x92, true
		case 2 /*850*/ :
			return 0x92, true
		case 16 /*1252*/ :
			return 0x8E, true
		case 19 /*858*/ :
			return 0x92, true
		default:
			return 0x92, true
		}
	case 'æ':
		switch cp {
		case 5:
			return 0x91, true
		case 2:
			return 0x91, true
		case 16:
			return 0x84, true
		case 19:
			return 0x91, true
		default:
			return 0x91, true
		}
	case 'Ø':
		switch cp {
		case 5:
			return 0x9D, true
		case 2:
			return 0x9D, true
		case 16:
			return 0x8D, true
		case 19:
			return 0x9D, true
		default:
			return 0x9D, true
		}
	case 'ø':
		switch cp {
		case 5:
			return 0x9B, true
		case 2:
			return 0x9B, true
		case 16:
			return 0x9D, true
		case 19:
			return 0x9B, true
		default:
			return 0x9B, true
		}
	case 'Å':
		switch cp {
		case 5:
			return 0x8F, true
		case 2:
			return 0x8F, true
		case 16:
			return 0x8F, true
		case 19:
			return 0x8F, true
		default:
			return 0x8F, true
		}
	case 'å':
		switch cp {
		case 5:
			return 0x86, true
		case 2:
			return 0x86, true
		case 16:
			return 0x86, true
		case 19:
			return 0x86, true
		default:
			return 0x86, true
		}
	// Common European diacritics (minimal subset)
	case 'Ä':
		return 0x8E, true
	case 'Ö':
		return 0x99, true
	case 'Ü':
		return 0x9A, true
	case 'ä':
		return 0x84, true
	case 'ö':
		return 0x94, true
	case 'ü':
		return 0x81, true
	case 'É':
		return 0x90, true
	case 'é':
		return 0x82, true
	case 'È':
		return 0xD2, true
	case 'è':
		return 0x8A, true
	case 'Ç':
		return 0x80, true
	case 'ç':
		return 0x87, true
	}
	// Newline, tab
	if r == '\n' {
		return 0x0A, true
	}
	if r == '\r' {
		return 0x0D, true
	}
	if r == '\t' {
		return 0x09, true
	}
	return 0, false
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
