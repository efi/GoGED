package chart

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
	"github.com/mattn/go-runewidth"
)

// A lifespan chart draws the lives of a person's family as bars on a time
// axis: the parents, the person with their siblings, and each partner
// with the children. The parts of the lives while the person lived stand
// out, so the chart shows who knew whom, and for how long.
//
//	                      1800      1820      1840      1860      1880          lived  with John
//	                      ┬────┬────┬────┬────┬────┬────┬────┬────┬────┬
//	father William Smith  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━   ┊              1790–1850  32 y
//	       John Smith           ╺━━━━━━━━━━━⚭━━⚭━━━━━━━━━━━━━━━━┊         1817–1880  63 y
//	wife   Ann Taylor           ┊ ━━━━━━━━━━⚭━╸                 ┊         1820–1845  ⚭ 5 y

// LifespanOptions control lifespan charts.
type LifespanOptions struct {
	// Generations is how far the chart reaches: 1 shows the parents,
	// siblings, partners and children; 2 adds the grandparents, the aunts
	// and uncles and the grandchildren, and so on. Values below 1 select 1.
	Generations int
	// Width is the display width to fill. 0 draws two years per column.
	Width int
	// ASCII draws with plain ASCII characters.
	ASCII bool
}

// Life is a row of a lifespan chart: a person and what the chart knows
// of their life.
type Life struct {
	Ind *gedcom.Individual
	// Relation is what the person is to the chart's person, such as
	// "father", "half-sister" or "wife"; empty for that person.
	Relation string
	// Gen counts the generations above the chart's person, negative
	// below.
	Gen int
	// Depth indents children under their other parent and grandchildren
	// under their parent.
	Depth int
	// Partner is set for the partners of the chart's person.
	Partner bool
	// Born and Died are the years the dates of birth and death may
	// denote, with room for approximate dates: about 1795 is 1793–1797.
	Born, Died Years
	// Records are the years of the earliest and the latest dated record
	// of the person, which stand in for a birth or death not known.
	Records Years
	// Marriages are the years of marriages: all of the chart's person's,
	// and of a partner those with the chart's person.
	Marriages []int
	// Together is how long the person lived at the same time as the
	// chart's person, for a partner how long they were married, and for
	// the chart's person the age they reached.
	Together Together
	// Node is the person's node in Chart.Nodes.
	Node int
}

// Years is a range of years; OK is false if there is none.
type Years struct {
	From, To int
	OK       bool
}

// Together is how long two lives went on at the same time.
type Together struct {
	Years   int
	Married bool // the years of a marriage; without OK, its date is not known
	Approx  bool // an approximate date is involved
	AtLeast bool // a birth or death is not known: it may have been longer
	OK      bool
}

// Text formats the time: "32 y", "~43 y", "≥ 22 y", "⚭ 5 y".
func (t Together) Text(ascii bool) string {
	marriage := ""
	if t.Married {
		marriage = "⚭"
		if ascii {
			marriage = "&"
		}
	}
	if !t.OK || (t.AtLeast && t.Years == 0) {
		return marriage
	}
	s := strconv.Itoa(t.Years) + " y"
	switch {
	case t.AtLeast && ascii:
		s = ">= " + s
	case t.AtLeast:
		s = "≥ " + s
	case t.Approx:
		s = "~" + s
	}
	if marriage != "" {
		s = marriage + " " + s
	}
	return s
}

// lifeGlyphs draw a lifespan chart: a solid bar and its left and right
// halves, uncertain and unknown stretches, the guides at the beginning
// and end of the chart's person's life, marriages, and the axis.
type lifeGlyphs struct {
	solid, left, right, approx, unknown, guide, marriage, rule, tick string
}

var (
	unicodeLife = lifeGlyphs{"━", "╸", "╺", "╍", "┄", "┊", "⚭", "─", "┬"}
	asciiLife   = lifeGlyphs{"=", "=", "=", "~", ".", ":", "&", "-", "+"}
)

// The longest relation and name shown in full; longer ones are shortened.
const (
	maxRelation = 28
	maxName     = 24
)

// Lifespans draws the lifespans of root's family.
func Lifespans(root *gedcom.Individual, opts LifespanOptions) *Chart {
	rows := lives(root, max(1, opts.Generations))
	l := newLifeLayout(root, rows, opts)
	return l.draw(rows)
}

// lives gathers the rows of a lifespan chart in order, nil between
// groups: the ancestors from the earliest generation down, each followed
// by their siblings within the generations shown; the person with their
// siblings; then each partner with the children of that family and their
// descendants. Everybody is shown once. The relationships are named from
// the way they were found, which is quicker than looking for them.
func lives(root *gedcom.Individual, gens int) []*Life {
	var out []*Life
	shown := map[*gedcom.Individual]bool{root: true}
	add := func(ind *gedcom.Individual, gen, depth int, relation string) bool {
		if ind == nil || shown[ind] {
			return false
		}
		shown[ind] = true
		out = append(out, &Life{Ind: ind, Gen: gen, Depth: depth, Relation: relation})
		return true
	}
	// siblings adds the siblings of ind, who is up generations above root
	// by links of the kind given.
	siblings := func(ind *gedcom.Individual, up int, kind gedcom.Pedigree) {
		for _, s := range ind.Siblings() {
			k := kind
			for _, l := range ind.FamiliesAsChild() {
				if sl, ok := s.ChildLink(l.Family); ok {
					k = farther(farther(k, l.Kind()), sl.Kind())
				}
			}
			add(s, up, 0, genealogy.Describe(up+1, 1, s.Sex, false, k))
		}
		for _, s := range ind.HalfSiblings() {
			add(s, up, 0, genealogy.Describe(up+1, 1, s.Sex, true, kind))
		}
	}
	group := func() {
		if len(out) > 0 && out[len(out)-1] != nil {
			out = append(out, nil)
		}
	}

	// The ancestors, generation by generation, as the pedigree shows them.
	type ancestor struct {
		ind  *gedcom.Individual
		kind gedcom.Pedigree // a link on the way that is not a birth link
	}
	levels := [][]ancestor{{{root, gedcom.PedigreeUnknown}}}
	for g := 1; g <= gens; g++ {
		var next []ancestor
		for _, a := range levels[g-1] {
			for _, parent := range []*gedcom.Individual{a.ind.Father(), a.ind.Mother()} {
				if parent == nil {
					continue
				}
				kind := a.kind
				for _, pl := range a.ind.ParentLinks() {
					if pl.Parent == parent {
						kind = farther(kind, pl.Pedigree)
					}
				}
				next = append(next, ancestor{parent, kind})
			}
		}
		levels = append(levels, next)
	}
	for g := gens; g >= 1; g-- {
		for _, a := range levels[g] {
			if add(a.ind, g, 0, genealogy.Describe(g, 0, a.ind.Sex, false, a.kind)) && g < gens {
				siblings(a.ind, g, a.kind)
			}
		}
		group()
	}

	out = append(out, &Life{Ind: root})
	siblings(root, 0, gedcom.PedigreeUnknown)
	group()

	var descend func(parent, c *gedcom.Individual, f *gedcom.Family, g int, kind gedcom.Pedigree)
	descend = func(parent, c *gedcom.Individual, f *gedcom.Family, g int, kind gedcom.Pedigree) {
		if l, ok := c.ChildLink(f); ok {
			kind = farther(kind, l.Of(parent))
		}
		if !add(c, -g, g, genealogy.Describe(0, g, c.Sex, false, kind)) || g >= gens {
			return
		}
		for _, cf := range c.FamiliesAsSpouse() {
			for _, cc := range cf.Children {
				descend(c, cc, cf, g+1, kind)
			}
		}
	}
	for _, f := range root.FamiliesAsSpouse() {
		if p := f.Partner(root); p != nil && add(p, 0, 0, spouse(p)) {
			out[len(out)-1].Partner = true
		}
		for _, c := range f.Children {
			descend(root, c, f, 1, gedcom.PedigreeUnknown)
		}
		group()
	}
	if n := len(out); n > 0 && out[n-1] == nil {
		out = out[:n-1]
	}
	return out
}

// farther returns the kind of link that is not a birth link, if either is.
func farther(a, b gedcom.Pedigree) gedcom.Pedigree {
	if a.IsBirth() {
		return b
	}
	return a
}

func spouse(ind *gedcom.Individual) string {
	switch ind.Sex {
	case gedcom.SexMale:
		return "husband"
	case gedcom.SexFemale:
		return "wife"
	}
	return "spouse"
}

// yearsOf returns the years a date may denote, widened for approximate
// dates as in the search: about 1795 is 1793–1797, before 1850 is
// 1840–1850, and after 1850 is 1850–1860.
func yearsOf(d gedcom.Date) Years {
	lo, hi, ok := d.YearRange()
	if !ok {
		return Years{}
	}
	switch d.Modifier {
	case gedcom.DateAbout, gedcom.DateCalculated, gedcom.DateEstimated:
		lo, hi = lo-2, hi+2
	case gedcom.DateBefore:
		lo = hi - 10
	case gedcom.DateAfter:
		hi = lo + 10
	}
	return Years{lo, hi, true}
}

// records returns the earliest and the latest dated event of a person,
// their families' included.
func records(ind *gedcom.Individual) (first, last *gedcom.Event) {
	consider := func(e *gedcom.Event) {
		if !e.Date.IsValid() {
			return
		}
		if first == nil || e.Date.Compare(first.Date) < 0 {
			first = e
		}
		if last == nil || e.Date.Compare(last.Date) > 0 {
			last = e
		}
	}
	for _, e := range ind.Events {
		consider(e)
	}
	for _, f := range ind.FamiliesAsSpouse() {
		for _, e := range f.Events {
			consider(e)
		}
	}
	return first, last
}

// span is the beginning and end of a life as dates: the birth and death,
// or where those are not known, the first and last record.
type span struct {
	start, end           gedcom.Date
	startKnown, endKnown bool // start is the birth, end the death
}

func spanOf(ind *gedcom.Individual) span {
	s := span{start: ind.BirthDate(), end: ind.DeathDate()}
	s.startKnown, s.endKnown = s.start.IsValid(), s.end.IsValid()
	first, last := records(ind)
	if !s.startKnown && first != nil {
		s.start = first.Date
	}
	if !s.endKnown && last != nil {
		s.end = last.Date
	}
	return s
}

// bound is a date, and whether it is a birth or death rather than a
// record standing in for one.
type bound struct {
	date  gedcom.Date
	known bool
}

// later returns the later of two bounds, known if either equal one is.
func later(a, b bound) bound {
	switch c := a.date.Compare(b.date); {
	case c > 0:
		return a
	case c < 0:
		return b
	}
	return bound{a.date, a.known || b.known}
}

func earlier(a, b bound) bound {
	switch c := a.date.Compare(b.date); {
	case c < 0:
		return a
	case c > 0:
		return b
	}
	return bound{a.date, a.known || b.known}
}

// between measures the time from one bound to another.
func between(from, to bound) Together {
	if !from.date.IsValid() || !to.date.IsValid() || to.date.Compare(from.date) < 0 {
		return Together{}
	}
	t := Together{OK: true, Approx: from.date.IsApproximate() || to.date.IsApproximate(), AtLeast: !from.known || !to.known}
	if age, ok := gedcom.AgeBetween(from.date, to.date); ok {
		t.Years = age.Years
	} else {
		t.Years, t.Approx = max(0, to.date.Year()-from.date.Year()), true
	}
	return t
}

// together measures how long a person lived at the same time as the
// chart's person (root), or as root's partner was married to them.
func together(root *gedcom.Individual, rs span, l *Life) Together {
	s := spanOf(l.Ind)
	if l.Ind == root {
		if !rs.startKnown {
			return Together{}
		}
		return between(bound{rs.start, true}, bound{rs.end, rs.endKnown})
	}
	end := earlier(bound{rs.end, rs.endKnown}, bound{s.end, s.endKnown})
	if l.Partner {
		for _, f := range root.FamiliesAsSpouse() {
			m := f.Marriage()
			if f.Partner(root) != l.Ind || m == nil || !m.Date.IsValid() {
				continue
			}
			if div := f.FirstEvent("DIV"); div != nil && div.Date.IsValid() {
				end = earlier(end, bound{div.Date, true})
			}
			t := between(bound{m.Date, true}, end)
			t.Married = true
			return t
		}
		return Together{Married: true}
	}
	return between(later(bound{rs.start, rs.startKnown}, bound{s.start, s.startKnown}), end)
}

// lifeLayout places the parts of a lifespan chart.
type lifeLayout struct {
	g          lifeGlyphs
	ascii      bool
	root       *gedcom.Individual
	given      string // what the axes call root: their first given name
	relW       int    // the relations, indented
	nameW      int
	datesW     int // the lifespans; 0 if the columns on the right are left out
	togW       int
	width      int // to fit, or 0
	start, ypc int // the first year, and the years per column
	cells      int
	inside     Years // root's life, for the colours of the bars
	guides     []int // the columns of root's birth and death
}

// half returns the half column a year begins in.
func (l *lifeLayout) half(year int) int { return floorDiv((year-l.start)*2, l.ypc) }

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func newLifeLayout(root *gedcom.Individual, rows []*Life, opts LifespanOptions) *lifeLayout {
	l := &lifeLayout{g: unicodeLife, ascii: opts.ASCII, root: root, width: opts.Width}
	if opts.ASCII {
		l.g = asciiLife
	}
	l.given = root.DisplayName()
	if f := strings.Fields(root.Name().Given); len(f) > 0 {
		l.given = f[0]
	}
	rs := spanOf(root)
	first, last := 0, 0
	have := false
	for _, r := range rows {
		if r == nil {
			continue
		}
		ind := r.Ind
		r.Born, r.Died = yearsOf(ind.BirthDate()), yearsOf(ind.DeathDate())
		if f, e := records(ind); f != nil {
			r.Records = Years{yearsOf(f.Date).From, yearsOf(e.Date).To, true}
		}
		for _, f := range root.FamiliesAsSpouse() {
			if m := f.Marriage(); m != nil && m.Date.IsValid() && (ind == root || f.Partner(root) == ind) {
				r.Marriages = append(r.Marriages, m.Date.Year())
			}
		}
		r.Together = together(root, rs, r)

		l.relW = max(l.relW, min(maxRelation, 2*r.Depth+runewidth.StringWidth(r.Relation)))
		l.nameW = max(l.nameW, min(maxName, runewidth.StringWidth(ind.DisplayName())))
		l.datesW = max(l.datesW, runewidth.StringWidth(ind.Lifespan()))
		l.togW = max(l.togW, runewidth.StringWidth(r.Together.Text(opts.ASCII)))

		from, to, ok := r.drawn()
		if !ok {
			continue
		}
		if !have || from < first {
			first = from
		}
		if !have || to > last {
			last = to
		}
		have = true
	}
	l.datesW = max(l.datesW, len("lived"))
	l.togW = max(l.togW, runewidth.StringWidth("with "+l.given))
	if !have {
		return l
	}

	// Years per column: two by default, or the fewest that fit the width.
	// The columns on the right go first if there is not enough room, then
	// the names get shorter.
	span := last + 1 - first
	l.ypc = 2
	if opts.Width > 0 {
		avail := opts.Width - l.labelW() - 1 - 3 - l.datesW - 2 - l.togW
		if avail < 32 {
			l.datesW, l.togW = 0, 0
			avail = opts.Width - l.labelW() - 1 - 3
		}
		if avail < 20 && l.nameW > 12 {
			cut := min(l.nameW-12, 20-avail)
			l.nameW -= cut
			avail += cut
		}
		l.ypc = 0
		for _, y := range []int{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000} {
			if span/y+1 <= avail {
				l.ypc = y
				break
			}
		}
		if l.ypc == 0 {
			l.ypc = 1000
		}
	}
	l.start = floorDiv(first, l.ypc) * l.ypc
	l.cells = floorDiv(last-l.start, l.ypc) + 1

	for _, r := range rows {
		if r != nil && r.Ind == root {
			if from, to, ok := r.drawn(); ok {
				l.inside = Years{from, to, true}
			}
			if r.Born.OK {
				l.guides = append(l.guides, l.half(r.Born.From)/2)
			}
			if r.Died.OK {
				l.guides = append(l.guides, l.half(r.Died.To)/2)
			}
		}
	}
	return l
}

func (l *lifeLayout) labelW() int { return l.relW + 1 + l.nameW }

// drawn returns the years a life is known to span: from the birth or the
// first record to the death or the last record.
func (r *Life) drawn() (from, to int, ok bool) {
	switch {
	case r.Born.OK:
		from = r.Born.From
	case r.Records.OK:
		from = r.Records.From
	default:
		return 0, 0, false
	}
	switch {
	case r.Died.OK:
		to = r.Died.To
	case r.Records.OK:
		to = max(r.Records.To, from)
	default:
		to = from
	}
	return from, max(from, to), true
}

// Kinds of half columns of a bar, the weakest first.
const (
	halfNone = iota
	halfUnknown
	halfApprox
	halfSolid
)

// bar returns the kinds of the half columns of a life.
func (l *lifeLayout) bar(r *Life) []int {
	halves := make([]int, 2*l.cells)
	// Whatever is drawn takes half a column at least.
	paint := func(from, to, kind int) {
		if to <= from {
			return
		}
		h0, h1 := l.half(from), max(l.half(to), l.half(from)+1)
		for h := max(0, h0); h < min(len(halves), h1); h++ {
			halves[h] = max(halves[h], kind)
		}
	}
	from, to, ok := r.drawn()
	if !ok {
		return halves
	}
	end := l.start + l.cells*l.ypc
	solidFrom, solidTo := from, to
	if r.Born.OK {
		solidFrom = r.Born.To
		paint(r.Born.From, r.Born.To, halfApprox)
	} else {
		paint(max(l.start, to-100), from, halfUnknown)
	}
	if r.Died.OK {
		solidTo = r.Died.From
		paint(r.Died.From, r.Died.To, halfApprox)
	} else {
		paint(to, min(end, from+100), halfUnknown)
	}
	// A year at least is certain, as of a child who died the year they
	// were born, or of somebody with a single record.
	paint(solidFrom, max(solidTo, solidFrom+1), halfSolid)
	return halves
}

// inLife reports whether the chart's person lived in the middle of a
// column.
func (l *lifeLayout) inLife(c int) bool {
	mid2 := 2*l.start + (2*c+1)*l.ypc // twice the year in the middle of the column
	return l.inside.OK && mid2 >= 2*l.inside.From && mid2 <= 2*l.inside.To
}

// line builds a line from a label and the columns of the chart.
type lineBuilder struct {
	segs []Segment
}

func (b *lineBuilder) add(text string, node int, class Class) {
	if text == "" {
		return
	}
	if n := len(b.segs); n > 0 && b.segs[n-1].Node == node && b.segs[n-1].Class == class {
		b.segs[n-1].Text += text
		return
	}
	b.segs = append(b.segs, Segment{Text: text, Node: node, Class: class})
}

// done returns the segments without trailing spaces.
func (b *lineBuilder) done() []Segment {
	for n := len(b.segs); n > 0; n = len(b.segs) {
		last := &b.segs[n-1]
		last.Text = strings.TrimRight(last.Text, " ")
		if last.Text != "" {
			break
		}
		b.segs = b.segs[:n-1]
	}
	return b.segs
}

func padTo(s string, w int) string {
	s = truncate(s, w)
	return s + strings.Repeat(" ", max(0, w-runewidth.StringWidth(s)))
}

func padLeft(s string, w int) string {
	return strings.Repeat(" ", max(0, w-runewidth.StringWidth(s))) + s
}

func (l *lifeLayout) draw(rows []*Life) *Chart {
	c := &Chart{}
	addLine := func(segs []Segment) {
		c.Lines = append(c.Lines, segs)
		w := 0
		for _, s := range segs {
			w += runewidth.StringWidth(s.Text)
		}
		c.Width = max(c.Width, w)
	}
	right := l.datesW > 0
	axisW := l.cells + 3 // the last label of the axis may reach beyond the columns

	if l.cells > 0 {
		labels := []rune(strings.Repeat(" ", axisW))
		step := 10
		for _, s := range []int{10, 20, 50, 100, 200, 500, 1000, 2000} {
			if s >= 9*l.ypc {
				step = s
				break
			}
		}
		for y := l.start + floorMod(-l.start, step); ; y += step {
			col := (y - l.start) / l.ypc
			text := strconv.Itoa(y)
			if col+len(text) > axisW {
				break
			}
			copy(labels[col:], []rune(text))
		}
		var b lineBuilder
		b.add(strings.Repeat(" ", l.labelW()+1)+string(labels), -1, ClassDim)
		if right {
			b.add(padLeft("lived", l.datesW)+"  "+"with "+l.given, -1, ClassDim)
		}
		addLine(b.done())

		var rule strings.Builder
		for col := range l.cells {
			if floorMod(l.start+col*l.ypc, step/2) == 0 {
				rule.WriteString(l.g.tick)
			} else {
				rule.WriteString(l.g.rule)
			}
		}
		b = lineBuilder{}
		b.add(strings.Repeat(" ", l.labelW()+1)+rule.String(), -1, ClassDim)
		addLine(b.done())
		c.Head = 2
	}

	// The person comes first among the nodes, as the root of other charts.
	for _, r := range rows {
		if r != nil && r.Ind == l.root {
			r.Node = 0
			c.Nodes = append(c.Nodes, Node{Ind: r.Ind, Up: -1})
		}
	}
	for _, r := range rows {
		var b lineBuilder
		if r == nil {
			b.add(strings.Repeat(" ", l.labelW()+1), -1, ClassDim)
			l.columns(&b, nil)
			addLine(b.done())
			continue
		}
		if r.Ind != l.root {
			r.Node = len(c.Nodes)
			c.Nodes = append(c.Nodes, Node{Ind: r.Ind, Gen: r.Gen, Up: -1})
		}
		n := &c.Nodes[r.Node]
		n.Line, n.Width = len(c.Lines), l.labelW()
		b.add(padTo(strings.Repeat(" ", 2*r.Depth)+r.Relation, l.relW)+" ", r.Node, ClassDim)
		b.add(padTo(r.Ind.DisplayName(), l.nameW), r.Node, ClassDefault)
		b.add(" ", -1, ClassDim)
		l.columns(&b, r)
		if right {
			b.add("   ", -1, ClassDim)
			b.add(padLeft(r.Ind.Lifespan(), l.datesW), -1, ClassText)
			class := ClassDim
			if r.Ind == l.root {
				class = ClassText
			}
			b.add("  "+r.Together.Text(l.ascii), -1, class)
		}
		addLine(b.done())
		c.Lives = append(c.Lives, *r)
	}

	// The ages of the person, and what the lines mean.
	if l.cells > 0 {
		var root *Life
		for _, r := range rows {
			if r != nil && r.Ind == l.root {
				root = r
			}
		}
		if root != nil && root.Born.OK {
			ages := []rune(strings.Repeat(" ", axisW))
			step := 10
			for _, s := range []int{10, 20, 25, 50, 100} {
				if s >= 5*l.ypc {
					step = s
					break
				}
			}
			_, to, _ := root.drawn()
			for a := 0; root.Born.From+a <= to; a += step {
				col := l.half(root.Born.From+a) / 2
				text := strconv.Itoa(a)
				if col < 0 || col+len(text) > axisW {
					continue
				}
				copy(ages[col:], []rune(text))
			}
			var b lineBuilder
			b.add(padTo(l.given+"'s age", l.labelW())+" "+string(ages), -1, ClassDim)
			addLine(b.done())
			c.Foot++
		}
		// The legend, on as many lines as the width needs.
		addLine(nil)
		c.Foot++
		var b lineBuilder
		w := 0
		for _, part := range []struct {
			glyph string
			class Class
			text  string
		}{
			{l.g.solid, ClassBar, "while " + l.given + " lived"},
			{l.g.solid, ClassDim, "before or after"},
			{l.g.approx, ClassBar, "approximate"},
			{l.g.unknown, ClassDim, "not recorded"},
			{l.g.marriage, ClassMarker, "marriage"},
		} {
			pw := 2 + runewidth.StringWidth(part.text)
			if w > 0 && l.width > 0 && w+3+pw > l.width {
				addLine(b.done())
				c.Foot++
				b, w = lineBuilder{}, 0
			}
			if w > 0 {
				b.add("   ", -1, ClassDim)
				w += 3
			}
			b.add(part.glyph, -1, part.class)
			b.add(" "+part.text, -1, ClassDim)
			w += pw
		}
		addLine(b.done())
		c.Foot++
	}
	return c
}

func floorMod(a, b int) int { return a - floorDiv(a, b)*b }

// columns adds the bar of a life to a line, or for nil just the guides.
func (l *lifeLayout) columns(b *lineBuilder, r *Life) {
	var halves []int
	marriages := map[int]bool{}
	if r != nil {
		halves = l.bar(r)
		for _, y := range r.Marriages {
			marriages[l.half(y)/2] = true
		}
	}
	for col := range l.cells {
		left, right := halfNone, halfNone
		if halves != nil {
			left, right = halves[2*col], halves[2*col+1]
		}
		strong := max(left, right)
		glyph, class := " ", ClassDim
		switch {
		case marriages[col] && strong != halfNone:
			glyph, class = l.g.marriage, ClassMarker
		case strong == halfSolid && left == right:
			glyph = l.g.solid
		case strong == halfSolid && left == halfSolid && right == halfNone:
			glyph = l.g.left
		case strong == halfSolid && left == halfNone:
			glyph = l.g.right
		case strong == halfSolid:
			glyph = l.g.solid
		case strong == halfApprox:
			glyph = l.g.approx
		case strong == halfUnknown:
			glyph = l.g.unknown
		}
		switch {
		case class == ClassMarker:
		case strong == halfNone:
			for _, g := range l.guides {
				if g == col {
					glyph = l.g.guide
				}
			}
		case strong == halfUnknown:
		case r.Ind == l.root:
			class = ClassRootBar
		case l.inLife(col):
			class = ClassBar
		}
		b.add(glyph, -1, class)
	}
}

// About describes the life of the person of node i of a lifespan chart
// in words: "Ann Taylor · born 1820 · married 1840 in Manchester · died
// 1845 at 25, when John Smith was 28".
func (c *Chart) About(i int) string {
	if i < 0 || i >= len(c.Nodes) || len(c.Nodes) == 0 {
		return ""
	}
	ind, root := c.Nodes[i].Ind, c.Nodes[0].Ind
	parts := []string{ind.DisplayName()}
	// at gives the date and the place of an event, as far as they are
	// recorded: "1820 in Leeds", "in Leeds".
	at := func(e *gedcom.Event) string {
		s := e.Date.String()
		if p := e.Place.Short(); p != "" {
			s = strings.TrimSpace(s + " in " + p)
		}
		return s
	}
	// named names a birth "born", a death "died" and other events by
	// their label.
	named := func(e *gedcom.Event) string {
		switch e.Tag {
		case "BIRT":
			return "born"
		case "DEAT":
			return "died"
		}
		return lower(e.Label())
	}
	// undated describes an event recorded without a date.
	undated := func(e *gedcom.Event) string {
		return strings.TrimSpace(named(e)+" "+at(e)) + ", date not recorded"
	}
	// The birth and death, if recorded; dated ones are preferred.
	birth := ind.FirstDatedEvent(gedcom.BirthTags...)
	death := ind.FirstDatedEvent(gedcom.DeathTags...)
	first, last := records(ind)
	switch {
	case birth != nil && !birth.Date.IsZero():
		parts = append(parts, named(birth)+" "+at(birth))
	case first == nil:
		return parts[0] + " · no dates recorded"
	default:
		s := "birth not recorded"
		if birth != nil {
			s = undated(birth)
		}
		if first != death {
			s += ", first record " + lower(first.Label()) + " " + at(first)
		}
		parts = append(parts, s)
	}
	if ind != root {
		for _, f := range root.FamiliesAsSpouse() {
			if f.Partner(root) != ind {
				continue
			}
			if m := f.Marriage(); m != nil {
				parts = append(parts, strings.TrimSpace("married "+at(m)))
			}
			if d := f.FirstEvent("DIV"); d != nil {
				parts = append(parts, strings.TrimSpace("divorced "+at(d)))
			}
		}
	}
	age := func(e *gedcom.Event) string {
		s := ""
		if a, ok := gedcom.AgeBetween(ind.BirthDate(), e.Date); ok {
			s = " at " + a.String()
		}
		if ind != root {
			if a, ok := gedcom.AgeBetween(root.BirthDate(), e.Date); ok && (!root.DeathDate().IsValid() || e.Date.Compare(root.DeathDate()) <= 0) {
				s += ", when " + root.DisplayName() + " was " + a.String()
			}
		}
		return s
	}
	switch {
	case death != nil && !death.Date.IsZero():
		parts = append(parts, named(death)+" "+at(death)+age(death))
	case death != nil:
		parts = append(parts, undated(death))
	case last != nil && last != birth:
		parts = append(parts, "no death recorded, last record "+lower(last.Label())+" "+at(last)+age(last))
	default:
		parts = append(parts, "no death recorded")
	}
	return strings.Join(parts, " · ")
}

// lower makes the first letter of a label lower case, unless the label
// begins with an abbreviation.
func lower(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if next, _ := utf8.DecodeRuneInString(s[n:]); unicode.IsUpper(next) {
		return s
	}
	return string(unicode.ToLower(r)) + s[n:]
}
