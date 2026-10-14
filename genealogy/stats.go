package genealogy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/efi/goged/gedcom"
)

// NameCount is a name with the number of people bearing it.
type NameCount struct {
	Name  string
	Count int
}

// Stats summarizes a document.
type Stats struct {
	Individuals int
	Families    int
	Sources     int
	Events      int
	Places      int // distinct places: place records (_LOC) or names
	Males       int
	Females     int
	OtherSex    int // unknown or intersex

	Surnames   []NameCount // all surnames, most common first
	GivenNames []NameCount // first given names, most common first

	EarliestYear int // 0 if there are no dated events
	LatestYear   int

	// Lifespans counts people for whom an age at death could be computed;
	// AverageLifespan is the mean of those ages in years.
	Lifespans       int
	AverageLifespan float64
	LongestLived    *gedcom.Individual
	LongestAge      gedcom.Age

	// Generations is the length of the longest line of descent.
	Generations int

	// AncestorPositions counts the places in everybody's pedigree: every
	// line of birth links from a person up to an ancestor is one. It is a
	// float, as it doubles with every generation in which ancestry
	// collapses. DistinctAncestors counts the people in these places,
	// once per pedigree, and Collapsed the people who have an ancestor in
	// more than one place, as the children of cousins do.
	AncestorPositions float64
	DistinctAncestors int
	Collapsed         int

	// Distributions for charts: years of birth (baptism if there is no
	// birth; open-ended dates are left out), ages at death (as for
	// Lifespans), the partners' ages at their marriages, and the number
	// of children in each family.
	BirthYears   Distribution
	DeathAges    Distribution
	MarriageAges Distribution
	FamilySizes  Distribution
}

// ValueCount is a value with the number of times it occurs.
type ValueCount struct {
	Value int
	Count int
}

// Distribution lists how often values occur, the smallest value first.
type Distribution []ValueCount

func distribution(m map[int]int) Distribution {
	d := make(Distribution, 0, len(m))
	for v, n := range m {
		d = append(d, ValueCount{v, n})
	}
	sort.Slice(d, func(i, j int) bool { return d[i].Value < d[j].Value })
	return d
}

// Total returns the number of values counted.
func (d Distribution) Total() int {
	n := 0
	for _, vc := range d {
		n += vc.Count
	}
	return n
}

// Bucket is a range of values, from Lo to Hi, with the number of times
// they occur. The open last bucket of Buckets has no upper limit.
type Bucket struct {
	Lo, Hi int
	Open   bool
	Count  int
}

// Buckets groups the values in ranges width wide, aligned to multiples of
// width, from the range of the smallest value to that of the largest;
// ranges without values in between are kept. Values from last on share
// one open bucket, which starts at last; the ranges then go up to it.
func (d Distribution) Buckets(width, last int) []Bucket {
	if len(d) == 0 || width < 1 {
		return nil
	}
	floor := func(v int) int {
		if v < 0 {
			return -((-v + width - 1) / width) * width
		}
		return v / width * width
	}
	var out []Bucket
	lo := floor(d[0].Value)
	if d[0].Value < last {
		top := last - 1
		if d[len(d)-1].Value < last {
			top = d[len(d)-1].Value
		}
		for b := lo; b <= floor(top); b += width {
			out = append(out, Bucket{Lo: b, Hi: min(b+width, last) - 1})
		}
	}
	if d[len(d)-1].Value >= last {
		out = append(out, Bucket{Lo: last, Hi: d[len(d)-1].Value, Open: true})
	}
	for _, vc := range d {
		i := len(out) - 1
		if vc.Value < last {
			i = (floor(vc.Value) - lo) / width
		}
		out[i].Count += vc.Count
	}
	return out
}

// PedigreeCollapse returns the file's pedigree collapse: the share of the
// ancestor positions that are taken by an ancestor who also takes another
// position in the same pedigree, in percent. It is 0 without ancestors.
func (s Stats) PedigreeCollapse() float64 {
	if s.AncestorPositions == 0 {
		return 0
	}
	return 100 * (1 - float64(s.DistinctAncestors)/s.AncestorPositions)
}

// Compute gathers statistics about doc.
func Compute(doc *gedcom.Document) Stats {
	s := Stats{
		Individuals: len(doc.Individuals),
		Families:    len(doc.Families),
		Sources:     len(doc.Sources),
	}
	surnames := map[string]int{}
	given := map[string]int{}
	places := map[string]bool{}
	births, deaths, marriages, sizes := map[int]int{}, map[int]int{}, map[int]int{}, map[int]int{}
	totalAge := 0

	for _, ind := range doc.Individuals {
		switch ind.Sex {
		case gedcom.SexMale:
			s.Males++
		case gedcom.SexFemale:
			s.Females++
		default:
			s.OtherSex++
		}
		n := ind.Name()
		if n.Surname != "" {
			surnames[n.FullSurname()]++
		}
		if f := strings.Fields(n.Given); len(f) > 0 {
			given[f[0]]++
		}
		if y, ok := birthYear(ind); ok {
			births[y]++
		}
		if age, ok := gedcom.AgeBetween(ind.BirthDate(), ind.DeathDate()); ok {
			deaths[age.Years]++
			s.Lifespans++
			totalAge += age.Years
			if s.LongestLived == nil || age.Years > s.LongestAge.Years {
				s.LongestLived, s.LongestAge = ind, age
			}
		}
	}
	for _, e := range doc.Events() {
		s.Events++
		if l := e.Place.Location; l != nil {
			places["\x00"+l.ID] = true
		} else if p := e.Place.String(); p != "" {
			places[strings.ToLower(p)] = true
		}
		if lo, hi, ok := e.Date.YearRange(); ok {
			if s.EarliestYear == 0 || lo < s.EarliestYear {
				s.EarliestYear = lo
			}
			if s.LatestYear == 0 || hi > s.LatestYear {
				s.LatestYear = hi
			}
		}
	}
	s.Places = len(places)
	if s.Lifespans > 0 {
		s.AverageLifespan = float64(totalAge) / float64(s.Lifespans)
	}
	s.Surnames = sortCounts(surnames)
	s.GivenNames = sortCounts(given)
	for _, f := range doc.Families {
		sizes[len(f.Children)]++
		m := f.Marriage()
		if m == nil {
			continue
		}
		for _, p := range f.Partners() {
			if age, ok := gedcom.AgeBetween(p.BirthDate(), m.Date); ok {
				marriages[age.Years]++
			}
		}
	}
	s.BirthYears, s.DeathAges = distribution(births), distribution(deaths)
	s.MarriageAges, s.FamilySizes = distribution(marriages), distribution(sizes)
	s.Generations = generations(doc)
	pedigreeCollapse(doc, &s)
	return s
}

// birthYear returns the year of birth (or baptism), the middle one of a
// range; open-ended dates and periods give none.
func birthYear(ind *gedcom.Individual) (int, bool) {
	d := ind.BirthDate()
	switch d.Modifier {
	case gedcom.DateBefore, gedcom.DateAfter, gedcom.DateFrom, gedcom.DateTo, gedcom.DateFromTo:
		return 0, false
	}
	lo, hi, ok := d.YearRange()
	return lo + (hi-lo)/2, ok
}

// CollapsedPeople says how many people have an ancestor more than once.
func (s Stats) CollapsedPeople() string {
	switch s.Collapsed {
	case 0:
		return "no one has an ancestor more than once"
	case 1:
		return "1 person has ancestors more than once"
	}
	return fmt.Sprintf("%d people have ancestors more than once", s.Collapsed)
}

// pedigreeCollapse counts the ancestor positions and the distinct
// ancestors in every pedigree, following birth links (see Stats). Paths
// are not walked one by one, as their number grows exponentially where
// ancestry collapses.
func pedigreeCollapse(doc *gedcom.Document, s *Stats) {
	n := len(doc.Individuals)
	index := make(map[*gedcom.Individual]int, n)
	for i, ind := range doc.Individuals {
		index[ind] = i
	}
	parents := make([][]int, n)
	for i, ind := range doc.Individuals {
		for _, pl := range ind.ParentLinks() {
			if j, ok := index[pl.Parent]; ok && pl.Pedigree.IsBirth() {
				parents[i] = append(parents[i], j)
			}
		}
	}

	// A person's positions are their parents and the parents' positions.
	// A loop in the data is cut where it is found.
	const (
		unvisited = iota
		inProgress
		done
	)
	positions := make([]float64, n)
	state := make([]int8, n)
	var visit func(i int)
	visit = func(i int) {
		state[i] = inProgress
		for _, p := range parents[i] {
			switch state[p] {
			case unvisited:
				visit(p)
			case inProgress:
				continue
			}
			positions[i] += 1 + positions[p]
		}
		state[i] = done
	}

	// The distinct ancestors: each pedigree marks the people it reached.
	seen := make([]int, n)
	var stack []int
	for i := range n {
		if state[i] == unvisited {
			visit(i)
		}
		mark := i + 1
		seen[i] = mark
		count := 0
		stack = append(stack[:0], parents[i]...)
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[p] == mark {
				continue
			}
			seen[p] = mark
			count++
			stack = append(stack, parents[p]...)
		}
		// A loop can hide positions; every ancestor takes one at least.
		pos := max(positions[i], float64(count))
		s.AncestorPositions += pos
		s.DistinctAncestors += count
		if pos > float64(count) {
			s.Collapsed++
		}
	}
}

func sortCounts(m map[string]int) []NameCount {
	out := make([]NameCount, 0, len(m))
	for name, n := range m {
		out = append(out, NameCount{name, n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// generations returns the number of people in the longest chain of
// parent-child links. Ancestry loops are cut where they are detected.
func generations(doc *gedcom.Document) int {
	depth := make(map[*gedcom.Individual]int, len(doc.Individuals))
	const inProgress = -1
	var visit func(*gedcom.Individual) int
	visit = func(ind *gedcom.Individual) int {
		switch d := depth[ind]; {
		case d == inProgress:
			return 0
		case d > 0:
			return d
		}
		depth[ind] = inProgress
		best := 0
		for _, p := range ind.Parents() {
			best = max(best, visit(p))
		}
		depth[ind] = best + 1
		return best + 1
	}
	longest := 0
	for _, ind := range doc.Individuals {
		longest = max(longest, visit(ind))
	}
	return longest
}
