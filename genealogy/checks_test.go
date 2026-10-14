package genealogy

import (
	"fmt"
	"strings"
	"testing"

	"github.com/efi/goged/gedcom"
)

// checksGED has one problem for each check, and cases that look like
// problems but are not: approximate dates that may be right, an adopted
// child, twins, namesake siblings, aliases and people of different sex.
const checksGED = `0 HEAD
0 @A@ INDI
1 NAME Anna /Early/
1 SEX F
1 BIRT
2 DATE 1 JAN 1900
1 DEAT
2 DATE 1 JAN 1890
0 @B@ INDI
1 NAME Bert /Twice/
1 SEX M
1 BIRT
2 DATE 1 JAN 1900
1 BIRT
2 DATE 2 JAN 1900
1 BIRT
2 DATE ABT 1900
0 @C@ INDI
1 NAME Carl /Before/
1 SEX M
1 BIRT
2 DATE 1900
1 OCCU Weaver
2 DATE 1890
1 RESI
2 DATE ABT 1899
1 RESI
2 DATE BEF 1905
1 FAMS @F1@
0 @CW@ INDI
1 NAME Clara /Wife/
1 SEX F
1 BIRT
2 DATE 1860
1 FAMS @F1@
0 @D@ INDI
1 NAME Dora /After/
1 SEX F
1 DEAT
2 DATE 1950
1 BURI
2 DATE 1951
1 PROB
2 DATE 1952
1 RESI
2 DATE 1960
0 @E@ INDI
1 NAME Emil /Old/
1 SEX M
1 BIRT
2 DATE 1800
1 DEAT
2 DATE 1915
0 @H@ INDI
1 NAME Hugo /Father/
1 SEX M
1 BIRT
2 DATE 1810
0 @M@ INDI
1 NAME Mia /Mother/
1 SEX F
1 BIRT
2 DATE 1 JAN 1890
0 @K@ INDI
1 NAME Kurt /Father/
1 SEX M
1 BIRT
2 DATE 1 JUN 1900
0 @ADA@ INDI
1 NAME Ada /Father/
1 SEX F
1 BIRT
2 DATE 1 JUN 1901
1 FAMC @F2@
2 PEDI adopted
0 @P@ INDI
1 NAME Paul /Late/
1 SEX M
1 BIRT
2 DATE 1870
1 DEAT
2 DATE 1 JAN 1902
0 @R@ INDI
1 NAME Rita /Late/
1 SEX F
1 BIRT
2 DATE 1875
1 DEAT
2 DATE 1 MAR 1901
0 @TOM@ INDI
1 NAME Tom /Late/
1 SEX M
1 BIRT
2 DATE 1 JUN 1900
0 @TIM@ INDI
1 NAME Tim /Late/
1 SEX M
1 BIRT
2 DATE 2 JUN 1900
0 @TIA@ INDI
1 NAME Tia /Late/
1 SEX F
1 BIRT
2 DATE 1 OCT 1900
0 @TED@ INDI
1 NAME Ted /Late/
1 SEX M
1 BIRT
2 DATE 1 DEC 1903
0 @MAX@ INDI
1 NAME Max /Young/
1 SEX M
1 BIRT
2 DATE 1 JAN 1900
0 @EVE@ INDI
1 NAME Eve /Young/
1 SEX F
1 BIRT
2 DATE 1 JAN 1890
0 @J1@ INDI
1 NAME Johann /Schmidt/
1 SEX M
1 BIRT
2 DATE 1850
0 @J2@ INDI
1 NAME Johan /Schmitt/
1 SEX M
1 BIRT
2 DATE 1851
0 @W1@ INDI
1 NAME Fritz /Wolf/
1 SEX M
1 BIRT
2 DATE 1840
0 @W2@ INDI
1 NAME Fritz /Wolf/
1 SEX M
1 BIRT
2 DATE 1842
0 @W3@ INDI
1 NAME Gustav /Wolf/
1 SEX M
1 BIRT
2 DATE 3 MAR 1845
0 @W4@ INDI
1 NAME Gustav /Wolf/
1 SEX M
1 BIRT
2 DATE 3 MAR 1845
0 @WM@ INDI
1 NAME Wanda /Wolf/
1 SEX F
0 @O1@ INDI
1 NAME Otto /Alias/
1 SEX M
1 BIRT
2 DATE 1860
1 ALIA @O2@
0 @O2@ INDI
1 NAME Oto /Alias/
1 SEX M
1 BIRT
2 DATE 1860
0 @L1@ INDI
1 NAME Kim /Lee/
1 SEX M
1 BIRT
2 DATE 1900
0 @L2@ INDI
1 NAME Kim /Lee/
1 SEX F
1 BIRT
2 DATE 1900
0 @F1@ FAM
1 HUSB @C@
1 WIFE @CW@
1 MARR
2 DATE 1880
0 @F2@ FAM
1 HUSB @H@
1 WIFE @M@
1 CHIL @K@
1 CHIL @ADA@
0 @F3@ FAM
1 HUSB @P@
1 WIFE @R@
1 CHIL @TOM@
1 CHIL @TIM@
1 CHIL @TIA@
1 CHIL @TED@
0 @F4@ FAM
1 HUSB @MAX@
1 WIFE @EVE@
1 MARR
2 DATE 1 JAN 1912
0 @F5@ FAM
1 WIFE @WM@
1 CHIL @W1@
1 CHIL @W2@
1 CHIL @W3@
1 CHIL @W4@
0 TRLR
`

// run returns the findings of a check as "ID>OTHER: message" lines; a
// finding without a person has "-" for the ID.
func run(t *testing.T, doc *gedcom.Document, id string, param int) string {
	t.Helper()
	return runWith(t, doc, id, param, Context{})
}

func runWith(t *testing.T, doc *gedcom.Document, id string, param int, ctx Context) string {
	t.Helper()
	for _, c := range Checks {
		if c.ID == id {
			var out []string
			for _, f := range c.Run(doc, param, ctx) {
				who := "-"
				if f.Person != nil {
					who = f.Person.ID
				}
				if f.Other != nil {
					who += ">" + f.Other.ID
				}
				out = append(out, who+": "+f.Message)
			}
			return strings.Join(out, "\n")
		}
	}
	t.Fatalf("no check %q", id)
	return ""
}

func TestChecks(t *testing.T) {
	doc, err := gedcom.ParseString(checksGED)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"born-after-death":        "A: birth 1 Jan 1900 after death 1 Jan 1890",
		"vital-twice":             "B: birth 1 Jan 1900 and birth 2 Jan 1900",
		"event-before-birth":      "C: occupation 1890 before birth 1900\nC>CW: marriage 1880 before birth 1900",
		"event-after-death":       "D: residence 1960 after death 1950",
		"old-age":                 "E: lived at least 114 years: birth 1800, death 1915",
		"mother-young":            "M>K: at most 10 years old at the birth of Kurt Father (1 Jun 1900)",
		"mother-old":              "",
		"father-young":            "",
		"father-old":              "H>K: at least 89 years old at the birth of Kurt Father (1 Jun 1900)",
		"born-after-mother-death": "TED>R: birth 1 Dec 1903, at least 1005 days after the mother's death 1 Mar 1901",
		"born-after-father-death": "TED>P: birth 1 Dec 1903, at least 699 days after the father's death 1 Jan 1902",
		"siblings-close":          "TIA>TIM: birth 1 Oct 1900, at most 121 days after Tim Late (2 Jun 1900)",
		"married-young":           "MAX>EVE: at most 12 years old at the marriage 1 Jan 1912",
		"duplicates": "J1>J2: may be the same as Johan Schmitt: names sound alike, birth 1850 and 1851\n" +
			"W3>W4: may be the same as Gustav Wolf: names sound alike, birth 3 Mar 1845 and 3 Mar 1845",
	}
	checks := 0
	for _, c := range Checks {
		if c.Hint {
			continue
		}
		checks++
		if got := run(t, doc, c.ID, c.Param); got != want[c.ID] {
			t.Errorf("%s:\n%s\nwant:\n%s", c.ID, got, want[c.ID])
		}
	}
	if len(want) != checks {
		t.Errorf("%d checks, %d expectations", checks, len(want))
	}

	// Thresholds decide.
	for _, tt := range []struct {
		id    string
		param int
		found bool
	}{
		{"old-age", 114, false},
		{"old-age", 113, true},
		{"mother-young", 10, false},
		{"mother-young", 11, true},
		{"father-old", 89, false},
		{"father-old", 88, true},
		{"born-after-father-death", 24, false},
		{"siblings-close", 3, false},
		{"siblings-close", 4, true},
		{"married-young", 12, false},
		{"duplicates", 0, true}, // only the Gustavs, born the same day
	} {
		got := run(t, doc, tt.id, tt.param)
		if (got != "") != tt.found {
			t.Errorf("%s with %d: %q", tt.id, tt.param, got)
		}
	}
	if got := run(t, doc, "duplicates", 0); strings.Contains(got, "J1") {
		t.Errorf("born a year apart: %q", got)
	}
}

func TestCheckNames(t *testing.T) {
	ids := map[string]bool{}
	hints := false
	for _, c := range Checks {
		name := c.Name(c.Param)
		if strings.Contains(name, "%") || name == "" || ids[c.ID] {
			t.Errorf("%s: name %q", c.ID, name)
		}
		ids[c.ID] = true
		// A threshold is a number with a unit, or a level.
		switch {
		case !c.HasParam():
		case c.Levels != nil:
			if c.Min != 0 || c.Max != len(c.Levels)-1 || c.Param > c.Max || !strings.Contains(c.Title, "%s") || c.Value(c.Max) != c.Levels[c.Max] {
				t.Errorf("%s: levels %d of %d..%d", c.ID, c.Param, c.Min, c.Max)
			}
		case c.Param < c.Min || c.Param > c.Max || c.Unit == "" || !strings.Contains(c.Title, "%d") || c.Value(c.Param) != fmt.Sprint(c.Param):
			t.Errorf("%s: threshold %d in %d..%d %q", c.ID, c.Param, c.Min, c.Max, c.Unit)
		}
		// The hints come after the checks.
		if hints && !c.Hint {
			t.Errorf("%s: a check among the hints", c.ID)
		}
		hints = c.Hint
	}
	if Checks[0].Name(7) != "Born after death" || fmt.Sprint(Checks[4].Name(99)) != "Lived longer than 99 years" {
		t.Errorf("names %q, %q", Checks[0].Name(7), Checks[4].Name(99))
	}
}
