package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/efi/goged/chart"
	"github.com/efi/goged/export"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
)

// The PDF and HTML exports present the lines of the views as a book: the
// contents, a block per person and the places. The views are built for a
// screen of bookWidth columns, of which the interface uses two as a
// margin, so the lines have bookContent columns.
const (
	bookContent = 88
	bookWidth   = bookContent + 2
)

// bookLine is a line of the book. If ref is not -1, it refers to that
// block: the PDF prints its page number and links to it, the HTML links
// to it.
type bookLine struct {
	line
	ref int
}

// bookBlock is a part of the book: the contents, a person or a place.
type bookBlock struct {
	id      string // anchor of the block in the HTML
	title   string // bookmark of the block, if any
	part    string // bookmark of the part the block begins, if any
	newPage bool
	lines   []bookLine
}

// anchor turns an identifier into one that is safe in a URL fragment.
func anchor(id string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, id)
}

// ExportFormats returns the formats the Export tab offers for doc, for use
// without the interface: those of package export, and the PDF and HTML,
// which present the views of this package.
func ExportFormats(doc *gedcom.Document, opts Options) []export.Format {
	m := New(doc, opts)
	return m.exportFormats()
}

// exportFormats lists the export formats: those of package export, and
// the PDF and HTML, which present the views of this package.
func (m *Model) exportFormats() []export.Format {
	return append(slices.Clone(export.Formats), export.Format{
		Name:        "PDF",
		Description: "pages to print or read: the people, then the places (A4)",
		Extension:   ".pdf",
		Write:       m.writePDF,
	}, export.Format{
		Name:        "HTML",
		Description: "one web page: the people, then the places, with links",
		Extension:   ".html",
		Write:       m.writeHTML,
	})
}

// bookBlocks builds the book: the contents, a block per person in name
// order, then the places with their events. The marked person is
// forgotten while the blocks are built, so no one is starred and no
// relationships to them appear.
func (m *Model) bookBlocks(sel *export.Selection, opts export.Options) []bookBlock {
	p := *m
	p.reference = nil

	var people []*gedcom.Individual
	results, _ := p.index.SearchString("")
	for _, r := range results {
		if sel.Has(r.Individual) {
			people = append(people, r.Individual)
		}
	}
	blocks := []bookBlock{{id: "contents", part: "Contents", newPage: true}}
	personBlock := map[*gedcom.Individual]int{}
	for i, ind := range people {
		personBlock[ind] = i + 1
	}
	ref := func(l line) bookLine {
		if b, ok := personBlock[l.target]; ok && l.target != nil {
			return bookLine{l, b}
		}
		return bookLine{l, -1}
	}
	for i, ind := range people {
		b := bookBlock{id: anchor(ind.ID), title: chart.Label(ind), newPage: true}
		if i == 0 {
			b.part = "People"
		}
		for _, l := range p.personLines(ind, bookWidth) {
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
		b := bookBlock{id: fmt.Sprintf("place-%d", len(places)+1), title: n.Full, newPage: len(places) == 0}
		if len(places) == 0 {
			b.part = "Places"
		}
		heading := textLine(span{text: n.Full, style: p.st.section})
		heading.heading = true
		b.lines = append(b.lines, bookLine{heading, -1})
		for _, info := range placeInfo(n) {
			for _, w := range wrap(info, bookContent) {
				b.lines = append(b.lines, bookLine{dimLine(&p, w), -1})
			}
		}
		for _, ev := range events {
			b.lines = append(b.lines, ref(p.eventLine(ev, bookWidth)))
		}
		b.lines = append(b.lines, bookLine{textLine(), -1})
		blocks = append(blocks, b)
		places = append(places, n)
		return true
	})

	// The contents: what the file holds, then indexes of the people and
	// the places.
	c := &blocks[0]
	add := func(l line, ref int) { c.lines = append(c.lines, bookLine{l, ref}) }
	title := opts.Title
	if title == "" {
		title = "goged"
	}
	add(textLine(span{text: title, style: p.st.name}), -1)
	for _, w := range wrap(export.Describe(sel, opts), bookContent) {
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
	return blocks
}

// paperColor returns the color of a style for printing on white paper: the
// light variant of adaptive colors, black if there is none.
func paperColor(c lipgloss.TerminalColor) [3]int {
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
