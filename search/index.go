package search

import (
	"sort"
	"strings"

	"github.com/efi/goged/gedcom"
)

// Result is a matching individual with its relevance score.
type Result struct {
	Individual *gedcom.Individual
	Score      int
	// NameIndex is the index (into Individual.Names) of the name that
	// matched the query's name terms best; 0 is the primary name.
	NameIndex int
}

type foldedName struct {
	full      string     // "john william smith"
	givenFull string     // "john william"
	given     []string   // ["john", "william"]
	surname   string     // "van der berg"
	words     []string   // all words of the name
	sounds    []phonetic // phonetic codes of the words
}

// phonetic holds the codes that must agree for names to sound alike.
type phonetic struct {
	soundex, cologne string
}

type eventSpan struct {
	lo, hi int
	ok     bool
	place  string
}

type entry struct {
	ind         *gedcom.Individual
	id          string
	names       []foldedName
	birth       eventSpan
	death       eventSpan
	hasBirth    bool
	hasDeath    bool
	events      []eventSpan
	occupations string
	notes       string
	sources     string
	text        string // all values of the record, for any:
	sortKey     string
}

// Index holds pre-folded search data for every individual of a document.
type Index struct {
	doc     *gedcom.Document
	entries []*entry // sorted alphabetically by surname, given name
}

// NewIndex builds the search index for doc.
func NewIndex(doc *gedcom.Document) *Index {
	ix := &Index{doc: doc, entries: make([]*entry, 0, len(doc.Individuals))}
	for _, ind := range doc.Individuals {
		ix.entries = append(ix.entries, newEntry(ind))
	}
	sort.SliceStable(ix.entries, func(i, j int) bool { return ix.entries[i].sortKey < ix.entries[j].sortKey })
	return ix
}

// Len returns the number of indexed individuals.
func (ix *Index) Len() int { return len(ix.entries) }

// eventYears returns the year span an event date may denote, widened by two
// years for approximate dates. Dates before or after a year count for that
// year only, unless openEnded widens them by ten years in the open
// direction: someone born before 1855 may well have been born in 1850, but
// a residence after 1996 is no evidence for the year 2005.
func eventYears(d gedcom.Date, openEnded bool) (lo, hi int, ok bool) {
	lo, hi, ok = d.YearRange()
	if !ok {
		return 0, 0, false
	}
	switch d.Modifier {
	case gedcom.DateAbout, gedcom.DateCalculated, gedcom.DateEstimated:
		lo, hi = lo-2, hi+2
	case gedcom.DateBefore:
		if openEnded {
			lo = hi - 10
		}
	case gedcom.DateAfter:
		if openEnded {
			hi = lo + 10
		}
	}
	return lo, hi, true
}

func spanOf(e *gedcom.Event, openEnded bool) eventSpan {
	if e == nil {
		return eventSpan{}
	}
	s := eventSpan{place: Fold(strings.Join(e.Place.Names(), "\n"))}
	s.lo, s.hi, s.ok = eventYears(e.Date, openEnded)
	return s
}

func newEntry(ind *gedcom.Individual) *entry {
	e := &entry{ind: ind, id: Fold(ind.ID)}
	for _, n := range ind.Names {
		fn := foldedName{
			full:      Fold(collapse(n.Prefix + " " + n.Given + " " + n.FullSurname() + " " + n.Suffix + " " + n.Nickname)),
			givenFull: Fold(collapse(n.Given)),
			surname:   Fold(n.FullSurname()),
			given:     words(Fold(n.Given)),
		}
		fn.words = words(fn.full)
		for _, w := range fn.words {
			if sx := Soundex(w); sx != "" {
				fn.sounds = append(fn.sounds, phonetic{sx, Cologne(w)})
			}
		}
		e.names = append(e.names, fn)
	}

	if b := ind.FirstDatedEvent(gedcom.BirthTags...); b != nil {
		e.birth, e.hasBirth = spanOf(b, true), true
	}
	if d := ind.FirstDatedEvent(gedcom.DeathTags...); d != nil {
		e.death, e.hasDeath = spanOf(d, true), true
	}

	var occ, notes, srcs []string
	for _, ev := range ind.Events {
		e.events = append(e.events, spanOf(ev, false))
		if ev.Tag == "OCCU" {
			occ = append(occ, ev.Value)
		}
		notes = append(notes, ev.Notes...)
	}
	for _, f := range ind.FamiliesAsSpouse() {
		for _, ev := range f.Events {
			e.events = append(e.events, spanOf(ev, false))
		}
	}
	notes = append(notes, ind.Notes()...)
	for _, c := range ind.Citations() {
		srcs = append(srcs, c.Text, c.Page)
	}
	e.occupations = Fold(strings.Join(occ, "\n"))
	e.notes = Fold(strings.Join(notes, "\n"))
	e.sources = Fold(strings.Join(srcs, "\n"))

	var text []string
	ind.Node.Walk(func(n *gedcom.Node) bool {
		if n.Value != "" && !n.IsPointer() {
			text = append(text, n.Value)
		}
		return true
	})
	e.text = Fold(strings.Join(text, "\n")) + "\n" + e.notes + "\n" + e.sources

	n := ind.Name()
	e.sortKey = Fold(n.SortSurname()) + "\x00" + Fold(n.Given) + "\x00" + Fold(n.Suffix) + "\x00" + sortableYear(ind) + "\x00" + ind.ID
	return e
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// sortableYear renders the birth year so that it sorts lexically; people
// without a known birth sort last.
func sortableYear(ind *gedcom.Individual) string {
	k, ok := ind.BirthDate().Key()
	if !ok {
		return "~"
	}
	s := make([]byte, 12)
	k += 1 << 30 // keep positive
	for i := len(s) - 1; i >= 0; i-- {
		s[i] = byte('0' + k%10)
		k /= 10
	}
	return string(s)
}

// Search returns the individuals matching every term of q, best matches
// first; equally good matches are ordered by name.
func (ix *Index) Search(q Query) []Result {
	results := make([]Result, 0, 64)
	for _, e := range ix.entries {
		score, name, ok := ix.match(e, q)
		if ok {
			results = append(results, Result{Individual: e.ind, Score: score, NameIndex: name})
		}
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results
}

// SearchString parses and runs a query.
func (ix *Index) SearchString(s string) ([]Result, error) {
	q, err := Parse(s)
	if err != nil {
		return nil, err
	}
	return ix.Search(q), nil
}

// match scores an entry against all terms. It also reports which name
// matched the first positive name term.
func (ix *Index) match(e *entry, q Query) (score, name int, ok bool) {
	nameSet := false
	for i := range q.Terms {
		t := &q.Terms[i]
		s, idx := ix.matchTerm(e, t)
		if t.Negate {
			if s > 0 {
				return 0, 0, false
			}
			continue
		}
		if s == 0 {
			return 0, 0, false
		}
		if idx >= 0 && !nameSet {
			name, nameSet = idx, true
		}
		score += s
	}
	return score, name, true
}

// Scores for name matches.
const (
	scoreSubstring = 1
	scorePrefix    = 2
	scoreWord      = 3
	scoreID        = 10
)

// matchWords scores how well needle matches a list of words.
func matchWords(needle string, list []string) int {
	best := 0
	for _, w := range list {
		switch {
		case w == needle:
			return scoreWord
		case strings.HasPrefix(w, needle):
			best = max(best, scorePrefix)
		case len(needle) >= 3 && strings.Contains(w, needle):
			best = max(best, scoreSubstring)
		}
	}
	return best
}

// matchNameWords scores a name term typed without spaces or hyphens
// against the words of a name. The term is split into words like the name
// itself, so "o'brien" is the word "obrien"; if punctuation splits it into
// several words, as in "st.clair", every one of them must match.
func matchNameWords(needle, list []string) int {
	if len(needle) == 0 {
		return 0
	}
	score := scoreWord
	for _, w := range needle {
		score = min(score, matchWords(w, list))
	}
	return score
}

// bestName applies score to every name and returns the best score and the
// index of the (first) name achieving it.
func (e *entry) bestName(score func(n *foldedName) int) (int, int) {
	best, idx := 0, -1
	for i := range e.names {
		if s := score(&e.names[i]); s > best {
			best, idx = s, i
		}
	}
	return best, idx
}

// isPhrase reports whether a name term contains spaces or hyphens; such
// terms are compared with a whole name or name part (see matchPhrase).
func isPhrase(t *Term) bool { return strings.ContainsAny(t.folded, " -") }

// matchPhrase scores a phrase term: it may be all of text or a part of it.
func matchPhrase(needle, text string) int {
	switch {
	case text == needle:
		return scoreWord
	case strings.Contains(text, needle):
		return scorePrefix
	}
	return 0
}

func (e *entry) matchName(t *Term) (int, int) {
	return e.bestName(func(n *foldedName) int {
		if isPhrase(t) {
			return matchPhrase(t.folded, n.full)
		}
		return matchNameWords(t.words, n.words)
	})
}

func (e *entry) matchGiven(t *Term) (int, int) {
	return e.bestName(func(n *foldedName) int {
		if isPhrase(t) {
			return matchPhrase(t.folded, n.givenFull)
		}
		return matchNameWords(t.words, n.given)
	})
}

func (e *entry) matchSurname(t *Term) (int, int) {
	return e.bestName(func(n *foldedName) int {
		if n.surname == t.folded || isPhrase(t) {
			return matchPhrase(t.folded, n.surname)
		}
		return matchNameWords(t.words, words(n.surname))
	})
}

func boolScore(b bool) int {
	if b {
		return 1
	}
	return 0
}

// matchVital matches born:/died: terms against a birth or death event.
func matchVital(t *Term, has bool, s eventSpan) bool {
	if !has {
		return false
	}
	if t.isYears {
		return s.ok && t.years.intersects(s.lo, s.hi)
	}
	return strings.Contains(s.place, t.folded)
}

// matchTerm scores one term. For name terms it also returns the index of
// the best matching name, else -1.
func (ix *Index) matchTerm(e *entry, t *Term) (int, int) {
	switch t.Field {
	case FieldText:
		if t.folded == e.id {
			return scoreID, -1
		}
		if t.isYears {
			return boolScore(matchVital(t, e.hasBirth, e.birth) || matchVital(t, e.hasDeath, e.death)), -1
		}
		return e.matchName(t)
	case FieldName:
		return e.matchName(t)
	case FieldGiven:
		return e.matchGiven(t)
	case FieldSurname:
		return e.matchSurname(t)
	case FieldSounds:
		// Soundex alone confuses e.g. Mustermann and Musterow (M236); the
		// Cologne phonetics, made for German names, tell them apart.
		return e.bestName(func(n *foldedName) int {
			for _, p := range n.sounds {
				if p.soundex == t.soundex && p.cologne == t.cologne {
					return 1
				}
			}
			return 0
		})
	}
	return ix.matchField(e, t), -1
}

// matchField scores terms that do not concern names.
func (ix *Index) matchField(e *entry, t *Term) int {
	switch t.Field {
	case FieldBorn:
		return boolScore(matchVital(t, e.hasBirth, e.birth))
	case FieldDied:
		return boolScore(matchVital(t, e.hasDeath, e.death))
	case FieldPlace:
		for _, s := range e.events {
			if strings.Contains(s.place, t.folded) {
				return 1
			}
		}
		return 0
	case FieldYear:
		for _, s := range e.events {
			if s.ok && t.years.intersects(s.lo, s.hi) {
				return 1
			}
		}
		return 0
	case FieldAlive:
		return boolScore(e.aliveDuring(t.years))
	case FieldSex:
		return boolScore(string(e.ind.Sex) == t.sex)
	case FieldID:
		return boolScore(e.id == t.folded)
	case FieldOccupation:
		return boolScore(strings.Contains(e.occupations, t.folded))
	case FieldNote:
		return boolScore(strings.Contains(e.notes, t.folded))
	case FieldSource:
		return boolScore(strings.Contains(e.sources, t.folded))
	case FieldAny:
		return boolScore(strings.Contains(e.text, t.folded))
	case FieldTag:
		return boolScore(matchTagPath(e.ind.Node, t.tagPath, t.tagVal))
	case FieldHas:
		return boolScore(e.has(t.folded))
	}
	return 0
}

// aliveDuring reports whether the person may have been alive at some point
// in the year range, assuming a lifespan of at most 100 years when only one
// end of the life is known.
func (e *entry) aliveDuring(r yearRange) bool {
	const maxAge = 100
	switch {
	case e.birth.ok && e.death.ok:
		return r.intersects(e.birth.lo, e.death.hi)
	case e.birth.ok:
		return r.intersects(e.birth.lo, e.birth.hi+maxAge)
	case e.death.ok:
		return r.intersects(e.death.lo-maxAge, e.death.hi)
	}
	return false
}

func (e *entry) has(what string) bool {
	ind := e.ind
	switch what {
	case "birth":
		return ind.FirstEvent(gedcom.BirthTags...) != nil
	case "death":
		return ind.IsDeceased()
	case "parents":
		return len(ind.Parents()) > 0
	case "father":
		return ind.Father() != nil
	case "mother":
		return ind.Mother() != nil
	case "spouse":
		return len(ind.Spouses()) > 0
	case "children":
		return len(ind.Children()) > 0
	case "siblings":
		return len(ind.Siblings()) > 0 || len(ind.HalfSiblings()) > 0
	case "notes":
		return e.notes != ""
	case "sources":
		return len(ind.Citations()) > 0
	case "media":
		found := false
		ind.Node.Walk(func(n *gedcom.Node) bool {
			if n.Tag == "OBJE" {
				found = true
			}
			return !found
		})
		return found
	case "occupation":
		return len(ind.EventsWithTag("OCCU")) > 0
	}
	return false
}

// matchTagPath reports whether a chain of tags exists below n (at any depth
// for the first tag) and, if val is non-empty, whether the last node's
// value contains it.
func matchTagPath(n *gedcom.Node, path []string, val string) bool {
	found := false
	n.Walk(func(c *gedcom.Node) bool {
		if found {
			return false
		}
		if c != n && c.Tag == path[0] && followPath(c, path[1:], val) {
			found = true
			return false
		}
		return true
	})
	return found
}

func followPath(n *gedcom.Node, rest []string, val string) bool {
	if len(rest) == 0 {
		return val == "" || strings.Contains(Fold(n.Value), val)
	}
	for _, c := range n.Children {
		if c.Tag == rest[0] && followPath(c, rest[1:], val) {
			return true
		}
	}
	return false
}
