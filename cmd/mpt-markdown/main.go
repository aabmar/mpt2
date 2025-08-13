// mpt-markdown: Print a Markdown file to the thermal printer with simple formatting
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aabmar/mpt2/go/pkg/connections"
	"github.com/aabmar/mpt2/go/pkg/escpos"
	"github.com/aabmar/mpt2/go/pkg/printer"
)

// Options for printing
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

	// Connect via default USB
	factory := connections.NewConnectionFactory()
	conn, err := factory.CreateUSBConnection(connections.USBConnectionParams{VendorID: 0x0483, ProductID: 0x5840})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create USB connection: %v\n", err)
		os.Exit(1)
	}

	p := printer.NewThermalPrinter(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := p.Connect(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to connect to printer: %v\n", err)
		os.Exit(1)
	}
	defer p.Disconnect()

	if err := p.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize printer: %v\n", err)
		os.Exit(1)
	}

	opts := Options{LineWidth: *width, FeedLines: *feed, Cut: *cut}
	if err := PrintMarkdown(p, string(data), opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if opts.FeedLines > 0 {
		_ = p.Feed(opts.FeedLines)
	}
	if opts.Cut {
		_ = p.Cut()
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
}

// PrintMarkdown parses and prints a subset of original Markdown.
// Mapping to ESC/POS:
// - Headings: H1 (center, bold, double height+width), H2 (bold, double width), H3 (bold), H4-6 (bold)
// - Emphasis: *italic*/_italic_ => underline; **bold**/__bold__ => bold; combined => bold+underline
// - Horizontal rule: separator line
// - Lists: basic unordered (-, +, *) and ordered (1.) with indentation
// - Blockquotes: prefixed with "> ", printed indented with a leading "| "
// - Code blocks: 4-space or tab indented lines; printed monospaced as-is (no inline formatting)
// - Links: [text](url) => "text (url)"; Images: ![alt](url) => "[Image: alt] (url)"
func PrintMarkdown(p *printer.ThermalPrinter, md string, opts Options) error {
	// Normalize line endings
	md = strings.ReplaceAll(md, "\r\n", "\n")
	md = strings.ReplaceAll(md, "\r", "\n")

	lines := strings.Split(md, "\n")
	// Pre-detect setext headers: when a line of text is followed by === or ---
	i := 0
	for i < len(lines) {
		line := lines[i]

		// Horizontal rule
		if isHorizontalRule(line) {
			if err := p.PrintLine("", "-", opts.LineWidth); err != nil {
				return err
			}
			i++
			continue
		}

		// Code block (indented >=4 spaces or tab)
		if isCodeLine(line) {
			if err := printCodeBlock(p, lines, &i, opts.LineWidth); err != nil {
				return err
			}
			continue
		}

		// Blank line => paragraph break
		if strings.TrimSpace(line) == "" {
			// Add spacing between blocks
			if err := p.Feed(1); err != nil {
				return err
			}
			i++
			continue
		}

		// Setext headers
		if i+1 < len(lines) {
			underline := lines[i+1]
			if isSetextHeaderUnderline(underline) {
				level := 2
				if strings.TrimSpace(underline)[0] == '=' {
					level = 1
				}
				if err := printHeading(p, strings.TrimSpace(line), level); err != nil {
					return err
				}
				i += 2
				continue
			}
		}

		// ATX heading (#...)
		if h, lvl := parseATXHeading(line); h {
			text := strings.TrimSpace(line[lvl:])
			if err := printHeading(p, text, lvl); err != nil {
				return err
			}
			i++
			continue
		}

		// Blockquote
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), ">") {
			if err := printBlockquote(p, lines, &i, opts.LineWidth); err != nil {
				return err
			}
			continue
		}

		// List (unordered or ordered)
		if isListItem(line) || isOrderedListItem(line) {
			if err := printList(p, lines, &i, opts.LineWidth); err != nil {
				return err
			}
			continue
		}

		// Paragraph
		if err := printParagraph(p, lines, &i, opts.LineWidth); err != nil {
			return err
		}
	}
	return nil
}

func isHorizontalRule(line string) bool {
	s := strings.TrimSpace(line)
	if len(s) < 3 {
		return false
	}
	// Three or more of -, *, _ possibly with spaces
	s = strings.ReplaceAll(s, " ", "")
	return allSame(s, '-') || allSame(s, '*') || allSame(s, '_')
}

func allSame(s string, ch byte) bool {
	if len(s) < 3 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != ch {
			return false
		}
	}
	return true
}

func isCodeLine(line string) bool {
	return strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
}

func isSetextHeaderUnderline(line string) bool {
	t := strings.TrimSpace(line)
	if len(t) == 0 {
		return false
	}
	// All '=' or all '-'
	for i := 0; i < len(t); i++ {
		if t[i] != '=' && t[i] != '-' {
			return false
		}
	}
	return len(t) >= 3
}

func parseATXHeading(line string) (bool, int) {
	t := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(t, "#") {
		return false, 0
	}
	lvl := 0
	for lvl < len(t) && t[lvl] == '#' {
		lvl++
	}
	if lvl == 0 || lvl > 6 {
		return false, 0
	}
	// requires a space after hashes per original spec common usage
	if len(t) > lvl && (t[lvl] == ' ' || t[lvl] == '\t') {
		return true, lvl
	}
	return false, 0
}

func printHeading(p *printer.ThermalPrinter, text string, level int) error {
	// Normalize level
	if level < 1 {
		level = 1
	}
	if level > 6 {
		level = 6
	}
	// Center headings
	if err := p.SetAlign(escpos.AlignCenter); err != nil {
		return err
	}
	// Apply styles by level
	switch level {
	case 1:
		if err := p.SetBold(true); err != nil {
			return err
		}
		if err := p.SetDoubleWidth(true); err != nil {
			return err
		}
		if err := p.SetDoubleHeight(true); err != nil {
			return err
		}
	case 2:
		if err := p.SetBold(true); err != nil {
			return err
		}
		if err := p.SetDoubleWidth(true); err != nil {
			return err
		}
	default:
		if err := p.SetBold(true); err != nil {
			return err
		}
	}
	// Print and reset
	if err := p.PrintText(text); err != nil {
		return err
	}
	_ = p.SetBold(false)
	_ = p.SetDoubleWidth(false)
	_ = p.SetDoubleHeight(false)
	return p.SetAlign(escpos.AlignLeft)
}

func printCodeBlock(p *printer.ThermalPrinter, lines []string, idx *int, width int) error {
	// No inline formatting; print lines as-is, trimmed of one leading indent level
	for *idx < len(lines) {
		line := lines[*idx]
		if !isCodeLine(line) {
			break
		}
		// strip one indent level
		if strings.HasPrefix(line, "    ") {
			line = line[4:]
		} else if strings.HasPrefix(line, "\t") {
			line = line[1:]
		}
		// wrap long code lines
		for _, wl := range wrap(line, width) {
			if err := p.PrintText(wl); err != nil {
				return err
			}
		}
		*idx++
	}
	return nil
}

func printBlockquote(p *printer.ThermalPrinter, lines []string, idx *int, width int) error {
	var block []string
	for *idx < len(lines) {
		line := lines[*idx]
		t := strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(t, ">") {
			break
		}
		t = strings.TrimSpace(strings.TrimPrefix(t, ">"))
		block = append(block, t)
		*idx++
	}
	text := strings.Join(block, " ")
	wrapped := wrap(text, width-2)
	// Print with a "| " prefix and underline to mimic italicized quote
	_ = p.SetUnderline(true)
	for _, l := range wrapped {
		if err := printInline(p, "| "+l); err != nil {
			return err
		}
	}
	_ = p.SetUnderline(false)
	return nil
}

func printList(p *printer.ThermalPrinter, lines []string, idx *int, width int) error {
	// Consume consecutive list items (simple, no nesting)
	for *idx < len(lines) {
		line := lines[*idx]
		if strings.TrimSpace(line) == "" {
			break
		}
		if !(isListItem(line) || isOrderedListItem(line)) {
			break
		}
		bullet, text := listParts(line)
		indent := 2
		wrapped := wrap(text, width-indent-2)
		if len(wrapped) == 0 {
			wrapped = []string{""}
		}
		// First line shows bullet; subsequent are indented
		if err := printInline(p, bullet+" "+wrapped[0]); err != nil {
			return err
		}
		for i := 1; i < len(wrapped); i++ {
			if err := printInline(p, strings.Repeat(" ", indent)+wrapped[i]); err != nil {
				return err
			}
		}
		*idx++
	}
	return nil
}

var (
	reUnordered = regexp.MustCompile(`^\s*([\-*+])\s+(.+)$`)
	reOrdered   = regexp.MustCompile(`^\s*(\d+)\.\s+(.+)$`)
)

func isListItem(line string) bool        { return reUnordered.MatchString(line) }
func isOrderedListItem(line string) bool { return reOrdered.MatchString(line) }

func listParts(line string) (bullet, text string) {
	if m := reUnordered.FindStringSubmatch(line); m != nil {
		return "-", m[2]
	}
	if m := reOrdered.FindStringSubmatch(line); m != nil {
		return m[1] + ".", m[2]
	}
	return "-", strings.TrimSpace(line)
}

func printParagraph(p *printer.ThermalPrinter, lines []string, idx *int, width int) error {
	// Gather until a blank line or a new block marker
	var parts []string
	for *idx < len(lines) {
		line := lines[*idx]
		if strings.TrimSpace(line) == "" || isHorizontalRule(line) || isCodeLine(line) || isListItem(line) || isOrderedListItem(line) || strings.HasPrefix(strings.TrimLeft(line, " \t"), ">") || isPotentialHeading(line) {
			break
		}
		parts = append(parts, strings.TrimRight(line, " "))
		*idx++
	}
	text := strings.Join(parts, " ")
	for _, l := range wrap(text, width) {
		if err := printInline(p, l); err != nil {
			return err
		}
	}
	return nil
}

func isPotentialHeading(line string) bool {
	if ok, _ := parseATXHeading(line); ok {
		return true
	}
	return false
}

// inlineState not persisted across lines; we reset formatting each line
func printInline(p *printer.ThermalPrinter, text string) error {
	// Reset inline styles at line start
	_ = p.SetBold(false)
	_ = p.SetUnderline(false)

	i := 0
	bold := false
	underline := false
	// helper to apply current styles
	apply := func() error {
		if err := p.SetBold(bold); err != nil {
			return err
		}
		if err := p.SetUnderline(underline); err != nil {
			return err
		}
		return nil
	}

	// buffer to accumulate plain text
	var buf strings.Builder

	flush := func() error {
		if buf.Len() == 0 {
			return nil
		}
		if err := p.Send([]byte(buf.String())); err != nil {
			return err
		}
		buf.Reset()
		return nil
	}

	for i < len(text) {
		// Links and images
		if text[i] == '!' && i+1 < len(text) && text[i+1] == '[' {
			// image
			if alt, url, n, ok := parseLinkLike(text[i+1:]); ok {
				// print as [Image: alt] (url)
				if err := flush(); err != nil {
					return err
				}
				if err := apply(); err != nil {
					return err
				}
				if err := p.Send([]byte("[Image: " + alt + "] (" + url + ")")); err != nil {
					return err
				}
				i += 1 + n
				continue
			}
		}
		if text[i] == '[' {
			if label, url, n, ok := parseLinkLike(text[i:]); ok {
				if err := flush(); err != nil {
					return err
				}
				if err := apply(); err != nil {
					return err
				}
				if err := p.Send([]byte(label)); err != nil {
					return err
				}
				if url != "" {
					if err := p.Send([]byte(" (" + url + ")")); err != nil {
						return err
					}
				}
				i += n
				continue
			}
		}

		// Inline code `code`
		if text[i] == '`' {
			// find closing backtick
			if j := strings.IndexByte(text[i+1:], '`'); j >= 0 {
				if err := flush(); err != nil {
					return err
				}
				// Temporarily disable styles for code span
				prevBold, prevUnderline := bold, underline
				bold, underline = false, false
				if err := apply(); err != nil {
					return err
				}
				segment := text[i+1 : i+1+j]
				if err := p.Send([]byte(segment)); err != nil {
					return err
				}
				// restore previous styles
				bold, underline = prevBold, prevUnderline
				if err := apply(); err != nil {
					return err
				}
				i = i + 1 + j + 1
				continue
			}
			// no closing backtick: fallthrough and print backtick as normal
		}

		// Bold/italic markers
		if strings.HasPrefix(text[i:], "***") || strings.HasPrefix(text[i:], "___") {
			if err := flush(); err != nil {
				return err
			}
			// toggle both
			bold = !bold
			underline = !underline
			if err := apply(); err != nil {
				return err
			}
			i += 3
			continue
		}
		if strings.HasPrefix(text[i:], "**") || strings.HasPrefix(text[i:], "__") {
			if err := flush(); err != nil {
				return err
			}
			bold = !bold
			if err := apply(); err != nil {
				return err
			}
			i += 2
			continue
		}
		if text[i] == '*' || text[i] == '_' {
			if err := flush(); err != nil {
				return err
			}
			underline = !underline
			if err := apply(); err != nil {
				return err
			}
			i++
			continue
		}

		// Normal char
		buf.WriteByte(text[i])
		i++
	}

	if err := flush(); err != nil {
		return err
	}
	// end of line: reset styles and add LF
	if err := p.SetBold(false); err != nil {
		return err
	}
	if err := p.SetUnderline(false); err != nil {
		return err
	}
	return p.Send(append([]byte{}, escpos.LINEFEED...))
}

// parseLinkLike parses forms: [label](url) and returns label, url, consumed, ok
func parseLinkLike(s string) (label, url string, consumed int, ok bool) {
	// s begins with '[' in our calls (or "[" when from image we skip '!')
	if len(s) == 0 || s[0] != '[' {
		return "", "", 0, false
	}
	// find matching ']'
	end := strings.IndexByte(s, ']')
	if end <= 0 {
		return "", "", 0, false
	}
	label = s[1:end]
	// next must be '('
	rest := s[end+1:]
	if len(rest) == 0 || rest[0] != '(' {
		return label, "", end + 1, true
	}
	// find ')'
	close := strings.IndexByte(rest, ')')
	if close < 0 {
		return label, "", end + 1, true
	}
	url = rest[1:close]
	consumed = end + 1 + close + 1
	return label, url, consumed, true
}

// wrap splits text into lines not exceeding width, breaking on spaces.
func wrap(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	words := splitWords(text)
	var lines []string
	var cur strings.Builder
	for _, w := range words {
		if cur.Len() == 0 {
			if len(w) <= width {
				cur.WriteString(w)
			} else {
				// hard-break long word
				for len(w) > width {
					lines = append(lines, w[:width])
					w = w[width:]
				}
				if len(w) > 0 {
					cur.WriteString(w)
				}
			}
			continue
		}
		if cur.Len()+1+len(w) <= width {
			cur.WriteByte(' ')
			cur.WriteString(w)
		} else {
			lines = append(lines, cur.String())
			cur.Reset()
			if len(w) <= width {
				cur.WriteString(w)
			} else {
				for len(w) > width {
					lines = append(lines, w[:width])
					w = w[width:]
				}
				if len(w) > 0 {
					cur.WriteString(w)
				}
			}
		}
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	if len(lines) == 0 {
		return []string{}
	}
	return lines
}

func splitWords(s string) []string {
	// collapse whitespace to single spaces for wrapping
	fields := strings.Fields(s)
	return fields
}
