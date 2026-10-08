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

// mapState drives the (experimental) map view: a zoomable world map with
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
	s.world, s.err = worldmap.Load()
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
		s.markers[i].X, s.markers[i].Y = worldmap.Project(p.Lat, p.Lon)
		s.markers[i].Label = p.Name
	}
	m.places.root.Walk(func(n *genealogy.PlaceNode) bool {
		if len(n.Events) > 0 && !n.HasCoords {
			s.unknown++
		}
		return true
	})
	m.fitMap()
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
	s.view = worldmap.Fit(minX, minY, maxX, maxY, m.width, m.mapRows())
}

func (m *Model) showWorld() {
	m.mapv.view = worldmap.Fit(0, 0.1, 1, 0.9, m.width, m.mapRows())
}

// selectMarker selects a place and centers the map on it.
func (m *Model) selectMarker(i int) {
	s := &m.mapv
	if len(s.places) == 0 {
		return
	}
	s.sel = (i + len(s.places)) % len(s.places)
	s.view.X, s.view.Y = s.markers[s.sel].X, s.markers[s.sel].Y
}

// selectMapPlace selects the located place n or, if n has no coordinates,
// fits the map to the located places within it.
func (m *Model) selectMapPlace(n *genealogy.PlaceNode) {
	s := &m.mapv
	for i, p := range s.places {
		if p == n {
			m.selectMarker(i)
			s.view.Zoom = math.Max(s.view.Zoom, 7)
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
		x, y := worldmap.Project(p.Lat, p.Lon)
		minX, maxX = math.Min(minX, x), math.Max(maxX, x)
		minY, maxY = math.Min(minY, y), math.Max(maxY, y)
	}
	s.view = worldmap.Fit(minX, minY, maxX, maxY, m.width, m.mapRows())
	for i, p := range s.places {
		if p == inside[0] {
			s.sel = i
		}
	}
}

func (m *Model) updateMap(msg tea.KeyMsg) (bool, tea.Cmd) {
	s := &m.mapv
	spanX, spanY := s.view.Span(m.width, m.mapRows())
	switch msg.String() {
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
	s.view.X = math.Max(0, math.Min(1, s.view.X))
	s.view.Y = math.Max(0, math.Min(1, s.view.Y))
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
		worldmap.ClassBorder:     m.st.mapBorder,
		worldmap.ClassCoast:      m.st.mapCoast,
		worldmap.ClassLake:       m.st.mapWater,
		worldmap.ClassRiver:      m.st.mapWater,
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
	lat, lon := worldmap.Unproject(s.view.X, s.view.Y)
	info := m.st.section.Render("Map") + m.st.dim.Render(fmt.Sprintf(" (experimental) · zoom %.1f · center %s · %s with coordinates",
		s.view.Zoom, formatLatLon(lat, lon), plural(len(s.places), "place", "places")))
	if s.unknown > 0 {
		info += m.st.dim.Render(fmt.Sprintf(", %d without", s.unknown))
	}
	var selLine string
	if len(s.places) == 0 {
		selLine = m.st.dim.Render("  no place in this file has coordinates — they are read from PLAC.MAP.LATI and LONG")
	} else {
		p := s.places[s.sel]
		selLine = m.st.mapSelected.Render("◉") + " " + m.st.name.Render(p.Full) +
			m.st.dim.Render(fmt.Sprintf(" · %s · %s · %s", formatLatLon(p.Lat, p.Lon),
				plural(p.Count(), "event", "events"), plural(p.People(), "person", "people")))
	}
	sel := s.sel
	if len(s.places) == 0 {
		sel = -1
	}
	frame := s.world.Render(s.view, m.width, rows, s.markers, sel)

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
		out = append(out, b.String())
	}
	out = append(out, fit(m.st.dim.Render("  Map data: "+worldmap.Attribution+" (public domain) · press L for licenses"), m.width))
	return strings.Join(out, "\n")
}
