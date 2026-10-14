package tui

import (
	"bufio"
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/export"
)

// htmlStyle lays the book out like the screen: a monospaced font, columns
// as wide as on screen (in ch), and the text after them wrapping where it
// began. Links are whole lines, as the selectable lines of the interface.
const htmlStyle = `
:root { color-scheme: light; }
body { margin: 0; background: #fff; color: #1a1a1a;
  font: 14px/1.4 ui-monospace, "SF Mono", Menlo, Consolas, "DejaVu Sans Mono", "Liberation Mono", monospace; }
main { max-width: 92ch; margin: 0 auto; padding: 1em 2ch 3em; }
section { padding: 0.7em 0; border-bottom: 1px solid #e6e6e6; }
section:target { background: #faf8ff; }
.l { display: flex; min-height: 1.4em; white-space: pre-wrap; overflow-wrap: anywhere; color: inherit; text-decoration: none; }
.c { flex: none; }
.t { flex: 1; min-width: 0; }
a.l:hover { background: #f0ecfa; }
.up { display: block; margin-top: 0.4em; color: #7a7a7a; text-decoration: none; }
footer { max-width: 92ch; margin: 0 auto; padding: 0 2ch 2em; color: #7a7a7a; }
`

// writeHTML writes the book as one web page: the contents, then a section
// per person and per place, linked to each other.
func (m *Model) writeHTML(w io.Writer, sel *export.Selection, opts export.Options) error {
	blocks := m.bookBlocks(sel, opts)
	title := opts.Title
	if title == "" {
		title = "goged"
	}
	b := bufio.NewWriter(w)
	fmt.Fprintf(b, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<meta name="generator" content="goged" />
<meta name="description" content="%s" />
<title>%s</title>
<style>%s</style>
</head>
<body>
<main>
`, htmlText(export.Describe(sel, opts)), htmlText(title), htmlStyle)
	for i, bl := range blocks {
		fmt.Fprintf(b, "<section id=\"%s\">\n", htmlText(bl.id))
		for _, l := range bl.lines {
			ref := ""
			if l.ref >= 0 && l.ref < len(blocks) {
				ref = blocks[l.ref].id
			}
			htmlLine(b, l.line, ref)
		}
		if i > 0 {
			b.WriteString("<a class=\"up\" href=\"#contents\">↑ contents</a>\n")
		}
		b.WriteString("</section>\n")
	}
	b.WriteString("</main>\n<footer>Exported by goged</footer>\n</body>\n</html>\n")
	return b.Flush()
}

// htmlLine writes a line: its columns with their width, then the text
// after them, which wraps where it began and keeps its leading spaces as
// indentation. A line that refers to a block links to it.
func htmlLine(b *bufio.Writer, l line, ref string) {
	tag, attrs := "div", ""
	if ref != "" {
		tag, attrs = "a", fmt.Sprintf(` href="#%s"`, htmlText(ref))
	}
	fmt.Fprintf(b, `<%s class="l"%s>`, tag, attrs)
	tail := l.tail()
	for _, sp := range l.spans[:tail] {
		width := sp.width
		if width == 0 {
			width = ansi.StringWidth(sp.text)
		}
		fmt.Fprintf(b, `<span class="c" style="width:%dch">`, width)
		htmlSpan(b, sp.text, sp)
		b.WriteString("</span>")
	}
	if rest := l.spans[tail:]; len(rest) > 0 {
		var all strings.Builder
		for _, sp := range rest {
			all.WriteString(sp.text)
		}
		hang := len(all.String()) - len(strings.TrimLeft(all.String(), " "))
		if hang > 0 {
			fmt.Fprintf(b, `<span class="t" style="padding-left:%dch">`, hang)
		} else {
			b.WriteString(`<span class="t">`)
		}
		for _, sp := range rest {
			text := sp.text
			if hang > 0 {
				n := min(hang, len(text)-len(strings.TrimLeft(text, " ")))
				text, hang = text[n:], hang-n
			}
			htmlSpan(b, text, sp)
		}
		b.WriteString("</span>")
	}
	fmt.Fprintf(b, "</%s>\n", tag)
}

// htmlSpan writes text in the color and weight of the span's style.
func htmlSpan(b *bufio.Writer, text string, sp span) {
	if text == "" {
		return
	}
	var css []string
	if c := paperColor(sp.style.GetForeground()); c != [3]int{} {
		css = append(css, fmt.Sprintf("color:#%02x%02x%02x", c[0], c[1], c[2]))
	}
	if sp.style.GetBold() {
		css = append(css, "font-weight:bold")
	}
	if len(css) == 0 {
		b.WriteString(htmlText(text))
		return
	}
	fmt.Fprintf(b, `<span style="%s">%s</span>`, strings.Join(css, ";"), htmlText(text))
}

// htmlText escapes text for HTML and replaces control characters, which
// HTML does not allow.
func htmlText(s string) string {
	return html.EscapeString(strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' || r == 0x7f {
			return '\uFFFD'
		}
		return r
	}, s))
}
