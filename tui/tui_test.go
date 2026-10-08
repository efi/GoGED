package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/efi/goged/gedcom"
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

func TestPersonViewDetails(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I21")
	a.contains("Father        Arthur Smith (1866–1940)  (adopted)", "Brother       Harold Smith", "Sibling       Infant Smith")
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

func TestTreeView(t *testing.T) {
	a := newApp(t, Options{})
	a.openPerson("I20")
	a.press("t")
	if a.m.active != viewTree || a.m.tree.mode != modePedigree {
		t.Fatalf("active=%v mode=%v", a.m.active, a.m.tree.mode)
	}
	a.contains("[3 Tree]", "Pedigree of Harold Smith · 4 generations · selected: Harold Smith (1891–)", "┌─ Arthur Smith (1866–1940)", "└─ Lucy King")
	a.press("right")
	a.contains("selected: Arthur Smith (1866–1940)")
	a.press("right", "down")
	a.contains("selected: Ann Taylor (1820–1845)")
	a.press("left", "left")
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
	if a.m.tree.mode != modePedigree {
		t.Error("v toggles the tree mode")
	}
	a.press("p")
	a.contains("Pedigree of Arthur Smith")
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
	if a.m.tree.gens != maxGenerations {
		t.Errorf("gens = %d", a.m.tree.gens)
	}
	a.contains("generations are limited to 2–12")
	for range 20 {
		a.press("-")
	}
	if a.m.tree.gens != minGenerations {
		t.Errorf("gens = %d", a.m.tree.gens)
	}
	a.press("m")
	a.contains("marked Arthur Smith as reference")
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
	a.press("alt+6")
	a.contains("[6 Stats]", "Encoding", "UTF-8", "Individuals", "24  (10 male, 13 female, 1 other/unknown)", "Generations", "Longest lived",
		"Mary Smith, ~80 years", "Most common surnames", "Smith")
	a.press("end")
	a.contains("Warnings (0)", "none — the file looks consistent", "Most common given names")
	a.press("home", "enter")
	if a.currentID() != "I4" || a.m.active != viewPerson {
		t.Errorf("longest lived link should open Mary, got %s", a.currentID())
	}
	a.press("6", "home", "down", "enter")
	if a.m.active != viewSearch || a.m.search.input.Value() != `surname:"Smith"` {
		t.Fatalf("active=%v query=%q", a.m.active, a.m.search.input.Value())
	}
	a.contains("11 matches") // includes Jane Doe, née Smith by marriage
	a.press("tab")           // -> person (a current person exists)
	a.press("tab", "tab", "tab", "tab")
	a.press("pgdown", "pgup", "up", "down")
	if a.m.active != viewStats {
		t.Errorf("active = %v", a.m.active)
	}
}

func TestStatsWarnings(t *testing.T) {
	doc, err := gedcom.ParseString("0 HEAD\n0 @I1@ INDI\n1 NAME A /B/\n1 FAMS @F9@\n")
	if err != nil {
		t.Fatal(err)
	}
	m := New(doc, Options{})
	a := &app{t: t, m: m}
	a.resize(120, 40)
	a.press("alt+6")
	a.contains("Warnings (2)", "refers to missing family @F9@", "missing TRLR")
}

func TestHelp(t *testing.T) {
	a := newApp(t, Options{})
	a.press("?")
	if !a.m.showHelp {
		t.Fatal("? opens help")
	}
	a.contains("[Help]", "Global")
	a.press("down", "pgdown", "pgdown", "pgdown")
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
	if a.m.active != viewStats {
		t.Errorf("active = %v", a.m.active)
	}
	a.press("tab")
	if a.m.active != viewSearch {
		t.Errorf("active = %v", a.m.active)
	}
	a.press("shift+tab")
	if a.m.active != viewStats {
		t.Errorf("shift+tab -> %v", a.m.active)
	}
	a.press("2")
	a.contains("select a person first")
	a.press("3")
	if a.m.active != viewStats {
		t.Error("tree needs a person")
	}
	a.openPerson("I3")
	var seq []view
	for range 6 {
		a.press("tab")
		seq = append(seq, a.m.active)
	}
	want := []view{viewTree, viewEvents, viewPlaces, viewStats, viewSearch, viewPerson}
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
		for _, keys := range [][]string{{"1"}, {"2"}, {"3"}, {"d"}, {"4"}, {"5"}, {"enter"}, {"6"}, {"?"}} {
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
			if keys[0] == "?" {
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
	a.press("enter", "left", "right", "-", "+", "down")
	a.press("6")
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
	if a.m.places.editing || a.m.active != viewStats {
		t.Errorf("tab leaves the filter: editing=%v active=%v", a.m.places.editing, a.m.active)
	}
}
