package genealogy

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/efi/goged/gedcom"
)

// hintsGED has a gap or a doubtful place name for every hint. ME is the
// marked person: the father FA has only a mother, GM; the mother MO has no
// parents.
const hintsGED = `0 HEAD
0 @S1@ SOUR
1 TITL Register
0 @ME@ INDI
1 NAME Me /Root/
1 SEX M
1 BIRT
2 DATE 1950
2 PLAC Köln, Preußen
2 SOUR @S1@
1 FAMC @FP@
0 @FA@ INDI
1 NAME Fred /Root/
1 SEX M
1 BIRT
2 DATE 1920
2 PLAC Koeln, Preußen
1 FAMC @FG@
0 @MO@ INDI
1 NAME Mia /Other/
1 SEX F
1 DEAT
2 DATE 1990
0 @GM@ INDI
1 NAME Gerda /Root/
1 SEX F
1 BIRT
2 DATE 1890
2 PLAC Cöln, Preußen
0 @NOB@ INDI
1 NAME No /Body/
0 @D1@ INDI
1 NAME Dan /Div/
1 SEX M
1 BIRT
2 DATE 1960
2 PLAC Yorks, England
0 @D2@ INDI
1 NAME Dora /Div/
1 SEX F
1 SOUR @S1@
1 BIRT
2 DATE 1962
2 PLAC Yorkshire, England
0 @P1@ INDI
1 NAME Paul /Penn/
1 BIRT
2 DATE ABT 1700
2 PLAC Philadelphia, Pennsylvania
0 @P2@ INDI
1 NAME Pia /Penn/
1 BIRT
2 DATE 1705
2 PLAC Filadelfia, Pennsylvania
1 DEAT
2 DATE 1780
0 @W1@ INDI
1 NAME Will /Wendel/
1 BIRT
2 DATE 1800
2 PLAC St. Wendel, Saarland
1 DEAT
2 DATE 1850
0 @W2@ INDI
1 NAME Wanda /Wendel/
1 CHR
2 DATE 1801
2 PLAC Sankt Wendel, Saarland
1 BURI
2 DATE 1860
0 @X@ INDI
1 NAME Xaver /Undated/
1 BIRT
0 @FP@ FAM
1 HUSB @FA@
1 WIFE @MO@
1 MARR Y
1 CHIL @ME@
0 @FG@ FAM
1 WIFE @GM@
1 CHIL @FA@
0 @FD@ FAM
1 HUSB @D1@
1 WIFE @D2@
1 DIV
2 DATE 1990
0 TRLR
`

func TestHints(t *testing.T) {
	doc, err := gedcom.ParseString(hintsGED)
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{Marked: doc.Individual("ME"), Today: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	want := map[string]string{
		"missing-parents": "FA: father unknown · the marked person's father\n" +
			"MO: parents unknown · the marked person's mother\n" +
			"GM: parents unknown · the marked person's grandmother",
		"no-birth-date": "NOB: no birth or baptism recorded\nMO: no birth or baptism recorded\nX: birth or baptism recorded without a date",
		"no-birth-place": "NOB: no place of birth or baptism recorded\nMO: no place of birth or baptism recorded\n" +
			"X: no place of birth or baptism recorded",
		"no-death":         "P1: birth about 1700, at least 323 years ago\nGM: birth 1890, at least 135 years ago",
		"no-events":        "NOB: nothing recorded but the name and the family links",
		"no-marriage-date": "D1>D2: divorce recorded, but no marriage, with Dora Div\nFA>MO: married, without a date, with Mia Other",
		// The birth of ME cites a source, and D2's record does.
		"unsourced": "D1: birth 1960\nMO: death 1990\nP1: birth about 1700\nP2: birth 1705\nP2: death 1780\nFA: birth 1920\n" +
			"FA>MO: marriage with Mia Other\nGM: birth 1890\nX: birth\nW2: christening 1801\nW2: burial 1860\nW1: birth 1800\nW1: death 1850",
		"similar-places": "-: Cöln · Koeln · Köln (in Preußen)\n-: Sankt Wendel · St. Wendel (in Saarland)\n-: Yorks · Yorkshire (in England)",
	}
	hints := 0
	for _, c := range Checks {
		if !c.Hint {
			continue
		}
		hints++
		if got := runWith(t, doc, c.ID, c.Param, ctx); got != want[c.ID] {
			t.Errorf("%s:\n%s\nwant:\n%s", c.ID, got, want[c.ID])
		}
	}
	if hints != len(want) {
		t.Errorf("%d hints, %d expectations", hints, len(want))
	}

	// The ancestry of the marked person, generation by generation.
	missing := Checks[slices.IndexFunc(Checks, func(c Check) bool { return c.ID == "missing-parents" })]
	if got := missing.Summary(doc, 4, ctx); got != "known: parents 2 of 2 · grandparents 1 of 4 · great-grandparents 0 of 8 · 2nd great-grandparents 0 of 16" {
		t.Errorf("summary = %q", got)
	}
	if got := missing.Summary(doc, 1, ctx); got != "known: parents 2 of 2" || runWith(t, doc, "missing-parents", 1, ctx) != "" {
		t.Errorf("one generation: %q", got)
	}
	if got := missing.Summary(doc, 4, Context{}); got != "nobody is marked: mark a person to see the gaps in their ancestry" ||
		runWith(t, doc, "missing-parents", 4, Context{}) != "" || !missing.UsesMarked() || Checks[0].UsesMarked() {
		t.Errorf("nobody marked: %q", got)
	}
	if got := runWith(t, doc, "missing-parents", 2, ctx); strings.Contains(got, "GM") {
		t.Errorf("two generations: %q", got)
	}

	// Thresholds and levels.
	if got := runWith(t, doc, "no-death", 135, ctx); got != "P1: birth about 1700, at least 323 years ago" {
		t.Errorf("no-death 135: %q", got)
	}
	for level, want := range []string{
		"-: Koeln · Köln (in Preußen)\n-: Sankt Wendel · St. Wendel (in Saarland)",
		"-: Cöln · Koeln · Köln (in Preußen)\n-: Sankt Wendel · St. Wendel (in Saarland)\n-: Yorks · Yorkshire (in England)",
		"-: Cöln · Koeln · Köln (in Preußen)\n-: Filadelfia · Philadelphia (in Pennsylvania)\n" +
			"-: Sankt Wendel · St. Wendel (in Saarland)\n-: Yorks · Yorkshire (in England)",
	} {
		if got := runWith(t, doc, "similar-places", level, ctx); got != want {
			t.Errorf("places, level %d:\n%s\nwant:\n%s", level, got, want)
		}
	}
	places := Checks[len(Checks)-1]
	if places.Name(0) != "Places that may be the same: almost identical names" || places.Value(2) != "somewhat resembling" {
		t.Errorf("names %q, %q", places.Name(0), places.Value(2))
	}
	f := places.Run(doc, 1, ctx)[0]
	if f.Person != nil || len(f.Places) != 3 || f.Places[0].Full != "Cöln, Preußen" || f.Places[2].Full != "Köln, Preußen" {
		t.Errorf("places of %q: %v", f.Message, f.Places)
	}
}

func TestPlaceNames(t *testing.T) {
	level := func(a, b string) int {
		for l := almostIdentical; l <= resembling; l++ {
			if alike(newPlaceName(a), newPlaceName(b), l) {
				return l
			}
		}
		return -1
	}
	for _, tt := range []struct {
		a, b string
		want int
	}{
		{"Köln", "KOELN", almostIdentical},
		{"Bad-Homburg", "Bad Homburg", almostIdentical},
		{"Straße", "Strasse", almostIdentical},
		{"Saint-Denis", "St Denis", almostIdentical},
		{"Mannheim", "Manheim", similar},
		{"Yorks", "Yorkshire", similar},
		{"Ber.", "Berlin", similar},
		{"Ft. Wayne", "Fort Wayne", resembling}, // a contraction, not an abbreviation
		{"Neu", "Neustadt", -1},                 // too short without a dot
		{"Neu.", "Neustadt", similar},
		{"St Peter's", "St Peter's Churchyard", resembling},
		{"Philadelphia", "Filadelfia", resembling},
		{"Berlin", "Bern", -1},
		{"Leeds", "York", -1},
		{"", "", -1},
	} {
		if got := level(tt.a, tt.b); got != tt.want {
			t.Errorf("%q and %q: level %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
	if distance("kitten", "sitting") != 3 || distance("", "abc") != 3 || distance("Köln", "Koln") != 1 {
		t.Error("distance")
	}
	// Groups form through chains of resemblance.
	var ps []*PlaceNode
	for _, n := range []string{"Aachen", "Cöln", "Köln", "Kölln", "Bonn"} {
		ps = append(ps, &PlaceNode{Name: n})
	}
	groups := groupSimilar(ps, similar)
	var names []string
	for _, g := range groups {
		for _, p := range g {
			names = append(names, p.Name)
		}
		names = append(names, "|")
	}
	if got := strings.Join(names, " "); got != "Cöln Kölln Köln |" {
		t.Errorf("groups = %s", got)
	}
	if groupSimilar(ps[:1], resembling) != nil {
		t.Error("a group of one")
	}
}
