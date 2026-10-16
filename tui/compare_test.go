package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
)

func TestCompareView(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I1")
	a.press("C")
	a.contains("mark a person with m first")
	a.press("m", "C")
	a.contains("is the marked person: select another one")

	// The person shown, compared with the marked one.
	a.openPerson("I3")
	a.press("C")
	if !a.m.showHelp || a.m.overlay != "Compare" {
		t.Fatal("M does not open the comparison")
	}
	a.contains("[Compare]", "Comparison of two people", "11 differences, 1 the same", "Marked        William Smith (1790–1850) ★  I1",
		"Selected      John Smith (1817–1880)  I3", "≠ name", "William Smith", "John Smith", "  sex", "≠ burial",
		"↑↓ select  enter open  any other key close")
	for _, l := range strings.Split(ansi.Strip(a.view()), "\n") {
		if ansi.StringWidth(l) > 100 {
			t.Errorf("line of %d columns: %q", ansi.StringWidth(l), l)
		}
	}
	// Enter opens the person selected, the marked one first.
	a.press("down", "enter")
	if a.m.showHelp || a.currentID() != "I3" {
		t.Errorf("enter: overlay %v, person %s", a.m.showHelp, a.currentID())
	}

	// Narrow, the rows wrap within their columns.
	a.resize(60, 40)
	a.press("C")
	for _, l := range strings.Split(ansi.Strip(a.view()), "\n") {
		if ansi.StringWidth(l) > 60 {
			t.Errorf("line of %d columns: %q", ansi.StringWidth(l), l)
		}
	}
	a.contains("Leeds, Yorkshire,")
	a.press("esc")

	// In the search, alt+C compares the result selected.
	a.resize(100, 30)
	a.press("/", "esc")
	a.typ("id:I14")
	a.press("alt+C")
	if a.m.overlay != "Compare" || a.m.compare.b.ID != "I14" || a.m.compare.a.ID != "I1" {
		t.Errorf("alt+C compares %v with %v", a.m.compare.b, a.m.compare.a)
	}

	// A reload keeps the comparison, with the people of the file loaded now.
	a.m.replaceDoc(loadSample(t))
	a.m.rebuildOverlay()
	if a.m.compare.b != a.m.doc.Individual("I14") {
		t.Error("after a reload, the comparison is of the people loaded before")
	}
	a.contains("Comparison of two people", "Arthur Smith")
}

// In the checks, C compares a possible duplicate with the other record,
// and a group of places that may be the same in a table.
func TestCompareInChecks(t *testing.T) {
	doc, err := gedcom.ParseString(`0 HEAD
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Johann /Müller/
1 SEX M
1 BIRT
2 DATE 1850
2 PLAC Köln, Deutschland
0 @I2@ INDI
1 NAME Johan /Mueller/
1 SEX M
1 BIRT
2 DATE 1851
2 PLAC Koeln, Deutschland
0 @I3@ INDI
1 NAME Anna /Schmidt/
1 BIRT
2 DATE 1860
2 PLAC Cöln, Deutschland
3 MAP
4 LATI N50.94
4 LONG E6.96
1 DEAT
2 DATE 1930
2 PLAC Köln, Deutschland
0 TRLR
`)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{t: t, m: New(doc, Options{Title: "x.ged"})}
	a.resize(100, 30)
	a.press("alt+8")
	findingOf := func(id string) {
		t.Helper()
		for r, cr := range a.m.checks.rows {
			if genealogy.Checks[cr.check].ID == id && cr.finding >= 0 {
				a.m.checks.cursor = r
				return
			}
		}
		t.Fatalf("no finding of %s", id)
	}

	findingOf("duplicates")
	a.press("C")
	if a.m.overlay != "Compare" || a.m.compare.a.ID != "I1" || a.m.compare.b.ID != "I2" {
		t.Fatalf("overlay %q, comparison %+v", a.m.overlay, a.m.compare)
	}
	a.contains("One record    Johann Müller (1850–)  I1", "Other record  Johan Mueller (1851–)  I2", "≠ name",
		"≠ birth  1850, Köln, Deutschland")
	a.press("esc")

	findingOf("similar-places")
	a.press("C")
	a.contains("Places that may be the same", "similar names: Cöln · Koeln · Köln (in Deutschland)",
		"place  events  people  years      coordinates     from Cöln  record  GOV",
		"Cöln   1       1       1860       50.94°N 6.96°E  0 km       –       –",
		"Köln   2       2       1850–1930  –               –          –       –")
	a.press("enter")
	if a.m.showHelp || a.m.active != viewPlaces || a.m.selectedPlace().Full != "Cöln, Deutschland" {
		t.Errorf("enter: overlay %v, view %d, place %v", a.m.showHelp, a.m.active, a.m.selectedPlace())
	}

	// Other findings compare their person with the marked one.
	a.press("alt+8")
	findingOf("no-death")
	a.press("C")
	a.contains("mark a person with m first")
}
