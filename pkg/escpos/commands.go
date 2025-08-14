// Package escpos provides ESC/POS command constants and utilities for thermal printers
package escpos

// ESC/POS command constants - direct port from Python version
var (
	ESC      = []byte{0x1B}
	GS       = []byte{0x1D}
	INIT     = append(ESC, 0x40)      // ESC @ - Initialize printer
	CUT      = append(GS, 0x56, 0x01) // GS V 1 - Cut paper
	LINEFEED = []byte{0x0A}           // LF - Line feed

	// Text formatting commands
	BOLD_ON       = append(ESC, 0x45, 0x01) // ESC E 1 - Bold on
	BOLD_OFF      = append(ESC, 0x45, 0x00) // ESC E 0 - Bold off
	DOUBLE_HEIGHT = append(ESC, 0x21, 0x10) // ESC ! 16 - Double height
	DOUBLE_WIDTH  = append(ESC, 0x21, 0x20) // ESC ! 32 - Double width
	UNDERLINE_ON  = append(ESC, 0x2D, 0x01) // ESC - 1 - Underline on
	UNDERLINE_OFF = append(ESC, 0x2D, 0x00) // ESC - 0 - Underline off

	// Text alignment
	ALIGN_LEFT   = append(ESC, 0x61, 0x00) // ESC a 0 - Left align
	ALIGN_CENTER = append(ESC, 0x61, 0x01) // ESC a 1 - Center align
	ALIGN_RIGHT  = append(ESC, 0x61, 0x02) // ESC a 2 - Right align
)

// Code page selection: ESC t n
// Returns the ESC/POS command to select character code table 'n'
func CodePageCommand(n byte) []byte {
	return append(append([]byte{}, ESC...), 0x74, n)
}

// Common code page numbers per ESC/POS (may vary by model):
// 0: PC437 (USA, Standard Europe)
// 2: PC850 (Multilingual)
// 3: PC860 (Portuguese)
// 4: PC863 (Canadian-French)
// 5: PC865 (Nordic)
// 16: WPC1252
// 17: PC866 (Cyrillic#2)
// 18: PC852 (Latin 2)
// 19: PC858 (Euro)

// TextAlign represents text alignment options
type TextAlign int

const (
	AlignLeft TextAlign = iota
	AlignCenter
	AlignRight
)

// GetAlignCommand returns the ESC/POS command for the specified alignment
func GetAlignCommand(align TextAlign) []byte {
	switch align {
	case AlignCenter:
		return ALIGN_CENTER
	case AlignRight:
		return ALIGN_RIGHT
	default:
		return ALIGN_LEFT
	}
}

// FormatMode represents text formatting options
type FormatMode struct {
	Bold         bool
	Underline    bool
	DoubleWidth  bool
	DoubleHeight bool
}

// GetFormatCommands returns the ESC/POS commands to apply the formatting
func (f FormatMode) GetFormatCommands() [][]byte {
	var commands [][]byte

	if f.Bold {
		commands = append(commands, BOLD_ON)
	} else {
		commands = append(commands, BOLD_OFF)
	}

	if f.Underline {
		commands = append(commands, UNDERLINE_ON)
	} else {
		commands = append(commands, UNDERLINE_OFF)
	}

	if f.DoubleWidth {
		commands = append(commands, DOUBLE_WIDTH)
	}

	if f.DoubleHeight {
		commands = append(commands, DOUBLE_HEIGHT)
	}

	return commands
}

// ReceiptItem represents an item in a receipt
type ReceiptItem struct {
	Name  string
	Price float64
}

// FormatReceiptLine formats a receipt line with proper spacing
func FormatReceiptLine(item, price string, lineWidth int) []byte {
	if lineWidth <= 0 {
		lineWidth = 32 // Default width for 58mm paper
	}

	totalLength := len(item) + len(price)
	if totalLength >= lineWidth {
		// Truncate item name if too long
		maxItemLength := lineWidth - len(price) - 1
		if maxItemLength > 0 {
			item = item[:maxItemLength]
		}
		totalLength = len(item) + len(price)
	}

	spaces := lineWidth - totalLength
	if spaces < 0 {
		spaces = 0
	}

	line := item
	for i := 0; i < spaces; i++ {
		line += " "
	}
	line += price

	return []byte(line)
}
