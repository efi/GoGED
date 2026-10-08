package worldmap

import (
	"compress/gzip"
	"io"
)

func newGzip(w io.Writer) *gzip.Writer { return gzip.NewWriter(w) }
