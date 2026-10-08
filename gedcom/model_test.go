package gedcom

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func loadSample(t testing.TB) *Document {
	t.Helper()
	doc, err := ParseFile(filepath.Join("..", "testdata", "family.ged"))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func ids(list []*Individual) string {
	var out []string
	for _, ind := range list {
		out = append(out, ind.ID)
	}
	return strings.Join(out, ",")
}

func TestIndividualAccessors(t *testing.T) {
	doc := loadSample(t)
	john := doc.Individual("@I3@")
	if john == nil {
		t.Fatal("I3 not found")
	}
	if john.DisplayName() != "John Smith" || john.SortName() != "Smith, John" {
		t.Errorf("names: %q / %q", john.DisplayName(), john.SortName())
	}
	if john.Sex != SexMale || john.Sex.String() != "male" {
		t.Errorf("sex = %v", john.Sex)
	}
	if john.Document() != doc {
		t.Error("Document()")
	}
	if got := john.Lifespan(); got != "1817–1880" {
		t.Errorf("Lifespan = %q", got)
	}
	if got := john.Birth().Place.String(); got != "Leeds, Yorkshire, England" {
		t.Errorf("birth place = %q", got)
	}
	if ids(john.Parents()) != "I1,I2" {
		t.Errorf("parents = %s", ids(john.Parents()))
	}
	if john.Father().ID != "I1" || john.Mother().ID != "I2" {
		t.Error("father/mother")
	}
	if ids(john.Spouses()) != "I5,I6" {
		t.Errorf("spouses = %s", ids(john.Spouses()))
	}
	if ids(john.Children()) != "I7,I8,I9" {
		t.Errorf("children = %s", ids(john.Children()))
	}
	if ids(john.Siblings()) != "I4" {
		t.Errorf("siblings = %s", ids(john.Siblings()))
	}
	if len(john.FamiliesAsSpouse()) != 2 || len(john.FamiliesAsChild()) != 1 {
		t.Error("family links")
	}
	if notes := john.Notes(); len(notes) != 1 || !strings.HasSuffix(notes[0], "\nHe married twice.") {
		t.Errorf("notes = %q", notes)
	}
	if got := len(john.EventsWithTag("OCCU")); got != 1 {
		t.Errorf("OCCU events = %d", got)
	}

	thomas := doc.Individual("I7")
	if ids(thomas.HalfSiblings()) != "I8,I9" {
		t.Errorf("half siblings = %s", ids(thomas.HalfSiblings()))
	}
	if len(thomas.Siblings()) != 0 {
		t.Errorf("Thomas has no full siblings, got %s", ids(thomas.Siblings()))
	}
	emma := doc.Individual("I8")
	if ids(emma.Siblings()) != "I9" || ids(emma.HalfSiblings()) != "I7" {
		t.Errorf("Emma siblings %s half %s", ids(emma.Siblings()), ids(emma.HalfSiblings()))
	}

	william := doc.Individual("I1")
	if notes := william.Notes(); len(notes) != 1 || notes[0] != "William was a hand-loom weaver in the parish of St Peter, Leeds.\nHis will was proved at York in 1850." {
		t.Errorf("shared note = %q", notes)
	}
	cits := william.Citations()
	if len(cits) != 1 || cits[0].Source == nil || cits[0].Text != "Parish register of St Peter, Leeds" || cits[0].Page != "Baptisms 1790, folio 12" || cits[0].Path != "INDI.BIRT.SOUR" {
		t.Errorf("citations = %+v", cits)
	}
	if got := william.Death().Detail(); got != "cause: Consumption" {
		t.Errorf("death detail = %q", got)
	}
	if william.Father() != nil || william.Mother() != nil || len(william.Parents()) != 0 {
		t.Error("William has no parents")
	}
}

func TestLifespans(t *testing.T) {
	doc := loadSample(t)
	tests := map[string]string{
		"I1":  "1790–1850",
		"I2":  "c.1795–1860",
		"I13": "1844–",
		"I15": "1845–",
	}
	for id, want := range tests {
		if got := doc.Individual(id).Lifespan(); got != want {
			t.Errorf("%s Lifespan = %q, want %q", id, got, want)
		}
	}
	doc2 := mustParse(t, "0 HEAD\n0 @I1@ INDI\n1 BIRT\n2 DATE 1900\n1 DEAT Y\n0 @I2@ INDI\n0 @I3@ INDI\n1 BURI\n2 DATE 1950\n0 @I4@ INDI\n1 CHR\n2 DATE BEF 1800\n0 TRLR\n")
	for id, want := range map[string]string{"I1": "1900–?", "I2": "", "I3": "–1950", "I4": "bef.1800–"} {
		if got := doc2.Individual(id).Lifespan(); got != want {
			t.Errorf("%s Lifespan = %q, want %q", id, got, want)
		}
	}
	if doc2.Individual("I2").DisplayName() != "(unnamed)" || doc2.Individual("I2").SortName() != "(unnamed)" {
		t.Error("unnamed placeholder")
	}
}

func TestAlternateNames(t *testing.T) {
	doc := loadSample(t)
	jane := doc.Individual("I13")
	if len(jane.Names) != 2 || jane.Names[1].Type != "married" || jane.Names[1].String() != "Jane Smith" {
		t.Errorf("names = %+v", jane.Names)
	}
	fritz := doc.Individual("I15")
	if fritz.DisplayName() != "Friedrich Müller" {
		t.Errorf("DisplayName = %q", fritz.DisplayName())
	}
}

func TestAdoption(t *testing.T) {
	doc := loadSample(t)
	rose := doc.Individual("I21")
	links := rose.FamiliesAsChild()
	if len(links) != 1 || links[0].Pedigree != "adopted" || links[0].IsBirth() {
		t.Errorf("links = %+v", links)
	}
	// With no birth family, the adoptive family provides the parents.
	if rose.Father() == nil || rose.Father().ID != "I14" {
		t.Errorf("father = %v", rose.Father())
	}
}

func TestPedigreePerPartner(t *testing.T) {
	doc, err := ParseFile(filepath.Join("..", "testdata", "Muster_GEDCOM_UTF-8.ged"))
	if err != nil {
		t.Fatal(err)
	}
	markus := doc.Individual("I6")
	var got []string
	for _, l := range markus.FamiliesAsChild() {
		got = append(got, fmt.Sprintf("%s:%s/%s/%s", l.Family.ID, l.Pedigree, l.Husband, l.Wife))
	}
	// F7: only the husband adopted (ADOP HUSB); the wife is the birth
	// mother from F6. F8: the wife adopted; the husband had adopted in F7.
	if want := "F6:birth/birth/birth F7:adopted/adopted/birth F8:adopted/adopted/adopted"; strings.Join(got, " ") != want {
		t.Errorf("links = %s\nwant    %s", strings.Join(got, " "), want)
	}
	var parents []string
	for _, pl := range markus.ParentLinks() {
		parents = append(parents, pl.Parent.ID+":"+string(pl.Pedigree))
	}
	if want := "I18:birth I17:birth I19:adopted I20:adopted"; strings.Join(parents, " ") != want {
		t.Errorf("ParentLinks = %s, want %s", strings.Join(parents, " "), want)
	}
	if b := idsOfInds(markus.BirthParents()); b != "I18,I17" {
		t.Errorf("BirthParents = %s", b)
	}
	if markus.Father().ID != "I18" || markus.Mother().ID != "I17" {
		t.Errorf("Father/Mother = %s/%s", markus.Father().ID, markus.Mother().ID)
	}
	f7 := markus.FamiliesAsChild()[1]
	if f7.IsBirth() || f7.Kind() != PedigreeAdopted || f7.Of(doc.Individual("I17")) != PedigreeBirth || f7.Of(nil) != PedigreeUnknown || f7.Of(markus) != PedigreeUnknown {
		t.Errorf("F7 link: %+v", f7)
	}
	if _, ok := markus.ChildLink(doc.Family("F9")); ok {
		t.Error("ChildLink of a foreign family")
	}
}

func TestPedigreeFRELMREL(t *testing.T) {
	doc, err := ParseString(`0 HEAD
0 @C@ INDI
1 FAMC @F1@
0 @D@ INDI
1 FAMC @F1@
0 @E@ INDI
1 ADOP
2 FAMC @F1@
0 @H@ INDI
1 SEX M
0 @W@ INDI
1 SEX F
0 @F1@ FAM
1 HUSB @H@
1 WIFE @W@
1 CHIL @C@
2 _FREL Step
2 _MREL Natural
1 CHIL @D@
2 _FREL Foster
1 CHIL @E@
0 TRLR
`)
	if err != nil {
		t.Fatal(err)
	}
	check := func(id string, husband, wife Pedigree, kind Pedigree) {
		t.Helper()
		l := doc.Individual(id).FamiliesAsChild()[0]
		if l.Husband != husband || l.Wife != wife || l.Kind() != kind {
			t.Errorf("%s: %+v, kind %q", id, l, l.Kind())
		}
	}
	check("C", PedigreeStep, PedigreeBirth, PedigreeStep)
	check("D", PedigreeFoster, PedigreeUnknown, PedigreeFoster)
	check("E", PedigreeAdopted, PedigreeAdopted, PedigreeAdopted) // ADOP without HUSB/WIFE: both
	if half := doc.Individual("C").HalfSiblings(); len(half) != 0 {
		t.Errorf("HalfSiblings = %v", half)
	}
}

func TestParsePedigree(t *testing.T) {
	for in, want := range map[string]Pedigree{
		"": PedigreeUnknown, "Unknown": PedigreeUnknown, "birth": PedigreeBirth, "BIRTH": PedigreeBirth,
		"Natural": PedigreeBirth, "adopted": PedigreeAdopted, "ADOPTED": PedigreeAdopted, "foster": PedigreeFoster,
		"sealed": PedigreeSealed, "SEALING": PedigreeSealed, "Step": PedigreeStep, "OTHER": PedigreeOther, "guardian": PedigreeOther,
	} {
		if got := ParsePedigree(in); got != want {
			t.Errorf("ParsePedigree(%q) = %q, want %q", in, got, want)
		}
	}
	for p, want := range map[Pedigree]string{
		PedigreeUnknown: "", PedigreeBirth: "", PedigreeAdopted: "adoptive", PedigreeFoster: "foster",
		PedigreeStep: "step", PedigreeSealed: "sealed", PedigreeOther: "non-biological",
	} {
		if got := p.Adjective(); got != want {
			t.Errorf("%q.Adjective() = %q, want %q", p, got, want)
		}
	}
}

func idsOfInds(list []*Individual) string {
	var s []string
	for _, i := range list {
		s = append(s, i.ID)
	}
	return strings.Join(s, ",")
}

func TestFamilyAccessors(t *testing.T) {
	doc := loadSample(t)
	f := doc.Family("F6")
	if f.Title() != "Friedrich Müller & Emma Smith" {
		t.Errorf("Title = %q", f.Title())
	}
	if f.Marriage() == nil || f.Marriage().Date.Year() != 1870 {
		t.Error("marriage")
	}
	if div := f.FirstEvent("DIV"); div == nil || !div.IsVital() || div.Label() != "Divorce" {
		t.Error("divorce")
	}
	if f.Partner(f.Husband) != f.Wife || f.Partner(f.Wife) != f.Husband || f.Partner(doc.Individual("I1")) != nil {
		t.Error("Partner")
	}
	marr := f.Marriage()
	if ids(marr.Principals()) != "I15,I8" {
		t.Errorf("principals = %s", ids(marr.Principals()))
	}
	lonely := &Family{Wife: doc.Individual("I2")}
	if lonely.Title() != "? & Elizabeth Brown" {
		t.Errorf("Title = %q", lonely.Title())
	}
	if len(f.Notes()) != 0 {
		t.Error("no family notes expected")
	}
}

func TestEvents(t *testing.T) {
	doc := loadSample(t)
	all := doc.Events()
	if len(all) != 54 {
		t.Errorf("only %d events", len(all))
	}
	arthur := doc.Individual("I14")
	milt := arthur.FirstEvent("_MILT")
	if milt == nil {
		t.Fatal("_MILT not recognized as event")
	}
	if milt.Label() != "Military service" || milt.Date.Modifier != DateFromTo || milt.Place.Name != "France" {
		t.Errorf("milt = %+v", milt)
	}
	if ids(milt.Principals()) != "I14" {
		t.Error("principals")
	}
	occu := doc.Individual("I3").FirstEvent("OCCU")
	if occu.Class() != ClassAttribute || occu.Detail() != "Mill worker" || occu.IsVital() {
		t.Errorf("occu = %+v", occu)
	}
}

func TestEventDetailsAndLabels(t *testing.T) {
	doc := mustParse(t, `0 HEAD
0 @I1@ INDI
1 EVEN
2 TYPE Award
2 DATE 1920
1 FACT Tall
2 TYPE Height
1 _CUSTOM
2 PLAC Somewhere
1 _NODATE something
1 BIRT Y
2 ADDR 12 High Street
3 CONT Flat 2
3 CITY Leeds
3 CTRY England
2 NOTE Born at home
1 RESI
2 DATE 1930
0 TRLR
`)
	ind := doc.Individual("I1")
	var labels []string
	for _, e := range ind.Events {
		labels = append(labels, e.Label())
	}
	if want := []string{"Award", "Height", "Custom", "Birth", "Residence"}; !reflect.DeepEqual(labels, want) {
		t.Errorf("labels = %q, want %q", labels, want)
	}
	if d := ind.Events[1].Detail(); d != "Tall" {
		t.Errorf("FACT detail = %q", d)
	}
	birth := ind.Birth()
	if birth.Detail() != "" {
		t.Errorf("'Y' should not be a detail: %q", birth.Detail())
	}
	if birth.Address != "12 High Street, Flat 2, Leeds, England" {
		t.Errorf("address = %q", birth.Address)
	}
	if len(birth.Notes) != 1 || birth.Notes[0] != "Born at home" {
		t.Errorf("event notes = %q", birth.Notes)
	}
	if got := EventLabel("_MY_THING"); got != "My thing" {
		t.Errorf("EventLabel = %q", got)
	}
	if got := EventLabel("_"); got != "_" {
		t.Errorf("EventLabel(_) = %q", got)
	}
}

func TestEventTagsByLabel(t *testing.T) {
	got := EventTagsByLabel("bir")
	if len(got) != 1 || got[0] != "BIRT" {
		t.Errorf("bir -> %v", got)
	}
	marr := EventTagsByLabel("marriage")
	if len(marr) != 5 { // MARR, MARB, MARC, MARL, MARS
		t.Errorf("marriage -> %v", marr)
	}
	if EventTagsByLabel(" ") != nil {
		t.Error("empty prefix")
	}
}

func TestPlace(t *testing.T) {
	p := Place{Name: " Leeds ,Yorkshire,, England "}
	if got := p.Parts(); !reflect.DeepEqual(got, []string{"Leeds", "Yorkshire", "England"}) {
		t.Errorf("Parts = %q", got)
	}
	if p.String() != "Leeds, Yorkshire, England" || p.Short() != "Leeds" {
		t.Errorf("String/Short = %q/%q", p.String(), p.Short())
	}
	if (Place{}).Short() != "" {
		t.Error("empty Short")
	}
}

func TestSexParsing(t *testing.T) {
	tests := map[string]Sex{"M": SexMale, "f": SexFemale, "male": SexMale, "X": SexIntersex, "U": SexUnknown, "": SexUnknown}
	for in, want := range tests {
		if got := parseSex(in); got != want {
			t.Errorf("parseSex(%q) = %q", in, got)
		}
	}
	for s, want := range map[Sex]string{SexMale: "male", SexFemale: "female", SexIntersex: "intersex", SexUnknown: "unknown"} {
		if s.String() != want {
			t.Errorf("%q.String() = %q", s, s.String())
		}
	}
}

func TestSourceRecords(t *testing.T) {
	doc := mustParse(t, `0 HEAD
0 @S1@ SOUR
1 TITL A long
2 CONC  title
0 @S2@ SOUR
1 ABBR Abbr
0 @S3@ SOUR
0 @I1@ INDI
1 SOUR Inline source text
1 SOUR @S9@
1 SOUR
2 TITL Titled inline
0 TRLR
`)
	if doc.Source("S1").Title != "A long title" || doc.Source("S2").Title != "Abbr" || doc.Source("S3").Title != "Source S3" {
		t.Errorf("titles: %q %q %q", doc.Source("S1").Title, doc.Source("S2").Title, doc.Source("S3").Title)
	}
	cits := doc.Individual("I1").Citations()
	if len(cits) != 3 || cits[0].Text != "Inline source text" || cits[1].Text != "@S9@" || cits[2].Text != "Titled inline" {
		t.Errorf("citations = %+v", cits)
	}
}

func TestParseCoordinate(t *testing.T) {
	tests := []struct {
		in     string
		lat    bool
		want   float64
		wantOK bool
	}{
		{"N50.781464", true, 50.781464, true},
		{"S33.8688", true, -33.8688, true},
		{"E10.786089", false, 10.786089, true},
		{"W0.1278", false, -0.1278, true},
		{" n 51.5 ", true, 51.5, true},
		{"-0.1278", false, -0.1278, true},
		{"51.5", true, 51.5, true},
		{"51.5N", true, 51.5, true},
		{"0.12W", false, -0.12, true},
		{"N50,78", true, 50.78, true},
		{"N91", true, 0, false},
		{"E181", false, 0, false},
		{"W180", false, -180, true},
		{"", true, 0, false},
		{"N", true, 0, false},
		{"north", true, 0, false},
		{"E10.7", true, 0, false}, // wrong hemisphere letter for a latitude
		{"NaN", true, 0, false},
		{"Inf", false, 0, false},
	}
	for _, tt := range tests {
		pos, neg, limit := byte('E'), byte('W'), 180.0
		if tt.lat {
			pos, neg, limit = 'N', 'S', 90
		}
		got, ok := ParseCoordinate(tt.in, pos, neg, limit)
		if ok != tt.wantOK || (ok && got != tt.want) {
			t.Errorf("ParseCoordinate(%q, lat=%v) = %v, %v; want %v, %v", tt.in, tt.lat, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestPlaceCoordinates(t *testing.T) {
	doc := mustParse(t, `0 HEAD
0 @I1@ INDI
1 BIRT
2 PLAC TheSpecificPlace
3 MAP
4 LATI N50.781464
4 LONG E10.786089
1 DEAT
2 PLAC Somewhere
2 MAP
3 LATI S12.5
3 LONG W77
1 BURI
2 PLAC Broken
3 MAP
4 LATI N50.7
1 RESI
2 PLAC Nowhere
0 TRLR
`)
	ind := doc.Individual("I1")
	birth := ind.Birth().Place
	if !birth.HasCoords || birth.Lat != 50.781464 || birth.Lon != 10.786089 || birth.Name != "TheSpecificPlace" {
		t.Errorf("birth place = %+v", birth)
	}
	if d := ind.Death().Place; !d.HasCoords || d.Lat != -12.5 || d.Lon != -77 {
		t.Errorf("MAP below the event: %+v", d)
	}
	if b := ind.FirstEvent("BURI").Place; b.HasCoords {
		t.Errorf("incomplete MAP must be ignored: %+v", b)
	}
	if r := ind.FirstEvent("RESI").Place; r.HasCoords || r.Name != "Nowhere" {
		t.Errorf("place without MAP: %+v", r)
	}
}

func TestLocations(t *testing.T) {
	doc, err := ParseFile(filepath.Join("..", "testdata", "Muster_GEDCOM_UTF-8.ged"))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Locations) != 11 || len(doc.Warnings) != 0 {
		t.Fatalf("%d locations, warnings %v", len(doc.Locations), doc.Warnings)
	}
	tempelhof := doc.Location("@P29@")
	if tempelhof.Name() != "Tempelhof" || tempelhof.Type != "Stadtbezirk" || tempelhof.HasCoords || len(tempelhof.Parents) != 3 {
		t.Fatalf("P29 = %+v", tempelhof)
	}
	if p := tempelhof.Parents[2]; p.Location.Name() != "Berlin" || p.Type != "POLI" || p.Date.String() != "from 3 Oct 1990" {
		t.Errorf("third parent = %+v (%s)", p, p.Date)
	}
	brosowo := doc.Location("P_BROOWOJO93FH")
	if !brosowo.HasCoords || brosowo.Lat != 53.32 || brosowo.Lon != 18.42 || brosowo.GOV != "BROOWOJO93FH" ||
		len(brosowo.Names) != 2 || brosowo.Names[1].Name != "Brzozowo" || brosowo.Names[1].Lang != "Polish" {
		t.Errorf("Brosowo = %+v", brosowo)
	}
	if brosowo.GOVURL() != "https://gov.genealogy.net/item/show/BROOWOJO93FH" || (&Location{}).GOVURL() != "" || (&Location{}).Name() != "" {
		t.Error("GOVURL/Name")
	}
	// Erika's birth refers to P29 without coordinates; Max's birth to
	// Brosowo, whose coordinates come from the place record.
	birth := doc.Individual("I2").Birth()
	if birth.Place.Location != tempelhof || birth.Place.HasCoords || birth.Place.GOV != "" {
		t.Errorf("Erika's birth place = %+v", birth.Place)
	}
	birth = doc.Individual("I1").Birth()
	if birth.Place.Location != brosowo || !birth.Place.HasCoords || birth.Place.Lat != 53.32 || birth.Place.GOV != "BROOWOJO93FH" {
		t.Errorf("Max's birth place = %+v", birth.Place)
	}
	if got := strings.Join(birth.Place.Names(), "|"); got != "Brosowo, Kulm, Bromberg, Danzig-Westpreussen, Deutsches Reich|Brosowo|Brzozowo" {
		t.Errorf("Names = %q", got)
	}
}

func TestLocationEdgeCases(t *testing.T) {
	doc, err := ParseString(`0 HEAD
0 @I1@ INDI
1 BIRT
2 PLAC
3 _LOC @L1@
1 DEAT
2 PLAC Somewhere
3 MAP
4 LATI N1
4 LONG E2
3 _LOC @L1@
3 _GOV OWN_ID
1 BURI
2 PLAC Nowhere
3 _LOC @MISSING@
0 @L1@ _LOC
1 NAME Town
1 MAP
2 LATI S10.5
2 LONG W20.25
1 _LOC @L2@
1 _LOC @L3@
0 @L2@ _LOC
1 TYPE Country
0 _LOC
1 NAME no identifier
0 TRLR
`)
	if err != nil {
		t.Fatal(err)
	}
	ind := doc.Individual("I1")
	// A place without text takes the name of its record.
	if p := ind.Birth().Place; p.Name != "Town" || p.Lat != -10.5 || p.Lon != -20.25 {
		t.Errorf("birth place = %+v", p)
	}
	// Coordinates and GOV identifiers below PLAC win over the record's.
	if p := ind.Death().Place; p.Lat != 1 || p.Lon != 2 || p.GOV != "OWN_ID" {
		t.Errorf("death place = %+v", p)
	}
	if p := ind.FirstEvent("BURI").Place; p.Location != nil || p.Name != "Nowhere" {
		t.Errorf("burial place = %+v", p)
	}
	if l := doc.Location("L1"); len(l.Parents) != 1 || l.Parents[0].Location != doc.Location("L2") {
		t.Errorf("parents = %+v", l.Parents)
	}
	var msgs []string
	for _, w := range doc.Warnings {
		msgs = append(msgs, w.Msg)
	}
	want := []string{
		"place L1 refers to missing place @L3@",
		"_LOC record without cross-reference identifier ignored",
		`place "Nowhere" refers to missing place record @MISSING@`,
	}
	for _, w := range want {
		if !containsString(msgs, w) {
			t.Errorf("missing warning %q in %q", w, msgs)
		}
	}
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
