package tui

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
	"github.com/efi/goged/worldmap"
)

// The surname view of the map (s): the places of the people born with one
// surname, with the number of their events; the other places stay as
// faint dots. A panel lists the surnames, the selected place and the
// surname's events per decade. Events count at the nearest place with
// coordinates.

// Kinds of events shown, cycled with e.
const (
	eventsAll = iota
	eventsBirth
	eventsDeath
	eventsOther
	eventKinds
)

var eventKindNames = [eventKinds]string{"all events", "births", "deaths", "other events"}

// surnamePanel is the width of the panel beside the map, with its rule;
// narrower screens show the map only.
const (
	surnamePanel    = 32
	surnamePanelMin = 72
)

type surnameMap struct {
	on     bool
	names  []surnameEntry // birth surnames, most people first
	name   int            // the surname shown
	offset int            // the first surname in the panel's list
	kind   int

	surnameOf map[*gedcom.Individual]string
	places    map[string]int // places on the map per surname, for kind
	placesFor int            // the kind places was worked out for, plus 1

	// For the surname and kind shown: its events at each marker, its people
	// there, and all its events of the kind, also those without coordinates.
	events [][]*gedcom.Event
	people []int
	all    []*gedcom.Event
}

type surnameEntry struct {
	name   string
	people int
}

// mapCols is the width of the map itself.
func (m *Model) mapCols() int {
	if (m.mapv.sn.on || m.mapv.pm.on) && m.width >= surnamePanelMin {
		return m.width - surnamePanel
	}
	return m.width
}

// ensureSurnames lists the birth surnames and where events count, the
// first time the surname view is used.
func (m *Model) ensureSurnames() {
	s := &m.mapv
	sn := &s.sn
	if sn.surnameOf != nil {
		return
	}
	d := m.sh.surnames()
	sn.names, sn.surnameOf = d.names, d.surnameOf
}

// kindOf tells which kind of event e is.
func kindOf(e *gedcom.Event) int {
	switch {
	case slices.Contains(gedcom.BirthTags, e.Tag):
		return eventsBirth
	case slices.Contains(gedcom.DeathTags, e.Tag):
		return eventsDeath
	}
	return eventsOther
}

// surnamesOf returns the birth surnames of the people in an event.
func (sn *surnameMap) surnamesOf(e *gedcom.Event) []string {
	var out []string
	for _, p := range e.Principals() {
		if name, ok := sn.surnameOf[p]; ok && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// computeSurname works out where the surname shown has events of the kind
// shown, and how many places every surname has.
func (m *Model) computeSurname() {
	s := &m.mapv
	sn := &s.sn
	if len(sn.names) == 0 {
		return
	}
	name := sn.names[sn.name].name
	sn.events = make([][]*gedcom.Event, len(s.places))
	people := make([]map[*gedcom.Individual]bool, len(s.places))
	sn.all = nil
	counting := sn.placesFor != sn.kind+1
	if counting {
		sn.places = map[string]int{}
	}
	seen := map[string]map[int]bool{}
	placeOf := m.eventMarkers()
	for _, e := range m.doc.Events() {
		if sn.kind != eventsAll && kindOf(e) != sn.kind {
			continue
		}
		at, located := placeOf[e]
		for _, sname := range sn.surnamesOf(e) {
			if counting && located {
				if seen[sname] == nil {
					seen[sname] = map[int]bool{}
				}
				if !seen[sname][at] {
					seen[sname][at] = true
					sn.places[sname]++
				}
			}
			if sname != name {
				continue
			}
			sn.all = append(sn.all, e)
			if located {
				sn.events[at] = append(sn.events[at], e)
				if people[at] == nil {
					people[at] = map[*gedcom.Individual]bool{}
				}
				for _, p := range e.Principals() {
					if sn.surnameOf[p] == name {
						people[at][p] = true
					}
				}
			}
		}
	}
	sn.placesFor = sn.kind + 1
	sn.people = make([]int, len(s.places))
	for i, ps := range people {
		sn.people[i] = len(ps)
	}
}

// setSurname shows surname i, its busiest place selected and all its
// places in view.
func (m *Model) setSurname(i int) {
	s := &m.mapv
	sn := &s.sn
	if len(sn.names) == 0 {
		return
	}
	sn.name = clamp(i, 0, len(sn.names)-1)
	sn.scrollList()
	m.computeSurname()
	m.selectBusiest()
	m.fitSurname()
}

// scrollList scrolls the panel's list of surnames, if need be, so that it
// shows the surname shown on the map. The list scrolls here rather than
// when it is drawn, which happens on a copy of the model: a click in the
// list must find the surnames where they are shown.
func (sn *surnameMap) scrollList() {
	rows := min(surnameListRows, len(sn.names))
	sn.offset = clamp(sn.offset, 0, len(sn.names)-rows)
	if sn.name < sn.offset {
		sn.offset = sn.name
	}
	if sn.name >= sn.offset+rows {
		sn.offset = sn.name - rows + 1
	}
}

// selectBusiest selects the place with the most events of the surname.
func (m *Model) selectBusiest() {
	sn := &m.mapv.sn
	best := -1
	for i, evs := range sn.events {
		if len(evs) > 0 && (best < 0 || len(evs) > len(sn.events[best])) {
			best = i
		}
	}
	if best >= 0 {
		m.mapv.sel = best
	}
}

// fitSurname shows all places of the surname, if it has any.
func (m *Model) fitSurname() {
	s := &m.mapv
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	any := false
	for i, evs := range s.sn.events {
		if len(evs) > 0 {
			mk := s.markers[i]
			minX, maxX = math.Min(minX, mk.X), math.Max(maxX, mk.X)
			minY, maxY = math.Min(minY, mk.Y), math.Max(maxY, mk.Y)
			any = true
		}
	}
	if !any {
		m.setStatus(fmt.Sprintf("no %s of %s took place where coordinates are known", eventKindNames[s.sn.kind], s.sn.names[s.sn.name].name))
		return
	}
	s.view = s.view.Proj.Fit(minX, minY, maxX, maxY, m.mapCols(), m.mapRows())
	// Close places would fill the screen with countryside; a region keeps
	// the coasts in view.
	s.view.Zoom = math.Min(s.view.Zoom, surnameZoom)
	m.clampMap()
}

// surnameZoom is the closest a surname's places are shown at first.
const surnameZoom = 5.0

// toggleSurnames turns the surname view on or off.
func (m *Model) toggleSurnames() {
	s := &m.mapv
	if s.sn.on {
		s.sn.on = false
		m.setStatus("all places · s shows the places of a surname")
		return
	}
	m.ensureSurnames()
	if len(s.sn.names) == 0 {
		m.setStatus("nobody in the file has a surname")
		return
	}
	s.sn.on, s.pm.on = true, false
	m.setSurname(s.sn.name)
}

// showSurnameOnMap opens the map with the places of a birth surname.
func (m *Model) showSurnameOnMap(name string) {
	m.ensureMap()
	m.ensureSurnames()
	for i, e := range m.mapv.sn.names {
		if strings.EqualFold(e.name, name) {
			m.switchTo(viewMap)
			m.mapv.sn.on = true
			m.setSurname(i)
			return
		}
	}
	m.setStatus("nobody was born with the surname " + name)
}

// cycleKind shows the next kind of events.
func (m *Model) cycleKind() {
	sn := &m.mapv.sn
	sn.kind = (sn.kind + 1) % eventKinds
	m.computeSurname()
	if len(sn.events[m.mapv.sel]) == 0 {
		m.selectBusiest()
	}
	m.setStatus(fmt.Sprintf("%s of %s: %d", eventKindNames[sn.kind], sn.names[sn.name].name, len(sn.all)))
}

// stepSurnamePlace selects the next (dir 1) or previous place of the
// surname and centers the map on it.
func (m *Model) stepSurnamePlace(dir int) {
	s := &m.mapv
	n := len(s.places)
	for k := 1; k <= n; k++ {
		i := ((s.sel+dir*k)%n + n) % n
		if len(s.sn.events[i]) > 0 {
			m.selectMarker(i)
			return
		}
	}
}

// surnameMarkers are the markers of the surname view: the places of the
// surname with their counts, the others faint.
func (m *Model) surnameMarkers() []worldmap.Marker {
	s := &m.mapv
	out := slices.Clone(s.markers)
	for i := range out {
		if n := len(s.sn.events[i]); n > 0 {
			out[i].Label = fmt.Sprintf("%s %d", s.places[i].Name, n)
		} else {
			out[i].Faint = true
		}
	}
	return out
}

// surnameInfo is the line on the map's selected place in the surname view.
func (m *Model) surnameInfo() string {
	s := &m.mapv
	name := s.sn.names[s.sn.name].name
	evs := s.sn.events[s.sel]
	if len(evs) == 0 {
		return m.st.dim.Render(fmt.Sprintf(" · no %s of %s here", eventKindNames[s.sn.kind], name))
	}
	text := fmt.Sprintf(" · %s: %s · %s", name, plural(len(evs), "event", "events"), plural(s.sn.people[s.sel], "person", "people"))
	if span := yearSpan(evs); span != "" {
		text += " · " + span
	}
	return m.st.dim.Render(text)
}

// yearSpan gives the years of dated events: "1790–1880".
func yearSpan(evs []*gedcom.Event) string {
	lo, hi := math.MaxInt, math.MinInt
	for _, e := range evs {
		if y := e.Date.Year(); y != 0 {
			lo, hi = min(lo, y), max(hi, y)
		}
	}
	switch {
	case lo == math.MaxInt:
		return ""
	case lo == hi:
		return strconv.Itoa(lo)
	}
	return fmt.Sprintf("%d–%d", lo, hi)
}

// surnameListRows is how many surnames the panel lists at most.
const surnameListRows = 8

// panelLines lays out the panel beside the map, rows lines high and
// width columns wide: the surnames, the selected place and the surname's
// events per decade.
func (m *Model) panelLines(rows, width int) []string {
	s := &m.mapv
	sn := &s.sn
	name := sn.names[sn.name].name
	listRows := min(surnameListRows, len(sn.names))
	out := []string{m.st.section.Render("Surnames") + m.st.dim.Render(" · PgUp PgDn"), ""}
	for i := sn.offset; i < sn.offset+listRows; i++ {
		e := sn.names[i]
		places := "–"
		if n := sn.places[e.name]; n > 0 {
			places = plural(n, "place", "places")
		}
		text := pad(e.name, width-17) + fmt.Sprintf("%4d", e.people) + "  " + pad(places, 9)
		if i == sn.name {
			out = append(out, m.st.selected.Render("▸ "+text))
		} else {
			out = append(out, "  "+m.st.name.Render(pad(e.name, width-17))+m.st.dim.Render(fmt.Sprintf("%4d", e.people)+"  "+places))
		}
	}
	if more := len(sn.names) - sn.offset - listRows; more > 0 {
		out = append(out, m.st.dim.Render(fmt.Sprintf("  … %d more", more)))
	}

	if evs := sn.events[s.sel]; len(evs) > 0 {
		out = append(out, "", m.st.section.Render(name+" in "+s.places[s.sel].Name),
			m.st.dim.Render("  "+plural(len(evs), "event", "events")+" · "+plural(sn.people[s.sel], "person", "people")))
		if span := yearSpan(evs); span != "" {
			out = append(out, m.st.dim.Render("  "+span))
		}
		var people []*gedcom.Individual
		for _, e := range evs {
			for _, p := range e.Principals() {
				if sn.surnameOf[p] == name && !slices.Contains(people, p) {
					people = append(people, p)
				}
			}
		}
		for i, p := range people {
			if i == 3 && len(people) > 4 {
				out = append(out, m.st.dim.Render(fmt.Sprintf("  … %d more", len(people)-3)))
				break
			}
			out = append(out, "  "+m.personStyle(p, m.st.mapLabel).Render(p.DisplayName())+" "+m.st.dim.Render(p.Lifespan()))
		}
	}

	// The surname's events per decade, or per longer periods if they are
	// many, in the rows that are left.
	years := map[int]int{}
	for _, e := range sn.all {
		if y := e.Date.Year(); y != 0 {
			years[y]++
		}
	}
	if room := rows - len(out) - 2; len(years) > 0 && room > 0 {
		var d genealogy.Distribution
		for y, n := range years {
			d = append(d, genealogy.ValueCount{Value: y, Count: n})
		}
		sort.Slice(d, func(i, j int) bool { return d[i].Value < d[j].Value })
		var buckets []genealogy.Bucket
		period := "decade"
		for _, p := range birthPeriods {
			if buckets, period = d.Buckets(p.years, math.MaxInt), p.name; len(buckets) <= room {
				break
			}
		}
		out = append(out, "", m.st.section.Render(name+" per "+period))
		most := 0
		for _, b := range buckets {
			most = max(most, b.Count)
		}
		for _, b := range buckets[:min(len(buckets), room)] {
			label := fmt.Sprintf("%ds", b.Lo)
			if b.Hi-b.Lo != 9 {
				label = fmt.Sprintf("%d", b.Lo)
			}
			out = append(out, "  "+m.st.label.Render(pad(label, 6))+m.st.bar.Render(bar(b.Count, most, width-10, m.opts.ASCII)))
		}
	}
	for i := range out {
		out[i] = fit(out[i], width)
	}
	return out
}

// clickPanel chooses the surname at body row y of the panel, if any, or
// the place in the person view.
func (m *Model) clickPanel(y int) {
	if m.mapv.pm.on {
		m.clickPersonPanel(y)
		return
	}
	sn := &m.mapv.sn
	// The list starts below the panel's title and a blank line, on the
	// map's first row (the body's third).
	i := sn.offset + y - 2 - 2
	if i >= sn.offset && i < min(len(sn.names), sn.offset+surnameListRows) && i != sn.name {
		m.setSurname(i)
	}
}
