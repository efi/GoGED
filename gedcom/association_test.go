package gedcom

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAssociations(t *testing.T) {
	doc, err := ParseFile(filepath.Join("..", "testdata", "Muster_GEDCOM_UTF-8.ged"))
	if err != nil {
		t.Fatal(err)
	}
	// Max Herbert Mustermann (I35) is the godparent of Karl Junior (I22).
	karl, max := doc.Individual("I22"), doc.Individual("I35")
	as := karl.Associations()
	if len(as) != 1 || as[0].To != max || as[0].Label() != "Godparent" || as[0].From != karl || as[0].Event != nil {
		t.Fatalf("Karl's associations = %+v", as)
	}
	if by := max.AssociatedBy(); len(by) != 1 || by[0] != as[0] || by[0].Principals()[0] != karl {
		t.Errorf("Max is associated by %+v", by)
	}
	// Hans (I9) and Elton John (I32), with a note.
	hans := doc.Individual("I9")
	if as := hans.Associations(); len(as) != 1 || as[0].To.ID != "I32" || as[0].Label() != "Langjähriger Weggefährte" ||
		len(as[0].Notes) != 1 || !strings.HasPrefix(as[0].Notes[0], "Dave Sample spielte") {
		t.Errorf("Hans's associations = %+v", as)
	}
	// Witnesses recorded with _ASSO below the marriages of Max and Erika.
	var witnesses []string
	for _, a := range doc.Individual("I1").Associations() {
		witnesses = append(witnesses, a.To.ID+":"+a.Label()+":"+a.Event.Tag)
		if a.Family == nil || a.From != nil || len(a.Principals()) != 2 {
			t.Errorf("witness association = %+v", a)
		}
	}
	if got := strings.Join(witnesses, " "); got != "I4:Witness of marriage:MARR I12:Witness of marriage:MARR I14:Witness of marriage:MARR I13:Witness of marriage:MARR" {
		t.Errorf("witnesses = %s", got)
	}
	// ALIA links in both directions.
	if a := max.Aliases(); len(a) != 1 || a[0].ID != "I36" {
		t.Errorf("aliases of I35 = %v", a)
	}
	if a := doc.Individual("I36").Aliases(); len(a) != 1 || a[0] != max {
		t.Errorf("aliases of I36 = %v", a)
	}
	if len(karl.Aliases()) != 0 {
		t.Error("Karl has no aliases")
	}
}

func TestAssociationLabels(t *testing.T) {
	for rel, want := range map[string]string{
		"GODP": "Godparent", "witn": "Witness", "Witness_of_Marriage": "Witness of marriage",
		"": "Associate", "best friend": "Best friend", "Trauzeuge": "Trauzeuge",
	} {
		if got := (&Association{Relation: rel}).Label(); got != want {
			t.Errorf("Label(%q) = %q, want %q", rel, got, want)
		}
	}
	if (&Association{}).Principals() != nil {
		t.Error("Principals of an empty association")
	}
}

func TestAssociationEdgeCases(t *testing.T) {
	doc, err := ParseString(`0 HEAD
0 @I1@ INDI
1 ALIA John /Doe/
1 ALIA @I9@
1 ASSO @I9@
2 ROLE FRIEND
1 ASSO @F1@
1 ASSO not a pointer
0 @I2@ INDI
1 ASSO @I1@
2 ROLE GODP
0 @F1@ FAM
0 TRLR
`)
	if err != nil {
		t.Fatal(err)
	}
	i1 := doc.Individual("I1")
	if len(i1.Names) != 1 || i1.Names[0].String() != "John Doe" || i1.Names[0].Type != "alias" {
		t.Errorf("alias name = %+v", i1.Names)
	}
	if len(i1.Associations()) != 0 || len(i1.Aliases()) != 0 {
		t.Errorf("unresolved links: %v %v", i1.Associations(), i1.Aliases())
	}
	if by := i1.AssociatedBy(); len(by) != 1 || by[0].Label() != "Godparent" {
		t.Errorf("AssociatedBy = %+v", by)
	}
	var msgs []string
	for _, w := range doc.Warnings {
		msgs = append(msgs, w.Msg)
	}
	if got := strings.Join(msgs, "|"); got != "association refers to missing record @I9@|I1 refers to missing alias record @I9@" {
		t.Errorf("warnings = %q", got)
	}
}

func TestRecordDetails(t *testing.T) {
	doc, err := ParseFile(filepath.Join("..", "testdata", "Muster_GEDCOM_UTF-8.ged"))
	if err != nil {
		t.Fatal(err)
	}
	max := doc.Individual("I1")
	if max.Name().CallName != "Max" || strings.Join(max.RefNumbers(), ",") != "1" {
		t.Errorf("call name %q, REFN %v", max.Name().CallName, max.RefNumbers())
	}
	if got := doc.Individual("I35").Changed(); got != "29 Sep 2012 20:46:42" {
		t.Errorf("Changed = %q", got)
	}
	if max.Changed() != "" || doc.Family("F1").Changed() != "31 May 2012 15:07:42" {
		t.Errorf("Changed = %q, %q", max.Changed(), doc.Family("F1").Changed())
	}
	var quotes int
	for _, c := range doc.Family("F1").Citations() {
		if strings.HasPrefix(c.Quote, "F4 Standesamtliche und kirchliche Trauung") {
			quotes++
		}
	}
	if quotes != 2 {
		t.Errorf("%d citations quote F4", quotes)
	}
	chr := doc.Individual("I37").FirstEvent("CHR")
	var facts []string
	for _, f := range chr.Facts() {
		facts = append(facts, f.Label+"="+f.Value)
	}
	if got := strings.Join(facts, "|"); got != "address=Michaelikirche, Wenningen|religion=evangelisch|godparents=Max Herbert Mustermann, Musiker" {
		t.Errorf("facts = %q", got)
	}
	marr := doc.Family("F1").Marriage()
	if f := marr.Facts(); len(f) != 2 || f[1] != (Fact{"witnesses", "Otto Mustermann, Franz-Xaver Gabler"}) {
		t.Errorf("marriage facts = %+v", f)
	}
}
