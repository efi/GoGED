// Package tui implements the interactive terminal user interface of goged
// using Bubble Tea.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	viewCount
)

var viewNames = [viewCount]string{"Search", "Person", "Tree", "Events", "Places", "Map", "Stats"}

// Options configure the interface.
type Options struct {
	// Title is shown in the header, typically the file name.
	Title string
	// StartPerson is the identifier of a person to open on start.
	StartPerson string
	// Generations shown in tree views (default 4).
	Generations int
	// ASCII draws trees with ASCII characters only.
	ASCII bool
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
	// reference is the marked person relationships are computed against.
	reference *gedcom.Individual

	search searchState
	person personState
	tree   treeState
	ev     eventsState
	places placesState
	mapv   mapState
	stats  statsState
}

// New creates the model for a document.
func New(doc *gedcom.Document, opts Options) Model {
	if opts.Generations < 2 {
		opts.Generations = 4
	}
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
	m.tree.gens = opts.Generations
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
	m.moveSearch(0)
	m.moveEvents(0)
	m.movePlaces(0)
	m.movePlaceEvents(0)
	m.rebuildPerson()
	m.ensureTreeVisible()
	m.rebuildOverlay()
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
	m.active = v
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

	if m.showHelp {
		switch key {
		case "up", "k":
			m.help.scroll(-1, m.bodyHeight())
		case "down", "j":
			m.help.scroll(1, m.bodyHeight())
		case "pgup", "ctrl+u":
			m.help.page(-1, m.bodyHeight())
		case "pgdown", "ctrl+d", " ":
			m.help.page(1, m.bodyHeight())
		case "q":
			return tea.Quit
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
	case "/":
		m.switchTo(viewSearch)
	case "tab":
		m.cycle(1)
	case "shift+tab":
		m.cycle(-1)
	case "1", "2", "3", "4", "5", "6", "7":
		m.switchTo(view(key[0] - '1'))
	case "alt+1", "alt+2", "alt+3", "alt+4", "alt+5", "alt+6", "alt+7":
		m.switchTo(view(key[4] - '1'))
	case "backspace", "b", "[", "alt+left":
		m.back()
	case "]", "alt+right":
		m.forward()
	case "m":
		m.mark(m.current())
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
	if m.overlay == "Licenses" {
		m.buildLicenses()
	} else {
		m.buildHelp()
	}
}

// typing reports whether keys currently go to a text input.
func (m *Model) typing() bool {
	return !m.showHelp && (m.active == viewSearch ||
		(m.active == viewEvents && m.ev.editing) ||
		(m.active == viewPlaces && m.places.editing))
}

// isViewKey reports whether a key switches views; text inputs let these
// keys through.
func isViewKey(key string) bool {
	switch key {
	case "tab", "shift+tab", "alt+1", "alt+2", "alt+3", "alt+4", "alt+5", "alt+6", "alt+7":
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
	}
	body = fixHeight(body, h)
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

func (m Model) viewHeader() string {
	info := fmt.Sprintf("%d people · %d families", len(m.doc.Individuals), len(m.doc.Families))
	if n := m.doc.Redactions; n > 0 {
		info += fmt.Sprintf(" · %d restricted hidden", n)
	}
	if m.opts.Title != "" {
		info = m.opts.Title + " · " + info
	}
	title := m.st.title.Render("goged") + " " + m.st.dim.Render(info)
	if m.reference != nil {
		title += "  " + m.st.reference.Render("★ "+m.reference.DisplayName())
	}
	var tabs []string
	for i, name := range viewNames {
		label := fmt.Sprintf("%d %s", i+1, name)
		if view(i) == m.active && !m.showHelp {
			tabs = append(tabs, m.st.tabActive.Render("["+label+"]"))
		} else {
			tabs = append(tabs, m.st.tabInactive.Render(" "+label+" "))
		}
	}
	if m.showHelp {
		tabs = append(tabs, m.st.tabActive.Render("["+m.overlay+"]"))
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
	case m.showHelp:
		hints = [][2]string{{"↑↓", "scroll"}, {"any key", "close"}}
	case m.active == viewSearch:
		hints = [][2]string{{"type", "search"}, {"↑↓", "select"}, {"enter", "open"}, {"esc", "clear/back"}, {"tab/alt+1-7", "views"}, {"?", "help"}}
	case m.active == viewPerson:
		hints = [][2]string{{"↑↓", "relatives"}, {"enter", "go"}, {"←→", "history"}, {"t/d", "trees"}, {"c", "context"}, {"m", "mark"}, {"/", "search"}, {"?", "help"}}
	case m.active == viewTree:
		hints = [][2]string{{"arrows", "move"}, {"enter", "open"}, {"space", "re-root"}, {"p/d", "pedigree/descendants"}, {"+/-", "generations"}, {"?", "help"}}
	case m.active == viewEvents && m.ev.editing:
		hints = [][2]string{{"type", "filter"}, {"enter", "done"}, {"esc", "clear"}}
	case m.active == viewEvents:
		hints = [][2]string{{"↑↓", "select"}, {"enter", "open"}, {"f", "filter"}, {"a", "all/important"}, {"?", "help"}}
	case m.active == viewPlaces && m.places.editing:
		hints = [][2]string{{"type", "filter"}, {"enter", "done"}, {"esc", "clear"}}
	case m.active == viewPlaces && m.places.detail != nil:
		hints = [][2]string{{"↑↓", "select"}, {"enter", "open person"}, {"esc", "back to places"}, {"?", "help"}}
	case m.active == viewMap:
		hints = [][2]string{{"arrows", "pan"}, {"+/-", "zoom"}, {"n/N", "next/previous place"}, {"enter", "events here"}, {"0/w", "fit places/world"}, {"L", "licenses"}, {"?", "help"}}
	case m.active == viewPlaces:
		hints = [][2]string{{"↑↓", "select"}, {"←→", "collapse/expand"}, {"enter", "events here"}, {"f", "filter"}, {"M", "map"}, {"?", "help"}}
	case m.active == viewStats:
		hints = [][2]string{{"↑↓", "surnames"}, {"enter", "search surname"}, {"?", "help"}}
	}
	var parts []string
	for _, h := range hints {
		parts = append(parts, m.st.key.Render(h[0])+" "+m.st.status.Render(h[1]))
	}
	return fit(strings.Join(parts, "  "), m.width)
}

// sectionLine renders a section heading.
func (m *Model) sectionLine(title string) line {
	return textLine(span{title, m.st.section})
}

// personSpans renders a person as name plus dimmed lifespan.
func (m *Model) personSpans(ind *gedcom.Individual) []span {
	spans := []span{{ind.DisplayName(), m.st.name}}
	if ls := ind.Lifespan(); ls != "" {
		spans = append(spans, span{" (" + ls + ")", m.st.dim})
	}
	if ind == m.reference {
		spans = append(spans, span{" ★", m.st.reference})
	}
	return spans
}

var _ tea.Model = Model{}
