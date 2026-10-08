package genealogy

import (
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
	Places      int // distinct place names
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
		if p := e.Place.String(); p != "" {
			places[p] = true
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
	return s
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
