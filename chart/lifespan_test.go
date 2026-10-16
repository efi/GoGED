package chart

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/efi/goged/gedcom"
	"github.com/mattn/go-runewidth"
)

func TestLifespansGolden(t *testing.T) {
	doc := loadSample(t)
	c := Lifespans(doc.Individual("I3"), LifespanOptions{})
	want := golden(`
                                1800      1820      1840      1860      1880      1900           lived  with John
                           ┬────┬────┬────┬────┬────┬────┬────┬────┬────┬────┬────┬────┬
father     William Smith   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━               ┊                    1790–1850  32 y
mother     Elizabeth Brown  ╍╍━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━          ┊                  c.1795–1860  43 y
                                        ┊                               ┊
           John Smith                   ╺━━━━━━━━━━━⚭━━⚭━━━━━━━━━━━━━━━━┊                    1817–1880  63 y
sister     Mary Smith                   ┊╺━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸           1819–1899  61 y
                                        ┊                               ┊
wife       Ann Taylor                   ┊ ━━━━━━━━━━⚭━╸                 ┊                    1820–1845  ⚭ 5 y
  son      Thomas Smith                 ┊            ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━      1842–1910  38 y
                                        ┊                               ┊
wife       Sarah White                  ┊   ╺━━━━━━━━━━⚭━━━━━━━━━━━━━━━━━━━━━                1825–1890  ⚭ 33 y
  daughter Emma Smith                   ┊               ━━━━━━━━━━━━━━━━━━━┄┄┄┄┄┄┄┄┄┄┄┄┄         1848–  32 y
  son      George Smith                 ┊                ╸              ┊                    1850–1851  1 y
John's age                              0    10   20   30   40   50   60

━ while John lived   ━ before or after   ╍ approximate   ┄ not recorded   ⚭ marriage
`)
	if got := c.String(); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if c.Head != 2 || c.Foot != 3 {
		t.Errorf("head %d, foot %d", c.Head, c.Foot)
	}

	// John is the first node, and every row is one.
	if c.Nodes[0].Ind.ID != "I3" || c.Nodes[0].Line != 5 || len(c.Nodes) != 9 || len(c.Lives) != 9 {
		t.Fatalf("nodes %+v", c.Nodes)
	}
	for i, n := range c.Nodes {
		if c.NodeAt(n.Line) != i {
			t.Errorf("node %d is not on line %d", i, n.Line)
		}
	}
	// Down moves row by row.
	if n := c.Next(c.Find(doc.Individual("I1")), 1); c.Nodes[n].Ind.ID != "I2" {
		t.Errorf("below William: %s", c.Nodes[n].Ind.ID)
	}

	// The bars are coloured while John lived, his own stands out, and
	// marriages are marked.
	classes := map[Class]bool{}
	for _, s := range c.Lines[c.Nodes[c.Find(doc.Individual("I1"))].Line] {
		classes[s.Class] = true
	}
	if !classes[ClassBar] || !classes[ClassDim] {
		t.Errorf("William's line: %+v", c.Lines[2])
	}
	marker, rootBar := false, false
	for _, s := range c.Lines[5] {
		marker = marker || (s.Class == ClassMarker && s.Text == "⚭")
		rootBar = rootBar || s.Class == ClassRootBar
	}
	if !marker || !rootBar {
		t.Errorf("John's line: %+v", c.Lines[5])
	}
}

func TestLifespansLives(t *testing.T) {
	doc := loadSample(t)
	life := func(c *Chart, id string) Life {
		t.Helper()
		for _, l := range c.Lives {
			if l.Ind.ID == id {
				return l
			}
		}
		t.Fatalf("%s is not in the chart", id)
		return Life{}
	}
	c := Lifespans(doc.Individual("I3"), LifespanOptions{})
	for id, want := range map[string]string{
		"I3": "63 y", "I1": "32 y", "I2": "43 y", "I4": "61 y", "I5": "⚭ 5 y", "I6": "⚭ 33 y", "I7": "38 y",
		// Emma was alive until her divorce in 1885 at least, after John's
		// death; George died the year after he was born.
		"I8": "32 y", "I9": "1 y",
	} {
		if got := life(c, id).Together.Text(false); got != want {
			t.Errorf("%s together: %s, want %s", id, got, want)
		}
	}
	if l := life(c, "I2"); l.Born != (Years{1793, 1797, true}) || l.Died != (Years{1860, 1860, true}) {
		t.Errorf("Elizabeth, born about 1795: %+v", l)
	}
	if l := life(c, "I8"); l.Died.OK || l.Records != (Years{1848, 1885, true}) {
		t.Errorf("Emma: %+v", l)
	}
	if l := life(c, "I3"); len(l.Marriages) != 2 || l.Marriages[0] != 1840 || l.Marriages[1] != 1847 {
		t.Errorf("John's marriages: %v", l.Marriages)
	}
	if l := life(c, "I5"); !l.Partner || len(l.Marriages) != 1 || l.Marriages[0] != 1840 || l.Gen != 0 {
		t.Errorf("Ann: %+v", l)
	}
	if l := life(c, "I7"); l.Gen != -1 || l.Depth != 1 || l.Relation != "son" {
		t.Errorf("Thomas: %+v", l)
	}

	// Two generations add the grandparents and Thomas's half-siblings,
	// Arthur's aunt and uncle.
	c = Lifespans(doc.Individual("I14"), LifespanOptions{Generations: 2})
	var rows []string
	for _, l := range c.Lives {
		rows = append(rows, l.Relation+":"+l.Ind.ID)
	}
	if got := strings.Join(rows, " "); got != "grandfather:I3 grandmother:I5 father:I7 half-aunt:I8 half-uncle:I9 mother:I13 "+
		":I14 wife:I19 son:I20 adoptive daughter:I21 child:I24" {
		t.Errorf("rows: %s", got)
	}
	if l := life(c, "I19"); l.Together.Text(false) != "⚭" || !l.Together.AtLeast {
		t.Errorf("Lucy, married in 1890, last recorded then: %+v", l.Together)
	}
	if l := life(c, "I8"); l.Together.Text(true) != ">= 19 y" {
		t.Errorf("Emma with Arthur: %s", l.Together.Text(true))
	}
	if !strings.Contains(c.String(), "  adoptive daughter Rose Smith") {
		t.Errorf("Rose is not indented:\n%s", c.String())
	}
	// Three generations add grandchildren, if there are any, and the
	// great-grandparents.
	c = Lifespans(doc.Individual("I20"), LifespanOptions{Generations: 3})
	if l := life(c, "I3"); l.Relation != "great-grandfather" || l.Gen != 3 {
		t.Errorf("John for Harold: %+v", l)
	}
}

func TestLifespansAbout(t *testing.T) {
	doc := loadSample(t)
	c := Lifespans(doc.Individual("I3"), LifespanOptions{})
	for id, want := range map[string]string{
		"I3": "John Smith · born 2 Feb 1817 in Leeds · died 1880 in Manchester at ~63",
		"I1": "William Smith · born 12 Mar 1790 in Leeds · died 4 Jan 1850 in Leeds at 59, when John Smith was 32",
		"I5": "Ann Taylor · born 1820 · married 1840 in Manchester · died 1845 in Manchester at ~25, when John Smith was ~28",
		"I8": "Emma Smith · born 1848 in Manchester · no death recorded, last record divorce 1885 at ~37",
	} {
		if got := c.About(c.Find(doc.Individual(id))); got != want {
			t.Errorf("about %s:\n got %s\nwant %s", id, got, want)
		}
	}
	if c.About(-1) != "" || c.About(len(c.Nodes)) != "" {
		t.Error("About out of range")
	}
}

// Births, deaths and marriages recorded without a date are described as
// such.
func TestLifespansAboutUndated(t *testing.T) {
	doc, err := gedcom.ParseString(`0 HEAD
0 @I1@ INDI
1 NAME A /X/
1 BIRT
2 DATE 1800
2 PLAC Leeds
1 DEAT Y
0 @I2@ INDI
1 NAME B /X/
1 BIRT
2 PLAC Leeds
1 RESI
2 DATE 1841
1 DEAT
2 PLAC York
0 @I3@ INDI
1 NAME C /X/
1 BIRT
2 DATE 1830
1 BURI
0 @I4@ INDI
1 NAME D /X/
1 BIRT
2 DATE 1805
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I4@
1 MARR
2 PLAC Leeds
1 CHIL @I2@
1 CHIL @I3@
0 TRLR
`)
	if err != nil {
		t.Fatal(err)
	}
	c := Lifespans(doc.Individual("I1"), LifespanOptions{})
	for id, want := range map[string]string{
		"I1": "A X · born 1800 in Leeds · died, date not recorded",
		"I2": "B X · born in Leeds, date not recorded, first record residence 1841 · died in York, date not recorded",
		"I3": "C X · born 1830 · burial, date not recorded",
		"I4": "D X · born 1805 · married in Leeds · no death recorded",
	} {
		if got := c.About(c.Find(doc.Individual(id))); got != want {
			t.Errorf("about %s:\n got %s\nwant %s", id, got, want)
		}
	}
}

func TestLifespansWidths(t *testing.T) {
	doc := loadSample(t)
	for _, w := range []int{50, 60, 84, 100, 120, 200} {
		for _, ascii := range []bool{false, true} {
			c := Lifespans(doc.Individual("I14"), LifespanOptions{Generations: 2, Width: w, ASCII: ascii})
			for i := range c.Lines {
				if lw := runewidth.StringWidth(c.LineText(i)); lw > w {
					t.Errorf("width %d: line of %d: %q", w, lw, c.LineText(i))
				}
			}
			text := c.String()
			if ascii && strings.ContainsAny(text, "━╸╺╍┄┊⚭─┬≥") {
				t.Errorf("width %d: ASCII chart with other characters:\n%s", w, text)
			}
			// The columns on the right need room.
			if right := strings.Contains(text, "with Arthur"); right != (w >= 100) {
				t.Errorf("width %d: right columns %v\n%s", w, right, text)
			}
		}
	}
	// A wide screen shows a year in a half column; no width draws two
	// years in a column.
	c := Lifespans(doc.Individual("I3"), LifespanOptions{Width: 200})
	if !strings.Contains(c.LineText(0), "1800      1810") {
		t.Errorf("200 columns: %q", c.LineText(0))
	}
}

func TestLifespansWithoutDates(t *testing.T) {
	doc, err := gedcom.ParseFile(filepath.Join("..", "testdata", "cousins.ged"))
	if err != nil {
		t.Fatal(err)
	}
	// Rosa's parents are cousins: Clara is her grandmother and her
	// father's aunt, and is shown once.
	c := Lifespans(doc.Individual("R"), LifespanOptions{Generations: 3, Width: 80})
	if c.Head != 0 || c.Foot != 0 || len(c.Lives) != 9 {
		t.Errorf("head %d, foot %d, %d lives:\n%s", c.Head, c.Foot, len(c.Lives), c.String())
	}
	if got := c.About(0); got != "Rosa Kern · no dates recorded" {
		t.Errorf("about Rosa: %s", got)
	}

	// Somebody who is their own grandfather must not hang the chart.
	doc, err = gedcom.ParseString(`0 HEAD
0 @A@ INDI
1 NAME A /X/
1 BIRT
2 DATE 1900
0 @B@ INDI
1 NAME B /X/
1 BIRT
2 DATE ABT 1880
1 DEAT
2 DATE BEF 1950
0 @F1@ FAM
1 HUSB @A@
1 CHIL @B@
0 @F2@ FAM
1 HUSB @B@
1 CHIL @A@
0 TRLR
`)
	if err != nil {
		t.Fatal(err)
	}
	c = Lifespans(doc.Individual("A"), LifespanOptions{Generations: 4, Width: 80})
	if len(c.Lives) != 2 || !strings.Contains(c.String(), "╍") {
		t.Errorf("loop:\n%s", c.String())
	}
}
