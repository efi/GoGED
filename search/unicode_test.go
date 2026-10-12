package search

import (
	"strings"
	"testing"

	"github.com/efi/goged/gedcom"
)

// The tests in this file cover text that looks the same to a reader but is
// written with different code points: precomposed letters (NFC) and letters
// followed by combining marks (NFD, as written by some programs on macOS),
// marks in a different order, compatibility characters, and scripts whose
// marks are optional. Escapes keep these invisible differences visible.

// parseLines parses a GEDCOM file given as lines.
func parseLines(t *testing.T, lines ...string) *gedcom.Document {
	t.Helper()
	doc, err := gedcom.ParseString(strings.Join(lines, "\n") + "\n")
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestFoldEquivalentSpellings(t *testing.T) {
	tests := []struct {
		name  string
		forms []string // spellings that must all fold to want
		want  string
	}{
		{"precomposed and decomposed", []string{"M\u00FCller", "Mu\u0308ller", "MU\u0308LLER", "M\u00DCLLER"}, "muller"},
		{"two marks on one letter", []string{"Nguy\u1EC5n", "Nguye\u0302\u0303n", "Nguy\u00EA\u0303n"}, "nguyen"},
		{"marks of the same class in either order", []string{"Nguye\u0303\u0302n"}, "nguyen"},
		{"marks of different classes in either order", []string{"M\u1EADn", "Ma\u0323\u0302n", "Ma\u0302\u0323n"}, "man"},
		{"a mark without a precomposed letter", []string{"Q\u0307uinn"}, "quinn"},
		{"a mark on a letter that does not decompose", []string{"\u00D8rsted", "O\u0338rsted"}, "orsted"},
		{"the Kelvin sign", []string{"\u212Aarl"}, "karl"},
		{"the Angstrom sign", []string{"\u212Bngstr\u00F6m", "\u00C5ngstr\u00F6m", "A\u030Angstro\u0308m"}, "angstrom"},
		{"Greek tonos and oxia", []string{"\u0395\u03BB\u03AD\u03BD\u03B7", "\u0395\u03BB\u1F73\u03BD\u03B7", "\u0395\u03BB\u03B5\u0301\u03BD\u03B7"}, "\u03B5\u03BB\u03B5\u03BD\u03B7"},
		{"a composition exclusion", []string{"\u0958", "\u0915\u093C"}, "\u0915"},
		{"Hangul syllables and jamo", []string{"\uAE40", "\u1100\u1175\u11B7"}, "\u1100\u1175\u11B7"},
		{"a ligature", []string{"\uFB01scher", "Fischer"}, "fischer"},
		{"fullwidth letters", []string{"\uFF2B\uFF41\uFF52\uFF4C", "Karl"}, "karl"},
		{"Hebrew points", []string{"\u05E9\u05B8\u05C2\u05E8\u05B8\u05D4", "\u05E9\u05E8\u05D4"}, "\u05E9\u05E8\u05D4"},
		{"Arabic vowel marks", []string{"\u0645\u064F\u062D\u064E\u0645\u064E\u0651\u062F", "\u0645\u062D\u0645\u062F"}, "\u0645\u062D\u0645\u062F"},
		{"a grapheme joiner and a variation selector", []string{"A\u034Fnna", "Anna\uFE0F"}, "anna"},
	}
	for _, tt := range tests {
		for _, form := range tt.forms {
			if got := Fold(form); got != tt.want {
				t.Errorf("%s: Fold(%+q) = %+q, want %+q", tt.name, form, got, tt.want)
			}
		}
	}
}

// equivalentsDoc has the same name and place written precomposed (NFC) and
// decomposed (NFD), and a married name in NFD.
func equivalentsDoc(t *testing.T) *gedcom.Document {
	return parseLines(t,
		"0 HEAD",
		"1 CHAR UTF-8",
		"0 @NFC@ INDI",
		"1 NAME J\u00FCrgen /M\u00FCller/",
		"1 BIRT",
		"2 DATE 1900",
		"2 PLAC M\u00FCnchen",
		"1 NOTE Lebte in M\u00FCnchen",
		"0 @NFD@ INDI",
		"1 NAME Ju\u0308rgen /Mu\u0308ller/",
		"1 BIRT",
		"2 DATE 1901",
		"2 PLAC Mu\u0308nchen",
		"1 NOTE Lebte in Mu",
		"2 CONC \u0308nchen", // the line break falls between letter and mark
		"0 @ALT@ INDI",
		"1 NAME Anna /Schmidt/",
		"1 NAME Anna /Mu\u0308ller/",
		"2 TYPE married",
		"0 TRLR",
	)
}

func TestSearchEquivalentSpellings(t *testing.T) {
	doc := equivalentsDoc(t)
	ix := NewIndex(doc)
	for _, tt := range []struct {
		q, want string
	}{
		{"m\u00FCller", "ALT,NFC,NFD"},
		{"mu\u0308ller", "ALT,NFC,NFD"},
		{"MU\u0308LLER", "ALT,NFC,NFD"},
		{"muller", "ALT,NFC,NFD"},
		{"name:mu\u0308ller", "ALT,NFC,NFD"},
		{"surname:m\u00FCller", "ALT,NFC,NFD"},
		{"surname:mu\u0308ller", "ALT,NFC,NFD"},
		{"~mu\u0308ller", "ALT,NFC,NFD"},
		{"~m\u00FCller", "ALT,NFC,NFD"},
		{"given:ju\u0308rgen", "NFC,NFD"},
		{"j\u00FCrg", "NFC,NFD"},
		{"ju\u0308", "NFC,NFD"}, // a prefix that ends with the mark
		{"ju", "NFC,NFD"},       // a prefix that ends before it
		{"\"j\u00FCrgen mu\u0308ller\"", "NFC,NFD"},
		{"place:mu\u0308nchen", "NFC,NFD"},
		{"born:m\u00FCnchen", "NFC,NFD"},
		{"note:m\u00FCnchen", "NFC,NFD"},
		{"any:mu\u0308nchen", "NFC,NFD"},
		{"tag:BIRT.PLAC=m\u00FCnchen", "NFC,NFD"},
		{"-mu\u0308ller", ""},
	} {
		rs, err := ix.SearchString(tt.q)
		if err != nil {
			t.Errorf("%+q: %v", tt.q, err)
			continue
		}
		if got := sortedIDs(rs); got != tt.want {
			t.Errorf("%+q = %s, want %s", tt.q, got, tt.want)
		}
	}

	// The married name matched, in either form.
	for _, q := range []string{"m\u00FCller given:anna", "mu\u0308ller given:anna"} {
		if rs, _ := ix.SearchString(q); len(rs) != 1 || rs[0].Individual.ID != "ALT" || rs[0].NameIndex != 1 {
			t.Errorf("%+q = %+v", q, rs)
		}
	}
	// Both forms of a name are equally good matches.
	rs, _ := ix.SearchString("m\u00FCller j\u00FCrgen")
	if len(rs) != 2 || rs[0].Score != rs[1].Score {
		t.Errorf("scores = %+v", rs)
	}
}

func TestSortEquivalentSpellings(t *testing.T) {
	doc := parseLines(t,
		"0 HEAD",
		"0 @Z@ INDI",
		"1 NAME Carl /Zander/",
		"0 @B@ INDI",
		"1 NAME Bernd /A\u0308rger/",
		"0 @A@ INDI",
		"1 NAME Anna /\u00C4rger/",
		"0 @D@ INDI",
		"1 NAME Dora /Adler/",
		"0 TRLR",
	)
	// Ärger sorts as Arger, after Adler, whichever way the Ä is written;
	// within it, the given names decide.
	rs, _ := NewIndex(doc).SearchString("")
	if got := resultIDs(rs); got != "D,A,B,Z" {
		t.Errorf("order = %s, want D,A,B,Z", got)
	}
}

func TestSearchScriptsWithMarks(t *testing.T) {
	doc := parseLines(t,
		"0 HEAD",
		"1 CHAR UTF-8",
		"0 @VI@ INDI",
		"1 NAME V\u0103n /Nguye\u0302\u0303n/",
		"0 @VI2@ INDI",
		"1 NAME Th\u1ECB /Nguy\u1EC5n/",
		"0 @KO@ INDI",
		"1 NAME \uBBFC\uC218 /\uAE40/",
		"0 @KOD@ INDI",
		"1 NAME \u1106\u1175\u11AB\u1109\u116E /\u1100\u1175\u11B7/",
		"0 @HE@ INDI",
		"1 NAME \u05E9\u05B8\u05C2\u05E8\u05B8\u05D4 /Cohen/",
		"0 @AR@ INDI",
		"1 NAME \u0645\u064F\u062D\u064E\u0645\u064E\u0651\u062F /Ali/",
		"0 @LIG@ INDI",
		"1 NAME Fritz /\uFB01scher/",
		"0 @FW@ INDI",
		"1 NAME \uFF2B\uFF41\uFF52\uFF4C /Zeh/",
		"0 @KEL@ INDI",
		"1 NAME \u212Aarl /Kelvin/",
		"0 TRLR",
	)
	ix := NewIndex(doc)
	for _, tt := range []struct {
		q, want string
	}{
		{"nguyen", "VI,VI2"},
		{"nguy\u1EC5n", "VI,VI2"},
		{"nguye\u0302\u0303n", "VI,VI2"},
		{"nguye\u0303\u0302n", "VI,VI2"},
		{"\uAE40", "KO,KOD"},
		{"\u1100\u1175\u11B7", "KO,KOD"},
		{"\uBBFC\uC218", "KO,KOD"},
		{"\uAE30", "KO,KOD"}, // a syllable typed before its final consonant
		{"\u05E9\u05E8\u05D4", "HE"},
		{"\u05E9\u05B8\u05C2\u05E8\u05B8\u05D4", "HE"},
		{"\u0645\u062D\u0645\u062F", "AR"},
		{"fischer", "LIG"},
		{"\uFB01scher", "LIG"},
		{"karl", "FW,KEL"},
		{"\uFF4B\uFF41\uFF52\uFF4C", "FW,KEL"},
		{"\u212Aarl", "FW,KEL"},
	} {
		rs, err := ix.SearchString(tt.q)
		if err != nil {
			t.Errorf("%+q: %v", tt.q, err)
			continue
		}
		if got := sortedIDs(rs); got != tt.want {
			t.Errorf("%+q = %s, want %s", tt.q, got, tt.want)
		}
	}
}

func TestEventFilterEquivalentSpellings(t *testing.T) {
	evs := NewEventIndex(equivalentsDoc(t).Events())
	for _, f := range []string{"place:m\u00FCnchen", "place:mu\u0308nchen", "name:mu\u0308ller", "name:j\u00FCrgen", "mu\u0308nchen"} {
		q, err := ParseEventQuery(f)
		if err != nil {
			t.Fatal(err)
		}
		if got := evs.Filter(q, false); len(got) != 2 {
			t.Errorf("%+q: %d events, want 2", f, len(got))
		}
	}
}

func TestSearchANSELDiacritics(t *testing.T) {
	// In ANSEL the diacritic (0xE8, diaeresis) comes before its letter.
	doc, err := gedcom.ParseBytes([]byte("0 HEAD\n1 CHAR ANSEL\n0 @A@ INDI\n1 NAME Hans /M\xe8uller/\n0 TRLR\n"))
	if err != nil {
		t.Fatal(err)
	}
	ix := NewIndex(doc)
	for _, q := range []string{"m\u00FCller", "mu\u0308ller", "muller"} {
		if rs, _ := ix.SearchString(q); sortedIDs(rs) != "A" {
			t.Errorf("%+q = %s", q, sortedIDs(rs))
		}
	}
}
