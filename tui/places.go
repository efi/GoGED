package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
	"github.com/efi/goged/search"
)

// placesState drives the places view: a collapsible tree of jurisdictions
// and, after pressing enter, the list of events at the selected place.
type placesState struct {
	root      *genealogy.PlaceNode
	folded    map[*genealogy.PlaceNode]string // folded names for filtering
	collapsed map[*genealogy.PlaceNode]bool
	input     textinput.Model
	editing   bool
	rows      []*genealogy.PlaceNode // visible rows
	cursor    int
	offset    int

	// Events of the selected place, shown instead of the tree when set.
	detail   *genealogy.PlaceNode
	events   []*gedcom.Event
	evCursor int
	evOffset int
}

func newPlacesState() placesState {
	return placesState{
		collapsed: map[*genealogy.PlaceNode]bool{},
		input:     newTextInput("Filter: ", "part of a place name, e.g. leeds"),
	}
}

// ensurePlaces builds the place tree the first time the view is shown.
func (m *Model) ensurePlaces() {
	p := &m.places
	if p.root != nil {
		return
	}
	p.root = genealogy.Places(m.doc)
	p.folded = map[*genealogy.PlaceNode]string{}
	p.root.Walk(func(n *genealogy.PlaceNode) bool {
		p.folded[n] = search.Fold(n.Name)
		return true
	})
	m.refreshPlaces()
}

// refreshPlaces recomputes the visible rows from the filter and the
// collapsed state, keeping the selected place selected if it stays visible.
func (m *Model) refreshPlaces() {
	p := &m.places
	if p.root == nil {
		return
	}
	var selected *genealogy.PlaceNode
	if p.cursor < len(p.rows) {
		selected = p.rows[p.cursor]
	}
	p.rows = p.rows[:0]
	needle := search.Fold(strings.TrimSpace(p.input.Value()))
	if needle == "" {
		var add func(n *genealogy.PlaceNode)
		add = func(n *genealogy.PlaceNode) {
			for _, c := range n.Children {
				p.rows = append(p.rows, c)
				if !p.collapsed[c] {
					add(c)
				}
			}
		}
		add(p.root)
	} else {
		// Show matching places with their surrounding jurisdictions and
		// everything inside them, regardless of the collapsed state.
		hit := map[*genealogy.PlaceNode]bool{}
		var mark func(n *genealogy.PlaceNode) bool
		mark = func(n *genealogy.PlaceNode) bool {
			found := n.Depth > 0 && strings.Contains(p.folded[n], needle)
			for _, c := range n.Children {
				if mark(c) {
					found = true
				}
			}
			hit[n] = found
			return found
		}
		mark(p.root)
		var add func(n *genealogy.PlaceNode, inside bool)
		add = func(n *genealogy.PlaceNode, inside bool) {
			for _, c := range n.Children {
				if !inside && !hit[c] {
					continue
				}
				p.rows = append(p.rows, c)
				add(c, inside || strings.Contains(p.folded[c], needle))
			}
		}
		add(p.root, false)
	}
	p.cursor = 0
	for i, r := range p.rows {
		if r == selected {
			p.cursor = i
		}
	}
	m.movePlaces(0)
}

func (m *Model) placesListHeight() int { return max(1, m.bodyHeight()-2) }

func (m *Model) movePlaces(delta int) {
	p := &m.places
	h := m.placesListHeight()
	p.cursor = clamp(p.cursor+delta, 0, len(p.rows)-1)
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+h {
		p.offset = p.cursor - h + 1
	}
	p.offset = clamp(p.offset, 0, max(0, len(p.rows)-h))
}

func (m *Model) movePlaceEvents(delta int) {
	p := &m.places
	h := m.placesListHeight()
	p.evCursor = clamp(p.evCursor+delta, 0, len(p.events)-1)
	if p.evCursor < p.evOffset {
		p.evOffset = p.evCursor
	}
	if p.evCursor >= p.evOffset+h {
		p.evOffset = p.evCursor - h + 1
	}
	p.evOffset = clamp(p.evOffset, 0, max(0, len(p.events)-h))
}

func (m *Model) selectedPlace() *genealogy.PlaceNode {
	p := &m.places
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return nil
	}
	return p.rows[p.cursor]
}

// selectPlace moves the cursor to a visible place.
func (m *Model) selectPlace(n *genealogy.PlaceNode) {
	for i, r := range m.places.rows {
		if r == n {
			m.places.cursor = i
			m.movePlaces(0)
			return
		}
	}
}

// filtering reports whether a place filter is active; collapsing is
// disabled while filtering.
func (m *Model) filteringPlaces() bool { return strings.TrimSpace(m.places.input.Value()) != "" }

func (m *Model) setAllCollapsed(collapsed bool) {
	p := &m.places
	sel := m.selectedPlace()
	p.root.Walk(func(n *genealogy.PlaceNode) bool {
		if n.Depth > 0 && len(n.Children) > 0 {
			p.collapsed[n] = collapsed
		}
		return true
	})
	// Keep the selection on the jurisdiction that now contains it.
	for sel != nil && collapsed && sel.Depth > 1 {
		sel = sel.Parent
	}
	m.refreshPlaces()
	if sel != nil {
		m.selectPlace(sel)
	}
}

func (m *Model) updatePlacesFilter(msg tea.KeyMsg) (bool, tea.Cmd) {
	p := &m.places
	switch key := msg.String(); {
	case key == "enter":
		p.editing = false
		p.input.Blur()
	case key == "esc":
		p.input.SetValue("")
		p.editing = false
		p.input.Blur()
		m.refreshPlaces()
	case key == "up":
		m.movePlaces(-1)
	case key == "down":
		m.movePlaces(1)
	case isViewKey(key):
		p.editing = false
		p.input.Blur()
		return false, nil
	default:
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		m.refreshPlaces()
		return true, cmd
	}
	return true, nil
}

func (m *Model) updatePlaces(msg tea.KeyMsg) (bool, tea.Cmd) {
	p := &m.places
	h := m.placesListHeight()
	key := msg.String()

	if p.detail != nil {
		switch key {
		case "up", "k":
			m.movePlaceEvents(-1)
		case "down", "j":
			m.movePlaceEvents(1)
		case "pgup", "ctrl+u":
			m.movePlaceEvents(-h)
		case "pgdown", "ctrl+d", " ":
			m.movePlaceEvents(h)
		case "home", "g":
			m.movePlaceEvents(-len(p.events))
		case "end", "G":
			m.movePlaceEvents(len(p.events))
		case "enter":
			if len(p.events) > 0 {
				m.open(eventPerson(p.events[p.evCursor]))
			}
		case "esc", "left", "h":
			p.detail, p.events = nil, nil
		default:
			return false, nil
		}
		return true, nil
	}

	sel := m.selectedPlace()
	switch key {
	case "up", "k":
		m.movePlaces(-1)
	case "down", "j":
		m.movePlaces(1)
	case "pgup", "ctrl+u":
		m.movePlaces(-h)
	case "pgdown", "ctrl+d", " ":
		m.movePlaces(h)
	case "home", "g":
		m.movePlaces(-len(p.rows))
	case "end", "G":
		m.movePlaces(len(p.rows))
	case "left", "h":
		switch {
		case sel == nil:
		case len(sel.Children) > 0 && !p.collapsed[sel] && !m.filteringPlaces():
			p.collapsed[sel] = true
			m.refreshPlaces()
		case sel.Depth > 1:
			m.selectPlace(sel.Parent)
		}
	case "right", "l":
		switch {
		case sel == nil || len(sel.Children) == 0:
		case p.collapsed[sel] && !m.filteringPlaces():
			delete(p.collapsed, sel)
			m.refreshPlaces()
		default:
			m.selectPlace(sel.Children[0])
		}
	case "-":
		m.setAllCollapsed(true)
	case "+", "=":
		m.setAllCollapsed(false)
	case "enter":
		if sel != nil {
			p.detail = sel
			p.events = sel.AllEvents()
			p.evCursor, p.evOffset = 0, 0
		}
	case "f", "e":
		p.editing = true
		return true, p.input.Focus()
	case "M":
		if sel != nil {
			m.switchTo(viewMap)
			m.selectMapPlace(sel)
		}
	case "x", "esc":
		if p.input.Value() == "" {
			return false, nil
		}
		p.input.SetValue("")
		m.refreshPlaces()
	default:
		return false, nil
	}
	return true, nil
}

func (m Model) viewPlaces(h int) string {
	p := &m.places
	if p.root == nil {
		return ""
	}
	var out []string
	if p.detail != nil {
		n := p.detail
		header := m.st.section.Render("Events in "+n.Full) +
			m.st.dim.Render(" · "+plural(n.Count(), "event", "events")+" · "+plural(n.People(), "person", "people"))
		out = append(out, fit(header, m.width))
		out = append(out, fit(m.st.dim.Render("  esc returns to the places · enter opens the person"), m.width))
		out = append(out, m.eventRows(p.events, p.evCursor, p.evOffset, h-len(out))...)
		return strings.Join(out, "\n")
	}

	if p.editing || p.input.Value() != "" {
		out = append(out, fit(p.input.View(), m.width))
	} else {
		out = append(out, fit(m.st.dim.Render("  press f to filter · enter lists the events at a place · ←/→ collapse and expand"), m.width))
	}
	places := 0
	p.root.Walk(func(n *genealogy.PlaceNode) bool {
		if len(n.Events) > 0 {
			places++
		}
		return true
	})
	summary := fmt.Sprintf("  %d places · %d events with a place", places, p.root.Count())
	if m.filteringPlaces() && len(p.rows) == 0 {
		summary = "  no place matches the filter"
	}
	out = append(out, fit(m.st.dim.Render(summary), m.width))

	countW := 26
	nameW := max(10, m.width-2-countW)
	for i := p.offset; i < len(p.rows) && len(out) < h; i++ {
		n := p.rows[i]
		marker := "  "
		if len(n.Children) > 0 {
			marker = "▾ "
			if p.collapsed[n] && !m.filteringPlaces() {
				marker = "▹ "
			}
		}
		left := strings.Repeat("  ", n.Depth-1) + marker + n.Name
		counts := fmt.Sprintf("%5d %-6s %5d %-6s", n.Count(), noun(n.Count(), "event", "events"), n.People(), noun(n.People(), "person", "people"))
		if i == p.cursor {
			out = append(out, fit(m.st.selected.Render("▸ "+pad(left, nameW)+counts), m.width))
			continue
		}
		style := m.st.name.UnsetBold()
		if n.Depth == 1 {
			style = m.st.name
		}
		out = append(out, fit("  "+style.Render(pad(left, nameW))+m.st.dim.Render(counts), m.width))
	}
	return strings.Join(out, "\n")
}

// noun returns the singular or plural form for a count.
func noun(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// plural formats a count with the matching form of a noun.
func plural(n int, one, many string) string { return fmt.Sprintf("%d %s", n, noun(n, one, many)) }
