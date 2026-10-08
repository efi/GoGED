// Package worldmap draws a zoomable world map with country borders,
// coastlines, rivers and lakes in the terminal and marks places on it.
//
// The map data comes from Natural Earth (public domain) and is compiled
// into the binary in a compact format produced by the mkworld tool.
package worldmap

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// Kind is the type of a map feature.
type Kind uint8

// Feature kinds, in drawing order.
const (
	Border Kind = iota
	Coastline
	Lake
	River
	kindCount
)

func (k Kind) String() string {
	switch k {
	case Border:
		return "border"
	case Coastline:
		return "coastline"
	case Lake:
		return "lake"
	case River:
		return "river"
	}
	return fmt.Sprintf("kind(%d)", k)
}

// Polyline is a feature in geographic coordinates (degrees).
type Polyline struct {
	Kind    Kind
	MinZoom uint8 // the feature is shown from this zoom level on
	Lon     []float64
	Lat     []float64
}

// The storage format is a gzip stream of:
//
//	magic "GOGEDMAP" and version byte 1
//	uvarint number of polylines, then for each polyline:
//	  byte kind, byte min zoom, uvarint number of points,
//	  the points as zigzag varints of lon and lat in units of 1/scale
//	  degrees, each relative to the previous point.
const (
	magic   = "GOGEDMAP"
	version = 1
	scale   = 10000 // 1/10000 degree, about 11 m
)

// Encode writes polylines in the storage format.
func Encode(w io.Writer, lines []Polyline) error {
	zw, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(zw)
	var buf [binary.MaxVarintLen64]byte
	putU := func(v uint64) { bw.Write(buf[:binary.PutUvarint(buf[:], v)]) }
	putS := func(v int64) { bw.Write(buf[:binary.PutVarint(buf[:], v)]) }

	bw.WriteString(magic)
	bw.WriteByte(version)
	putU(uint64(len(lines)))
	for _, l := range lines {
		if len(l.Lon) != len(l.Lat) {
			return errors.New("worldmap: polyline with mismatched coordinates")
		}
		bw.WriteByte(byte(l.Kind))
		bw.WriteByte(l.MinZoom)
		putU(uint64(len(l.Lon)))
		var px, py int64
		for i := range l.Lon {
			x := int64(math.Round(l.Lon[i] * scale))
			y := int64(math.Round(l.Lat[i] * scale))
			putS(x - px)
			putS(y - py)
			px, py = x, y
		}
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	return zw.Close()
}

// Decode reads polylines in the storage format.
func Decode(data []byte) ([]Polyline, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("worldmap: %w", err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("worldmap: %w", err)
	}
	if len(raw) < len(magic)+1 || string(raw[:len(magic)]) != magic {
		return nil, errors.New("worldmap: not a map data file")
	}
	if raw[len(magic)] != version {
		return nil, fmt.Errorf("worldmap: unsupported version %d", raw[len(magic)])
	}
	r := bytes.NewReader(raw[len(magic)+1:])
	fail := func() ([]Polyline, error) { return nil, errors.New("worldmap: truncated map data") }

	n, err := binary.ReadUvarint(r)
	if err != nil || n > uint64(len(raw)) {
		return fail()
	}
	lines := make([]Polyline, 0, n)
	for range n {
		kind, err1 := r.ReadByte()
		minZoom, err2 := r.ReadByte()
		count, err3 := binary.ReadUvarint(r)
		if err1 != nil || err2 != nil || err3 != nil || kind >= byte(kindCount) || count > uint64(r.Len()) {
			return fail()
		}
		l := Polyline{Kind: Kind(kind), MinZoom: minZoom, Lon: make([]float64, count), Lat: make([]float64, count)}
		var x, y int64
		for i := range count {
			dx, err1 := binary.ReadVarint(r)
			dy, err2 := binary.ReadVarint(r)
			if err1 != nil || err2 != nil {
				return fail()
			}
			x += dx
			y += dy
			l.Lon[i] = float64(x) / scale
			l.Lat[i] = float64(y) / scale
		}
		lines = append(lines, l)
	}
	return lines, nil
}
