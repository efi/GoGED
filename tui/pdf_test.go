package tui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/efi/goged/export"
)

// plainLines makes PDF lines from text; "#" marks a heading, "" is blank.
func plainLines(texts ...string) []pdfLine {
	var out []pdfLine
	for _, t := range texts {
		l := textLine(span{strings.TrimPrefix(t, "#"), lipgloss.NewStyle()})
		l.heading = strings.HasPrefix(t, "#")
		out = append(out, pdfLine{l, -1})
	}
	return out
}

func pageTexts(pages [][]pdfLine) string {
	var out []string
	for _, p := range pages {
		var ls []string
		for _, l := range p {
			ls = append(ls, l.plain())
		}
		out = append(out, strings.Join(ls, ","))
	}
	return strings.Join(out, " | ")
}

func TestPaginate(t *testing.T) {
	blocks := []pdfBlock{
		{newPage: true, lines: plainLines("a1", "a2", "", "#h", "a3", "a4")},
		{newPage: true, lines: plainLines("", "b1")},      // the leading blank is left out
		{lines: plainLines("c1", "c2", "c3", "c4", "c5")}, // four lines fit after b1
		{lines: plainLines("d1", "d2", "d3")},
		{lines: plainLines("e1", "e2")}, // only one line would fit after d3
	}
	pages, starts := paginate(blocks, 5)
	// The heading would have fewer than three lines on the first page.
	want := "a1,a2, | h,a3,a4 | b1,c1,c2,c3,c4 | c5,d1,d2,d3 | e1,e2"
	if got := pageTexts(pages); got != want {
		t.Errorf("pages = %s\nwant    %s", got, want)
	}
	if fmt.Sprint(starts) != "[{0 0} {2 0} {2 1} {3 1} {4 0}]" {
		t.Errorf("starts = %v", starts)
	}

	// Short blocks share a page; a full page continues on the next.
	pages, starts = paginate([]pdfBlock{
		{lines: plainLines("a1", "a2")},
		{lines: plainLines("b1", "b2", "b3", "b4")},
	}, 5)
	if got := pageTexts(pages); got != "a1,a2,b1,b2,b3 | b4" || fmt.Sprint(starts) != "[{0 0} {0 2}]" {
		t.Errorf("pages = %s, starts = %v", got, starts)
	}
}

func TestPDFBlocks(t *testing.T) {
	a := newMusterApp(t)
	a.openPerson("I1")
	a.press("m") // the PDF forgets the marked person
	m := &a.m
	sel, err := export.Select(m.doc, export.Options{})
	if err != nil {
		t.Fatal(err)
	}
	blocks := m.pdfBlocks(sel, export.Options{Title: "muster.ged", Generated: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)})
	if m.reference == nil || m.reference.ID != "I1" {
		t.Error("the marked person was cleared")
	}
	// Lines wider than the page are cut when printed, as on screen.
	for i, b := range blocks {
		for _, l := range b.lines {
			if text := l.plain(); strings.Contains(text, "★") || strings.HasPrefix(text, "Relationship") {
				t.Errorf("block %d marks a person: %q", i, text)
			}
			if l.ref >= len(blocks) {
				t.Errorf("block %d refers to block %d", i, l.ref)
			}
		}
	}
	// The contents, a page for each of the 37 people, then the places.
	if len(blocks) <= 39 || blocks[0].part != "Contents" || blocks[1].part != "People" || blocks[38].part != "Places" {
		t.Fatalf("%d blocks; parts %q, %q, %q", len(blocks), blocks[0].part, blocks[1].part, blocks[38].part)
	}
	for i, b := range blocks {
		if b.newPage != (i <= 38) {
			t.Errorf("block %d (%s): newPage = %v", i, b.title, b.newPage)
		}
	}
	// Max Manfred's page links to his father's.
	contents := blocks[0].lines
	if contents[0].plain() != "muster.ged" || !strings.Contains(contents[1].plain(), "All 37 people") ||
		contents[2].plain() != "exported by goged on 12 Oct 2026" {
		t.Errorf("contents = %q / %q / %q", contents[0].plain(), contents[1].plain(), contents[2].plain())
	}
	var max *pdfBlock
	for i := range blocks {
		if blocks[i].title == "Max Manfred Mustermann (1943–)" {
			max = &blocks[i]
		}
	}
	if max == nil {
		t.Fatal("no page for Max Manfred")
	}
	found := false
	for _, l := range max.lines {
		if strings.HasPrefix(l.plain(), "Father") && l.ref > 0 && blocks[l.ref].title == "Otto Mustermann (–aft.1943)" {
			found = true
		}
	}
	if !found {
		t.Error("no link to the father's page")
	}

	// The connected people only: the index lists fewer people.
	sel, _ = export.Select(m.doc, export.Options{Scope: export.Component, Root: m.reference})
	if n := len(m.pdfBlocks(sel, export.Options{})); n >= len(blocks) {
		t.Errorf("component: %d blocks, all: %d", n, len(blocks))
	}
}

func TestWritePDF(t *testing.T) {
	a := newApp(t, Options{})
	sel, _ := export.Select(a.m.doc, export.Options{})
	opts := export.Options{Title: "family.ged", Generated: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}
	var b bytes.Buffer
	if err := a.m.writePDF(&b, sel, opts); err != nil {
		t.Fatal(err)
	}
	pdf := b.String()
	if !strings.HasPrefix(pdf, "%PDF-") || !strings.HasSuffix(strings.TrimSpace(pdf), "%%EOF") {
		t.Fatalf("not a PDF: %.40q … %q", pdf, pdf[max(0, len(pdf)-20):])
	}
	pages, _ := paginate(a.m.pdfBlocks(sel, opts), pdfLines)
	count := func(re string) int { return len(regexp.MustCompile(re).FindAllString(pdf, -1)) }
	if n := count(`/Type /Page\b`); n != len(pages) {
		t.Errorf("%d pages, want %d", n, len(pages))
	}
	// A bookmark per person and place, and the three parts.
	if n := count(`/Title `); n < 24+3 {
		t.Errorf("%d bookmarks", n)
	}
	if n := count(`/Subtype /Link`); n < 24 {
		t.Errorf("%d links", n)
	}
	if n := count(`/FontFile2`); n != 2 {
		t.Errorf("%d embedded fonts, want regular and bold", n)
	}
	if !strings.Contains(pdf, "/CreationDate (D:20261012") {
		t.Error("creation date not set")
	}
}

func TestPDFColor(t *testing.T) {
	for _, tt := range []struct {
		c    lipgloss.TerminalColor
		want [3]int
	}{
		{lipgloss.AdaptiveColor{Light: "#5B3FA6", Dark: "#B9A2FF"}, [3]int{0x5B, 0x3F, 0xA6}},
		{lipgloss.Color("#7A7A7A"), [3]int{0x7A, 0x7A, 0x7A}},
		{lipgloss.Color("12"), [3]int{}},
		{lipgloss.NoColor{}, [3]int{}},
	} {
		if got := pdfColor(tt.c); got != tt.want {
			t.Errorf("pdfColor(%v) = %v, want %v", tt.c, got, tt.want)
		}
	}
}

func TestExportViewPDF(t *testing.T) {
	a := newApp(t, Options{})
	dir := t.TempDir()
	t.Chdir(dir)
	a.press("alt+8", "right")
	a.contains("◂ PDF ▸", "pages to print or read", "family.pdf", "Nobody is marked")
	a.press("down", "down", "down", "enter")
	a.contains("as PDF to family.pdf")
	data, err := os.ReadFile(filepath.Join(dir, "family.pdf"))
	if err != nil || !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("family.pdf: %v %.20q", err, data)
	}
	a.press("up", "up", "up", "right") // the formats wrap around
	a.contains("◂ GEXF ▸", "family.gexf")
}
