package gedcom

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustParse(t *testing.T, s string) *Document {
	t.Helper()
	doc, err := ParseString(s)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	return doc
}

func hasWarning(doc *Document, substr string) bool {
	for _, w := range doc.Warnings {
		if strings.Contains(w.String(), substr) {
			return true
		}
	}
	return false
}

func TestParseTree(t *testing.T) {
	doc := mustParse(t, `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Smith/
2 GIVN John
1 BIRT
2 DATE 1 JAN 1900
2 PLAC Leeds
1 NOTE First line
2 CONT second line
2 CONC  continued
2 CONT
2 CONT fourth
0 TRLR
`)
	if len(doc.Records) != 3 {
		t.Fatalf("records = %d, want 3", len(doc.Records))
	}
	if doc.Version != "5.5.1" {
		t.Errorf("Version = %q", doc.Version)
	}
	if len(doc.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", doc.Warnings)
	}
	indi := doc.Record("I1")
	if indi == nil || indi.Tag != "INDI" || indi.Xref != "@I1@" {
		t.Fatalf("Record(I1) = %+v", indi)
	}
	if got := indi.Path("BIRT", "PLAC").Value; got != "Leeds" {
		t.Errorf("BIRT.PLAC = %q", got)
	}
	if got := indi.Path("NAME", "GIVN"); got == nil || got.Parent.Tag != "NAME" || got.Level != 2 {
		t.Errorf("NAME.GIVN = %+v", got)
	}
	note := indi.First("NOTE")
	if want := "First line\nsecond line continued\n\nfourth"; note.Value != want {
		t.Errorf("NOTE = %q, want %q", note.Value, want)
	}
	if len(note.Children) != 0 {
		t.Errorf("continuation lines must not become children: %v", note.Children)
	}
	if got := indi.Path("BIRT", "DATE").TagPath(); got != "INDI.BIRT.DATE" {
		t.Errorf("TagPath = %q", got)
	}
	if got := indi.Path("BIRT", "DATE").Line; got != 9 {
		t.Errorf("line number = %d, want 9", got)
	}
	if indi.Path("BIRT", "NOPE") != nil || indi.Path() != indi {
		t.Error("Path misbehaves")
	}
	var nilNode *Node
	if nilNode.First("X") != nil || nilNode.All("X") != nil || nilNode.Val("X") != "" || nilNode.IsPointer() {
		t.Error("nil node accessors should be safe")
	}
}

func TestNodeHelpers(t *testing.T) {
	doc := mustParse(t, "0 HEAD\n0 @I1@ INDI\n1 NAME A\n1 NAME B\n1 FAMS @F1@\n0 TRLR\n")
	r := doc.Record("@I1@")
	if n := len(r.All("NAME")); n != 2 {
		t.Errorf("All(NAME) = %d", n)
	}
	if r.Val("NAME") != "A" {
		t.Errorf("Val(NAME) = %q", r.Val("NAME"))
	}
	if !r.First("FAMS").IsPointer() || r.First("NAME").IsPointer() {
		t.Error("IsPointer")
	}
	var tags []string
	r.Walk(func(n *Node) bool {
		tags = append(tags, n.Tag)
		return n.Tag != "NAME"
	})
	if strings.Join(tags, ",") != "INDI,NAME,NAME,FAMS" {
		t.Errorf("Walk = %v", tags)
	}
	if s := r.String(); s != "0 @I1@ INDI" {
		t.Errorf("String = %q", s)
	}
	if s := r.First("NAME").String(); s != "1 NAME A" {
		t.Errorf("String = %q", s)
	}
}

func TestParseWarnings(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"missing head", "0 @I1@ INDI\n1 NAME x\n0 TRLR\n", "does not start with a HEAD"},
		{"missing trailer", "0 HEAD\n0 @I1@ INDI\n", "missing TRLR"},
		{"level jump", "0 HEAD\n0 @I1@ INDI\n1 BIRT\n3 DATE 1900\n0 TRLR\n", "level jumps from 1 to 3"},
		{"garbage line", "0 HEAD\n0 @I1@ INDI\n1 NOTE abc\nstray text\n0 TRLR\n", "treated as continuation of line 3"},
		{"leading garbage", "garbage\n0 HEAD\n0 TRLR\n", "line ignored"},
		{"level before record", "1 NAME x\n0 HEAD\n0 TRLR\n", "appears before any record"},
		{"data after trailer", "0 HEAD\n0 TRLR\n0 @I1@ INDI\n", "data after TRLR"},
		{"duplicate id", "0 HEAD\n0 @I1@ INDI\n0 @I1@ INDI\n0 TRLR\n", "duplicate identifier @I1@"},
		{"xref on sub line", "0 HEAD\n0 @I1@ INDI\n1 @X1@ NAME x\n0 TRLR\n", "cross-reference identifier @X1@"},
		{"indi without xref", "0 HEAD\n0 INDI\n0 TRLR\n", "INDI record without cross-reference"},
		{"missing family", "0 HEAD\n0 @I1@ INDI\n1 FAMS @F9@\n0 TRLR\n", "refers to missing family @F9@"},
		{"missing individual", "0 HEAD\n0 @F1@ FAM\n1 HUSB @I9@\n0 TRLR\n", "refers to missing individual @I9@"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := mustParse(t, tt.in)
			if !hasWarning(doc, tt.want) {
				t.Errorf("warnings %v do not mention %q", doc.Warnings, tt.want)
			}
		})
	}
}

func TestUnrecognizedDateWarnings(t *testing.T) {
	doc := mustParse(t, `0 HEAD
0 @I1@ INDI
1 BIRT
2 DATE 12.03.1850
1 DEAT
2 DATE (in the war)
1 BURI
2 DATE
1 RESI
2 DATE ABT 1900
0 @F1@ FAM
1 MARR
2 DATE sometime in May
0 @L1@ _LOC
1 NAME Neustadt
2 DATE from the beginning
1 _LOC @L2@
2 DATE until the war
0 @L2@ _LOC
1 NAME Sachsen
0 TRLR
`)
	// Phrases, empty and valid dates are fine.
	want := []string{
		`line 16: unrecognized date "from the beginning"; it is kept as text but not used as a date`,
		`line 18: unrecognized date "until the war"; it is kept as text but not used as a date`,
		`line 4: unrecognized date "12.03.1850"; it is kept as text but not used as a date`,
		`line 13: unrecognized date "sometime in May"; it is kept as text but not used as a date`,
	}
	var got []string
	for _, w := range doc.Warnings {
		got = append(got, w.String())
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("warnings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if b := doc.Individual("I1").Birth(); b.Date.String() != "12.03.1850" || b.Date.IsValid() {
		t.Errorf("birth date = %q, valid %v", b.Date.String(), b.Date.IsValid())
	}

	// Restricted data that is hidden is not mentioned in warnings either.
	doc = mustParse(t, "0 HEAD\n0 @I1@ INDI\n1 BIRT\n2 DATE 12.03.1850\n2 RESN confidential\n0 TRLR\n")
	if !hasWarning(doc, "unrecognized date") {
		t.Errorf("warnings = %v", doc.Warnings)
	}
	if red := doc.Redacted(); hasWarning(red, "unrecognized date") {
		t.Errorf("redacted warnings = %v", red.Warnings)
	}
}

func TestParseIgnoresEverythingAfterTrailer(t *testing.T) {
	doc := mustParse(t, "0 HEAD\n0 @I1@ INDI\n0 TRLR\nsome text\n0 @I2@ INDI\nmore text\n")
	if len(doc.Warnings) != 1 || doc.Warnings[0].String() != "line 4: data after TRLR record ignored" {
		t.Errorf("warnings = %v", doc.Warnings)
	}
	if trlr := doc.Records[len(doc.Records)-1]; trlr.Tag != "TRLR" || trlr.Value != "" {
		t.Errorf("last record = %q", trlr.String())
	}
	if doc.Individual("I2") != nil {
		t.Error("record after TRLR was read")
	}
}

func TestParseStrayTextBecomesContinuation(t *testing.T) {
	doc := mustParse(t, "0 HEAD\n0 @I1@ INDI\n1 NOTE line one\nline two\n0 TRLR\n")
	if got := doc.Record("I1").Val("NOTE"); got != "line one\nline two" {
		t.Errorf("NOTE = %q", got)
	}
}

func TestParseLevelJumpAttachesToPrevious(t *testing.T) {
	doc := mustParse(t, "0 HEAD\n0 @I1@ INDI\n1 BIRT\n3 DATE 1900\n0 TRLR\n")
	if d := doc.Record("I1").Path("BIRT", "DATE"); d == nil || d.Value != "1900" {
		t.Errorf("DATE not attached to BIRT: %+v", d)
	}
	if got := doc.Individual("I1").BirthDate().Year(); got != 1900 {
		t.Errorf("birth year = %d", got)
	}
}

func TestParseNoRecords(t *testing.T) {
	for _, in := range []string{"", "\n\n", "hello world\nnot gedcom"} {
		if _, err := ParseString(in); !errors.Is(err, ErrNoRecords) {
			t.Errorf("ParseString(%q) error = %v, want ErrNoRecords", in, err)
		}
	}
}

func TestParseLineEndings(t *testing.T) {
	base := []string{"0 HEAD", "0 @I1@ INDI", "1 NAME A /B/", "0 TRLR"}
	for name, sep := range map[string]string{"LF": "\n", "CRLF": "\r\n", "CR": "\r"} {
		doc := mustParse(t, strings.Join(base, sep)+sep)
		if ind := doc.Individual("I1"); ind == nil || ind.Name().Surname != "B" {
			t.Errorf("%s: individual not parsed", name)
		}
		if len(doc.Warnings) != 0 {
			t.Errorf("%s: warnings %v", name, doc.Warnings)
		}
	}
}

func TestParseFileAndReader(t *testing.T) {
	doc, err := ParseFile(filepath.Join("..", "testdata", "family.ged"))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Individuals) != 24 || len(doc.Families) != 9 || len(doc.Sources) != 2 {
		t.Errorf("counts: %d individuals, %d families, %d sources", len(doc.Individuals), len(doc.Families), len(doc.Sources))
	}
	if len(doc.Warnings) != 0 {
		t.Errorf("sample file produced warnings: %v", doc.Warnings)
	}
	if doc.Encoding != EncodingUTF8 {
		t.Errorf("encoding = %s", doc.Encoding)
	}
	if doc.SourceSoftware() != "goged sample data" {
		t.Errorf("SourceSoftware = %q", doc.SourceSoftware())
	}

	f, err := os.Open(filepath.Join("..", "testdata", "family.ged"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	doc2, err := Parse(f)
	if err != nil || len(doc2.Individuals) != 24 {
		t.Errorf("Parse(reader) = %v, %v", doc2, err)
	}

	if _, err := ParseFile(filepath.Join("..", "testdata", "does-not-exist.ged")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLinkRepair(t *testing.T) {
	// Links recorded on only one side are completed in both directions.
	doc := mustParse(t, `0 HEAD
0 @I1@ INDI
1 NAME Dad /X/
1 SEX M
1 FAMS @F1@
0 @I2@ INDI
1 NAME Mum /Y/
1 SEX F
0 @I3@ INDI
1 NAME Kid /X/
1 FAMC @F1@
0 @I4@ INDI
1 NAME Other /X/
0 @F1@ FAM
1 WIFE @I2@
1 CHIL @I4@
0 TRLR
`)
	f := doc.Family("F1")
	if f.Husband != doc.Individual("I1") {
		t.Errorf("husband not repaired: %v", f.Husband)
	}
	if len(f.Children) != 2 {
		t.Errorf("children = %d, want 2", len(f.Children))
	}
	mum := doc.Individual("I2")
	if len(mum.FamiliesAsSpouse()) != 1 {
		t.Error("wife not linked to family")
	}
	other := doc.Individual("I4")
	if other.Father() != doc.Individual("I1") || other.Mother() != mum {
		t.Error("child listed only in FAM not linked to parents")
	}
	if !hasWarning(doc, "I1 lists family F1 as spouse") || !hasWarning(doc, "I3 lists family F1 as parents") {
		t.Errorf("missing repair warnings: %v", doc.Warnings)
	}
}

func TestVoidPointers(t *testing.T) {
	doc := mustParse(t, "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @F1@ FAM\n1 HUSB @VOID@\n1 WIFE @I1@\n0 @I1@ INDI\n1 FAMS @F1@\n1 FAMC @VOID@\n0 TRLR\n")
	if len(doc.Warnings) != 0 {
		t.Errorf("@VOID@ pointers should not warn: %v", doc.Warnings)
	}
	if doc.Family("F1").Husband != nil {
		t.Error("void husband")
	}
}

func TestAncestryLoopDetected(t *testing.T) {
	doc := mustParse(t, `0 HEAD
0 @I1@ INDI
1 FAMC @F1@
1 FAMS @F2@
0 @I2@ INDI
1 FAMC @F2@
1 FAMS @F1@
0 @F1@ FAM
1 HUSB @I2@
1 CHIL @I1@
0 @F2@ FAM
1 HUSB @I1@
1 CHIL @I2@
0 TRLR
`)
	if !hasWarning(doc, "ancestry loop") {
		t.Errorf("loop not reported: %v", doc.Warnings)
	}
}

func TestGedcom7Features(t *testing.T) {
	doc := mustParse(t, `0 HEAD
1 GEDC
2 VERS 7.0
0 @N1@ SNOTE Shared note text
0 @I1@ INDI
1 NAME Pat /Doe/
1 SEX X
1 SNOTE @N1@
1 NOTE Inline
1 BIRT
2 DATE JULIAN 1 JAN 1700
0 TRLR
`)
	ind := doc.Individual("I1")
	if ind.Sex != SexIntersex {
		t.Errorf("sex = %v", ind.Sex)
	}
	notes := ind.Notes()
	if len(notes) != 2 || notes[0] != "Shared note text" || notes[1] != "Inline" {
		t.Errorf("notes = %q", notes)
	}
	if c := ind.Birth().Date.Start.Calendar; c != Julian {
		t.Errorf("calendar = %v", c)
	}
}
