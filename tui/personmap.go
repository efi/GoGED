package tui

import (
	"fmt"
	"math"
	"slices"

	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
	"github.com/efi/goged/worldmap"
)

// The person view of the map (M where a person is selected): the places
// of their events, marked with the number of events there, the other
// places faint, and a dotted route joining the places of their own events
// in date order. Relatives' events count as the person view's c says; c
// switches it here too, and l the route. A panel lists the places in the
// order of the first event there, and the events at the place selected.
// Events count at the nearest place with coordinates, as in the surname
// view.

type personMap struct {
	on      bool
	ind     *gedcom.Individual
	noRoute bool // l hides the route
	// events are the events at each marker, in date order; stops the
	// markers with events, in the order of their first event; route the
	// markers of the person's own dated events in date order.
	events [][]genealogy.TimelineEntry
	stops  []int
	route  []int
	offset int // the first stop in the panel's list
}

// personListRows is how many places the panel lists at most.
const personListRows = 8

// eventMarkers returns the marker each event counts at: that of its
// place, or of the nearest place around it with coordinates.
func (m *Model) eventMarkers() map[*gedcom.Event]int {
	s := &m.mapv
	if s.placeOf != nil {
		return s.placeOf
	}
	index := map[*genealogy.PlaceNode]int{}
	for i, p := range s.places {
		index[p] = i
	}
	s.placeOf = map[*gedcom.Event]int{}
	var walk func(n *genealogy.PlaceNode, at int)
	walk = func(n *genealogy.PlaceNode, at int) {
		if i, ok := index[n]; ok {
			at = i
		}
		if at >= 0 {
			for _, e := range n.Events {
				s.placeOf[e] = at
			}
		}
		for _, c := range n.Children {
			walk(c, at)
		}
	}
	walk(m.places.root, -1)
	return s.placeOf
}

// mapPerson shows the places of a person's events on the map.
func (m *Model) mapPerson(ind *gedcom.Individual) {
	if ind == nil {
		m.setStatus("select a person first: M shows their places on the map")
		return
	}
	m.switchTo(viewMap)
	s := &m.mapv
	s.sn.on = false
	s.pm = personMap{on: true, ind: ind, noRoute: s.pm.noRoute}
	m.computePersonMap()
	if len(s.pm.stops) == 0 {
		m.setStatus("no event of " + ind.DisplayName() + " took place where coordinates are known")
		return
	}
	s.sel = s.pm.stops[0]
	s.pm.scrollList(s.sel)
	m.fitPerson()
}

// leavePersonMap returns to all places.
func (m *Model) leavePersonMap() {
	m.mapv.pm.on = false
	m.setStatus("all places · M shows the places of the selected person")
}

// computePersonMap works out where the person's events took place.
func (m *Model) computePersonMap() {
	s := &m.mapv
	pm := &s.pm
	placeOf := m.eventMarkers()
	pm.events = make([][]genealogy.TimelineEntry, len(s.places))
	pm.stops, pm.route = nil, nil
	for _, e := range genealogy.Timeline(pm.ind, genealogy.TimelineOptions{Relatives: !m.person.hideRelatives}) {
		at, ok := placeOf[e.Event]
		if !ok {
			continue
		}
		if len(pm.events[at]) == 0 {
			pm.stops = append(pm.stops, at)
		}
		pm.events[at] = append(pm.events[at], e)
		if e.Own() && e.Event.Date.IsValid() && (len(pm.route) == 0 || pm.route[len(pm.route)-1] != at) {
			pm.route = append(pm.route, at)
		}
	}
}

// scrollList scrolls the panel's list of places, if need be, so that it
// shows the place of marker sel.
func (pm *personMap) scrollList(sel int) {
	rows := min(personListRows, len(pm.stops))
	pm.offset = clamp(pm.offset, 0, len(pm.stops)-rows)
	k := slices.Index(pm.stops, sel)
	if k < 0 {
		return
	}
	if k < pm.offset {
		pm.offset = k
	}
	if k >= pm.offset+rows {
		pm.offset = k - rows + 1
	}
}

// fitPerson shows all places of the person.
func (m *Model) fitPerson() {
	s := &m.mapv
	if len(s.pm.stops) == 0 {
		return
	}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, i := range s.pm.stops {
		mk := s.markers[i]
		minX, maxX = math.Min(minX, mk.X), math.Max(maxX, mk.X)
		minY, maxY = math.Min(minY, mk.Y), math.Max(maxY, mk.Y)
	}
	s.view = s.view.Proj.Fit(minX, minY, maxX, maxY, m.mapCols(), m.mapRows())
	s.view.Zoom = math.Min(s.view.Zoom, personZoom)
	m.clampMap()
}

// personZoom is the closest a person's places are shown at first: a
// region, with its coasts in view.
const personZoom = 6.0

// stepStop selects the next (dir 1) or previous place of the person.
func (m *Model) stepStop(dir int) {
	s := &m.mapv
	n := len(s.pm.stops)
	if n == 0 {
		return
	}
	k := slices.Index(s.pm.stops, s.sel)
	if k < 0 {
		k = -dir
		if dir < 0 {
			k = n
		}
	}
	m.selectMarker(s.pm.stops[((k+dir)%n+n)%n])
	s.pm.scrollList(s.sel)
}

// updatePersonMap handles the keys of the person view; it reports
// whether it handled the key.
func (m *Model) updatePersonMap(key string) bool {
	s := &m.mapv
	pm := &s.pm
	switch key {
	case "n", " ":
		m.stepStop(1)
	case "N", "p":
		m.stepStop(-1)
	case "0", "home":
		m.fitPerson()
	case "esc":
		m.leavePersonMap()
	case "l":
		pm.noRoute = !pm.noRoute
		if pm.noRoute {
			m.setStatus("the route is hidden (l shows it)")
		} else {
			m.setStatus("the route joins " + pm.ind.DisplayName() + "'s places in date order (l hides it)")
		}
	case "c":
		// The same switch as c in the person view.
		m.person.hideRelatives = !m.person.hideRelatives
		m.rebuildPerson()
		m.computePersonMap()
		if len(pm.events[s.sel]) == 0 && len(pm.stops) > 0 {
			s.sel = pm.stops[0]
		}
		pm.scrollList(s.sel)
		if m.person.hideRelatives {
			m.setStatus("relatives' events are left out, here and in the person view (c shows them)")
		} else {
			m.setStatus("relatives' events are shown, here and in the person view (c leaves them out)")
		}
	default:
		return false
	}
	return true
}

// personMarkers are the markers of the person view: the places of the
// person with their counts, the others faint.
func (m *Model) personMarkers() []worldmap.Marker {
	s := &m.mapv
	out := slices.Clone(s.markers)
	for i := range out {
		if n := len(s.pm.events[i]); n > 0 {
			out[i].Label = fmt.Sprintf("%s %d", s.places[i].Name, n)
		} else {
			out[i].Faint = true
		}
	}
	return out
}

// personRoute is the route drawn: the person's places in date order,
// unless l hid it.
func (m *Model) personRoute() []int {
	if m.mapv.pm.noRoute {
		return nil
	}
	return m.mapv.pm.route
}

// personInfo is the line on the selected place in the person view.
func (m *Model) personInfo() string {
	s := &m.mapv
	evs := s.pm.events[s.sel]
	if len(evs) == 0 {
		return m.renderNames(" · no events of "+s.pm.ind.DisplayName()+" here", m.st.dim, s.pm.ind)
	}
	text := " · " + plural(len(evs), "event", "events")
	if span := yearSpan(entryEvents(evs)); span != "" {
		text += " · " + span
	}
	return m.st.dim.Render(text)
}

// personHeading is the first line of the person view.
func (m *Model) personHeading() string {
	pm := &m.mapv.pm
	located := 0
	for _, evs := range pm.events {
		located += len(evs)
	}
	relatives := "shown"
	if m.person.hideRelatives {
		relatives = "left out"
	}
	return m.st.section.Render("Map") + m.st.dim.Render(" · places of ") + m.personStyle(pm.ind, m.st.mapMarker).Render(pm.ind.DisplayName()) +
		m.st.dim.Render(fmt.Sprintf(" · %s at %s · ", plural(located, "event", "events"), plural(len(pm.stops), "place", "places"))) +
		m.st.key.Render("c") + m.st.dim.Render(" relatives' events "+relatives+" · ") + m.st.key.Render("l") + m.st.dim.Render(" route · ") +
		m.st.key.Render("esc") + m.st.dim.Render(" all places")
}

func entryEvents(entries []genealogy.TimelineEntry) []*gedcom.Event {
	out := make([]*gedcom.Event, len(entries))
	for i, e := range entries {
		out[i] = e.Event
	}
	return out
}

// personPanel lays out the panel beside the map, rows lines high and
// width columns wide: the person, their places in the order of the first
// event there, and the events at the place selected.
func (m *Model) personPanel(rows, width int) []string {
	s := &m.mapv
	pm := &s.pm
	out := []string{m.personStyle(pm.ind, m.st.section).Render(pm.ind.DisplayName())}
	about := plural(len(pm.stops), "place", "places") + " · n N"
	if ls := pm.ind.Lifespan(); ls != "" {
		about = ls + " · " + about
	}
	out = append(out, m.st.dim.Render(about), "")
	listRows := min(personListRows, len(pm.stops))
	for k := pm.offset; k < pm.offset+listRows; k++ {
		i := pm.stops[k]
		year := ""
		for _, e := range pm.events[i] {
			if e.Event.Date.IsValid() {
				year = e.Event.Date.ShortYear()
				break
			}
		}
		text := pad(year, 9) + pad(s.places[i].Name, width-15) + fmt.Sprintf("%4d", len(pm.events[i]))
		if i == s.sel {
			out = append(out, m.st.selected.Render("▸ "+text))
		} else {
			out = append(out, "  "+m.st.dim.Render(pad(year, 9))+m.st.name.Render(pad(s.places[i].Name, width-15))+m.st.dim.Render(fmt.Sprintf("%4d", len(pm.events[i]))))
		}
	}
	if more := len(pm.stops) - pm.offset - listRows; more > 0 {
		out = append(out, m.st.dim.Render(fmt.Sprintf("  … %d more", more)))
	}
	if evs := pm.events[s.sel]; len(evs) > 0 {
		out = append(out, "", m.st.section.Render(s.places[s.sel].Name))
		for k, e := range evs {
			if len(out) >= rows {
				break
			}
			if len(out) == rows-1 && k < len(evs)-1 {
				out = append(out, m.st.dim.Render(fmt.Sprintf("  … %d more", len(evs)-k)))
				break
			}
			date, _ := e.Event.Date.Fit(10)
			if !e.Event.Date.IsValid() {
				date = "—"
			}
			style := m.st.name.UnsetBold()
			if !e.Own() {
				style = m.st.dim
			}
			out = append(out, "  "+m.st.dim.Render(pad(date, 11))+m.renderNames(e.Title(), style, e.Relative, e.Spouse))
		}
	}
	for i := range out {
		out[i] = fit(out[i], width)
	}
	return out
}

// clickPersonPanel selects the place at body row y of the panel, if any.
func (m *Model) clickPersonPanel(y int) {
	pm := &m.mapv.pm
	// The list starts below the name, the lifespan and a blank line, on
	// the map's first row (the body's third).
	k := pm.offset + y - 2 - 3
	if k >= pm.offset && k < min(len(pm.stops), pm.offset+personListRows) {
		m.selectMarker(pm.stops[k])
		pm.scrollList(m.mapv.sel)
	}
}

// personHints are the keys of the person view for the footer.
var personHints = [][2]string{{"n/N", "places"}, {"c", "relatives"}, {"l", "route"}, {"enter", "events here"},
	{"esc", "all places"}, {"arrows +/-", "pan, zoom"}, {"?", "help"}}
