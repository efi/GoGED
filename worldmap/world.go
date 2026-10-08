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

// segment is a piece of a polyline in Web Mercator coordinates with its
// bounding box. Long lines are split so off-screen parts can be skipped.
type segment struct {
	kind                   Kind
	minZoom                uint8
	x, y                   []float32
	minX, minY, maxX, maxY float32
}

// World holds map features ready for rendering.
type World struct {
	segs []segment
}

const maxSegmentPoints = 64

// New prepares polylines for rendering.
func New(lines []Polyline) *World {
	w := &World{}
	for _, l := range lines {
		for start := 0; start < len(l.Lon)-1; start += maxSegmentPoints - 1 {
			end := min(start+maxSegmentPoints, len(l.Lon))
			s := segment{kind: l.Kind, minZoom: l.MinZoom}
			s.minX, s.minY = math.MaxFloat32, math.MaxFloat32
			s.maxX, s.maxY = -math.MaxFloat32, -math.MaxFloat32
			for i := start; i < end; i++ {
				x, y := Project(l.Lat[i], l.Lon[i])
				fx, fy := float32(x), float32(y)
				s.x = append(s.x, fx)
				s.y = append(s.y, fy)
				s.minX, s.maxX = min(s.minX, fx), max(s.maxX, fx)
				s.minY, s.maxY = min(s.minY, fy), max(s.maxY, fy)
			}
			w.segs = append(w.segs, s)
		}
	}
	return w
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

// maxLat is the latitude limit of the Web Mercator projection.
const maxLat = 85.05112878

// Project converts a latitude and longitude in degrees to Web Mercator
// coordinates in [0, 1]: x grows towards the east, y towards the south.
func Project(lat, lon float64) (x, y float64) {
	lat = max(-maxLat, min(maxLat, lat))
	x = (lon + 180) / 360
	s := math.Sin(lat * math.Pi / 180)
	y = 0.5 - math.Log((1+s)/(1-s))/(4*math.Pi)
	return x, y
}

// Unproject converts Web Mercator coordinates back to degrees.
func Unproject(x, y float64) (lat, lon float64) {
	lon = x*360 - 180
	lat = math.Atan(math.Sinh(math.Pi*(1-2*y))) * 180 / math.Pi
	return lat, lon
}
