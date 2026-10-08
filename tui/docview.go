package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/gedcom"
)

// span is a piece of styled text.
type span struct {
	text  string
	style lipgloss.Style
}

// line is one row of a scrollable document. Lines with a target person or
// a query are selectable links.
type line struct {
	spans  []span
	target *gedcom.Individual
	query  string
}

func (l line) isLink() bool { return l.target != nil || l.query != "" }

func (l line) plain() string {
	var b strings.Builder
	for _, s := range l.spans {
		b.WriteString(s.text)
	}
	return b.String()
}

func textLine(parts ...span) line { return line{spans: parts} }

// docView is a vertically scrolling list of lines with a cursor that jumps
// between links. Documents without links simply scroll.
type docView struct {
	lines  []line
	links  []int // indices of link lines
	cursor int   // index into links
	offset int   // first visible line
}

// set replaces the content, keeping the cursor position where possible.
func (d *docView) set(lines []line) {
	d.lines = lines
	d.links = d.links[:0]
	for i, l := range lines {
		if l.isLink() {
			d.links = append(d.links, i)
		}
	}
	d.cursor = clamp(d.cursor, 0, len(d.links)-1)
	d.offset = clamp(d.offset, 0, max(0, len(d.lines)-1))
}

// reset moves to the top of the document.
func (d *docView) reset() { d.cursor, d.offset = 0, 0 }

// selected returns the selected link line, or nil.
func (d *docView) selected() *line {
	if len(d.links) == 0 {
		return nil
	}
	return &d.lines[d.links[d.cursor]]
}

func (d *docView) selectedLine() int {
	if len(d.links) == 0 {
		return -1
	}
	return d.links[d.cursor]
}

// move advances the cursor by delta links. When there is no further link
// in that direction the view scrolls instead, so that content after the
// last link (or before the first) can still be read.
func (d *docView) move(delta, height int) {
	if len(d.links) == 0 {
		d.scroll(delta, height)
		return
	}
	next := d.cursor + delta
	if next < 0 || next >= len(d.links) {
		d.cursor = clamp(next, 0, len(d.links)-1)
		d.scroll(delta, height)
		return
	}
	d.cursor = next
	d.ensureVisible(height)
}

// scroll moves the viewport without regard to links.
func (d *docView) scroll(delta, height int) {
	d.offset = clamp(d.offset+delta, 0, max(0, len(d.lines)-height))
}

// visible reports whether a line is inside the viewport.
func (d *docView) visible(i, height int) bool { return i >= d.offset && i < d.offset+height }

// page scrolls by a page. If the selected link leaves the screen, the
// nearest visible link is selected instead.
func (d *docView) page(dir, height int) {
	d.scroll(dir*max(1, height-1), height)
	if len(d.links) == 0 || d.visible(d.selectedLine(), height) {
		return
	}
	pick := -1
	for i, l := range d.links {
		if d.visible(l, height) {
			pick = i
			if dir > 0 {
				break
			}
		}
	}
	if pick >= 0 {
		d.cursor = pick
	}
}

func (d *docView) home(height int) {
	d.cursor, d.offset = 0, 0
	d.ensureVisible(height)
}

func (d *docView) end(height int) {
	d.cursor = max(0, len(d.links)-1)
	d.offset = max(0, len(d.lines)-height)
	d.ensureVisible(height)
}

// ensureVisible scrolls so that the selected link is on screen.
func (d *docView) ensureVisible(height int) {
	if height <= 0 {
		return
	}
	sel := d.selectedLine()
	if sel < 0 {
		d.offset = clamp(d.offset, 0, max(0, len(d.lines)-height))
		return
	}
	if sel < d.offset {
		d.offset = sel
	}
	if sel >= d.offset+height {
		d.offset = sel - height + 1
	}
	// Show the heading above the first link when scrolling to the top.
	if d.cursor == 0 && sel < height {
		d.offset = 0
	}
}

// render draws the visible part of the document.
func (d *docView) render(width, height int, st styles) string {
	var out []string
	sel := d.selectedLine()
	for i := d.offset; i < len(d.lines) && len(out) < height; i++ {
		l := d.lines[i]
		var s string
		if i == sel {
			s = st.selected.Render("▸ " + l.plain())
		} else {
			var b strings.Builder
			b.WriteString("  ")
			for _, sp := range l.spans {
				b.WriteString(sp.style.Render(sp.text))
			}
			s = b.String()
		}
		out = append(out, fit(s, width))
	}
	for len(out) < height {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}

// --- small helpers -----------------------------------------------------------

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}

// fit truncates a (possibly styled) string to width display columns.
func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "…")
}

// pad right-pads plain text to width display columns, truncating if needed.
func pad(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := ansi.StringWidth(s)
	if w > width {
		return ansi.Truncate(s, width, "…")
	}
	return s + strings.Repeat(" ", width-w)
}

// wrap breaks text into lines of at most width columns at spaces.
func wrap(text string, width int) []string {
	if width < 10 {
		width = 10
	}
	var out []string
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		cur := ""
		for _, w := range words {
			switch {
			case cur == "":
				cur = w
			case ansi.StringWidth(cur)+1+ansi.StringWidth(w) <= width:
				cur += " " + w
			default:
				out = append(out, cur)
				cur = w
			}
			for ansi.StringWidth(cur) > width {
				head := ansi.Truncate(cur, width, "")
				out = append(out, head)
				cur = strings.TrimPrefix(cur, head)
			}
		}
		out = append(out, cur)
	}
	return out
}
