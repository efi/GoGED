package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/efi/goged/worldmap"
)

// M shows the places of a person's events on the map, like the surname
// view: their places marked, a route joining them in date order, a panel
// with their places and the events at the one selected.
func TestPersonMap(t *testing.T) {
	a := newApp(t, Options{})
	a.resize(120, 34)
	a.openPerson("I3")
	a.press("M")
	s := &a.m.mapv
	if a.m.active != viewMap || !s.pm.on || s.pm.ind.ID != "I3" {
		t.Fatalf("view %d, person view %v", a.m.active, s.pm.on)
	}
	a.contains("Map · places of John Smith · 12 events at 4 places · c relatives' events shown · l route · esc all places",
		"◉ Leeds, Yorkshire, England · 4 events · 1817–1860", "John Smith", "1817–1880 · 4 places · n N",
		"▸ 1817     Leeds", "1840     Manchester", "Manchester 6", "2 Feb 1817 Birth",
		"n/N places  c relatives  l route")
	if a.m.mapCols() != 120-surnamePanel {
		t.Errorf("map of %d columns", a.m.mapCols())
	}
	// The route joins the places of his own events: born in Leeds,
	// christened at St Peter's, then Manchester. Salford is a relative's.
	var route []string
	for _, i := range s.pm.route {
		route = append(route, s.places[i].Name)
	}
	if strings.Join(route, " ") != "Leeds St Peter's Manchester" {
		t.Errorf("route %v", route)
	}
	routeCells := func() (n int) {
		f := s.world.RenderRoute(s.view, a.m.mapCols(), a.m.mapRows(), a.m.personMarkers(), s.sel, a.m.personRoute())
		for _, row := range f.Cells {
			for _, c := range row {
				if c.Class == worldmap.ClassRoute {
					n++
				}
			}
		}
		return n
	}
	if routeCells() == 0 {
		t.Error("no route drawn")
	}
	a.press("l")
	if routeCells() != 0 || !s.pm.noRoute {
		t.Error("l does not hide the route")
	}
	a.contains("the route is hidden")
	a.press("l")

	// n and N step through the places in the order of the first event.
	a.press("n")
	if s.places[s.sel].Name != "St Peter's" {
		t.Errorf("n: %s", s.places[s.sel].Full)
	}
	a.press("N", "N")
	if s.places[s.sel].Name != "Salford" {
		t.Errorf("N from the first: %s", s.places[s.sel].Full)
	}

	// c leaves out the relatives' events, here and in the person view.
	a.press("c")
	if !a.m.person.hideRelatives || len(s.pm.stops) != 3 {
		t.Errorf("c: hide %v, %d places", a.m.person.hideRelatives, len(s.pm.stops))
	}
	a.contains("5 events at 3 places · c relatives' events left out")
	if slices.Contains(s.pm.stops, s.sel) == false {
		t.Error("the selection is not one of the places")
	}
	a.press("c")

	// A click in the panel selects a place.
	a.clickOn("Manchester    ")
	if s.places[s.sel].Name != "Manchester" {
		t.Errorf("click: %s", s.places[s.sel].Full)
	}

	// A reload keeps the person view, with the person loaded now.
	a.m.replaceDoc(loadSample(t))
	if s := &a.m.mapv; !s.pm.on || s.pm.ind != a.m.doc.Individual("I3") || len(s.pm.stops) != 4 {
		t.Errorf("after a reload: %v", s.pm.on)
	}

	// esc and M return to all places; s goes to the surnames.
	a.press("esc")
	if a.m.mapv.pm.on {
		t.Error("esc leaves the person view on")
	}
	a.press("M")
	if !a.m.mapv.pm.on || a.m.mapv.pm.ind.ID != "I3" {
		t.Error("M on the map shows the current person")
	}
	a.press("M")
	if a.m.mapv.pm.on {
		t.Error("M again leaves the person view on")
	}
	a.press("M", "s")
	if a.m.mapv.pm.on || !a.m.mapv.sn.on {
		t.Error("s does not switch to the surnames")
	}
}

// M shows the map of the person selected: a search result, a person in
// a chart, a finding.
func TestPersonMapFromViews(t *testing.T) {
	a := newApp(t, Options{})
	a.resize(120, 34)
	a.typ("id:I14")
	a.press("alt+M")
	if a.m.active != viewMap || !a.m.mapv.pm.on || a.m.mapv.pm.ind.ID != "I14" {
		t.Errorf("alt+M in the search: view %d, %v", a.m.active, a.m.mapv.pm.ind)
	}
	a.openPerson("I3")
	a.press("t", "right", "M") // John's father
	if a.m.mapv.pm.ind.ID != "I1" {
		t.Errorf("M in a chart shows %s", a.m.mapv.pm.ind.ID)
	}
	// Infant Smith's events have no places.
	a.openPerson("I24")
	a.press("M")
	if a.m.active != viewMap || len(a.m.mapv.pm.stops) != 0 {
		t.Fatalf("view %d, %d places", a.m.active, len(a.m.mapv.pm.stops))
	}
	a.contains("no event of Infant Smith took place where coordinates are known")
	// M on a place leaves the person view.
	a.press("alt+5", "M")
	if a.m.mapv.pm.on {
		t.Error("M on a place keeps the person view")
	}
}
