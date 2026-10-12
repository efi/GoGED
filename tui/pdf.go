package tui

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"codeberg.org/go-pdf/fpdf"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/chart"
	"github.com/efi/goged/export"
	"github.com/efi/goged/fonts"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
	"github.com/mattn/go-runewidth"
	"golang.org/x/text/unicode/norm"
)

// The PDF export prints the lines of the views on A4 pages: a grid of 96
// columns of Martian Mono Condensed, 0.6 em wide, at 9 points. The last
// columns hold page references, so the views are built for a screen of
// pdfWidth columns, of which the interface uses two as a margin.
const (
	pdfFont       = "Martian Mono"
	pdfFontSize   = 9.0
	pdfCell       = 0.6 * pdfFontSize
	pdfColumns    = 96
	pdfRefColumns = 8
	pdfContent    = pdfColumns - pdfRefColumns // columns for the views
	pdfWidth      = pdfContent + 2
	pdfPageWidth  = 595.28
	pdfLeft       = (pdfPageWidth - pdfColumns*pdfCell) / 2
	pdfPitch      = 1.25 * pdfFontSize // from one baseline to the next
	pdfTop        = 52.0               // the first baseline
	pdfFooter     = 812.0              // the baseline of the footer
	pdfLines      = 65                 // lines per page
)

// pdfLine is a line of the PDF. If ref is not -1, it refers to that block:
// its page number is printed at the right margin and links to it.
type pdfLine struct {
	line
	ref int
}

// pdfBlock is a part of the PDF: the contents, a person's page or a
// place's section.
type pdfBlock struct {
	title   string // bookmark of the block, if any
	part    string // bookmark of the part the block begins, if any
	newPage bool
	lines   []pdfLine
}

// pdfPos is a line on a page, both counted from 0.
type pdfPos struct{ page, line int }

// ExportFormats returns the formats the Export tab offers for doc, for use
// without the interface: those of package export, and the PDF, which
// prints the views of this package.
func ExportFormats(doc *gedcom.Document, opts Options) []export.Format {
	m := New(doc, opts)
	return m.exportFormats()
}

// exportFormats lists the export formats: those of package export, and
// the PDF, which prints the views of this package.
func (m *Model) exportFormats() []export.Format {
	return append(slices.Clone(export.Formats), export.Format{
		Name:        "PDF",
		Description: "pages to print or read: the people, then the places (A4)",
		Extension:   ".pdf",
		Write:       m.writePDF,
	})
}

// pdfBlocks builds the PDF: a contents page, a page per person in name
// order, then the places with their events. The marked person is
// forgotten while the pages are built, so no one is starred and no
// relationships to them appear.
func (m *Model) pdfBlocks(sel *export.Selection, opts export.Options) []pdfBlock {
	p := *m
	p.reference = nil

	var people []*gedcom.Individual
	results, _ := p.index.SearchString("")
	for _, r := range results {
		if sel.Has(r.Individual) {
			people = append(people, r.Individual)
		}
	}
	blocks := []pdfBlock{{part: "Contents", newPage: true}}
	personBlock := map[*gedcom.Individual]int{}
	for i, ind := range people {
		personBlock[ind] = i + 1
	}
	ref := func(l line) pdfLine {
		if b, ok := personBlock[l.target]; ok && l.target != nil {
			return pdfLine{l, b}
		}
		return pdfLine{l, -1}
	}
	for i, ind := range people {
		b := pdfBlock{title: chart.Label(ind), newPage: true}
		if i == 0 {
			b.part = "People"
		}
		for _, l := range p.personLines(ind, pdfWidth) {
			b.lines = append(b.lines, ref(l))
		}
		blocks = append(blocks, b)
	}

	// Places, in the order of the hierarchy, with the events of the
	// selected people there.
	var places []*genealogy.PlaceNode
	genealogy.Places(p.doc).Walk(func(n *genealogy.PlaceNode) bool {
		var events []*gedcom.Event
		for _, ev := range n.Events {
			if slices.ContainsFunc(ev.Principals(), sel.Has) {
				events = append(events, ev)
			}
		}
		if len(events) == 0 {
			return true
		}
		slices.SortStableFunc(events, func(a, b *gedcom.Event) int { return a.Date.Compare(b.Date) })
		b := pdfBlock{title: n.Full, newPage: len(places) == 0}
		if len(places) == 0 {
			b.part = "Places"
		}
		heading := textLine(span{text: n.Full, style: p.st.section})
		heading.heading = true
		b.lines = append(b.lines, pdfLine{heading, -1})
		for _, info := range placeInfo(n) {
			for _, w := range wrap(info, pdfContent) {
				b.lines = append(b.lines, pdfLine{dimLine(&p, w), -1})
			}
		}
		for _, ev := range events {
			b.lines = append(b.lines, ref(p.eventLine(ev, pdfWidth)))
		}
		b.lines = append(b.lines, pdfLine{textLine(), -1})
		blocks = append(blocks, b)
		places = append(places, n)
		return true
	})

	// The contents: what the file holds, then indexes of the people and
	// the places.
	c := &blocks[0]
	add := func(l line, ref int) { c.lines = append(c.lines, pdfLine{l, ref}) }
	title := opts.Title
	if title == "" {
		title = "goged"
	}
	add(textLine(span{text: title, style: p.st.name}), -1)
	for _, w := range wrap(export.Describe(sel, opts), pdfContent) {
		add(dimLine(&p, w), -1)
	}
	if !opts.Generated.IsZero() {
		add(dimLine(&p, "exported by goged on "+opts.Generated.Format("2 Jan 2006")), -1)
	}
	add(textLine(), -1)
	add(p.sectionLine("People"), -1)
	for i, ind := range people {
		spans := []span{{text: ind.SortName(), style: p.st.name.UnsetBold(), width: 47}, {text: " "}, {text: ind.Lifespan(), style: p.st.dim}}
		add(textLine(spans...), i+1)
	}
	if len(places) > 0 {
		add(textLine(), -1)
		add(p.sectionLine("Places"), -1)
		for i, n := range places {
			add(textLine(span{text: n.Full, style: p.st.name.UnsetBold()}), len(people)+1+i)
		}
	}
	// Nothing is cut on paper: long lines are wrapped. The page reference
	// goes with the first row.
	for i := range blocks {
		var rows []pdfLine
		for _, l := range blocks[i].lines {
			for j, row := range expand([]line{l.line}, pdfContent) {
				ref := l.ref
				if j > 0 {
					ref = -1
				}
				rows = append(rows, pdfLine{row, ref})
			}
		}
		blocks[i].lines = rows
	}
	return blocks
}

// paginate distributes the blocks over pages of n lines. A block with
// newPage begins a page, and so does a block or a heading that would have
// fewer than three of its lines on the current page. The rows of a wrapped
// line stay together if they fit on a page. Blank lines are left out at
// the top of a page. paginate returns the pages and where each block
// begins.
func paginate(blocks []pdfBlock, n int) (pages [][]pdfLine, starts []pdfPos) {
	var page []pdfLine
	next := func() {
		if len(page) > 0 {
			pages = append(pages, page)
			page = nil
		}
	}
	for _, b := range blocks {
		if b.newPage || n-len(page) < min(3, len(b.lines)) {
			next()
		}
		started := false
		for i, l := range b.lines {
			rows := 1
			for !l.cont && i+rows < len(b.lines) && b.lines[i+rows].cont {
				rows++
			}
			if (l.heading && i > 0 && n-len(page) < 3) || len(page) == n || (!l.cont && len(page)+rows > n && rows <= n) {
				next()
			}
			if len(page) == 0 && strings.TrimSpace(l.plain()) == "" {
				continue
			}
			if !started {
				starts = append(starts, pdfPos{len(pages), len(page)})
				started = true
			}
			page = append(page, l)
		}
		if !started {
			starts = append(starts, pdfPos{len(pages), len(page)})
		}
	}
	next()
	return pages, starts
}

// writePDF writes the selection as pages to print.
func (m *Model) writePDF(w io.Writer, sel *export.Selection, opts export.Options) error {
	blocks := m.pdfBlocks(sel, opts)
	pages, starts := paginate(blocks, pdfLines)

	pdf := fpdf.New("P", "pt", "A4", "")
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetCreator("goged", true)
	pdf.SetTitle(opts.Title, true)
	pdf.SetSubject(export.Describe(sel, opts), true)
	if !opts.Generated.IsZero() {
		pdf.SetCreationDate(opts.Generated)
		pdf.SetModificationDate(opts.Generated)
	}
	pdf.AddUTF8FontFromBytes(pdfFont, "", fonts.Regular)
	pdf.AddUTF8FontFromBytes(pdfFont, "B", fonts.Bold)
	pdf.SetFont(pdfFont, "", pdfFontSize)

	// Links to every block, and the blocks that begin at each position.
	links := make([]int, len(blocks))
	begins := map[pdfPos][]int{}
	for i, s := range starts {
		links[i] = pdf.AddLink()
		pdf.SetLink(links[i], pdfTop+float64(s.line-1)*pdfPitch, s.page+1)
		begins[s] = append(begins[s], i)
	}

	grey := pdfColor(m.st.dim.GetForeground())
	for pi, page := range pages {
		pdf.AddPage()
		for li, l := range page {
			y := pdfTop + float64(li)*pdfPitch
			for _, bi := range begins[pdfPos{pi, li}] {
				pdf.SetFont(pdfFont, "", pdfFontSize)
				if p := blocks[bi].part; p != "" {
					pdf.Bookmark(p, 0, y-pdfPitch)
				}
				if t := blocks[bi].title; t != "" {
					pdf.Bookmark(t, 1, y-pdfPitch)
				}
			}
			pdfText(pdf, l.line, y)
			if l.ref >= 0 && l.ref < len(starts) {
				ref := fmt.Sprintf("p. %d", starts[l.ref].page+1)
				pdf.SetFont(pdfFont, "", pdfFontSize)
				pdf.SetTextColor(grey[0], grey[1], grey[2])
				pdf.Text(pdfLeft+float64(pdfColumns-len(ref))*pdfCell, y, ref)
				pdf.Link(pdfLeft, y-pdfFontSize, pdfColumns*pdfCell, pdfPitch, links[l.ref])
			}
		}
		// The footer: what this is and where.
		pdf.SetFont(pdfFont, "", pdfFontSize-1.5)
		pdf.SetTextColor(grey[0], grey[1], grey[2])
		pdf.Text(pdfLeft, pdfFooter, "goged · "+opts.Title)
		num := fmt.Sprintf("page %d of %d", pi+1, len(pages))
		pdf.Text(pdfLeft+pdfColumns*pdfCell-pdf.GetStringWidth(num), pdfFooter, num)
	}
	return pdf.Output(w)
}

// pdfText prints a line on the grid: every character takes as many
// columns as on a terminal, so the columns line up as on screen. Text is
// composed (NFC) first, since the font places marks as characters of
// their own.
func pdfText(pdf *fpdf.Fpdf, l line, y float64) {
	col := 0
	for _, sp := range l.spans {
		if col >= pdfContent {
			return
		}
		text := norm.NFC.String(sp.text)
		if col+ansi.StringWidth(text) > pdfContent {
			text = ansi.Truncate(text, pdfContent-col, "…")
		}
		style := ""
		if sp.style.GetBold() {
			style = "B"
		}
		pdf.SetFont(pdfFont, style, pdfFontSize)
		c := pdfColor(sp.style.GetForeground())
		pdf.SetTextColor(c[0], c[1], c[2])
		if w := ansi.StringWidth(text); w == len([]rune(text)) {
			if strings.TrimSpace(text) != "" {
				pdf.Text(pdfLeft+float64(col)*pdfCell, y, text)
			}
			col += w
			continue
		}
		// Wide characters take two columns; marks that did not compose go
		// over the character before them.
		for _, r := range text {
			w := runewidth.RuneWidth(r)
			x := col
			if w == 0 {
				x = max(0, col-1)
			}
			if r != ' ' {
				pdf.Text(pdfLeft+float64(x)*pdfCell, y, string(r))
			}
			col += w
		}
	}
}

// pdfColor returns the color of a style for printing on white paper: the
// light variant of adaptive colors, black if there is none.
func pdfColor(c lipgloss.TerminalColor) [3]int {
	var hex string
	switch c := c.(type) {
	case lipgloss.AdaptiveColor:
		hex = c.Light
	case lipgloss.Color:
		hex = string(c)
	}
	if v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32); err == nil && len(hex) == 7 {
		return [3]int{int(v >> 16), int(v >> 8 & 0xFF), int(v & 0xFF)}
	}
	return [3]int{0, 0, 0}
}
