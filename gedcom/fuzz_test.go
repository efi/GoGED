package gedcom

import (
	"os"
	"path/filepath"
	"testing"
)

// The fuzz targets run their seed corpus as regular tests; run them with
// "go test -fuzz FuzzParse ./gedcom" to explore further.

func FuzzParse(f *testing.F) {
	sample, err := os.ReadFile(filepath.Join("..", "testdata", "family.ged"))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(sample)
	f.Add([]byte("0 HEAD\n1 CHAR ANSEL\n0 @I1@ INDI\n1 NAME J\xe2os\xe8e\n0 TRLR\n"))
	f.Add([]byte("0 HEAD\n0 @I1@ INDI\n1 FAMC @F1@\n1 FAMS @F1@\n0 @F1@ FAM\n1 HUSB @I1@\n1 CHIL @I1@\n"))
	f.Add([]byte("\xff\xfe0\x00 \x00H\x00E\x00A\x00D\x00"))
	f.Add([]byte("0 @I1@ INDI\n5 BIRT\n1 CONC x\n1 CONT y\nbad line\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := ParseBytes(data)
		if err != nil {
			return
		}
		for _, ind := range doc.Individuals {
			_ = ind.Lifespan()
			_ = ind.DisplayName()
			_ = ind.Siblings()
			_ = ind.HalfSiblings()
			_ = ind.Citations()
			_ = ind.Notes()
		}
		for _, e := range doc.Events() {
			_ = e.Label()
			_ = e.Date.String()
			_, _ = e.Date.Key()
		}
	})
}

func FuzzParseDate(f *testing.F) {
	for _, s := range []string{
		"12 MAR 1850", "ABT 1850", "BET 1850 AND 1860", "FROM 1 JAN 1700 TO 1800", "@#DHEBREW@ 1 TSH 5785",
		"@#DFRENCH R@ 18 BRUM 8", "1750/51", "44 BC", "INT 1850 (text)", "(phrase)", "1850-01-02", "c.1850",
		"@#DHEBREW@ 99999", "1/1", "99999/9",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		d := ParseDate(s)
		_ = d.String()
		_ = d.ShortYear()
		_ = d.Year()
		if k, ok := d.Key(); ok {
			lo, hi, _ := d.Span()
			if lo > hi {
				t.Fatalf("%q: span %d > %d", s, lo, hi)
			}
			_ = k
		}
		_, _ = AgeBetween(d, ParseDate("2000"))
	})
}

func FuzzParseLine(f *testing.F) {
	for _, s := range []string{"0 HEAD", "0 @I1@ INDI", "1 NAME a /b/", "2 CONC  x", "1 EMAIL a@@b"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		l, err := ParseLine(s, 1)
		if err == nil && l.Tag == "" {
			t.Fatalf("%q: parsed without tag", s)
		}
	})
}
