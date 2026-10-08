package search

import (
	"path/filepath"
	"sort"
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

func TestFold(t *testing.T) {
	tests := map[string]string{
		"Smith":       "smith",
		"Müller":      "muller",
		"Straße":      "strasse",
		"Ærøskøbing":  "aeroskobing",
		"Łódź":        "lodz",
		"François":    "francois",
		"Þór":         "thor",
		"İstanbul":    "istanbul",
		"Dvořák":      "dvorak",
		"O’Brien":     "o'brien",
		"ĐORĐE":       "dorde",
		"":            "",
		"Ελληνικά":    "ελληνικα",
		"Søren ØSTER": "soren oster",
	}
	for in, want := range tests {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFoldDecomposedAndCombining(t *testing.T) {
	tests := map[string]string{
		"Mu\u0308ller": "muller", // decomposed ü
		"\u0301abc":    "abc",    // stray combining mark
		"ÅNGSTRÖM":     "angstrom",
		"ǅemal":        "dzemal",
		"Ǆ":            "dz",
		"\u1E9Eaa":     "ssaa", // capital sharp s
	}
	for in, want := range tests {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
	// The second call is served from the rune cache.
	for range 2 {
		if got := Fold("Ørsted Ł"); got != "orsted l" {
			t.Errorf("Fold = %q", got)
		}
	}
}

func TestWords(t *testing.T) {
	got := words("o'brien van-der berg, jr.")
	want := []string{"obrien", "van", "der", "berg", "jr"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("words = %q", got)
	}
}

func TestSoundex(t *testing.T) {
	tests := map[string]string{
		"Robert": "R163", "Rupert": "R163", "Rubin": "R150", "Ashcraft": "A261", "Ashcroft": "A261",
		"Tymczak": "T522", "Pfister": "P236", "Honeyman": "H555", "Lee": "L000", "Smith": "S530",
		"Smyth": "S530", "Schmidt": "S530", "Müller": "M460", "Miller": "M460", "Jackson": "J250",
		"O'Brien": "O165", "": "", "123": "", "A": "A000", "Washington": "W252", "Gutierrez": "G362",
	}
	for in, want := range tests {
		if got := Soundex(in); got != want {
			t.Errorf("Soundex(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCologne(t *testing.T) {
	tests := map[string]string{
		// Reference values from the description of the algorithm.
		"Wikipedia": "3412", "Müller-Lüdenscheidt": "65752682", "Breschnew": "17863",
		"Mustermann": "682766", "Musterman": "682766", "Musterow": "68273",
		"Smith": "862", "Smyth": "862", "Schmidt": "862", "Meyer": "67", "Maier": "67",
		"Anna": "06", "Max": "648", "Maks": "648", "Xaver": "4837", "Christoph": "47823",
		"Philipp": "351", "Cäsar": "487", "Claudia": "452", "Dachs": "248", "Sachs": "848",
		"": "", "123": "",
	}
	for in, want := range tests {
		if got := Cologne(in); got != want {
			t.Errorf("Cologne(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseYearRange(t *testing.T) {
	tests := []struct {
		in     string
		lo, hi int
		ok     bool
	}{
		{"1850", 1850, 1850, true},
		{"1850..1860", 1850, 1860, true},
		{"1860..1850", 1850, 1860, true},
		{"1850-1860", 1850, 1860, true},
		{"1850..", 1850, maxYear, true},
		{"..1860", minYear, 1860, true},
		{"<1850", minYear, 1849, true},
		{"<=1850", minYear, 1850, true},
		{">1850", 1851, maxYear, true},
		{">=1850", 1850, maxYear, true},
		{"~1850", 1845, 1855, true},
		{"1850s", 1850, 1859, true},
		{"1855s", 0, 0, false},
		{"..", 0, 0, false},
		{"abc", 0, 0, false},
		{"", 0, 0, false},
		{"123456", 0, 0, false},
		{"1850-03-12", 0, 0, false},
		{"<", 0, 0, false},
	}
	for _, tt := range tests {
		r, ok := parseYearRange(tt.in)
		if ok != tt.ok || (ok && (r.lo != tt.lo || r.hi != tt.hi)) {
			t.Errorf("parseYearRange(%q) = %+v, %v; want %d..%d, %v", tt.in, r, ok, tt.lo, tt.hi, tt.ok)
		}
	}
	for s, want := range map[string]bool{"1850": true, "185": true, "18": false, "12345": false, "1850..1900": true, "<1900": true, "smith": false, "": false} {
		if got := looksLikeYear(s); got != want {
			t.Errorf("looksLikeYear(%q) = %v", s, got)
		}
	}
}

func TestParse(t *testing.T) {
	q, err := Parse(`john -surname:"van der berg" b:1850..1860 ~smyth sex:F tag:birt.plac=Leeds id:@I1@ "two words"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Terms) != 8 {
		t.Fatalf("terms = %d: %+v", len(q.Terms), q.Terms)
	}
	checks := []struct {
		field  Field
		value  string
		negate bool
	}{
		{FieldText, "john", false},
		{FieldSurname, "van der berg", true},
		{FieldBorn, "1850..1860", false},
		{FieldSounds, "smyth", false},
		{FieldSex, "F", false},
		{FieldTag, "birt.plac=Leeds", false},
		{FieldID, "@I1@", false},
		{FieldText, "two words", false},
	}
	for i, c := range checks {
		tm := q.Terms[i]
		if tm.Field != c.field || tm.Value != c.value || tm.Negate != c.negate {
			t.Errorf("term %d = %+v, want %+v", i, tm, c)
		}
	}
	if q.Terms[2].years != (yearRange{1850, 1860}) || !q.Terms[2].isYears {
		t.Errorf("born years = %+v", q.Terms[2].years)
	}
	if q.Terms[3].soundex != "S530" {
		t.Errorf("soundex = %q", q.Terms[3].soundex)
	}
	if q.Terms[5].tagPath[0] != "BIRT" || q.Terms[5].tagPath[1] != "PLAC" || q.Terms[5].tagVal != "leeds" {
		t.Errorf("tag = %+v", q.Terms[5])
	}
	if q.Terms[6].folded != "i1" {
		t.Errorf("id folded = %q", q.Terms[6].folded)
	}
	if got := q.String(); got != `john -surname:"van der berg" born:1850..1860 sounds:smyth sex:F tag:birt.plac=Leeds id:@I1@ "two words"` {
		t.Errorf("String = %q", got)
	}
	if q, _ := Parse("   "); !q.IsEmpty() {
		t.Error("blank query should be empty")
	}
	if q, _ := Parse(`"" -`); len(q.Terms) != 1 || q.Terms[0].Value != "-" {
		t.Errorf("degenerate tokens: %+v", q.Terms)
	}
	if q, _ := Parse("~1850"); q.Terms[0].Field != FieldText || !q.Terms[0].isYears {
		t.Errorf("~1850 should be a fuzzy year: %+v", q.Terms[0])
	}
}

func TestParseErrors(t *testing.T) {
	tests := map[string]string{
		"foo:bar":       `unknown field "foo"`,
		"sex:z":         "sex: expects",
		"year:abc":      "year: expects",
		"alive:x":       "alive: expects",
		"has:wings":     "has: expects one of",
		`"unterminated`: "unterminated quote",
		"tag:.":         "tag: expects",
		"tag:BIRT..X":   "tag: expects",
		"sounds:123":    "sounds: expects a name",
		"name:":         "missing value for name:",
	}
	for in, want := range tests {
		_, err := Parse(in)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%q) error = %v, want %q", in, err, want)
		}
	}
	_, err := Parse("foo:bar")
	if !strings.Contains(err.Error(), "born") || strings.Contains(err.Error(), "type") {
		t.Errorf("error should list person fields: %v", err)
	}
}

func resultIDs(rs []Result) string {
	var ids []string
	for _, r := range rs {
		ids = append(ids, r.Individual.ID)
	}
	return strings.Join(ids, ",")
}

func sortedIDs(rs []Result) string {
	var ids []string
	for _, r := range rs {
		ids = append(ids, r.Individual.ID)
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

func TestSearchOrderAndEmptyQuery(t *testing.T) {
	ix := NewIndex(loadSample(t))
	if ix.Len() != 24 {
		t.Fatalf("Len = %d", ix.Len())
	}
	rs, err := ix.SearchString("")
	if err != nil {
		t.Fatal(err)
	}
	want := "I2,I13,I17,I22,I12,I23,I11,I10,I18,I19,I16,I15,I14,I8,I9,I20,I24,I3,I4,I21,I7,I1,I5,I6"
	if got := resultIDs(rs); got != want {
		t.Errorf("order = %s\nwant    %s", got, want)
	}
}

func TestSearch(t *testing.T) {
	ix := NewIndex(loadSample(t))
	tests := []struct {
		q    string
		want string // sorted IDs
	}{
		{"smith", "I1,I13,I14,I20,I21,I24,I3,I4,I7,I8,I9"},
		{"SMI", "I1,I13,I14,I20,I21,I24,I3,I4,I7,I8,I9"},
		{"mith", "I1,I13,I14,I20,I21,I24,I3,I4,I7,I8,I9"},
		{"mi", "I15,I16"}, // prefix of Miller; too short for substring matches
		{"john", "I3"},
		{"jo", "I10,I11,I12,I18,I23,I3"},
		{"john smith", "I3"},
		{`"john smith"`, "I3"},
		{`"smith john"`, ""},
		{"muller", "I15"},
		{"MÜLLER", "I15"},
		{"miller", "I15,I16"},
		{"i3", "I3"},
		{"surname:jones", "I10,I11,I12,I18,I23"},
		{"given:mary", "I4"},
		{"g:fred", "I15"},
		{`surname:"van der"`, ""},
		{"born:1850", "I9"},
		{"born:1840..1845", "I11,I12,I13,I15,I17,I7"},
		{"born:<1800", "I1,I2"},
		{"born:leeds", "I1,I3,I4"},
		{"died:manchester", "I3,I5"},
		{"place:toronto", "I18"},
		{"place:leeds", "I1,I18,I2,I22,I3,I4"},
		{"year:1905", "I18"},
		{"year:1916", "I14"},
		{"alive:1795", "I1,I2"},
		{"sex:f", "I12,I13,I16,I17,I19,I2,I21,I22,I23,I4,I5,I6,I8"},
		{"sex:u", "I24"},
		{"-sex:m -sex:f", "I24"},
		{"id:@I14@", "I14"},
		{"occu:weaver", "I1"},
		{"occupation:engineer", "I7"},
		{"note:cotton", "I3"},
		{"note:weaver", "I1"},
		{"source:census", "I3"},
		{"src:folio", "I1,I3"},
		{"any:consumption", "I1"},
		{"any:married", "I13,I3"},
		{"tag:_MILT", "I14"},
		{"tag:BIRT.PLAC=salford", "I14"},
		{"tag:famc.pedi=adopted", "I21"},
		{"tag:CAUS", "I1"},
		{"tag:NAME.TYPE=aka", "I15"},
		{"tag:BIRT.NOPE", ""},
		{"-has:parents", "I1,I10,I13,I15,I17,I19,I2,I22,I5,I6"},
		{"has:notes", "I1,I3"},
		{"has:sources", "I1,I3"},
		{"has:occupation", "I1,I3,I7"},
		{"has:media", ""},
		{"has:father -has:mother", ""},
		{"has:siblings sex:m", "I11,I20,I3,I7,I9"},
		{"has:spouse has:children has:death sex:f", "I2,I4,I5,I6"},
		{"has:birth -has:death surname:jones", "I10,I11,I12,I18,I23"},
		{"~smyth", "I1,I13,I14,I20,I21,I24,I3,I4,I7,I8,I9"},
		{"sounds:jonas", "I10,I11,I12,I18,I23"},
		{"~mueller", "I15,I16"},
		{"smith -given:john", "I1,I13,I14,I20,I21,I24,I4,I7,I8,I9"},
		{"1850", "I1,I9"},
		{"smith 1850", "I1,I9"},
		{"nobody", ""},
	}
	for _, tt := range tests {
		rs, err := ix.SearchString(tt.q)
		if err != nil {
			t.Errorf("%q: %v", tt.q, err)
			continue
		}
		if got := sortedIDs(rs); got != tt.want {
			t.Errorf("%q = %s, want %s", tt.q, got, tt.want)
		}
	}
	if _, err := ix.SearchString("bogus:x"); err == nil {
		t.Error("expected error")
	}
}

func TestSearchAlive(t *testing.T) {
	ix := NewIndex(loadSample(t))
	rs, _ := ix.SearchString("alive:1930")
	got := "," + sortedIDs(rs) + ","
	for _, in := range []string{"I14", "I20", "I21", "I23", "I15"} {
		if !strings.Contains(got, ","+in+",") {
			t.Errorf("alive:1930 should include %s: %s", in, got)
		}
	}
	for _, out := range []string{"I10", "I7", "I24", "I1"} {
		if strings.Contains(got, ","+out+",") {
			t.Errorf("alive:1930 should exclude %s: %s", out, got)
		}
	}
}

func TestSearchRanking(t *testing.T) {
	ix := NewIndex(loadSample(t))
	rs, _ := ix.SearchString("jo")
	// "Jones" and "John" are both prefix matches; ties keep name order.
	if got := resultIDs(rs); got != "I12,I23,I11,I10,I18,I3" {
		t.Errorf("jo = %s", got)
	}
	rs, _ = ix.SearchString("rose")
	if len(rs) != 1 || rs[0].Score != scoreWord {
		t.Errorf("rose = %+v", rs)
	}
	// An exact word match outranks a prefix match.
	doc, _ := gedcom.ParseString("0 HEAD\n0 @I1@ INDI\n1 NAME Annabel /A/\n0 @I2@ INDI\n1 NAME Ann /B/\n0 @I3@ INDI\n1 NAME Joanna /C/\n0 TRLR\n")
	rs, _ = NewIndex(doc).SearchString("ann")
	if got := resultIDs(rs); got != "I2,I1,I3" {
		t.Errorf("ann = %s", got)
	}
	if rs[0].Score != scoreWord || rs[1].Score != scorePrefix || rs[2].Score != scoreSubstring {
		t.Errorf("scores = %+v", rs)
	}
	rs, _ = NewIndex(doc).SearchString("i3")
	if len(rs) != 1 || rs[0].Score != scoreID {
		t.Errorf("id match = %+v", rs)
	}
}

func TestSearchMatchedName(t *testing.T) {
	ix := NewIndex(loadSample(t))
	rs, _ := ix.SearchString("smith")
	for _, r := range rs {
		want := 0
		if r.Individual.ID == "I13" {
			want = 1 // Jane Doe matches through her married name
		}
		if r.NameIndex != want {
			t.Errorf("%s: NameIndex = %d, want %d", r.Individual.ID, r.NameIndex, want)
		}
	}
	rs, _ = ix.SearchString("born:1845 frederick")
	if len(rs) != 1 || rs[0].NameIndex != 1 {
		t.Errorf("aka match = %+v", rs)
	}
	rs, _ = ix.SearchString("~smyth given:jane")
	if len(rs) != 1 || rs[0].NameIndex != 1 {
		t.Errorf("soundex match on alternate name = %+v", rs)
	}
	rs, _ = ix.SearchString("sex:m")
	if rs[0].NameIndex != 0 {
		t.Error("non-name queries report the primary name")
	}
}

func TestSearchApproximateDates(t *testing.T) {
	doc, _ := gedcom.ParseString(`0 HEAD
0 @I1@ INDI
1 BIRT
2 DATE ABT 1850
0 @I2@ INDI
1 BIRT
2 DATE BEF 1850
0 @I3@ INDI
1 BIRT
2 DATE AFT 1850
0 @I4@ INDI
1 CHR
2 DATE 1852
0 @I5@ INDI
1 BIRT
1 CHR
2 DATE 1853
0 TRLR
`)
	ix := NewIndex(doc)
	tests := map[string]string{
		"born:1852": "I1,I3,I4",
		"born:1845": "I2",
		"born:1858": "I3",
		"born:1853": "I3,I5",
		"born:1849": "I1,I2",
		"born:1851": "I1,I3",
	}
	for q, want := range tests {
		rs, _ := ix.SearchString(q)
		if got := sortedIDs(rs); got != want {
			t.Errorf("%q = %s, want %s", q, got, want)
		}
	}
}

func TestEventQuery(t *testing.T) {
	doc := loadSample(t)
	ix := NewEventIndex(doc.Events())
	if ix.Len() != 54 {
		t.Fatalf("Len = %d", ix.Len())
	}
	all := ix.Filter(EventQuery{}, false)
	if all[0].Individual.ID != "I1" || all[0].Tag != "BIRT" {
		t.Errorf("first event = %+v", all[0])
	}
	if last := all[len(all)-1]; last.Date.IsValid() || last.Tag != "OCCU" {
		t.Errorf("undated events should come last, got %+v", last)
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].Date.Compare(all[i].Date) > 0 {
			t.Fatalf("events not sorted at %d", i)
		}
	}
	if n := len(ix.Filter(EventQuery{}, true)); n != 46 {
		t.Errorf("vital events = %d, want 46", n)
	}

	tests := []struct {
		q     string
		vital bool
		want  int
	}{
		{"type:birth", true, 24},
		{"type:marr", false, 9},
		{"type:birth,death", false, 34},
		{"type:BURI", false, 1},
		{"type:mil", false, 1},
		{"1850", false, 4}, // includes an occupation from 1835 to 1870
		{"year:1850", true, 3},
		{"1840..1849", true, 10},
		{"place:leeds type:marr", false, 2},
		{"name:jones type:birth", false, 5},
		{"-type:birth -type:death", true, 12},
		{"weaver", false, 1},
		{"toronto", false, 1},
		{"müller", false, 4},
		{"nothing-matches", false, 0},
	}
	for _, tt := range tests {
		q, err := ParseEventQuery(tt.q)
		if err != nil {
			t.Errorf("%q: %v", tt.q, err)
			continue
		}
		if got := len(ix.Filter(q, tt.vital)); got != tt.want {
			t.Errorf("%q (vital %v) = %d events, want %d", tt.q, tt.vital, got, tt.want)
		}
	}

	for in, want := range map[string]string{
		"foo:bar": "unknown field",
		"sex:m":   "not available for events",
		"year:x":  "year: expects",
		"place:":  "missing value",
		`"open`:   "unterminated",
	} {
		if _, err := ParseEventQuery(in); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseEventQuery(%q) error = %v, want %q", in, err, want)
		}
	}
	if q, err := ParseEventQuery("  "); err != nil || len(q.terms) != 0 {
		t.Error("blank event query")
	}
}

func BenchmarkSearch(b *testing.B) {
	doc := loadSample(b)
	ix := NewIndex(doc)
	q, _ := Parse("smith born:1800..1900 -sex:f")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ix.Search(q)
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"john", `-surname:"van der" b:1850..1860`, "~smyth", "tag:BIRT.PLAC=x", "has:notes", `"`, "a:b:c"} {
		f.Add(s)
	}
	ix := NewIndex(loadSample(f))
	f.Fuzz(func(t *testing.T, s string) {
		q, err := Parse(s)
		if err != nil {
			return
		}
		ix.Search(q)
		if eq, err := ParseEventQuery(s); err == nil {
			_ = eq
		}
	})
}
