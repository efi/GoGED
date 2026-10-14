package tui

import (
	"fmt"
	"io"
	"strings"

	"codeberg.org/go-pdf/fpdf"
	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/export"
	"github.com/efi/goged/fonts"
	"github.com/mattn/go-runewidth"
	"golang.org/x/text/unicode/norm"
)

// The PDF export prints the book on A4 pages: a grid of 96 columns of
// Martian Mono Condensed, 0.6 em wide, at 9 points. The lines of the book
// take bookContent columns; the last ones hold page references.
const (
	pdfFont       = "Martian Mono"
	pdfFontSize   = 9.0
	pdfCell       = 0.6 * pdfFontSize
	pdfRefColumns = 8
	pdfContent    = bookContent
	pdfColumns    = pdfContent + pdfRefColumns
	pdfPageWidth  = 595.28
	pdfLeft       = (pdfPageWidth - pdfColumns*pdfCell) / 2
	pdfPitch      = 1.25 * pdfFontSize // from one baseline to the next
	pdfTop        = 52.0               // the first baseline
	pdfFooter     = 812.0              // the baseline of the footer
	pdfLines      = 65                 // lines per page
)

// pdfPos is a line on a page, both counted from 0.
type pdfPos struct{ page, line int }

// paginate distributes the blocks over pages of n lines. A block with
// newPage begins a page, and so does a block or a heading that would have
// fewer than three of its lines on the current page. The rows of a wrapped
// line stay together if they fit on a page. Blank lines are left out at
// the top of a page. paginate returns the pages and where each block
// begins.
func paginate(blocks []bookBlock, n int) (pages [][]bookLine, starts []pdfPos) {
	var page []bookLine
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

// wrapToPage wraps the lines of the blocks to the width of the page, so
// that nothing is cut on paper. The page reference goes with the first row
// of a line.
func wrapToPage(blocks []bookBlock) {
	for i := range blocks {
		var rows []bookLine
		for _, l := range blocks[i].lines {
			for j, row := range expand([]line{l.line}, pdfContent) {
				ref := l.ref
				if j > 0 {
					ref = -1
				}
				rows = append(rows, bookLine{row, ref})
			}
		}
		blocks[i].lines = rows
	}
}

// writePDF writes the selection as pages to print.
func (m *Model) writePDF(w io.Writer, sel *export.Selection, opts export.Options) error {
	blocks := m.bookBlocks(sel, opts)
	wrapToPage(blocks)
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

	grey := paperColor(m.st.dim.GetForeground())
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
		c := paperColor(sp.style.GetForeground())
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
