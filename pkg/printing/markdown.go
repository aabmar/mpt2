// Package printing provides Markdown document parsing and printing
package printing

import (
	"regexp"
	"strings"

	"github.com/aabmar/mpt2/go/pkg/escpos"
	"github.com/aabmar/mpt2/go/pkg/printer"
)

// MarkdownOptions configures Markdown printing
type MarkdownOptions struct {
	LineWidth int  // Character width for line wrapping
	FeedLines int  // Lines to feed after printing
	Cut       bool // Cut paper after printing
}

// DefaultMarkdownOptions returns sensible defaults
func DefaultMarkdownOptions() MarkdownOptions {
	return MarkdownOptions{
		LineWidth: 32, // Standard for 58mm paper
		FeedLines: 2,
		Cut:       false,
	}
}

// PrintMarkdown parses and prints a Markdown document to the thermal printer
func PrintMarkdown(p *printer.ThermalPrinter, markdown string, opts MarkdownOptions) error {
	// Normalize line endings
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	markdown = strings.ReplaceAll(markdown, "\r", "\n")

	lines := strings.Split(markdown, "\n")

	// Parse and print the document
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

		// Setext headers (underlined with === or ---)
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

		// ATX heading (# ## ### etc.)
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

	// Final formatting
	if opts.FeedLines > 0 {
		if err := p.Feed(opts.FeedLines); err != nil {
			return err
		}
	}
	if opts.Cut {
		if err := p.Cut(); err != nil {
			return err
		}
	}

	return nil
}

// Markdown parsing helper functions

func isHorizontalRule(line string) bool {
	s := strings.TrimSpace(line)
	if len(s) < 3 {
		return false
	}
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
	if len(t) > lvl && (t[lvl] == ' ' || t[lvl] == '\t') {
		return true, lvl
	}
	return false, 0
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

// Printing functions for different Markdown elements

func printHeading(p *printer.ThermalPrinter, text string, level int) error {
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
	for *idx < len(lines) {
		line := lines[*idx]
		if !isCodeLine(line) {
			break
		}

		// Strip one indent level
		if strings.HasPrefix(line, "    ") {
			line = line[4:]
		} else if strings.HasPrefix(line, "\t") {
			line = line[1:]
		}

		// Wrap long code lines
		for _, wl := range wrapText(line, width) {
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
	wrapped := wrapText(text, width-2)

	// Print with "| " prefix and underline to mimic italicized quote
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
		wrapped := wrapText(text, width-indent-2)
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

func printParagraph(p *printer.ThermalPrinter, lines []string, idx *int, width int) error {
	var parts []string
	for *idx < len(lines) {
		line := lines[*idx]
		if strings.TrimSpace(line) == "" || isHorizontalRule(line) || isCodeLine(line) ||
			isListItem(line) || isOrderedListItem(line) ||
			strings.HasPrefix(strings.TrimLeft(line, " \t"), ">") ||
			isPotentialHeading(line) {
			break
		}
		parts = append(parts, strings.TrimRight(line, " "))
		*idx++
	}

	text := strings.Join(parts, " ")
	for _, l := range wrapText(text, width) {
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

// printInline handles inline formatting like bold, italic, links
func printInline(p *printer.ThermalPrinter, text string) error {
	// Reset inline styles at line start
	_ = p.SetBold(false)
	_ = p.SetUnderline(false)

	i := 0
	bold := false
	underline := false

	// Apply current styles
	apply := func() error {
		if err := p.SetBold(bold); err != nil {
			return err
		}
		if err := p.SetUnderline(underline); err != nil {
			return err
		}
		return nil
	}

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
			// Image: ![alt](url) -> [Image: alt] (url)
			if alt, url, n, ok := parseLinkLike(text[i+1:]); ok {
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
			// Link: [text](url) -> text (url)
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
				// Restore previous styles
				bold, underline = prevBold, prevUnderline
				if err := apply(); err != nil {
					return err
				}
				i = i + 1 + j + 1
				continue
			}
		}

		// Bold/italic markers
		if strings.HasPrefix(text[i:], "***") || strings.HasPrefix(text[i:], "___") {
			if err := flush(); err != nil {
				return err
			}
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

		// Normal character
		buf.WriteByte(text[i])
		i++
	}

	if err := flush(); err != nil {
		return err
	}

	// End of line: reset styles and add LF
	if err := p.SetBold(false); err != nil {
		return err
	}
	if err := p.SetUnderline(false); err != nil {
		return err
	}
	return p.Send(append([]byte{}, escpos.LINEFEED...))
}

// parseLinkLike parses [label](url) and returns label, url, consumed, ok
func parseLinkLike(s string) (label, url string, consumed int, ok bool) {
	if len(s) == 0 || s[0] != '[' {
		return "", "", 0, false
	}

	end := strings.IndexByte(s, ']')
	if end <= 0 {
		return "", "", 0, false
	}
	label = s[1:end]

	rest := s[end+1:]
	if len(rest) == 0 || rest[0] != '(' {
		return label, "", end + 1, true
	}

	close := strings.IndexByte(rest, ')')
	if close < 0 {
		return label, "", end + 1, true
	}
	url = rest[1:close]
	consumed = end + 1 + close + 1
	return label, url, consumed, true
}

// wrapText splits text into lines not exceeding width
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{}
	}

	var lines []string
	var current strings.Builder

	for _, word := range words {
		if current.Len() == 0 {
			if len(word) <= width {
				current.WriteString(word)
			} else {
				// Hard-break long word
				for len(word) > width {
					lines = append(lines, word[:width])
					word = word[width:]
				}
				if len(word) > 0 {
					current.WriteString(word)
				}
			}
			continue
		}

		if current.Len()+1+len(word) <= width {
			current.WriteByte(' ')
			current.WriteString(word)
		} else {
			lines = append(lines, current.String())
			current.Reset()
			if len(word) <= width {
				current.WriteString(word)
			} else {
				for len(word) > width {
					lines = append(lines, word[:width])
					word = word[width:]
				}
				if len(word) > 0 {
					current.WriteString(word)
				}
			}
		}
	}

	if current.Len() > 0 {
		lines = append(lines, current.String())
	}

	return lines
}
