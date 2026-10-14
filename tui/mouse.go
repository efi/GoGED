package tui

import (
	"math"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/efi/goged/export"
	"github.com/efi/goged/worldmap"
	"github.com/mattn/go-runewidth"
)

// The mouse: a click selects, and a click on what is selected does what
// enter does. Tabs switch views, the wheel scrolls (and zooms the map),
// dragging pans the map, and the mouse's back and forward buttons go
// through the history.

const (
	headerRows = 2 // the title and the tab bar, above the body
	wheelLines = 3 // rows scrolled by a notch of the wheel
	wheelCols  = 6 // columns scrolled by a sideways notch
)

func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	x, y := msg.X, msg.Y-headerRows
	switch msg.Action {
	case tea.MouseActionMotion:
		if msg.Button == tea.MouseButtonLeft && m.mapv.dragging && m.active == viewMap && !m.showHelp {
			m.mapv.opening = false
			m.dragMap(x, y)
		}
		return nil
	case tea.MouseActionRelease:
		s := &m.mapv
		open := s.opening && m.active == viewMap && !m.showHelp
		s.dragging, s.opening = false, false
		if open {
			return m.press("enter")
		}
		return nil
	}
	m.status, m.statusErr = "", false
	if m.watch.asking {
		// The dialog only answers its buttons.
		if msg.Button == tea.MouseButtonLeft {
			return m.clickDialog(x, y)
		}
		return nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.wheel(0, -wheelLines, x, y)
	case tea.MouseButtonWheelDown:
		m.wheel(0, wheelLines, x, y)
	case tea.MouseButtonWheelLeft:
		m.wheel(-wheelCols, 0, x, y)
	case tea.MouseButtonWheelRight:
		m.wheel(wheelCols, 0, x, y)
	case tea.MouseButtonBackward:
		m.back()
	case tea.MouseButtonForward:
		m.forward()
	case tea.MouseButtonLeft:
		return m.click(x, y)
	}
	return nil
}

// press handles a key as if it was typed.
func (m *Model) press(key string) tea.Cmd {
	if key == "enter" {
		return m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	}
	return m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

// click handles the left button at column x of body row y; the header is
// above the body (y < 0).
func (m *Model) click(x, y int) tea.Cmd {
	switch {
	case y == -2:
		// The marked person, at the end of the title.
		start := ansi.StringWidth(m.title() + "  ")
		if m.reference != nil && x >= start {
			m.stopEditing()
			m.showHelp = false
			m.open(m.reference)
		}
		return nil
	case y == -1:
		col := 0
		for _, t := range m.tabs() {
			w := ansi.StringWidth(m.renderTab(t))
			if x >= col && x < col+w {
				m.stopEditing()
				m.showHelp = false
				if t.view >= 0 {
					m.switchTo(t.view)
				}
				return nil
			}
			col += w + 1
		}
		return nil
	case y >= m.bodyHeight():
		return nil // the footer
	case m.showHelp && m.overlay == "Changes":
		return m.clickLink(&m.help, y)
	case m.showHelp:
		m.showHelp = false
		return nil
	}

	switch m.active {
	case viewSearch:
		s := &m.search
		return m.clickRow(y-2, s.offset, len(s.results), &s.cursor, func() { m.moveSearch(0) })
	case viewPerson:
		return m.clickLink(&m.person.doc, y)
	case viewStats:
		return m.clickLink(&m.stats.doc, y)
	case viewTree:
		if i := m.treeNodeAt(x, y); i >= 0 {
			if i == m.tree.sel {
				return m.press("enter")
			}
			m.selectTreeNode(i)
		}
	case viewEvents:
		e := &m.ev
		if y == 0 {
			if !e.editing {
				return m.press("f")
			}
			return nil
		}
		m.stopEditing()
		return m.clickRow(y-2, e.offset, len(e.list), &e.cursor, func() { m.moveEvents(0) })
	case viewPlaces:
		return m.clickPlaces(x, y)
	case viewMap:
		m.clickMap(x, y)
	case viewChecks:
		c := &m.checks
		if r := c.offset + y - 2; y >= 2 && r < len(c.rows) && !c.rows[r].selectable() {
			return nil // a heading or a summary
		}
		return m.clickRow(y-2, c.offset, len(c.rows), &c.cursor, func() { m.moveChecks(0) })
	case viewExport:
		return m.clickExport(x, y)
	}
	return nil
}

// stopEditing ends the editing of a filter or file name, as enter does.
func (m *Model) stopEditing() {
	if m.typing() && m.active != viewSearch {
		m.press("enter")
	}
}

// clickRow handles a click on row i of a list shown from offset: it
// selects the entry or, if it is selected, does what enter does.
func (m *Model) clickRow(i, offset, n int, cursor *int, moved func()) tea.Cmd {
	if i < 0 || offset+i >= n {
		return nil
	}
	if offset+i == *cursor {
		return m.press("enter")
	}
	*cursor = offset + i
	moved()
	return nil
}

func (m *Model) clickLink(d *docView, y int) tea.Cmd {
	i := d.linkAt(y)
	if i < 0 {
		return nil
	}
	if i == d.cursor {
		return m.press("enter")
	}
	d.cursor = i
	d.ensureVisible(m.bodyHeight())
	return nil
}

// treeNodeAt returns the chart node shown at column x of body row y, or -1.
func (m *Model) treeNodeAt(x, y int) int {
	t := &m.tree
	if t.chart == nil || y < 1 || t.top+y-1 >= len(t.chart.Lines) {
		return -1
	}
	col, at := 0, t.left+x
	for _, seg := range t.chart.Lines[t.top+y-1] {
		w := runewidth.StringWidth(seg.Text)
		if at >= col && at < col+w {
			return seg.Node
		}
		col += w
	}
	return -1
}

func (m *Model) clickPlaces(x, y int) tea.Cmd {
	p := &m.places
	if p.root == nil {
		return nil
	}
	if p.detail != nil {
		// The events follow the header, the place's details and a hint
		// line; a click on the hint returns to the places.
		start := 2 + len(placeInfo(p.detail))
		if y == start-1 {
			return m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
		}
		return m.clickRow(y-start, p.evOffset, len(p.events), &p.evCursor, func() { m.movePlaceEvents(0) })
	}
	if y == 0 {
		if !p.editing {
			return m.press("f")
		}
		return nil
	}
	m.stopEditing()
	i := p.offset + y - 2
	if y < 2 || i >= len(p.rows) {
		return nil
	}
	// The marker before a place with places within it folds and unfolds it.
	n := p.rows[i]
	if marker := 2 * n.Depth; x >= marker && x < marker+2 && len(n.Children) > 0 && !m.filteringPlaces() {
		p.cursor = i
		p.collapsed[n] = !p.collapsed[n]
		m.refreshPlaces()
		return nil
	}
	return m.clickRow(y-2, p.offset, len(p.rows), &p.cursor, func() { m.movePlaces(0) })
}

// clickMap selects the place whose marker is at or next to the cell
// clicked. The press also starts dragging; on the selected place, a release
// without dragging opens it.
func (m *Model) clickMap(x, y int) {
	s := &m.mapv
	if s.world == nil || len(s.places) == 0 {
		return
	}
	if y == 1 {
		// The line about the selected place.
		m.press("enter")
		return
	}
	row := y - 2
	if row < 0 || row >= m.mapRows() {
		return
	}
	s.dragging, s.dragX, s.dragY = true, x, y
	best, dist := -1, 2
	for i, mk := range s.markers {
		col, r, ok := s.view.Locate(mk.X, mk.Y, m.width, m.mapRows())
		if d := max(col-x, x-col, r-row, row-r); ok && d < dist {
			best, dist = i, d
		}
	}
	switch best {
	case -1:
	case s.sel:
		s.opening = true
	default:
		s.sel = best
	}
}

// dragMap moves the map with the mouse.
func (m *Model) dragMap(x, y int) {
	s := &m.mapv
	w, h := s.view.Span(m.width, m.mapRows())
	s.view.X -= float64(x-s.dragX) * w / float64(m.width)
	s.view.Y -= float64(y-s.dragY) * h / float64(m.mapRows())
	s.dragX, s.dragY = x, y
	m.clampMap()
}

func (m *Model) clampMap() {
	s := &m.mapv
	s.view.X = math.Max(0, math.Min(1, s.view.X))
	s.view.Y = math.Max(0, math.Min(1, s.view.Y))
}

// mapPoint returns the Web Mercator point in the middle of a map cell.
func (m *Model) mapPoint(col, row int) (x, y float64) {
	v, cols, rows := m.mapv.view, m.width, m.mapRows()
	w, h := v.Span(cols, rows)
	return v.X + (float64(col)+0.5-float64(cols)/2)*w/float64(cols),
		v.Y + (float64(row)+0.5-float64(rows)/2)*h/float64(rows)
}

func (m *Model) clickExport(x, y int) tea.Cmd {
	e := &m.export
	if e.editing && y == m.exportRowY(exportRowFile) {
		return nil
	}
	m.stopEditing()
	row := -1
	for r := range exportRows {
		if m.exportRowY(r) == y {
			row = r
		}
	}
	if row < 0 {
		return nil
	}
	// The arrows around a choice change it at once.
	if row == exportRowFormat || row == exportRowPeople {
		value := m.exportChoice(row)
		switch end := exportValueX + ansi.StringWidth(value); {
		case x >= exportValueX && x < exportValueX+2:
			e.row = row
			return m.press("h")
		case x >= end-2 && x < end:
			e.row = row
			return m.press("l")
		}
	}
	if row == e.row {
		return m.press("enter")
	}
	e.row, e.confirm = row, ""
	return nil
}

// exportValueX is the column where the values of the export settings start.
const exportValueX = 14

// exportRowY returns the body row of an export setting, as viewExport lays
// them out.
func (m *Model) exportRowY(row int) int {
	y := 2 + row
	if row >= exportRowFile && m.reference == nil {
		y++ // a hint follows the people
	}
	if row == exportRowExport {
		y++ // a blank line comes before the button
	}
	return y
}

// exportChoice returns the value shown for the format or people row.
func (m *Model) exportChoice(row int) string {
	e := &m.export
	formats := m.exportFormats()
	if row == exportRowFormat {
		if len(formats) > 1 {
			return "◂ " + formats[e.format].Name + " ▸"
		}
		return formats[e.format].Name
	}
	people := "all"
	if e.scope == export.Component && m.reference != nil {
		people = "connected to ★ " + m.reference.DisplayName()
	}
	return "◂ " + people + " ▸"
}

// wheel scrolls the view under the mouse by dy rows or dx columns; on the
// map it zooms in or out around the mouse at column x of body row y.
func (m *Model) wheel(dx, dy, x, y int) {
	h := m.bodyHeight()
	if m.showHelp {
		m.help.scroll(dy, h)
		return
	}
	switch m.active {
	case viewSearch:
		s := &m.search
		scrollList(&s.cursor, &s.offset, len(s.results), m.searchListHeight(), dy)
	case viewPerson:
		m.person.doc.scrollBy(dy, h)
	case viewStats:
		m.stats.doc.scrollBy(dy, h)
	case viewTree:
		t := &m.tree
		if t.chart == nil {
			return
		}
		t.top = clamp(t.top+dy, 0, max(0, len(t.chart.Lines)-m.treeHeight()))
		t.left = clamp(t.left+dx, 0, max(0, t.chart.Width-m.width+2))
	case viewEvents:
		e := &m.ev
		scrollList(&e.cursor, &e.offset, len(e.list), m.eventsListHeight(), dy)
	case viewPlaces:
		p := &m.places
		if p.detail != nil {
			scrollList(&p.evCursor, &p.evOffset, len(p.events), m.placesListHeight(), dy)
		} else {
			scrollList(&p.cursor, &p.offset, len(p.rows), m.placesListHeight(), dy)
		}
	case viewMap:
		m.zoomMap(dx, dy, x, y-2)
	case viewChecks:
		c := &m.checks
		scrollList(&c.cursor, &c.offset, len(c.rows), m.checksHeight(), dy)
	}
}

// zoomMap zooms in (dy < 0) or out, keeping the point under the mouse
// where it is; sideways it pans.
func (m *Model) zoomMap(dx, dy, col, row int) {
	s := &m.mapv
	if s.world == nil {
		return
	}
	w, _ := s.view.Span(m.width, m.mapRows())
	s.view.X += float64(dx) * w / float64(m.width)
	if dy != 0 {
		if row < 0 || row >= m.mapRows() {
			col, row = m.width/2, m.mapRows()/2
		}
		x0, y0 := m.mapPoint(col, row)
		zoom := s.view.Zoom - math.Copysign(zoomStep, float64(dy))
		s.view.Zoom = math.Max(worldmap.MinZoom, math.Min(worldmap.MaxZoom, zoom))
		x1, y1 := m.mapPoint(col, row)
		s.view.X += x0 - x1
		s.view.Y += y0 - y1
	}
	m.clampMap()
}

// scrollList scrolls a list of n rows shown h at a time by delta rows. If
// the selected row leaves the screen, the nearest visible one is selected.
func scrollList(cursor, offset *int, n, h, delta int) {
	*offset = clamp(*offset+delta, 0, max(0, n-h))
	*cursor = clamp(*cursor, *offset, min(n, *offset+h)-1)
}
