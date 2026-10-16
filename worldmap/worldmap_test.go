package worldmap

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	in := []Polyline{
		{Kind: Coastline, MinZoom: 0, Lon: []float64{-1.5492, -1.5, 10.786089}, Lat: []float64{53.7997, 53.8, 50.781464}},
		{Kind: River, MinZoom: 5, Lon: []float64{179.9999, -179.9999}, Lat: []float64{-85, 85}},
		{Kind: Border, Lon: []float64{0, 0}, Lat: []float64{0, 0}},
		{Kind: Lake, MinZoom: 3, Lon: []float64{}, Lat: []float64{}},
	}
	var buf bytes.Buffer
	if err := Encode(&buf, in); err != nil {
		t.Fatal(err)
	}
	out, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(in) {
		t.Fatalf("decoded %d lines", len(out))
	}
	for i := range in {
		if out[i].Kind != in[i].Kind || out[i].MinZoom != in[i].MinZoom || len(out[i].Lon) != len(in[i].Lon) {
			t.Fatalf("line %d = %+v, want %+v", i, out[i], in[i])
		}
		for j := range in[i].Lon {
			if math.Abs(out[i].Lon[j]-in[i].Lon[j]) > 0.5/scale || math.Abs(out[i].Lat[j]-in[i].Lat[j]) > 0.5/scale {
				t.Errorf("line %d point %d = %v,%v want %v,%v", i, j, out[i].Lat[j], out[i].Lon[j], in[i].Lat[j], in[i].Lon[j])
			}
		}
	}
	if err := Encode(&buf, []Polyline{{Lon: []float64{1}, Lat: nil}}); err == nil {
		t.Error("mismatched coordinates must fail")
	}
}

func TestDecodeErrors(t *testing.T) {
	var good bytes.Buffer
	Encode(&good, []Polyline{{Kind: River, Lon: []float64{1, 2, 3}, Lat: []float64{4, 5, 6}}})
	tests := map[string][]byte{
		"not gzip": []byte("hello"),
		"empty":    {},
	}
	for name, data := range tests {
		if _, err := Decode(data); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	// Valid gzip with bad content.
	for name, raw := range map[string]string{
		"bad magic":   "NOTAMAPX\x01\x00",
		"bad version": magic + "\x02\x00",
		"truncated":   magic + "\x01\x01\x03\x00\x05\x02",
		"bad kind":    magic + "\x01\x01\x09\x00\x00",
		"short":       "GOGED",
	} {
		var buf bytes.Buffer
		zw := newGzip(&buf)
		zw.Write([]byte(raw))
		zw.Close()
		if _, err := Decode(buf.Bytes()); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, err := Decode(good.Bytes()); err != nil {
		t.Errorf("good data: %v", err)
	}
}

func TestProjections(t *testing.T) {
	for p := range projections {
		// The equator is in the middle, 180°W and 180°E at the edges.
		if x, y := p.Project(0, 0); math.Abs(x-0.5) > 1e-12 || math.Abs(y-0.5) > 1e-12 {
			t.Errorf("%s: Equirectangular.Project(0, 0) = %v,%v", p, x, y)
		}
		if x, _ := p.Project(0, -180); math.Abs(x) > 1e-12 {
			t.Errorf("%s: 180°W at %v", p, x)
		}
		if x, _ := p.Project(0, 180); math.Abs(x-1) > 1e-12 {
			t.Errorf("%s: 180°E at %v", p, x)
		}
		if _, y := p.Project(50, 0); y >= 0.5 {
			t.Errorf("%s: north must be above the equator", p)
		}
		for _, pt := range [][2]float64{{53.7997, -1.5492}, {-33.87, 151.21}, {50.781464, 10.786089}, {0, 179}, {79.9, -179.9}, {-59.9, 120}} {
			x, y := p.Project(pt[0], pt[1])
			lat, lon := p.Unproject(x, y)
			if math.Abs(lat-pt[0]) > 1e-9 || math.Abs(lon-pt[1]) > 1e-9 {
				t.Errorf("%s: round trip %v -> %v,%v", p, pt, lat, lon)
			}
		}
		if p.Next() == p || p.Next().Next().Next() != p {
			t.Errorf("%s: Next cycles through all three", p)
		}
	}
	if Equirectangular != 0 || Projection(0).String() != "Equirectangular" || EqualEarth.String() != "Equal Earth" || WebMercator.String() != "Web Mercator" {
		t.Error("names, or the default is not equirectangular")
	}
	// Web Mercator stretches the north the most, Equirectangular the least.
	_, m := WebMercator.Project(60, 0)
	_, e := EqualEarth.Project(60, 0)
	_, q := Equirectangular.Project(60, 0)
	if !(m < e && e < q) {
		t.Errorf("60°N at %v, %v, %v", m, e, q)
	}
	if x, y := WebMercator.Project(90, -180); x != 0 || math.Abs(y) > 1e-6 {
		t.Errorf("Web Mercator's north-west corner = %v,%v", x, y)
	}
	if _, y := Equirectangular.Project(90, 0); y != 0.25 {
		t.Errorf("the North Pole is a quarter down: %v", y)
	}
	// Equal Earth's meridians curve: 180° is closer to the middle at 60°N.
	if x, _ := EqualEarth.Project(60, 180); x > 0.9 {
		t.Errorf("Equal Earth at 60°N, 180°: %v", x)
	}
}

func TestViewHelpers(t *testing.T) {
	v := View{X: 0.5, Y: 0.5, Zoom: 0}
	w, h := v.Span(64, 16)
	if w != 0.5 || h != 0.25 {
		t.Errorf("Span = %v,%v", w, h)
	}
	if c, r, ok := v.Locate(0.5, 0.5, 64, 16); !ok || c != 32 || r != 8 {
		t.Errorf("center cell = %d,%d,%v", c, r, ok)
	}
	if _, _, ok := v.Locate(0, 0, 64, 16); ok {
		t.Error("corner of the world is off screen")
	}

	f := Equirectangular.Fit(0.4, 0.3, 0.6, 0.5, 80, 24)
	if f.X != 0.5 || f.Y != 0.4 {
		t.Errorf("Fit center = %v,%v", f.X, f.Y)
	}
	sw, sh := f.Span(80, 24)
	if sw < 0.25-1e-9 || sh < 0.25-1e-9 || (sw > 0.26 && sh > 0.26) {
		t.Errorf("Fit should just cover the box with a margin: span %v x %v", sw, sh)
	}
	if one := Equirectangular.Fit(0.3, 0.3, 0.3, 0.3, 80, 24); one.Zoom != 7 || one.X != 0.3 {
		t.Errorf("single point = %+v", one)
	}
	if wide := Equirectangular.Fit(0, 0, 1, 1, 10, 5); wide.Zoom != MinZoom {
		t.Errorf("tiny screen clamps to MinZoom: %+v", wide)
	}
	if close := Equirectangular.Fit(0.5, 0.5, 0.5000001, 0.5000001, 80, 24); close.Zoom != 10 {
		t.Errorf("zoom is capped at a regional level: %+v", close)
	}
	if flat := Equirectangular.Fit(0.2, 0.5, 0.4, 0.5, 80, 24); flat.Zoom <= 0 || flat.Zoom > 10 {
		t.Errorf("horizontal box: %+v", flat)
	}
}

func TestClip(t *testing.T) {
	tests := []struct {
		in   [4]float64
		want [4]float64
		ok   bool
	}{
		{[4]float64{1, 1, 5, 5}, [4]float64{1, 1, 5, 5}, true},
		{[4]float64{-5, 5, 15, 5}, [4]float64{0, 5, 10, 5}, true},
		{[4]float64{5, -5, 5, 15}, [4]float64{5, 0, 5, 10}, true},
		{[4]float64{-5, -5, -1, -1}, [4]float64{}, false},
		{[4]float64{11, 0, 20, 10}, [4]float64{}, false},
		{[4]float64{-10, 5, -1, 5}, [4]float64{}, false},
		{[4]float64{5, 12, 5, 20}, [4]float64{}, false},
	}
	for _, tt := range tests {
		x0, y0, x1, y1, ok := clip(tt.in[0], tt.in[1], tt.in[2], tt.in[3], 0, 0, 10, 10)
		if ok != tt.ok || (ok && [4]float64{x0, y0, x1, y1} != tt.want) {
			t.Errorf("clip(%v) = %v,%v,%v,%v,%v want %v %v", tt.in, x0, y0, x1, y1, ok, tt.want, tt.ok)
		}
	}
}

// square returns a closed polyline around the equator/meridian crossing.
func square(kind Kind, d float64) Polyline {
	return Polyline{Kind: kind, Lon: []float64{-d, d, d, -d, -d}, Lat: []float64{d, d, -d, -d, d}}
}

func TestRenderGolden(t *testing.T) {
	w := New([]Polyline{square(Coastline, 5)})
	v := View{X: 0.5, Y: 0.5, Zoom: 2} // the world is 1024 dots wide: 5° is ~14 dots
	f := w.Render(v, 40, 12, nil, -1)
	want := strings.Join([]string{
		"                                        ",
		"                                        ",
		"            ⢰⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⡆            ",
		"            ⢸              ⡇            ",
		"            ⢸              ⡇            ",
		"            ⢸              ⡇            ",
		"⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⢹⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⡏⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉", // the equator
		"            ⢸              ⡇            ",
		"            ⢸              ⡇            ",
		"            ⠸⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠇            ",
		"                                        ",
		"                                        ",
	}, "\n") + "\n"
	if got := f.String(); got != want {
		t.Errorf("render:\n%s\nwant:\n%s", got, want)
	}
	if f.Cells[2][12].Class != ClassCoast || f.Cells[0][0].Class != ClassEmpty || f.Cells[6][0].Class != ClassEquator ||
		f.Cells[6][12].Class != ClassCoast {
		t.Error("classes")
	}
}

func TestRenderClassesAndDetail(t *testing.T) {
	w := New([]Polyline{
		square(Border, 10),
		{Kind: River, MinZoom: 9, Lon: []float64{-10, 10}, Lat: []float64{0, 0}},
		{Kind: Lake, MinZoom: 0, Lon: []float64{0, 0}, Lat: []float64{-10, 10}},
	})
	v := View{X: 0.5, Y: 0.5, Zoom: 2}
	f := w.Render(v, 40, 12, nil, -1)
	// The river needs zoom 9 - detailBias and is hidden; the lake shows.
	classes := map[Class]int{}
	for _, row := range f.Cells {
		for _, c := range row {
			classes[c.Class]++
		}
	}
	if classes[ClassRiver] != 0 || classes[ClassLake] == 0 || classes[ClassBorder] == 0 {
		t.Errorf("classes at zoom 2: %v", classes)
	}
	// Borders are dotted: the top edge does not fill every dot.
	var borders strings.Builder
	for _, row := range f.Cells {
		for _, c := range row {
			if c.Class == ClassBorder {
				borders.WriteRune(c.Rune)
			} else {
				borders.WriteRune(' ')
			}
		}
		borders.WriteByte('\n')
	}
	if strings.Contains(borders.String(), "⠉⠉⠉⠉") {
		t.Errorf("borders should be dotted:\n%s", borders.String())
	}
	v.Zoom = 8
	f = w.Render(v, 40, 12, nil, -1)
	found := false
	for _, row := range f.Cells {
		for _, c := range row {
			if c.Class == ClassRiver {
				found = true
			}
		}
	}
	if !found {
		t.Error("river should appear when zoomed in")
	}
}

// A route joins markers with a dotted line beneath them.
func TestRenderRoute(t *testing.T) {
	x1, y1 := Equirectangular.Project(0, -20)
	x2, y2 := Equirectangular.Project(0, 20)
	x3, y3 := Equirectangular.Project(10, 20)
	markers := []Marker{{X: x1, Y: y1}, {X: x2, Y: y2}, {X: x3, Y: y3}}
	_, centre := Equirectangular.Project(5, 0)
	v := View{X: 0.5, Y: centre, Zoom: 2}
	route := func(f Frame) (cells int) {
		for _, row := range f.Cells {
			for _, c := range row {
				if c.Class == ClassRoute {
					cells++
				}
			}
		}
		return cells
	}
	if n := route(New(nil).Render(v, 60, 20, markers, -1)); n != 0 {
		t.Errorf("%d route cells without a route", n)
	}
	f := New(nil).RenderRoute(v, 60, 20, markers, -1, []int{0, 1, 1, 2, 7})
	// The route along the equator from the first marker to the second.
	r := f.MarkerCells[0][1]
	for c := f.MarkerCells[0][0] + 1; c < f.MarkerCells[1][0]; c++ {
		if f.Cells[r][c].Class != ClassRoute {
			t.Errorf("cell %d of the equator: %+v", c, f.Cells[r][c])
		}
	}
	if n := route(f); n < 20 {
		t.Errorf("%d route cells", n)
	}
	// The markers stay on top.
	for i, mc := range f.MarkerCells {
		if f.Cells[mc[1]][mc[0]].Class != ClassMarker {
			t.Errorf("marker %d is %+v", i, f.Cells[mc[1]][mc[0]])
		}
	}
}

func TestRenderMarkersAndLabel(t *testing.T) {
	x, y := Equirectangular.Project(-0.05, 0.05)   // inside the cell right below the center
	x2, y2 := Equirectangular.Project(-0.06, 0.06) // same cell at zoom 2
	x3, y3 := Equirectangular.Project(5, 5)
	far, farY := Equirectangular.Project(80, 170)
	markers := []Marker{{X: x, Y: y, Label: "A"}, {X: x2, Y: y2, Label: "B"}, {X: x3, Y: y3, Label: "Leeds"}, {X: far, Y: farY, Label: "Far"}}
	v := View{X: 0.5, Y: 0.5, Zoom: 2}
	f := New(nil).Render(v, 40, 12, markers, 2)
	if f.MarkerCells[3] != [2]int{-1, -1} {
		t.Errorf("off-screen marker = %v", f.MarkerCells[3])
	}
	c, r := f.MarkerCells[0][0], f.MarkerCells[0][1]
	if f.Cells[r][c].Rune != '2' || f.Cells[r][c].Class != ClassMarker {
		t.Errorf("two markers in one cell = %q", f.Cells[r][c].Rune)
	}
	sc, sr := f.MarkerCells[2][0], f.MarkerCells[2][1]
	if f.Cells[sr][sc].Rune != '◉' || f.Cells[sr][sc].Class != ClassSelected {
		t.Errorf("selected marker = %q", f.Cells[sr][sc].Rune)
	}
	line := strings.Split(f.String(), "\n")[sr]
	if !strings.Contains(line, "◉ Leeds") {
		t.Errorf("label missing: %q", line)
	}
	// Near the right edge the label goes to the left of the marker.
	edge := New(nil).Render(View{X: x - 0.033, Y: y, Zoom: 2}, 40, 12, []Marker{{X: x, Y: y, Label: "Leeds"}}, 0)
	ec, er := edge.MarkerCells[0][0], edge.MarkerCells[0][1]
	if ec < 30 {
		t.Fatalf("marker not near the edge: %d", ec)
	}
	if line := strings.Split(edge.String(), "\n")[er]; !strings.Contains(line, " Leeds ◉") {
		t.Errorf("label should be left of the marker: %q", line)
	}
	many := make([]Marker, 12)
	for i := range many {
		many[i] = Marker{X: x, Y: y, Label: "Same"}
	}
	f = New(nil).Render(v, 40, 12, many, -1)
	if f.Cells[f.MarkerCells[0][1]][f.MarkerCells[0][0]].Rune != '+' {
		t.Error("more than nine markers show +")
	}
}

func TestLabelWideAndLong(t *testing.T) {
	f := Frame{Cols: 12, Rows: 1, Cells: [][]Cell{make([]Cell, 12)}}
	f.label(0, 0, "東京", ClassLabel)
	if got := f.String(); got != "  東京      \n" { // the marker column stays empty
		t.Errorf("wide label = %q", got)
	}
	f = Frame{Cols: 10, Rows: 1, Cells: [][]Cell{make([]Cell, 10)}}
	f.label(5, 0, "A very long place name", ClassLabel)
	if got := strings.TrimRight(f.String(), "\n"); !strings.Contains(got, "…") && len([]rune(got)) > 10 {
		t.Errorf("long label = %q", got)
	}
	f.label(5, 0, "", ClassLabel)
	minor := Frame{Cols: 10, Rows: 1, Cells: [][]Cell{make([]Cell, 10)}}
	minor.label(5, 0, "A very long place name", ClassMinorLabel)
	if strings.TrimSpace(minor.String()) != "" {
		t.Errorf("minor labels are not truncated but skipped: %q", minor.String())
	}
}

func TestMinorLabels(t *testing.T) {
	x1, y1 := Equirectangular.Project(-0.5, -7) // west
	x2, y2 := Equirectangular.Project(-0.5, 3)  // east, same row
	x3, y3 := Equirectangular.Project(-0.5, 4)  // right next to east, so east's label moves left
	x4, y4 := Equirectangular.Project(-0.5, 5)  // leaves no room for crowded's label on either side
	v := View{X: 0.5, Y: 0.5, Zoom: 2}
	f := New(nil).Render(v, 40, 12, []Marker{{X: x1, Y: y1, Label: "West"}, {X: x2, Y: y2, Label: "East"}, {X: x3, Y: y3, Label: "Crowded"}, {X: x4, Y: y4, Label: "Hidden"}}, 0)
	row := f.MarkerCells[0][1]
	line := strings.Split(f.String(), "\n")[row]
	if !strings.Contains(line, "◉ West") || !strings.Contains(line, " East ●") || strings.Contains(line, "Crowded") || !strings.Contains(line, "● Hidden") {
		t.Errorf("labels = %q", line)
	}
	if f.Cells[row][f.MarkerCells[1][0]-2].Class != ClassMinorLabel {
		t.Error("unselected labels use the minor class")
	}
	// Too many visible places: only the selected one is labeled.
	var many []Marker // a 5×9 grid of places, each in its own cell
	for r := range 5 {
		for c := range 9 {
			mx, my := Equirectangular.Project(-0.5-float64(r)*1.5, -12+float64(c)*3)
			many = append(many, Marker{X: mx, Y: my, Label: "P"})
		}
	}
	f = New(nil).Render(v, 40, 12, many, -1)
	visible := map[[2]int]bool{}
	for _, pos := range f.MarkerCells {
		if pos[0] >= 0 {
			visible[pos] = true
		}
	}
	if len(visible) <= maxMinorLabels {
		t.Fatalf("test needs more than %d visible places, has %d", maxMinorLabels, len(visible))
	}
	if strings.Contains(f.String(), "P") {
		t.Errorf("crowded map should not label unselected places:\n%s", f.String())
	}
}

func TestLoadEmbedded(t *testing.T) {
	w, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[Kind]int{}
	for _, s := range w.segments(Equirectangular) {
		kinds[s.kind]++
		if len(s.x) < 2 || len(s.x) > maxSegmentPoints {
			t.Fatalf("segment with %d points", len(s.x))
		}
	}
	for k := Border; k < kindCount; k++ {
		if kinds[k] == 0 {
			t.Errorf("no %s features", k)
		}
	}
	// Leeds lies on land in England: the British coastline passes nearby.
	x, y := Equirectangular.Project(53.8, -1.55)
	f := w.Render(View{X: x, Y: y, Zoom: 5}, 60, 20, []Marker{{X: x, Y: y, Label: "Leeds"}}, 0)
	coast := 0
	for _, row := range f.Cells {
		for _, c := range row {
			if c.Class == ClassCoast {
				coast++
			}
		}
	}
	if coast < 20 {
		t.Errorf("expected the British coastline around Leeds, got %d cells:\n%s", coast, f.String())
	}
	if w2, _ := Load(); w2 != w {
		t.Error("Load must return the shared world")
	}
	if !strings.Contains(License, "public domain") || Attribution == "" {
		t.Error("license text")
	}
	for k, want := range map[Kind]string{Border: "border", Coastline: "coastline", Lake: "lake", River: "river", Kind(9): "kind(9)"} {
		if k.String() != want {
			t.Errorf("%d.String() = %q", k, k.String())
		}
	}
}

func BenchmarkRenderWorld(b *testing.B) {
	w, _ := Load()
	for i := 0; i < b.N; i++ {
		w.Render(View{X: 0.5, Y: 0.5, Zoom: 0}, 160, 48, nil, -1)
	}
}

func BenchmarkRenderRegion(b *testing.B) {
	w, _ := Load()
	x, y := Equirectangular.Project(53.8, -1.55)
	for i := 0; i < b.N; i++ {
		w.Render(View{X: x, Y: y, Zoom: 7}, 160, 48, nil, -1)
	}
}

func TestRenderFaintMarkers(t *testing.T) {
	x, y := Equirectangular.Project(-0.05, 0.05)
	x2, y2 := Equirectangular.Project(-5, -5)
	v := View{X: 0.5, Y: 0.5, Zoom: 2}
	// A faint marker alone, and one in the cell of another marker.
	markers := []Marker{{X: x2, Y: y2, Label: "Faint", Faint: true}, {X: x, Y: y, Label: "A"}, {X: x, Y: y, Label: "Under", Faint: true}}
	f := New(nil).Render(v, 40, 12, markers, -1)
	c, r := f.MarkerCells[0][0], f.MarkerCells[0][1]
	if f.Cells[r][c] != (Cell{Rune: '·', Class: ClassFaint}) || strings.Contains(f.String(), "Faint") {
		t.Errorf("faint marker %q:\n%s", f.Cells[r][c].Rune, f)
	}
	c, r = f.MarkerCells[1][0], f.MarkerCells[1][1]
	if f.Cells[r][c].Rune != '●' || !strings.Contains(f.String(), "A") || strings.Contains(f.String(), "Under") {
		t.Errorf("a marker over a faint one:\n%s", f)
	}
}

func TestRenderBand(t *testing.T) {
	// A meridian from the Arctic to Antarctica, and an island near the
	// South Pole.
	w := New([]Polyline{
		{Kind: Coastline, Lon: []float64{0, 0}, Lat: []float64{85, -85}},
		{Kind: Coastline, Lon: []float64{-3, 3, 3, -3, -3}, Lat: []float64{-69, -69, -71, -71, -69}},
	})
	cols, rows := 80, 40
	v := View{X: 0.5, Y: 0.5, Zoom: 0} // 256 dots: the whole world in 64 rows
	f := w.Render(v, cols, rows, nil, -1)
	_, north := Equirectangular.Project(North, 0)
	_, south := Equirectangular.Project(South, 0)
	d := v.dotsPerWorld()
	top := int(((north-v.Y)*d + float64(rows*2)) / 4)
	bottom := int(((south-v.Y)*d + float64(rows*2)) / 4)
	for r, row := range f.Cells {
		c := row[cols/2]
		drawn := c.Rune != 0 && c.Class == ClassCoast
		if want := r >= top && r <= bottom; drawn != want {
			t.Errorf("row %d (band %d–%d): drawn %v", r, top, bottom, drawn)
		}
	}
	// The equator crosses the whole world, and only the world.
	_, eq, _ := v.Locate(0.5, 0.5, cols, rows)
	if f.Cells[eq][0].Class != ClassEquator || f.Cells[eq][cols-1].Class != ClassEquator {
		t.Errorf("equator row %d: %+v", eq, f.Cells[eq][:3])
	}
	narrow := New(nil).Render(View{X: 0.5, Y: 0.5, Zoom: -1}, cols, rows, nil, -1)
	if narrow.Cells[eq][0].Rune != 0 || narrow.Cells[eq][cols/2].Class != ClassEquator {
		t.Errorf("zoomed out, the equator ends with the world")
	}
}

func TestClampedAndWholeWorld(t *testing.T) {
	for p := range projections {
		north, south := p.band()
		// Zoomed in, the view stays within the band and the world.
		v := View{X: -0.2, Y: 0.01, Zoom: 5, Proj: p}.Clamped(80, 20)
		_, h := v.Span(80, 20)
		if v.X != 0 || math.Abs(v.Y-h/2-north) > 1e-12 || v.Proj != p {
			t.Errorf("%s: north-west: %+v", p, v)
		}
		v = View{X: 1.5, Y: 0.99, Zoom: 5, Proj: p}.Clamped(80, 20)
		if v.X != 1 || math.Abs(v.Y+h/2-south) > 1e-12 {
			t.Errorf("%s: south-east: %+v", p, v)
		}
		if v := (View{X: 0.3, Y: 0.4, Zoom: 5, Proj: p}).Clamped(80, 20); v.X != 0.3 || v.Y != 0.4 {
			t.Errorf("%s: a view within the band moved: %+v", p, v)
		}
		// Zoomed out further than the band, it is centered.
		if v := (View{X: 0.5, Y: 0.1, Zoom: -1, Proj: p}).Clamped(80, 40); v.Y != (north+south)/2 {
			t.Errorf("%s: zoomed out: %+v", p, v)
		}
		// The whole world fills the screen with the band or the width.
		for _, size := range [][2]int{{110, 25}, {200, 25}, {60, 40}} {
			w := p.WholeWorld(size[0], size[1])
			sw, sh := w.Span(size[0], size[1])
			if sw < 1-1e-9 && sh < south-north-1e-9 || sw > 1+1e-9 && sh > south-north+1e-9 {
				t.Errorf("%s %v: spans %v × %v", p, size, sw, sh)
			}
			if w.X != 0.5 || math.Abs(w.Y-(north+south)/2) > 1e-12 || w.Proj != p {
				t.Errorf("%s %v: center %+v", p, size, w)
			}
		}
	}
}

func TestRenderProjections(t *testing.T) {
	// A meridian at 90°E from the Arctic to the Antarctic.
	w := New([]Polyline{{Kind: Coastline, Lon: []float64{90, 90, 90}, Lat: []float64{70, 0, -50}}})
	rows := map[Projection][2]int{}
	for p := range projections {
		v := p.WholeWorld(100, 30)
		f := w.Render(v, 100, 30, nil, -1)
		first, last := -1, -1
		outline := 0
		for r, row := range f.Cells {
			for _, c := range row {
				if c.Class == ClassCoast {
					if first < 0 {
						first = r
					}
					last = r
				}
				if c.Class == ClassOutline {
					outline++
				}
			}
		}
		rows[p] = [2]int{first, last}
		// Only Equal Earth draws the edge of the world, which is curved.
		if (outline > 0) != (p == EqualEarth) {
			t.Errorf("%s: %d cells of outline", p, outline)
		}
		if first < 0 {
			t.Errorf("%s: no meridian:\n%s", p, f.String())
		}
	}
	// Web Mercator gives the north more rows than the others.
	if m, q := rows[WebMercator], rows[Equirectangular]; m[1]-m[0] <= q[1]-q[0] {
		t.Errorf("rows of the meridian: %v", rows)
	}
}

func TestPrepareWhileRendering(t *testing.T) {
	w := New([]Polyline{square(Coastline, 5)})
	done := make(chan bool)
	go func() {
		for p := range projections {
			w.Prepare(p)
		}
		done <- true
	}()
	for p := range projections {
		if f := w.Render(View{X: 0.5, Y: 0.5, Zoom: 2, Proj: p}, 40, 12, nil, -1); !strings.ContainsRune(f.String(), '⡇') {
			t.Errorf("%s:\n%s", p, f.String())
		}
	}
	<-done
}
