package worldmap

import (
	"math"

	"github.com/mattn/go-runewidth"
)

// View is the visible part of the map: its center in Web Mercator
// coordinates and a zoom level. At zoom z the whole world is 256·2^z
// Braille dots wide; every terminal cell holds 2×4 dots.
type View struct {
	X, Y float64
	Zoom float64
}

// Zoom limits.
const (
	MinZoom = -1.0
	MaxZoom = 14.0
)

// dotsPerWorld returns the width of the world in dots.
func (v View) dotsPerWorld() float64 { return 256 * math.Pow(2, v.Zoom) }

// Span returns the width and height of the view in Web Mercator units for
// a screen of cols×rows cells.
func (v View) Span(cols, rows int) (w, h float64) {
	d := v.dotsPerWorld()
	return float64(cols*2) / d, float64(rows*4) / d
}

// Locate returns the cell that shows the Web Mercator point (x, y).
func (v View) Locate(x, y float64, cols, rows int) (col, row int, ok bool) {
	d := v.dotsPerWorld()
	dx := (x-v.X)*d + float64(cols)
	dy := (y-v.Y)*d + float64(rows*2)
	col, row = int(math.Floor(dx/2)), int(math.Floor(dy/4))
	return col, row, col >= 0 && row >= 0 && col < cols && row < rows
}

// Fit returns a view that shows the Web Mercator box with a small margin.
// A degenerate box (a single place) is shown at a regional zoom level.
func Fit(minX, minY, maxX, maxY float64, cols, rows int) View {
	v := View{X: (minX + maxX) / 2, Y: (minY + maxY) / 2, Zoom: 7}
	w, h := (maxX-minX)*1.25, (maxY-minY)*1.25
	if w <= 0 && h <= 0 {
		return v
	}
	zx, zy := MaxZoom, MaxZoom
	if w > 0 {
		zx = math.Log2(float64(cols*2) / (256 * w))
	}
	if h > 0 {
		zy = math.Log2(float64(rows*4) / (256 * h))
	}
	v.Zoom = max(MinZoom, min(MaxZoom, math.Min(zx, zy), 10))
	return v
}

// Class tells the user interface how to color a cell.
type Class uint8

// Cell classes from lowest to highest priority.
const (
	ClassEmpty Class = iota
	ClassBorder
	ClassCoast
	ClassLake
	ClassRiver
	ClassMinorLabel
	ClassLabel
	ClassMarker
	ClassSelected
)

var kindClass = [kindCount]Class{Border: ClassBorder, Coastline: ClassCoast, Lake: ClassLake, River: ClassRiver}

// Cell is one character of a rendered map.
type Cell struct {
	Rune  rune
	Class Class
}

// Marker is a place to highlight on the map.
type Marker struct {
	X, Y  float64 // Web Mercator coordinates
	Label string  // name shown next to the marker if there is room
}

// maxMinorLabels limits labeling of unselected markers to views that do not
// show too many places at once.
const maxMinorLabels = 40

// Frame is a rendered map.
type Frame struct {
	Cols, Rows int
	Cells      [][]Cell // [row][col]
	// MarkerCells holds the cell of every marker, or -1/-1 if it is off
	// screen.
	MarkerCells [][2]int
}

// String returns the frame as plain text.
func (f Frame) String() string {
	b := make([]rune, 0, (f.Cols+1)*f.Rows)
	for _, row := range f.Cells {
		for _, c := range row {
			switch {
			case c.Rune == 0:
				b = append(b, ' ')
			case c.Rune > 0:
				b = append(b, c.Rune)
			}
		}
		b = append(b, '\n')
	}
	return string(b)
}

// canvas is a Braille dot canvas.
type canvas struct {
	cols, rows int
	w, h       int // in dots
	dots       []uint8
	class      []Class
}

// brailleBits maps a dot position within a cell (x 0-1, y 0-3) to its bit.
var brailleBits = [4][2]uint8{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}

func (c *canvas) set(x, y int, cl Class) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	i := (y/4)*c.cols + x/2
	c.dots[i] |= brailleBits[y%4][x%2]
	if cl > c.class[i] {
		c.class[i] = cl
	}
}

// clip restricts the segment (x0,y0)-(x1,y1) to the rectangle
// [0,w]×[0,h] (Liang-Barsky). It reports false if nothing is visible.
func clip(x0, y0, x1, y1, w, h float64) (float64, float64, float64, float64, bool) {
	t0, t1 := 0.0, 1.0
	dx, dy := x1-x0, y1-y0
	for _, e := range [4][2]float64{{-dx, x0}, {dx, w - x0}, {-dy, y0}, {dy, h - y0}} {
		p, q := e[0], e[1]
		if p == 0 {
			if q < 0 {
				return 0, 0, 0, 0, false
			}
			continue
		}
		r := q / p
		if p < 0 {
			if r > t1 {
				return 0, 0, 0, 0, false
			}
			t0 = max(t0, r)
		} else {
			if r < t0 {
				return 0, 0, 0, 0, false
			}
			t1 = min(t1, r)
		}
	}
	return x0 + t0*dx, y0 + t0*dy, x0 + t1*dx, y0 + t1*dy, true
}

// line draws a clipped segment with Bresenham's algorithm. Dotted lines
// set every other dot.
func (c *canvas) line(x0, y0, x1, y1 float64, cl Class, dotted bool) {
	x0, y0, x1, y1, ok := clip(x0, y0, x1, y1, float64(c.w)-0.001, float64(c.h)-0.001)
	if !ok {
		return
	}
	ix0, iy0, ix1, iy1 := int(x0), int(y0), int(x1), int(y1)
	dx, dy := abs(ix1-ix0), -abs(iy1-iy0)
	sx, sy := sign(ix1-ix0), sign(iy1-iy0)
	err := dx + dy
	for step := 0; ; step++ {
		if !dotted || step%2 == 0 {
			c.set(ix0, iy0, cl)
		}
		if ix0 == ix1 && iy0 == iy1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			ix0 += sx
		}
		if e2 <= dx {
			err += dx
			iy0 += sy
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func sign(x int) int {
	switch {
	case x < 0:
		return -1
	case x > 0:
		return 1
	}
	return 0
}

// detailBias shows features slightly before the zoom level Natural Earth
// suggests for them, since a terminal shows a larger area per dot than a
// web map shows per pixel.
const detailBias = 1.5

// Render draws the map for a screen of cols×rows cells. Markers are drawn
// on top; the marker with index selected (or none if -1) is highlighted
// and always labeled. Other markers are labeled where there is room, in
// the order given, so the most important places should come first.
func (w *World) Render(v View, cols, rows int, markers []Marker, selected int) Frame {
	cols, rows = max(cols, 1), max(rows, 1)
	c := &canvas{cols: cols, rows: rows, w: cols * 2, h: rows * 4}
	c.dots = make([]uint8, cols*rows)
	c.class = make([]Class, cols*rows)

	d := v.dotsPerWorld()
	spanX, spanY := v.Span(cols, rows)
	left, top := v.X-spanX/2, v.Y-spanY/2
	right, bottom := left+spanX, top+spanY
	if w != nil {
		for i := range w.segs {
			s := &w.segs[i]
			if float64(s.minZoom) > v.Zoom+detailBias ||
				float64(s.maxX) < left || float64(s.minX) > right ||
				float64(s.maxY) < top || float64(s.minY) > bottom {
				continue
			}
			cl := kindClass[s.kind]
			px := (float64(s.x[0]) - left) * d
			py := (float64(s.y[0]) - top) * d
			for j := 1; j < len(s.x); j++ {
				nx := (float64(s.x[j]) - left) * d
				ny := (float64(s.y[j]) - top) * d
				c.line(px, py, nx, ny, cl, s.kind == Border)
				px, py = nx, ny
			}
		}
	}

	f := Frame{Cols: cols, Rows: rows, Cells: make([][]Cell, rows)}
	for r := range rows {
		f.Cells[r] = make([]Cell, cols)
		for col := range cols {
			i := r*cols + col
			if c.dots[i] != 0 {
				f.Cells[r][col] = Cell{Rune: rune(0x2800 + int(c.dots[i])), Class: c.class[i]}
			}
		}
	}

	// Markers: one dot per place, a digit where several share a cell.
	count := map[[2]int]int{}
	f.MarkerCells = make([][2]int, len(markers))
	for i, m := range markers {
		col, row, ok := v.Locate(m.X, m.Y, cols, rows)
		if !ok {
			f.MarkerCells[i] = [2]int{-1, -1}
			continue
		}
		f.MarkerCells[i] = [2]int{col, row}
		count[[2]int{col, row}]++
	}
	for pos, n := range count {
		r := '●'
		switch {
		case n > 9:
			r = '+'
		case n > 1:
			r = rune('0' + n)
		}
		f.Cells[pos[1]][pos[0]] = Cell{Rune: r, Class: ClassMarker}
	}
	if selected >= 0 && selected < len(markers) && f.MarkerCells[selected][0] >= 0 {
		col, row := f.MarkerCells[selected][0], f.MarkerCells[selected][1]
		f.Cells[row][col] = Cell{Rune: '◉', Class: ClassSelected}
		f.label(col, row, markers[selected].Label, ClassLabel)
	}
	if len(count) <= maxMinorLabels {
		for i, m := range markers {
			pos := f.MarkerCells[i]
			if i == selected || pos[0] < 0 || m.Label == "" || count[pos] > 1 {
				continue
			}
			f.label(pos[0], pos[1], m.Label, ClassMinorLabel)
		}
	}
	return f
}

// label writes text next to a marker: to its right if it fits, else to
// its left. Minor labels are only placed where they cover no other label
// or marker and are skipped if neither side has room; the selected label
// is truncated if necessary.
func (f *Frame) label(col, row int, text string, cl Class) {
	if text == "" {
		return
	}
	runes := []rune(" " + text + " ")
	width := runewidth.StringWidth(string(runes))
	free := func(start int) bool {
		if start < 0 || start+width > f.Cols {
			return false
		}
		if cl != ClassMinorLabel {
			return true
		}
		for x := start; x < start+width; x++ {
			if f.Cells[row][x].Class >= ClassMinorLabel {
				return false
			}
		}
		return true
	}
	start := col + 1
	switch {
	case free(col + 1):
	case free(col - width):
		start = col - width
	case cl == ClassMinorLabel:
		return
	case col+1+width > f.Cols && col-width >= 0:
		start = col - width
	case col+1+width > f.Cols:
		start = 0
		runes = []rune(runewidth.Truncate(string(runes), f.Cols-1, "…"))
	}
	x := start
	for _, r := range runes {
		rw := runewidth.RuneWidth(r)
		if x+rw > f.Cols || x == col {
			break
		}
		f.Cells[row][x] = Cell{Rune: r, Class: cl}
		for k := 1; k < rw; k++ {
			f.Cells[row][x+k] = Cell{Rune: -1, Class: cl} // covered by a wide rune
		}
		x += rw
	}
}
