package genealogy

import (
	"sort"
	"time"

	"github.com/efi/goged/gedcom"
)

// Anniversary is an event that happened on the same day of the year as a
// given date.
type Anniversary struct {
	Event    *gedcom.Event
	Year     int // year of the event in the Gregorian calendar
	YearsAgo int
}

// OnThisDay returns the events that happened on the day and month of today
// in earlier years (or earlier today), oldest first. Only exact dates
// count; dates in other calendars are compared by their Gregorian
// equivalent. Events on 29 February are listed on 28 February in common
// years.
func OnThisDay(doc *gedcom.Document, today time.Time) []Anniversary {
	month, day := int(today.Month()), today.Day()
	leapDayToo := month == 2 && day == 28 && !isLeap(today.Year())
	var out []Anniversary
	for _, e := range doc.Events() {
		if e.Date.Modifier != gedcom.DateExact && e.Date.Modifier != gedcom.DateInterpreted {
			continue
		}
		first, last, ok := e.Date.Span()
		if !ok || first != last {
			continue // no day known
		}
		y, m, d := gedcom.JDNToGregorian(first)
		if y > today.Year() || m != month || (d != day && !(leapDayToo && d == 29)) {
			continue
		}
		out = append(out, Anniversary{Event: e, Year: y, YearsAgo: today.Year() - y})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Year < out[j].Year })
	return out
}

func isLeap(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }
