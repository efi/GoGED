// Command mkworld converts Natural Earth GeoJSON layers into the compact
// map data embedded in goged:
//
//	go run ./worldmap/mkworld -o worldmap/world.bin \
//	    -coast ne_10m_coastline.geojson \
//	    -borders ne_10m_admin_0_boundary_lines_land.geojson \
//	    -rivers ne_10m_rivers_lake_centerlines.geojson,ne_10m_rivers_europe.geojson,ne_10m_rivers_north_america.geojson,ne_10m_rivers_australia.geojson \
//	    -lakes ne_10m_lakes.geojson
//
// Every layer accepts a comma-separated list of files; the regional river
// supplements add smaller rivers to the global layer.
//
// Natural Earth data is public domain: https://www.naturalearthdata.com/
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/efi/goged/worldmap"
)

type feature struct {
	Properties map[string]any `json:"properties"`
	Geometry   *struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	} `json:"geometry"`
}

// lines extracts the line strings of a geometry; polygon rings count as
// lines.
func lines(f feature) ([][][2]float64, error) {
	if f.Geometry == nil {
		return nil, nil
	}
	c := f.Geometry.Coordinates
	switch f.Geometry.Type {
	case "LineString":
		var l [][2]float64
		err := json.Unmarshal(c, &l)
		return [][][2]float64{l}, err
	case "MultiLineString", "Polygon":
		var ls [][][2]float64
		err := json.Unmarshal(c, &ls)
		return ls, err
	case "MultiPolygon":
		var ps [][][][2]float64
		if err := json.Unmarshal(c, &ps); err != nil {
			return nil, err
		}
		var out [][][2]float64
		for _, p := range ps {
			out = append(out, p...)
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported geometry %s", f.Geometry.Type)
}

func number(props map[string]any, key string) (float64, bool) {
	v, ok := props[key].(float64)
	return v, ok
}

// simplify applies the Douglas-Peucker algorithm with tolerance eps (in
// degrees).
func simplify(pts [][2]float64, eps float64) [][2]float64 {
	if len(pts) <= 2 || eps <= 0 {
		return pts
	}
	keep := make([]bool, len(pts))
	keep[0], keep[len(pts)-1] = true, true
	type span struct{ a, b int }
	stack := []span{{0, len(pts) - 1}}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		ax, ay := pts[s.a][0], pts[s.a][1]
		bx, by := pts[s.b][0], pts[s.b][1]
		dx, dy := bx-ax, by-ay
		length := math.Hypot(dx, dy)
		best, bestDist := -1, eps
		for i := s.a + 1; i < s.b; i++ {
			px, py := pts[i][0], pts[i][1]
			var d float64
			if length == 0 {
				d = math.Hypot(px-ax, py-ay)
			} else {
				d = math.Abs(dy*px-dx*py+bx*ay-by*ax) / length
			}
			if d > bestDist {
				best, bestDist = i, d
			}
		}
		if best >= 0 {
			keep[best] = true
			stack = append(stack, span{s.a, best}, span{best, s.b})
		}
	}
	var out [][2]float64
	for i, k := range keep {
		if k {
			out = append(out, pts[i])
		}
	}
	return out
}

type layer struct {
	kind     worldmap.Kind
	paths    string // comma-separated GeoJSON files
	eps      float64
	maxRank  float64 // skip features with a larger scalerank (0 = keep all)
	baseZoom uint8   // min zoom for features without a min_zoom property
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "mkworld:", err)
		os.Exit(1)
	}
}

func run(args []string, log io.Writer) error {
	fs := flag.NewFlagSet("mkworld", flag.ContinueOnError)
	out := fs.String("o", "world.bin", "output file")
	coast := fs.String("coast", "", "coastline GeoJSON (comma-separated `files`)")
	borders := fs.String("borders", "", "land boundary GeoJSON (comma-separated `files`)")
	rivers := fs.String("rivers", "", "rivers GeoJSON (comma-separated `files`)")
	lakes := fs.String("lakes", "", "lakes GeoJSON (comma-separated `files`)")
	eps := fs.Float64("epsilon", 0.005, "simplification tolerance in degrees")
	if err := fs.Parse(args); err != nil {
		return err
	}

	layers := []layer{
		{worldmap.Border, *borders, *eps, 0, 0},
		{worldmap.Coastline, *coast, *eps, 0, 0},
		{worldmap.Lake, *lakes, *eps, 7, 3},
		{worldmap.River, *rivers, *eps, 0, 3},
	}
	var all []worldmap.Polyline
	for _, ly := range layers {
		if ly.paths == "" {
			continue
		}
		before, after, count := 0, 0, 0
		for _, path := range strings.Split(ly.paths, ",") {
			data, err := os.ReadFile(strings.TrimSpace(path))
			if err != nil {
				return err
			}
			var fc struct {
				Features []feature `json:"features"`
			}
			if err := json.Unmarshal(data, &fc); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			for _, f := range fc.Features {
				if r, ok := number(f.Properties, "scalerank"); ok && ly.maxRank > 0 && r > ly.maxRank {
					continue
				}
				zoom := ly.baseZoom
				if z, ok := number(f.Properties, "min_zoom"); ok && z >= 0 {
					zoom = uint8(math.Round(z))
				}
				ls, err := lines(f)
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				for _, l := range ls {
					before += len(l)
					s := simplify(l, ly.eps)
					if len(s) < 2 {
						continue
					}
					p := worldmap.Polyline{Kind: ly.kind, MinZoom: zoom}
					for _, pt := range s {
						p.Lon = append(p.Lon, pt[0])
						p.Lat = append(p.Lat, pt[1])
					}
					after += len(s)
					count++
					all = append(all, p)
				}
			}
		}
		fmt.Fprintf(log, "%-10s %6d lines %8d -> %7d points\n", ly.kind, count, before, after)
	}
	var buf bytes.Buffer
	if err := worldmap.Encode(&buf, all); err != nil {
		return err
	}
	if err := os.WriteFile(*out, buf.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(log, "wrote %s (%d bytes)\n", *out, buf.Len())
	return nil
}
