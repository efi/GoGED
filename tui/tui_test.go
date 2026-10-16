package tui

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/chart"
	"github.com/efi/goged/export"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
	"github.com/efi/goged/worldmap"
	"github.com/muesli/termenv"
)

func TestMain(m *testing.M) {
	// Render without colors so views can be compared as plain text.
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

func loadSample(t testing.TB) *gedcom.Document {
	t.Helper()
	doc, err := gedcom.ParseFile(filepath.Join("..", "testdata", "family.ged"))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// app wraps the model for convenient scripting in tests.
type app struct {
	t *testing.T
	m Model
}

func newApp(t *testing.T, opts Options) *app {
	t.Helper()
	if opts.Title == "" {
		opts.Title = "family.ged"
	}
	a := &app{t: t, m: New(loadSample(t), opts)}
	a.resize(100, 30)
	return a
}

// newMusterApp opens the GEDCOM-L sample file.
func newMusterApp(t *testing.T) *app {
	t.Helper()
	doc, err := gedcom.ParseFile(filepath.Join("..", "testdata", "Muster_GEDCOM_UTF-8.ged"))
	if err != nil {
		t.Fatal(err)
	}
	a := &app{t: t, m: New(doc, Options{Title: "Muster_GEDCOM_UTF-8.ged"})}
	a.resize(110, 60)
	return a
}

func (a *app) send(msg tea.Msg) tea.Cmd {
	model, cmd := a.m.Update(msg)
	a.m = model.(Model)
	return cmd
}

func (a *app) resize(w, h int) { a.send(tea.WindowSizeMsg{Width: w, Height: h}) }

var keyTypes = map[string]tea.KeyType{
	"enter": tea.KeyEnter, "esc": tea.KeyEsc, "up": tea.KeyUp, "down": tea.KeyDown,
	"left": tea.KeyLeft, "right": tea.KeyRight, "tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab,
	"backspace": tea.KeyBackspace, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
	"home": tea.KeyHome, "end": tea.KeyEnd, "ctrl+c": tea.KeyCtrlC, "space": tea.KeySpace,
	"ctrl+u": tea.KeyCtrlU, "ctrl+d": tea.KeyCtrlD, "f1": tea.KeyF1,
}

func keyMsg(k string) tea.KeyMsg {
	if t, ok := keyTypes[k]; ok {
		return tea.KeyMsg{Type: t}
	}
	if rest, ok := strings.CutPrefix(k, "alt+"); ok {
		if t, ok := keyTypes[rest]; ok {
			return tea.KeyMsg{Type: t, Alt: true}
		}
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(rest), Alt: true}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// press sends named keys ("enter", "down") or single characters.
func (a *app) press(keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		cmd = a.send(keyMsg(k))
	}
	return cmd
}

// typ types text one character at a time.
func (a *app) typ(text string) {
	for _, r := range text {
		a.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func (a *app) view() string { return a.m.View() }

func (a *app) contains(want ...string) {
	a.t.Helper()
	v := a.view()
	for _, w := range want {
		if !strings.Contains(v, w) {
			a.t.Errorf("view does not contain %q:\n%s", w, v)
		}
	}
}

func (a *app) notContains(unwanted ...string) {
	a.t.Helper()
	v := a.view()
	for _, w := range unwanted {
		if strings.Contains(v, w) {
			a.t.Errorf("view unexpectedly contains %q:\n%s", w, v)
		}
	}
}

func (a *app) currentID() string {
	if c := a.m.current(); c != nil {
		return c.ID
	}
	return ""
}

func (a *app) openPerson(id string) {
	a.t.Helper()
	a.press("/")
	a.m.setQuery("id:" + id)
	a.press("enter")
	if a.currentID() != id {
		a.t.Fatalf("could not open %s (current %q)", id, a.currentID())
	}
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestStartsInSearch(t *testing.T) {
	a := newApp(t, Options{})
	if a.m.active != viewSearch {
		t.Fatalf("active view = %v", a.m.active)
	}
	a.contains("goged", "family.ged", "24 people · 9 families", "[1 Search]", "24 people — type to search", "▸ Brown, Elizabeth", "Smith, William")
	if cmd := a.m.Init(); cmd == nil {
		t.Error("Init should set the window title")
	}
}

func TestSearchAndOpen(t *testing.T) {
	a := newApp(t, Options{})
	a.typ("john")
	a.contains("Search: john", "1 match", "▸ Smith, John")
	a.press("enter")
	if a.m.active != viewPerson || a.currentID() != "I3" {
		t.Fatalf("active=%v current=%s", a.m.active, a.currentID())
	}
	a.contains("[2 Person]", "John Smith", "male · I3 · 1817–1880", "Parents", "▸ Father", "William Smith (1790–1850)",
		"Mother", "Siblings", "Sister", "Family with Ann Taylor", "married 1840, Manchester", "Son", "Thomas Smith (1842–1910)",
		"Family with Sarah White", "Events", "Birth of son Thomas Smith")
	// The remaining content is further down.
	a.press("end")
	a.contains("Notes", "cotton mills", "He married twice.", "Sources", "1851 Census of England and Wales — HO107/2229 folio 316")
}

func TestSearchShowsMatchedAlternateName(t *testing.T) {
	a := newApp(t, Options{})
	a.typ("smith")
	a.contains("11 matches", "Doe, Jane (Jane Smith)")
}

func TestSearchViewAlignsDecomposedNames(t *testing.T) {
	// The same name, precomposed (NFC) and decomposed (NFD): the
	// combining mark takes no column, so the columns stay aligned.
	doc, err := gedcom.ParseString(strings.Join([]string{
		"0 HEAD",
		"0 @I1@ INDI", "1 NAME Hans /M\u00FCller/", "1 BIRT", "2 DATE 1900", "2 PLAC Berlin",
		"0 @I2@ INDI", "1 NAME Hanna /Mu\u0308ller/", "1 BIRT", "2 DATE 1901", "2 PLAC Berlin",
		"0 TRLR", "",
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	a := &app{t: t, m: New(doc, Options{})}
	a.resize(100, 30)
	a.typ("mu\u0308ller")
	a.contains("2 matches")
	cols := map[string][]int{}
	for _, line := range strings.Split(ansi.Strip(a.view()), "\n") {
		for _, word := range []string{"1900", "1901", "Berlin"} {
			if i := strings.Index(line, word); i >= 0 {
				cols[word] = append(cols[word], ansi.StringWidth(line[:i]))
			}
		}
	}
	if len(cols["1900"]) != 1 || len(cols["1901"]) != 1 || cols["1900"][0] != cols["1901"][0] ||
		len(cols["Berlin"]) != 2 || cols["Berlin"][0] != cols["Berlin"][1] {
		t.Errorf("columns differ: %v\n%s", cols, a.view())
	}
}

func TestSearchErrorsAndNavigation(t *testing.T) {
	a := newApp(t, Options{})
	a.typ("foo:bar")
	a.contains(`unknown field "foo"`)
	a.press("enter")
	if a.m.active != viewSearch {
		t.Error("enter without results must not leave search")
	}
	a.press("esc")
	if a.m.search.input.Value() != "" {
		t.Error("esc should clear the query")
	}
	a.press("down", "down", "down")
	if a.m.search.cursor != 3 {
		t.Errorf("cursor = %d", a.m.search.cursor)
	}
	a.press("pgdown")
	if a.m.search.cursor != 23 {
		t.Errorf("cursor after pgdown = %d", a.m.search.cursor)
	}
	a.press("pgup", "up")
	if a.m.search.cursor != 0 {
		t.Errorf("cursor after pgup = %d", a.m.search.cursor)
	}
	a.resize(100, 10)
	a.press("pgdown", "pgdown")
	if a.m.search.offset == 0 || a.m.search.cursor < a.m.search.offset {
		t.Errorf("list should scroll: cursor %d offset %d", a.m.search.cursor, a.m.search.offset)
	}
	a.contains("▸ ")
	// '?' is typed when a query is in progress.
	a.typ("x?")
	if a.m.showHelp || a.m.search.input.Value() != "x?" {
		t.Errorf("'?' should be typed: help=%v value=%q", a.m.showHelp, a.m.search.input.Value())
	}
}

func TestResizeKeepsSelectionVisible(t *testing.T) {
	a := newApp(t, Options{})
	a.press("pgdown", "pgdown")
	if a.m.search.cursor != 23 {
		t.Fatalf("cursor = %d", a.m.search.cursor)
	}
	a.resize(100, 12)
	if s := a.m.search; s.cursor < s.offset || s.cursor >= s.offset+a.m.searchListHeight() {
		t.Errorf("selection off screen after resize: cursor %d offset %d", s.cursor, s.offset)
	}
	a.contains("▸ White, Sarah")
}

func TestEscReturnsToPreviousView(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I3")
	a.press("/")
	if a.m.active != viewSearch {
		t.Fatal("/ should open search")
	}
	a.press("esc") // query "id:I3" is cleared first
	a.press("esc")
	if a.m.active != viewPerson {
		t.Errorf("esc should return to the person view, got %v", a.m.active)
	}
}

func TestPersonNavigationAndHistory(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I3")
	a.press("enter") // Father
	if a.currentID() != "I1" {
		t.Fatalf("current = %s", a.currentID())
	}
	a.press("b")
	if a.currentID() != "I3" {
		t.Errorf("back -> %s", a.currentID())
	}
	a.press("]")
	if a.currentID() != "I1" {
		t.Errorf("forward -> %s", a.currentID())
	}
	a.press("left")
	if a.currentID() != "I3" {
		t.Errorf("left -> %s", a.currentID())
	}
	a.press("right")
	if a.currentID() != "I1" {
		t.Errorf("right -> %s", a.currentID())
	}
	a.press("right")
	a.contains("no later person in history")
	a.press("b", "b")
	a.contains("no earlier person in history")

	// Visiting someone new drops the forward history.
	a.press("down", "down", "down", "enter") // John -> Ann Taylor (wife)
	if a.currentID() != "I5" {
		t.Fatalf("expected Ann Taylor, got %s", a.currentID())
	}
	a.press("]")
	if a.currentID() != "I5" {
		t.Error("forward history should have been truncated")
	}
}

func TestPersonLinks(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I3")
	var targets []string
	for _, i := range a.m.person.doc.links {
		targets = append(targets, a.m.person.doc.lines[i].target.ID)
	}
	want := "I1,I2,I4,I5,I7,I6,I8,I9,I4,I7,I5,I8,I9,I1,I9,I2,I7,I14,I8,I16"
	if got := strings.Join(targets, ","); got != want {
		t.Errorf("links = %s\nwant    %s", got, want)
	}
}

func TestPersonViewAdoption(t *testing.T) {
	a := newMusterApp(t)
	a.openPerson("I6")
	a.contains("Father        Wilhelm Schüchter (1936–1964)\n", "Mother        Mathilde Mustermann (1939–1970)\n",
		"Father        Gerold Freiwein (1938–)  (adopted)", "Mother        Brigitte Marquardt (1942–)  (adopted)")
	if n := strings.Count(a.view(), "Mother        Mathilde Mustermann"); n != 1 {
		t.Errorf("Mathilde is listed %d times:\n%s", n, a.view())
	}
	a.openPerson("I17")
	a.contains("Son           Markus Schüchter (1963–)\n")
	a.notContains("(adopted)")
	a.openPerson("I19")
	a.contains("Son           Markus Schüchter (1963–)  (adopted)")
}

func TestRestrictedNotice(t *testing.T) {
	doc, err := gedcom.ParseFile(filepath.Join("..", "testdata", "Muster_GEDCOM_UTF-8.ged"))
	if err != nil {
		t.Fatal(err)
	}
	a := &app{t: t, m: New(doc.Redacted(), Options{Title: "Muster_GEDCOM_UTF-8.ged"})}
	a.resize(110, 30)
	a.contains("37 people · 18 families · 2 restricted hidden")
	a.openPerson("I22")
	a.notContains("1 May 1933")
	a = newMusterApp(t)
	a.notContains("restricted")
	a.openPerson("I22")
	a.contains("1 May 1933          Birth  Wenningen", "confidential")
}

func TestPersonViewAssociations(t *testing.T) {
	a := newMusterApp(t)
	a.resize(130, 150)
	a.openPerson("I35")
	a.contains("Associations", "Same person?  Herbert Mustermann (1905–)  (alias record)",
		"Godparent of  Karl Müller Junior (1933–1980)", "Record last changed 29 Sep 2012 20:46:42")
	a.openPerson("I1")
	a.contains("male · I1 · 1943– · ref. 1\n", "call name Max\n",
		"Witness       Otto Mustermann (–aft.1943)  (at civil marriage 12 Sep 1968)",
		"witnesses: Otto Mustermann, Franz-Xaver Gabler", "address: Kölner Dom",
		"GenWiki: GEDCOM Kennzeichen FAM — Vereinbarung F4", "“F4 Standesamtliche und kirchliche Trauung")
	a.openPerson("I4")
	a.contains("Witness of    Max Manfred Mustermann & Erika Gabler  (at civil marriage 12 Sep 1968)")
	a.openPerson("I32")
	a.contains("Associate of  Hans Mustermann (1942–)  (langjähriger Weggefährte)",
		"              Dave Sample spielte von 1971 bis 1999 in der Elton John Band")
	a.openPerson("I37")
	a.contains("religion: evangelisch", "godparents: Max Herbert Mustermann, Musiker")
	// Associations are links.
	a.openPerson("I22")
	a.contains("Godparent     Max Herbert Mustermann\n")
	for i := 0; i < 10 && !strings.Contains(a.view(), "▸ Godparent"); i++ {
		a.press("down")
	}
	a.press("enter")
	if a.currentID() != "I35" {
		t.Errorf("enter on the godparent opened %s", a.currentID())
	}
}

func TestTimelineWording(t *testing.T) {
	a := newMusterApp(t)
	a.resize(150, 60)
	a.openPerson("I11")
	a.contains("29 Sep 2002         Divorce from Brigitte Gutedel filed",
		"Jul 1997            Pilgerreise nach Rom with Brigitte Gutedel")
	a.openPerson("I1")
	a.contains("Apr 1958–Mar 1961   Education  Werkzeugschlosser Lehre",
		"bet. 1961–1964      Education  Studium Maschinenbau Ingenieur · between Apr 1961 and Mar 1964",
		"12 Sep 1968         Civil marriage with Erika Gabler")
	a.openPerson("I6")
	a.contains("7 May 1967          Adoption  by Gerold Freiwein · Köln")
}

func TestTreeHorizontalScroll(t *testing.T) {
	// Leon's pedigree is wider than the window; selecting his
	// great-grandfather scrolls it sideways.
	a := newMusterApp(t)
	a.resize(100, 30)
	a.openPerson("I3")
	a.press("t", "right", "right", "right")
	if a.m.tree.left == 0 {
		t.Fatal("the chart did not scroll")
	}
	for i, want := range []string{"┌─ Otto Mustermann (–aft.1943)", "┌─ Max Manfred Mustermann (1943–) ─┤", "└─ Maria von Stradonitz", "┌─ Erwin Mustermann (1972–) ─┤"} {
		line := ansi.Strip(a.m.renderTreeLine(i))
		node := a.m.tree.chart.LineText(i)
		if !strings.Contains(line, want) || ansi.StringWidth(line) > a.m.width {
			t.Errorf("line %d = %q, want it to contain %q", i, line, want)
		}
		// The visible part is exactly the chart line from the scroll offset.
		if wantLine := ansi.Cut(node, a.m.tree.left, a.m.tree.left+a.m.width); line != wantLine {
			t.Errorf("line %d = %q\nwant      %q", i, line, wantLine)
		}
	}
}

func TestTreeLabelsInFull(t *testing.T) {
	a := newMusterApp(t)
	a.resize(80, 40)
	a.openPerson("I30")
	a.press("d")
	// Descendant labels are never cut, at any number of generations.
	for i := 0; i < 3; i++ {
		a.contains("Descendants · selected: Freiherr Erich Karl von Stradonitz (1855–)\n",
			"\nFreiherr Erich Karl von Stradonitz (1855–)\n", "⚭ Freifrau Charlotte v. Musterow (1877–)  m. 1892")
		lines := strings.Split(a.view(), "\n")
		for _, l := range lines[3 : len(lines)-1] { // the chart, without header and key help
			if strings.Contains(l, "…") {
				t.Errorf("cut label: %q", l)
			}
		}
		a.press("+")
	}
	// The header keeps the generations when there is room.
	a.openPerson("I12")
	a.press("d")
	a.contains("Descendants · 7 generations · selected: Otto Mustermann (–aft.1943)")
	// Pedigree columns stay narrow, but names are shortened, not the dates.
	a.openPerson("I13")
	a.press("p")
	a.contains("Freifrau Charlotte v. Muste… (1877–)")
}

func TestPersonViewTitles(t *testing.T) {
	a := newMusterApp(t)
	a.openPerson("I30")
	a.contains("Freiherr Erich Karl von Stradonitz\n", "Also known as Erich Karl Freiherr von Stradonitz\n")
	a.openPerson("I32")
	a.contains("Also known as Sir Elton John (pseudonym)")
}

func TestPersonViewDetails(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I21")
	a.contains("Father        Arthur Smith (1866–1940)  (adopted)", "Brother       Harold Smith (1891–)  (adoptive)", "Sibling       Infant Smith (1895–1895)  (adoptive)")
	a.openPerson("I13")
	a.contains("Also known as Jane Smith (married)")
	a.openPerson("I8")
	a.contains("Family with Friedrich Müller", "married 1870 · divorce 1885")
	a.openPerson("I1")
	a.contains("Parents", "none recorded", "Occupation  Weaver", "cause: Consumption")
	a.openPerson("I14")
	a.contains("Daughter      Rose Smith (1893–)  (adopted)", "Military service")
	a.openPerson("I12")
	a.contains("Alice Jones")
	a.notContains("Family with")
}

func TestRelativesToggle(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I3")
	a.contains("Birth of son Thomas Smith")
	a.press("c")
	a.notContains("Birth of son Thomas Smith")
	a.contains("hiding relatives' events")
	a.press("c")
	a.contains("Birth of son Thomas Smith", "showing relatives' events")
}

func TestMarkReference(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I20")
	a.press("m")
	a.contains("marked Harold Smith as reference", "★ Harold Smith", "★ this is the reference person")
	a.openPerson("I23")
	a.contains("Relationship  Harold Smith's third cousin", "common ancestors: William Smith & Elizabeth Brown")
	a.openPerson("I14")
	a.contains("Harold Smith's father")
	a.openPerson("I22")
	a.contains("Harold Smith's wife of second cousin once removed")
	a.openPerson("I15")
	a.contains("Harold Smith's husband of half-great-aunt")
	a.openPerson("I20")
	a.press("m")
	a.contains("reference person cleared")
	a.notContains("★")
	a.openPerson("I22")
	a.press("m")
	a.openPerson("I15")
	a.contains("no relationship to Grace Hill found")
}

// m marks the person selected in the view shown, else the current one.
func TestMarkSelected(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I1")
	marked := func(view string, want *gedcom.Individual) {
		t.Helper()
		if want == nil || a.m.reference != want {
			t.Errorf("%s: marked %v, want %v", view, a.m.reference, want)
		}
		a.m.reference = nil
	}

	a.press("/", "esc")
	a.typ("smith")
	a.press("down", "alt+m")
	marked("search", a.m.search.results[1].Individual)
	if a.m.search.input.Value() != "smith" {
		t.Errorf("alt+m changed the query to %q", a.m.search.input.Value())
	}

	a.press("alt+4", "down", "down", "down", "m")
	marked("events", eventPerson(a.m.ev.list[3]))

	a.press("alt+5")
	for a.m.selectedPlace().Count() < 2 {
		a.press("down")
	}
	a.press("enter", "down", "m")
	marked("events at a place", eventPerson(a.m.places.events[1]))

	a.press("alt+7", "home", "m")
	marked("stats", a.m.stats.stats.LongestLived)

	a.press("alt+8")
	for r, cr := range a.m.checks.rows {
		if cr.finding >= 0 && a.m.checks.results[cr.check][cr.finding].Person != nil {
			a.m.checks.cursor = r
			break
		}
	}
	a.press("m")
	marked("checks", a.m.finding().Person)

	a.press("alt+6", "m")
	marked("map", a.m.doc.Individual("I1"))
}

func TestPersonViewWrap(t *testing.T) {
	a := newMusterApp(t)
	a.resize(80, 60)
	a.openPerson("I1")
	a.press("down") // Mother
	mother := a.m.person.doc.selected().target
	a.contains("Civil marriage with Erika Gabler")
	a.notContains("Deutschland · age 25") // cut at the edge of the screen

	// rowWith returns the row of the view that contains text.
	rowWith := func(text string) string {
		for _, row := range strings.Split(a.view(), "\n") {
			if strings.Contains(row, text) {
				return row
			}
		}
		return ""
	}
	a.press("W")
	a.contains("long lines in the person view are wrapped (W to cut them)")
	// The rest of the entry continues under its text, after the date.
	if row := rowWith("Deutschland · age 25"); !strings.HasPrefix(row, strings.Repeat(" ", 22)) || row[22] == ' ' {
		t.Errorf("continued row = %q", row)
	}
	if a.m.person.doc.selected().target != mother {
		t.Error("the selection moved")
	}
	for _, row := range strings.Split(a.view(), "\n") {
		if w := lipgloss.Width(row); w > 80 {
			t.Errorf("row too wide (%d): %q", w, row)
		}
	}

	// The setting is global: it stays when the view or the size changes,
	// and W works in other views too, except while typing.
	a.press("alt+3", "alt+2")
	a.contains("Deutschland · age 25")
	a.resize(100, 60)
	a.notContains("…")
	a.press("alt+3", "W")
	a.contains("long lines in the person view are cut (W to wrap them)")
	a.press("alt+2")
	a.notContains("Deutschland · age 25")
	a.press("W", "/", "W") // on, then a W typed into the search
	if !a.m.wrap || !strings.HasSuffix(a.m.search.input.Value(), "W") {
		t.Errorf("W while typing: wrap %v, query %q", a.m.wrap, a.m.search.input.Value())
	}
}

func TestTreeView(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I20")
	a.press("t")
	if a.m.active != viewTree || a.m.tree.mode != modePedigree {
		t.Fatalf("active=%v mode=%v", a.m.active, a.m.tree.mode)
	}
	a.contains("[3 Charts]", "Pedigree of Harold Smith · 4 generations · selected: Harold Smith (1891–)", "┌─ Arthur Smith (1866–1940)", "└─ Lucy King")
	a.press("right")
	a.contains("selected: Arthur Smith (1866–1940)")
	a.press("right", "down")
	a.contains("selected: Jane Doe (1844–)") // Thomas's wife, not his mother
	a.press("up")
	a.contains("selected: Thomas Smith (1842–1910)")
	a.press("left")
	a.contains("selected: Arthur Smith")
	a.press("home")
	a.contains("selected: Harold Smith")
	a.press("right", "enter")
	if a.m.active != viewPerson || a.currentID() != "I14" {
		t.Fatalf("enter should open Arthur: active=%v current=%s", a.m.active, a.currentID())
	}

	a.press("d")
	a.contains("Descendants of Arthur Smith", "⚭ Lucy King (1868–)  m. 1890", "├── Harold Smith (1891–)")
	a.press("down", "down")
	a.contains("selected: Harold Smith")
	a.press("space")
	if a.currentID() != "I20" {
		t.Errorf("space should re-root at Harold, current %s", a.currentID())
	}
	a.contains("Descendants of Harold Smith")
	a.press("space")
	a.contains("Harold Smith is already the root")
	a.press("b")
	a.contains("Descendants of Arthur Smith")
	a.press("v")
	if a.m.tree.mode != modeHourglass {
		t.Error("v goes on to the hourglass")
	}
	a.press("v")
	if a.m.tree.mode != modeLifespans {
		t.Error("v goes on to the lifespans")
	}
	a.press("v")
	if a.m.tree.mode != modePedigree {
		t.Error("v goes round to the pedigree")
	}
	a.press("p")
	a.contains("Pedigree of Arthur Smith")
}

func TestTreeHourglass(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I1")
	a.openPerson("I3")
	a.press("h") // the hourglass, not back in the history
	if a.m.active != viewTree || a.m.tree.mode != modeHourglass || a.currentID() != "I3" {
		t.Fatalf("active=%v mode=%v current=%s", a.m.active, a.m.tree.mode, a.currentID())
	}
	a.contains("Hourglass of John Smith · 4 generations · selected: John Smith (1817–1880)", "John Smith (1817–1880) ─┤")
	// The root starts in the middle of the screen.
	tr := a.m.tree
	if root := tr.chart.Nodes[0]; tr.left == 0 || root.Col-tr.left < 20 || root.Col+root.Width-tr.left > 80 {
		t.Errorf("root at columns %d–%d, view from %d", root.Col, root.Col+root.Width, tr.left)
	}

	// The arrows move as the chart is drawn: descendants to the left.
	for _, step := range []struct{ key, want string }{
		{"left", "Ann Taylor (1820–1845)"},
		{"left", "Thomas Smith (1842–1910)"},
		{"down", "Emma Smith (1848–)"},
		{"up", "Thomas Smith (1842–1910)"},
		{"right", "Ann Taylor (1820–1845)"},
		{"right", "John Smith (1817–1880)"},
		{"right", "William Smith (1790–1850)"},
		{"right", "William Smith (1790–1850)"}, // no parents recorded
		{"left", "John Smith (1817–1880)"},
	} {
		a.press(step.key)
		a.contains("selected: " + step.want)
	}
	a.press("left", "left", "+")
	a.contains("5 generations · selected: Thomas Smith (1842–1910)")
	a.press("enter")
	if a.m.active != viewPerson || a.currentID() != "I7" {
		t.Errorf("enter opened %s", a.currentID())
	}
	a.press("t", "h")
	a.contains("Hourglass of Thomas Smith")
	a.press("v", "v")
	if a.m.tree.mode != modePedigree {
		t.Error("v goes round from the hourglass by the lifespans to the pedigree")
	}
}

func TestTreeGenerations(t *testing.T) {
	a := newApp(t, Options{Generations: 3})
	a.openPerson("I20")
	a.press("t")
	a.contains("3 generations", "▸") // Thomas has more ancestors
	a.press("right")
	a.press("+")
	a.contains("showing 4 generations", "selected: Arthur Smith")
	for range 20 {
		a.press("+")
	}
	if a.m.tree.gens != maxPedigreeGenerations {
		t.Errorf("gens = %d", a.m.tree.gens)
	}
	a.contains("generations are limited to 2–20")
	for range 20 {
		a.press("-")
	}
	if a.m.tree.gens != minGenerations {
		t.Errorf("gens = %d", a.m.tree.gens)
	}
	a.press("m")
	a.contains("marked Arthur Smith as reference")
}

// The generations given at the start keep to the limits of the charts.
func TestTreeGenerationsOption(t *testing.T) {
	for _, tt := range []struct{ given, want int }{{0, 4}, {1, minGenerations}, {7, 7}, {50, maxPedigreeGenerations}} {
		a := newApp(t, Options{Generations: tt.given})
		a.openPerson("I20")
		a.press("t")
		if a.m.tree.gens != tt.want {
			t.Errorf("%d generations given: %d shown", tt.given, a.m.tree.gens)
		}
	}
	// + does not lower the generations.
	a := newApp(t, Options{Generations: 50})
	a.openPerson("I20")
	a.press("t", "+")
	if a.m.tree.gens != maxPedigreeGenerations {
		t.Errorf("after +: %d generations", a.m.tree.gens)
	}
	a.contains("generations are limited to 2–20")
}

func TestTreeDuplicates(t *testing.T) {
	doc, err := gedcom.ParseFile(filepath.Join("..", "testdata", "cousins.ged"))
	if err != nil {
		t.Fatal(err)
	}
	a := &app{t: t, m: New(doc, Options{Title: "cousins.ged", StartPerson: "R", Generations: 5})}
	a.resize(130, 30)
	sel := func() chart.Node { return a.m.tree.chart.Nodes[a.m.tree.sel] }
	toSecond := []string{"home", "right", "down", "right", "down", "right", "right"} // via Marta and Clara
	toFirst := []string{"home", "right", "right", "right", "right"}                  // via Felix and Anton

	a.press("t")
	a.press(toSecond...)
	a.contains("selected: ancestors of Georg Kern, shown elsewhere; enter or space shows them here", "─┤  as", "└─ duplicate")
	if n := sel(); n.Kind != chart.KindDuplicate || n.Path != "MMF" {
		t.Fatalf("selected %+v", n)
	}

	// Enter shows Georg's ancestors here and truncates the first occurrence.
	a.press("enter")
	if a.m.active != viewTree {
		t.Fatal("enter on a placeholder stays in the tree")
	}
	a.contains("selected: Jakob Kern", "the ancestors of Georg Kern are shown here now and truncated elsewhere")
	if n := sel(); n.Path != "MMFF" {
		t.Errorf("selected %+v", n)
	}
	a.press(toFirst...)
	if n := sel(); n.Kind != chart.KindDuplicate || n.Path != "FFF" {
		t.Fatalf("first occurrence: selected %+v", n)
	}

	// Space moves them back instead of making Georg the root.
	a.press("space")
	if a.currentID() != "R" || sel().Path != "FFFF" {
		t.Errorf("space: root %s, selected %+v", a.currentID(), sel())
	}

	// With fewer generations the placeholder's person stays selected.
	a.press(toSecond...)
	a.press("-")
	if n := sel(); n.Kind != chart.KindPerson || n.Path != "MMF" {
		t.Errorf("after -: selected %+v", n)
	}
	a.contains("selected: Georg Kern", "Georg Kern ▸")
	a.notContains("duplicate")

	// A new root starts with the ancestors at the first occurrence again.
	a.press("+")
	a.press(toSecond...)
	a.press("enter", "home", "right", "space", "b")
	if a.currentID() != "R" {
		t.Fatalf("current %s", a.currentID())
	}
	if c := a.m.tree.chart; c.FindPath("FFFF") < 0 || c.FindPath("MMFF") >= 0 {
		t.Errorf("choices kept for a new root:\n%s", c)
	}

	// A person who is their own father: their ancestors cannot be shown
	// among themselves.
	doc, err = gedcom.ParseString("0 HEAD\n0 @I1@ INDI\n1 NAME Loop /Person/\n1 FAMC @F1@\n1 FAMS @F1@\n0 @F1@ FAM\n1 HUSB @I1@\n1 CHIL @I1@\n0 TRLR\n")
	if err != nil {
		t.Fatal(err)
	}
	a = &app{t: t, m: New(doc, Options{StartPerson: "I1"})}
	a.resize(100, 30)
	a.press("t", "right", "right", "enter")
	a.contains("the ancestors of Loop Person cannot be shown here")
	if n := sel(); n.Kind != chart.KindPerson || n.Path != "F" {
		t.Errorf("loop: selected %+v", n)
	}
}

func TestChecksView(t *testing.T) {
	a := newMusterApp(t)
	a.press("alt+8")
	a.contains("[8 Checks]", "Checks · 3 findings in 3 people", "Hints · ", "Born less than 9 months after a sibling (twins excepted)",
		"Wilhelm Schüchter (1936–1964)  birth 17 Nov 1936 and birth 18 Nov 1936", "at most 189 days after Hans Mustermann")
	row := func(id string) int {
		for r, cr := range a.m.checks.rows {
			if genealogy.Checks[cr.check].ID == id && cr.finding < 0 {
				return r
			}
		}
		t.Fatalf("no row for %s", id)
		return 0
	}

	// A check without a threshold says so.
	a.press("left")
	a.contains("this check has no threshold")

	// Lower the threshold: the findings change at once.
	a.m.checks.cursor = row("siblings-close")
	a.m.moveChecks(0)
	a.press("left", "left", "left")
	a.contains("Born less than 6 months after a sibling (twins excepted): 1 finding", "Checks · 2 findings in 2 people")
	a.notContains("189 days")
	a.press("right", "right", "right")
	a.contains("189 days", "Checks · 3 findings in 3 people")

	// Switched off, a check finds nothing.
	a.press(" ")
	a.contains("Checks · 1 finding in 1 person")
	a.notContains("189 days")
	a.press(" ")
	a.contains("3 findings")

	// enter folds and unfolds the check, and opens the person of a
	// finding; o opens the other person.
	a.press("enter")
	a.notContains("189 days")
	a.press("enter", "down", "down")
	a.press("enter")
	if a.m.active != viewPerson || a.currentID() != "I1" {
		t.Fatalf("enter opened %v %s", a.m.active, a.currentID())
	}
	a.press("alt+8", "o")
	if a.currentID() != "I9" {
		t.Errorf("o opened %s, want Hans (I9)", a.currentID())
	}

	// Thresholds stay within their range; r resets them.
	a.press("alt+8")
	a.m.checks.cursor = row("old-age")
	for range 50 {
		a.press("right")
	}
	a.contains("Lived longer than 150 years")
	a.press("r")
	a.contains("all checks and hints are on, with the usual thresholds", "Lived longer than 110 years")
	a.press("-")
	a.notContains("Wilhelm Schüchter")
	a.press("+")
	a.contains("Wilhelm Schüchter")
	for _, l := range strings.Split(a.view(), "\n") {
		if w := lipgloss.Width(l); w > 110 {
			t.Errorf("line too wide (%d): %q", w, l)
		}
	}
}

func TestSearchBooleanTyping(t *testing.T) {
	a := newApp(t, Options{})
	// Half typed, the query already finds the people.
	a.typ("(smith | jones")
	a.contains("Smith, John", "Jones, Henry")
	a.notContains("unmatched")
	a.typ(") born:<1816 |")
	a.contains("Smith, William", "Jones, Robert")
	a.notContains("Smith, John ")
	a.typ(")")
	a.contains("unmatched )")
}

func TestSearchKinTyping(t *testing.T) {
	a := newApp(t, Options{})
	a.typ("ancestor:I99")
	a.contains("ancestor: nobody has the ID I99")
	a.press("backspace", "backspace")
	a.typ("20")
	a.contains("8 matches", "Smith, William", "King, Lucy")
	a.typ("=..2 -sex:m")
	a.contains("2 matches", "Doe, Jane", "King, Lucy")
	a.notContains("Brown, Elizabeth")
}

func TestHintsView(t *testing.T) {
	a := newApp(t, Options{Today: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)})
	a.resize(110, 60)
	a.press("alt+8")
	a.contains("Checks · 0 findings in 0 people", "Hints · 74 findings in 24 people",
		"▾ Unknown parents within 4 generations of the marked person",
		"      nobody is marked: mark a person to see the gaps in their ancestry",
		"Harold Smith (1891–)  birth 1891, at least 134 years ago")
	row := func(id string) int {
		for r, cr := range a.m.checks.rows {
			if genealogy.Checks[cr.check].ID == id && cr.finding == rowCheck {
				return r
			}
		}
		t.Fatalf("no row for %s", id)
		return 0
	}
	at := func() string { return genealogy.Checks[a.m.checks.rows[a.m.checks.cursor].check].ID }

	// The cursor passes the headings and the summaries.
	a.m.checks.cursor = row("duplicates")
	a.press("down")
	if at() != "missing-parents" {
		t.Errorf("down from the last check: %s", at())
	}
	a.press("down")
	if at() != "no-birth-date" || a.m.finding() != nil {
		t.Errorf("down past the summary: %s", at())
	}
	a.press("up", "up")
	if at() != "duplicates" {
		t.Errorf("up past the heading: %s", at())
	}
	a.press("home")
	if a.m.checks.cursor != 1 || a.m.checks.offset != 0 {
		t.Errorf("home: cursor %d, offset %d", a.m.checks.cursor, a.m.checks.offset)
	}
	a.clickOn("Hints · ")
	if a.m.checks.cursor != 1 {
		t.Error("a click on a heading moved the cursor")
	}

	// Marking a person runs the hint on their ancestry again.
	a.openPerson("I20")
	a.press("m", "alt+8")
	a.contains("Hints · 77 findings in 24 people",
		"      known: parents 2 of 2 · grandparents 2 of 4 · great-grandparents 2 of 8 · 2nd great-grandparents 2 of 16",
		"Lucy King (1868–)  parents unknown · the marked person's mother",
		"Jane Doe (1844–)  parents unknown · the marked person's grandmother",
		"Ann Taylor (1820–1845)  parents unknown · the marked person's great-grandmother")
	a.m.checks.cursor = row("missing-parents")
	a.press("left")
	a.contains("Unknown parents within 3 generations of the marked person: 2 findings")
	a.notContains("Ann Taylor (1820–1845)  parents unknown")
	a.press("enter")
	a.notContains("known: parents")
	a.press("enter", "m")
	a.contains("nobody is marked")

	// Groups of place names open in the places view, which shows them.
	a.press("alt+5", "-", "alt+8")
	a.m.checks.cursor = row("similar-places")
	a.m.moveChecks(0)
	a.press("up") // the level stands out where the check is not selected
	a.contains("Places that may be the same: " + a.m.st.key.Render("similar") + a.m.st.name.Render(" names"))
	a.press("down", "right")
	a.contains("Places that may be the same: somewhat resembling names: 1 finding")
	a.press("down")
	a.contains("St Peter's · St Peter's Churchyard (in Leeds, Yorkshire, England)")
	a.press("enter")
	if p := a.m.selectedPlace(); a.m.active != viewPlaces || p == nil || p.Full != "St Peter's, Leeds, Yorkshire, England" {
		t.Fatalf("view %d, place %v", a.m.active, p)
	}
	a.contains("place 1 of 2 in the group")
	for _, want := range []string{"St Peter's Churchyard, Leeds, Yorkshire, England", "St Peter's, Leeds, Yorkshire, England"} {
		a.press("alt+8", "o")
		if p := a.m.selectedPlace(); a.m.active != viewPlaces || p == nil || p.Full != want {
			t.Errorf("o opened %v, want %s", p, want)
		}
	}
	a.contains("place 1 of 2 in the group")
}

func TestExportView(t *testing.T) {
	a := newApp(t, Options{})
	dir := t.TempDir()
	t.Chdir(dir) // default file names are relative to the current directory
	a.press("alt+9")
	a.contains("[9 Export]", "Format", "GEXF", "Graph Exchange XML Format", "◂ all ▸", "24 people · 37 relationships",
		"family.gexf", "mark a person with m to export only the people connected to them", "↑↓ choose")

	// The connected people need a marked person.
	a.press("down", "right")
	a.contains("mark a person with m first")
	if a.m.export.scope != export.All {
		t.Error("scope changed without a marked person")
	}

	// exported checks that a file is in dir, the current directory, and
	// contains want. The message names the directory too, but that may
	// not fit on the line.
	exported := func(name string, nodes int, want string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || strings.Count(string(data), "<node ") != nodes || !strings.Contains(string(data), want) {
			t.Fatalf("%s: %v\n%.600s", name, err, data)
		}
	}

	// Export everybody; a second export asks before replacing the file.
	a.press("down", "down", "enter")
	a.contains("exported 24 people and 37 relationships as GEXF to family.gexf")
	exported("family.gexf", 24, "<description>All 24 people of family.ged and their relationships</description>")
	if err := os.WriteFile(filepath.Join(dir, "family.gexf"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.press("enter")
	a.contains("family.gexf exists: press enter again to replace it")
	a.press("up", "down", "enter") // another key in between asks again
	a.contains("family.gexf exists")
	a.press("enter")
	a.contains("exported 24 people")
	exported("family.gexf", 24, "<gexf ") // replaced

	// Mark Harold: his connected people get a file name of their own.
	a.openPerson("I20")
	a.press("m", "alt+9", "up", "up", "right")
	a.contains("◂ connected to ★ Harold Smith ▸", "24 people · 37 relationships", "family-I20.gexf")
	a.notContains("mark a person with m to export")
	a.press("down", "down", "enter")
	a.contains("exported 24 people and 37 relationships as GEXF to family-I20.gexf")
	exported("family-I20.gexf", 24, "connected to Harold Smith (I20)")

	// A typed file name stays when the settings change; esc cancels typing.
	a.press("up", "enter")
	if !a.m.typing() {
		t.Fatal("enter on the file name starts editing")
	}
	a.contains("type file name")
	a.typ("x")
	a.press("esc")
	a.contains("family-I20.gexf")
	a.notContains("family-I20.gexfx")
	a.press("enter")
	a.typ("-8") // digits are typed, not view switches
	a.press("enter")
	if a.m.active != viewExport || a.m.export.path.Value() != "family-I20.gexf-8" {
		t.Fatalf("active %v, path %q", a.m.active, a.m.export.path.Value())
	}
	a.press("up", "left")
	a.contains("◂ all ▸", "family-I20.gexf-8")
	// An emptied name follows the settings again.
	a.press("down", "enter", "ctrl+u", "enter")
	a.contains("family.gexf")

	// Unmarking returns to everybody.
	a.press("up", "right")
	a.contains("family-I20.gexf")
	a.press("m")
	a.contains("reference person cleared", "◂ all ▸", "family.gexf")

	// A failed export says why.
	a.press("down", "enter", "ctrl+u")
	a.typ(filepath.Join("missing", "x.gexf"))
	a.press("enter", "down", "enter")
	a.contains("export failed:")

	a.resize(80, 24)
	for _, line := range strings.Split(a.view(), "\n") {
		if w := lipgloss.Width(line); w > 80 {
			t.Errorf("line too wide (%d): %q", w, line)
		}
	}
}

func TestTreeScrolling(t *testing.T) {
	a := newApp(t, Options{Generations: 5})
	a.resize(50, 8)
	a.openPerson("I20")
	a.press("t")
	// Walk to William Smith, the rightmost and topmost node.
	a.press("right", "right", "right", "right")
	sel := a.m.tree.chart.Nodes[a.m.tree.sel]
	if sel.Ind.ID != "I1" {
		t.Fatalf("selected %s", sel.Ind.ID)
	}
	if a.m.tree.left == 0 {
		t.Error("chart should scroll horizontally")
	}
	a.contains("William Smith")
	a.press("pgdown")
	if a.m.tree.chart.Nodes[a.m.tree.sel].Line <= sel.Line {
		t.Error("pgdown should move down")
	}
	a.press("pgup")
	a.press("home")
	if a.m.tree.sel != 0 {
		t.Error("home selects the root")
	}
	for _, line := range strings.Split(a.view(), "\n") {
		if w := lipgloss.Width(line); w > 50 {
			t.Errorf("line too wide (%d): %q", w, line)
		}
	}
}

func TestEventsView(t *testing.T) {
	a := newApp(t, Options{})
	a.press("4")
	if a.m.active != viewSearch || a.m.search.input.Value() != "4" {
		t.Fatal("digits are typed in the search view")
	}
	a.press("esc", "alt+4")
	if a.m.active != viewEvents {
		t.Fatal("alt+4 opens events")
	}
	a.contains("[4 Events]", "46 important events", "12 Mar 1790", "Birth", "William Smith")
	a.press("a")
	a.contains("54 events", "showing all events")
	a.press("f")
	if !a.m.ev.editing {
		t.Fatal("f starts editing the filter")
	}
	a.typ("type:marr")
	a.contains("Filter: type:marr", "9 events", "William Smith & Elizabeth Brown")
	a.press("enter")
	if a.m.ev.editing {
		t.Error("enter finishes editing")
	}
	a.press("down", "up", "enter")
	if a.m.active != viewPerson || a.currentID() != "I1" {
		t.Errorf("enter opens the husband: %v %s", a.m.active, a.currentID())
	}
	a.press("4", "x")
	a.contains("54 events")
	a.press("f")
	a.typ("foo:bar")
	a.contains(`unknown field "foo"`)
	a.press("esc")
	if a.m.ev.editing || a.m.ev.input.Value() != "" {
		t.Error("esc clears and stops editing")
	}
	a.press("a")
	a.contains("showing important events only")
	a.press("end")
	if a.m.ev.cursor != 45 {
		t.Errorf("end -> cursor %d", a.m.ev.cursor)
	}
	a.press("home", "pgdown", "pgup")
	if a.m.ev.cursor != 0 {
		t.Errorf("cursor %d", a.m.ev.cursor)
	}
	a.press("f")
	a.press("down", "pgdown", "pgup", "up", "tab")
	if a.m.ev.editing || a.m.active != viewPlaces {
		t.Errorf("tab leaves the filter and switches view: editing=%v active=%v", a.m.ev.editing, a.m.active)
	}
}

func TestStatsView(t *testing.T) {
	a := newApp(t, Options{})
	a.resize(100, 150)
	a.press("alt+7")
	a.contains("[7 Stats]", "Encoding", "UTF-8", "Individuals", "24  (10 male, 13 female, 1 other/unknown)", "Generations", "Longest lived",
		"Mary Smith, ~80 years", "Most common surnames", "Smith")
	a.press("end")
	a.contains("Warnings (0)", "none — the file looks consistent", "Most common given names",
		"Pedigree collapse", "0.0% of all ancestor positions · no one has an ancestor more than once")
	if v := a.view(); strings.Index(v, "Pedigree collapse") < strings.Index(v, "On this day") || strings.Index(v, "Pedigree collapse") > strings.Index(v, "Warnings (0)") {
		t.Errorf("the pedigree collapse belongs between On this day and the warnings:\n%s", v)
	}
	a.press("home", "enter")
	if a.currentID() != "I4" || a.m.active != viewPerson {
		t.Errorf("longest lived link should open Mary, got %s", a.currentID())
	}
	a.press("7", "home", "down", "enter")
	a.contains("What would you like to see of Smith?", "[ People with that surname ]  [ Surname on the map ]", "p people")
	a.press("p")
	if a.m.active != viewSearch || a.m.search.input.Value() != `surname:"Smith"` {
		t.Fatalf("active=%v query=%q", a.m.active, a.m.search.input.Value())
	}
	a.contains("11 matches") // includes Jane Doe, née Smith by marriage
	a.press("tab")           // -> person (a current person exists)
	a.press("tab", "tab", "tab", "tab", "tab")
	a.press("pgdown", "pgup", "up", "down")
	if a.m.active != viewStats {
		t.Errorf("active = %v", a.m.active)
	}
}

func TestStatsCharts(t *testing.T) {
	a := newApp(t, Options{})
	a.resize(100, 80)
	a.press("alt+7", "end")
	a.contains("Births per decade\n", "  1840s  7 "+strings.Repeat("━", 87)+"\n", "  1800s  0 \n", "  1850s  1 "+strings.Repeat("━", 12)+"╸\n",
		"Age at death\n", "  60–69  4 ━━━━", "Age at marriage (each partner)\n", "  20–24  13 ━━━━",
		"Children per family\n", "  3  1 "+strings.Repeat("━", 18)+"\n")
	v := a.view()
	if i := strings.Index(v, "Births per decade"); i < strings.Index(v, "Pedigree collapse") || i > strings.Index(v, "Warnings (0)") {
		t.Errorf("the charts belong between the pedigree collapse and the warnings:\n%s", v)
	}
	// The bars of births link to the people born then. Selected, the bar
	// keeps its look.
	for range 20 {
		if strings.Contains(a.view(), "▸ 1840s") {
			break
		}
		a.press("up")
	}
	a.contains(a.m.st.selected.Render("▸ 1840s  7 ") + a.m.st.bar.Render(strings.Repeat("━", 87)))
	a.press("enter")
	if a.m.active != viewSearch || a.m.search.input.Value() != "born:1840..1849" {
		t.Fatalf("view %d, query %q", a.m.active, a.m.search.input.Value())
	}

	// The bars fit a narrower screen.
	a.press("alt+7")
	a.resize(60, 80)
	a.press("end")
	a.contains("  1840s  7 " + strings.Repeat("━", 47) + "\n")
	for _, l := range strings.Split(ansi.Strip(a.view()), "\n") {
		if w := ansi.StringWidth(l); w > 60 || strings.Contains(l, "━…") {
			t.Errorf("line of %d columns: %q", w, l)
		}
	}

	b := newApp(t, Options{ASCII: true})
	b.resize(100, 80)
	b.press("alt+7", "end")
	b.contains("  1850s  1 ============\n")
}

func TestBar(t *testing.T) {
	for _, tt := range []struct {
		n, most, room int
		ascii         bool
		want          string
	}{
		{1, 1, 8, false, "━━━━━━━━"},
		{1, 16, 8, false, "╸"},
		{1, 1000, 8, false, "╸"},
		{3, 8, 8, false, "━━━"},
		{1, 2, 3, false, "━╸"},
		{0, 5, 10, false, ""},
		{1, 1000, 8, true, "="},
		{5, 10, 8, true, "===="},
	} {
		if got := bar(tt.n, tt.most, tt.room, tt.ascii); got != tt.want {
			t.Errorf("bar(%d, %d, %d, %v) = %q, want %q", tt.n, tt.most, tt.room, tt.ascii, got, tt.want)
		}
	}
}

func TestStatsOnThisDay(t *testing.T) {
	a := &app{t: t, m: New(loadSample(t), Options{Today: time.Date(2026, time.January, 4, 9, 0, 0, 0, time.Local)})}
	a.resize(120, 80)
	a.press("alt+7", "end")
	a.contains("On this day (4 January)\n", "1850 · 176 years ago  Death  William Smith  Leeds, Yorkshire, England")
	view := a.view()
	if strings.Index(view, "On this day") > strings.Index(view, "Warnings (0)") || strings.Index(view, "On this day") < strings.Index(view, "Most common given names") {
		t.Errorf("On this day belongs between the given names and the warnings:\n%s", view)
	}
	// Up from the bars of the charts, which come after it.
	for i := 0; i < 50 && !strings.Contains(a.view(), "▸ 1850 · "); i++ {
		a.press("up")
	}
	a.press("enter")
	if a.currentID() != "I1" || a.m.active != viewPerson {
		t.Errorf("enter opened %s", a.currentID())
	}

	a = &app{t: t, m: New(loadSample(t), Options{Today: time.Date(2026, time.January, 5, 9, 0, 0, 0, time.Local)})}
	a.resize(120, 80)
	a.press("alt+7", "end")
	a.contains("On this day (5 January)", "no events recorded for 5 January")
}

func TestStatsWarnings(t *testing.T) {
	doc, err := gedcom.ParseString("0 HEAD\n0 @I1@ INDI\n1 NAME A /B/\n1 FAMS @F9@\n")
	if err != nil {
		t.Fatal(err)
	}
	m := New(doc, Options{})
	a := &app{t: t, m: m}
	a.resize(120, 40)
	a.press("alt+7")
	a.contains("Warnings (2)", "refers to missing family @F9@", "missing TRLR")
}

func TestHelp(t *testing.T) {
	a := newApp(t, Options{})
	a.press("?")
	if !a.m.showHelp {
		t.Fatal("? opens help")
	}
	a.contains("[Help]", "Global")
	for range 10 { // to the end of the help, however long it grows
		a.press("pgdown")
	}
	a.contains("tag:_MILT", "Event filter")
	a.press("up", "pgup")
	a.press("x")
	if a.m.showHelp {
		t.Error("any other key closes help")
	}
	a.press("f1")
	if !a.m.showHelp {
		t.Error("f1 opens help")
	}
	if cmd := a.press("q"); !isQuit(cmd) {
		t.Error("q quits from help")
	}
	if !strings.Contains(helpText(), "tag:_MILT") {
		t.Error("helpText")
	}
}

func TestViewSwitching(t *testing.T) {
	a := newApp(t, Options{})
	a.press("tab")
	if a.m.active != viewEvents {
		t.Errorf("tab without a person skips person and tree: %v", a.m.active)
	}
	a.press("tab")
	if a.m.active != viewPlaces {
		t.Errorf("active = %v", a.m.active)
	}
	a.press("tab")
	if a.m.active != viewMap {
		t.Errorf("active = %v", a.m.active)
	}
	a.press("tab")
	if a.m.active != viewStats {
		t.Errorf("active = %v", a.m.active)
	}
	a.press("tab")
	if a.m.active != viewChecks {
		t.Errorf("active = %v", a.m.active)
	}
	a.press("tab")
	if a.m.active != viewExport {
		t.Errorf("active = %v", a.m.active)
	}
	a.press("tab")
	if a.m.active != viewSearch {
		t.Errorf("active = %v", a.m.active)
	}
	a.press("shift+tab")
	if a.m.active != viewExport {
		t.Errorf("shift+tab -> %v", a.m.active)
	}
	a.press("2")
	a.contains("select a person first")
	a.press("3")
	if a.m.active != viewExport {
		t.Error("tree needs a person")
	}
	a.openPerson("I3")
	var seq []view
	for range 9 {
		a.press("tab")
		seq = append(seq, a.m.active)
	}
	want := []view{viewTree, viewEvents, viewPlaces, viewMap, viewStats, viewChecks, viewExport, viewSearch, viewPerson}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("tab sequence = %v, want %v", seq, want)
		}
	}
	a.press("1")
	if a.m.active != viewSearch {
		t.Error("1 opens search")
	}
	a.press("esc", "esc")
	a.press("i")
	if a.m.active != viewPerson {
		t.Error("i shows the current person")
	}
}

func TestStartPerson(t *testing.T) {
	a := newApp(t, Options{StartPerson: "@I7@"})
	if a.m.active != viewPerson || a.currentID() != "I7" {
		t.Fatalf("start person: %v %s", a.m.active, a.currentID())
	}
	a.contains("Thomas Smith")
	b := newApp(t, Options{StartPerson: "nobody"})
	if b.m.active != viewSearch {
		t.Error("invalid start person falls back to search")
	}
	b.contains(`no individual with ID "nobody"`)
}

func TestQuit(t *testing.T) {
	a := newApp(t, Options{})
	if !isQuit(a.press("ctrl+c")) {
		t.Error("ctrl+c quits in search")
	}
	if isQuit(a.press("q")) {
		t.Error("q is typed in search")
	}
	a.openPerson("I1")
	if !isQuit(a.press("q")) {
		t.Error("q quits in the person view")
	}
}

func TestMultiRuneKeys(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I3")
	a.press("enter") // to William
	a.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("bt")})
	if a.currentID() != "I3" || a.m.active != viewTree {
		t.Errorf("'bt' should go back and open the tree: %s %v", a.currentID(), a.m.active)
	}
	a.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/sm")})
	if a.m.active != viewSearch || !strings.HasSuffix(a.m.search.input.Value(), "sm") {
		t.Errorf("'/sm' should open search and type: %v %q", a.m.active, a.m.search.input.Value())
	}
}

func TestLayoutFitsWindow(t *testing.T) {
	sizes := [][2]int{{100, 30}, {60, 20}, {30, 8}, {12, 4}, {1, 1}, {0, 0}}
	for _, size := range sizes {
		a := newApp(t, Options{})
		a.openPerson("I3")
		a.resize(size[0], size[1])
		for _, keys := range [][]string{{"1"}, {"2"}, {"3"}, {"d"}, {"4"}, {"5"}, {"enter"}, {"6"}, {"7"}, {"?"}, {"L"}} {
			a.press(keys...)
			v := a.view()
			lines := strings.Split(v, "\n")
			wantLines := max(size[1], 4)
			if len(lines) != wantLines {
				t.Errorf("%dx%d %v: %d lines, want %d", size[0], size[1], keys, len(lines), wantLines)
			}
			for _, l := range lines {
				if w := lipgloss.Width(l); w > max(size[0], 1) && size[0] > 0 {
					t.Errorf("%dx%d %v: line too wide (%d): %q", size[0], size[1], keys, w, l)
				}
			}
			if keys[0] == "?" || keys[0] == "L" {
				a.press("x")
			}
			if keys[0] == "1" {
				a.press("esc", "esc")
			}
		}
	}
}

func TestEmptyDocumentViews(t *testing.T) {
	doc, _ := gedcom.ParseString("0 HEAD\n0 TRLR\n")
	a := &app{t: t, m: New(doc, Options{})}
	a.resize(80, 20)
	a.contains("0 people")
	a.press("enter", "down", "esc")
	a.press("alt+4")
	a.contains("0 important events")
	a.press("enter", "down")
	a.press("5")
	a.contains("[5 Places]", "0 places · 0 events with a place")
	a.press("enter", "left", "right", "-", "+", "down", "M")
	a.press("6")
	a.contains("[6 Map]", "no place in this file has coordinates")
	a.press("n", "enter", "+", "-", "left", "0", "w")
	a.press("7")
	a.contains("Individuals")
	if a.m.View() == "" {
		t.Error("empty view")
	}
	// Person and tree views without a current person.
	if !strings.Contains(a.m.viewPerson(5), "No person selected") || !strings.Contains(a.m.viewTree(5), "No person selected") {
		t.Error("placeholder views")
	}
}

func TestPlacesView(t *testing.T) {
	a := newApp(t, Options{})
	a.press("alt+5")
	if a.m.active != viewPlaces {
		t.Fatalf("alt+5 opens places, got %v", a.m.active)
	}
	a.contains("[5 Places]", "11 places · 22 events with a place",
		"▸ ▾ Canada", "▾ England", "Yorkshire", "St Peter's Churchyard", "Preußen", "Köln")
	if len(a.m.places.rows) != 17 {
		t.Errorf("visible places = %d, want 17", len(a.m.places.rows))
	}
	// Collapse and expand.
	a.press("down", "down", "down") // Canada, Ontario, Toronto, England
	if sel := a.m.selectedPlace(); sel.Name != "England" {
		t.Fatalf("selected %q", sel.Name)
	}
	a.contains("19 events    11 people", "1 event      1 person")
	if plural(1, "person", "people") != "1 person" || plural(0, "event", "events") != "0 events" {
		t.Error("plural")
	}
	a.press("left")
	a.contains("▸ ▹ England")
	a.notContains("Lancashire")
	a.press("left") // already collapsed and top level: stays
	if a.m.selectedPlace().Name != "England" {
		t.Error("left on a collapsed top-level place stays")
	}
	a.press("right")
	a.contains("Lancashire")
	a.press("right", "right") // into Lancashire, then Liverpool
	if sel := a.m.selectedPlace(); sel.Name != "Liverpool" {
		t.Fatalf("selected %q", sel.Name)
	}
	a.press("right") // a leaf: nothing happens
	a.press("left")  // to Lancashire
	if a.m.selectedPlace().Name != "Lancashire" {
		t.Errorf("left goes to the enclosing place, got %q", a.m.selectedPlace().Name)
	}
	a.press("-")
	if len(a.m.places.rows) != 4 || a.m.selectedPlace().Name != "England" {
		t.Errorf("collapse all: %d rows, selected %q", len(a.m.places.rows), a.m.selectedPlace().Name)
	}
	a.press("+")
	if len(a.m.places.rows) != 17 || a.m.selectedPlace().Name != "England" {
		t.Errorf("expand all: %d rows, selected %q", len(a.m.places.rows), a.m.selectedPlace().Name)
	}

	// Events at a place and the places within it.
	a.press("down", "down", "down", "down", "down", "down", "down") // Lancashire (3 towns), Yorkshire, Hull, Leeds
	if a.m.selectedPlace().Name != "Leeds" {
		t.Fatalf("selected %q", a.m.selectedPlace().Name)
	}
	a.press("enter")
	a.contains("Events in Leeds, Yorkshire, England · 9 events · 6 people", "12 Mar 1790", "St Peter's Churchyard")
	if len(a.m.places.events) != 9 {
		t.Errorf("events = %d", len(a.m.places.events))
	}
	a.press("end", "home", "pgdown", "pgup", "down", "up")
	a.press("enter")
	if a.m.active != viewPerson || a.currentID() != "I1" {
		t.Fatalf("enter opens William: %v %s", a.m.active, a.currentID())
	}
	a.press("5")
	a.contains("Events in Leeds") // the place list is remembered
	a.press("esc")
	a.contains("▸     ▾ Leeds")

	// Filtering shows matches with their enclosing and contained places.
	a.press("f")
	a.typ("peter")
	a.contains("Filter: peter", "England", "Yorkshire", "Leeds", "St Peter's", "St Peter's Churchyard")
	a.notContains("Lancashire", "Canada")
	if len(a.m.places.rows) != 5 {
		t.Errorf("filtered rows = %d", len(a.m.places.rows))
	}
	a.press("enter")
	a.press("left") // collapsing is disabled while filtering
	if len(a.m.places.rows) != 5 {
		t.Error("collapse while filtering")
	}
	a.press("x")
	if len(a.m.places.rows) != 17 {
		t.Errorf("clearing the filter restores the tree: %d rows", len(a.m.places.rows))
	}
	a.press("f")
	a.typ("yorks")
	if len(a.m.places.rows) != 7 { // England, Yorkshire and everything in it
		t.Errorf("rows = %d", len(a.m.places.rows))
	}
	a.press("down", "up")
	a.typ("x")
	a.contains("no place matches the filter")
	a.press("esc")
	if a.m.places.editing || a.m.places.input.Value() != "" {
		t.Error("esc clears the filter")
	}
	a.press("f")
	a.typ("köln")
	a.press("tab")
	if a.m.places.editing || a.m.active != viewMap {
		t.Errorf("tab leaves the filter: editing=%v active=%v", a.m.places.editing, a.m.active)
	}
}

func TestMapView(t *testing.T) {
	a := newApp(t, Options{})
	a.resize(120, 36)
	a.press("alt+6")
	if a.m.active != viewMap {
		t.Fatalf("alt+6 opens the map, got %v", a.m.active)
	}
	a.contains("[6 Map]", "Map · Equirectangular · zoom", "10 places with coordinates, 1 without",
		"◉ Leeds, Yorkshire, England · 53.80°N 1.55°W · 9 events · 6 people",
		"Map data: Made with Natural Earth (public domain) · press L for licenses")
	if len(a.m.mapv.places) != 10 || a.m.mapv.places[0].Name != "Leeds" || a.m.mapv.places[1].Name != "Manchester" {
		t.Errorf("places = %d, first %q, %q", len(a.m.mapv.places), a.m.mapv.places[0].Name, a.m.mapv.places[1].Name)
	}
	// The initial view fits all places: Toronto and Köln are both visible.
	frameHasRune := func(r rune) bool { return strings.ContainsRune(a.view(), r) }
	if !frameHasRune('◉') || !strings.Contains(a.view(), "Toronto") {
		t.Errorf("overview should show the selected marker and Toronto:\n%s", a.view())
	}
	if !strings.ContainsAny(a.view(), "⠁⠂⠄⡀⠈⠐⠠⢀") {
		t.Error("the map should contain braille line art")
	}

	zoom := a.m.mapv.view.Zoom
	a.press("+", "+")
	if a.m.mapv.view.Zoom != zoom+1 {
		t.Errorf("zoom = %v, want %v", a.m.mapv.view.Zoom, zoom+1)
	}
	a.press("-")
	if a.m.mapv.view.Zoom != zoom+0.5 {
		t.Errorf("zoom out = %v", a.m.mapv.view.Zoom)
	}
	for range 40 {
		a.press("+")
	}
	if a.m.mapv.view.Zoom != worldmapMaxZoom() {
		t.Errorf("zoom clamps at the maximum: %v", a.m.mapv.view.Zoom)
	}
	for range 60 {
		a.press("-")
	}
	if a.m.mapv.view.Zoom != worldmapMinZoom() {
		t.Errorf("zoom clamps at the minimum: %v", a.m.mapv.view.Zoom)
	}

	a.press("0")
	x0, y0 := a.m.mapv.view.X, a.m.mapv.view.Y
	a.press("right", "down")
	if a.m.mapv.view.X <= x0 || a.m.mapv.view.Y <= y0 {
		t.Error("right/down pan east/south")
	}
	a.press("left", "up", "h", "k", "l", "j")
	if diff := a.m.mapv.view.X - x0; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("panning back returns to the start: %v", diff)
	}
	for range 50 {
		a.press("left", "up")
	}
	// The center stays on the map, and the top at 80°N.
	_, north := a.m.mapv.view.Proj.Project(worldmap.North, 0)
	_, spanY := a.m.mapv.view.Span(a.m.mapCols(), a.m.mapRows())
	if v := a.m.mapv.view; v.X != 0 || math.Abs(v.Y-spanY/2-north) > 1e-9 {
		t.Errorf("the center stays on the map: %v,%v", v.X, v.Y)
	}

	// Cycle through places; the view follows the selection.
	a.press("n")
	a.contains("◉ Manchester, Lancashire, England")
	if a.m.mapv.view.X != a.m.mapv.markers[1].X {
		t.Error("n centers on the next place")
	}
	a.press("N", "N")
	if a.m.mapv.sel != 9 {
		t.Errorf("N wraps around: sel %d", a.m.mapv.sel)
	}
	a.press("p", "space", "c")
	a.press("w")
	if a.m.mapv.view != worldmap.Equirectangular.WholeWorld(a.m.mapCols(), a.m.mapRows()) {
		t.Errorf("w shows the whole map: %+v", a.m.mapv.view)
	}
	// Equirectangular by default: the middle of 80°N and 60°S is 10°N.
	a.contains("Map · Equirectangular · ", "center 10.00°N 0.00°E")

	// v goes through the projections, keeping the center where it is
	// zoomed in, and moving the markers.
	leeds := a.m.mapv.markers[0]
	a.press("+", "+", "+", "left", "up")
	center := func() (float64, float64) { return a.m.mapv.view.Proj.Unproject(a.m.mapv.view.X, a.m.mapv.view.Y) }
	lat0, lon0 := center()
	a.press("v")
	lat, lon := center()
	a.contains("Map · Equal Earth · ", "Equal Earth projection: areas true")
	if a.m.mapv.view.Proj != worldmap.EqualEarth || a.m.mapv.markers[0] == leeds || math.Abs(lat-lat0) > 1e-6 || math.Abs(lon-lon0) > 1e-6 {
		t.Errorf("Equal Earth: view %+v, Leeds %+v, center %v,%v", a.m.mapv.view, a.m.mapv.markers[0], lat, lon)
	}
	if x, y := worldmap.EqualEarth.Project(53.7997, -1.5492); a.m.mapv.markers[0].X != x || a.m.mapv.markers[0].Y != y {
		t.Errorf("Leeds in Equal Earth at %+v", a.m.mapv.markers[0])
	}
	a.press("v")
	if lat2, lon2 := center(); a.m.mapv.view.Proj != worldmap.WebMercator || math.Abs(lat2-lat) > 1e-6 || math.Abs(lon2-lon) > 1e-6 {
		t.Errorf("Web Mercator: %+v, center %v,%v", a.m.mapv.view, lat2, lon2)
	}
	a.contains("Map · Web Mercator · ")
	a.press("v")
	if a.m.mapv.view.Proj != worldmap.Equirectangular || a.m.mapv.markers[0] != leeds {
		t.Errorf("back to Equirectangular: %+v, Leeds %+v", a.m.mapv.view, a.m.mapv.markers[0])
	}

	// Enter lists the events at the place.
	a.press("0", "enter")
	if a.m.active != viewPlaces || a.m.places.detail == nil || a.m.places.detail.Name != "York" {
		t.Fatalf("enter opens the place: %v %v", a.m.active, a.m.places.detail)
	}
	a.contains("Events in York, Yorkshire, England")
}

func TestPlaceRecords(t *testing.T) {
	a := newMusterApp(t)
	a.resize(160, 40)
	a.press("alt+5", "f")
	a.typ("tempelhof")
	a.press("enter", "down", "down") // Deutschland, Berlin, Tempelhof
	if sel := a.m.selectedPlace(); sel == nil || sel.Name != "Tempelhof" {
		t.Fatalf("selected %v", sel)
	}
	a.press("enter")
	a.contains("Events in Tempelhof, Berlin, Deutschland · 5 events · 5 people",
		"also written as Tempelhof, amerikanischer Sektor, Berlin (West), Deutschland · Tempelhof, Berlin (West), Deutschland",
		"Stadtbezirk\n", "Birth              Erika Gabler")
	a.press("esc", "esc", "f")
	a.typ("brosowo")
	a.press("enter", "down", "down", "down", "down", "enter")
	a.contains("names: Brzozowo (Polish) · 39 Ort · 53.32°N 18.42°E · GOV https://gov.genealogy.net/item/show/BROOWOJO93FH")
}

func TestPlacesToMap(t *testing.T) {
	a := newApp(t, Options{})
	a.resize(120, 36)
	a.press("alt+5", "down", "down") // Canada, Ontario, Toronto
	a.press("M")
	if a.m.active != viewMap || a.m.mapv.places[a.m.mapv.sel].Name != "Toronto" {
		t.Fatalf("M shows Toronto on the map: %v", a.m.active)
	}
	if a.m.mapv.view.Zoom < 7 {
		t.Errorf("a single place is shown zoomed in: %v", a.m.mapv.view.Zoom)
	}
	a.press("5", "up", "up", "down", "down", "down") // England
	if a.m.selectedPlace().Name != "England" {
		t.Fatalf("selected %q", a.m.selectedPlace().Name)
	}
	a.press("M")
	if a.m.active != viewMap {
		t.Fatal("M on a region without coordinates of its own")
	}
	// The view fits the English places, which excludes Toronto and Köln.
	for i, p := range a.m.mapv.places {
		_, _, visible := a.m.mapv.view.Locate(a.m.mapv.markers[i].X, a.m.mapv.markers[i].Y, a.m.width, a.m.mapRows())
		english := strings.HasSuffix(p.Full, "England")
		if visible != english {
			t.Errorf("%s visible=%v", p.Full, visible)
		}
	}
	a.press("5")
	for range 11 { // from England down to France
		a.press("down")
	}
	if a.m.selectedPlace().Name != "France" {
		t.Fatalf("selected %q", a.m.selectedPlace().Name)
	}
	a.press("M")
	a.contains("France has no coordinates")
}

func TestLicensesScreen(t *testing.T) {
	a := newApp(t, Options{})
	a.press("esc", "alt+4") // in the search box, L would be typed
	a.press("L")
	if !a.m.showHelp || a.m.overlay != "Licenses" {
		t.Fatal("L opens the licenses")
	}
	a.contains("[Licenses]", "Map data", "Natural Earth", "public domain")
	text := ""
	for _, l := range a.m.help.lines {
		text += l.plain() + "\n"
	}
	for _, want := range []string{"Font", "Martian Mono", "SIL OPEN FONT LICENSE Version 1.1", "Third-party software", "Go standard library",
		"github.com/charmbracelet/bubbletea", "golang.org/x/text", "codeberg.org/go-pdf/fpdf", "MIT License"} {
		if !strings.Contains(text, want) {
			t.Errorf("licenses lack %q", want)
		}
	}
	a.resize(90, 30) // the overlay content survives a resize
	if a.m.overlay != "Licenses" || !strings.Contains(a.view(), "Natural Earth") {
		t.Error("resize rebuilt the wrong overlay")
	}
	a.press("x")
	if a.m.showHelp {
		t.Error("any key closes the licenses")
	}
	a.press("?")
	a.contains("[Help]")
	if !strings.Contains(helpText(), "Map\n") || !strings.Contains(helpText(), "PLAC.MAP.LATI") {
		t.Error("help explains the map")
	}
}

func TestFormatLatLon(t *testing.T) {
	tests := map[[2]float64]string{
		{53.7997, -1.5492}: "53.80°N 1.55°W",
		{-33.87, 151.21}:   "33.87°S 151.21°E",
		{0, 0}:             "0.00°N 0.00°E",
	}
	for in, want := range tests {
		if got := formatLatLon(in[0], in[1]); got != want {
			t.Errorf("formatLatLon(%v) = %q, want %q", in, got, want)
		}
	}
}

func worldmapMaxZoom() float64 { return worldmap.MaxZoom }
func worldmapMinZoom() float64 { return worldmap.MinZoom }

func TestLifespansView(t *testing.T) {
	a := newApp(t, Options{StartPerson: "I3"})
	a.resize(120, 30)
	a.press("s")
	if a.m.active != viewTree || a.m.tree.mode != modeLifespans {
		t.Fatalf("view %d, mode %d", a.m.active, a.m.tree.mode)
	}
	a.contains("[3 Charts]", "Lifespans of John Smith · parents to children · selected: John Smith (1817–1880)",
		"father     William Smith", "lived  with John", "1817–1880  63 y", "John's age",
		"John Smith · born 2 Feb 1817 in Leeds · died 1880 in Manchester at ~63", "⚭ marriage",
		"p/d/h/s pedigree/descendants/hourglass/lifespans")
	lines := strings.Split(ansi.Strip(a.view()), "\n")
	if lines[3] != "" || !strings.Contains(lines[4], "1800      1820") {
		t.Errorf("a blank line, then the axis:\n%s", a.view())
	}
	// The details of the selected person follow the ages, before the legend.
	_, ages := a.find("John's age")
	_, about := a.find("John Smith · born")
	_, legend := a.find("while John lived")
	if about != ages+2 || legend != about+1 {
		t.Errorf("ages %d, details %d, legend %d", ages, about, legend)
	}

	a.press("down")
	a.contains("selected: Mary Smith (1819–1899)", "Mary Smith · born 1819 in Leeds · died 1899 at ~80")
	a.press("up", "up")
	a.contains("selected: Elizabeth Brown", "when John Smith was ~43")

	// A click anywhere on a row selects the person, a second opens them.
	x, y := a.find("Ann Taylor")
	a.click(x+40, y)
	a.contains("selected: Ann Taylor (1820–1845)", "Ann Taylor · born 1820 · married 1840 in Manchester")
	a.click(x+40, y)
	if a.m.active != viewPerson || a.currentID() != "I5" {
		t.Fatalf("second click: view %d, %s", a.m.active, a.currentID())
	}
	a.press("b", "s")
	a.contains("Lifespans of John Smith")

	// Space makes the selected person the centre; + adds a generation each
	// way, with the aunts and uncles.
	a.clickOn("Thomas Smith")
	a.press(" ")
	a.contains("Lifespans of Thomas Smith · parents to children")
	a.press("+")
	a.contains("Lifespans of Thomas Smith · grandparents to grandchildren", "showing grandparents to grandchildren",
		"\ngrandfather ", "\naunt ", "\n    grandson ", "\n    adoptive granddaughter Rose Smith")
	a.press("-", "-")
	a.contains("generations are limited to 1–10")
	if a.m.tree.gens != 4 {
		t.Errorf("the other charts' generations changed: %d", a.m.tree.gens)
	}

	// The chart fits a narrower screen.
	a.resize(84, 30)
	for _, l := range strings.Split(a.view(), "\n") {
		if w := ansi.StringWidth(l); w > 84 {
			t.Errorf("line of %d columns: %q", w, ansi.Strip(l))
		}
	}
	a.press("down", "up")
	a.contains("p/d/h/s charts", "selected: Thomas Smith")

	// On a low screen the rows scroll between the axes.
	a.resize(110, 14)
	a.press("+")
	for range 12 {
		a.press("down")
	}
	a.contains("selected: Infant Smith", "Thomas's age", "1850", "Infant Smith · born 1895")
	a.notContains("William Smith")
	a.wheel(tea.MouseButtonWheelUp, 50, 6)
	a.wheel(tea.MouseButtonWheelUp, 50, 6)
	a.contains("Thomas's age", "1850")
	if a.m.tree.top < a.m.tree.chart.Head {
		t.Errorf("scrolled into the head: %d", a.m.tree.top)
	}

	// Loading the file again keeps the chart, its generations and the
	// selection.
	a.m.replaceDoc(loadSample(t))
	if tr := a.m.tree; tr.mode != modeLifespans || tr.lifeGens != 2 || tr.chart.Nodes[tr.sel].Ind.ID != "I24" {
		t.Errorf("after loading again: mode %d, %d generations, %s selected", tr.mode, tr.lifeGens, tr.chart.Nodes[tr.sel].Ind.ID)
	}

	// The person view opens the lifespans with s, and v goes on from them.
	a.press("alt+2", "s")
	a.contains("Lifespans of Thomas Smith")
	a.press("v")
	if a.m.tree.mode != modePedigree {
		t.Errorf("v from the lifespans: %d", a.m.tree.mode)
	}
}
