// Package chart renders family trees as text: a horizontal pedigree chart
// of ancestors and an indented tree of descendants. Charts record the
// position of every person so that a user interface can highlight and
// navigate between them.
package chart

import (
	"sort"
	"strings"

	"github.com/efi/goged/gedcom"
	"github.com/mattn/go-runewidth"
)

// Options control chart rendering.
type Options struct {
	// Generations is the number of generations shown, including the root
	// person. Values below 1 select the default of 4.
	Generations int
	// MaxLabel truncates person labels to this display width, shortening
	// the name rather than the life dates. Values below 8 select the default
	// of 40.
	MaxLabel int
	// ASCII draws lines with plain ASCII characters instead of Unicode box
	// drawing characters.
	ASCII bool
}

func (o Options) normalized() Options {
	if o.Generations < 1 {
		o.Generations = 4
	}
	if o.MaxLabel < 8 {
		o.MaxLabel = 40
	}
	return o
}

// NodeKind distinguishes people in the line of the chart from spouses shown
// in descendant charts.
type NodeKind int

// Node kinds.
const (
	KindPerson NodeKind = iota
	KindSpouse
)

// Node is a person placed on the chart.
type Node struct {
	Ind   *gedcom.Individual
	Kind  NodeKind
	Line  int // line index in Chart.Lines
	Col   int // display column at which the label starts
	Width int // display width of the label
	Gen   int // generation relative to the root (0)
	// Up is the node one step towards the root (-1 for the root). Down
	// lists the nodes one step away from it: parents in a pedigree chart,
	// spouses and children in a descendant chart.
	Up   int
	Down []int
	// More is set when the person has relatives beyond the generation limit.
	More bool
}

// Segment is a run of text on a line. Node is the index of the node the
// text belongs to, or -1 for lines and padding.
type Segment struct {
	Text string
	Node int
}

// Chart is a rendered tree.
type Chart struct {
	Lines [][]Segment
	Nodes []Node
	Width int // display width of the widest line
}

// String returns the chart as plain text.
func (c *Chart) String() string {
	var b strings.Builder
	for i := range c.Lines {
		b.WriteString(c.LineText(i))
		b.WriteByte('\n')
	}
	return b.String()
}

// LineText returns one line of the chart as plain text.
func (c *Chart) LineText(i int) string {
	var b strings.Builder
	for _, s := range c.Lines[i] {
		b.WriteString(s.Text)
	}
	return b.String()
}

// NodeAt returns the first node on a line, or -1.
func (c *Chart) NodeAt(line int) int {
	for i, n := range c.Nodes {
		if n.Line == line {
			return i
		}
	}
	return -1
}

// Find returns the index of the first node showing ind, or -1.
func (c *Chart) Find(ind *gedcom.Individual) int {
	for i, n := range c.Nodes {
		if n.Ind == ind {
			return i
		}
	}
	return -1
}

// Next returns the node on the closest line after (dir > 0) or before
// (dir < 0) the line of node i, or i itself if there is none.
func (c *Chart) Next(i, dir int) int {
	if i < 0 || i >= len(c.Nodes) {
		return i
	}
	line := c.Nodes[i].Line
	best := i
	bestLine := 0
	for j, n := range c.Nodes {
		if dir > 0 && n.Line > line && (best == i || n.Line < bestLine) {
			best, bestLine = j, n.Line
		}
		if dir < 0 && n.Line < line && (best == i || n.Line > bestLine) {
			best, bestLine = j, n.Line
		}
	}
	return best
}

// Label formats a person as "Name (1820–1890)".
func Label(ind *gedcom.Individual) string {
	if ind == nil {
		return "?"
	}
	l := ind.DisplayName()
	if span := ind.Lifespan(); span != "" {
		l += " (" + span + ")"
	}
	return l
}

// fitLabel formats a person as Label does in at most width display
// columns. Long names are shortened so that the life dates remain:
// "Freiherr Erich Karl von St… (1855–)".
func fitLabel(ind *gedcom.Individual, width int) string {
	full := Label(ind)
	if ind == nil || runewidth.StringWidth(full) <= width {
		return full
	}
	dates := ""
	if span := ind.Lifespan(); span != "" {
		dates = " (" + span + ")"
	}
	room := width - runewidth.StringWidth(dates)
	if dates == "" || room < 6 {
		return truncate(full, width)
	}
	return truncate(ind.DisplayName(), room) + dates
}

// truncate shortens s to at most width display columns, ending in "…".
func truncate(s string, width int) string {
	if runewidth.StringWidth(s) <= width {
		return s
	}
	return runewidth.Truncate(s, width, "…")
}

type glyphs struct {
	horiz, vert, top, bottom, teeBoth, teeUp, teeDown string
	branch, lastBranch, pipe, blank, marriage         string
	moreUp, moreDown                                  string
}

var unicodeGlyphs = glyphs{
	horiz: "─", vert: "│", top: "┌", bottom: "└", teeBoth: "┤", teeUp: "┘", teeDown: "┐",
	branch: "├── ", lastBranch: "└── ", pipe: "│   ", blank: "    ", marriage: "⚭ ",
	moreUp: " ▸", moreDown: " ▾",
}

var asciiGlyphs = glyphs{
	horiz: "-", vert: "|", top: "+", bottom: "+", teeBoth: "+", teeUp: "+", teeDown: "+",
	branch: "|-- ", lastBranch: "`-- ", pipe: "|   ", blank: "    ", marriage: "= ",
	moreUp: " >", moreDown: " +",
}

func (o Options) glyphs() glyphs {
	if o.ASCII {
		return asciiGlyphs
	}
	return unicodeGlyphs
}

// placement is text positioned at a display column of a line.
type placement struct {
	col  int
	text string
	node int
}

// assemble turns placements into segments, filling gaps with spaces and
// merging adjacent connector text.
func assemble(ps []placement) ([]Segment, int) {
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].col < ps[j].col })
	var segs []Segment
	col := 0
	add := func(text string, node int) {
		if text == "" {
			return
		}
		if n := len(segs); n > 0 && node == -1 && segs[n-1].Node == -1 {
			segs[n-1].Text += text
			return
		}
		segs = append(segs, Segment{Text: text, Node: node})
	}
	for _, p := range ps {
		if p.col > col {
			add(strings.Repeat(" ", p.col-col), -1)
			col = p.col
		}
		add(p.text, p.node)
		col += runewidth.StringWidth(p.text)
	}
	return segs, col
}

// Pedigree draws the ancestors of root as a horizontal chart with the root
// on the left, fathers above and mothers below:
//
//	                    ┌─ Grandfather
//	         ┌─ Father ─┤
//	         │          └─ Grandmother
//	Root ────┤
//	         └─ Mother
func Pedigree(root *gedcom.Individual, opts Options) *Chart {
	opts = opts.normalized()
	g := opts.glyphs()
	c := &Chart{}
	if root == nil {
		return c
	}

	// Build the tree; rows are assigned in-order (father's subtree, the
	// person, mother's subtree) so every node gets a row of its own.
	type pnode struct {
		label string
		row   int
	}
	var meta []pnode
	row := 0
	var build func(ind *gedcom.Individual, gen, up int) int
	build = func(ind *gedcom.Individual, gen, up int) int {
		idx := len(c.Nodes)
		c.Nodes = append(c.Nodes, Node{Ind: ind, Kind: KindPerson, Gen: gen, Up: up})
		meta = append(meta, pnode{})
		father, mother := ind.Father(), ind.Mother()
		expand := gen < opts.Generations-1
		if !expand && (father != nil || mother != nil) {
			c.Nodes[idx].More = true
		}
		if expand && father != nil {
			f := build(father, gen+1, idx)
			c.Nodes[idx].Down = append(c.Nodes[idx].Down, f)
		}
		meta[idx].row = row
		row++
		if expand && mother != nil {
			m := build(mother, gen+1, idx)
			c.Nodes[idx].Down = append(c.Nodes[idx].Down, m)
		}
		label := fitLabel(ind, opts.MaxLabel)
		if c.Nodes[idx].More {
			label += g.moreUp
		}
		meta[idx].label = label
		return idx
	}
	build(root, 0, -1)

	// Column layout: each generation is as wide as its widest label.
	maxGen := 0
	for _, n := range c.Nodes {
		maxGen = max(maxGen, n.Gen)
	}
	widths := make([]int, maxGen+1)
	for i, n := range c.Nodes {
		widths[n.Gen] = max(widths[n.Gen], runewidth.StringWidth(meta[i].label))
	}
	x0 := make([]int, maxGen+1) // label start per generation
	xc := make([]int, maxGen+1) // connector column per generation
	for gen := 0; gen <= maxGen; gen++ {
		if gen > 0 {
			x0[gen] = xc[gen-1] + 3
		}
		xc[gen] = x0[gen] + widths[gen] + 2
	}

	lines := make([][]placement, row)
	for i := range c.Nodes {
		n := &c.Nodes[i]
		n.Line = meta[i].row
		n.Col = x0[n.Gen]
		n.Width = runewidth.StringWidth(meta[i].label)
		lines[n.Line] = append(lines[n.Line], placement{n.Col, meta[i].label, i})
		if n.Up >= 0 {
			lines[n.Line] = append(lines[n.Line], placement{xc[n.Gen-1] + 1, g.horiz + " ", -1})
		}
		if len(n.Down) == 0 {
			continue
		}
		// Line from the label to the connector column.
		fill := " " + strings.Repeat(g.horiz, widths[n.Gen]+1-n.Width)
		lines[n.Line] = append(lines[n.Line], placement{n.Col + n.Width, fill, -1})
		top, bottom := n.Line, n.Line
		for _, d := range n.Down {
			top = min(top, meta[d].row)
			bottom = max(bottom, meta[d].row)
		}
		x := xc[n.Gen]
		for r := top; r <= bottom; r++ {
			ch := g.vert
			switch {
			case r == n.Line && top < n.Line && bottom > n.Line:
				ch = g.teeBoth
			case r == n.Line && top < n.Line:
				ch = g.teeUp
			case r == n.Line:
				ch = g.teeDown
			case r == top:
				ch = g.top
			case r == bottom:
				ch = g.bottom
			}
			lines[r] = append(lines[r], placement{x, ch, -1})
		}
	}
	for _, ps := range lines {
		segs, w := assemble(ps)
		c.Lines = append(c.Lines, segs)
		c.Width = max(c.Width, w)
	}
	return c
}

// Descendants draws root and their descendants as an indented tree. Each
// family appears as a spouse line with the children of that family below:
//
//	John Smith (1817–1880)
//	├── ⚭ Ann Taylor (1820–1845)  m. 1840
//	│   └── Thomas Smith (1842–1910)
//	└── ⚭ Sarah White (1825–1890)  m. 1847
//	    ├── Emma Smith (1848–)
//	    └── George Smith (1850–1851)  (adopted)
//
// Children who are not birth children of both partners are marked with the
// kind of link. A person who appears a second time, e.g. as a child of two
// families, is not expanded again.
func Descendants(root *gedcom.Individual, opts Options) *Chart {
	opts = opts.normalized()
	g := opts.glyphs()
	c := &Chart{}
	if root == nil {
		return c
	}
	emit := func(ps []placement) {
		segs, w := assemble(ps)
		c.Lines = append(c.Lines, segs)
		c.Width = max(c.Width, w)
	}
	addNode := func(ind *gedcom.Individual, kind NodeKind, gen, up int, col int, label string) int {
		idx := len(c.Nodes)
		c.Nodes = append(c.Nodes, Node{
			Ind: ind, Kind: kind, Gen: gen, Up: up, Line: len(c.Lines), Col: col,
			Width: runewidth.StringWidth(label),
		})
		if up >= 0 {
			c.Nodes[up].Down = append(c.Nodes[up].Down, idx)
		}
		return idx
	}

	seen := map[*gedcom.Individual]bool{}
	var person func(ind *gedcom.Individual, gen, up int, prefix, branch, note string)
	person = func(ind *gedcom.Individual, gen, up int, prefix, branch, note string) {
		lead := prefix + branch
		col := runewidth.StringWidth(lead)
		label := fitLabel(ind, opts.MaxLabel)
		families := ind.FamiliesAsSpouse()
		expand := gen < opts.Generations-1 && !seen[ind]
		if seen[ind] {
			note += "  (see above)"
		}
		seen[ind] = true
		more := false
		if !expand && note == "" {
			for _, f := range families {
				if len(f.Children) > 0 {
					more = true
				}
			}
		}
		idx := addNode(ind, KindPerson, gen, up, col, label)
		c.Nodes[idx].More = more
		ps := []placement{{0, lead, -1}, {col, label, idx}}
		end := col + c.Nodes[idx].Width
		if more {
			ps = append(ps, placement{end, g.moreDown, -1})
			end += runewidth.StringWidth(g.moreDown)
		}
		if note != "" {
			ps = append(ps, placement{end, note, -1})
		}
		emit(ps)
		if !expand {
			return
		}

		// Children continue the vertical line of their parent's branch.
		childPrefix := prefix
		switch branch {
		case g.branch:
			childPrefix += g.pipe
		case g.lastBranch:
			childPrefix += g.blank
		}
		for fi, f := range families {
			lastFam := fi == len(families)-1
			famBranch, famPipe := g.branch, g.pipe
			if lastFam {
				famBranch, famPipe = g.lastBranch, g.blank
			}
			spouse := f.Partner(ind)
			lead := childPrefix + famBranch + g.marriage
			col := runewidth.StringWidth(lead)
			parent := idx
			ps := []placement{{0, lead, -1}}
			if spouse != nil {
				label := fitLabel(spouse, opts.MaxLabel)
				parent = addNode(spouse, KindSpouse, gen, idx, col, label)
				ps = append(ps, placement{col, label, parent})
				col += c.Nodes[parent].Width
			} else {
				ps = append(ps, placement{col, "(unknown)", -1})
				col += len("(unknown)")
			}
			if m := f.Marriage(); m != nil && m.Date.IsValid() {
				ps = append(ps, placement{col, "  m. " + m.Date.ShortYear(), -1})
			}
			emit(ps)
			for ci, child := range f.Children {
				br := g.branch
				if ci == len(f.Children)-1 {
					br = g.lastBranch
				}
				note := ""
				if l, _ := child.ChildLink(f); !l.IsBirth() {
					note = "  (" + string(l.Kind()) + ")"
				}
				person(child, gen+1, parent, childPrefix+famPipe, br, note)
			}
		}
	}
	person(root, 0, -1, "", "", "")
	return c
}
