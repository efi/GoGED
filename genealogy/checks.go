package genealogy

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/search"
)

// Check is a plausibility check: a rule that dates and relationships
// usually follow. A hint (Hint) reports a gap or a likely duplicate
// instead. A check may have a threshold, whose default is Param and which
// may be changed between Min and Max.
type Check struct {
	ID string
	// Title describes what is reported; %d stands for the threshold, %s
	// for its level.
	Title string
	// Param is the default threshold, Min and Max its range; all are 0 for
	// checks without one.
	Param, Min, Max int
	Unit            string // of the threshold: "years" or "months"
	// Levels name the steps of a threshold that is not a number, from the
	// strictest; Param, Min and Max index them.
	Levels []string
	Hint   bool

	run func(doc *gedcom.Document, param int, ctx Context) []Finding
	// summary describes the result as a whole, if set.
	summary func(doc *gedcom.Document, param int, ctx Context) string
	ordered bool // the findings keep the order of run
	marked  bool // the check depends on the marked person
}

// Context is what some checks need besides the file.
type Context struct {
	Marked *gedcom.Individual // the reference person of the browser
	Today  time.Time          // the current date; zero means now
}

func (ctx Context) today() time.Time {
	if ctx.Today.IsZero() {
		return time.Now()
	}
	return ctx.Today
}

// Finding is a doubtful fact found by a check.
type Finding struct {
	Person *gedcom.Individual // the person concerned, or nil
	Other  *gedcom.Individual // a second person involved, or nil
	// Places are the places concerned, for hints on place names.
	Places  []*PlaceNode
	Message string
}

// HasParam reports whether the check has a threshold.
func (c Check) HasParam() bool { return c.Max > 0 }

// UsesMarked reports whether the result depends on the marked person.
func (c Check) UsesMarked() bool { return c.marked }

// Name returns the title with the threshold filled in.
func (c Check) Name(param int) string {
	switch {
	case !c.HasParam():
		return c.Title
	case c.Levels != nil:
		return fmt.Sprintf(c.Title, c.Levels[param])
	}
	return fmt.Sprintf(c.Title, param)
}

// Value returns the threshold as shown: a number or a level.
func (c Check) Value(param int) string {
	if c.Levels != nil {
		return c.Levels[param]
	}
	return fmt.Sprint(param)
}

// Run applies the check with the threshold param. The findings are sorted
// by the name of the person concerned, unless the check orders them.
func (c Check) Run(doc *gedcom.Document, param int, ctx Context) []Finding {
	out := c.run(doc, param, ctx)
	if !c.ordered {
		slices.SortStableFunc(out, func(a, b Finding) int {
			return strings.Compare(search.Fold(a.Person.SortName()), search.Fold(b.Person.SortName()))
		})
	}
	return out
}

// Summary describes the result as a whole; most checks have none.
func (c Check) Summary(doc *gedcom.Document, param int, ctx Context) string {
	if c.summary == nil {
		return ""
	}
	return c.summary(doc, param, ctx)
}

// Checks lists the plausibility checks in the order they are shown.
//
// Dates are often imprecise, so a check only reports what is certain
// within the range a date may denote: "before 1850" may be any time
// before, "after 1850" any time after, and "about", "calculated" and
// "estimated" dates may be off by two years. Ages of parents only count
// for birth (or unspecified) links, not for adoptive or foster parents.
var Checks = []Check{
	{ID: "born-after-death", Title: "Born after death", run: bornAfterDeath},
	{ID: "vital-twice", Title: "Born, christened, died or buried more than once, on different days", run: vitalTwice},
	{ID: "event-before-birth", Title: "Events before birth", run: eventsBeforeBirth},
	{ID: "event-after-death", Title: "Events after death (other than burial, cremation and probate)", run: eventsAfterDeath},
	{ID: "old-age", Title: "Lived longer than %d years", Param: 110, Min: 80, Max: 150, Unit: "years", run: oldAge},
	{ID: "mother-young", Title: "Mother younger than %d at a child's birth", Param: 12, Min: 5, Max: 30, Unit: "years", run: parentAge(gedcom.SexFemale, false)},
	{ID: "mother-old", Title: "Mother older than %d at a child's birth", Param: 55, Min: 35, Max: 80, Unit: "years", run: parentAge(gedcom.SexFemale, true)},
	{ID: "father-young", Title: "Father younger than %d at a child's birth", Param: 13, Min: 5, Max: 30, Unit: "years", run: parentAge(gedcom.SexMale, false)},
	{ID: "father-old", Title: "Father older than %d at a child's birth", Param: 80, Min: 40, Max: 110, Unit: "years", run: parentAge(gedcom.SexMale, true)},
	{ID: "born-after-mother-death", Title: "Born after the mother's death", run: bornAfterParentDeath(gedcom.SexFemale)},
	{ID: "born-after-father-death", Title: "Born more than %d months after the father's death", Param: 10, Min: 1, Max: 24, Unit: "months", run: bornAfterParentDeath(gedcom.SexMale)},
	{ID: "siblings-close", Title: "Born less than %d months after a sibling (twins excepted)", Param: 9, Min: 1, Max: 24, Unit: "months", run: siblingsClose},
	{ID: "married-young", Title: "Married younger than %d", Param: 14, Min: 5, Max: 30, Unit: "years", run: marriedYoung},
	{ID: "duplicates", Title: "Possibly the same person: names sound alike, born at most %d years apart", Param: 2, Min: 0, Max: 20, Unit: "years", run: duplicates},

	{ID: "missing-parents", Title: "Unknown parents within %d generations of the marked person", Param: 4, Min: 1, Max: 12, Unit: "generations",
		Hint: true, run: missingParents, summary: completeness, ordered: true, marked: true},
	{ID: "no-birth-date", Title: "No date of birth or baptism", Hint: true, run: noBirthDate},
	{ID: "no-birth-place", Title: "No place of birth or baptism", Hint: true, run: noBirthPlace},
	{ID: "no-death", Title: "Born more than %d years ago, but no death recorded", Param: 110, Min: 60, Max: 150, Unit: "years", Hint: true, run: noDeath},
	{ID: "no-events", Title: "No events or facts at all", Hint: true, run: noEvents},
	{ID: "no-marriage-date", Title: "Married, but no date of marriage", Hint: true, run: noMarriageDate},
	{ID: "unsourced", Title: "Births, baptisms, marriages, deaths and burials without a source", Hint: true, run: unsourced},
	{ID: "similar-places", Title: "Places that may be the same: %s names", Param: 1, Min: 0, Max: 2,
		Levels: []string{"almost identical", "similar", "somewhat resembling"}, Hint: true, run: similarPlaces, ordered: true},
}

const (
	daysPerYear  = 365.2425
	daysPerMonth = daysPerYear / 12
	slack        = 2 * 365 // days by which approximate dates may be off
	twinDays     = 3       // births this close are twins
)

// bounds returns the earliest and latest day (Julian Day Number) a date
// may denote. Open ends are infinite.
func bounds(d gedcom.Date) (lo, hi float64, ok bool) {
	first, last, ok := d.Span()
	if !ok {
		return 0, 0, false
	}
	lo, hi = float64(first), float64(last)
	switch d.Modifier {
	case gedcom.DateBefore:
		lo = math.Inf(-1)
	case gedcom.DateAfter:
		hi = math.Inf(1)
	case gedcom.DateAbout, gedcom.DateCalculated, gedcom.DateEstimated:
		lo, hi = lo-slack, hi+slack
	}
	return lo, hi, true
}

// between returns the least and the greatest number of days from a to b;
// they are negative where b may come before a.
func between(a, b gedcom.Date) (least, most float64, ok bool) {
	alo, ahi, ok1 := bounds(a)
	blo, bhi, ok2 := bounds(b)
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return blo - ahi, bhi - alo, true
}

// ages returns the least and the greatest age in completed years that
// someone born on date a may have reached on date b.
func ages(a, b gedcom.Date) (least, most float64, ok bool) {
	alo, ahi, ok1 := bounds(a)
	blo, bhi, ok2 := bounds(b)
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return completed(ahi, blo), completed(alo, bhi), true
}

// completed returns the completed years from day a to day b, as people
// count their age.
func completed(a, b float64) float64 {
	if math.IsInf(a, 0) || math.IsInf(b, 0) {
		return b - a // infinite
	}
	y1, m1, d1 := gedcom.JDNToGregorian(int(a))
	y2, m2, d2 := gedcom.JDNToGregorian(int(b))
	n := y2 - y1
	if m2 < m1 || (m2 == m1 && d2 < d1) {
		n--
	}
	return float64(n)
}

// birthOf and deathOf return the dated event that best marks the start
// and the end of a life (birth or baptism, death or burial), or nil.
func birthOf(ind *gedcom.Individual) *gedcom.Event { return datedEvent(ind, gedcom.BirthTags) }
func deathOf(ind *gedcom.Individual) *gedcom.Event { return datedEvent(ind, gedcom.DeathTags) }

func datedEvent(ind *gedcom.Individual, tags []string) *gedcom.Event {
	if e := ind.FirstDatedEvent(tags...); e != nil && e.Date.IsValid() {
		return e
	}
	return nil
}

// what names an event for a message, e.g. "birth 12 Mar 1850".
func what(e *gedcom.Event) string { return strings.ToLower(e.Label()) + " " + e.Date.String() }

// lifeEvents returns the dated events of a person: their own and those of
// their families as a partner, with the partner.
func lifeEvents(ind *gedcom.Individual) (events []*gedcom.Event, partners []*gedcom.Individual) {
	for _, e := range ind.Events {
		if e.Date.IsValid() {
			events, partners = append(events, e), append(partners, nil)
		}
	}
	for _, f := range ind.FamiliesAsSpouse() {
		for _, e := range f.Events {
			if e.Date.IsValid() {
				events, partners = append(events, e), append(partners, f.Partner(ind))
			}
		}
	}
	return events, partners
}

func bornAfterDeath(doc *gedcom.Document, _ int, _ Context) []Finding {
	var out []Finding
	for _, ind := range doc.Individuals {
		b, d := birthOf(ind), deathOf(ind)
		if b == nil || d == nil {
			continue
		}
		if least, _, ok := between(d.Date, b.Date); ok && least > 0 {
			out = append(out, Finding{Person: ind, Message: what(b) + " after " + what(d)})
		}
	}
	return out
}

// vitalTwice reports births, christenings, deaths and burials recorded
// more than once whose dates cannot be the same day.
func vitalTwice(doc *gedcom.Document, _ int, _ Context) []Finding {
	var out []Finding
	for _, ind := range doc.Individuals {
		for _, tag := range []string{"BIRT", "CHR", "BAPM", "DEAT", "BURI", "CREM"} {
			events := ind.EventsWithTag(tag)
			for i, a := range events {
				for _, b := range events[i+1:] {
					if least, most, ok := between(a.Date, b.Date); ok && (least > 0 || most < 0) {
						out = append(out, Finding{Person: ind, Message: what(a) + " and " + what(b)})
					}
				}
			}
		}
	}
	return out
}

func eventsBeforeBirth(doc *gedcom.Document, _ int, _ Context) []Finding {
	var out []Finding
	for _, ind := range doc.Individuals {
		b := birthOf(ind)
		if b == nil {
			continue
		}
		events, partners := lifeEvents(ind)
		for i, e := range events {
			// A birth after death is reported as such.
			if e == b || e.Tag == "BIRT" || slices.Contains(gedcom.DeathTags, e.Tag) {
				continue
			}
			if _, most, ok := between(b.Date, e.Date); ok && most < 0 {
				out = append(out, Finding{Person: ind, Other: partners[i], Message: what(e) + " before " + what(b)})
			}
		}
	}
	return out
}

func eventsAfterDeath(doc *gedcom.Document, _ int, _ Context) []Finding {
	after := map[string]bool{"DEAT": true, "BURI": true, "CREM": true, "PROB": true}
	var out []Finding
	for _, ind := range doc.Individuals {
		d := deathOf(ind)
		if d == nil {
			continue
		}
		events, partners := lifeEvents(ind)
		for i, e := range events {
			if e == d || after[e.Tag] || slices.Contains(gedcom.BirthTags, e.Tag) {
				continue
			}
			if least, _, ok := between(d.Date, e.Date); ok && least > 0 {
				out = append(out, Finding{Person: ind, Other: partners[i], Message: what(e) + " after " + what(d)})
			}
		}
	}
	return out
}

func oldAge(doc *gedcom.Document, limit int, _ Context) []Finding {
	var out []Finding
	for _, ind := range doc.Individuals {
		b, d := birthOf(ind), deathOf(ind)
		if b == nil || d == nil {
			continue
		}
		if least, _, ok := ages(b.Date, d.Date); ok && least > float64(limit) {
			out = append(out, Finding{Person: ind, Message: fmt.Sprintf("lived at least %.0f years: %s, %s", least, what(b), what(d))})
		}
	}
	return out
}

// birthParents calls fn for every child with a dated birth and each of its
// birth parents of the given sex.
func birthParents(doc *gedcom.Document, sex gedcom.Sex, fn func(child, parent *gedcom.Individual, birth *gedcom.Event)) {
	for _, child := range doc.Individuals {
		b := birthOf(child)
		if b == nil {
			continue
		}
		for _, pl := range child.ParentLinks() {
			if pl.Parent.Sex == sex && pl.Pedigree.IsBirth() {
				fn(child, pl.Parent, b)
			}
		}
	}
}

// parentAge reports parents of the sex who were younger (or, with old,
// older) than the threshold at the birth of a child.
func parentAge(sex gedcom.Sex, old bool) func(*gedcom.Document, int, Context) []Finding {
	return func(doc *gedcom.Document, limit int, _ Context) []Finding {
		var out []Finding
		birthParents(doc, sex, func(child, parent *gedcom.Individual, cb *gedcom.Event) {
			pb := birthOf(parent)
			if pb == nil {
				return
			}
			least, most, ok := ages(pb.Date, cb.Date)
			if !ok {
				return
			}
			at := fmt.Sprintf("at the birth of %s (%s)", child.DisplayName(), cb.Date)
			var msg string
			switch {
			case !old && most < 0:
				msg = "born after the child, " + at
			case !old && most < float64(limit):
				msg = fmt.Sprintf("at most %.0f years old %s", most, at)
			case old && least > float64(limit):
				msg = fmt.Sprintf("at least %.0f years old %s", least, at)
			default:
				return
			}
			out = append(out, Finding{Person: parent, Other: child, Message: msg})
		})
		return out
	}
}

// bornAfterParentDeath reports children born after the death of their
// birth parent of the sex, or more than the threshold in months after it.
func bornAfterParentDeath(sex gedcom.Sex) func(*gedcom.Document, int, Context) []Finding {
	return func(doc *gedcom.Document, months int, _ Context) []Finding {
		var out []Finding
		birthParents(doc, sex, func(child, parent *gedcom.Individual, cb *gedcom.Event) {
			pd := deathOf(parent)
			if pd == nil {
				return
			}
			least, _, ok := between(pd.Date, cb.Date)
			if !ok || least <= float64(months)*daysPerMonth {
				return
			}
			role := "mother"
			if sex == gedcom.SexMale {
				role = "father"
			}
			msg := fmt.Sprintf("%s, at least %d days after the %s's %s", what(cb), int(least), role, what(pd))
			out = append(out, Finding{Person: child, Other: parent, Message: msg})
		})
		return out
	}
}

func siblingsClose(doc *gedcom.Document, months int, _ Context) []Finding {
	var out []Finding
	limit := float64(months) * daysPerMonth
	for _, mother := range doc.Individuals {
		if mother.Sex != gedcom.SexFemale {
			continue
		}
		type birth struct {
			child *gedcom.Individual
			event *gedcom.Event
			mid   float64
		}
		var births []birth
		for _, f := range mother.FamiliesAsSpouse() {
			for _, c := range f.Children {
				l, _ := c.ChildLink(f)
				b := birthOf(c)
				if b == nil || !l.Of(mother).IsBirth() || slices.ContainsFunc(births, func(x birth) bool { return x.child == c }) {
					continue
				}
				lo, hi, _ := bounds(b.Date)
				if math.IsInf(lo, 0) || math.IsInf(hi, 0) {
					continue
				}
				births = append(births, birth{c, b, (lo + hi) / 2})
			}
		}
		slices.SortFunc(births, func(a, b birth) int { return int(a.mid - b.mid) })
		for i := 1; i < len(births); i++ {
			a, b := births[i-1], births[i]
			least, most, ok := between(a.event.Date, b.event.Date)
			if ok && least > twinDays && most < limit {
				msg := fmt.Sprintf("%s, at most %d days after %s (%s)", what(b.event), int(most), a.child.DisplayName(), a.event.Date)
				out = append(out, Finding{Person: b.child, Other: a.child, Message: msg})
			}
		}
	}
	return out
}

func marriedYoung(doc *gedcom.Document, limit int, _ Context) []Finding {
	var out []Finding
	for _, f := range doc.Families {
		m := f.Marriage()
		if !dated(m) {
			continue
		}
		for _, p := range f.Partners() {
			b := birthOf(p)
			if b == nil {
				continue
			}
			// Marriages before birth are reported as events before birth.
			if _, most, ok := ages(b.Date, m.Date); ok && most >= 0 && most < float64(limit) {
				msg := fmt.Sprintf("at most %.0f years old at the %s", most, what(m))
				out = append(out, Finding{Person: p, Other: f.Partner(p), Message: msg})
			}
		}
	}
	return out
}

func dated(e *gedcom.Event) bool { return e != nil && e.Date.IsValid() }

// duplicates reports pairs of people whose given names and surnames sound
// alike (as ~name in the search) and who were born at most the threshold
// in years apart. Siblings, who are often named alike, only count if they
// may have been born on the same day. People linked as aliases are
// already known to be the same.
func duplicates(doc *gedcom.Document, limit int, _ Context) []Finding {
	type person struct {
		ind   *gedcom.Individual
		birth *gedcom.Event
	}
	buckets := map[string][]person{}
	var keys []string
	for _, ind := range doc.Individuals {
		b := birthOf(ind)
		n := ind.Name()
		given := strings.Fields(n.Given)
		if b == nil || len(given) == 0 || n.Surname == "" {
			continue
		}
		key := phonetic(given[0]) + "|" + phonetic(n.Surname)
		if _, ok := buckets[key]; !ok {
			keys = append(keys, key)
		}
		buckets[key] = append(buckets[key], person{ind, b})
	}
	var out []Finding
	for _, key := range keys {
		ps := buckets[key]
		for i, a := range ps {
			for _, b := range ps[i+1:] {
				if a.ind.Sex != b.ind.Sex && a.ind.Sex != gedcom.SexUnknown && b.ind.Sex != gedcom.SexUnknown {
					continue
				}
				if slices.Contains(a.ind.Aliases(), b.ind) || slices.Contains(b.ind.Aliases(), a.ind) {
					continue
				}
				least, most, ok := between(a.birth.Date, b.birth.Date)
				if !ok {
					continue
				}
				gap := 0.0 // the smallest possible distance of the births
				if least > 0 {
					gap = least
				} else if most < 0 {
					gap = -most
				}
				siblings := slices.Contains(a.ind.Siblings(), b.ind) || slices.Contains(a.ind.HalfSiblings(), b.ind)
				if (siblings && gap > 0) || gap > float64(limit)*daysPerYear {
					continue
				}
				msg := fmt.Sprintf("may be the same as %s: names sound alike, %s and %s", b.ind.DisplayName(), what(a.birth), b.birth.Date)
				out = append(out, Finding{Person: a.ind, Other: b.ind, Message: msg})
			}
		}
	}
	return out
}

// phonetic is the key under which names that sound alike meet.
func phonetic(name string) string { return search.Soundex(name) + "/" + search.Cologne(name) }
