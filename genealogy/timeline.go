package genealogy

import (
	"sort"

	"github.com/efi/goged/gedcom"
)

// TimelineEntry is one event in a person's timeline.
type TimelineEntry struct {
	Event *gedcom.Event
	// Relative is set for events of other people that are shown for
	// context, such as the birth of a child or the death of a parent.
	Relative *gedcom.Individual
	// Relation describes Relative from the subject's point of view, e.g.
	// "son" or "half-sister".
	Relation string
	// Spouse is the partner for family events of the subject (marriages,
	// divorces, ...).
	Spouse *gedcom.Individual
	// Age of the subject at the event, if it can be computed.
	Age    gedcom.Age
	HasAge bool
}

// Own reports whether the entry is an event of the subject or one of the
// subject's families.
func (e TimelineEntry) Own() bool { return e.Relative == nil }

// Title describes the entry: "Birth", or "Birth of son Arthur Smith" for
// relatives' events.
func (e TimelineEntry) Title() string {
	if e.Relative == nil {
		return e.Event.Label()
	}
	return e.Event.Label() + " of " + e.Relation + " " + e.Relative.DisplayName()
}

// TimelineOptions control which events a timeline includes.
type TimelineOptions struct {
	// Relatives adds dated births, marriages and deaths of close relatives
	// that fall within the subject's lifetime.
	Relatives bool
}

// maxLifespanDays bounds the time line of people whose birth or death is
// unknown (about 100 years).
const maxLifespanDays = 36525

// Timeline lists the events of ind's life in chronological order. Undated
// births come first and undated deaths and burials last.
func Timeline(ind *gedcom.Individual, opts TimelineOptions) []TimelineEntry {
	var entries []TimelineEntry
	for _, e := range ind.Events {
		entries = append(entries, TimelineEntry{Event: e})
	}
	for _, f := range ind.FamiliesAsSpouse() {
		for _, e := range f.Events {
			entries = append(entries, TimelineEntry{Event: e, Spouse: f.Partner(ind)})
		}
	}
	if opts.Relatives {
		entries = append(entries, relativeEvents(ind)...)
	}

	birth := ind.BirthDate()
	isBirthEvent := func(e TimelineEntry) bool {
		return e.Relative == nil && e.Event.Individual == ind && isOneOf(e.Event.Tag, gedcom.BirthTags)
	}
	for i := range entries {
		if isBirthEvent(entries[i]) {
			continue
		}
		if age, ok := gedcom.AgeBetween(birth, entries[i].Event.Date); ok {
			entries[i].Age, entries[i].HasAge = age, true
		}
	}

	bucket := func(e TimelineEntry) int {
		if e.Event.Date.IsValid() {
			return 1
		}
		if e.Relative == nil && isOneOf(e.Event.Tag, gedcom.BirthTags) {
			return 0
		}
		if e.Relative == nil && (isOneOf(e.Event.Tag, gedcom.DeathTags) || e.Event.Tag == "PROB" || e.Event.Tag == "WILL") {
			return 3
		}
		return 2
	}
	sort.SliceStable(entries, func(i, j int) bool {
		bi, bj := bucket(entries[i]), bucket(entries[j])
		if bi != bj {
			return bi < bj
		}
		if bi != 1 {
			return false
		}
		return entries[i].Event.Date.Compare(entries[j].Event.Date) < 0
	})
	return entries
}

func isOneOf(tag string, tags []string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// lifetime returns the range of day numbers during which relatives' events
// are relevant to ind.
func lifetime(ind *gedcom.Individual) (lo, hi int, ok bool) {
	b1, _, okB := ind.BirthDate().Span()
	_, d2, okD := ind.DeathDate().Span()
	switch {
	case okB && okD:
		return b1, d2, true
	case okB:
		return b1, b1 + maxLifespanDays, true
	case okD:
		return d2 - maxLifespanDays, d2, true
	}
	return 0, 0, false
}

func relativeEvents(ind *gedcom.Individual) []TimelineEntry {
	lo, hi, bounded := lifetime(ind)
	var out []TimelineEntry
	add := func(rel *gedcom.Individual, relation string, e *gedcom.Event) {
		if e == nil || !e.Date.IsValid() {
			return
		}
		if bounded {
			k, _ := e.Date.Key()
			if k < lo || k > hi {
				return
			}
		}
		out = append(out, TimelineEntry{Event: e, Relative: rel, Relation: relation})
	}
	births := func(r *gedcom.Individual) *gedcom.Event { return r.FirstDatedEvent(gedcom.BirthTags...) }
	deaths := func(r *gedcom.Individual) *gedcom.Event { return r.FirstDatedEvent(gedcom.DeathTags...) }

	for _, p := range ind.Parents() {
		add(p, gendered(p.Sex, "father", "mother", "parent"), deaths(p))
	}
	for _, s := range ind.Spouses() {
		add(s, gendered(s.Sex, "husband", "wife", "spouse"), deaths(s))
	}
	for _, s := range ind.Siblings() {
		rel := gendered(s.Sex, "brother", "sister", "sibling")
		add(s, rel, births(s))
		add(s, rel, deaths(s))
	}
	for _, s := range ind.HalfSiblings() {
		rel := "half-" + gendered(s.Sex, "brother", "sister", "sibling")
		add(s, rel, births(s))
		add(s, rel, deaths(s))
	}
	for _, c := range ind.Children() {
		rel := gendered(c.Sex, "son", "daughter", "child")
		add(c, rel, births(c))
		for _, f := range c.FamiliesAsSpouse() {
			add(c, rel, f.Marriage())
		}
		add(c, rel, deaths(c))
		for _, gc := range c.Children() {
			add(gc, gendered(gc.Sex, "grandson", "granddaughter", "grandchild"), births(gc))
		}
	}
	return out
}
