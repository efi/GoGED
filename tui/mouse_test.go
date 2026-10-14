package tui

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func (a *app) mouse(b tea.MouseButton, action tea.MouseAction, x, y int) tea.Cmd {
	return a.send(tea.MouseMsg{X: x, Y: y, Button: b, Action: action})
}

func (a *app) click(x, y int) tea.Cmd {
	cmd := a.mouse(tea.MouseButtonLeft, tea.MouseActionPress, x, y)
	a.mouse(tea.MouseButtonLeft, tea.MouseActionRelease, x, y)
	return cmd
}

func (a *app) wheel(b tea.MouseButton, x, y int) { a.mouse(b, tea.MouseActionPress, x, y) }

// find returns the screen position of the first occurrence of text.
func (a *app) find(text string) (x, y int) {
	a.t.Helper()
	return a.findBelow(text, 0)
}

// findBelow finds text in screen row top or further down.
func (a *app) findBelow(text string, top int) (x, y int) {
	a.t.Helper()
	for y, l := range strings.Split(ansi.Strip(a.view()), "\n") {
		if i := strings.Index(l, text); i >= 0 && y >= top {
			return ansi.StringWidth(l[:i]), y
		}
	}
	a.t.Fatalf("%q is not shown:\n%s", text, a.view())
	return 0, 0
}

// clickOn clicks the first character of text on the screen.
func (a *app) clickOn(text string) tea.Cmd {
	a.t.Helper()
	return a.click(a.find(text))
}

func TestMouseTabsAndOverlay(t *testing.T) {
	a := newApp(t, Options{})
	a.clickOn("3 Tree")
	if a.m.active != viewSearch {
		t.Error("the tree opened without a person")
	}
	a.contains("select a person first")
	a.clickOn("7 Stats")
	if a.m.active != viewStats {
		t.Fatalf("active view %d", a.m.active)
	}

	// The wheel scrolls the help, a click closes it, and so does its tab.
	a.press("?")
	a.wheel(tea.MouseButtonWheelDown, 10, 10)
	if a.m.help.offset != wheelLines {
		t.Errorf("help offset %d", a.m.help.offset)
	}
	a.click(10, 10)
	if a.m.showHelp {
		t.Error("a click did not close the help")
	}
	a.press("L")
	a.clickOn("[Licenses]")
	if a.m.showHelp {
		t.Error("the overlay's tab did not close it")
	}
	a.press("?")
	a.clickOn("4 Events")
	if a.m.showHelp || a.m.active != viewEvents {
		t.Errorf("help %v, view %d", a.m.showHelp, a.m.active)
	}
	// Nothing happens below the tabs' end or in the footer.
	a.click(99, 1)
	a.click(5, 29)
	if a.m.active != viewEvents {
		t.Errorf("view %d", a.m.active)
	}
}

func TestMouseSearchAndPerson(t *testing.T) {
	a := newApp(t, Options{})
	a.clickOn("Doe, Jane")
	a.contains("▸ Doe, Jane")
	if a.m.active != viewSearch {
		t.Fatal("the first click opened the person")
	}
	a.clickOn("Doe, Jane")
	if a.m.active != viewPerson || a.currentID() != "I13" {
		t.Fatalf("view %d, person %s", a.m.active, a.currentID())
	}

	// A relative is selected first, then visited.
	a.openPerson("I1")
	a.clickOn("John Smith (1817")
	if sel := a.m.person.doc.selected(); sel == nil || sel.target.ID != "I3" || a.currentID() != "I1" {
		t.Fatalf("selected %v, current %s", sel, a.currentID())
	}
	a.clickOn("John Smith (1817")
	if a.currentID() != "I3" {
		t.Fatalf("current %s", a.currentID())
	}
	// Clicking text that links nowhere changes nothing.
	a.clickOn("Parents")
	if a.currentID() != "I3" || a.m.person.doc.cursor != 0 {
		t.Errorf("current %s, cursor %d", a.currentID(), a.m.person.doc.cursor)
	}

	// The mouse's back and forward buttons go through the history.
	a.mouse(tea.MouseButtonBackward, tea.MouseActionPress, 0, 5)
	if a.currentID() != "I1" {
		t.Errorf("back: %s", a.currentID())
	}
	a.mouse(tea.MouseButtonForward, tea.MouseActionPress, 0, 5)
	if a.currentID() != "I3" {
		t.Errorf("forward: %s", a.currentID())
	}

	// The marked person in the title opens them.
	a.press("m")
	a.mouse(tea.MouseButtonBackward, tea.MouseActionPress, 0, 5)
	a.press("1")
	a.clickOn("★ John Smith")
	if a.m.active != viewPerson || a.currentID() != "I3" {
		t.Errorf("view %d, person %s", a.m.active, a.currentID())
	}
}

func TestMouseWheel(t *testing.T) {
	a := newApp(t, Options{})
	a.resize(100, 12)
	h := a.m.searchListHeight()
	a.wheel(tea.MouseButtonWheelDown, 10, 5)
	if s := a.m.search; s.offset != wheelLines || s.cursor != wheelLines {
		t.Errorf("search: offset %d, cursor %d", s.offset, s.cursor)
	}
	a.press("down", "down")
	a.wheel(tea.MouseButtonWheelUp, 10, 5)
	if s := a.m.search; s.offset != 0 || s.cursor != 5 {
		t.Errorf("search: offset %d, cursor %d", s.offset, s.cursor)
	}
	for range 20 {
		a.wheel(tea.MouseButtonWheelDown, 10, 5)
	}
	if s := a.m.search; s.offset != len(s.results)-h || s.cursor != s.offset {
		t.Errorf("search at the end: offset %d, cursor %d of %d", s.offset, s.cursor, len(s.results))
	}

	// The selection stays on screen, on the nearest link.
	a.openPerson("I1")
	d := &a.m.person.doc
	for range 3 {
		a.wheel(tea.MouseButtonWheelDown, 10, 5)
	}
	if d.offset != 3*wheelLines || !d.visible(d.selectedLine(), a.m.bodyHeight()) {
		t.Errorf("person: offset %d, selected line %d", d.offset, d.selectedLine())
	}
	if first := d.links[d.cursor]; first < d.offset || (d.cursor > 0 && d.links[d.cursor-1] >= d.offset) {
		t.Errorf("the selected link is not the first visible one")
	}

	// The tree scrolls both ways.
	a.openPerson("I20")
	a.press("t", "+", "+")
	a.wheel(tea.MouseButtonWheelDown, 10, 5)
	a.wheel(tea.MouseButtonWheelRight, 10, 5)
	if tr := a.m.tree; tr.top == 0 || tr.left != wheelCols {
		t.Errorf("tree: top %d, left %d", tr.top, tr.left)
	}
}

func TestMouseTree(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I20")
	a.press("t")
	// The label in the chart, not the selection in the heading.
	x, y := a.find("─ Arthur Smith")
	a.click(x+2, y)
	tr := a.m.tree
	if n := tr.chart.Nodes[tr.sel]; n.Ind.ID != "I14" || a.m.active != viewTree {
		t.Fatalf("selected %s", n.Ind.ID)
	}
	a.clickOn("─┤") // a line, not a person
	if a.m.tree.sel != tr.sel {
		t.Error("a line was selected")
	}
	a.click(x+2, y)
	if a.m.active != viewPerson || a.currentID() != "I14" {
		t.Errorf("view %d, person %s", a.m.active, a.currentID())
	}
}

func TestMouseEventsAndPlaces(t *testing.T) {
	a := newApp(t, Options{})
	a.press("alt+4")
	a.clickOn("press f to filter")
	if !a.m.ev.editing {
		t.Fatal("the filter is not edited")
	}
	a.typ("type:death")
	a.click(10, 5)
	if a.m.ev.editing || a.m.ev.cursor != 1 {
		t.Errorf("editing %v, cursor %d", a.m.ev.editing, a.m.ev.cursor)
	}

	a.press("alt+5")
	a.clickOn("▾ Canada")
	a.contains("▹ Canada")
	a.notContains("Ontario")
	a.clickOn("▹ Canada")
	a.contains("Ontario")
	a.clickOn("Lancashire")
	a.contains("▸   ▾ Lancashire")
	a.clickOn("Lancashire")
	a.contains("Events in Lancashire, England")
	a.clickOn("esc returns")
	if a.m.places.detail != nil {
		t.Error("the hint did not return to the places")
	}
	a.contains("▸   ▾ Lancashire")
}

func TestMouseMap(t *testing.T) {
	a := newApp(t, Options{})
	a.press("alt+6")
	s := &a.m.mapv
	x, y := a.find("● Köln")
	a.click(x, y)
	if p := s.places[s.sel]; p.Name != "Köln" || a.m.active != viewMap {
		t.Fatalf("selected %s", p.Name)
	}
	a.click(x, y)
	if a.m.active != viewPlaces || a.m.places.detail == nil || a.m.places.detail.Name != "Köln" {
		t.Fatalf("view %d", a.m.active)
	}

	// The wheel zooms around the mouse.
	a.press("alt+6")
	col, row := 30, 10
	x0, y0 := a.m.mapPoint(col, row)
	zoom := s.view.Zoom
	a.wheel(tea.MouseButtonWheelUp, col, row+headerRows+2)
	x1, y1 := a.m.mapPoint(col, row)
	if s.view.Zoom != zoom+zoomStep || math.Abs(x1-x0) > 1e-9 || math.Abs(y1-y0) > 1e-9 {
		t.Errorf("zoom %v → %v, point (%v, %v) → (%v, %v)", zoom, s.view.Zoom, x0, y0, x1, y1)
	}

	// Dragging moves the map with the mouse.
	a.mouse(tea.MouseButtonLeft, tea.MouseActionPress, 40, 12)
	a.mouse(tea.MouseButtonLeft, tea.MouseActionMotion, 44, 14)
	x2, y2 := a.m.mapPoint(col+4, row+2)
	a.mouse(tea.MouseButtonLeft, tea.MouseActionRelease, 44, 14)
	if math.Abs(x2-x1) > 1e-9 || math.Abs(y2-y1) > 1e-9 {
		t.Errorf("the map did not follow the mouse: (%v, %v) is at (%v, %v)", x1, y1, x2, y2)
	}
	a.mouse(tea.MouseButtonLeft, tea.MouseActionMotion, 60, 20)
	if x3, _ := a.m.mapPoint(col+4, row+2); x3 != x2 {
		t.Error("the map moved after the button was released")
	}

	// A drag that starts on the selected place moves the map, and does not
	// open the place.
	a.press("c")
	x, y = a.findBelow("◉ Köln", headerRows+2) // on the map, not above it
	x1, _ = a.m.mapPoint(col, row)
	a.mouse(tea.MouseButtonLeft, tea.MouseActionPress, x, y)
	a.mouse(tea.MouseButtonLeft, tea.MouseActionMotion, x+3, y)
	a.mouse(tea.MouseButtonLeft, tea.MouseActionRelease, x+3, y)
	if a.m.active != viewMap {
		t.Errorf("view %d", a.m.active)
	}
	if x4, _ := a.m.mapPoint(col+3, row); math.Abs(x4-x1) > 1e-9 {
		t.Error("the map did not move with the mouse")
	}
}

func TestMouseChecks(t *testing.T) {
	a := newMusterApp(t)
	a.press("alt+8")
	a.clickOn("Wilhelm")
	if f := a.m.finding(); f == nil || !strings.Contains(f.Person.DisplayName(), "Wilhelm") {
		t.Fatalf("finding %v", f)
	}
	a.clickOn("Wilhelm")
	if a.m.active != viewPerson || !strings.Contains(a.m.current().DisplayName(), "Wilhelm") {
		t.Fatalf("view %d", a.m.active)
	}
	a.press("alt+8")
	a.clickOn("Lived longer than")
	a.clickOn("Lived longer than")
	if i := a.m.checks.rows[a.m.checks.cursor].check; !a.m.checks.folded[i] {
		t.Error("a second click did not fold the check")
	}
}

func TestMouseExport(t *testing.T) {
	a := newApp(t, Options{})
	dir := t.TempDir()
	t.Chdir(dir)
	a.press("alt+9")
	x, y := a.find("◂ GEXF ▸")
	a.click(x+ansi.StringWidth("◂ GEXF ▸")-1, y)
	a.contains("◂ PDF ▸", "family.pdf")
	x, y = a.find("◂ PDF ▸")
	a.click(x, y)
	a.contains("◂ GEXF ▸", "family.gexf")

	a.clickOn("File")
	if a.m.export.row != exportRowFile || a.m.export.editing {
		t.Fatalf("row %d, editing %v", a.m.export.row, a.m.export.editing)
	}
	a.clickOn("File")
	if !a.m.export.editing {
		t.Fatal("the file name is not edited")
	}
	a.press("backspace", "backspace", "backspace", "backspace")
	a.typ("xml")
	a.clickOn("[ Export ]")
	if a.m.export.editing || a.m.export.row != exportRowExport {
		t.Fatalf("row %d, editing %v", a.m.export.row, a.m.export.editing)
	}
	a.clickOn("[ Export ]")
	if _, err := os.Stat(filepath.Join(dir, "family.xml")); err != nil {
		t.Fatal(err)
	}
	// An existing file is replaced after a second click only.
	a.clickOn("[ Export ]")
	a.contains("exists")
	a.clickOn("Format")
	a.clickOn("[ Export ]")
	a.clickOn("[ Export ]")
	a.contains("exists")
}
