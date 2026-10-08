package gedcom

import (
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

// The fuzz targets run their seed corpus as regular tests; run them with
// "go test -fuzz FuzzParse ./gedcom" to explore further.

func FuzzParse(f *testing.F) {
	sample, err := os.ReadFile(filepath.Join("..", "testdata", "family.ged"))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(sample)
	if muster, err := os.ReadFile(filepath.Join("..", "testdata", "Muster_GEDCOM_UTF-8.ged")); err == nil {
		f.Add(muster)
	}
	f.Add([]byte("0 HEAD\n0 @I1@ INDI\n1 RESN privacy\n1 ADOP\n2 FAMC @F1@\n3 ADOP WIFE\n1 FAMC @F1@\n2 PEDI adopted\n1 ALIA @I1@\n1 ASSO @I1@\n0 @F1@ FAM\n1 WIFE @I1@\n1 CHIL @I1@\n2 _MREL Step\n0 @L1@ _LOC\n1 _LOC @L1@\n"))
	f.Add([]byte("0 HEAD\n1 CHAR ANSEL\n0 @I1@ INDI\n1 NAME J\xe2os\xe8e\n0 TRLR\n"))
	f.Add([]byte("0 HEAD\n0 @I1@ INDI\n1 FAMC @F1@\n1 FAMS @F1@\n0 @F1@ FAM\n1 HUSB @I1@\n1 CHIL @I1@\n"))
	f.Add([]byte("\xff\xfe0\x00 \x00H\x00E\x00A\x00D\x00"))
	f.Add([]byte("0 @I1@ INDI\n5 BIRT\n1 CONC x\n1 CONT y\nbad line\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := ParseBytes(data)
		if err != nil {
			return
		}
		for _, d := range []*Document{doc, doc.Redacted()} {
			for _, ind := range d.Individuals {
				_ = ind.Lifespan()
				_ = ind.DisplayName()
				_ = ind.SortName()
				_ = ind.Siblings()
				_ = ind.HalfSiblings()
				_ = ind.ParentLinks()
				_ = ind.Father()
				_ = ind.Citations()
				_ = ind.Notes()
				_ = ind.Aliases()
				_ = ind.Changed()
				for _, a := range ind.Associations() {
					_ = a.Label()
				}
			}
			for _, e := range d.Events() {
				_ = e.Label()
				_ = e.Detail()
				_ = e.Facts()
				_ = e.Place.Names()
				_ = e.Date.String()
				_, _ = e.Date.Fit(12)
				_, _ = e.Date.Key()
			}
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
		for _, w := range []int{0, 1, 9, 19} {
			if short, _ := d.Fit(w); utf8.RuneCountInString(short) > w {
				t.Fatalf("%q: Fit(%d) = %q", s, w, short)
			}
		}
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
