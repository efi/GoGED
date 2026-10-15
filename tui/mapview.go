package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/efi/goged/genealogy"
	"github.com/efi/goged/worldmap"
)

// mapState drives the map view: a zoomable world map with
// markers for every place that has coordinates.
type mapState struct {
	world   *worldmap.World
	err     error
	ready   bool
	view    worldmap.View
	places  []*genealogy.PlaceNode // places with coordinates, most events first
	markers []worldmap.Marker
	sel     int
	unknown int // places without coordinates

	// The cell where the mouse was while it drags the map, and whether
	// releasing the button opens the selected place: it was pressed on
	// its marker and did not drag.
	dragging     bool
	dragX, dragY int
	opening      bool

	sn surnameMap
}

const (
	panFraction = 0.25 // part of the screen moved by one arrow key
	zoomStep    = 0.5  // one +/- step zooms by a factor of √2
)

// mapRows returns the number of rows available for the map itself.
func (m *Model) mapRows() int { return max(1, m.bodyHeight()-3) }

// ensureMap loads the map data and the located places the first time the
// view is shown.
func (m *Model) ensureMap() {
	s := &m.mapv
	if s.ready {
		return
	}
	s.ready = true
	if s.world == nil && s.err == nil {
		s.world, s.err = worldmap.Load()
		if s.world != nil {
			// The other projections are ready by the time v asks for them.
			go func(w *worldmap.World, shown worldmap.Projection) {
				for p := shown.Next(); p != shown; p = p.Next() {
					w.Prepare(p)
				}
			}(s.world, s.view.Proj)
		}
	}
	m.ensurePlaces()
	located := m.places.root.Located()
	sort.SliceStable(located, func(i, j int) bool {
		if located[i].Count() != located[j].Count() {
			return located[i].Count() > located[j].Count()
		}
		return located[i].Full < located[j].Full
	})
	s.places = located
	s.markers = make([]worldmap.Marker, len(located))
	for i, p := range located {
		s.markers[i].Label = p.Name
	}
	m.projectMarkers()
	m.places.root.Walk(func(n *genealogy.PlaceNode) bool {
		if len(n.Events) > 0 && !n.HasCoords {
			s.unknown++
		}
		return true
	})
	m.fitMap()
}

// projectMarkers places the markers in the projection of the map.
func (m *Model) projectMarkers() {
	s := &m.mapv
	for i, p := range s.places {
		s.markers[i].X, s.markers[i].Y = s.view.Proj.Project(p.Lat, p.Lon)
	}
}

// cycleProjection shows the map in the next projection, centered where
// it was.
func (m *Model) cycleProjection() {
	s := &m.mapv
	lat, lon := s.view.Proj.Unproject(s.view.X, s.view.Y)
	s.view.Proj = s.view.Proj.Next()
	s.view.X, s.view.Y = s.view.Proj.Project(lat, max(-180, min(180, lon)))
	m.projectMarkers()
	m.clampMap()
	m.setStatus(s.view.Proj.String() + " projection: " + projectionNotes[s.view.Proj])
}

// projectionNotes say what each projection is good for.
var projectionNotes = map[worldmap.Projection]string{
	worldmap.Equirectangular: "latitudes evenly spaced, lands wider towards the poles",
	worldmap.EqualEarth:      "areas true, meridians curving towards the poles",
	worldmap.WebMercator:     "shapes true, sizes growing towards the poles",
}

// fitMap shows all located places, or the whole world if there are none.
func (m *Model) fitMap() {
	s := &m.mapv
	if len(s.markers) == 0 {
		m.showWorld()
		return
	}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, mk := range s.markers {
		minX, maxX = math.Min(minX, mk.X), math.Max(maxX, mk.X)
		minY, maxY = math.Min(minY, mk.Y), math.Max(maxY, mk.Y)
	}
	s.view = s.view.Proj.Fit(minX, minY, maxX, maxY, m.mapCols(), m.mapRows())
	m.clampMap()
}

// showWorld shows all of the map: the latitudes between 80°N and 60°S.
func (m *Model) showWorld() {
	m.mapv.view = m.mapv.view.Proj.WholeWorld(m.mapCols(), m.mapRows())
}

// selectMarker selects a place and centers the map on it.
func (m *Model) selectMarker(i int) {
	s := &m.mapv
	if len(s.places) == 0 {
		return
	}
	s.sel = (i + len(s.places)) % len(s.places)
	s.view.X, s.view.Y = s.markers[s.sel].X, s.markers[s.sel].Y
	m.clampMap()
}

// selectMapPlace selects the located place n or, if n has no coordinates,
// fits the map to the located places within it.
func (m *Model) selectMapPlace(n *genealogy.PlaceNode) {
	s := &m.mapv
	for i, p := range s.places {
		if p == n {
			s.view.Zoom = math.Max(s.view.Zoom, 7)
			m.selectMarker(i)
			return
		}
	}
	inside := n.Located()
	if len(inside) == 0 {
		m.setStatus(n.Full + " has no coordinates (add PLAC.MAP.LATI/LONG to the file)")
		return
	}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range inside {
		x, y := s.view.Proj.Project(p.Lat, p.Lon)
		minX, maxX = math.Min(minX, x), math.Max(maxX, x)
		minY, maxY = math.Min(minY, y), math.Max(maxY, y)
	}
	s.view = s.view.Proj.Fit(minX, minY, maxX, maxY, m.mapCols(), m.mapRows())
	m.clampMap()
	for i, p := range s.places {
		if p == inside[0] {
			s.sel = i
		}
	}
}

func (m *Model) updateMap(msg tea.KeyMsg) (bool, tea.Cmd) {
	s := &m.mapv
	spanX, spanY := s.view.Span(m.mapCols(), m.mapRows())
	key := msg.String()
	if key == "s" {
		m.toggleSurnames()
		return true, nil
	}
	if s.sn.on {
		switch key {
		case "pgdown", "pgup":
			dir := 1
			if key == "pgup" {
				dir = -1
			}
			m.setSurname(s.sn.name + dir)
			return true, nil
		case "e":
			m.cycleKind()
			return true, nil
		case "n", " ":
			m.stepSurnamePlace(1)
			return true, nil
		case "N", "p":
			m.stepSurnamePlace(-1)
			return true, nil
		case "0", "home":
			m.fitSurname()
			return true, nil
		}
	}
	switch key {
	case "left", "h":
		s.view.X -= spanX * panFraction
	case "right", "l":
		s.view.X += spanX * panFraction
	case "up", "k":
		s.view.Y -= spanY * panFraction
	case "down", "j":
		s.view.Y += spanY * panFraction
	case "+", "=":
		s.view.Zoom = math.Min(worldmap.MaxZoom, s.view.Zoom+zoomStep)
	case "-", "_":
		s.view.Zoom = math.Max(worldmap.MinZoom, s.view.Zoom-zoomStep)
	case "n", " ":
		m.selectMarker(s.sel + 1)
	case "N", "p":
		m.selectMarker(s.sel - 1)
	case "c":
		m.selectMarker(s.sel)
	case "0", "home":
		m.fitMap()
	case "w":
		m.showWorld()
	case "v":
		m.cycleProjection()
	case "enter":
		if len(s.places) == 0 {
			return true, nil
		}
		p := s.places[s.sel]
		m.switchTo(viewPlaces)
		m.selectPlace(p)
		m.places.detail = p
		m.places.events = p.AllEvents()
		m.places.evCursor, m.places.evOffset = 0, 0
	default:
		return false, nil
	}
	m.clampMap()
	return true, nil
}

// formatLatLon renders coordinates as "53.80°N 1.55°W".
func formatLatLon(lat, lon float64) string {
	ns, ew := "N", "E"
	if lat < 0 {
		ns, lat = "S", -lat
	}
	if lon < 0 {
		ew, lon = "W", -lon
	}
	return fmt.Sprintf("%.2f°%s %.2f°%s", lat, ns, lon, ew)
}

// mapStyles maps cell classes to styles.
func (m Model) mapStyles() map[worldmap.Class]lipgloss.Style {
	return map[worldmap.Class]lipgloss.Style{
		worldmap.ClassEmpty:      lipgloss.NewStyle(),
		worldmap.ClassOutline:    m.st.mapBorder,
		worldmap.ClassEquator:    m.st.mapEquator,
		worldmap.ClassBorder:     m.st.mapBorder,
		worldmap.ClassCoast:      m.st.mapCoast,
		worldmap.ClassLake:       m.st.mapWater,
		worldmap.ClassRiver:      m.st.mapWater,
		worldmap.ClassFaint:      m.st.dim,
		worldmap.ClassLabel:      m.st.name,
		worldmap.ClassMinorLabel: m.st.mapLabel,
		worldmap.ClassMarker:     m.st.mapMarker,
		worldmap.ClassSelected:   m.st.mapSelected,
	}
}

func (m Model) viewMap(h int) string {
	s := &m.mapv
	if s.err != nil {
		return m.st.err.Render("  the map data could not be loaded: " + s.err.Error())
	}
	rows := max(1, h-3)
	lat, lon := s.view.Proj.Unproject(s.view.X, s.view.Y)
	info := m.st.section.Render("Map") + m.st.dim.Render(fmt.Sprintf(" · %s · zoom %.1f · center %s · %s with coordinates",
		s.view.Proj, s.view.Zoom, formatLatLon(lat, max(-180, min(180, lon))), plural(len(s.places), "place", "places")))
	if s.unknown > 0 {
		info += m.st.dim.Render(fmt.Sprintf(", %d without", s.unknown))
	}
	var selLine string
	byName := s.sn.on
	switch {
	case len(s.places) == 0:
		selLine = m.st.dim.Render("  no place in this file has coordinates — they are read from PLAC.MAP.LATI and LONG")
	case byName:
		selLine = m.st.mapSelected.Render("◉") + " " + m.st.name.Render(s.places[s.sel].Full) + (&m).surnameInfo()
	default:
		p := s.places[s.sel]
		selLine = m.st.mapSelected.Render("◉") + " " + m.st.name.Render(p.Full) +
			m.st.dim.Render(fmt.Sprintf(" · %s · %s · %s", formatLatLon(p.Lat, p.Lon),
				plural(p.Count(), "event", "events"), plural(p.People(), "person", "people")))
	}
	if byName {
		sn := &s.sn
		info = m.st.section.Render("Map") + m.st.dim.Render(" · surname ") + m.st.mapMarker.Render(sn.names[sn.name].name) +
			m.st.dim.Render(fmt.Sprintf(" · %s born with it · %s at %s · ", plural(sn.names[sn.name].people, "person", "people"),
				eventKindNames[sn.kind], plural(sn.places[sn.names[sn.name].name], "place", "places"))) +
			m.st.key.Render("e") + m.st.dim.Render(" events · ") + m.st.key.Render("s") + m.st.dim.Render(" all places")
	}
	sel := s.sel
	if len(s.places) == 0 {
		sel = -1
	}
	markers := s.markers
	if byName {
		markers = (&m).surnameMarkers()
	}
	cols := (&m).mapCols()
	frame := s.world.Render(s.view, cols, rows, markers, sel)
	var panel []string
	if cols < m.width {
		panel = (&m).panelLines(rows, m.width-cols-3)
	}

	out := []string{fit(info, m.width), fit(selLine, m.width)}
	styles := m.mapStyles()
	for _, row := range frame.Cells {
		var b strings.Builder
		var run []rune
		cur := worldmap.ClassEmpty
		flush := func() {
			if len(run) > 0 {
				b.WriteString(styles[cur].Render(string(run)))
				run = run[:0]
			}
		}
		for _, c := range row {
			if c.Rune < 0 {
				continue // covered by the wide rune before it
			}
			if c.Class != cur {
				flush()
				cur = c.Class
			}
			if c.Rune == 0 {
				run = append(run, ' ')
			} else {
				run = append(run, c.Rune)
			}
		}
		flush()
		if cols < m.width {
			side := ""
			if r := len(out) - 2; r < len(panel) {
				side = panel[r]
			}
			b.WriteString(m.st.dim.Render(" │ ") + side)
		}
		out = append(out, b.String())
	}
	out = append(out, fit(m.st.dim.Render("  Map data: "+worldmap.Attribution+" (public domain) · press L for licenses"), m.width))
	return strings.Join(out, "\n")
}
