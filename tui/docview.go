package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/gedcom"
	"github.com/mattn/go-runewidth"
)

// span is a piece of styled text. A span with a width is a column: it
// takes exactly that many columns, and its text is cut or wrapped within
// them. Text without a width flows (see line.layout).
type span struct {
	text  string
	style lipgloss.Style
	width int
}

// line is one row of a scrollable document. Lines with a target person or
// a query are selectable links. A heading starts a section; printed pages
// keep it together with what follows. A continuation (cont) is a further
// row of the line before it, made by wrapping. Selected, a line is shown
// in reverse; with colsOnly, only its columns are, and the text after
// them keeps its style, so that a bar keeps its look.
type line struct {
	spans    []span
	target   *gedcom.Individual
	query    string
	heading  bool
	cont     bool
	colsOnly bool
}

func (l line) isLink() bool { return l.target != nil || l.query != "" }

// plain returns the text of the line as one row, with cut columns.
func (l line) plain() string {
	var b strings.Builder
	for _, s := range l.layout(0, false)[0] {
		b.WriteString(s.text)
	}
	return b.String()
}

// tail returns the index of the first span after the last column; the
// spans from there on flow.
func (l line) tail() int {
	t := len(l.spans)
	for t > 0 && l.spans[t-1].width == 0 {
		t--
	}
	return t
}

// layout arranges the line in rows width columns wide. Columns keep their
// place and width, and so does text between them. The text after the last
// column flows. Cut, there is a single row: columns are cut to their
// width, and the flowing text runs on for the caller to cut. Wrapped,
// every column continues on further rows within its place, and the
// flowing text continues where it began, after its leading spaces.
func (l line) layout(width int, wrapped bool) [][]span {
	tail := l.tail()
	type cell struct {
		x    int
		rows [][]span
	}
	var cells []cell
	x := 0
	for _, sp := range l.spans[:tail] {
		c := cell{x: x}
		switch {
		case sp.width == 0:
			c.rows = [][]span{{sp}}
			x += ansi.StringWidth(sp.text)
		case wrapped:
			c.rows = flow([]span{sp}, sp.width, sp.width, 0)
			x += sp.width
		default:
			c.rows = [][]span{{{text: pad(sp.text, sp.width), style: sp.style}}}
			x += sp.width
		}
		cells = append(cells, c)
	}
	if rest := l.spans[tail:]; len(rest) > 0 {
		c := cell{x: x}
		if wrapped {
			var text strings.Builder
			for _, sp := range rest {
				text.WriteString(sp.text)
			}
			room := max(1, width-x)
			hang := len(text.String()) - len(strings.TrimLeft(text.String(), " "))
			hang = min(hang, max(0, room-10))
			c.rows = flow(rest, room, room-hang, hang)
		} else {
			c.rows = [][]span{rest}
		}
		cells = append(cells, c)
	}

	n := 1
	for _, c := range cells {
		n = max(n, len(c.rows))
	}
	rows := make([][]span, n)
	for r := range rows {
		col := 0
		for _, c := range cells {
			if r >= len(c.rows) {
				continue
			}
			if c.x > col {
				rows[r] = append(rows[r], span{text: strings.Repeat(" ", c.x-col)})
				col = c.x
			}
			for _, sp := range c.rows[r] {
				rows[r] = append(rows[r], span{text: sp.text, style: sp.style})
				col += ansi.StringWidth(sp.text)
			}
		}
	}
	return rows
}

// flow breaks styled text into rows: the first first columns wide, the
// others rest columns wide after hang spaces. Rows break at spaces and
// after hyphens within words; a word longer than a row is split, and
// characters of no width (combining marks) stay with the one before them.
func flow(spans []span, first, rest, hang int) [][]span {
	var runes []rune
	var from []int // the span each rune comes from
	for i, sp := range spans {
		for _, r := range sp.text {
			runes = append(runes, r)
			from = append(from, i)
		}
	}
	first, rest = max(1, first), max(1, rest)
	var rows [][]span
	for pos := 0; pos < len(runes) || len(rows) == 0; {
		room := first
		if len(rows) > 0 {
			room = rest
			for pos < len(runes) && runes[pos] == ' ' {
				pos++
			}
			if pos == len(runes) {
				break
			}
		}
		end, used, brk := pos, 0, -1 // brk: where the row may end
		for end < len(runes) {
			w := runewidth.RuneWidth(runes[end])
			if used+w > room {
				break
			}
			switch {
			case runes[end] == ' ':
				brk = end
			case runes[end] == '-' && end > pos && runes[end-1] != ' ' && end+1 < len(runes) && runes[end+1] != ' ':
				brk = end + 1
			}
			used += w
			end++
		}
		if end < len(runes) && runes[end] != ' ' && brk > pos {
			end = brk
		}
		if end == pos && pos < len(runes) {
			end = pos + 1
		}
		for end < len(runes) && runewidth.RuneWidth(runes[end]) == 0 {
			end++
		}
		var row []span
		if len(rows) > 0 && hang > 0 {
			row = append(row, span{text: strings.Repeat(" ", hang)})
		}
		for i := pos; i < end; {
			j := i
			for j < end && from[j] == from[i] {
				j++
			}
			row = append(row, span{text: string(runes[i:j]), style: spans[from[i]].style})
			i = j
		}
		rows = append(rows, row)
		pos = end
	}
	return rows
}

// expand wraps lines to width columns: every line becomes its rows, the
// further ones marked as continuations of it.
func expand(lines []line, width int) []line {
	var out []line
	for _, l := range lines {
		for i, row := range l.layout(width, true) {
			out = append(out, line{spans: row, target: l.target, query: l.query, heading: l.heading && i == 0, cont: i > 0})
		}
	}
	return out
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
		if l.isLink() && !l.cont {
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

// page scrolls by a page.
func (d *docView) page(dir, height int) { d.scrollBy(dir*max(1, height-1), height) }

// scrollBy scrolls by delta lines. If the selected link leaves the
// screen, the nearest visible link is selected instead.
func (d *docView) scrollBy(delta, height int) {
	d.scroll(delta, height)
	if len(d.links) == 0 || d.visible(d.selectedLine(), height) {
		return
	}
	pick := -1
	for i, l := range d.links {
		if d.visible(l, height) {
			pick = i
			if delta > 0 {
				break
			}
		}
	}
	if pick >= 0 {
		d.cursor = pick
	}
}

// linkAt returns the link shown in row y of the view (by any of its rows),
// or -1.
func (d *docView) linkAt(y int) int {
	i := d.offset + y
	if y < 0 || i >= len(d.lines) {
		return -1
	}
	for i > 0 && d.lines[i].cont {
		i--
	}
	for k, l := range d.links {
		if l == i {
			return k
		}
	}
	return -1
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
	// Show the rows of a wrapped link as well, if they fit.
	last := sel
	for last+1 < len(d.lines) && d.lines[last+1].cont {
		last++
	}
	if last >= d.offset+height && last-sel < height {
		d.offset = last - height + 1
	}
	// Show the heading above the first link when scrolling to the top.
	if d.cursor == 0 && last < height {
		d.offset = 0
	}
}

// render draws the visible part of the document.
func (d *docView) render(width, height int, st styles) string {
	var out []string
	sel := d.selectedLine()
	// The rows of a wrapped link are selected with it.
	head := d.offset
	for head > 0 && head < len(d.lines) && d.lines[head].cont {
		head--
	}
	selected := sel >= 0 && head == sel
	for i := d.offset; i < len(d.lines) && len(out) < height; i++ {
		l := d.lines[i]
		if !l.cont {
			selected = i == sel
		}
		var s string
		switch {
		case selected && l.colsOnly:
			cols := line{spans: l.spans[:l.tail()]}
			var b strings.Builder
			b.WriteString(st.selected.Render("▸ " + cols.plain()))
			for _, sp := range l.spans[l.tail():] {
				b.WriteString(sp.style.Render(sp.text))
			}
			s = b.String()
		case selected:
			marker := "▸ "
			if l.cont {
				marker = "  "
			}
			s = st.selected.Render(marker + l.plain())
		default:
			var b strings.Builder
			b.WriteString("  ")
			for _, sp := range l.layout(width-2, false)[0] {
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
