package chart

import (
	"strings"

	"github.com/efi/goged/gedcom"
	"github.com/mattn/go-runewidth"
)

// Hourglass draws root with the ancestors to the right, as Pedigree does,
// and the descendants to the left, mirrored like a bowtie. Each family is
// the partner, with the children of the family further left; names on the
// left are aligned towards the root:
//
//	◂ Thomas Smith (1842–1910) ──── ⚭ Ann Taylor (1820–1845) ─┐                          ┌─ William Smith (1790–1850)
//	                                                          ├─ John Smith (1817–1880) ─┤
//	      ◂ Emma Smith (1848–) ─┐                             │                          └─ Elizabeth Brown (c.1795–1860)
//	                            ├─ ⚭ Sarah White (1825–1890) ─┘
//	  George Smith (1850–1851) ─┘
//
// A person or family with one child, or an odd number, shares a line with
// the middle one. ◂ marks descendants beyond the generations shown, as ▸
// does ancestors. A person who appears again among the descendants is not
// expanded again. The nodes on the left are marked Left; Up leads towards
// the root on both sides, and the root is node 0.
func Hourglass(root *gedcom.Individual, opts Options) *Chart {
	opts = opts.normalized()
	c, right := pedigree(root, opts)
	if root == nil {
		return c
	}
	c.columns = make([]int, len(c.Nodes))
	for i, n := range c.Nodes {
		c.columns[i] = n.Gen
	}
	left, rootLeft := c.descendantsLeft(root, opts)

	// The root's line is the same on both sides, and the left side starts
	// at column 0.
	rootRight := c.Nodes[0].Line
	top := max(rootLeft, rootRight)
	dl, dr := top-rootLeft, top-rootRight
	shift := 0
	for _, ps := range left {
		for _, p := range ps {
			shift = max(shift, -p.col)
		}
	}
	lines := make([][]placement, max(dl+len(left), dr+len(right)))
	for _, side := range []struct {
		lines [][]placement
		down  int
	}{{left, dl}, {right, dr}} {
		for r, ps := range side.lines {
			for _, p := range ps {
				p.col += shift
				lines[r+side.down] = append(lines[r+side.down], p)
			}
		}
	}
	for i := range c.Nodes {
		n := &c.Nodes[i]
		n.Col += shift
		if n.Left {
			n.Line += dl
		} else {
			n.Line += dr
		}
	}
	c.byGeneration = false
	c.assemble(lines)
	return c
}

// descendantsLeft adds the descendants of root, node 0 of c, and their
// partners as nodes on the left. It returns the placements of their lines,
// numbered from 0, with columns relative to the start of the root's label
// (negative), and the line of the root.
func (c *Chart) descendantsLeft(root *gedcom.Individual, opts Options) ([][]placement, int) {
	g := opts.glyphs()
	// A tree of what is drawn: the root, families and children in turn.
	type item struct {
		node   int // the node shown, or -1 (the root's own label is drawn by the pedigree; a partner may be unknown)
		prefix string
		label  string // the node's label, or the text shown without a node
		level  int    // 0 for the root, 1 for its families, 2 for its children …
		kids   []int
		row    int
	}
	width := func(it item) int { return runewidth.StringWidth(it.prefix + it.label) }
	items := []item{{node: 0}}
	add := func(ind *gedcom.Individual, kind NodeKind, gen, up int) int {
		idx := len(c.Nodes)
		c.Nodes = append(c.Nodes, Node{Ind: ind, Kind: kind, Gen: gen, Up: up, Left: true})
		c.Nodes[up].Down = append(c.Nodes[up].Down, idx)
		return idx
	}
	seen := map[*gedcom.Individual]bool{root: true}
	hasChildren := func(ind *gedcom.Individual) bool {
		for _, f := range ind.FamiliesAsSpouse() {
			if len(f.Children) > 0 {
				return true
			}
		}
		return false
	}

	var families func(it, gen int)
	families = func(it, gen int) {
		ind, node := c.Nodes[items[it].node].Ind, items[it].node
		for _, f := range ind.FamiliesAsSpouse() {
			fam := item{node: -1, prefix: g.marriage, label: "(unknown)", level: items[it].level + 1}
			parent := node
			if spouse := f.Partner(ind); spouse != nil {
				fam.node = add(spouse, KindSpouse, gen, node)
				fam.label = fitLabel(spouse, opts.MaxLabel)
				parent = fam.node
			}
			fi := len(items)
			items = append(items, fam)
			items[it].kids = append(items[it].kids, fi)
			for _, child := range f.Children {
				label := fitLabel(child, opts.MaxLabel)
				prefix := ""
				if l, _ := child.ChildLink(f); !l.IsBirth() {
					prefix = "(" + string(l.Kind()) + ") "
				}
				expand := gen+1 < opts.Generations-1 && !seen[child]
				switch {
				case seen[child]:
					prefix += "(see above) "
				case !expand && hasChildren(child):
					prefix += g.moreLeft
				}
				seen[child] = true
				ci := len(items)
				items = append(items, item{node: add(child, KindPerson, gen+1, parent), prefix: prefix, label: label, level: fam.level + 1})
				items[fi].kids = append(items[fi].kids, ci)
				c.Nodes[items[ci].node].More = strings.Contains(prefix, g.moreLeft)
				if expand {
					families(ci, gen+1)
				}
			}
		}
	}
	if opts.Generations > 1 {
		families(0, 0)
	}

	// Rows: a parent sits between the halves of its children, or on the
	// line of the middle one.
	row := 0
	var place func(i int)
	place = func(i int) {
		kids := items[i].kids
		switch k := len(kids); {
		case k == 0:
			items[i].row = row
			row++
		case k%2 == 1:
			for j, kid := range kids {
				place(kid)
				if j == k/2 {
					items[i].row = items[kid].row
				}
			}
		default:
			for j, kid := range kids {
				if j == k/2 {
					items[i].row = row
					row++
				}
				place(kid)
			}
		}
	}
	place(0)

	// Columns, from the root to the left: each level as wide as its widest
	// text, which ends next to the connector of its parent.
	levels := 0
	for _, it := range items {
		levels = max(levels, it.level)
	}
	widths := make([]int, levels+1)
	for _, it := range items[1:] {
		widths[it.level] = max(widths[it.level], width(it))
	}
	end := make([]int, levels+1)  // where the texts of a level end
	conn := make([]int, levels+1) // the connector of a level's children
	conn[0] = -3
	for l := 1; l <= levels; l++ {
		end[l] = conn[l-1] - 2
		conn[l] = end[l] - widths[l] - 3
	}

	lines := make([][]placement, row)
	for i, it := range items {
		start := 0 // where the text starts
		if i > 0 {
			start = end[it.level] - width(it)
			labelAt := start + runewidth.StringWidth(it.prefix)
			lines[it.row] = append(lines[it.row], placement{start, it.prefix, -1}, placement{labelAt, it.label, it.node},
				placement{end[it.level], " " + g.horiz, -1})
			if it.node >= 0 {
				n := &c.Nodes[it.node]
				n.Line, n.Col, n.Width = it.row, labelAt, runewidth.StringWidth(it.label)
			}
		}
		if len(it.kids) == 0 {
			if i == 0 && opts.Generations <= 1 && hasChildren(root) {
				lines[it.row] = append(lines[it.row], placement{-2, g.moreLeft, -1})
			}
			continue
		}
		k := conn[it.level]
		lines[it.row] = append(lines[it.row], placement{k + 1, strings.Repeat(g.horiz, start-k-2) + " ", -1})
		first, last := it.row, it.row
		kidRow := map[int]bool{}
		for _, kid := range it.kids {
			first, last = min(first, items[kid].row), max(last, items[kid].row)
			kidRow[items[kid].row] = true
		}
		for r := first; r <= last; r++ {
			lines[r] = append(lines[r], placement{k, g.box(r > first, r == it.row, r < last, kidRow[r]), -1})
		}
	}
	for i := range c.Nodes {
		if c.Nodes[i].Left {
			c.columns = append(c.columns, 0)
		}
	}
	for _, it := range items[1:] {
		if it.node >= 0 {
			c.columns[it.node] = -it.level
		}
	}
	return lines, items[0].row
}
