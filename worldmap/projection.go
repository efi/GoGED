package worldmap

import "math"

// Projection maps latitudes and longitudes onto the plane of the map: x
// runs from 0 at 180°W to 1 at 180°E, y grows towards the south in the
// same units, with the equator at 0.5.
type Projection uint8

// Projections. The zero value is the default.
const (
	// Equirectangular spaces the latitudes evenly; lands get wider
	// towards the poles.
	Equirectangular Projection = iota
	// EqualEarth keeps the areas true, its meridians curving towards the
	// poles (Šavrič, Patterson and Jenny, 2018).
	EqualEarth
	// WebMercator keeps the shapes true, as web maps do; sizes grow
	// towards the poles.
	WebMercator
	projections
)

// String returns the name of the projection.
func (p Projection) String() string {
	return [projections]string{"Equirectangular", "Equal Earth", "Web Mercator"}[p%projections]
}

// Next returns the projection after p, and after the last the first.
func (p Projection) Next() Projection { return (p + 1) % projections }

// maxLat is the latitude limit of the Web Mercator projection.
const maxLat = 85.05112878

// Coefficients of the Equal Earth projection.
const (
	eeA1, eeA2, eeA3, eeA4 = 1.340264, -0.081106, 0.000893, 0.003796
)

// sqrt3 is √3, which Equal Earth uses throughout.
const sqrt3 = 1.7320508075688772

// eeHalfWidth is the distance from the central meridian to 180° on the
// equator, in the projection's own units.
const eeHalfWidth = 2 * sqrt3 * math.Pi / (3 * eeA1)

// Project converts a latitude and longitude in degrees to the plane of the
// map.
func (p Projection) Project(lat, lon float64) (x, y float64) {
	lat = max(-90, min(90, lat))
	switch p {
	case WebMercator:
		lat = max(-maxLat, min(maxLat, lat))
		s := math.Sin(lat * math.Pi / 180)
		return (lon + 180) / 360, 0.5 - math.Log((1+s)/(1-s))/(4*math.Pi)
	case EqualEarth:
		phi, lambda := lat*math.Pi/180, lon*math.Pi/180
		theta := math.Asin(sqrt3 / 2 * math.Sin(phi))
		t2 := theta * theta
		t6 := t2 * t2 * t2
		ex := 2 * sqrt3 * lambda * math.Cos(theta) / (3 * (9*eeA4*t6*t2 + 7*eeA3*t6 + 3*eeA2*t2 + eeA1))
		ey := theta * (eeA1 + eeA2*t2 + t6*(eeA3+eeA4*t2))
		return (ex + eeHalfWidth) / (2 * eeHalfWidth), 0.5 - ey/(2*eeHalfWidth)
	}
	return (lon + 180) / 360, 0.5 - lat/360
}

// Unproject converts a point of the map back to degrees. Beyond the edge
// of the world, the longitude is beyond ±180°.
func (p Projection) Unproject(x, y float64) (lat, lon float64) {
	switch p {
	case WebMercator:
		return math.Atan(math.Sinh(math.Pi*(1-2*y))) * 180 / math.Pi, x*360 - 180
	case EqualEarth:
		ex, ey := (2*x-1)*eeHalfWidth, (1-2*y)*eeHalfWidth
		// Newton's method finds the parametric latitude θ.
		theta := ey / eeA1
		for range 20 {
			t2 := theta * theta
			t6 := t2 * t2 * t2
			f := theta*(eeA1+eeA2*t2+t6*(eeA3+eeA4*t2)) - ey
			d := eeA1 + 3*eeA2*t2 + t6*(7*eeA3+9*eeA4*t2)
			step := f / d
			theta -= step
			if math.Abs(step) < 1e-12 {
				break
			}
		}
		theta = max(-math.Pi/3, min(math.Pi/3, theta))
		t2 := theta * theta
		t6 := t2 * t2 * t2
		lambda := ex * 3 * (9*eeA4*t6*t2 + 7*eeA3*t6 + 3*eeA2*t2 + eeA1) / (2 * sqrt3 * math.Cos(theta))
		return math.Asin(max(-1, min(1, 2*math.Sin(theta)/sqrt3))) * 180 / math.Pi, lambda * 180 / math.Pi
	}
	return (0.5 - y) * 360, x*360 - 180
}

// The map is drawn from North to South only: beyond, there is mostly ice,
// which Web Mercator stretches the most.
const (
	North = 80.0
	South = -60.0
)

// band returns North and South on the plane of the map.
func (p Projection) band() (top, bottom float64) {
	_, top = p.Project(North, 0)
	_, bottom = p.Project(South, 0)
	return top, bottom
}
