package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/search"
)

type searchState struct {
	input   textinput.Model
	results []search.Result
	err     string
	cursor  int
	offset  int
	lastRun string
}

func newTextInput(prompt, placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.Placeholder = placeholder
	ti.Cursor.SetMode(cursor.CursorStatic)
	return ti
}

func newSearchState() searchState {
	ti := newTextInput("Search: ", "name, or field:value — e.g. smith born:1840..1860 place:leeds (? for help)")
	ti.Focus()
	return searchState{input: ti}
}

// runSearch executes the query in the input if it changed.
func (m *Model) runSearch() {
	q := m.search.input.Value()
	if q == m.search.lastRun && m.search.results != nil {
		return
	}
	m.search.lastRun = q
	results, err := m.index.SearchString(q)
	if err != nil {
		m.search.err = err.Error()
		m.search.results = []search.Result{}
	} else {
		m.search.err = ""
		m.search.results = results
	}
	m.search.cursor, m.search.offset = 0, 0
}

// setQuery replaces the search query and runs it.
func (m *Model) setQuery(q string) {
	m.search.input.SetValue(q)
	m.search.input.CursorEnd()
	m.runSearch()
}

func (m *Model) searchListHeight() int { return max(1, m.bodyHeight()-2) }

func (m *Model) moveSearch(delta int) {
	s := &m.search
	s.cursor = clamp(s.cursor+delta, 0, len(s.results)-1)
	h := m.searchListHeight()
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+h {
		s.offset = s.cursor - h + 1
	}
}

func (m *Model) updateSearch(msg tea.KeyMsg) (bool, tea.Cmd) {
	s := &m.search
	switch msg.String() {
	case "up", "ctrl+p", "ctrl+k":
		m.moveSearch(-1)
	case "down", "ctrl+n", "ctrl+j":
		m.moveSearch(1)
	case "pgup":
		m.moveSearch(-m.searchListHeight())
	case "pgdown":
		m.moveSearch(m.searchListHeight())
	case "ctrl+home":
		m.moveSearch(-len(s.results))
	case "ctrl+end":
		m.moveSearch(len(s.results))
	case "enter":
		if len(s.results) > 0 {
			m.open(s.results[s.cursor].Individual)
		}
	case "esc":
		switch {
		case s.input.Value() != "":
			m.setQuery("")
		case m.current() != nil:
			prev := m.prev
			if prev == viewSearch {
				prev = viewPerson
			}
			m.switchTo(prev)
		}
	case "f1":
		return false, nil
	case "?":
		// Show help, unless the user is in the middle of typing a query.
		if s.input.Value() == "" {
			return false, nil
		}
		return true, m.typeInSearch(msg)
	default:
		if isViewKey(msg.String()) {
			return false, nil
		}
		return true, m.typeInSearch(msg)
	}
	return true, nil
}

// typeInSearch forwards a key to the query input and reruns the search.
func (m *Model) typeInSearch(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	m.search.input, cmd = m.search.input.Update(msg)
	m.runSearch()
	return cmd
}

func (m Model) viewSearch(h int) string {
	s := &m.search
	var out []string
	out = append(out, fit(s.input.View(), m.width))
	switch {
	case s.err != "":
		out = append(out, fit(m.st.err.Render("  "+s.err), m.width))
	case s.input.Value() == "":
		out = append(out, fit(m.st.dim.Render(fmt.Sprintf("  %d people — type to search", len(s.results))), m.width))
	case len(s.results) == 1:
		out = append(out, m.st.dim.Render("  1 match"))
	default:
		out = append(out, fit(m.st.dim.Render(fmt.Sprintf("  %d matches", len(s.results))), m.width))
	}

	listH := h - 2
	nameW := clamp(m.width*2/5, 16, 44)
	spanW := 13
	idW := 7
	placeW := max(0, m.width-2-nameW-spanW-idW-3)
	for i := s.offset; i < len(s.results) && i < s.offset+listH; i++ {
		r := s.results[i]
		ind := r.Individual
		place := ""
		if b := ind.FirstDatedEvent(gedcom.BirthTags...); b != nil {
			place = b.Place.String()
		}
		name := ind.SortName()
		alt := ""
		if r.NameIndex > 0 && r.NameIndex < len(ind.Names) {
			// The query matched an alternate name; show it.
			alt = " (" + ind.Names[r.NameIndex].String() + ")"
		}
		if i == s.cursor {
			cols := pad(name+alt, nameW) + " " + pad(ind.Lifespan(), spanW) + " " + pad(place, placeW) + " " + pad(ind.ID, idW)
			out = append(out, fit(m.st.selected.Render("▸ "+strings.TrimRight(cols, " ")), m.width))
			continue
		}
		nameCol := m.st.name.Render(name) + m.st.dim.Render(alt)
		if w := ansi.StringWidth(name + alt); w > nameW {
			nameCol = m.st.name.Render(pad(name+alt, nameW))
		} else {
			nameCol += strings.Repeat(" ", nameW-w)
		}
		row := "  " + nameCol + " " + m.st.dim.Render(pad(ind.Lifespan(), spanW)) + " " + pad(place, placeW) + " " + m.st.dim.Render(ind.ID)
		out = append(out, fit(row, m.width))
	}
	return strings.Join(out, "\n")
}
