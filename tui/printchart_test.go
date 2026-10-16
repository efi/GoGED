package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"codeberg.org/go-pdf/fpdf"
	"github.com/charmbracelet/lipgloss"
)

// mediaBox returns the page size written in a PDF file.
func mediaBox(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "%PDF") {
		t.Fatalf("%s is not a PDF", path)
	}
	return regexp.MustCompile(`/MediaBox \[([^]]*)\]`).FindStringSubmatch(string(data))[1]
}

func TestPrintChart(t *testing.T) {
	dir := t.TempDir()
	gedcom := filepath.Join(dir, "family.ged")
	a := newApp(t, Options{Title: "family.ged", Path: gedcom})
	a.openPerson("I14")
	a.press("t", "P")
	a.contains("Print chart", "Print the pedigree chart of Arthur Smith to a PDF file:", "[ Print ]  [ Cancel ]",
		"type file name  enter print  esc cancel")
	want := filepath.Join(dir, "family-pedigree-I14.pdf")
	if a.m.dlg == nil || a.m.dlg.input.Value() != want {
		t.Fatalf("suggested %v", a.m.dlg)
	}
	a.press("enter")
	if box := mediaBox(t, want); box != "0 0 841.89 595.28" {
		t.Errorf("page %s, want landscape A4", box)
	}
	a.contains("printed the pedigree chart to family-pedigree-I14.pdf")

	// The next suggestion is not taken yet; esc prints nothing.
	a.press("P")
	if v := a.m.dlg.input.Value(); v != filepath.Join(dir, "family-pedigree-I14-2.pdf") {
		t.Errorf("second suggestion %s", v)
	}
	a.press("esc")
	if a.m.dlg != nil || fileExists(filepath.Join(dir, "family-pedigree-I14-2.pdf")) {
		t.Error("esc printed")
	}

	// An existing file is replaced only after asking again.
	if err := os.WriteFile(want, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.press("P", "ctrl+u")
	a.typ(want)
	a.press("enter")
	// The dialog cuts a long path to the width of the screen; the start
	// of the question will do.
	asked := []rune(want + " exists.")
	a.contains(string(asked[:min(40, len(asked))]), "Print replaces it:")
	if data, _ := os.ReadFile(want); string(data) != "old" {
		t.Error("replaced without asking")
	}
	a.press("enter")
	mediaBox(t, want)
	a.contains("printed the pedigree chart")

	// The GEDCOM file is never replaced.
	a.press("P", "ctrl+u")
	a.typ(gedcom)
	a.press("enter")
	a.contains("the chart would replace the GEDCOM file family.ged")
	if fileExists(gedcom) {
		t.Error("the GEDCOM file was written")
	}

	// The lifespans, at their natural width.
	a.press("s", "P", "enter")
	if !fileExists(filepath.Join(dir, "family-lifespans-I14.pdf")) {
		t.Error("the lifespans were not printed")
	}
	a.contains("printed the lifespans chart")
}

// The page is A4, landscape for wide lines, larger as the lines need.
func TestChartPage(t *testing.T) {
	lines := func(n, cols int) []line {
		var ls []line
		for range n {
			ls = append(ls, textLine(span{text: strings.Repeat("x", cols)}))
		}
		return ls
	}
	for _, tt := range []struct {
		n, cols int
		w, h    float64
	}{
		{10, 80, a4Short, a4Long},
		{10, 120, a4Long, a4Short},
		{10, 200, 200*pdfCell + 2*printMargin, a4Short},
		{100, 80, a4Short, 100*pdfPitch + 2*printMargin},
	} {
		if w, h := chartPage(lines(tt.n, tt.cols)); w != tt.w || h != tt.h {
			t.Errorf("%d lines of %d: %.1f × %.1f, want %.1f × %.1f", tt.n, tt.cols, w, h, tt.w, tt.h)
		}
	}
}

// Line drawing, and the symbols the font lacks, are drawn; the rest is
// printed.
func TestDrawnGlyphs(t *testing.T) {
	pdf := fpdf.New("P", "pt", "A4", "")
	pdf.AddPage()
	for _, r := range "─│┌┐└┘├┤┬┴┼━╸╺╍┄┊▸◂▾●◉⚭★≥" {
		if !drawGlyph(pdf, r, 10, 10) {
			t.Errorf("%c is not drawn", r)
		}
	}
	for _, r := range "a+|-…·" {
		if drawGlyph(pdf, r, 10, 10) {
			t.Errorf("%c is drawn", r)
		}
	}
	lines := []line{textLine(span{text: "─┤ Name (1850–) ▸ ⚭ ★ ≥ 5 y", style: lipgloss.NewStyle().Bold(true)})}
	w, h := chartPage(lines)
	var out strings.Builder
	if err := writeChartPDF(&out, lines, w, h, "glyphs"); err != nil || !strings.HasPrefix(out.String(), "%PDF") {
		t.Errorf("PDF: %v", err)
	}
}
