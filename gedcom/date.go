package gedcom

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Modifier qualifies a GEDCOM date value.
type Modifier int

// Date modifiers. DateExact is a plain date such as "12 MAR 1850".
const (
	DateExact       Modifier = iota
	DateAbout                // ABT
	DateCalculated           // CAL
	DateEstimated            // EST
	DateBefore               // BEF
	DateAfter                // AFT
	DateBetween              // BET ... AND ...
	DateFrom                 // FROM ...
	DateTo                   // TO ...
	DateFromTo               // FROM ... TO ...
	DateInterpreted          // INT ... (phrase)
	DatePhrase               // (phrase)
	DateInvalid              // text that could not be parsed
)

// SimpleDate is a single calendar date in which the day and month may be
// unknown (zero).
type SimpleDate struct {
	Calendar Calendar
	Year     int // historical year number, always positive when valid
	Month    int // 1-12 (1-13 for Hebrew and French Republican), 0 if unknown
	Day      int // 0 if unknown
	DualYear int // second year of a dual date such as 1750/51, else 0
	BC       bool
}

// Valid reports whether the date has at least a year.
func (s SimpleDate) Valid() bool { return s.Year > 0 }

// effectiveYear is the year used for calculations. For dual dates the later
// ("new style") year is used.
func (s SimpleDate) effectiveYear() int {
	y := s.Year
	if s.DualYear != 0 {
		y = s.DualYear
	}
	return astronomicalYear(y, s.BC)
}

func (s SimpleDate) monthCount() int {
	switch s.Calendar {
	case Hebrew, FrenchRepublican:
		return 13
	}
	return 12
}

func (s SimpleDate) monthDays(m int) int {
	y := s.effectiveYear()
	switch s.Calendar {
	case Julian:
		return julianMonthDays(y, m)
	case Hebrew:
		return hebrewMonthDays(y, m)
	case FrenchRepublican:
		return frenchMonthDays(y, m)
	}
	return gregorianMonthDays(y, m)
}

func (s SimpleDate) toJDN(m, d int) int {
	y := s.effectiveYear()
	switch s.Calendar {
	case Julian:
		return JulianToJDN(y, m, d)
	case Hebrew:
		return HebrewToJDN(y, m, d)
	case FrenchRepublican:
		return FrenchToJDN(y, m, d)
	}
	return GregorianToJDN(y, m, d)
}

// Span returns the first and last Julian Day Number the date may denote,
// e.g. the whole year for "1850" or the whole month for "MAR 1850".
func (s SimpleDate) Span() (first, last int, ok bool) {
	if !s.Valid() || s.Calendar == UnknownCalendar {
		return 0, 0, false
	}
	if s.Month == 0 {
		lastMonth := s.monthCount()
		for lastMonth > 1 && s.monthDays(lastMonth) == 0 {
			lastMonth--
		}
		return s.toJDN(1, 1), s.toJDN(lastMonth, s.monthDays(lastMonth)), true
	}
	if s.Day == 0 {
		return s.toJDN(s.Month, 1), s.toJDN(s.Month, s.monthDays(s.Month)), true
	}
	j := s.toJDN(s.Month, s.Day)
	return j, j, true
}

var gregorianMonthAbbr = []string{"", "JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}
var hebrewMonthAbbr = []string{"", "TSH", "CSH", "KSL", "TVT", "SHV", "ADR", "ADS", "NSN", "IYR", "SVN", "TMZ", "AAV", "ELL"}
var frenchMonthAbbr = []string{"", "VEND", "BRUM", "FRIM", "NIVO", "PLUV", "VENT", "GERM", "FLOR", "PRAI", "MESS", "THER", "FRUC", "COMP"}

var gregorianMonthNames = []string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
var hebrewMonthNames = []string{"", "Tishri", "Cheshvan", "Kislev", "Tevet", "Shevat", "Adar", "Adar II", "Nisan", "Iyar", "Sivan", "Tammuz", "Av", "Elul"}
var frenchMonthNames = []string{"", "Vendémiaire", "Brumaire", "Frimaire", "Nivôse", "Pluviôse", "Ventôse", "Germinal", "Floréal", "Prairial", "Messidor", "Thermidor", "Fructidor", "jours complémentaires"}

// monthLookup maps upper-case month tokens to (calendar, month). Full English
// month names are accepted for the Gregorian/Julian calendars as a courtesy
// to files that do not follow the standard.
var monthLookup = func() map[string]struct {
	cal   Calendar
	month int
} {
	m := map[string]struct {
		cal   Calendar
		month int
	}{}
	for i, a := range gregorianMonthAbbr[1:] {
		m[a] = struct {
			cal   Calendar
			month int
		}{Gregorian, i + 1}
	}
	full := []string{"JANUARY", "FEBRUARY", "MARCH", "APRIL", "MAY", "JUNE", "JULY", "AUGUST", "SEPTEMBER", "OCTOBER", "NOVEMBER", "DECEMBER"}
	for i, a := range full {
		m[a] = struct {
			cal   Calendar
			month int
		}{Gregorian, i + 1}
	}
	m["SEPT"] = m["SEP"]
	for i, a := range hebrewMonthAbbr[1:] {
		m[a] = struct {
			cal   Calendar
			month int
		}{Hebrew, i + 1}
	}
	for i, a := range frenchMonthAbbr[1:] {
		m[a] = struct {
			cal   Calendar
			month int
		}{FrenchRepublican, i + 1}
	}
	return m
}()

// String formats the date for display, e.g. "12 Mar 1850", "1750/51" or
// "3 Brumaire VIII".
func (s SimpleDate) String() string {
	if !s.Valid() {
		return ""
	}
	var parts []string
	if s.Day > 0 {
		parts = append(parts, strconv.Itoa(s.Day))
	}
	if s.Month > 0 {
		switch s.Calendar {
		case Hebrew:
			parts = append(parts, hebrewMonthNames[s.Month])
		case FrenchRepublican:
			parts = append(parts, frenchMonthNames[s.Month])
		default:
			parts = append(parts, gregorianMonthNames[s.Month])
		}
	}
	year := strconv.Itoa(s.Year)
	if s.Calendar == FrenchRepublican {
		year = "an " + roman(s.Year)
	}
	if s.DualYear != 0 {
		dual := strconv.Itoa(s.DualYear)
		if len(dual) >= 2 && s.DualYear/100 == s.Year/100 {
			dual = dual[len(dual)-2:]
		}
		year += "/" + dual
	}
	if s.BC {
		year += " BC"
	}
	parts = append(parts, year)
	out := strings.Join(parts, " ")
	switch s.Calendar {
	case Julian:
		out += " (Julian)"
	case Hebrew:
		out += " (Hebrew)"
	}
	return out
}

func roman(n int) string {
	if n <= 0 || n >= 4000 {
		return strconv.Itoa(n)
	}
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	syms := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(syms[i])
			n -= v
		}
	}
	return b.String()
}

// Date is a parsed GEDCOM date value.
type Date struct {
	Raw      string
	Modifier Modifier
	Start    SimpleDate // the date, or the first date of a range or period
	End      SimpleDate // the second date of BET/AND and FROM/TO
	Phrase   string     // free text of INT and phrase dates
}

// IsZero reports whether the date is empty.
func (d Date) IsZero() bool { return strings.TrimSpace(d.Raw) == "" }

// IsValid reports whether the date can be placed on a time line.
func (d Date) IsValid() bool {
	_, ok := d.Key()
	return ok
}

// IsApproximate reports whether the date is not an exact date.
func (d Date) IsApproximate() bool {
	switch d.Modifier {
	case DateExact, DateInterpreted, DateFrom, DateTo, DateFromTo:
		return false
	}
	return true
}

// main returns the simple date that defines the position of d.
func (d Date) main() SimpleDate {
	if d.Modifier == DateTo {
		return d.End
	}
	return d.Start
}

// Span returns the range of Julian Day Numbers the date may denote. Open
// ranges ("BEF 1850") are reported with ok=true and the open side set to the
// closed side.
func (d Date) Span() (first, last int, ok bool) {
	switch d.Modifier {
	case DateBetween, DateFromTo:
		f, _, ok1 := d.Start.Span()
		_, l, ok2 := d.End.Span()
		if ok1 && ok2 {
			if l < f {
				f, l = l, f
			}
			return f, l, true
		}
		if ok1 {
			return d.Start.Span()
		}
		return d.End.Span()
	case DatePhrase, DateInvalid:
		return 0, 0, false
	}
	return d.main().Span()
}

// Key returns a sort key (based on the Julian Day Number) that orders dates
// chronologically. "BEF x" sorts just before x and "AFT x" just after it.
func (d Date) Key() (int, bool) {
	first, last, ok := d.Span()
	if !ok {
		return 0, false
	}
	switch d.Modifier {
	case DateBefore:
		return first - 1, true
	case DateAfter:
		return last + 1, true
	}
	return first, true
}

// Compare orders dates chronologically. Dates that cannot be placed on a
// time line sort after all valid dates.
func (d Date) Compare(o Date) int {
	k1, ok1 := d.Key()
	k2, ok2 := o.Key()
	switch {
	case !ok1 && !ok2:
		return 0
	case !ok1:
		return 1
	case !ok2:
		return -1
	case k1 < k2:
		return -1
	case k1 > k2:
		return 1
	}
	return 0
}

// Year returns the (Gregorian) year of the date's position on the time line,
// negative for years BC. It returns 0 if the date is not valid.
func (d Date) Year() int {
	first, _, ok := d.main().Span()
	if !ok {
		if first, _, ok = d.Span(); !ok {
			return 0
		}
	}
	return displayYear(first)
}

// YearRange returns the earliest and latest Gregorian year the date may
// denote.
func (d Date) YearRange() (lo, hi int, ok bool) {
	first, last, ok := d.Span()
	if !ok {
		return 0, 0, false
	}
	return displayYear(first), displayYear(last), true
}

// displayYear converts a JDN to a historical Gregorian year (no year zero).
func displayYear(jdn int) int {
	y, _, _ := JDNToGregorian(jdn)
	if y <= 0 {
		return y - 1
	}
	return y
}

// ShortYear formats the year with a compact qualifier, as used in
// lifespans: "1850", "c.1850", "bef.1850", "aft.1850".
func (d Date) ShortYear() string {
	if !d.IsValid() {
		return ""
	}
	y := d.Year()
	s := strconv.Itoa(y)
	if y < 0 {
		s = strconv.Itoa(-y) + "BC"
	}
	switch d.Modifier {
	case DateAbout, DateCalculated, DateEstimated, DateBetween:
		return "c." + s
	case DateBefore:
		return "bef." + s
	case DateAfter:
		return "aft." + s
	}
	return s
}

// Fit formats the date in at most width characters, for columns: the full
// form if it fits, else a compact one ("Apr 1958–Mar 1961", "abt. 1850"),
// with days and months left out if necessary. rest is what the short form
// leaves out, for display elsewhere: the full date, or just the phrase of
// an interpreted date ("3. Brumaire III"); it is empty if nothing is
// missing.
func (d Date) Fit(width int) (s, rest string) {
	full := d.String()
	if utf8.RuneCountInString(full) <= width {
		return full, ""
	}
	if d.IsValid() {
		for precision := 0; precision < 3; precision++ {
			c := d.compact(precision)
			if utf8.RuneCountInString(c) > width {
				continue
			}
			switch {
			case precision > 0:
				return c, full
			case d.Modifier == DateInterpreted && d.Phrase != "":
				return c, d.Phrase
			}
			return c, ""
		}
	}
	if width < 1 {
		return "", full
	}
	return strings.TrimRight(string([]rune(full)[:width-1]), " ") + "…", full
}

// compact formats the date tersely with full dates (precision 0), months
// and years (1) or years only (2).
func (d Date) compact(precision int) string {
	f := func(s SimpleDate) string {
		if precision >= 1 {
			s.Day = 0
		}
		if precision >= 2 {
			s.Month = 0
		}
		return s.String()
	}
	switch d.Modifier {
	case DateAbout:
		return "abt. " + f(d.Start)
	case DateCalculated:
		return "cal. " + f(d.Start)
	case DateEstimated:
		return "est. " + f(d.Start)
	case DateBefore:
		return "bef. " + f(d.Start)
	case DateAfter:
		return "aft. " + f(d.Start)
	case DateBetween:
		return "bet. " + f(d.Start) + "–" + f(d.End)
	case DateFrom:
		return "from " + f(d.Start)
	case DateTo:
		return "to " + f(d.End)
	case DateFromTo:
		return f(d.Start) + "–" + f(d.End)
	}
	return f(d.Start)
}

// String formats the date for display.
func (d Date) String() string {
	switch d.Modifier {
	case DateExact:
		return d.Start.String()
	case DateAbout:
		return "about " + d.Start.String()
	case DateCalculated:
		return "calculated " + d.Start.String()
	case DateEstimated:
		return "estimated " + d.Start.String()
	case DateBefore:
		return "before " + d.Start.String()
	case DateAfter:
		return "after " + d.Start.String()
	case DateBetween:
		return "between " + d.Start.String() + " and " + d.End.String()
	case DateFrom:
		return "from " + d.Start.String()
	case DateTo:
		return "to " + d.End.String()
	case DateFromTo:
		return "from " + d.Start.String() + " to " + d.End.String()
	case DateInterpreted:
		if d.Phrase != "" {
			return d.Start.String() + " (" + d.Phrase + ")"
		}
		return d.Start.String()
	case DatePhrase:
		return d.Phrase
	}
	return strings.TrimSpace(d.Raw)
}

// ParseDate parses a GEDCOM date value. It accepts the GEDCOM 5.5.1 and 7.0
// grammars as well as common deviations (full month names, "Abt.", "circa",
// ISO 8601 dates, "Month day, year"). Unparseable input yields a Date with
// Modifier DateInvalid that still carries the raw text.
func ParseDate(raw string) Date {
	d := Date{Raw: raw}
	s := strings.TrimSpace(raw)
	if s == "" {
		d.Modifier = DateInvalid
		return d
	}

	// Pure phrase: "(text)".
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		d.Modifier = DatePhrase
		d.Phrase = strings.TrimSpace(s[1 : len(s)-1])
		return d
	}

	// Split off a trailing phrase for INT dates (or stray phrases).
	phrase := ""
	if i := strings.Index(s, "("); i >= 0 {
		phrase = strings.TrimSpace(strings.TrimSuffix(s[i+1:], ")"))
		s = strings.TrimSpace(s[:i])
	}

	toks := tokenizeDate(s)
	if len(toks) == 0 {
		d.Modifier = DateInvalid
		return d
	}

	fail := func() Date {
		return Date{Raw: raw, Modifier: DateInvalid, Phrase: phrase}
	}

	kw := toks[0]
	rest := toks[1:]
	switch kw {
	case "ABT", "ABOUT", "CIRCA", "CA", "C", "APPROX", "APPROXIMATELY", "AROUND":
		d.Modifier = DateAbout
	case "CAL", "CALCULATED":
		d.Modifier = DateCalculated
	case "EST", "ESTIMATED":
		d.Modifier = DateEstimated
	case "BEF", "BEFORE":
		d.Modifier = DateBefore
	case "AFT", "AFTER":
		d.Modifier = DateAfter
	case "INT", "INTERPRETED":
		d.Modifier = DateInterpreted
		d.Phrase = phrase
	case "BET", "BETWEEN", "BTW":
		i := indexToken(rest, "AND", "-")
		if i < 0 {
			return fail()
		}
		start, ok1 := parseSimpleDate(rest[:i])
		end, ok2 := parseSimpleDate(rest[i+1:])
		if !ok1 || !ok2 {
			return fail()
		}
		d.Modifier, d.Start, d.End = DateBetween, start, end
		return d
	case "FROM":
		if i := indexToken(rest, "TO"); i >= 0 {
			start, ok1 := parseSimpleDate(rest[:i])
			end, ok2 := parseSimpleDate(rest[i+1:])
			if !ok1 || !ok2 {
				return fail()
			}
			d.Modifier, d.Start, d.End = DateFromTo, start, end
			return d
		}
		d.Modifier = DateFrom
	case "TO":
		end, ok := parseSimpleDate(rest)
		if !ok {
			return fail()
		}
		d.Modifier, d.End = DateTo, end
		return d
	default:
		d.Modifier = DateExact
		rest = toks
	}

	sd, ok := parseSimpleDate(rest)
	if !ok {
		return fail()
	}
	d.Start = sd
	return d
}

// tokenizeDate upper-cases s and splits it into tokens. Calendar escapes are
// kept as single tokens, trailing periods on keywords and months are dropped
// and commas are treated as separators.
func tokenizeDate(s string) []string {
	s = strings.ToUpper(s)
	s = strings.ReplaceAll(s, "@#DFRENCH R@", "@#DFRENCH_R@")
	s = strings.ReplaceAll(s, ",", " ")
	var toks []string
	for _, f := range strings.Fields(s) {
		switch f {
		case "B.C.", "B.C", "BC", "BCE", "B.C.E.":
			toks = append(toks, "BC")
			continue
		}
		f = strings.TrimSuffix(f, ".")
		// "c.1850" and "abt.1850" glued together.
		if i := strings.IndexByte(f, '.'); i > 0 && i < len(f)-1 {
			head, tail := f[:i], f[i+1:]
			if _, err := strconv.Atoi(tail); err == nil {
				toks = append(toks, head, tail)
				continue
			}
		}
		if f != "" {
			toks = append(toks, f)
		}
	}
	return toks
}

func indexToken(toks []string, want ...string) int {
	for i, t := range toks {
		for _, w := range want {
			if t == w {
				return i
			}
		}
	}
	return -1
}

func parseCalendarToken(t string) (Calendar, bool) {
	switch t {
	case "@#DGREGORIAN@", "GREGORIAN":
		return Gregorian, true
	case "@#DJULIAN@", "JULIAN":
		return Julian, true
	case "@#DHEBREW@", "HEBREW":
		return Hebrew, true
	case "@#DFRENCH_R@", "FRENCH_R":
		return FrenchRepublican, true
	case "@#DROMAN@", "@#DUNKNOWN@":
		return UnknownCalendar, true
	}
	return 0, false
}

// parseSimpleDate parses "[calendar] [day] [month] year[/dual] [BC]".
func parseSimpleDate(toks []string) (SimpleDate, bool) {
	var sd SimpleDate
	calSet := false
	if len(toks) > 0 {
		if c, ok := parseCalendarToken(toks[0]); ok {
			sd.Calendar = c
			calSet = true
			toks = toks[1:]
		}
	}
	if n := len(toks); n > 0 && toks[n-1] == "BC" {
		sd.BC = true
		toks = toks[:n-1]
	}

	// ISO 8601: 1850-03-12 or 1850-03.
	if len(toks) == 1 && strings.Count(toks[0], "-") >= 1 {
		parts := strings.Split(toks[0], "-")
		if len(parts) <= 3 && len(parts[0]) == 4 {
			nums := make([]int, len(parts))
			for i, p := range parts {
				n, err := strconv.Atoi(p)
				if err != nil {
					return sd, false
				}
				nums[i] = n
			}
			sd.Year = nums[0]
			if len(nums) > 1 {
				sd.Month = nums[1]
			}
			if len(nums) > 2 {
				sd.Day = nums[2]
			}
			return sd, validateSimpleDate(sd)
		}
		return sd, false
	}

	var (
		day, month int
		monthCal   Calendar
		haveMonth  bool
		numbers    []string
	)
	for _, t := range toks {
		if mc, ok := monthLookup[t]; ok && !haveMonth {
			month, monthCal, haveMonth = mc.month, mc.cal, true
			continue
		}
		numbers = append(numbers, t)
	}

	switch len(numbers) {
	case 1: // year
	case 2: // day year ("12 MAR 1850" or "MAR 12 1850")
		if !haveMonth {
			return sd, false
		}
		n, err := strconv.Atoi(numbers[0])
		if err != nil || n <= 0 {
			return sd, false
		}
		day = n
		numbers = numbers[1:]
	default:
		return sd, false
	}

	if !parseYear(numbers[0], &sd) {
		return sd, false
	}
	if haveMonth {
		switch {
		case !calSet && monthCal != Gregorian:
			sd.Calendar = monthCal
		case calSet && (sd.Calendar == Gregorian || sd.Calendar == Julian) && monthCal != Gregorian:
			return sd, false
		case calSet && (sd.Calendar == Hebrew || sd.Calendar == FrenchRepublican) && monthCal != sd.Calendar:
			return sd, false
		}
		sd.Month = month
	}
	sd.Day = day
	if day > 0 && month == 0 {
		return sd, false
	}
	return sd, validateSimpleDate(sd)
}

func parseYear(t string, sd *SimpleDate) bool {
	yearStr, dualStr, dual := strings.Cut(t, "/")
	y, err := strconv.Atoi(yearStr)
	if err != nil || y <= 0 || len(yearStr) > 5 {
		return false
	}
	sd.Year = y
	if dual {
		if dualStr == "" || len(dualStr) > len(yearStr) {
			return false
		}
		n, err := strconv.Atoi(dualStr)
		if err != nil {
			return false
		}
		// Expand an abbreviated second year: 1750/51 -> 1751, 1799/00 -> 1800.
		mod := 1
		for range dualStr {
			mod *= 10
		}
		full := y - y%mod + n
		if full <= y {
			full += mod
		}
		sd.DualYear = full
	}
	return true
}

func validateSimpleDate(sd SimpleDate) bool {
	if sd.Year <= 0 {
		return false
	}
	if sd.Month == 0 {
		return sd.Day == 0
	}
	if sd.Month < 0 || sd.Month > sd.monthCount() {
		return false
	}
	if sd.Day == 0 {
		return true
	}
	if sd.Calendar == UnknownCalendar {
		return sd.Day >= 1 && sd.Day <= 31
	}
	return sd.Day >= 1 && sd.Day <= sd.monthDays(sd.Month)
}

// Age is an age computed from two dates.
type Age struct {
	Years  int
	Approx bool // true if either date is imprecise
}

func (a Age) String() string {
	if a.Approx {
		return fmt.Sprintf("~%d", a.Years)
	}
	return strconv.Itoa(a.Years)
}

// AgeBetween computes the age in whole years of a person born at birth on
// the date at. It returns ok=false if either date is unusable, if the
// second date precedes the first or if open-ended dates (BEF/AFT) or
// periods (FROM/TO) are involved.
func AgeBetween(birth, at Date) (Age, bool) {
	if !birth.IsValid() || !at.IsValid() {
		return Age{}, false
	}
	for _, m := range []Modifier{birth.Modifier, at.Modifier} {
		if m == DateBefore || m == DateAfter || m == DateFrom || m == DateTo || m == DateFromTo {
			return Age{}, false
		}
	}
	b1, b2, _ := birth.Span()
	a1, a2, _ := at.Span()
	if a2 < b1 {
		return Age{}, false
	}
	// The age lies between the youngest and the oldest possible value.
	lo := max(wholeYears(b2, a1), 0)
	hi := max(wholeYears(b1, a2), 0)
	approx := birth.IsApproximate() || at.IsApproximate() || b1 != b2 || a1 != a2
	if lo == hi {
		return Age{Years: lo, Approx: approx}, true
	}
	// Otherwise use the distance between the midpoints of both spans.
	days := (a1 + (a2-a1)/2) - (b1 + (b2-b1)/2)
	years := int(float64(days)/365.2425 + 0.5)
	return Age{Years: min(max(years, lo), hi), Approx: true}, true
}

// wholeYears returns the number of completed years between two days.
func wholeYears(from, to int) int {
	fy, fm, fd := JDNToGregorian(from)
	ty, tm, td := JDNToGregorian(to)
	years := ty - fy
	if tm < fm || (tm == fm && td < fd) {
		years--
	}
	return years
}
