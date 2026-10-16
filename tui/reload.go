package tui

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/efi/goged/diff"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
)

// The browser watches its file. When another program has changed it, and
// the file stays the same for a moment, a dialog asks whether to load it
// again; the browser then keeps what it can of what is shown.

const watchInterval = time.Second

// fileState tells versions of the file apart.
type fileState struct {
	mod  int64 // modification time in nanoseconds
	size int64
}

// statMsg reports the state of the file; ok is false if it is missing.
type statMsg struct {
	state fileState
	ok    bool
}

// reloadedMsg brings the file loaded again, and its state before loading.
type reloadedMsg struct {
	doc   *gedcom.Document
	state fileState
	err   error
}

type watchState struct {
	off     bool      // never this session
	loaded  fileState // the version shown, or the one not loaded this time
	seen    fileState // a newer version, asked about when it stays the same
	asking  bool
	loading bool
}

const (
	choiceReload = iota
	choiceNotNow
	choiceNever
)

var reloadChoices = []string{"Yes, reload", "Not this time", "Never this session"}

func statFile(path string) statMsg {
	fi, err := os.Stat(path)
	if err != nil {
		return statMsg{}
	}
	return statMsg{state: fileState{fi.ModTime().UnixNano(), fi.Size()}, ok: true}
}

func (m *Model) watching() bool { return m.opts.Path != "" && m.opts.Load != nil && !m.watch.off }

// watchTick looks at the file after a while.
func (m *Model) watchTick() tea.Cmd {
	path := m.opts.Path
	return tea.Tick(watchInterval, func(time.Time) tea.Msg { return statFile(path) })
}

func (m *Model) fileStat(msg statMsg) tea.Cmd {
	w := &m.watch
	if !m.watching() {
		return nil
	}
	switch {
	case !msg.ok || msg.state == w.loaded:
		w.seen = fileState{}
	case w.asking || w.loading:
	case msg.state != w.seen:
		// Still being written, perhaps: wait until it stays the same.
		w.seen = msg.state
	default:
		w.asking = true
		m.ask(&dialog{
			title:   m.opts.Title + " changed",
			text:    []string{"Another program has changed " + m.opts.Title + ".", "Load it again? The views keep what they can."},
			choices: reloadChoices,
			keys:    map[string]int{"y": choiceReload, "n": choiceNotNow, "N": choiceNever},
			cancel:  choiceNotNow,
			hints:   [][2]string{{"y", "reload"}, {"n", "not this time"}, {"N", "never this session"}},
			since:   time.Now(),
			answer:  func(m *Model, choice int) tea.Cmd { return m.answer(choice) },
		})
	}
	return m.watchTick()
}

// answer carries out a choice of the dialog.
func (m *Model) answer(choice int) tea.Cmd {
	w := &m.watch
	w.asking = false
	name := m.opts.Title
	switch choice {
	case choiceReload:
		w.loading = true
		m.setStatus("loading " + name + " again …")
		load, path := m.opts.Load, m.opts.Path
		return func() tea.Msg {
			// A change while loading is noticed afterwards.
			st := statFile(path)
			doc, err := load()
			return reloadedMsg{doc, st.state, err}
		}
	case choiceNotNow:
		w.loaded = w.seen
		m.setStatus(name + " was not loaded again; a further change asks again")
	case choiceNever:
		w.off = true
		m.setStatus(name + " is not watched any more in this session")
	}
	return nil
}

func (m *Model) reloaded(msg reloadedMsg) {
	m.watch.loading = false
	m.watch.loaded = msg.state
	if msg.err != nil {
		m.setError(fmt.Sprintf("%s could not be loaded again: %v", m.opts.Title, msg.err))
		return
	}
	changes := diff.Compare(m.doc, msg.doc)
	m.replaceDoc(msg.doc)
	m.changes = &changes
	if m.showHelp {
		// An open overlay shows the file loaded now: its links lead into
		// it, and the list of changes is that of this reload.
		m.rebuildOverlay()
		if m.overlay == "Changes" {
			m.help.reset()
		}
	}
	if changes.Empty() {
		m.setStatus(fmt.Sprintf("loaded %s again: no differences", m.opts.Title))
		return
	}
	m.setStatus(fmt.Sprintf("loaded %s again · %s · D lists the changes", m.opts.Title, changes.Summary()))
}

// buildChanges lists what changed when the file was last loaded again;
// the people and families link to them.
func (m *Model) buildChanges() {
	r := m.changes
	ls := []line{
		textLine(span{text: "Changes in " + m.opts.Title + " when it was loaded again", style: m.st.name}),
		dimLine(m, r.Summary()),
	}
	symbol := func(k diff.Kind) span {
		style := m.st.dim
		switch k {
		case diff.Added:
			style = m.st.key
		case diff.Removed:
			style = m.st.err
		}
		return span{text: k.Symbol() + " ", style: style}
	}
	if len(r.Header) > 0 {
		ls = append(ls, textLine(), m.sectionLine("Header"))
		for _, d := range r.Header {
			ls = append(ls, textLine(symbol(d.Kind), span{text: d.String(), style: m.st.name.UnsetBold()}))
		}
	}
	for _, section := range []struct {
		name string
		recs []diff.Record
	}{{"People", r.People}, {"Families", r.Families}, {"Other records", r.Others}} {
		if len(section.recs) == 0 {
			continue
		}
		ls = append(ls, textLine(), m.sectionLine(section.name))
		idW := 0
		for _, rec := range section.recs {
			idW = max(idW, len(rec.ID))
		}
		for _, rec := range section.recs {
			title := rec.Title
			if rec.Tag != "INDI" && rec.Tag != "FAM" {
				title = strings.TrimSpace(rec.Noun() + " " + title)
			}
			l := textLine(symbol(rec.Kind), span{text: rec.ID, style: m.st.dim, width: idW + 2}, span{text: title, style: m.st.name})
			// What was removed is gone from the file.
			if rec.Kind != diff.Removed {
				switch {
				case rec.Person != nil:
					l.target = m.doc.Individual(rec.ID)
				case rec.Family != nil:
					if f := m.doc.Family(rec.ID); f != nil && len(f.Partners()) > 0 {
						l.target = f.Partners()[0]
					}
				}
			}
			ls = append(ls, l)
			for _, d := range rec.Details {
				ls = append(ls, dimLine(m, strings.Repeat(" ", idW+4)+d.String()))
			}
		}
	}
	m.help.set(ls)
}

// replaceDoc shows a document loaded again. It keeps the view, the people
// in the history and the marked person, the query and the filters, the
// settings of the views, and the selections where they can still be.
func (m *Model) replaceDoc(doc *gedcom.Document) {
	opts := m.opts
	opts.StartPerson = ""
	n := New(doc, opts)
	n.width, n.height = m.width, m.height
	n.layout()
	n.watch, n.wrap, n.prev = m.watch, m.wrap, m.prev
	n.showHelp, n.overlay, n.help = m.showHelp, m.overlay, m.help
	same := func(ind *gedcom.Individual) *gedcom.Individual {
		if ind == nil {
			return nil
		}
		return doc.Individual(ind.ID)
	}

	// The history, without the people who are gone.
	n.histPos = -1
	for i, ind := range m.history {
		p := same(ind)
		if p == nil {
			continue
		}
		if len(n.history) == 0 || n.history[len(n.history)-1] != p {
			n.history = append(n.history, p)
		}
		if i <= m.histPos {
			n.histPos = len(n.history) - 1
		}
	}
	if n.histPos < 0 && len(n.history) > 0 {
		n.histPos = 0
	}
	n.reference = same(m.reference)
	n.person.hideRelatives = m.person.hideRelatives
	n.tree.mode, n.tree.gens, n.tree.lifeGens = m.tree.mode, m.tree.gens, m.tree.lifeGens
	n.currentChanged()
	if n.current() != nil && m.current() != nil && n.current().ID == m.current().ID {
		n.person.doc.cursor, n.person.doc.offset = m.person.doc.cursor, m.person.doc.offset
		n.rebuildPerson()
		n.person.doc.ensureVisible(n.bodyHeight())
		if t := m.tree.chart; t != nil && n.tree.chart != nil && n.tree.mode == m.tree.mode {
			old := t.Nodes[m.tree.sel]
			i := -1
			if n.tree.mode != modeDescendants && n.tree.mode != modeLifespans && !old.Left {
				i = n.tree.chart.FindPath(old.Path)
			}
			if i < 0 {
				i = n.tree.chart.Find(same(old.Ind))
			}
			if i >= 0 {
				n.selectTreeNode(i)
			}
		}
	}

	n.setQuery(m.search.input.Value())
	n.search.cursor = m.search.cursor
	n.moveSearch(0)

	n.ev.all = m.ev.all
	n.ev.input.SetValue(m.ev.input.Value())
	if m.events != nil {
		n.ensureEvents()
		n.ev.cursor, n.ev.offset = m.ev.cursor, m.ev.offset
		n.moveEvents(0)
	}

	n.places.input.SetValue(m.places.input.Value())
	if m.places.root != nil {
		n.ensurePlaces()
		place := map[string]*genealogy.PlaceNode{}
		n.places.root.Walk(func(p *genealogy.PlaceNode) bool {
			place[p.Full] = p
			return true
		})
		for p := range m.places.collapsed {
			if q := place[p.Full]; q != nil && m.places.collapsed[p] {
				n.places.collapsed[q] = true
			}
		}
		n.refreshPlaces()
		if sel := m.selectedPlace(); sel != nil && place[sel.Full] != nil {
			n.selectPlace(place[sel.Full])
		}
		if d := m.places.detail; d != nil && place[d.Full] != nil {
			n.places.detail = place[d.Full]
			n.places.events = n.places.detail.AllEvents()
			n.places.evCursor, n.places.evOffset = m.places.evCursor, m.places.evOffset
			n.movePlaceEvents(0)
		}
	}
	if m.mapv.ready {
		n.mapv.world, n.mapv.err = m.mapv.world, m.mapv.err
		n.mapv.view.Proj = m.mapv.view.Proj
		n.ensureMap()
		n.mapv.view = m.mapv.view
		n.clampMap()
		if len(m.mapv.places) > 0 {
			sel := m.mapv.places[m.mapv.sel].Full
			for i, p := range n.mapv.places {
				if p.Full == sel {
					n.mapv.sel = i
				}
			}
		}
		// The surname view stays on the surname, if anyone still has it.
		if sn := m.mapv.sn; sn.on {
			n.ensureSurnames()
			for i, e := range n.mapv.sn.names {
				if e.name == sn.names[sn.name].name {
					n.mapv.sn.on, n.mapv.sn.name, n.mapv.sn.kind = true, i, sn.kind
					n.mapv.sn.offset = sn.offset
					n.mapv.sn.scrollList()
					n.computeSurname()
				}
			}
		}
	}

	if c := m.checks; c.results != nil {
		n.checks.params, n.checks.enabled, n.checks.folded = slices.Clone(c.params), slices.Clone(c.enabled), slices.Clone(c.folded)
		n.ensureChecks()
		if c.cursor < len(c.rows) {
			n.checks.cursor = n.checkRowOf(c.rows[c.cursor].check)
			n.moveChecks(0)
		}
	}

	e := m.export
	n.export.row, n.export.format, n.export.scope, n.export.edited = e.row, e.format, e.scope, e.edited
	if e.edited {
		n.export.path.SetValue(e.path.Value())
	}
	if e.all != nil {
		n.ensureExport()
	}

	active := m.active
	n.active = viewSearch
	n.switchTo(active)
	n.status, n.statusErr = "", false
	*m = n
}
