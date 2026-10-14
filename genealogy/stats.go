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
		if age, ok := gedcom.AgeBetween(ind.BirthDate(), ind.DeathDate()); ok {
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
	s.Generations = generations(doc)
	pedigreeCollapse(doc, &s)
	return s
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
