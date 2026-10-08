package gedcom

import (
	"strings"
	"testing"
)

func TestIsRestricted(t *testing.T) {
	for v, want := range map[string]bool{
		"confidential": true, "privacy": true, "PRIVACY": true, "CONFIDENTIAL, LOCKED": true,
		"locked": false, "": false, "LOCKED, other": false,
	} {
		if got := IsRestricted(v); got != want {
			t.Errorf("IsRestricted(%q) = %v", v, got)
		}
	}
}

func TestRedacted(t *testing.T) {
	doc, err := ParseString(`0 HEAD
0 @I1@ INDI
1 NAME Open /Person/
1 BIRT
2 DATE 1900
2 RESN confidential
1 DEAT
2 DATE 1980
2 RESN locked
1 NAME Secret /Alias/
2 RESN privacy
1 FAMS @F1@
0 @I2@ INDI
1 RESN privacy
1 NAME Living /Person/
1 SEX F
1 BIRT
2 DATE 1990
1 NOTE private note
1 FAMS @F1@
0 @F1@ FAM
1 RESN CONFIDENTIAL, LOCKED
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 DATE 2010
0 @S1@ SOUR
1 RESN confidential
1 TITL Secret source
0 @F2@ FAM
1 HUSB @I9@
0 TRLR
`)
	if err != nil {
		t.Fatal(err)
	}
	r := doc.Redacted()
	if r.Redactions != 5 {
		t.Errorf("Redactions = %d", r.Redactions)
	}
	if doc.Redactions != 0 || doc.Individual("I1").Birth() == nil {
		t.Error("the original document must not change")
	}
	open := r.Individual("I1")
	if open.Birth() != nil || open.Death() == nil || len(open.Names) != 1 {
		t.Errorf("I1: events %v, names %v", open.Events, open.Names)
	}
	living := r.Individual("I2")
	if living.DisplayName() != "Living Person" || living.Sex != SexFemale || len(living.Events) != 0 || len(living.Notes()) != 0 {
		t.Errorf("I2 = %+v, notes %v", living, living.Notes())
	}
	f := r.Family("F1")
	if f.Husband != open || f.Wife != living || len(f.Events) != 0 || len(living.Spouses()) != 1 {
		t.Errorf("F1 = %+v", f)
	}
	if s := r.Source("S1"); s.Title != "Source S1" {
		t.Errorf("source title = %q", s.Title)
	}
	// Warnings are not duplicated by the rebuild.
	if len(r.Warnings) != len(doc.Warnings) || len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0].Msg, "missing individual") {
		t.Errorf("warnings %v, original %v", r.Warnings, doc.Warnings)
	}
	if r.Encoding != doc.Encoding {
		t.Error("encoding")
	}
	if e := doc.Individual("I1").Birth(); e.Restriction != "confidential" {
		t.Errorf("Restriction = %q", e.Restriction)
	}
}
