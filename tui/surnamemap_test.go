package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/worldmap"
)

// surnameApp opens family.ged on the map's surname view.
func surnameApp(t *testing.T) *app {
	t.Helper()
	a := newApp(t, Options{})
	a.resize(110, 34)
	a.press("alt+6", "s")
	if !a.m.mapv.sn.on {
		t.Fatal("the surname view is off")
	}
	return a
}

// shown returns the surname shown.
func (a *app) shown() string { sn := a.m.mapv.sn; return sn.names[sn.name].name }

func TestSurnameMap(t *testing.T) {
	a := surnameApp(t)
	s := &a.m.mapv
	// The most frequent birth surname comes first, its busiest place
	// selected.
	a.contains("Map · surname Smith · 10 people born with it · all events at 5 places · e events · s all places",
		"◉ Leeds, Yorkshire, England · Smith: 5 events · 3 people · 1790–1850",
		"▸ Smith ", "Jones", "3 places",
		"Smith in Leeds", "William Smith 1790–1850", "Smith per ", "s all places")
	if a.shown() != "Smith" || s.places[s.sel].Name != "Leeds" || a.m.mapCols() != 110-surnamePanel {
		t.Fatalf("surname %s, place %s, map %d columns", a.shown(), s.places[s.sel].Name, a.m.mapCols())
	}
	// The other places are faint; the surname's are labelled with counts.
	faint, labelled := 0, 0
	for _, mk := range a.m.surnameMarkers() {
		switch {
		case mk.Faint:
			faint++
		case mk.Label == "Leeds 5" || mk.Label == "Manchester 5":
			labelled++
		}
	}
	if faint == 0 || labelled != 2 {
		t.Errorf("%d faint markers, %d labelled", faint, labelled)
	}
	for _, l := range strings.Split(a.view(), "\n") {
		if w := len([]rune(stripANSI(l))); w > 110 {
			t.Errorf("line of %d columns: %q", w, stripANSI(l))
		}
	}

	// n and N step through the surname's places only.
	start := s.sel
	for range len(s.places) + 1 {
		a.press("n")
		if len(s.sn.events[s.sel]) == 0 {
			t.Fatalf("n selected %s, which has no events of Smith", s.places[s.sel].Full)
		}
	}
	before := s.sel
	a.press("N", "n")
	if s.sel != before || start < 0 {
		t.Errorf("N and n do not undo each other: %d, %d", before, s.sel)
	}

	// PgDn and PgUp change the surname, as does a click in the list.
	a.press("pgdown")
	a.contains("surname Jones · 5 people born with it")
	a.press("pgup")
	if a.shown() != "Smith" {
		t.Errorf("pgup: %s", a.shown())
	}
	a.clickOn("Jones ")
	if a.shown() != "Jones" {
		t.Errorf("click: %s", a.shown())
	}

	// e cycles through the kinds of events.
	for _, want := range []string{"births", "deaths", "other events", "all events"} {
		a.press("e")
		a.contains(want + " of Jones: ")
		if eventKindNames[s.sn.kind] != want {
			t.Errorf("kind %s, want %s", eventKindNames[s.sn.kind], want)
		}
	}
	a.press("e")
	for _, e := range s.sn.all {
		if kindOf(e) != eventsBirth {
			t.Errorf("a %s among the births", e.Tag)
		}
	}
	a.press("e", "e", "e")

	// s returns to all places.
	a.press("s")
	a.contains("all places · s shows the places of a surname")
	a.notContains("Surnames · PgUp PgDn")
	if a.m.mapCols() != 110 || s.sn.on {
		t.Error("the surname view is still on")
	}
	a.press("s")
	if a.shown() != "Jones" {
		t.Errorf("s again shows %s", a.shown())
	}

	// A narrow screen has no room for the panel.
	a.resize(60, 30)
	if a.m.mapCols() != 60 {
		t.Errorf("map of %d columns on 60", a.m.mapCols())
	}
	a.notContains("Surnames · PgUp PgDn")
}

func TestSurnameDialog(t *testing.T) {
	a := newApp(t, Options{})
	a.resize(110, 40)
	a.press("alt+7", "home", "down", "enter")
	a.contains("What would you like to see of Smith?", "[ People with that surname ]  [ Surname on the map ]", "m map")
	a.press("esc")
	if a.m.dlg != nil || a.m.active != viewStats {
		t.Fatal("esc does not close the dialog")
	}
	a.press("enter", "m")
	if a.m.active != viewMap || !a.m.mapv.sn.on || a.shown() != "Smith" {
		t.Fatalf("view %d, surname view %v", a.m.active, a.m.mapv.sn.on)
	}
	// By mouse, on the second button.
	a.press("alt+7", "down")
	a.press("enter")
	x, y := a.find("[ Surname on the map ]")
	a.click(x+3, y)
	if a.m.active != viewMap || a.shown() != "Jones" {
		t.Errorf("view %d, surname %s", a.m.active, a.shown())
	}
	// Married names are not birth surnames.
	a.m.showSurnameOnMap("Nobody")
	a.contains("nobody was born with the surname Nobody")
}

// A click in the list of surnames chooses the surname clicked, also when
// the list has scrolled to the one shown, and after a reload.
func TestSurnameListScrolls(t *testing.T) {
	var b strings.Builder
	b.WriteString("0 HEAD\n1 CHAR UTF-8\n")
	// Twelve surnames, the earlier ones more frequent, each in a place of
	// its own.
	for i := range 12 {
		for k := range 13 - i {
			fmt.Fprintf(&b, "0 @I%d_%d@ INDI\n1 NAME X /Name%02d/\n1 BIRT\n2 PLAC Town%02d\n3 MAP\n4 LATI N%d\n4 LONG E%d\n",
				i, k, i, i, 40+i, i)
		}
	}
	b.WriteString("0 TRLR\n")
	doc, err := gedcom.ParseString(b.String())
	if err != nil {
		t.Fatal(err)
	}
	a := &app{t: t, m: New(doc, Options{Title: "names.ged"})}
	a.resize(110, 34)
	a.press("alt+6", "s")
	for range 10 {
		a.press("pgdown")
	}
	a.contains("▸ Name10 ", "Name03 ")
	a.notContains("Name02 ")
	a.clickOn("Name03 ")
	if a.shown() != "Name03" {
		t.Errorf("a click on Name03 shows %s", a.shown())
	}

	// A reload keeps the list where it is.
	a.press("pgdown", "pgdown", "pgdown", "pgdown", "pgdown", "pgdown", "pgdown", "pgdown")
	a.m.replaceDoc(NewShared(doc))
	a.contains("▸ Name11 ", "Name04 ")
	a.clickOn("Name04 ")
	if a.shown() != "Name04" {
		t.Errorf("after the reload, a click on Name04 shows %s", a.shown())
	}
}

func TestSurnameMapReload(t *testing.T) {
	a, path := watchedApp(t)
	a.resize(110, 34)
	a.press("alt+6", "s", "pgdown", "e", "v")
	rewrite(t, path, `1 NAME John /Smith/`, `1 NAME Johann /Smith/`)
	a.poll(path)
	a.poll(path)
	a.answerable()
	a.finish(a.press("y"))
	if sn := a.m.mapv.sn; !sn.on || a.shown() != "Jones" || sn.kind != eventsBirth {
		t.Errorf("after the reload: on %v, %s, %s", sn.on, a.shown(), eventKindNames[sn.kind])
	}
	// The projection stays, and the markers are in it.
	s := &a.m.mapv
	if x, y := worldmap.EqualEarth.Project(s.places[0].Lat, s.places[0].Lon); s.view.Proj != worldmap.EqualEarth || s.markers[0].X != x || s.markers[0].Y != y {
		t.Errorf("after the reload: %s, marker %+v", s.view.Proj, s.markers[0])
	}
}

// stripANSI removes escape sequences.
func stripANSI(s string) string {
	var b strings.Builder
	esc := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			esc = true
		case esc && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'):
			esc = false
		case !esc:
			b.WriteRune(r)
		}
	}
	return b.String()
}
