package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/efi/goged/worldmap"
)

func TestSimplify(t *testing.T) {
	line := [][2]float64{{0, 0}, {1, 0.001}, {2, -0.001}, {3, 0}, {3, 3}}
	got := simplify(line, 0.01)
	want := [][2]float64{{0, 0}, {3, 0}, {3, 3}}
	if len(got) != len(want) {
		t.Fatalf("simplify = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("simplify = %v, want %v", got, want)
		}
	}
	if len(simplify(line, 0)) != len(line) || len(simplify(line[:2], 1)) != 2 {
		t.Error("no-op cases")
	}
	loop := [][2]float64{{0, 0}, {1, 1}, {2, 0}, {0, 0}}
	if got := simplify(loop, 0.1); len(got) != 4 {
		t.Errorf("closed ring keeps its shape: %v", got)
	}
}

func TestLines(t *testing.T) {
	parse := func(s string) feature {
		var f feature
		if err := json.Unmarshal([]byte(s), &f); err != nil {
			t.Fatal(err)
		}
		return f
	}
	tests := map[string]int{
		`{"geometry":{"type":"LineString","coordinates":[[0,0],[1,1]]}}`:                                   1,
		`{"geometry":{"type":"MultiLineString","coordinates":[[[0,0],[1,1]],[[2,2],[3,3]]]}}`:              2,
		`{"geometry":{"type":"Polygon","coordinates":[[[0,0],[1,1],[1,0],[0,0]]]}}`:                        1,
		`{"geometry":{"type":"MultiPolygon","coordinates":[[[[0,0],[1,0],[0,0]]],[[[5,5],[6,5],[5,5]]]]}}`: 2,
		`{"geometry":null}`: 0,
	}
	for in, want := range tests {
		ls, err := lines(parse(in))
		if err != nil || len(ls) != want {
			t.Errorf("%s: %d lines, %v", in, len(ls), err)
		}
	}
	if _, err := lines(parse(`{"geometry":{"type":"Point","coordinates":[0,0]}}`)); err == nil {
		t.Error("points are not supported")
	}
	if v, ok := number(map[string]any{"min_zoom": 3.0}, "min_zoom"); !ok || v != 3 {
		t.Error("number")
	}
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	coast := write("coast.json", `{"features":[{"properties":{"min_zoom":1},"geometry":{"type":"LineString","coordinates":[[0,0],[1,0.0001],[2,0],[2,2]]}}]}`)
	lakes := write("lakes.json", `{"features":[
		{"properties":{"scalerank":2},"geometry":{"type":"Polygon","coordinates":[[[5,5],[6,5],[6,6],[5,5]]]}},
		{"properties":{"scalerank":9},"geometry":{"type":"Polygon","coordinates":[[[7,7],[8,7],[8,8],[7,7]]]}}]}`)
	out := filepath.Join(dir, "world.bin")
	var log bytes.Buffer
	if err := run([]string{"-o", out, "-coast", coast, "-lakes", lakes}, &log); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	lines, err := worldmap.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("lines = %d (the small-scale lake must be dropped)", len(lines))
	}
	if lines[0].Kind != worldmap.Coastline || lines[0].MinZoom != 1 || len(lines[0].Lon) != 3 {
		t.Errorf("coastline = %+v (simplified to 3 points)", lines[0])
	}
	if lines[1].Kind != worldmap.Lake || lines[1].MinZoom != 3 {
		t.Errorf("lake = %+v (default min zoom 3)", lines[1])
	}
	if !strings.Contains(log.String(), "wrote") {
		t.Errorf("log = %q", log.String())
	}

	// Several files per layer, e.g. the regional river supplements.
	rivers := write("rivers.json", `{"features":[{"properties":{"min_zoom":2},"geometry":{"type":"LineString","coordinates":[[0,0],[3,3]]}}]}`)
	europe := write("europe.json", `{"features":[{"properties":{"min_zoom":6.7},"geometry":{"type":"LineString","coordinates":[[10,50],[11,51]]}}]}`)
	if err := run([]string{"-o", out, "-rivers", rivers + "," + europe}, &log); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(out)
	if lines, err = worldmap.Decode(data); err != nil || len(lines) != 2 || lines[0].MinZoom != 2 || lines[1].MinZoom != 7 || lines[1].Lon[0] != 10 {
		t.Errorf("rivers from two files = %+v, %v", lines, err)
	}

	bad := write("bad.json", `{"features":[{"geometry":{"type":"Point","coordinates":[0,0]}}]}`)
	for _, args := range [][]string{
		{"-coast", filepath.Join(dir, "missing.json")},
		{"-coast", write("garbage.json", "not json")},
		{"-rivers", bad},
		{"-rivers", rivers + "," + filepath.Join(dir, "missing.json")},
		{"-bogus"},
		{"-o", filepath.Join(dir, "no", "such", "dir.bin"), "-coast", coast},
	} {
		if err := run(args, &log); err == nil {
			t.Errorf("%v: expected error", args)
		}
	}
}
