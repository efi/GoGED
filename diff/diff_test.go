package diff

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/efi/goged/gedcom"
)

const oldGED = `0 HEAD
1 SOUR GENJ
2 VERS 3.0
1 DATE 1 JAN 2026
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
1 SUBM @U1@
0 @U1@ SUBM
1 NAME Ann Researcher
0 @S1@ SOUR
1 TITL Parish register
0 @S2@ SOUR
1 TITL Census 1881
0 @I1@ INDI
1 NAME John /Smith/
1 SEX M
1 BIRT
2 DATE 1817
2 PLAC Leeds
1 RESI
2 DATE 1841
2 PLAC Leeds
1 RESI
2 DATE 1851
2 PLAC York
1 OCCU Weaver
1 FAMS @F1@
1 CHAN
2 DATE 1 JAN 2026
0 @I2@ INDI
1 NAME Mary /Brown/
1 SEX F
1 FAMS @F1@
0 @I3@ INDI
1 NAME Tom /Smith/
1 FAMC @F1@
0 @I4@ INDI
1 NAME Old /Gone/
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 CHIL @I3@
1 MARR
2 DATE 1840
0 TRLR
`

const newGED = `0 HEAD
1 SOUR GENJ
2 VERS 3.1
1 DATE 14 OCT 2026
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
1 SUBM @U1@
0 @U1@ SUBM
1 NAME Ann Researcher-Lee
0 @S1@ SOUR
1 TITL Parish register of Leeds
0 @I1@ INDI
1 NAME Johann /Smith/
1 SEX M
1 BIRT
2 DATE 2 FEB 1817
2 PLAC Leeds
1 RESI
2 DATE 1851
2 PLAC York
1 OCCU Weaver
2 NOTE wool
1 FAMS @F1@
1 CHAN
2 DATE 14 OCT 2026
0 @I2@ INDI
1 NAME Mary /Brown/
1 SEX F
1 FAMS @F1@
0 @I3@ INDI
1 NAME Tom /Smith/
1 FAMC @F1@
2 PEDI adopted
0 @I5@ INDI
1 NAME Jane /New/
1 BIRT
2 DATE 1900
1 FAMC @F1@
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 CHIL @I3@
1 CHIL @I5@
1 MARR
2 DATE 1840
0 @N1@ NOTE A new shared note
0 TRLR
`

func parse(t *testing.T, s string) *gedcom.Document {
	t.Helper()
	doc, err := gedcom.ParseString(s)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// lines writes a result out as "~ ID kind title: detail; detail" lines.
func lines(r Result) string {
	var out []string
	for _, d := range r.Header {
		out = append(out, d.Kind.Symbol()+" header "+d.String())
	}
	for _, group := range [][]Record{r.People, r.Families, r.Others} {
		for _, rec := range group {
			var ds []string
			for _, d := range rec.Details {
				ds = append(ds, d.String())
			}
			line := rec.Kind.Symbol() + " " + rec.ID + " " + rec.Noun() + " " + rec.Title
			if len(ds) > 0 {
				line += ": " + strings.Join(ds, "; ")
			}
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func TestCompare(t *testing.T) {
	r := Compare(parse(t, oldGED), parse(t, newGED))
	want := `~ header program: GENJ · 3.0 → GENJ · 3.1
~ header date of the file: 1 JAN 2026 → 14 OCT 2026
- I4 person Old Gone
+ I5 person Jane New (1900–)
~ I1 person Johann Smith (1817–): name: John Smith → Johann Smith; birth: 1817, Leeds → 2 Feb 1817, Leeds; occupation: details changed; residence removed: 1841, Leeds
~ I3 person Tom Smith: parents: John Smith & Mary Brown → Johann Smith & Mary Brown (adopted)
~ F1 family Johann Smith & Mary Brown: child added: Jane New (1900–)
~ U1 submitter Ann Researcher-Lee: name: Ann Researcher → Ann Researcher-Lee
+ N1 note A new shared note
- S2 source Census 1881
~ S1 source Parish register of Leeds: title: Parish register → Parish register of Leeds`
	if got := lines(r); got != want {
		t.Errorf("differences:\n%s\nwant:\n%s", got, want)
	}
	if got := r.Summary(); got != "people: 2 changed, 1 added, 1 removed · families: 1 changed · other records: 2 changed, 1 added, 1 removed · header: 2 changes" {
		t.Errorf("summary = %q", got)
	}
	if r.People[2].Person == nil || r.People[2].Person.Name().Given != "Johann" || r.People[0].Person.ID != "I4" || r.Families[0].Family == nil {
		t.Error("records of the versions")
	}
}

// SEX values are compared as the parser reads them: "male" and "M" are
// the same.
func TestCompareSex(t *testing.T) {
	old := parse(t, "0 HEAD\n0 @I1@ INDI\n1 NAME A /B/\n1 SEX male\n0 @I2@ INDI\n1 NAME C /D/\n1 SEX male\n0 TRLR\n")
	cur := parse(t, "0 HEAD\n0 @I1@ INDI\n1 NAME A /B/\n1 SEX M\n0 @I2@ INDI\n1 NAME C /D/\n1 SEX Female\n0 TRLR\n")
	want := "~ I1 person A B: sex: details changed\n~ I2 person C D: sex: male → female"
	if got := lines(Compare(old, cur)); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCompareSame(t *testing.T) {
	doc, err := gedcom.ParseFile(filepath.Join("..", "testdata", "Muster_GEDCOM_UTF-8.ged"))
	if err != nil {
		t.Fatal(err)
	}
	again, _ := gedcom.ParseFile(filepath.Join("..", "testdata", "Muster_GEDCOM_UTF-8.ged"))
	if r := Compare(doc, again); !r.Empty() || r.Summary() != "no differences" {
		t.Errorf("the same file differs: %s", lines(r))
	}
	// Only a change date changed.
	r := Compare(parse(t, oldGED), parse(t, strings.Replace(oldGED, "2 DATE 1 JAN 2026", "2 DATE 2 JAN 2026", 1)))
	if !r.Empty() {
		t.Errorf("a change date counts: %s", lines(r))
	}
}

func TestCompareEdges(t *testing.T) {
	// An identifier that names another kind of record now.
	r := Compare(parse(t, "0 HEAD\n0 @X1@ INDI\n1 NAME A /B/\n0 TRLR\n"), parse(t, "0 HEAD\n0 @X1@ SOUR\n1 TITL A book\n0 TRLR\n"))
	if got := lines(r); got != "- X1 person A B\n+ X1 source A book" {
		t.Errorf("another kind:\n%s", got)
	}
	// A header that is added.
	r = Compare(parse(t, "0 @I1@ INDI\n0 TRLR\n"), parse(t, "0 HEAD\n1 CHAR UTF-8\n0 @I1@ INDI\n0 TRLR\n"))
	if got := lines(r); got != "+ header character set added: UTF-8" {
		t.Errorf("header added: %q", got)
	}

	for _, tt := range []struct {
		d    Detail
		want string
	}{
		{Detail{Added, "birth", "", ""}, "birth added"},
		{Detail{Removed, "note", "", ""}, "note removed"},
		{Detail{Removed, "note", "old text", ""}, "note removed: old text"},
		{Detail{Changed, "sex", "male", "female"}, "sex: male → female"},
	} {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("%+v: %q, want %q", tt.d, got, tt.want)
		}
	}
	if shorten(strings.Repeat("x", 80)) != strings.Repeat("x", 59)+"…" || (Record{Tag: "_FOO"}).Noun() != "foo" {
		t.Error("shorten, Noun")
	}
}
