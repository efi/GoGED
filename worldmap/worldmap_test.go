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

func TestProjection(t *testing.T) {
	x, y := Project(0, 0)
	if x != 0.5 || math.Abs(y-0.5) > 1e-12 {
		t.Errorf("Project(0,0) = %v,%v", x, y)
	}
	x, y = Project(90, -180)
	if x != 0 || math.Abs(y) > 1e-6 {
		t.Errorf("north-west corner = %v,%v", x, y)
	}
	for _, p := range [][2]float64{{53.7997, -1.5492}, {-33.87, 151.21}, {50.781464, 10.786089}, {0, 179}} {
		x, y := Project(p[0], p[1])
		lat, lon := Unproject(x, y)
		if math.Abs(lat-p[0]) > 1e-9 || math.Abs(lon-p[1]) > 1e-9 {
			t.Errorf("round trip %v -> %v,%v", p, lat, lon)
		}
	}
	if _, y1 := Project(50, 0); y1 >= 0.5 {
		t.Error("north must be above the equator")
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

	f := Fit(0.4, 0.3, 0.6, 0.5, 80, 24)
	if f.X != 0.5 || f.Y != 0.4 {
		t.Errorf("Fit center = %v,%v", f.X, f.Y)
	}
	sw, sh := f.Span(80, 24)
	if sw < 0.25-1e-9 || sh < 0.25-1e-9 || (sw > 0.26 && sh > 0.26) {
		t.Errorf("Fit should just cover the box with a margin: span %v x %v", sw, sh)
	}
	if one := Fit(0.3, 0.3, 0.3, 0.3, 80, 24); one.Zoom != 7 || one.X != 0.3 {
		t.Errorf("single point = %+v", one)
	}
	if wide := Fit(0, 0, 1, 1, 10, 5); wide.Zoom != MinZoom {
		t.Errorf("tiny screen clamps to MinZoom: %+v", wide)
	}
	if close := Fit(0.5, 0.5, 0.5000001, 0.5000001, 80, 24); close.Zoom != 10 {
		t.Errorf("zoom is capped at a regional level: %+v", close)
	}
	if flat := Fit(0.2, 0.5, 0.4, 0.5, 80, 24); flat.Zoom <= 0 || flat.Zoom > 10 {
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
		x0, y0, x1, y1, ok := clip(tt.in[0], tt.in[1], tt.in[2], tt.in[3], 10, 10)
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
		"            ⢸              ⡇            ",
		"            ⢸              ⡇            ",
		"            ⢸              ⡇            ",
		"            ⠸⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠇            ",
		"                                        ",
		"                                        ",
	}, "\n") + "\n"
	if got := f.String(); got != want {
		t.Errorf("render:\n%s\nwant:\n%s", got, want)
	}
	if f.Cells[2][12].Class != ClassCoast || f.Cells[0][0].Class != ClassEmpty {
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
	if strings.Contains(f.String(), "⠉⠉⠉⠉") {
		t.Errorf("borders should be dotted:\n%s", f.String())
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

func TestRenderMarkersAndLabel(t *testing.T) {
	x, y := Project(-0.05, 0.05)   // inside the cell right below the center
	x2, y2 := Project(-0.06, 0.06) // same cell at zoom 2
	x3, y3 := Project(5, 5)
	far, farY := Project(80, 170)
	markers := []Marker{{x, y, "A"}, {x2, y2, "B"}, {x3, y3, "Leeds"}, {far, farY, "Far"}}
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
	edge := New(nil).Render(View{X: x - 0.033, Y: y, Zoom: 2}, 40, 12, []Marker{{x, y, "Leeds"}}, 0)
	ec, er := edge.MarkerCells[0][0], edge.MarkerCells[0][1]
	if ec < 30 {
		t.Fatalf("marker not near the edge: %d", ec)
	}
	if line := strings.Split(edge.String(), "\n")[er]; !strings.Contains(line, " Leeds ◉") {
		t.Errorf("label should be left of the marker: %q", line)
	}
	many := make([]Marker, 12)
	for i := range many {
		many[i] = Marker{x, y, "Same"}
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
	x1, y1 := Project(-0.5, -7) // west
	x2, y2 := Project(-0.5, 3)  // east, same row
	x3, y3 := Project(-0.5, 4)  // right next to east, so east's label moves left
	x4, y4 := Project(-0.5, 5)  // leaves no room for crowded's label on either side
	v := View{X: 0.5, Y: 0.5, Zoom: 2}
	f := New(nil).Render(v, 40, 12, []Marker{{x1, y1, "West"}, {x2, y2, "East"}, {x3, y3, "Crowded"}, {x4, y4, "Hidden"}}, 0)
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
			mx, my := Project(-0.5-float64(r)*1.5, -12+float64(c)*3)
			many = append(many, Marker{mx, my, "P"})
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
	for _, s := range w.segs {
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
	x, y := Project(53.8, -1.55)
	f := w.Render(View{X: x, Y: y, Zoom: 5}, 60, 20, []Marker{{x, y, "Leeds"}}, 0)
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
	x, y := Project(53.8, -1.55)
	for i := 0; i < b.N; i++ {
		w.Render(View{X: x, Y: y, Zoom: 7}, 160, 48, nil, -1)
	}
}
