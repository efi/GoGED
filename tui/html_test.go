package tui

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/efi/goged/export"
	"github.com/efi/goged/gedcom"
)

func TestWriteHTML(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I20")
	a.press("m") // nobody is marked in the export
	sel, _ := export.Select(a.m.doc, export.Options{})
	var b bytes.Buffer
	if err := a.m.writeHTML(&b, sel, export.Options{Title: "family.ged"}); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	if !strings.HasPrefix(page, "<!DOCTYPE html>\n<html lang=\"en\">") || strings.Contains(page, "★") || strings.Contains(page, "<script") {
		t.Fatalf("page:\n%.300s", page)
	}

	// The page is well-formed; every link leads to a section.
	ids, hrefs, sections, title := map[string]bool{}, []string{}, 0, ""
	dec := xml.NewDecoder(strings.NewReader(page))
	inTitle := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("not well-formed: %v", err)
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			inTitle = tok.Name.Local == "title"
			if tok.Name.Local == "section" {
				sections++
			}
			for _, attr := range tok.Attr {
				switch attr.Name.Local {
				case "id":
					if ids[attr.Value] {
						t.Errorf("duplicate id %q", attr.Value)
					}
					ids[attr.Value] = true
				case "href":
					hrefs = append(hrefs, attr.Value)
				}
			}
		case xml.CharData:
			if inTitle {
				title += string(tok)
			}
		case xml.EndElement:
			inTitle = false
		}
	}
	if title != "family.ged" || sections != len(a.m.bookBlocks(sel, export.Options{})) || !ids["contents"] || !ids["I20"] {
		t.Errorf("title %q, %d sections, ids %v", title, sections, ids)
	}
	for _, h := range hrefs {
		if !strings.HasPrefix(h, "#") || !ids[h[1:]] {
			t.Errorf("link %q leads nowhere", h)
		}
	}
	if len(hrefs) < 24 {
		t.Errorf("%d links", len(hrefs))
	}
	// Columns keep their width; the text after them wraps in the browser.
	if !strings.Contains(page, `<span class="c" style="width:14ch"><span style="color:#7a7a7a">Father</span></span>`) {
		t.Error("no label column")
	}
}

func TestHTMLLine(t *testing.T) {
	grey := lipgloss.NewStyle().Foreground(lipgloss.Color("#7A7A7A"))
	bold := lipgloss.NewStyle().Bold(true)
	for _, tt := range []struct {
		l    line
		ref  string
		want string
	}{
		{textLine(span{text: "Father", style: grey, width: 14}, span{text: "Tom & <Ltd>", style: bold}, span{text: " (1850–)", style: grey}), "I3",
			`<a class="l" href="#I3"><span class="c" style="width:14ch"><span style="color:#7a7a7a">Father</span></span>` +
				`<span class="t"><span style="font-weight:bold">Tom &amp; &lt;Ltd&gt;</span><span style="color:#7a7a7a"> (1850–)</span></span></a>`},
		// Leading spaces indent the text, also where it wraps.
		{textLine(span{text: "    address: Standesamt II"}), "",
			`<div class="l"><span class="t" style="padding-left:4ch">address: Standesamt II</span></div>`},
		// Text between columns keeps its width; control characters are replaced.
		{textLine(span{text: "1850", width: 6}, span{text: " "}, span{text: "Birth", width: 8}, span{text: "Bell\u0007"}), "",
			`<div class="l"><span class="c" style="width:6ch">1850</span><span class="c" style="width:1ch"> </span>` +
				`<span class="c" style="width:8ch">Birth</span><span class="t">` + "Bell\uFFFD</span></div>"},
		{textLine(), "", `<div class="l"></div>`},
	} {
		var b bytes.Buffer
		w := bufio.NewWriter(&b)
		htmlLine(w, tt.l, tt.ref)
		w.Flush()
		if got := strings.TrimSuffix(b.String(), "\n"); got != tt.want {
			t.Errorf("htmlLine =\n%s\nwant\n%s", got, tt.want)
		}
	}
}

func TestExportViewHTML(t *testing.T) {
	a := newApp(t, Options{})
	dir := t.TempDir()
	t.Chdir(dir)
	a.press("alt+9", "right", "right")
	a.contains("◂ HTML ▸", "one web page", "family.html", "Nobody is marked")
	a.press("down", "down", "down", "enter")
	a.contains("as HTML to family.html")
	data, err := os.ReadFile(filepath.Join(dir, "family.html"))
	if err != nil || !bytes.HasPrefix(data, []byte("<!DOCTYPE html>")) {
		t.Fatalf("family.html: %v %.20q", err, data)
	}
	// A GEDCOM file of one person exports as a page with contents and the person.
	doc, err := gedcom.ParseString("0 HEAD\n0 @I1@ INDI\n1 NAME Ann /Alone/\n0 TRLR\n")
	if err != nil {
		t.Fatal(err)
	}
	m := New(doc, Options{})
	sel, _ := export.Select(doc, export.Options{})
	var b bytes.Buffer
	if err := m.writeHTML(&b, sel, export.Options{}); err != nil || strings.Count(b.String(), "<section ") != 2 || !strings.Contains(b.String(), "<title>goged</title>") {
		t.Errorf("one person: %v\n%s", err, b.String())
	}
}
