package genealogy

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/efi/goged/gedcom"
)

func loadSample(t testing.TB) *gedcom.Document {
	t.Helper()
	doc, err := gedcom.ParseFile(filepath.Join("..", "testdata", "family.ged"))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func mustParse(t *testing.T, s string) *gedcom.Document {
	t.Helper()
	doc, err := gedcom.ParseString(s)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestRelateSample(t *testing.T) {
	doc := loadSample(t)
	tests := []struct {
		a, b string
		want string
		kind Kind
	}{
		{"I3", "I3", "self", KindSelf},
		{"I3", "I4", "sister", KindBlood},
		{"I4", "I3", "brother", KindBlood},
		{"I8", "I9", "brother", KindBlood},
		{"I7", "I8", "half-sister", KindBlood},
		{"I8", "I7", "half-brother", KindBlood},
		{"I3", "I1", "father", KindBlood},
		{"I3", "I2", "mother", KindBlood},
		{"I1", "I3", "son", KindBlood},
		{"I1", "I4", "daughter", KindBlood},
		{"I7", "I1", "grandfather", KindBlood},
		{"I14", "I1", "great-grandfather", KindBlood},
		{"I20", "I1", "2nd great-grandfather", KindBlood},
		{"I23", "I2", "2nd great-grandmother", KindBlood},
		{"I1", "I20", "2nd great-grandson", KindBlood},
		{"I1", "I14", "great-grandson", KindBlood},
		{"I1", "I24", "2nd great-grandchild", KindBlood},
		{"I7", "I4", "aunt", KindBlood},
		{"I4", "I7", "nephew", KindBlood},
		{"I3", "I11", "nephew", KindBlood},
		{"I3", "I12", "niece", KindBlood},
		{"I3", "I18", "great-nephew", KindBlood},
		{"I18", "I3", "great-uncle", KindBlood},
		{"I23", "I3", "2nd great-uncle", KindBlood},
		{"I11", "I8", "first cousin", KindBlood},
		{"I7", "I11", "first cousin", KindBlood},
		{"I14", "I11", "first cousin once removed", KindBlood},
		{"I11", "I14", "first cousin once removed", KindBlood},
		{"I14", "I18", "second cousin", KindBlood},
		{"I20", "I18", "second cousin once removed", KindBlood},
		{"I20", "I23", "third cousin", KindBlood},
		{"I20", "I12", "first cousin twice removed", KindBlood},
		{"I14", "I16", "half first cousin", KindBlood},
		{"I14", "I8", "half-aunt", KindBlood},
		{"I8", "I14", "half-nephew", KindBlood},
		{"I20", "I21", "adoptive sister", KindAdoptive},
		{"I20", "I24", "sibling", KindBlood},
		{"I19", "I21", "adoptive daughter", KindAdoptive},
		{"I3", "I5", "wife", KindMarriage},
		{"I5", "I3", "husband", KindMarriage},
		{"I6", "I7", "stepson", KindMarriage},
		{"I7", "I6", "stepmother", KindMarriage},
		{"I10", "I3", "brother-in-law", KindMarriage},
		{"I3", "I10", "brother-in-law", KindMarriage},
		{"I15", "I9", "brother-in-law", KindMarriage},
		{"I13", "I3", "father-in-law", KindMarriage},
		{"I13", "I6", "husband's stepmother", KindMarriage},
		{"I3", "I13", "daughter-in-law", KindMarriage},
		{"I12", "I22", "wife of nephew", KindMarriage},
		{"I22", "I12", "husband's aunt", KindMarriage},
		{"I22", "I15", "no relationship found", KindNone},
	}
	for _, tt := range tests {
		a, b := doc.Individual(tt.a), doc.Individual(tt.b)
		r := Relate(a, b)
		if r.Description != tt.want || r.Kind != tt.kind {
			t.Errorf("Relate(%s %s, %s %s) = %q (kind %d), want %q (kind %d)",
				tt.a, a.DisplayName(), tt.b, b.DisplayName(), r.Description, r.Kind, tt.want, tt.kind)
		}
		if r.String() != r.Description {
			t.Error("String()")
		}
	}
}

func idsOf(list []*gedcom.Individual) string {
	var s []string
	for _, i := range list {
		s = append(s, i.ID)
	}
	return strings.Join(s, ",")
}

func TestRelateDetails(t *testing.T) {
	doc := loadSample(t)
	r := Relate(doc.Individual("I7"), doc.Individual("I11"))
	if r.UpA != 2 || r.UpB != 2 || r.Half || idsOf(r.CommonAncestors) != "I1,I2" {
		t.Errorf("first cousins: %+v", r)
	}
	r = Relate(doc.Individual("I14"), doc.Individual("I16"))
	if !r.Half || idsOf(r.CommonAncestors) != "I3" {
		t.Errorf("half cousins: %+v", r)
	}
	r = Relate(doc.Individual("I20"), doc.Individual("I1"))
	if r.UpA != 4 || r.UpB != 0 || idsOf(r.CommonAncestors) != "I1" {
		t.Errorf("ancestor: %+v", r)
	}
	r = Relate(doc.Individual("I12"), doc.Individual("I22"))
	if r.Via == nil || r.Via.ID != "I18" {
		t.Errorf("Via = %v", r.Via)
	}
	r = Relate(doc.Individual("I3"), doc.Individual("I5"))
	if r.Via == nil || r.Via.ID != "I5" {
		t.Errorf("spouse Via = %v", r.Via)
	}
	if got := Relate(nil, doc.Individual("I1")); got.Kind != KindNone {
		t.Errorf("nil: %+v", got)
	}
}

func TestRelateAdoption(t *testing.T) {
	// Markus (I6) is the birth son of Wilhelm (I18) and Mathilde (I17).
	// Gerold (I19), Mathilde's second husband, adopted him alone; later
	// Gerold's second wife Brigitte (I20) adopted him too.
	doc, err := gedcom.ParseFile(filepath.Join("..", "testdata", "Muster_GEDCOM_UTF-8.ged"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		a, b string
		want string
		kind Kind
	}{
		{"I6", "I18", "father", KindBlood},
		{"I6", "I17", "mother", KindBlood},
		{"I6", "I19", "adoptive father", KindAdoptive},
		{"I6", "I20", "adoptive mother", KindAdoptive},
		{"I19", "I6", "adoptive son", KindAdoptive},
		{"I20", "I6", "adoptive son", KindAdoptive},
		{"I17", "I19", "husband", KindMarriage},
	}
	for _, tt := range tests {
		r := Relate(doc.Individual(tt.a), doc.Individual(tt.b))
		if r.Description != tt.want || r.Kind != tt.kind {
			t.Errorf("Relate(%s, %s) = %q (kind %d), want %q (kind %d)", tt.a, tt.b, r.Description, r.Kind, tt.want, tt.kind)
		}
	}
	if r := Relate(doc.Individual("I6"), doc.Individual("I19")); r.Pedigree != gedcom.PedigreeAdopted || idsOf(r.CommonAncestors) != "I19" {
		t.Errorf("adoptive father: %+v", r)
	}
}

func TestTimelineAdoptiveRelatives(t *testing.T) {
	doc := loadSample(t)
	var got []string
	for _, e := range Timeline(doc.Individual("I21"), TimelineOptions{Relatives: true}) {
		if !e.Own() {
			got = append(got, e.Title())
		}
	}
	want := "Birth of adoptive sibling Infant Smith|Death of adoptive sibling Infant Smith|Death of adoptive father Arthur Smith"
	if strings.Join(got, "|") != want {
		t.Errorf("relatives' events = %q", got)
	}
	got = nil
	for _, e := range Timeline(doc.Individual("I14"), TimelineOptions{Relatives: true}) {
		if e.Relative != nil && e.Relative.ID == "I21" {
			got = append(got, e.Title())
		}
	}
	if len(got) != 1 || got[0] != "Birth of adoptive daughter Rose Smith" {
		t.Errorf("adoptive daughter: %q", got)
	}
}

func TestRelateTwoMarriages(t *testing.T) {
	doc, err := gedcom.ParseFile(filepath.Join("..", "testdata", "Muster_GEDCOM_UTF-8.ged"))
	if err != nil {
		t.Fatal(err)
	}
	// Karl Müller Junior's son Friedhelm married Max's daughter Christiane.
	r := Relate(doc.Individual("I22"), doc.Individual("I1"))
	if r.Description != "co-father-in-law" || r.Kind != KindMarriage || r.Via.ID != "I21" {
		t.Errorf("co-father-in-law: %+v", r)
	}
	if r := Relate(doc.Individual("I22"), doc.Individual("I2")); r.Description != "co-mother-in-law" {
		t.Errorf("co-mother-in-law: %q", r.Description)
	}

	// M and F have son S and daughter D; D married H, whose brother is B
	// and whose mother W later married M2. S married W2, whose sister is X;
	// X married Y. M2 has a son C with another wife.
	doc = mustParse(t, `0 HEAD
0 @M@ INDI
1 SEX M
0 @F@ INDI
1 SEX F
0 @S@ INDI
1 SEX M
1 FAMC @F1@
0 @D@ INDI
1 SEX F
1 FAMC @F1@
0 @H@ INDI
1 SEX M
1 FAMC @F3@
0 @B@ INDI
1 SEX M
1 FAMC @F3@
0 @W@ INDI
1 SEX F
0 @W2@ INDI
1 SEX F
1 FAMC @F5@
0 @X@ INDI
1 SEX F
1 FAMC @F5@
0 @Y@ INDI
1 SEX M
0 @M2@ INDI
1 SEX M
0 @C@ INDI
1 SEX M
1 FAMC @F7@
0 @F1@ FAM
1 HUSB @M@
1 WIFE @F@
1 CHIL @S@
1 CHIL @D@
0 @F2@ FAM
1 HUSB @H@
1 WIFE @D@
0 @F3@ FAM
1 WIFE @W@
1 CHIL @H@
1 CHIL @B@
0 @F4@ FAM
1 HUSB @S@
1 WIFE @W2@
0 @F5@ FAM
1 CHIL @W2@
1 CHIL @X@
0 @F6@ FAM
1 HUSB @Y@
1 WIFE @X@
0 @F8@ FAM
1 HUSB @M2@
1 WIFE @W@
0 @F7@ FAM
1 HUSB @M2@
1 CHIL @C@
0 TRLR
`)
	tests := []struct{ a, b, want string }{
		{"M", "W", "co-mother-in-law"},
		{"D", "B", "brother-in-law"},
		{"S", "B", "sister's husband's brother"},
		{"S", "Y", "wife's sister's husband"},
		{"H", "C", "stepbrother"},
		{"D", "M2", "husband's stepfather"},
		{"S", "C", "no relationship found"},
	}
	for _, tt := range tests {
		if got := Relate(doc.Individual(tt.a), doc.Individual(tt.b)).Description; got != tt.want {
			t.Errorf("Relate(%s, %s) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestRelateNonBirthKinds(t *testing.T) {
	// P1 and P2 have a birth son B, a foster daughter F, a stepson S and an
	// adopted grandchild G (adopted by B); G's birth father is X.
	doc := mustParse(t, `0 HEAD
0 @P1@ INDI
1 SEX M
0 @P2@ INDI
1 SEX F
0 @B@ INDI
1 SEX M
1 FAMC @F1@
0 @F@ INDI
1 SEX F
1 FAMC @F1@
2 PEDI foster
0 @S@ INDI
1 SEX M
1 FAMC @F1@
2 PEDI step
0 @G@ INDI
1 SEX F
1 FAMC @F2@
2 PEDI adopted
1 FAMC @F3@
0 @X@ INDI
1 SEX M
0 @F1@ FAM
1 HUSB @P1@
1 WIFE @P2@
1 CHIL @B@
1 CHIL @F@
1 CHIL @S@
0 @F2@ FAM
1 HUSB @B@
1 CHIL @G@
0 @F3@ FAM
1 HUSB @X@
1 CHIL @G@
0 TRLR
`)
	tests := []struct{ a, b, want string }{
		{"B", "F", "foster sister"},
		{"F", "B", "foster brother"},
		{"B", "S", "stepbrother"},
		{"S", "P1", "stepfather"},
		{"G", "P1", "adoptive grandfather"},
		{"G", "F", "foster aunt"},
		{"G", "X", "father"},
		{"X", "B", "no relationship found"},
	}
	for _, tt := range tests {
		if got := Relate(doc.Individual(tt.a), doc.Individual(tt.b)).Description; got != tt.want {
			t.Errorf("Relate(%s, %s) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestRelatePedigreeCollapse(t *testing.T) {
	// First cousins C and D marry; their child E descends twice from G.
	doc := mustParse(t, `0 HEAD
0 @G@ INDI
1 SEX M
1 FAMS @F1@
0 @GW@ INDI
1 SEX F
1 FAMS @F1@
0 @A@ INDI
1 SEX M
1 FAMC @F1@
0 @B@ INDI
1 SEX F
1 FAMC @F1@
0 @C@ INDI
1 SEX M
1 FAMC @F2@
0 @D@ INDI
1 SEX F
1 FAMC @F3@
0 @E@ INDI
1 SEX F
1 FAMC @F4@
0 @F1@ FAM
1 HUSB @G@
1 WIFE @GW@
1 CHIL @A@
1 CHIL @B@
0 @F2@ FAM
1 HUSB @A@
1 CHIL @C@
0 @F3@ FAM
1 WIFE @B@
1 CHIL @D@
0 @F4@ FAM
1 HUSB @C@
1 WIFE @D@
1 CHIL @E@
0 TRLR
`)
	tests := []struct{ a, b, want string }{
		{"E", "G", "great-grandfather"},
		{"E", "A", "grandfather"},
		{"E", "B", "grandmother"},
		{"C", "D", "first cousin"}, // blood wins over marriage
		{"G", "E", "great-granddaughter"},
	}
	for _, tt := range tests {
		if got := Relate(doc.Individual(tt.a), doc.Individual(tt.b)).Description; got != tt.want {
			t.Errorf("Relate(%s, %s) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestRelateSiblingsWithoutParents(t *testing.T) {
	doc := mustParse(t, "0 HEAD\n0 @I1@ INDI\n1 SEX M\n0 @I2@ INDI\n1 SEX F\n0 @F1@ FAM\n1 CHIL @I1@\n1 CHIL @I2@\n0 TRLR\n")
	r := Relate(doc.Individual("I1"), doc.Individual("I2"))
	if r.Description != "sister" || r.Half || len(r.CommonAncestors) != 0 {
		t.Errorf("Relate = %+v", r)
	}
}

func TestRelateSurvivesLoops(t *testing.T) {
	doc := mustParse(t, "0 HEAD\n0 @I1@ INDI\n1 FAMC @F1@\n0 @I2@ INDI\n1 FAMC @F2@\n0 @I3@ INDI\n0 @F1@ FAM\n1 HUSB @I2@\n1 CHIL @I1@\n0 @F2@ FAM\n1 HUSB @I1@\n1 CHIL @I2@\n0 TRLR\n")
	if got := Relate(doc.Individual("I1"), doc.Individual("I2")).Description; got != "parent" {
		t.Errorf("got %q", got)
	}
	if got := Relate(doc.Individual("I1"), doc.Individual("I3")).Kind; got != KindNone {
		t.Errorf("got %v", got)
	}
}

func TestDescribe(t *testing.T) {
	tests := []struct {
		upA, upB int
		sex      gedcom.Sex
		half     bool
		want     string
	}{
		{0, 0, gedcom.SexMale, false, "self"},
		{1, 0, gedcom.SexUnknown, false, "parent"},
		{2, 0, gedcom.SexFemale, false, "grandmother"},
		{3, 0, gedcom.SexMale, false, "great-grandfather"},
		{5, 0, gedcom.SexMale, false, "3rd great-grandfather"},
		{13, 0, gedcom.SexMale, false, "11th great-grandfather"},
		{0, 2, gedcom.SexUnknown, false, "grandchild"},
		{1, 1, gedcom.SexUnknown, true, "half-sibling"},
		{1, 3, gedcom.SexFemale, false, "great-niece"},
		{1, 2, gedcom.SexUnknown, false, "nephew/niece"},
		{4, 1, gedcom.SexUnknown, false, "2nd great-uncle/aunt"},
		{2, 1, gedcom.SexMale, true, "half-uncle"},
		{3, 1, gedcom.SexFemale, true, "half-great-aunt"},
		{1, 4, gedcom.SexMale, true, "half-2nd great-nephew"},
		{3, 3, gedcom.SexMale, false, "second cousin"},
		{3, 6, gedcom.SexMale, false, "second cousin thrice removed"},
		{3, 7, gedcom.SexMale, false, "second cousin 4 times removed"},
		{12, 12, gedcom.SexMale, false, "11th cousin"},
		{11, 11, gedcom.SexMale, false, "tenth cousin"},
		{4, 5, gedcom.SexFemale, true, "half third cousin once removed"},
	}
	for _, tt := range tests {
		if got := describe(tt.upA, tt.upB, tt.sex, tt.half); got != tt.want {
			t.Errorf("describe(%d, %d, %v, %v) = %q, want %q", tt.upA, tt.upB, tt.sex, tt.half, got, tt.want)
		}
	}
}

func TestOrdinal(t *testing.T) {
	tests := map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 22: "22nd", 101: "101st", 111: "111th"}
	for n, want := range tests {
		if got := Ordinal(n); got != want {
			t.Errorf("Ordinal(%d) = %q", n, got)
		}
	}
}

func titles(entries []TimelineEntry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Title())
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTimelineOwnEvents(t *testing.T) {
	doc := loadSample(t)
	entries := Timeline(doc.Individual("I3"), TimelineOptions{})
	want := []string{"Birth", "Christening", "Occupation", "Marriage", "Marriage", "Census", "Death"}
	if got := titles(entries); !equalStrings(got, want) {
		t.Fatalf("timeline = %q\nwant       %q", got, want)
	}
	if entries[3].Spouse == nil || entries[3].Spouse.ID != "I5" || entries[4].Spouse.ID != "I6" {
		t.Error("marriage spouses")
	}
	for _, e := range entries {
		if !e.Own() {
			t.Errorf("%s should be own", e.Title())
		}
	}
	ages := map[string]string{}
	for _, e := range entries {
		if e.HasAge {
			ages[e.Title()+e.Event.Date.Raw] = e.Age.String()
		}
	}
	wantAges := map[string]string{
		"Christening10 FEB 1817":      "",
		"Census30 MAR 1851":           "34",
		"Death1880":                   "~63",
		"Marriage1840":                "~23",
		"Marriage1847":                "~30",
		"OccupationFROM 1835 TO 1870": "",
	}
	for k, want := range wantAges {
		if want == "" {
			if _, ok := ages[k]; ok {
				t.Errorf("%s: birth events and periods should have no age", k)
			}
			continue
		}
		if ages[k] != want {
			t.Errorf("age at %s = %q, want %q", k, ages[k], want)
		}
	}
	if entries[0].HasAge {
		t.Error("no age for the birth itself")
	}
}

func TestTimelineWithRelatives(t *testing.T) {
	doc := loadSample(t)
	entries := Timeline(doc.Individual("I3"), TimelineOptions{Relatives: true})
	want := []string{
		"Birth", "Christening", "Birth of sister Mary Smith", "Occupation", "Marriage",
		"Birth of son Thomas Smith", "Death of wife Ann Taylor", "Marriage", "Birth of daughter Emma Smith",
		"Birth of son George Smith", "Death of father William Smith", "Death of son George Smith", "Census",
		"Death of mother Elizabeth Brown", "Marriage of son Thomas Smith", "Birth of grandson Arthur Smith",
		"Marriage of daughter Emma Smith", "Birth of granddaughter Edith Miller", "Death",
	}
	if got := titles(entries); !equalStrings(got, want) {
		t.Fatalf("timeline =\n%q\nwant\n%q", got, want)
	}
	if entries[2].Own() || entries[2].Relative.ID != "I4" || entries[2].Relation != "sister" {
		t.Errorf("relative entry: %+v", entries[2])
	}
}

func TestTimelineHalfSiblingsAndBounds(t *testing.T) {
	doc := loadSample(t)
	got := titles(Timeline(doc.Individual("I7"), TimelineOptions{Relatives: true}))
	joined := strings.Join(got, "|")
	for _, want := range []string{"Birth of half-sister Emma Smith", "Death of half-brother George Smith", "Death of mother Ann Taylor", "Death of father John Smith"} {
		if !strings.Contains(joined, want) {
			t.Errorf("timeline of Thomas lacks %q: %q", want, got)
		}
	}
	// Henry (born 1840, no death) gets a 100 year window; his father Robert
	// has no death date and must not appear.
	got = titles(Timeline(doc.Individual("I11"), TimelineOptions{Relatives: true}))
	if strings.Contains(strings.Join(got, "|"), "Robert") {
		t.Errorf("undated events must be skipped: %q", got)
	}
	// Mary's death (1899) is outside John's lifetime (died 1880).
	got = titles(Timeline(doc.Individual("I3"), TimelineOptions{Relatives: true}))
	if strings.Contains(strings.Join(got, "|"), "Death of sister") {
		t.Errorf("event after death included: %q", got)
	}
}

func TestTimelineUndatedOrdering(t *testing.T) {
	doc := mustParse(t, `0 HEAD
0 @I1@ INDI
1 BURI
1 OCCU Farmer
1 DEAT
1 RESI
2 DATE 1900
1 BIRT
1 WILL
0 TRLR
`)
	got := titles(Timeline(doc.Individual("I1"), TimelineOptions{Relatives: true}))
	want := []string{"Birth", "Residence", "Occupation", "Burial", "Death", "Will"}
	if !equalStrings(got, want) {
		t.Errorf("timeline = %q, want %q", got, want)
	}
}

func TestTimelineDeathOnlyBounds(t *testing.T) {
	doc := mustParse(t, `0 HEAD
0 @I1@ INDI
1 DEAT
2 DATE 1900
1 FAMS @F1@
0 @I2@ INDI
1 BIRT
2 DATE 1880
0 @I3@ INDI
1 BIRT
2 DATE 1750
0 @F1@ FAM
1 HUSB @I1@
1 CHIL @I2@
1 CHIL @I3@
0 TRLR
`)
	got := strings.Join(titles(Timeline(doc.Individual("I1"), TimelineOptions{Relatives: true})), "|")
	if !strings.Contains(got, "Birth of child") || strings.Count(got, "Birth of child") != 1 {
		t.Errorf("timeline = %q", got)
	}
}

func TestStats(t *testing.T) {
	doc := loadSample(t)
	s := Compute(doc)
	if s.Individuals != 24 || s.Families != 9 || s.Sources != 2 || s.Events != 54 {
		t.Errorf("counts: %+v", s)
	}
	if s.Males != 10 || s.Females != 13 || s.OtherSex != 1 {
		t.Errorf("sex: %d/%d/%d", s.Males, s.Females, s.OtherSex)
	}
	if s.Places != 11 {
		t.Errorf("places = %d", s.Places)
	}
	if len(s.Surnames) < 3 || s.Surnames[0] != (NameCount{"Smith", 10}) || s.Surnames[1] != (NameCount{"Jones", 5}) || s.Surnames[2].Name != "Brown" {
		t.Errorf("surnames = %v", s.Surnames)
	}
	if len(s.GivenNames) != 24 || s.GivenNames[0].Name != "Alice" {
		t.Errorf("given names = %v", s.GivenNames)
	}
	if s.EarliestYear != 1790 || s.LatestYear != 1940 {
		t.Errorf("years %d-%d", s.EarliestYear, s.LatestYear)
	}
	if s.Generations != 5 {
		t.Errorf("generations = %d", s.Generations)
	}
	if s.Lifespans != 10 || math.Abs(s.AverageLifespan-50) > 1e-9 {
		t.Errorf("lifespans = %d avg %.2f", s.Lifespans, s.AverageLifespan)
	}
	if s.LongestLived == nil || s.LongestLived.ID != "I4" || s.LongestAge.Years != 80 {
		t.Errorf("longest lived = %v %v", s.LongestLived, s.LongestAge)
	}
}

func TestStatsEmptyAndLoops(t *testing.T) {
	doc := mustParse(t, "0 HEAD\n0 @I1@ INDI\n1 FAMC @F1@\n1 FAMS @F1@\n0 @F1@ FAM\n1 HUSB @I1@\n1 CHIL @I1@\n0 TRLR\n")
	s := Compute(doc)
	if s.Generations < 1 || s.EarliestYear != 0 || s.Lifespans != 0 || s.AverageLifespan != 0 {
		t.Errorf("stats = %+v", s)
	}
}

// placeOutline renders a place tree as "name (events/people)" lines.
func placeOutline(root *PlaceNode) string {
	var b strings.Builder
	root.Walk(func(n *PlaceNode) bool {
		if n.Depth > 0 {
			fmt.Fprintf(&b, "%s%s (%d/%d)\n", strings.Repeat("  ", n.Depth-1), n.Name, n.Count(), n.People())
		}
		return true
	})
	return b.String()
}

func TestPlaces(t *testing.T) {
	root := Places(loadSample(t))
	want := `Canada (1/1)
  Ontario (1/1)
    Toronto (1/1)
England (19/11)
  Lancashire (8/6)
    Liverpool (1/1)
    Manchester (6/4)
    Salford (1/1)
  Yorkshire (11/7)
    Hull (1/1)
    Leeds (9/6)
      St Peter's (1/1)
      St Peter's Churchyard (1/1)
    York (1/1)
France (1/1)
Preußen (1/1)
  Köln (1/1)
`
	if got := placeOutline(root); got != want {
		t.Errorf("place tree:\n%s\nwant:\n%s", got, want)
	}
	if root.Count() != 22 || root.Depth != 0 || root.Name != "" {
		t.Errorf("root = %+v", root)
	}

	leeds := root.Children[1].Children[1].Children[1]
	if leeds.Name != "Leeds" || leeds.Full != "Leeds, Yorkshire, England" || leeds.Depth != 3 {
		t.Fatalf("leeds = %+v", leeds)
	}
	if got := strings.Join(leeds.Path(), " > "); got != "England > Yorkshire > Leeds" {
		t.Errorf("Path = %q", got)
	}
	if len(leeds.Events) != 7 {
		t.Errorf("events exactly at Leeds = %d", len(leeds.Events))
	}
	all := leeds.AllEvents()
	if len(all) != 9 {
		t.Fatalf("events in Leeds and below = %d", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].Date.Compare(all[i].Date) > 0 {
			t.Fatal("AllEvents is not chronological")
		}
	}
	if all[0].Tag != "BIRT" || all[0].Individual.ID != "I1" {
		t.Errorf("first event at Leeds = %s %v", all[0].Tag, all[0].Individual)
	}
	if leeds.Parent.Name != "Yorkshire" || leeds.Children[0].Full != "St Peter's, Leeds, Yorkshire, England" {
		t.Error("parent/children links")
	}
	if len(root.Path()) != 0 {
		t.Error("root has an empty path")
	}
}

func TestPlacesGroupedByPlaceRecord(t *testing.T) {
	doc, err := gedcom.ParseFile(filepath.Join("..", "testdata", "Muster_GEDCOM_UTF-8.ged"))
	if err != nil {
		t.Fatal(err)
	}
	// Five events refer to the place record of Tempelhof, written in three
	// ways; they are grouped under the most recent spelling.
	var tempelhof []*PlaceNode
	Places(doc).Walk(func(n *PlaceNode) bool {
		if n.Name == "Tempelhof" {
			tempelhof = append(tempelhof, n)
		}
		return true
	})
	if len(tempelhof) != 1 {
		t.Fatalf("Tempelhof appears %d times", len(tempelhof))
	}
	n := tempelhof[0]
	if n.Full != "Tempelhof, Berlin, Deutschland" || len(n.Events) != 5 || n.Location != doc.Location("P29") || n.HasCoords {
		t.Errorf("Tempelhof = %+v", n)
	}
	if got := strings.Join(n.Aliases, "|"); got != "Tempelhof, amerikanischer Sektor, Berlin (West), Deutschland|Tempelhof, Berlin (West), Deutschland" {
		t.Errorf("aliases = %q", got)
	}
	located := Places(doc).Located()
	if len(located) != 1 || located[0].Name != "Brosowo" || located[0].GOV != "BROOWOJO93FH" || located[0].Lat != 53.32 {
		t.Errorf("located = %+v", located)
	}
	if s := Compute(doc); s.Places != 23 {
		t.Errorf("Places = %d", s.Places)
	}
}

func TestPlacesUndatedPlaceRecord(t *testing.T) {
	// Without dates, the first spelling is used.
	doc := mustParse(t, `0 HEAD
0 @I1@ INDI
1 BIRT
2 PLAC Town, Old County
3 _LOC @L1@
1 DEAT
2 PLAC Town, New County
3 _LOC @L1@
1 BURI
2 DATE 1900
2 PLAC Town, Newest County
3 _LOC @L2@
1 CREM
2 PLAC Town, Undated County
3 _LOC @L2@
0 @L1@ _LOC
1 NAME Town
0 @L2@ _LOC
1 NAME Town
0 TRLR
`)
	var names []string
	Places(doc).Walk(func(n *PlaceNode) bool {
		if len(n.Events) > 0 {
			names = append(names, fmt.Sprintf("%s:%d", n.Full, len(n.Events)))
		}
		return true
	})
	if got := strings.Join(names, " "); got != "Town, Newest County:2 Town, Old County:2" {
		t.Errorf("places = %s", got)
	}
}

func TestPlacesMergeCaseAndSkipBlanks(t *testing.T) {
	doc := mustParse(t, `0 HEAD
0 @I1@ INDI
1 BIRT
2 PLAC Leeds, Yorkshire, England
1 DEAT
2 PLAC leeds,  YORKSHIRE ,england
1 BURI
2 PLAC , Yorkshire, England
1 RESI
2 PLAC
1 CENS
2 DATE 1851
0 TRLR
`)
	root := Places(doc)
	want := "England (3/1)\n  Yorkshire (3/1)\n    Leeds (2/1)\n"
	if got := placeOutline(root); got != want {
		t.Errorf("place tree:\n%s\nwant:\n%s", got, want)
	}
	yorkshire := root.Children[0].Children[0]
	if len(yorkshire.Events) != 1 || yorkshire.Events[0].Tag != "BURI" {
		t.Errorf("a place with an empty smallest part belongs to its county: %+v", yorkshire.Events)
	}
	if empty := Places(mustParse(t, "0 HEAD\n0 TRLR\n")); empty.Count() != 0 || len(empty.Children) != 0 {
		t.Error("empty document")
	}
}

func TestPlaceCoordinates(t *testing.T) {
	doc := mustParse(t, `0 HEAD
0 @I1@ INDI
1 BIRT
2 PLAC Leeds, England
1 DEAT
2 PLAC Leeds, England
3 MAP
4 LATI N53.7997
4 LONG W1.5492
1 BURI
2 PLAC leeds, england
3 MAP
4 LATI N1
4 LONG E1
1 RESI
2 PLAC York, England
0 TRLR
`)
	root := Places(doc)
	england := root.Children[0]
	leeds, york := england.Children[0], england.Children[1]
	if !leeds.HasCoords || leeds.Lat != 53.7997 || leeds.Lon != -1.5492 {
		t.Errorf("Leeds = %+v (the first coordinates win)", leeds)
	}
	if england.HasCoords || york.HasCoords {
		t.Error("places without MAP have no coordinates")
	}
	if got := root.Located(); len(got) != 1 || got[0] != leeds {
		t.Errorf("Located = %v", got)
	}
}
