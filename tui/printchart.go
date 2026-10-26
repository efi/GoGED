package tui

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"codeberg.org/go-pdf/fpdf"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/efi/goged/chart"
	"github.com/efi/goged/export"
	"github.com/efi/goged/fonts"
	"github.com/mattn/go-runewidth"
	"golang.org/x/text/unicode/norm"
)

// Printing a chart (P): the whole chart as the screen would show it if it
// were large enough, with its heading but without the selection, in the
// colours of paper. The page is A4, turned to landscape for a wide chart,
// and grows as far as the chart needs. A dialog asks for the file name
// first; it suggests one next to the GEDCOM file that is not taken yet.

// printMargin is the margin around a printed chart, half an inch.
const printMargin = 36.0

// A4 in points.
const (
	a4Short = 595.28
	a4Long  = 841.89
)

// askPrintChart asks for the file to print the chart shown to.
func (m *Model) askPrintChart(path string, text ...string) {
	input := newTextInput("File: ", "file name")
	input.SetValue(path)
	input.CursorEnd()
	input.Focus()
	if len(text) == 0 {
		text = []string{"Print the " + strings.ToLower(chartNames[m.tree.mode]) + " chart of " + m.current().DisplayName() + " to a PDF file:"}
	}
	d := &dialog{
		title:   "Print chart",
		text:    text,
		choices: []string{"Print", "Cancel"},
		cancel:  -1,
		hints:   [][2]string{{"type", "file name"}, {"enter", "print"}, {"esc", "cancel"}},
		input:   &input,
	}
	confirmed := ""
	if len(text) > 1 { // asked again: the file exists
		confirmed = path
	}
	d.answer = func(m *Model, choice int) tea.Cmd {
		if choice == 0 {
			m.printChart(strings.TrimSpace(d.input.Value()), confirmed)
		}
		return nil
	}
	m.ask(d)
}

// chartFileName suggests a file for the chart shown: next to the GEDCOM
// file, named after it, the chart and the person, and not taken yet:
// family-pedigree-I3.pdf, family-pedigree-I3-2.pdf …
func (m *Model) chartFileName() string {
	dir, base := "", strings.TrimSuffix(m.opts.Title, filepath.Ext(m.opts.Title))
	if m.opts.Path != "" {
		dir = filepath.Dir(m.opts.Path)
	}
	if base == "" || m.opts.Title == "stdin" {
		base = "goged"
	}
	name := base + "-" + strings.ToLower(chartNames[m.tree.mode]) + "-" + anchor(m.current().ID)
	if m.opts.Save != nil {
		return name + ".pdf" // for the browser, which finds a free name itself
	}
	path := filepath.Join(dir, name+".pdf")
	for n := 2; fileExists(path); n++ {
		path = filepath.Join(dir, fmt.Sprintf("%s-%d.pdf", name, n))
	}
	return path
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// printChart writes the chart shown to path. An existing file is only
// replaced if it is confirmed, else the dialog asks again.
func (m *Model) printChart(path, confirmed string) {
	name := filepath.Base(path)
	switch {
	case path == "" || name == "." || name == string(filepath.Separator):
		m.setError("no file name: nothing printed")
		return
	case m.opts.Save != nil:
		// The browser takes the file: no file here is written or replaced.
	case m.opts.Path != "" && absPath(path) == absPath(m.opts.Path):
		m.setError("the chart would replace the GEDCOM file " + m.opts.Title)
		return
	case fileExists(path) && path != confirmed:
		m.askPrintChart(path, path+" exists.", "Print replaces it:")
		return
	}
	lines := m.printedChart()
	w, h := chartPage(lines)
	write := func(out io.Writer) error { return writeChartPDF(out, lines, w, h, m.chartTitle()) }
	var err error
	where := name
	if m.opts.Save != nil {
		err = m.opts.Save(name, write)
		where += " for the browser to save"
	} else {
		err = export.WriteFile(path, export.Format{Write: func(out io.Writer, _ *export.Selection, _ export.Options) error { return write(out) }}, nil, export.Options{})
		if abs, err := filepath.Abs(path); err == nil {
			where += " in " + filepath.Dir(abs)
		}
	}
	if err != nil {
		m.setError("printing failed: " + err.Error())
		return
	}
	m.setStatus(fmt.Sprintf("printed the %s chart to %s, on a page of %.0f × %.0f mm", strings.ToLower(chartNames[m.tree.mode]), where, w/72*25.4, h/72*25.4))
}

func absPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

// chartTitle names the chart shown and the person: "Pedigree of John
// Smith".
func (m Model) chartTitle() string {
	return chartNames[m.tree.mode] + " of " + m.current().DisplayName()
}

// printedChart returns the lines to print: the heading, a blank line and
// every line of the chart, styled as on screen without the selection.
// The lifespans are drawn at their natural width, not the screen's.
func (m *Model) printedChart() []line {
	c := m.tree.chart
	if m.tree.mode == modeLifespans {
		c = chart.Lifespans(m.current(), chart.LifespanOptions{Generations: m.tree.lifeGens, ASCII: m.opts.ASCII})
	}
	heading := append(m.nameSpans(m.chartTitle(), m.st.section, m.current()), span{text: m.chartReach(), style: m.st.dim})
	lines := []line{textLine(heading...), textLine()}
	for _, segs := range c.Lines {
		var l line
		for _, seg := range segs {
			style, ok := m.segmentStyle(c, seg, -1)
			if !ok {
				style = lipgloss.NewStyle()
			}
			l.spans = append(l.spans, span{text: seg.Text, style: style})
		}
		lines = append(lines, l)
	}
	return lines
}

// chartPage returns the size of the page for the lines in points: A4,
// landscape if the lines are wider than a portrait page, and larger as
// far as they need.
func chartPage(lines []line) (w, h float64) {
	cols := 0
	for _, l := range lines {
		n := 0
		for _, sp := range l.spans {
			n += runewidth.StringWidth(sp.text)
		}
		cols = max(cols, n)
	}
	contentW := float64(cols)*pdfCell + 2*printMargin
	contentH := float64(len(lines))*pdfPitch + 2*printMargin
	w, h = a4Short, a4Long
	if contentW > a4Short {
		w, h = a4Long, a4Short
	}
	return math.Max(w, contentW), math.Max(h, contentH)
}

// writeChartPDF writes the lines on a page of w × h points.
func writeChartPDF(out io.Writer, lines []line, w, h float64, title string) error {
	pdf := fpdf.NewCustom(&fpdf.InitType{UnitStr: "pt", Size: fpdf.SizeType{Wd: w, Ht: h}})
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetCreator("goged", true)
	pdf.SetTitle(title, true)
	pdf.AddUTF8FontFromBytes(pdfFont, "", fonts.Regular)
	pdf.AddUTF8FontFromBytes(pdfFont, "B", fonts.Bold)
	pdf.AddPage()
	for i, l := range lines {
		// The baseline of the line; the axis of its line drawing lies a
		// third of the font size above it.
		y := printMargin + 0.8*pdfPitch + float64(i)*pdfPitch
		col := 0
		for _, sp := range l.spans {
			style := ""
			if sp.style.GetBold() {
				style = "B"
			}
			pdf.SetFont(pdfFont, style, pdfFontSize)
			c := paperColor(sp.style.GetForeground())
			pdf.SetTextColor(c[0], c[1], c[2])
			pdf.SetDrawColor(c[0], c[1], c[2])
			pdf.SetFillColor(c[0], c[1], c[2])
			for _, r := range norm.NFC.String(sp.text) {
				x := printMargin + float64(col)*pdfCell
				rw := runewidth.RuneWidth(r)
				switch {
				case r == ' ':
				case drawGlyph(pdf, r, x, y):
				case rw == 0: // a mark that did not compose goes over the character before it
					pdf.Text(x-pdfCell, y, string(r))
				default:
					pdf.Text(x, y, string(r))
				}
				col += rw
			}
		}
	}
	return pdf.Output(out)
}

// Line weights of drawn lines, in points.
const (
	lightLine = 0.5
	heavyLine = 1.8
)

// boxArms gives the line drawing characters that are drawn rather than
// printed, as the arms from the middle of a cell: north, east, south and
// west; heavy lines and dashed ones (dashes per cell) are set apart.
var boxArms = map[rune]struct {
	n, e, s, w bool
	heavy      bool
	dashes     int
}{
	'─': {e: true, w: true}, '│': {n: true, s: true},
	'┌': {e: true, s: true}, '┐': {s: true, w: true}, '└': {n: true, e: true}, '┘': {n: true, w: true},
	'├': {n: true, e: true, s: true}, '┤': {n: true, s: true, w: true},
	'┬': {e: true, s: true, w: true}, '┴': {n: true, e: true, w: true}, '┼': {n: true, e: true, s: true, w: true},
	'━': {e: true, w: true, heavy: true}, '╸': {w: true, heavy: true}, '╺': {e: true, heavy: true},
	'╍': {e: true, w: true, heavy: true, dashes: 2}, '┄': {e: true, w: true, dashes: 3}, '┊': {n: true, s: true, dashes: 4},
}

// drawGlyph draws a character that is drawn rather than printed: line
// drawing, which so joins without gaps from line to line, and the
// symbols the font lacks. x is the left edge of its cell and y the
// baseline. It reports whether it drew r.
func drawGlyph(pdf *fpdf.Fpdf, r rune, x, y float64) bool {
	cw := pdfCell
	cx, cy := x+cw/2, y-pdfFontSize/3
	top, bottom := cy-pdfPitch/2, cy+pdfPitch/2
	if a, ok := boxArms[r]; ok {
		width := lightLine
		if a.heavy {
			width = heavyLine
		}
		pdf.SetLineWidth(width)
		pdf.SetLineCapStyle("butt")
		dashed := func(x1, y1, x2, y2 float64) {
			if a.dashes == 0 {
				pdf.Line(x1, y1, x2, y2)
				return
			}
			n := float64(a.dashes)
			for k := 0.0; k < n; k++ {
				f1, f2 := (k+0.2)/n, (k+0.8)/n
				pdf.Line(x1+(x2-x1)*f1, y1+(y2-y1)*f1, x1+(x2-x1)*f2, y1+(y2-y1)*f2)
			}
		}
		switch {
		case a.e && a.w:
			dashed(x, cy, x+cw, cy)
		case a.e:
			pdf.Line(cx, cy, x+cw, cy)
		case a.w:
			pdf.Line(x, cy, cx, cy)
		}
		switch {
		case a.n && a.s:
			dashed(cx, top, cx, bottom)
		case a.n:
			pdf.Line(cx, top, cx, cy)
		case a.s:
			pdf.Line(cx, cy, cx, bottom)
		}
		return true
	}
	u := cw / 2 // the half cell: the shapes fit into it
	pdf.SetLineWidth(lightLine)
	pdf.SetLineCapStyle("round")
	triangle := func(pts ...float64) {
		pdf.Polygon([]fpdf.PointType{{X: cx + pts[0]*u, Y: cy + pts[1]*u}, {X: cx + pts[2]*u, Y: cy + pts[3]*u},
			{X: cx + pts[4]*u, Y: cy + pts[5]*u}}, "F")
	}
	switch r {
	case '▸':
		triangle(-0.5, -0.6, -0.5, 0.6, 0.7, 0)
	case '◂':
		triangle(0.5, -0.6, 0.5, 0.6, -0.7, 0)
	case '▾':
		triangle(-0.6, -0.4, 0.6, -0.4, 0, 0.8)
	case '●':
		pdf.Circle(cx, cy, 0.6*u, "F")
	case '◉':
		pdf.Circle(cx, cy, 0.8*u, "D")
		pdf.Circle(cx, cy, 0.4*u, "F")
	case '⚭': // two rings, linked
		pdf.Circle(cx-0.35*u, cy, 0.55*u, "D")
		pdf.Circle(cx+0.35*u, cy, 0.55*u, "D")
	case '★':
		var pts []fpdf.PointType
		for k := range 10 {
			rad := 0.95 * u
			if k%2 == 1 {
				rad = 0.4 * u
			}
			a := -math.Pi/2 + float64(k)*math.Pi/5
			pts = append(pts, fpdf.PointType{X: cx + rad*math.Cos(a), Y: cy + rad*math.Sin(a)})
		}
		pdf.Polygon(pts, "F")
	case '≥':
		pdf.Line(cx-0.6*u, cy-0.9*u, cx+0.6*u, cy-0.3*u)
		pdf.Line(cx+0.6*u, cy-0.3*u, cx-0.6*u, cy+0.3*u)
		pdf.Line(cx-0.6*u, cy+0.8*u, cx+0.6*u, cy+0.8*u)
	default:
		return false
	}
	return true
}
