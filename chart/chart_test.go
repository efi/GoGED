package chart

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/efi/goged/gedcom"
	"github.com/mattn/go-runewidth"
)

func loadSample(t testing.TB) *gedcom.Document {
	t.Helper()
	doc, err := gedcom.ParseFile(filepath.Join("..", "testdata", "family.ged"))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// trimLines removes trailing spaces so golden strings stay readable.
func trimLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

func golden(s string) string { return strings.TrimPrefix(s, "\n") }

func TestPedigreeGolden(t *testing.T) {
	doc := loadSample(t)
	got := Pedigree(doc.Individual("I20"), Options{Generations: 5}).String()
	want := golden(`
                                                                                                           ┌─ William Smith (1790–1850)
                                                                                ┌─ John Smith (1817–1880) ─┤
                                                                                │                          └─ Elizabeth Brown (c.1795–1860)
                                                   ┌─ Thomas Smith (1842–1910) ─┤
                                                   │                            └─ Ann Taylor (1820–1845)
                      ┌─ Arthur Smith (1866–1940) ─┤
                      │                            └─ Jane Doe (1844–)
Harold Smith (1891–) ─┤
                      └─ Lucy King (1868–)
`)
	if trimLines(got) != want {
		t.Errorf("pedigree:\n%s\nwant:\n%s", got, want)
	}
}

func TestPedigreeASCIIAndLimit(t *testing.T) {
	doc := loadSample(t)
	c := Pedigree(doc.Individual("I14"), Options{Generations: 3, ASCII: true})
	want := golden(`
                                                       +- John Smith (1817–1880) >
                          +- Thomas Smith (1842–1910) -+
                          |                            +- Ann Taylor (1820–1845)
Arthur Smith (1866–1940) -+
                          +- Jane Doe (1844–)
`)
	if got := trimLines(c.String()); got != want {
		t.Errorf("pedigree:\n%s\nwant:\n%s", got, want)
	}
	john := c.Find(doc.Individual("I3"))
	if john < 0 || !c.Nodes[john].More {
		t.Error("John should be marked as having more ancestors")
	}
	if c.Find(doc.Individual("I1")) != -1 {
		t.Error("William is beyond the generation limit")
	}
}

func TestPedigreeOneParent(t *testing.T) {
	doc, _ := gedcom.ParseString(`0 HEAD
0 @I1@ INDI
1 NAME Kid /A/
1 FAMC @F1@
0 @I2@ INDI
1 NAME Mum /B/
1 FAMC @F2@
0 @I3@ INDI
1 NAME Granddad /B/
0 @F1@ FAM
1 WIFE @I2@
1 CHIL @I1@
0 @F2@ FAM
1 HUSB @I3@
1 CHIL @I2@
0 TRLR
`)
	got := trimLines(Pedigree(doc.Individual("I1"), Options{}).String())
	want := golden(`
Kid A ─┐
       │         ┌─ Granddad B
       └─ Mum B ─┘
`)
	if got != want {
		t.Errorf("pedigree with single parents:\n%s\nwant:\n%s", got, want)
	}
}

func TestPedigreeNodes(t *testing.T) {
	doc := loadSample(t)
	c := Pedigree(doc.Individual("I7"), Options{Generations: 3})
	if len(c.Nodes) != 5 || len(c.Lines) != 5 {
		t.Fatalf("nodes=%d lines=%d", len(c.Nodes), len(c.Lines))
	}
	root := c.Nodes[0]
	if root.Ind.ID != "I7" || root.Up != -1 || root.Gen != 0 || len(root.Down) != 2 {
		t.Errorf("root = %+v", root)
	}
	father, mother := c.Nodes[root.Down[0]], c.Nodes[root.Down[1]]
	if father.Ind.ID != "I3" || mother.Ind.ID != "I5" || father.Up != 0 || mother.Gen != 1 {
		t.Errorf("parents = %+v / %+v", father, mother)
	}
	if !(father.Line < root.Line && root.Line < mother.Line) {
		t.Errorf("fathers above, mothers below: %d %d %d", father.Line, root.Line, mother.Line)
	}
	for i, n := range c.Nodes {
		// The recorded position must match the rendered text.
		line := c.LineText(n.Line)
		col := 0
		for _, s := range c.Lines[n.Line] {
			if s.Node == i {
				break
			}
			col += runewidth.StringWidth(s.Text)
		}
		if col != n.Col {
			t.Errorf("node %d: Col=%d, rendered at %d in %q", i, n.Col, col, line)
		}
		if !strings.Contains(line, n.Ind.DisplayName()) {
			t.Errorf("node %d not on line %d: %q", i, n.Line, line)
		}
		if c.NodeAt(n.Line) != i {
			t.Errorf("NodeAt(%d) = %d, want %d", n.Line, c.NodeAt(n.Line), i)
		}
		if w := runewidth.StringWidth(line); w > c.Width {
			t.Errorf("line wider than chart: %d > %d", w, c.Width)
		}
	}
}

func TestDescendantsGolden(t *testing.T) {
	doc := loadSample(t)
	got := Descendants(doc.Individual("I1"), Options{Generations: 5}).String()
	want := golden(`
William Smith (1790–1850)
└── ⚭ Elizabeth Brown (c.1795–1860)  m. 1815
    ├── John Smith (1817–1880)
    │   ├── ⚭ Ann Taylor (1820–1845)  m. 1840
    │   │   └── Thomas Smith (1842–1910)
    │   │       └── ⚭ Jane Doe (1844–)  m. 1865
    │   │           └── Arthur Smith (1866–1940)
    │   │               └── ⚭ Lucy King (1868–)  m. 1890
    │   │                   ├── Harold Smith (1891–)
    │   │                   ├── Rose Smith (1893–)  (adopted)
    │   │                   └── Infant Smith (1895–1895)
    │   └── ⚭ Sarah White (1825–1890)  m. 1847
    │       ├── Emma Smith (1848–)
    │       │   └── ⚭ Friedrich Müller (1845–)  m. 1870
    │       │       └── Edith Miller (1872–)
    │       └── George Smith (1850–1851)
    └── Mary Smith (1819–1899)
        └── ⚭ Robert Jones (1815–)  m. 1838
            ├── Henry Jones (1840–)
            │   └── ⚭ Clara Green (1841–)  m. 1862
            │       └── Walter Jones (1863–)
            │           └── ⚭ Grace Hill (1865–)  m. 1888
            │               └── Dorothy Jones (1889–)
            └── Alice Jones (1842–)
`)
	if got != want {
		t.Errorf("descendants:\n%s\nwant:\n%s", got, want)
	}
}

func TestDescendantsASCIIAndLimit(t *testing.T) {
	doc := loadSample(t)
	c := Descendants(doc.Individual("I3"), Options{Generations: 2, ASCII: true})
	want := golden(`
John Smith (1817–1880)
|-- = Ann Taylor (1820–1845)  m. 1840
|   ` + "`" + `-- Thomas Smith (1842–1910) +
` + "`" + `-- = Sarah White (1825–1890)  m. 1847
    |-- Emma Smith (1848–) +
    ` + "`" + `-- George Smith (1850–1851)
`)
	if got := c.String(); got != want {
		t.Errorf("descendants:\n%s\nwant:\n%s", got, want)
	}
	thomas := c.Nodes[c.Find(doc.Individual("I7"))]
	george := c.Nodes[c.Find(doc.Individual("I9"))]
	if !thomas.More || george.More {
		t.Error("More markers")
	}
}

func TestPedigreeNavigation(t *testing.T) {
	doc, err := gedcom.ParseString(`0 HEAD
0 @C@ INDI
1 NAME Child /A/
0 @F@ INDI
1 NAME Father /A/
0 @M@ INDI
1 NAME Mother /B/
0 @FF@ INDI
1 NAME Grandfather /A/
0 @FM@ INDI
1 NAME Grandmother /C/
0 @MF@ INDI
1 NAME Grandfather /B/
0 @MM@ INDI
1 NAME Grandmother /D/
0 @F1@ FAM
1 HUSB @F@
1 WIFE @M@
1 CHIL @C@
0 @F2@ FAM
1 HUSB @FF@
1 WIFE @FM@
1 CHIL @F@
0 @F3@ FAM
1 HUSB @MF@
1 WIFE @MM@
1 CHIL @M@
0 TRLR
`)
	if err != nil {
		t.Fatal(err)
	}
	c := Pedigree(doc.Individual("C"), Options{Generations: 3})
	// The lines are FF, F, FM, C, MF, M, MM: every other line belongs to
	// another generation, but up and down stay within one.
	for _, tt := range []struct {
		from string
		dir  int
		want string
	}{
		{"F", 1, "M"}, // not the grandmother on the next line
		{"M", -1, "F"},
		{"M", 1, "M"}, // the end of the column
		{"F", -1, "F"},
		{"FF", 1, "FM"},
		{"FM", 1, "MF"}, // past the root's line
		{"MF", -1, "FM"},
		{"MM", 1, "MM"},
		{"C", 1, "C"}, // the root is alone in its generation
		{"C", -1, "C"},
	} {
		got := c.Nodes[c.Next(c.Find(doc.Individual(tt.from)), tt.dir)].Ind.ID
		if got != tt.want {
			t.Errorf("Next(%s, %d) = %s, want %s", tt.from, tt.dir, got, tt.want)
		}
	}
}

func TestDescendantsLinks(t *testing.T) {
	doc := loadSample(t)
	c := Descendants(doc.Individual("I3"), Options{Generations: 3})
	root := c.Nodes[0]
	if root.Ind.ID != "I3" || len(root.Down) != 2 {
		t.Fatalf("root = %+v", root)
	}
	ann := c.Nodes[root.Down[0]]
	if ann.Kind != KindSpouse || ann.Ind.ID != "I5" || ann.Up != 0 {
		t.Errorf("spouse node = %+v", ann)
	}
	thomas := c.Nodes[ann.Down[0]]
	if thomas.Ind.ID != "I7" || thomas.Kind != KindPerson || thomas.Gen != 1 || c.Nodes[thomas.Up].Ind.ID != "I5" {
		t.Errorf("child node = %+v", thomas)
	}
	// Navigation by line.
	if n := c.Next(0, 1); c.Nodes[n].Ind.ID != "I5" {
		t.Errorf("Next down from root = %v", c.Nodes[n].Ind.ID)
	}
	if n := c.Next(0, -1); n != 0 {
		t.Error("Next up from the first line stays put")
	}
	last := len(c.Nodes) - 1
	if c.Next(last, 1) != last || c.Next(-1, 1) != -1 {
		t.Error("Next at the end")
	}
	if c.Find(nil) != -1 || c.NodeAt(999) != -1 {
		t.Error("Find/NodeAt misses")
	}
}

func TestDescendantsUnknownSpouse(t *testing.T) {
	doc, _ := gedcom.ParseString("0 HEAD\n0 @I1@ INDI\n1 NAME Dad /X/\n1 FAMS @F1@\n0 @I2@ INDI\n1 NAME Kid /X/\n1 FAMC @F1@\n0 @F1@ FAM\n1 HUSB @I1@\n1 CHIL @I2@\n1 MARR\n2 DATE ABT 1900\n0 TRLR\n")
	c := Descendants(doc.Individual("I1"), Options{})
	want := "Dad X\n└── ⚭ (unknown)  m. c.1900\n    └── Kid X\n"
	if got := c.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	kid := c.Nodes[1]
	if kid.Ind.ID != "I2" || kid.Up != 0 {
		t.Errorf("kid attaches to the parent when the spouse is unknown: %+v", kid)
	}
}

func TestTruncationAndWideCharacters(t *testing.T) {
	doc, _ := gedcom.ParseString("0 HEAD\n0 @I1@ INDI\n1 NAME 山田 /太郎/\n1 FAMC @F1@\n0 @I2@ INDI\n1 NAME Bartholomew Maximilian Fitzgerald /Worthington-Smythe/\n1 FAMS @F1@\n0 @F1@ FAM\n1 HUSB @I2@\n1 CHIL @I1@\n0 TRLR\n")
	c := Pedigree(doc.Individual("I1"), Options{MaxLabel: 20})
	father := c.Nodes[c.Nodes[0].Down[0]]
	if father.Width > 20 {
		t.Errorf("label not truncated: width %d", father.Width)
	}
	if !strings.Contains(c.String(), "…") {
		t.Errorf("truncation marker missing:\n%s", c.String())
	}
	// The root label "山田 太郎" is 9 columns wide; the connector column
	// must account for the double-width characters.
	root := c.Nodes[0]
	if root.Width != 9 {
		t.Errorf("root width = %d", root.Width)
	}
	if father.Col != root.Width+2+3 {
		t.Errorf("father column = %d", father.Col)
	}
	if got := truncate("abc", 5); got != "abc" {
		t.Errorf("truncate = %q", got)
	}
}

func TestTruncationKeepsLifeDates(t *testing.T) {
	doc, _ := gedcom.ParseString(`0 HEAD
0 @I1@ INDI
1 NAME Bartholomew Maximilian Fitzgerald /Worthington-Smythe/
1 BIRT
2 DATE 1850
0 @I2@ INDI
1 NAME Bartholomew Maximilian Fitzgerald /Worthington-Smythe/
0 TRLR
`)
	tests := []struct {
		id    string
		width int
		want  string
	}{
		{"I1", 30, "Bartholomew Maximilia… (1850–)"},
		{"I1", 100, "Bartholomew Maximilian Fitzgerald Worthington-Smythe (1850–)"},
		{"I1", 12, "Bartholomew…"}, // too narrow to keep the dates
		{"I2", 20, "Bartholomew Maximil…"},
	}
	for _, tt := range tests {
		got := fitLabel(doc.Individual(tt.id), tt.width)
		if got != tt.want || runewidth.StringWidth(got) > tt.width {
			t.Errorf("fitLabel(%s, %d) = %q, want %q", tt.id, tt.width, got, tt.want)
		}
	}
	if fitLabel(nil, 10) != "?" {
		t.Error("fitLabel(nil)")
	}
}

func TestEmptyCharts(t *testing.T) {
	if c := Pedigree(nil, Options{}); len(c.Nodes) != 0 || c.String() != "" {
		t.Error("nil pedigree")
	}
	if c := Descendants(nil, Options{}); len(c.Nodes) != 0 {
		t.Error("nil descendants")
	}
	if Label(nil) != "?" {
		t.Error("Label(nil)")
	}
	doc := loadSample(t)
	c := Pedigree(doc.Individual("I13"), Options{})
	if c.String() != "Jane Doe (1844–)\n" {
		t.Errorf("single person pedigree = %q", c.String())
	}
}

func TestOptionsDefaults(t *testing.T) {
	o := Options{}.normalized()
	if o.Generations != 4 || o.MaxLabel != 40 {
		t.Errorf("defaults = %+v", o)
	}
	o = Options{Generations: 2, MaxLabel: 3}.normalized()
	if o.Generations != 2 || o.MaxLabel != 40 {
		t.Errorf("normalized = %+v", o)
	}
}

func TestLoopsTerminate(t *testing.T) {
	doc, _ := gedcom.ParseString("0 HEAD\n0 @I1@ INDI\n1 FAMC @F1@\n1 FAMS @F1@\n0 @F1@ FAM\n1 HUSB @I1@\n1 CHIL @I1@\n0 TRLR\n")
	if c := Pedigree(doc.Individual("I1"), Options{Generations: 6}); len(c.Nodes) != 6 {
		t.Errorf("pedigree of a self-loop has %d nodes", len(c.Nodes))
	}
	// The person's second appearance (as their own child) is not expanded.
	if c := Descendants(doc.Individual("I1"), Options{Generations: 6}); len(c.Nodes) != 2 || !strings.HasSuffix(c.LineText(2), "(see above)") {
		t.Errorf("descendants of a self-loop:\n%s", c)
	}
}

func TestDescendantsAdoptionAndRepeats(t *testing.T) {
	doc, err := gedcom.ParseFile(filepath.Join("..", "testdata", "Muster_GEDCOM_UTF-8.ged"))
	if err != nil {
		t.Fatal(err)
	}
	// Gerold adopted Markus in both of his marriages.
	want := `Gerold Freiwein (1938–)
├── ⚭ Mathilde Mustermann (1939–1970)  m. 1966
│   └── Markus Schüchter (1963–)  (adopted)
└── ⚭ Brigitte Marquardt (1942–)  m. 1972
    └── Markus Schüchter (1963–)  (adopted)  (see above)
`
	c := Descendants(doc.Individual("I19"), Options{})
	if got := c.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if n := c.Nodes[c.NodeAt(4)]; n.Ind.ID != "I6" || n.More || len(n.Down) != 0 {
		t.Errorf("repeated node = %+v", n)
	}
}
