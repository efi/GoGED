package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/efi/goged/genealogy"
)

// runJSON runs goged with -json and decodes what it prints into v.
func runJSON(t *testing.T, v any, args ...string) {
	t.Helper()
	r := runCLI(t, "", append([]string{"-json"}, args...)...)
	if r.code != 0 {
		t.Fatalf("%v: exit %d: %s", args, r.code, r.stderr)
	}
	dec := json.NewDecoder(strings.NewReader(r.stdout))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, r.stdout)
	}
	if strings.Contains(r.stdout, `\u0026`) {
		t.Error("& is escaped")
	}
}

func TestJSONQuery(t *testing.T) {
	var rs []jsonResult
	runJSON(t, &rs, "-q", "born:<1800 | smith born:1893", sample)
	if len(rs) != 3 || rs[0].ID != "I21" || rs[1].ID != "I2" || rs[2].ID != "I1" {
		t.Fatalf("results %+v", rs)
	}
	e := rs[1]
	if e.Name != "Elizabeth Brown" || e.Sex != "female" || e.Lifespan != "c.1795–1860" || e.Birth == nil ||
		e.Birth.Date != "about 1795" || e.Birth.DateGEDCOM != "ABT 1795" || e.Birth.Year != 1795 || e.Birth.Place != "York, Yorkshire, England" {
		t.Errorf("Elizabeth %+v, birth %+v", e, e.Birth)
	}
	if d := rs[2].Death; d == nil || d.Type != "DEAT" || d.Detail != "cause: Consumption" {
		t.Errorf("William's death %+v", d)
	}
	runJSON(t, &rs, "-q", "smith given:jane", sample)
	if len(rs) != 1 || rs[0].MatchedName != "Jane Smith" {
		t.Errorf("matched name %+v", rs)
	}
	runJSON(t, &rs, "-q", "nobody-at-all", sample)
	if rs == nil || len(rs) != 0 {
		t.Errorf("no results: %+v", rs)
	}
}

func TestJSONCharts(t *testing.T) {
	var tr jsonTree
	runJSON(t, &tr, "-tree", "I14", "-gen", "3", sample)
	if tr.ID != "I14" || tr.Father == nil || tr.Father.ID != "I7" || tr.Mother == nil || tr.Mother.ID != "I13" ||
		tr.Father.Father == nil || tr.Father.Father.ID != "I3" || !tr.Father.Father.MoreAncestors || tr.Families != nil {
		t.Errorf("pedigree %+v", tr)
	}

	tr = jsonTree{}
	runJSON(t, &tr, "-desc", "I14", sample)
	if tr.Father != nil || len(tr.Families) != 1 {
		t.Fatalf("descendants %+v", tr)
	}
	f := tr.Families[0]
	if f.ID != "F8" || f.Partner == nil || f.Partner.ID != "I19" || f.Marriage == nil || f.Marriage.Year != 1890 || len(f.Children) != 3 ||
		f.Children[1].ID != "I21" || f.Children[1].Link != "adopted" || f.Children[0].Link != "" {
		t.Errorf("family %+v", f)
	}

	tr = jsonTree{}
	runJSON(t, &tr, "-hourglass", "I3", "-gen", "2", sample)
	if tr.Father == nil || tr.Father.ID != "I1" || len(tr.Families) != 2 || tr.Families[0].Partner.ID != "I5" ||
		len(tr.Families[0].Children) != 1 || tr.Families[0].Children[0].ID != "I7" || !tr.Families[0].Children[0].MoreDescendants ||
		tr.Families[1].Children[0].Families != nil {
		t.Errorf("hourglass %+v", tr)
	}

	// People who appear more than once: their ancestors and descendants
	// are given once, as in the charts.
	cousins := filepath.Join("testdata", "cousins.ged")
	tr = jsonTree{}
	runJSON(t, &tr, "-tree", "R", "-gen", "5", cousins)
	var repeated []string
	var walk func(*jsonTree)
	walk = func(n *jsonTree) {
		if n == nil {
			return
		}
		if n.AncestorsElsewhere {
			repeated = append(repeated, n.ID)
		}
		walk(n.Father)
		walk(n.Mother)
	}
	walk(&tr)
	if strings.Join(repeated, ",") != "G1" {
		t.Errorf("ancestors given elsewhere for %v", repeated)
	}
	tr = jsonTree{}
	runJSON(t, &tr, "-desc", "G1", "-gen", "5", cousins)
	out, _ := json.Marshal(tr)
	if strings.Count(string(out), `"descendants_elsewhere":true`) != 1 || strings.Count(string(out), `"id":"R"`) != 2 {
		t.Errorf("Rosa twice among the descendants:\n%s", out)
	}
}

func TestJSONLifespans(t *testing.T) {
	var ls jsonLifespans
	runJSON(t, &ls, "-lifespans", "I3", sample)
	if ls.Person == nil || ls.Person.ID != "I3" || ls.Generations != 1 || len(ls.People) != 9 {
		t.Fatalf("lifespans %+v", ls)
	}
	life := map[string]jsonLife{}
	for _, l := range ls.People {
		life[l.ID] = l
	}
	if w := life["I1"]; w.Relation != "father" || w.Generation != 1 || w.Together == nil || w.Together.Years != 32 || w.Birth == nil {
		t.Errorf("William %+v", w)
	}
	if e := life["I2"]; e.BornYears == nil || *e.BornYears != (jsonYears{1793, 1797}) {
		t.Errorf("Elizabeth, born about 1795: %+v", e.BornYears)
	}
	if a := life["I5"]; !a.Partner || len(a.Marriages) != 1 || a.Marriages[0] != 1840 || a.Together == nil || !a.Together.Married || a.Together.Years != 5 {
		t.Errorf("Ann %+v, %+v", a, a.Together)
	}
	if e := life["I8"]; e.DiedYears != nil || e.LastRecord != 1885 || e.FirstRecord != 0 || e.Generation != -1 {
		t.Errorf("Emma %+v", e)
	}
	if j := life["I3"]; j.Relation != "" || j.Generation != 0 || len(j.Marriages) != 2 || j.Together.Years != 63 {
		t.Errorf("John %+v", j)
	}
	runJSON(t, &ls, "-lifespans", "I14", "-gen", "2", sample)
	if ls.Generations != 2 || ls.People[0].Relation != "grandfather" {
		t.Errorf("two generations: %+v", ls.People[0])
	}
}

func TestJSONPeopleAndEvents(t *testing.T) {
	var tl jsonTimeline
	runJSON(t, &tl, "-timeline", "I14", sample)
	found := false
	for _, e := range tl.Events {
		if e.Title == "Birth of son Harold Smith" && e.Relation == "son" && e.Relative != nil && e.Relative.ID == "I20" && e.Age != "" {
			found = true
		}
	}
	if tl.Person.ID != "I14" || !found || tl.Events[0].Type != "BIRT" {
		t.Errorf("timeline %+v", tl)
	}

	var rel jsonRelationship
	runJSON(t, &rel, "-relate", "I20,I23", sample)
	if rel.Kind != "blood" || rel.Description != "third cousin" || rel.Sentence != "Dorothy Jones is Harold Smith's third cousin." ||
		rel.UpA != 4 || len(rel.CommonAncestors) != 2 || rel.CommonAncestors[0].ID != "I1" || rel.A.ID != "I20" {
		t.Errorf("relationship %+v", rel)
	}
	runJSON(t, &rel, "-relate", "I22,I15", sample)
	if rel.Kind != "none" || rel.Sentence != "No relationship found between Grace Hill and Friedrich Müller." {
		t.Errorf("no relationship %+v", rel)
	}

	var evs []jsonEventPeople
	runJSON(t, &evs, "-events", "type:marr place:leeds", sample)
	if len(evs) != 2 || evs[0].Type != "MARR" || len(evs[0].People) != 2 || evs[0].People[0].ID != "I1" {
		t.Errorf("events %+v", evs)
	}

	var places []jsonPlace
	runJSON(t, &places, "-places", sample)
	var names []string
	for _, p := range places {
		names = append(names, p.Name)
	}
	if strings.Join(names, ",") != "Canada,England,France,Preußen" {
		t.Fatalf("places %v", names)
	}
	england := places[1]
	if england.Events != 19 || england.People != 11 || len(england.Places) != 2 {
		t.Errorf("England %+v", england)
	}
	var leeds *jsonPlace
	for i, p := range england.Places[1].Places {
		if p.Name == "Leeds" {
			leeds = &england.Places[1].Places[i]
		}
	}
	if leeds == nil || leeds.Full != "Leeds, Yorkshire, England" || leeds.Latitude == nil || *leeds.Latitude < 53 || len(leeds.Places) != 2 {
		t.Errorf("Leeds %+v", leeds)
	}
}

func TestJSONStatsAndDiff(t *testing.T) {
	var s jsonStats
	runJSON(t, &s, "-stats", sample)
	if s.File.Name != "family.ged" || s.File.Encoding != "UTF-8" || s.Individuals != 24 || s.Families != 9 || s.Generations != 5 ||
		s.LongestLived == nil || s.LongestLived.ID != "I4" || s.Surnames[0] != (jsonCount{"Smith", 10}) || s.Warnings == nil ||
		s.Collapse.AncestorPositions != 66 || s.Collapse.Percent != 0 {
		t.Errorf("stats %+v", s)
	}
	if fs := s.Distributions["family_sizes"]; len(fs) != 3 || fs[0] != (jsonValueCount{1, 5}) || len(s.Distributions) != 4 {
		t.Errorf("distributions %+v", s.Distributions)
	}

	data, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(t.TempDir(), "old.ged")
	if err := os.WriteFile(old, []byte(strings.Replace(string(data), "1 NAME John /Smith/", "1 NAME Jon /Smith/", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	var d jsonDiff
	runJSON(t, &d, "-diff", old, sample)
	if d.Summary != "people: 1 changed" || len(d.People) != 1 || d.Header == nil || d.Families == nil {
		t.Fatalf("diff %+v", d)
	}
	p := d.People[0]
	if p.Kind != "changed" || p.ID != "I3" || p.Type != "INDI" || p.Noun != "person" || len(p.Details) != 1 ||
		p.Details[0] != (jsonDiffDetail{"changed", "name", "Jon Smith", "John Smith", "name: Jon Smith → John Smith"}) {
		t.Errorf("person %+v", p)
	}
}

func TestChecksFlag(t *testing.T) {
	defer func(f func() time.Time) { now = f }(now)
	now = func() time.Time { return time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC) }

	r := runCLI(t, "", "-checks", "all", "-person", "I20", sample)
	for _, want := range []string{
		"Checks · 0 findings in 0 people\n  Born after death: 0\n",
		"Hints · 77 findings in 24 people\n  Unknown parents within 4 generations of the marked person: 3\n" +
			"      known: parents 2 of 2 · grandparents 2 of 4 · great-grandparents 2 of 8 · 2nd great-grandparents 2 of 16\n" +
			"    I19    Lucy King (1868–): parents unknown · the marked person's mother\n",
		"    I20    Harold Smith (1891–): birth 1891, at least 134 years ago\n",
		"  Places that may be the same: similar names: 0\n",
	} {
		if r.code != 0 || !strings.Contains(r.stdout, want) {
			t.Errorf("exit %d, output lacks %q:\n%s", r.code, want, r.stdout)
		}
	}
	r = runCLI(t, "", "-checks", "hints", sample)
	if !strings.HasPrefix(r.stdout, "Hints · 74 findings") || !strings.Contains(r.stdout, "nobody is marked") {
		t.Errorf("hints only:\n%s", r.stdout)
	}
	r = runCLI(t, "", "-checks", "checks", sample)
	if strings.Contains(r.stdout, "Hints") || strings.Count(r.stdout, ": 0\n") != 14 {
		t.Errorf("checks only:\n%s", r.stdout)
	}

	var j jsonChecks
	runJSON(t, &j, "-checks", "old-age=100, similar-places=somewhat resembling,no-death", sample)
	if j.Date != "2026-10-14" || j.Marked != nil || len(j.Checks) != 3 {
		t.Fatalf("checks %+v", j)
	}
	if c := j.Checks[0]; c.ID != "old-age" || c.Title != "Lived longer than 100 years" || c.Threshold != float64(100) || c.Unit != "years" || c.Hint {
		t.Errorf("old-age %+v", c)
	}
	if c := j.Checks[1]; c.Threshold != "somewhat resembling" || len(c.Findings) != 1 || c.Findings[0].Person != nil ||
		strings.Join(c.Findings[0].Places, " | ") != "St Peter's, Leeds, Yorkshire, England | St Peter's Churchyard, Leeds, Yorkshire, England" {
		t.Errorf("places %+v", c)
	}
	if c := j.Checks[2]; c.Threshold != float64(110) || len(c.Findings) != 14 || c.Findings[0].Message != "birth 1844, at least 181 years ago" {
		t.Errorf("no-death %+v", c)
	}
	runJSON(t, &j, "-checks", "all", sample)
	if len(j.Checks) != len(genealogy.Checks) {
		t.Errorf("%d checks of %d", len(j.Checks), len(genealogy.Checks))
	}

	for _, tt := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"-checks", "bogus"}, 2, `unknown check "bogus" (all, checks, hints or born-after-death,`},
		{[]string{"-checks", ","}, 2, "no check chosen"},
		{[]string{"-checks", "old-age=5"}, 2, "old-age: expects a number from 80 to 150 (years)"},
		{[]string{"-checks", "old-age=x"}, 2, "old-age: expects a number"},
		{[]string{"-checks", "no-events=3"}, 2, "no-events has no threshold"},
		{[]string{"-checks", "similar-places=very"}, 2, "similar-places: expects almost-identical, similar, somewhat-resembling"},
		{[]string{"-checks", "all", "-person", "I99"}, 1, `no individual with ID "I99"`},
		{[]string{"-json"}, 2, "-json goes with -q, -tree, -desc, -hourglass, -lifespans, -timeline, -relate, -events, -places, -stats, -checks, -diff"},
		{[]string{"-json", "-export", "gexf", "-o", "-"}, 2, "-json goes with"},
		{[]string{"-json", "-q", "foo:bar"}, 2, `unknown field "foo"`},
		{[]string{"-json", "-events", "sex:m"}, 2, "not available for events"},
	} {
		r := runCLI(t, "", append(tt.args, sample)...)
		if r.code != tt.code || !strings.Contains(r.stderr, tt.want) {
			t.Errorf("%v: exit %d, stderr %q, want %d %q", tt.args, r.code, r.stderr, tt.code, tt.want)
		}
	}
}
