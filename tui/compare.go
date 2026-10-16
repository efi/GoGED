package tui

import (
	"fmt"
	"math"

	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/diff"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
)

// The comparison (C): two people side by side, as diff.People pairs what
// their records hold, in an overlay like the help; the marked person is
// on the left. In the checks, C also compares a group of places that may
// be the same, in a table.

type compareState struct {
	a, b *gedcom.Individual
	// labels name a and b: the marked and the selected person, or two
	// records that may describe one person.
	labels [2]string
	// places are the full names of the places of a group, compared
	// instead of people; title says what the group is.
	places []string
	title  string
}

// compareSelected compares the selected person with the marked one.
func (m *Model) compareSelected() {
	sel, ref := m.selected(), m.reference
	switch {
	case ref == nil:
		m.setError("mark a person with m first: C compares the selected person with them")
	case sel == nil:
		m.setStatus("select a person to compare with " + ref.DisplayName())
	case sel == ref:
		m.setStatus(sel.DisplayName() + " is the marked person: select another one to compare with them")
	default:
		m.comparePeople(ref, sel, "Marked", "Selected")
	}
}

// comparePeople shows a and b side by side, labeled as given.
func (m *Model) comparePeople(a, b *gedcom.Individual, labelA, labelB string) {
	m.compare = compareState{a: a, b: b, labels: [2]string{labelA, labelB}}
	m.openOverlay("Compare")
}

// comparePlaces shows a group of places in a table; title says what
// they have in common.
func (m *Model) comparePlaces(places []*genealogy.PlaceNode, title string) {
	m.compare = compareState{title: title}
	for _, p := range places {
		m.compare.places = append(m.compare.places, p.Full)
	}
	m.openOverlay("Compare")
}

// buildCompare lays out the comparison for the width of the screen: a
// line on each person, which opens them, then a row for every detail,
// wrapped within its columns and marked with ≠ where they differ.
func (m *Model) buildCompare() {
	if m.compare.places != nil {
		m.buildPlaceComparison()
		return
	}
	a, b := m.compare.a, m.compare.b
	if a == nil || b == nil {
		m.help.set([]line{dimLine(m, "the people compared are not in the file any more")})
		return
	}
	rows := diff.People(a, b)
	differ, whatW := 0, 0
	for _, r := range rows {
		if !r.Same() {
			differ++
		}
		whatW = max(whatW, ansi.StringWidth(r.What))
	}
	whatW = min(whatW, labelWidth)
	width := max(40, m.width-2) // the overlay keeps two columns for the selection
	colW := max(10, (width-2-whatW-2-2)/2)
	gap := span{text: "  ", width: 2}

	ls := []line{
		textLine(span{text: "Comparison of two people", style: m.st.name}),
		dimLine(m, fmt.Sprintf("%s, %d the same · enter opens a person", plural(differ, "difference", "differences"), len(rows)-differ)),
		textLine(),
		m.relativeLine(m.compare.labels[0], a, "  "+a.ID),
		m.relativeLine(m.compare.labels[1], b, "  "+b.ID),
		textLine(),
		textLine(span{text: "", width: 2 + whatW}, gap, span{text: a.DisplayName(), style: m.st.label, width: colW}, gap,
			span{text: b.DisplayName(), style: m.st.label}),
	}
	for _, r := range rows {
		mark, style := "", m.st.dim
		if !r.Same() {
			mark, style = "≠", m.st.name.UnsetBold()
		}
		ls = append(ls, textLine(span{text: mark, style: m.st.key, width: 2}, span{text: r.What, style: m.st.label, width: whatW}, gap,
			span{text: r.Left, style: style, width: colW}, gap, span{text: r.Right, style: style}))
	}
	m.help.set(expand(ls, width))
}

// buildPlaceComparison lays out a group of places as a table: a row for
// each place, which opens it in the places, with its events, people,
// years, coordinates and distance from the first with coordinates, place
// record and GOV identifier. Columns in which all places agree are dim.
func (m *Model) buildPlaceComparison() {
	m.ensurePlaces()
	byFull := map[string]*genealogy.PlaceNode{}
	m.places.root.Walk(func(n *genealogy.PlaceNode) bool {
		byFull[n.Full] = n
		return true
	})
	var places []*genealogy.PlaceNode
	for _, full := range m.compare.places {
		if n := byFull[full]; n != nil {
			places = append(places, n)
		}
	}
	ls := []line{textLine(span{text: "Places that may be the same", style: m.st.name})}
	if m.compare.title != "" {
		ls = append(ls, dimLine(m, m.compare.title))
	}
	if len(places) == 0 {
		m.help.set(append(ls, dimLine(m, "the places are not in the file any more")))
		return
	}
	ls = append(ls, dimLine(m, "enter opens a place"), textLine())

	var first *genealogy.PlaceNode // the first with coordinates
	for _, p := range places {
		if p.HasCoords {
			first = p
			break
		}
	}
	titles := []string{"place", "events", "people", "years", "coordinates", "distance", "record", "GOV"}
	if first != nil {
		titles[5] = "from " + first.Name
	}
	cells := make([][]string, len(places))
	for i, p := range places {
		coords, dist, record, gov := "–", "–", "–", "–"
		if p.HasCoords {
			coords = formatLatLon(p.Lat, p.Lon)
			dist = formatDistance(distanceKm(first.Lat, first.Lon, p.Lat, p.Lon))
		}
		if p.Location != nil {
			record = p.Location.ID
		}
		if p.GOV != "" {
			gov = p.GOV
		}
		cells[i] = []string{p.Name, fmt.Sprint(p.Count()), fmt.Sprint(p.People()), yearSpan(p.AllEvents()), coords, dist, record, gov}
	}
	widths := make([]int, len(titles))
	same := make([]bool, len(titles))
	for c, title := range titles {
		widths[c], same[c] = ansi.StringWidth(title), true
		for i := range cells {
			widths[c] = max(widths[c], ansi.StringWidth(cells[i][c]))
			same[c] = same[c] && cells[i][c] == cells[0][c]
		}
	}
	widths[0] = min(widths[0], 30)
	row := func(texts []string, header bool) line {
		var spans []span
		for c, t := range texts {
			style := m.st.name.UnsetBold()
			switch {
			case header:
				style = m.st.label
			case c == 0:
				style = m.st.name
			case same[c]:
				style = m.st.dim
			}
			sp := span{text: t, style: style}
			if c < len(texts)-1 {
				sp.width = widths[c] + 2
			}
			spans = append(spans, sp)
		}
		return textLine(spans...)
	}
	ls = append(ls, row(titles, true))
	for i, p := range places {
		l := row(cells[i], false)
		l.place = p.Full
		ls = append(ls, l)
	}
	m.help.set(expand(ls, max(40, m.width-2)))
}

// distanceKm returns the distance of two points on the earth in
// kilometres (haversine).
func distanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	const radius = 6371.0
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	h := math.Pow(math.Sin(dLat/2), 2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Pow(math.Sin(dLon/2), 2)
	return 2 * radius * math.Asin(math.Sqrt(h))
}

// formatDistance renders a distance in kilometres: "0 km", "3.2 km",
// "41 km".
func formatDistance(km float64) string {
	if km < 10 && math.Round(km*10) != 0 {
		return fmt.Sprintf("%.1f km", km)
	}
	return fmt.Sprintf("%.0f km", km)
}
