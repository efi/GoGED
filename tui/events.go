package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/search"
)

type eventsState struct {
	input   textinput.Model
	editing bool
	all     bool // show all events, not only the important (vital) ones
	list    []*gedcom.Event
	err     string
	cursor  int
	offset  int
}

func newEventsState() eventsState {
	return eventsState{input: newTextInput("Filter: ", "e.g. type:marr 1850..1870 place:leeds name:smith")}
}

// ensureEvents builds the event index the first time the view is shown;
// sorting all events of a large file takes a moment.
func (m *Model) ensureEvents() {
	if m.events == nil {
		m.events = search.NewEventIndex(m.doc.Events())
		m.filterEvents()
	}
}

// filterEvents applies the filter input to the event list.
func (m *Model) filterEvents() {
	e := &m.ev
	q, err := search.ParseEventQuery(e.input.Value())
	if err != nil {
		e.err = err.Error()
		e.list = nil
	} else {
		e.err = ""
		e.list = m.events.Filter(q, !e.all)
	}
	e.cursor = clamp(e.cursor, 0, len(e.list)-1)
	m.moveEvents(0)
}

func (m *Model) eventsListHeight() int { return max(1, m.bodyHeight()-2) }

func (m *Model) moveEvents(delta int) {
	e := &m.ev
	e.cursor = clamp(e.cursor+delta, 0, len(e.list)-1)
	h := m.eventsListHeight()
	if e.cursor < e.offset {
		e.offset = e.cursor
	}
	if e.cursor >= e.offset+h {
		e.offset = e.cursor - h + 1
	}
	e.offset = clamp(e.offset, 0, max(0, len(e.list)-h))
}

func (m *Model) updateEventsFilter(msg tea.KeyMsg) (bool, tea.Cmd) {
	e := &m.ev
	switch msg.String() {
	case "enter":
		e.editing = false
		e.input.Blur()
	case "esc":
		e.input.SetValue("")
		e.editing = false
		e.input.Blur()
		m.filterEvents()
	case "up":
		m.moveEvents(-1)
	case "down":
		m.moveEvents(1)
	case "pgup":
		m.moveEvents(-m.eventsListHeight())
	case "pgdown":
		m.moveEvents(m.eventsListHeight())
	default:
		if isViewKey(msg.String()) {
			e.editing = false
			e.input.Blur()
			return false, nil
		}
		var cmd tea.Cmd
		e.input, cmd = e.input.Update(msg)
		e.cursor, e.offset = 0, 0
		m.filterEvents()
		return true, cmd
	}
	return true, nil
}

// eventPerson picks the person an event should open: its owner or, for
// family events, the first partner.
func eventPerson(ev *gedcom.Event) *gedcom.Individual {
	if p := ev.Principals(); len(p) > 0 {
		return p[0]
	}
	return nil
}

func (m *Model) updateEvents(msg tea.KeyMsg) (bool, tea.Cmd) {
	e := &m.ev
	switch msg.String() {
	case "up", "k":
		m.moveEvents(-1)
	case "down", "j":
		m.moveEvents(1)
	case "pgup", "ctrl+u":
		m.moveEvents(-m.eventsListHeight())
	case "pgdown", "ctrl+d", " ":
		m.moveEvents(m.eventsListHeight())
	case "home", "g":
		m.moveEvents(-len(e.list))
	case "end", "G":
		m.moveEvents(len(e.list))
	case "enter":
		if len(e.list) > 0 {
			m.open(eventPerson(e.list[e.cursor]))
		}
	case "f", "e":
		e.editing = true
		return true, e.input.Focus()
	case "a":
		e.all = !e.all
		m.filterEvents()
		if e.all {
			m.setStatus("showing all events")
		} else {
			m.setStatus("showing important events only (births, baptisms, marriages, divorces, deaths, burials)")
		}
	case "x", "esc":
		if e.input.Value() == "" {
			return false, nil
		}
		e.input.SetValue("")
		m.filterEvents()
	default:
		return false, nil
	}
	return true, nil
}

func (m Model) viewEvents(h int) string {
	e := &m.ev
	var out []string
	if e.editing || e.input.Value() != "" {
		out = append(out, fit(e.input.View(), m.width))
	} else {
		out = append(out, fit(m.st.dim.Render("  press f to filter, a to toggle all events"), m.width))
	}
	scope := "important events"
	if e.all {
		scope = "events"
	}
	if e.err != "" {
		out = append(out, fit(m.st.err.Render("  "+e.err), m.width))
	} else {
		out = append(out, fit(m.st.dim.Render(fmt.Sprintf("  %d %s", len(e.list), scope)), m.width))
	}

	out = append(out, m.eventRows(e.list, e.cursor, e.offset, h-len(out))...)
	return strings.Join(out, "\n")
}

// eventRows renders up to n rows of an event list starting at offset, with
// the row at cursor highlighted: date, event, people involved and place.
func (m Model) eventRows(list []*gedcom.Event, cursor, offset, n int) []string {
	var out []string
	dateW, labelW := 20, 18
	nameW := clamp((m.width-dateW-labelW-4)*3/5, 12, 40)
	placeW := max(0, m.width-2-dateW-labelW-nameW-3)
	for i := offset; i < len(list) && len(out) < n; i++ {
		ev := list[i]
		date := ev.Date.String()
		if !ev.Date.IsValid() {
			date = "—"
		}
		var names []string
		for _, p := range ev.Principals() {
			names = append(names, p.DisplayName())
		}
		label := ev.Label()
		if d := ev.Detail(); d != "" && ev.Class() == gedcom.ClassAttribute {
			label += ": " + d
		}
		who := strings.Join(names, " & ")
		if i == cursor {
			row := pad(date, dateW) + " " + pad(label, labelW) + " " + pad(who, nameW) + " " + ev.Place.String()
			out = append(out, fit(m.st.selected.Render("▸ "+strings.TrimRight(row, " ")), m.width))
			continue
		}
		row := "  " + m.st.label.Render(pad(date, dateW)) + " " + pad(label, labelW) + " " +
			m.st.name.Render(pad(who, nameW)) + " " + m.st.dim.Render(pad(ev.Place.String(), placeW))
		out = append(out, fit(row, m.width))
	}
	return out
}
