package genealogy

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/search"
)

// The hints of Checks: gaps in the research and places that may be the
// same.

// missingParents reports the people within the threshold in generations
// of the marked person (the marked person included) whose father or
// mother is unknown, generation by generation. People who appear in more
// than one place are reported once, at the nearest.
func missingParents(_ *gedcom.Document, gens int, ctx Context) []Finding {
	if ctx.Marked == nil {
		return nil
	}
	var out []Finding
	seen := map[*gedcom.Individual]bool{ctx.Marked: true}
	level := []*gedcom.Individual{ctx.Marked}
	for g := 0; g < gens && len(level) > 0; g++ {
		var next []*gedcom.Individual
		for _, ind := range level {
			parents := ind.BirthParents()
			if missing := unknownParents(parents); missing != "" {
				who := "the marked person"
				if g > 0 {
					who = "the marked person's " + describe(g, 0, ind.Sex, false)
				}
				out = append(out, Finding{Person: ind, Message: missing + " · " + who})
			}
			for _, p := range parents {
				if !seen[p] {
					seen[p] = true
					next = append(next, p)
				}
			}
		}
		level = next
	}
	return out
}

// unknownParents says which of a person's parents are unknown, or "".
func unknownParents(parents []*gedcom.Individual) string {
	switch {
	case len(parents) == 0:
		return "parents unknown"
	case len(parents) > 1:
		return ""
	case parents[0].Sex == gedcom.SexMale:
		return "mother unknown"
	case parents[0].Sex == gedcom.SexFemale:
		return "father unknown"
	}
	return "one parent unknown"
}

// completeness tells how many places in each generation of the marked
// person's ancestry are known: "parents 2 of 2 · grandparents 3 of 4".
func completeness(_ *gedcom.Document, gens int, ctx Context) string {
	if ctx.Marked == nil {
		return "nobody is marked: mark a person to see the gaps in their ancestry"
	}
	var parts []string
	level := []*gedcom.Individual{ctx.Marked}
	for g := 1; g <= gens; g++ {
		var next []*gedcom.Individual
		for _, ind := range level {
			next = append(next, ind.BirthParents()...)
		}
		parts = append(parts, fmt.Sprintf("%ss %d of %d", describe(g, 0, gedcom.SexUnknown, false), len(next), 1<<g))
		level = next
	}
	return "known: " + strings.Join(parts, " · ")
}

func noBirthDate(doc *gedcom.Document, _ int, _ Context) []Finding {
	var out []Finding
	for _, ind := range doc.Individuals {
		if birthOf(ind) != nil {
			continue
		}
		msg := "no birth or baptism recorded"
		if ind.FirstEvent(gedcom.BirthTags...) != nil {
			msg = "birth or baptism recorded without a date"
		}
		out = append(out, Finding{Person: ind, Message: msg})
	}
	return out
}

func noBirthPlace(doc *gedcom.Document, _ int, _ Context) []Finding {
	var out []Finding
	for _, ind := range doc.Individuals {
		found := false
		for _, tag := range gedcom.BirthTags {
			for _, e := range ind.EventsWithTag(tag) {
				found = found || e.Place.String() != ""
			}
		}
		if !found {
			out = append(out, Finding{Person: ind, Message: "no place of birth or baptism recorded"})
		}
	}
	return out
}

// noDeath reports people born more than the threshold in years ago for
// whom no death, burial or cremation is recorded.
func noDeath(doc *gedcom.Document, years int, ctx Context) []Finding {
	t := ctx.today()
	today := gedcom.ParseDate(strings.ToUpper(t.Format("2 Jan 2006")))
	var out []Finding
	for _, ind := range doc.Individuals {
		b := birthOf(ind)
		if b == nil || ind.IsDeceased() {
			continue
		}
		if least, _, ok := ages(b.Date, today); ok && least > float64(years) {
			out = append(out, Finding{Person: ind, Message: fmt.Sprintf("%s, at least %.0f years ago", what(b), least)})
		}
	}
	return out
}

func noEvents(doc *gedcom.Document, _ int, _ Context) []Finding {
	var out []Finding
	for _, ind := range doc.Individuals {
		if len(ind.Events) > 0 || slices.ContainsFunc(ind.FamiliesAsSpouse(), func(f *gedcom.Family) bool { return len(f.Events) > 0 }) {
			continue
		}
		out = append(out, Finding{Person: ind, Message: "nothing recorded but the name and the family links"})
	}
	return out
}

// noMarriageDate reports couples who are married, as a marriage or a
// divorce is recorded, without a dated marriage.
func noMarriageDate(doc *gedcom.Document, _ int, _ Context) []Finding {
	var out []Finding
	for _, f := range doc.Families {
		var marriages []*gedcom.Event
		for _, e := range f.Events {
			if e.Tag == "MARR" {
				marriages = append(marriages, e)
			}
		}
		divorce := f.FirstEvent("DIV", "ANUL")
		if (len(marriages) == 0 && divorce == nil) || slices.ContainsFunc(marriages, dated) {
			continue
		}
		partners := f.Partners()
		if len(partners) == 0 {
			continue
		}
		msg := "married, without a date"
		if len(marriages) == 0 {
			msg = strings.ToLower(divorce.Label()) + " recorded, but no marriage"
		}
		fn := Finding{Person: partners[0], Message: msg}
		if len(partners) > 1 {
			fn.Other = partners[1]
			fn.Message += ", with " + partners[1].DisplayName()
		}
		out = append(out, fn)
	}
	return out
}

// sourcedTags are the events that unsourced looks at.
var sourcedTags = map[string]bool{"BIRT": true, "CHR": true, "BAPM": true, "MARR": true, "DEAT": true, "BURI": true, "CREM": true}

// unsourced reports births, baptisms, marriages, deaths and burials that
// cite no source, neither themselves nor in their record as a whole (a
// SOUR directly in the record).
func unsourced(doc *gedcom.Document, _ int, _ Context) []Finding {
	var out []Finding
	report := func(e *gedcom.Event, record *gedcom.Node, person, other *gedcom.Individual) {
		if !sourcedTags[e.Tag] || cites(record, false) || cites(e.Node, true) {
			return
		}
		msg := strings.ToLower(e.Label())
		if e.Date.IsValid() {
			msg = what(e)
		}
		if other != nil {
			msg += " with " + other.DisplayName()
		}
		out = append(out, Finding{Person: person, Other: other, Message: msg})
	}
	for _, ind := range doc.Individuals {
		for _, e := range ind.Events {
			report(e, ind.Node, ind, nil)
		}
	}
	for _, f := range doc.Families {
		partners := f.Partners()
		if len(partners) == 0 {
			continue
		}
		for _, e := range f.Events {
			report(e, f.Node, partners[0], f.Partner(partners[0]))
		}
	}
	return out
}

// cites reports whether n has a SOUR below it: anywhere, or with deep
// false as a direct child.
func cites(n *gedcom.Node, deep bool) bool {
	if n == nil {
		return false
	}
	for _, c := range n.Children {
		if c.Tag == "SOUR" || (deep && cites(c, true)) {
			return true
		}
	}
	return false
}

// Levels of similarPlaces.
const (
	almostIdentical = iota
	similar
	resembling
)

// similarPlaces reports groups of places within the same place whose
// names may be spellings of one name, at the level of the threshold:
//
//   - almost identical: the same but for case, diacritics, umlauts written
//     out (Koeln, Köln), punctuation and spaces, and St, Sankt or Saint;
//   - similar: also names that sound alike (as ~name in the search), that
//     differ by one letter, or of which one abbreviates the other word by
//     word (Yorks, Yorkshire);
//   - somewhat resembling: also names that differ by two letters, or by
//     a quarter of the letters, that sound alike by one measure, or of
//     which one begins with the other (Berlin, Berlin (West)).
//
// A group holds places that are linked by such resemblances, one after
// the other. The names in a group and the groups are sorted
// alphabetically.
func similarPlaces(doc *gedcom.Document, level int, _ Context) []Finding {
	var out []Finding
	Places(doc).Walk(func(n *PlaceNode) bool {
		for _, group := range groupSimilar(n.Children, level) {
			names := make([]string, len(group))
			for i, p := range group {
				names[i] = p.Name
			}
			msg := strings.Join(names, " · ")
			if n.Depth > 0 {
				msg += " (in " + n.Full + ")"
			}
			out = append(out, Finding{Places: group, Message: msg})
		}
		return true
	})
	slices.SortStableFunc(out, func(a, b Finding) int {
		return strings.Compare(search.Fold(a.Places[0].Name), search.Fold(b.Places[0].Name))
	})
	return out
}

// groupSimilar returns the groups of more than one place among places.
func groupSimilar(places []*PlaceNode, level int) [][]*PlaceNode {
	if len(places) < 2 {
		return nil
	}
	names := make([]placeName, len(places))
	for i, p := range places {
		names[i] = newPlaceName(p.Name)
	}
	group := make([]int, len(places)) // union-find
	for i := range group {
		group[i] = i
	}
	find := func(i int) int {
		for group[i] != i {
			group[i] = group[group[i]]
			i = group[i]
		}
		return i
	}
	for i := range places {
		for j := i + 1; j < len(places); j++ {
			if alike(names[i], names[j], level) {
				group[find(i)] = find(j)
			}
		}
	}
	members := map[int][]*PlaceNode{}
	var roots []int
	for i, p := range places {
		r := find(i)
		if _, ok := members[r]; !ok {
			roots = append(roots, r)
		}
		members[r] = append(members[r], p)
	}
	var out [][]*PlaceNode
	for _, r := range roots {
		if g := members[r]; len(g) > 1 {
			slices.SortFunc(g, func(a, b *PlaceNode) int { return strings.Compare(search.Fold(a.Name), search.Fold(b.Name)) })
			out = append(out, g)
		}
	}
	return out
}

// placeName is a place name reduced for comparison: its words folded,
// with umlauts written out taken as plain vowels and Sankt or Saint as
// St; the key joins them.
type placeName struct {
	words            []string
	key              string
	dotted           bool // the name abbreviates with a dot
	soundex, cologne string
}

var vowels = strings.NewReplacer("ae", "a", "oe", "o", "ue", "u", "ss", "s")

func newPlaceName(name string) placeName {
	words := strings.FieldsFunc(search.Fold(name), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for i, w := range words {
		if w == "sankt" || w == "saint" {
			w = "st"
		}
		words[i] = vowels.Replace(w)
	}
	key := strings.Join(words, "")
	return placeName{words, key, strings.Contains(name, "."), search.Soundex(key), search.Cologne(key)}
}

// alike tells whether two place names resemble each other at the level.
func alike(a, b placeName, level int) bool {
	if a.key == b.key && a.key != "" {
		return true
	}
	if level < similar || a.key == "" || b.key == "" {
		return false
	}
	d := distance(a.key, b.key)
	short := min(len([]rune(a.key)), len([]rune(b.key)))
	sameSoundex, sameCologne := a.soundex == b.soundex, a.cologne == b.cologne
	switch {
	case sameSoundex && sameCologne:
		return true
	case d == 1 && short >= 4:
		return true
	case abbreviates(a, b) || abbreviates(b, a):
		return true
	case level < resembling:
		return false
	}
	begins := short >= 4 && (strings.HasPrefix(a.key, b.key) || strings.HasPrefix(b.key, a.key))
	return begins || sameSoundex || sameCologne || (d <= 2 && short >= 5) || (short >= 8 && d*4 <= short)
}

// abbreviates tells whether a abbreviates b word by word: as many words,
// each the same or the beginning of the other, of at least four letters
// unless the name has a dot.
func abbreviates(a, b placeName) bool {
	if len(a.words) != len(b.words) || len(a.key) >= len(b.key) {
		return false
	}
	for i, w := range a.words {
		if w != b.words[i] && (!strings.HasPrefix(b.words[i], w) || (len([]rune(w)) < 4 && !a.dotted)) {
			return false
		}
	}
	return true
}

// distance is the Levenshtein distance of two strings, in runes.
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
