package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestCompareView(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I1")
	a.press("M")
	a.contains("mark a person with m first")
	a.press("m", "M")
	a.contains("is the marked person: select another one")

	// The person shown, compared with the marked one.
	a.openPerson("I3")
	a.press("M")
	if !a.m.showHelp || a.m.overlay != "Compare" {
		t.Fatal("M does not open the comparison")
	}
	a.contains("[Compare]", "Comparison of two people", "11 differences, 1 the same", "Marked        William Smith (1790–1850) ★  I1",
		"Compared      John Smith (1817–1880)  I3", "≠ name", "William Smith", "John Smith", "  sex", "≠ burial",
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
	a.press("M")
	for _, l := range strings.Split(ansi.Strip(a.view()), "\n") {
		if ansi.StringWidth(l) > 60 {
			t.Errorf("line of %d columns: %q", ansi.StringWidth(l), l)
		}
	}
	a.contains("Leeds, Yorkshire,")
	a.press("esc")

	// In the search, alt+M compares the result selected.
	a.resize(100, 30)
	a.press("/", "esc")
	a.typ("id:I14")
	a.press("alt+M")
	if a.m.overlay != "Compare" || a.m.compare.b.ID != "I14" || a.m.compare.a.ID != "I1" {
		t.Errorf("alt+M compares %v with %v", a.m.compare.b, a.m.compare.a)
	}

	// A reload keeps the comparison, with the people of the file loaded now.
	a.m.replaceDoc(loadSample(t))
	a.m.rebuildOverlay()
	if a.m.compare.b != a.m.doc.Individual("I14") {
		t.Error("after a reload, the comparison is of the people loaded before")
	}
	a.contains("Comparison of two people", "Arthur Smith")
}
