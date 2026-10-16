// Package tui implements the interactive terminal user interface of goged
// using Bubble Tea.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/efi/goged/diff"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/search"
)

type view int

const (
	viewSearch view = iota
	viewPerson
	viewTree
	viewEvents
	viewPlaces
	viewMap
	viewStats
	viewChecks
	viewExport
	viewCount
)

var viewNames = [viewCount]string{"Search", "Person", "Charts", "Events", "Places", "Map", "Stats", "Checks", "Export"}

// Options configure the interface.
type Options struct {
	// Title is shown in the header, typically the file name.
	Title string
	// StartPerson is the identifier of a person to open on start.
	StartPerson string
	// Generations shown in the charts (default 4), from 2 to 20.
	Generations int
	// ASCII draws trees with ASCII characters only.
	ASCII bool
	// NoMouse leaves the mouse to the terminal, which can then select
	// text as usual.
	NoMouse bool
	// Path is the file shown. With Load, which reads it again, the browser
	// watches it and offers to reload it when another program changes it.
	Path string
	Load func() (*gedcom.Document, error)
	// Today is the date used for "On this day"; zero means the current date.
	Today time.Time
}

// Model is the Bubble Tea model of the application.
type Model struct {
	doc    *gedcom.Document
	index  *search.Index
	events *search.EventIndex
	opts   Options
	st     styles

	width, height int
	active        view
	prev          view // view to return to when leaving search with esc
	showHelp      bool
	overlay       string // title of the overlay shown when showHelp is set
	help          docView
	status        string
	statusErr     bool

	// Browser-like history of visited people.
	history []*gedcom.Individual
	histPos int
	// changes are the differences found when the file was last loaded
	// again; nil before.
	changes *diff.Result

	// reference is the marked person relationships are computed against.
	reference *gedcom.Individual
	// wrap shows long lines of the person view on several rows instead of
	// cutting them (W).
	wrap bool

	search searchState
	person personState
	tree   treeState
	ev     eventsState
	places placesState
	mapv   mapState
	stats  statsState
	checks checksState
	export exportState

	watch watchState
	// dlg is the dialog shown, if any.
	dlg *dialog
	// compare holds the people of the comparison overlay (M).
	compare compareState
}

// New creates the model for a document.
func New(doc *gedcom.Document, opts Options) Model {
	// Generations below 1 select the default; others are kept within the
	// limits that + and - keep to.
	if opts.Generations < 1 {
		opts.Generations = 4
	}
	opts.Generations = clamp(opts.Generations, minGenerations, maxPedigreeGenerations)
	m := Model{
		doc:     doc,
		index:   search.NewIndex(doc),
		opts:    opts,
		st:      defaultStyles(),
		width:   80,
		height:  24,
		histPos: -1,
	}
	m.search = newSearchState()
	m.ev = newEventsState()
	m.places = newPlacesState()
	m.export = newExportState()
	m.tree.gens, m.tree.lifeGens = opts.Generations, 1
	if opts.Path != "" {
		m.watch.loaded = statFile(opts.Path).state
	}
	m.runSearch()
	m.active = viewSearch
	if opts.StartPerson != "" {
		if ind := doc.Individual(opts.StartPerson); ind != nil {
			m.visit(ind)
			m.active = viewPerson
		} else {
			m.setError(fmt.Sprintf("no individual with ID %q", opts.StartPerson))
		}
	}
	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	title := "goged"
	if m.opts.Title != "" {
		title += " — " + m.opts.Title
	}
	if m.watching() {
		return tea.Batch(tea.SetWindowTitle(title), m.watchTick())
	}
	return tea.SetWindowTitle(title)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.update(msg)
	return m, cmd
}

func (m *Model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case statMsg:
		return m.fileStat(msg)
	case reloadedMsg:
		m.reloaded(msg)
	}
	return nil
}

// bodyHeight is the number of lines available between header and footer.
func (m *Model) bodyHeight() int { return max(1, m.height-3) }

// layout recomputes size dependent state.
func (m *Model) layout() {
	m.search.input.Width = max(10, m.width-12)
	m.ev.input.Width = max(10, m.width-12)
	m.places.input.Width = max(10, m.width-12)
	m.export.path.Width = max(10, m.width-18)
	m.moveSearch(0)
	m.moveEvents(0)
	m.movePlaces(0)
	m.movePlaceEvents(0)
	m.rebuildPerson()
	if m.mapv.ready {
		m.clampMap()
	}
	if t := &m.tree; t.mode == modeLifespans && t.chart != nil {
		// The lifespans fit the width.
		sel := t.chart.Nodes[t.sel].Ind
		m.rebuildTree()
		m.selectTreeNode(t.chart.Find(sel))
	}
	m.ensureTreeVisible()
	m.rebuildOverlay()
	if m.stats.computed {
		m.buildStats()
	}
}

func (m *Model) setStatus(s string) { m.status, m.statusErr = s, false }
func (m *Model) setError(s string)  { m.status, m.statusErr = s, true }

// current returns the person at the current history position.
func (m *Model) current() *gedcom.Individual {
	if m.histPos < 0 || m.histPos >= len(m.history) {
		return nil
	}
	return m.history[m.histPos]
}

// visit makes ind the current person, recording it in the history.
func (m *Model) visit(ind *gedcom.Individual) {
	if ind == nil || ind == m.current() {
		return
	}
	m.history = append(m.history[:m.histPos+1], ind)
	m.histPos = len(m.history) - 1
	m.currentChanged()
}

func (m *Model) back() {
	if m.histPos <= 0 {
		m.setStatus("no earlier person in history")
		return
	}
	m.histPos--
	m.currentChanged()
}

func (m *Model) forward() {
	if m.histPos >= len(m.history)-1 {
		m.setStatus("no later person in history")
		return
	}
	m.histPos++
	m.currentChanged()
}

func (m *Model) currentChanged() {
	m.person.doc.reset()
	m.rebuildPerson()
	m.rebuildTree()
}

// open shows a person in the person view.
func (m *Model) open(ind *gedcom.Individual) {
	if ind == nil {
		return
	}
	m.visit(ind)
	m.switchTo(viewPerson)
}

func (m *Model) switchTo(v view) {
	if v == m.active {
		return
	}
	if (v == viewPerson || v == viewTree) && m.current() == nil {
		m.setStatus("select a person first: search and press enter")
		return
	}
	if m.active == viewSearch {
		// Leaving the search uses the query.
		m.search.hist.add(m.search.input.Value())
	}
	if v == viewSearch {
		m.prev = m.active
		m.search.input.Focus()
	}
	if v == viewStats {
		m.ensureStats()
	}
	if v == viewEvents {
		m.ensureEvents()
	}
	if v == viewPlaces {
		m.ensurePlaces()
	}
	if v == viewMap {
		m.ensureMap()
	}
	if v == viewChecks {
		m.ensureChecks()
	}
	if v == viewExport {
		m.ensureExport()
	}
	m.active = v
}

// selected returns the person selected in the view shown: a search
// result, the person of an event, a person in a chart, a line of the
// statistics or the person of a finding; else the current person.
func (m *Model) selected() *gedcom.Individual {
	var ind *gedcom.Individual
	switch m.active {
	case viewSearch:
		if s := &m.search; s.cursor < len(s.results) {
			ind = s.results[s.cursor].Individual
		}
	case viewTree:
		if t := &m.tree; t.chart != nil && t.sel < len(t.chart.Nodes) {
			ind = t.chart.Nodes[t.sel].Ind
		}
	case viewEvents:
		if e := &m.ev; e.cursor < len(e.list) {
			ind = eventPerson(e.list[e.cursor])
		}
	case viewPlaces:
		if p := &m.places; p.detail != nil && p.evCursor < len(p.events) {
			ind = eventPerson(p.events[p.evCursor])
		}
	case viewStats:
		if d := &m.stats.doc; d.selected() != nil && d.visible(d.selectedLine(), m.bodyHeight()) {
			ind = d.selected().target
		}
	case viewChecks:
		if f := m.finding(); f != nil {
			ind = f.Person
		}
	}
	if ind == nil {
		return m.current()
	}
	return ind
}

func (m *Model) mark(ind *gedcom.Individual) {
	if ind == nil {
		return
	}
	if m.reference == ind {
		m.reference = nil
		m.setStatus("reference person cleared")
	} else {
		m.reference = ind
		m.setStatus(fmt.Sprintf("marked %s as reference: relationships are shown relative to them", ind.DisplayName()))
	}
	m.rebuildPerson()
	m.markChanged()
	if m.active == viewExport {
		m.ensureExport()
	}
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	// Several characters can arrive in one message when keys are typed
	// quickly or sent by a script. Outside text inputs, treat them as
	// individual key presses.
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste && !m.typing() {
		var cmds []tea.Cmd
		for _, r := range msg.Runes {
			cmds = append(cmds, m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: msg.Alt}))
		}
		return tea.Batch(cmds...)
	}
	key := msg.String()
	if key == "ctrl+c" {
		return tea.Quit
	}
	m.status, m.statusErr = "", false

	if m.dlg != nil {
		return m.updateDialog(msg)
	}
	if m.showHelp {
		// The overlay scrolls; the list of changes moves between its
		// people and families, and enter opens one.
		switch key {
		case "up", "k":
			m.help.move(-1, m.bodyHeight())
		case "down", "j":
			m.help.move(1, m.bodyHeight())
		case "pgup", "ctrl+u":
			m.help.page(-1, m.bodyHeight())
		case "pgdown", "ctrl+d", " ":
			m.help.page(1, m.bodyHeight())
		case "q":
			return tea.Quit
		case "enter":
			m.showHelp = false
			if sel := m.help.selected(); sel != nil && m.help.visible(m.help.selectedLine(), m.bodyHeight()) {
				switch {
				case sel.target != nil:
					m.open(sel.target)
				case sel.place != "":
					m.showPlace(sel.place)
				}
			}
		default:
			m.showHelp = false
		}
		return nil
	}

	// Views with an active text input get the keys first.
	switch {
	case m.active == viewSearch:
		if handled, cmd := m.updateSearch(msg); handled {
			return cmd
		}
	case m.active == viewEvents && m.ev.editing:
		if handled, cmd := m.updateEventsFilter(msg); handled {
			return cmd
		}
	case m.active == viewPlaces && m.places.editing:
		if handled, cmd := m.updatePlacesFilter(msg); handled {
			return cmd
		}
	case m.active == viewExport && m.export.editing:
		if handled, cmd := m.updateExportPath(msg); handled {
			return cmd
		}
	}

	// View specific keys.
	var handled bool
	var cmd tea.Cmd
	switch m.active {
	case viewPerson:
		handled, cmd = m.updatePerson(msg)
	case viewTree:
		handled, cmd = m.updateTree(msg)
	case viewEvents:
		handled, cmd = m.updateEvents(msg)
	case viewPlaces:
		handled, cmd = m.updatePlaces(msg)
	case viewMap:
		handled, cmd = m.updateMap(msg)
	case viewStats:
		handled, cmd = m.updateStats(msg)
	case viewChecks:
		handled, cmd = m.updateChecks(msg)
	case viewExport:
		handled, cmd = m.updateExport(msg)
	}
	if handled {
		return cmd
	}

	// Global keys.
	switch key {
	case "q":
		return tea.Quit
	case "?", "f1":
		m.openOverlay("Help")
	case "L":
		m.openOverlay("Licenses")
	case "D":
		if m.changes == nil {
			m.setStatus("D lists what changed when the file has been loaded again")
			break
		}
		m.openOverlay("Changes")
	case "/":
		m.switchTo(viewSearch)
	case "tab":
		m.cycle(1)
	case "shift+tab":
		m.cycle(-1)
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		m.switchTo(view(key[0] - '1'))
	case "alt+1", "alt+2", "alt+3", "alt+4", "alt+5", "alt+6", "alt+7", "alt+8", "alt+9":
		m.switchTo(view(key[4] - '1'))
	case "backspace", "b", "[", "alt+left":
		m.back()
	case "]", "alt+right":
		m.forward()
	case "m":
		m.mark(m.selected())
	case "C":
		m.compareSelected()
	case "M":
		m.mapPerson(m.selected())
	case "W":
		m.wrap = !m.wrap
		m.rebuildPerson()
		m.person.doc.ensureVisible(m.bodyHeight())
		if m.wrap {
			m.setStatus("long lines in the person view are wrapped (W to cut them)")
		} else {
			m.setStatus("long lines in the person view are cut (W to wrap them)")
		}
	case "i":
		m.switchTo(viewPerson)
	}
	return nil
}

// openOverlay shows the help or the licenses over the current view.
func (m *Model) openOverlay(title string) {
	m.showHelp = true
	m.overlay = title
	m.rebuildOverlay()
	m.help.reset()
}

// rebuildOverlay regenerates the overlay content, e.g. after a resize.
func (m *Model) rebuildOverlay() {
	switch m.overlay {
	case "Licenses":
		m.buildLicenses()
	case "Changes":
		m.buildChanges()
	case "Compare":
		m.buildCompare()
	default:
		m.buildHelp()
	}
}

// typing reports whether keys currently go to a text input.
func (m *Model) typing() bool {
	return !m.showHelp && (m.active == viewSearch ||
		(m.active == viewEvents && m.ev.editing) ||
		(m.active == viewPlaces && m.places.editing) ||
		(m.active == viewExport && m.export.editing))
}

// isViewKey reports whether a key switches views; text inputs let these
// keys through.
func isViewKey(key string) bool {
	switch key {
	case "tab", "shift+tab", "alt+1", "alt+2", "alt+3", "alt+4", "alt+5", "alt+6", "alt+7", "alt+8", "alt+9":
		return true
	}
	return false
}

// cycle moves to the next or previous view, skipping views that need a
// current person when there is none.
func (m *Model) cycle(dir int) {
	v := m.active
	for range viewCount {
		v = (v + view(dir) + viewCount) % viewCount
		if (v == viewPerson || v == viewTree) && m.current() == nil {
			continue
		}
		break
	}
	m.switchTo(v)
}

// View implements tea.Model.
func (m Model) View() string {
	header := m.viewHeader()
	var body string
	h := m.bodyHeight()
	switch {
	case m.showHelp:
		body = m.help.render(m.width, h, m.st)
	case m.active == viewSearch:
		body = m.viewSearch(h)
	case m.active == viewPerson:
		body = m.viewPerson(h)
	case m.active == viewTree:
		body = m.viewTree(h)
	case m.active == viewEvents:
		body = m.viewEvents(h)
	case m.active == viewPlaces:
		body = m.viewPlaces(h)
	case m.active == viewMap:
		body = m.viewMap(h)
	case m.active == viewStats:
		body = m.viewStats(h)
	case m.active == viewChecks:
		body = m.viewChecks(h)
	case m.active == viewExport:
		body = m.viewExport(h)
	}
	body = fixHeight(body, h)
	if m.dlg != nil {
		d := (&m).layoutDialog()
		body = overlay(body, d.lines, d.x, d.y)
	}
	return header + "\n" + body + "\n" + m.viewFooter()
}

// fixHeight pads or cuts a block to exactly h lines.
func fixHeight(s string, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// headerInfo is the text after the program name in the first line.
func (m *Model) headerInfo() string {
	info := fmt.Sprintf("%d people · %d families", len(m.doc.Individuals), len(m.doc.Families))
	if n := m.doc.Redactions; n > 0 {
		info += fmt.Sprintf(" · %d restricted hidden", n)
	}
	if m.opts.Title != "" {
		info = m.opts.Title + " · " + info
	}
	return info
}

// tab is a label in the tab bar: a view, or the open overlay (view -1).
type tab struct {
	label  string
	view   view
	active bool
}

// tabs lists the tab bar. An open overlay comes first, so that it shows
// however narrow the screen is.
func (m *Model) tabs() []tab {
	var tabs []tab
	if m.showHelp {
		tabs = append(tabs, tab{m.overlay, -1, true})
	}
	for i, name := range viewNames {
		tabs = append(tabs, tab{fmt.Sprintf("%d %s", i+1, name), view(i), view(i) == m.active && !m.showHelp})
	}
	return tabs
}

func (m *Model) renderTab(t tab) string {
	if t.active {
		return m.st.tabActive.Render("[" + t.label + "]")
	}
	return m.st.tabInactive.Render(" " + t.label + " ")
}

// title is the first line up to the marked person.
func (m *Model) title() string {
	return m.st.title.Render("goged") + " " + m.st.dim.Render(m.headerInfo())
}

func (m Model) viewHeader() string {
	title := m.title()
	if m.reference != nil {
		title += "  " + m.st.reference.Render("★ "+m.reference.DisplayName())
	}
	var tabs []string
	for _, t := range m.tabs() {
		tabs = append(tabs, m.renderTab(t))
	}
	return fit(title, m.width) + "\n" + fit(strings.Join(tabs, " "), m.width)
}

func (m Model) viewFooter() string {
	if m.status != "" {
		if m.statusErr {
			return fit(m.st.err.Render(m.status), m.width)
		}
		return fit(m.st.status.Render(m.status), m.width)
	}
	var hints [][2]string
	switch {
	case m.dlg != nil && m.dlg.input != nil:
		hints = m.dlg.hints
	case m.dlg != nil:
		hints = append([][2]string{{"←→", "choose"}, {"enter", "confirm"}}, m.dlg.hints...)
	case m.showHelp && (m.overlay == "Changes" || m.overlay == "Compare"):
		hints = [][2]string{{"↑↓", "select"}, {"enter", "open"}, {"any other key", "close"}}
	case m.showHelp:
		hints = [][2]string{{"↑↓", "scroll"}, {"any key", "close"}}
	case m.active == viewSearch:
		hints = [][2]string{{"type", "search"}, {"↑↓", "select"}, {"enter", "open"}, {"esc", "clear/back"}, {"alt+↑↓", "history"}, {"tab/alt+1-9", "views"}, {"?", "help"}}
	case m.active == viewPerson:
		hints = [][2]string{{"↑↓", "select"}, {"enter", "go"}, {"←→", "history"}, {"t/d/h/s", "charts"}, {"c", "context"}, {"m/C/M", "mark/compare/map"}, {"/", "search"}, {"?", "help"}}
	case m.active == viewTree:
		charts := "pedigree/descendants/hourglass/lifespans"
		if m.width < 120 {
			charts = "charts"
		}
		hints = [][2]string{{"arrows", "move"}, {"enter", "open"}, {"space", "re-root"}, {"p/d/h/s", charts}, {"+/-", "generations"}, {"P", "print"}, {"?", "help"}}
	case m.active == viewEvents && m.ev.editing:
		hints = [][2]string{{"type", "filter"}, {"enter", "done"}, {"esc", "clear"}, {"alt+↑↓", "history"}}
	case m.active == viewEvents:
		hints = [][2]string{{"↑↓", "select"}, {"enter", "open"}, {"f", "filter"}, {"a", "all/important"}, {"M", "map"}, {"?", "help"}}
	case m.active == viewPlaces && m.places.editing:
		hints = [][2]string{{"type", "filter"}, {"enter", "done"}, {"esc", "clear"}, {"alt+↑↓", "history"}}
	case m.active == viewPlaces && m.places.detail != nil:
		hints = [][2]string{{"↑↓", "select"}, {"enter", "open person"}, {"esc", "back to places"}, {"M", "map"}, {"?", "help"}}
	case m.active == viewMap:
		hints = [][2]string{{"arrows", "pan"}, {"+/-", "zoom"}, {"n/N", "next/previous place"}, {"enter", "events here"}, {"s", "surnames"}, {"v", "projection"}, {"0/w", "fit places/world"}, {"?", "help"}}
		if m.mapv.pm.on {
			hints = personHints
		}
		if m.mapv.sn.on {
			hints = [][2]string{{"s", "all places"}, {"PgUp/PgDn", "surname"}, {"e", "events"}, {"n/N", "places"}, {"enter", "events here"}, {"arrows +/-", "pan, zoom"}, {"?", "help"}}
		}
	case m.active == viewPlaces:
		hints = [][2]string{{"↑↓", "select"}, {"←→", "collapse/expand"}, {"enter", "events here"}, {"f", "filter"}, {"M", "map"}, {"?", "help"}}
	case m.active == viewStats:
		hints = [][2]string{{"↑↓", "surnames, births"}, {"enter", "list those people"}, {"?", "help"}}
	case m.active == viewChecks:
		hints = [][2]string{{"↑↓", "select"}, {"←→", "threshold"}, {"space", "on/off"}, {"enter", "fold/open"}, {"o", "other"}, {"C", "compare"}, {"M", "map"}, {"r", "reset"}, {"?", "help"}}
	case m.active == viewExport && m.export.editing:
		hints = [][2]string{{"type", "file name"}, {"enter", "done"}, {"esc", "cancel"}}
	case m.active == viewExport:
		hints = [][2]string{{"↑↓", "choose"}, {"←→", "change"}, {"enter", "edit/export"}, {"m", "mark"}, {"?", "help"}}
	}
	var parts []string
	for _, h := range hints {
		parts = append(parts, m.st.key.Render(h[0])+" "+m.st.status.Render(h[1]))
	}
	return fit(strings.Join(parts, "  "), m.width)
}

// sectionLine renders a section heading.
func (m *Model) sectionLine(title string) line {
	l := textLine(span{text: title, style: m.st.section})
	l.heading = true
	return l
}

// personSpans renders a person as name plus dimmed lifespan.
func (m *Model) personSpans(ind *gedcom.Individual) []span {
	spans := []span{{text: ind.DisplayName(), style: m.st.name}}
	if ls := ind.Lifespan(); ls != "" {
		spans = append(spans, span{text: " (" + ls + ")", style: m.st.dim})
	}
	if ind == m.reference {
		spans = append(spans, span{text: " ★", style: m.st.reference})
	}
	return spans
}

var _ tea.Model = Model{}
