package search

import (
	"fmt"
	"sort"
	"strings"

	"github.com/efi/goged/gedcom"
)

// EventQuery filters events. Terms are separated by spaces and must all
// match:
//
//	1850, 1850..1900, <1900   event year
//	type:birth,death          event type (tag or label prefix, comma list)
//	place:leeds               place contains
//	name:smith                a principal's name contains
//	text                      label, names, place or detail contain
//
// Any term can be negated with a leading '-'.
type EventQuery struct {
	Raw   string
	terms []eventTerm
}

type eventTerm struct {
	field  Field
	negate bool
	folded string
	years  yearRange
	tags   map[string]bool
}

// ParseEventQuery parses an event filter.
func ParseEventQuery(s string) (EventQuery, error) {
	q := EventQuery{Raw: s}
	toks, err := tokenize(s)
	if err != nil {
		return q, err
	}
	for _, tok := range toks {
		var t eventTerm
		if strings.HasPrefix(tok, "-") && len(tok) > 1 {
			t.negate = true
			tok = tok[1:]
		}
		if name, value, ok := strings.Cut(tok, ":"); ok && name != "" {
			f, known := fieldAliases[strings.ToLower(name)]
			switch {
			case !known:
				return EventQuery{Raw: s}, &ParseError{fmt.Sprintf("unknown field %q", name)}
			case f != FieldType && f != FieldPlace && f != FieldName && f != FieldYear:
				return EventQuery{Raw: s}, &ParseError{fmt.Sprintf("%s: is not available for events (use type:, place:, name: or year:)", name)}
			}
			t.field = f
			tok = value
		}
		tok = strings.TrimSpace(tok)
		if tok == "" {
			if t.field == FieldText {
				continue
			}
			return EventQuery{Raw: s}, &ParseError{fmt.Sprintf("missing value for %s:", t.field)}
		}
		t.folded = Fold(tok)
		switch t.field {
		case FieldText:
			if looksLikeYear(tok) {
				t.field = FieldYear
				t.years, _ = parseYearRange(tok)
			}
		case FieldYear:
			var ok bool
			if t.years, ok = parseYearRange(tok); !ok {
				return EventQuery{Raw: s}, &ParseError{"year: expects a year or range such as 1850 or 1800..1850"}
			}
		case FieldType:
			t.tags = map[string]bool{}
			for _, part := range strings.Split(tok, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				for _, tag := range gedcom.EventTagsByLabel(part) {
					t.tags[tag] = true
				}
				t.tags[strings.ToUpper(part)] = true
			}
		}
		q.terms = append(q.terms, t)
	}
	return q, nil
}

type eventItem struct {
	ev     *gedcom.Event
	text   string
	names  string
	place  string
	lo, hi int
	ok     bool
	key    int  // chronological sort key
	dated  bool // key is valid
}

// EventIndex holds all events of a document in chronological order with
// pre-folded text for filtering.
type EventIndex struct {
	items []eventItem
}

// NewEventIndex builds an index over events, sorted chronologically.
// Undated events come last.
func NewEventIndex(events []*gedcom.Event) *EventIndex {
	ix := &EventIndex{items: make([]eventItem, 0, len(events))}
	for _, ev := range events {
		var names []string
		for _, p := range ev.Principals() {
			for _, n := range p.Names {
				names = append(names, n.String())
			}
		}
		it := eventItem{
			ev:    ev,
			names: Fold(strings.Join(names, "\n")),
			place: Fold(strings.Join(ev.Place.Names(), "\n")),
		}
		it.text = Fold(ev.Label()+"\n"+ev.Detail()) + "\n" + it.names + "\n" + it.place
		it.lo, it.hi, it.ok = eventYears(ev.Date)
		it.key, it.dated = ev.Date.Key()
		ix.items = append(ix.items, it)
	}
	sort.SliceStable(ix.items, func(i, j int) bool {
		a, b := &ix.items[i], &ix.items[j]
		if a.dated != b.dated {
			return a.dated
		}
		return a.key < b.key
	})
	return ix
}

// Len returns the number of indexed events.
func (ix *EventIndex) Len() int { return len(ix.items) }

// Filter returns the events matching q in chronological order. If vitalOnly
// is set only vital events (births, marriages, deaths, ...) are returned.
func (ix *EventIndex) Filter(q EventQuery, vitalOnly bool) []*gedcom.Event {
	var out []*gedcom.Event
	for i := range ix.items {
		it := &ix.items[i]
		if vitalOnly && !it.ev.IsVital() {
			continue
		}
		if q.matches(it) {
			out = append(out, it.ev)
		}
	}
	return out
}

func (q EventQuery) matches(it *eventItem) bool {
	for _, t := range q.terms {
		var m bool
		switch t.field {
		case FieldYear:
			m = it.ok && t.years.intersects(it.lo, it.hi)
		case FieldType:
			m = t.tags[it.ev.Tag] || strings.HasPrefix(Fold(it.ev.Label()), t.folded)
		case FieldPlace:
			m = strings.Contains(it.place, t.folded)
		case FieldName:
			m = strings.Contains(it.names, t.folded)
		default:
			m = strings.Contains(it.text, t.folded)
		}
		if m == t.negate {
			return false
		}
	}
	return true
}
