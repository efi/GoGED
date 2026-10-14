package chart

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/efi/goged/gedcom"
)

func TestHourglassGolden(t *testing.T) {
	doc := loadSample(t)
	got := Hourglass(doc.Individual("I3"), Options{Generations: 2}).String()
	want := golden(`
◂ Thomas Smith (1842–1910) ──── ⚭ Ann Taylor (1820–1845) ─┐                          ┌─ William Smith (1790–1850)
                                                          ├─ John Smith (1817–1880) ─┤
      ◂ Emma Smith (1848–) ─┐                             │                          └─ Elizabeth Brown (c.1795–1860)
                            ├─ ⚭ Sarah White (1825–1890) ─┘
  George Smith (1850–1851) ─┘
`)
	if got != want {
		t.Errorf("hourglass:\n%s\nwant:\n%s", got, want)
	}

	// Three children: the middle one shares the line of the family.
	got = Hourglass(doc.Individual("I14"), Options{Generations: 3, ASCII: true}).String()
	want = golden(`
                                                                                                                +- John Smith (1817–1880) >
                                                                                   +- Thomas Smith (1842–1910) -+
        Harold Smith (1891–) -+                                                    |                            +- Ann Taylor (1820–1845)
(adopted) Rose Smith (1893–) -+- = Lucy King (1868–) --- Arthur Smith (1866–1940) -+
    Infant Smith (1895–1895) -+                                                    +- Jane Doe (1844–)
`)
	if got != want {
		t.Errorf("ASCII hourglass:\n%s\nwant:\n%s", got, want)
	}
}

func TestHourglassNodes(t *testing.T) {
	doc := loadSample(t)
	c := Hourglass(doc.Individual("I3"), Options{Generations: 3})
	node := func(id string) int {
		t.Helper()
		i := c.Find(doc.Individual(id))
		if i < 0 {
			t.Fatalf("%s is not in the chart", id)
		}
		return i
	}
	root, ann, thomas, arthur := c.Nodes[0], c.Nodes[node("I5")], c.Nodes[node("I7")], c.Nodes[node("I14")]
	if root.Ind.ID != "I3" || root.Left || root.Up != -1 {
		t.Errorf("root %+v", root)
	}
	if !ann.Left || ann.Kind != KindSpouse || ann.Up != 0 || !thomas.Left || thomas.Up != node("I5") || !arthur.More {
		t.Errorf("Ann %+v, Thomas %+v, Arthur %+v", ann, thomas, arthur)
	}
	// The root leads to its parents and its families; Down is in that order.
	if f := c.Nodes[root.Down[0]]; f.Ind.ID != "I1" || f.Left || c.Nodes[root.Down[2]].Ind.ID != "I5" {
		t.Errorf("root.Down = %v", root.Down)
	}
	// Labels are where the nodes say, and the root's line is shared by its
	// first family and that family's only child.
	for i, n := range c.Nodes {
		if got := c.LineText(n.Line); !strings.Contains(got, Label(n.Ind)) {
			t.Errorf("node %d (%s) is not on line %d: %q", i, n.Ind.ID, n.Line, got)
		}
	}
	if ann.Line != thomas.Line || ann.Line != root.Line-1 {
		t.Errorf("lines: root %d, Ann %d, Thomas %d", root.Line, ann.Line, thomas.Line)
	}

	// Up and down stay within a column on either side.
	for _, tt := range []struct {
		from string
		dir  int
		want string
	}{
		{"I7", 1, "I8"}, // Thomas to his half-sister Emma
		{"I8", 1, "I9"},
		{"I9", 1, "I9"},
		{"I5", 1, "I6"}, // between the partners
		{"I1", 1, "I2"}, // between the parents
		{"I3", 1, "I3"},
		{"I14", -1, "I14"}, // alone among the grandchildren on this side
	} {
		if got := c.Nodes[c.Next(node(tt.from), tt.dir)].Ind.ID; got != tt.want {
			t.Errorf("Next(%s, %d) = %s, want %s", tt.from, tt.dir, got, tt.want)
		}
	}
}

func TestHourglassEdges(t *testing.T) {
	doc := loadSample(t)
	if got := Hourglass(doc.Individual("I3"), Options{Generations: 1}).String(); got != "◂ John Smith (1817–1880) ▸\n" {
		t.Errorf("one generation: %q", got)
	}
	if c := Hourglass(nil, Options{}); len(c.Nodes) != 0 || c.String() != "" {
		t.Error("nil hourglass")
	}

	// Without descendants, the hourglass is the pedigree, placeholders and
	// all.
	cousins, err := gedcom.ParseFile(filepath.Join("..", "testdata", "cousins.ged"))
	if err != nil {
		t.Fatal(err)
	}
	find := func(name string) *gedcom.Individual {
		for _, ind := range cousins.Individuals {
			if ind.DisplayName() == name {
				return ind
			}
		}
		t.Fatalf("no %s", name)
		return nil
	}
	rosa := find("Rosa Kern")
	if h, p := Hourglass(rosa, Options{Generations: 5}), Pedigree(rosa, Options{Generations: 5}); h.String() != p.String() || len(h.Nodes) != len(p.Nodes) {
		t.Errorf("hourglass without descendants:\n%s\npedigree:\n%s", h, p)
	}
	// The child of cousins descends from Georg Kern twice.
	got := Hourglass(find("Georg Kern"), Options{Generations: 5}).String()
	if strings.Count(got, "Rosa Kern") != 2 || !strings.Contains(got, "(see above) Rosa Kern ─── ⚭ Felix Kern") {
		t.Errorf("repeated descendant:\n%s", got)
	}

	u, err := gedcom.ParseString("0 HEAD\n0 @A@ INDI\n1 NAME A /X/\n0 @C@ INDI\n1 NAME C /X/\n0 @F@ FAM\n1 HUSB @A@\n1 CHIL @C@\n0 TRLR\n")
	if err != nil {
		t.Fatal(err)
	}
	c := Hourglass(u.Individual("A"), Options{})
	if c.String() != "C X ─── ⚭ (unknown) ─── A X\n" || len(c.Nodes) != 2 || c.Nodes[1].Up != 0 || !c.Nodes[1].Left {
		t.Errorf("unknown partner: %q, %+v", c.String(), c.Nodes)
	}
}
