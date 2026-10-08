package genealogy

import (
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
		{"I20", "I21", "sister", KindBlood},
		{"I20", "I24", "sibling", KindBlood},
		{"I19", "I21", "daughter", KindBlood},
		{"I3", "I5", "wife", KindMarriage},
		{"I5", "I3", "husband", KindMarriage},
		{"I6", "I7", "stepson", KindMarriage},
		{"I7", "I6", "stepmother", KindMarriage},
		{"I10", "I3", "brother-in-law", KindMarriage},
		{"I3", "I10", "brother-in-law", KindMarriage},
		{"I15", "I9", "brother-in-law", KindMarriage},
		{"I13", "I3", "father-in-law", KindMarriage},
		{"I13", "I6", "no relationship found", KindNone}, // husband's stepmother: too distant
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
