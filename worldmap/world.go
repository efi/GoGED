package worldmap

import (
	_ "embed"
	"math"
	"sync"
)

//go:embed world.bin
var embedded []byte

// Attribution is the short credit shown with the map.
const Attribution = "Made with Natural Earth"

// License describes the terms of the embedded map data.
const License = `Map data: Natural Earth (https://www.naturalearthdata.com/)
1:10m coastline, land boundaries, rivers and lake centerlines (with the
regional supplements for Europe, North America and Australia), and lakes,
simplified for display in the terminal.

All versions of Natural Earth raster and vector map data are in the
public domain. You may use the maps in any manner, including modifying
the content and design, electronic dissemination, and offset printing.
The primary authors, Tom Patterson and Nathaniel Vaughn Kelso, and all
other contributors renounce all financial claim to the maps and invite
you to use them for personal, educational, and commercial purposes.

No permission is needed to use Natural Earth. Crediting the authors is
unnecessary; goged credits them anyway: Made with Natural Earth. Free
vector and raster map data @ naturalearthdata.com.

Borders show the de facto situation as published by Natural Earth and
do not imply any position on disputed territories.`

// segment is a piece of a polyline on the plane of a projection, with its
// bounding box. Long lines are split so off-screen parts can be skipped.
type segment struct {
	kind                   Kind
	minZoom                uint8
	x, y                   []float32
	minX, minY, maxX, maxY float32
}

// World holds map features ready for rendering: the lines in degrees, and
// on the plane of each projection once it has been drawn.
type World struct {
	lines []Polyline
	once  [projections]sync.Once
	segs  [projections][]segment
}

const maxSegmentPoints = 64

// New prepares polylines for rendering.
func New(lines []Polyline) *World { return &World{lines: lines} }

// Prepare projects the lines for p, if that has not been done yet, so
// that the first map in that projection is drawn as fast as the others.
// It may run alongside Render.
func (w *World) Prepare(p Projection) { w.segments(p) }

// segments returns the lines projected by p, projecting them the first
// time.
func (w *World) segments(p Projection) []segment {
	w.once[p].Do(func() {
		for _, l := range w.lines {
			for start := 0; start < len(l.Lon)-1; start += maxSegmentPoints - 1 {
				end := min(start+maxSegmentPoints, len(l.Lon))
				s := segment{kind: l.Kind, minZoom: l.MinZoom, x: make([]float32, 0, end-start), y: make([]float32, 0, end-start)}
				s.minX, s.minY = math.MaxFloat32, math.MaxFloat32
				s.maxX, s.maxY = -math.MaxFloat32, -math.MaxFloat32
				for i := start; i < end; i++ {
					x, y := p.Project(l.Lat[i], l.Lon[i])
					fx, fy := float32(x), float32(y)
					s.x = append(s.x, fx)
					s.y = append(s.y, fy)
					s.minX, s.maxX = min(s.minX, fx), max(s.maxX, fx)
					s.minY, s.maxY = min(s.minY, fy), max(s.maxY, fy)
				}
				w.segs[p] = append(w.segs[p], s)
			}
		}
	})
	return w.segs[p]
}

var (
	loadOnce  sync.Once
	loaded    *World
	errLoaded error
)

// Load decodes the embedded map data. It is decoded once and shared.
func Load() (*World, error) {
	loadOnce.Do(func() {
		lines, err := Decode(embedded)
		if err != nil {
			errLoaded = err
			return
		}
		loaded = New(lines)
	})
	return loaded, errLoaded
}
