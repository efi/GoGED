package gedcom

// Calendar identifies the calendar a date is expressed in.
type Calendar int

// Calendars defined by GEDCOM.
const (
	Gregorian Calendar = iota
	Julian
	Hebrew
	FrenchRepublican
	UnknownCalendar
)

func (c Calendar) String() string {
	switch c {
	case Gregorian:
		return "Gregorian"
	case Julian:
		return "Julian"
	case Hebrew:
		return "Hebrew"
	case FrenchRepublican:
		return "French Republican"
	default:
		return "unknown"
	}
}

// floorDiv divides rounding towards negative infinity.
func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func floorMod(a, b int) int { return a - b*floorDiv(a, b) }

// astronomicalYear converts a historical year (no year zero) into an
// astronomical year number where 1 BC is year 0.
func astronomicalYear(year int, bc bool) int {
	if bc {
		return 1 - year
	}
	return year
}

// GregorianToJDN returns the Julian Day Number of a proleptic Gregorian date
// given as an astronomical year.
func GregorianToJDN(y, m, d int) int {
	a := floorDiv(14-m, 12)
	y2 := y + 4800 - a
	m2 := m + 12*a - 3
	return d + floorDiv(153*m2+2, 5) + 365*y2 + floorDiv(y2, 4) - floorDiv(y2, 100) + floorDiv(y2, 400) - 32045
}

// JulianToJDN returns the Julian Day Number of a Julian calendar date given
// as an astronomical year.
func JulianToJDN(y, m, d int) int {
	a := floorDiv(14-m, 12)
	y2 := y + 4800 - a
	m2 := m + 12*a - 3
	return d + floorDiv(153*m2+2, 5) + 365*y2 + floorDiv(y2, 4) - 32083
}

// JDNToGregorian converts a Julian Day Number into a proleptic Gregorian
// date with an astronomical year.
func JDNToGregorian(jdn int) (y, m, d int) {
	a := jdn + 32044
	b := floorDiv(4*a+3, 146097)
	c := a - floorDiv(146097*b, 4)
	dd := floorDiv(4*c+3, 1461)
	e := c - floorDiv(1461*dd, 4)
	mm := floorDiv(5*e+2, 153)
	d = e - floorDiv(153*mm+2, 5) + 1
	m = mm + 3 - 12*floorDiv(mm, 10)
	y = 100*b + dd - 4800 + floorDiv(mm, 10)
	return y, m, d
}

func isGregorianLeap(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

func isJulianLeap(y int) bool { return floorMod(y, 4) == 0 }

func gregorianMonthDays(y, m int) int {
	switch m {
	case 2:
		if isGregorianLeap(y) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

func julianMonthDays(y, m int) int {
	if m == 2 {
		if isJulianLeap(y) {
			return 29
		}
		return 28
	}
	return gregorianMonthDays(2001, m)
}

// --- Hebrew calendar -------------------------------------------------------
//
// GEDCOM numbers Hebrew months in civil order starting with Tishri:
// TSH=1 CSH=2 KSL=3 TVT=4 SHV=5 ADR=6 ADS=7 NSN=8 IYR=9 SVN=10 TMZ=11 AAV=12
// ELL=13. ADR is Adar in common years and Adar I in leap years; ADS (Adar II)
// only exists in leap years. The arithmetic below follows Dershowitz and
// Reingold, "Calendrical Calculations".

func hebrewLeap(y int) bool { return floorMod(7*y+1, 19) < 7 }

// hebrewElapsedDays is the number of days from the Hebrew epoch to the
// start of year y.
func hebrewElapsedDays(y int) int {
	monthsElapsed := 235*floorDiv(y-1, 19) + 12*floorMod(y-1, 19) + floorDiv(7*floorMod(y-1, 19)+1, 19)
	partsElapsed := 204 + 793*floorMod(monthsElapsed, 1080)
	hoursElapsed := 5 + 12*monthsElapsed + 793*floorDiv(monthsElapsed, 1080) + floorDiv(partsElapsed, 1080)
	conjDay := 1 + 29*monthsElapsed + floorDiv(hoursElapsed, 24)
	conjParts := 1080*floorMod(hoursElapsed, 24) + floorMod(partsElapsed, 1080)
	alt := conjDay
	if conjParts >= 19440 ||
		(floorMod(conjDay, 7) == 2 && conjParts >= 9924 && !hebrewLeap(y)) ||
		(floorMod(conjDay, 7) == 1 && conjParts >= 16789 && hebrewLeap(y-1)) {
		alt++
	}
	if d := floorMod(alt, 7); d == 0 || d == 3 || d == 5 {
		alt++
	}
	return alt
}

func hebrewYearDays(y int) int { return hebrewElapsedDays(y+1) - hebrewElapsedDays(y) }

// hebrewMonthDays returns the length of a month in GEDCOM numbering.
func hebrewMonthDays(y, m int) int {
	switch m {
	case 1: // Tishri
		return 30
	case 2: // Cheshvan: 30 in "complete" years
		if hebrewYearDays(y)%10 == 5 {
			return 30
		}
		return 29
	case 3: // Kislev: 29 in "deficient" years
		if hebrewYearDays(y)%10 == 3 {
			return 29
		}
		return 30
	case 4: // Tevet
		return 29
	case 5: // Shevat
		return 30
	case 6: // Adar (common year) or Adar I (leap year)
		if hebrewLeap(y) {
			return 30
		}
		return 29
	case 7: // Adar II
		if hebrewLeap(y) {
			return 29
		}
		return 0
	case 8: // Nisan
		return 30
	case 9: // Iyar
		return 29
	case 10: // Sivan
		return 30
	case 11: // Tammuz
		return 29
	case 12: // Av
		return 30
	case 13: // Elul
		return 29
	}
	return 0
}

// hebrewEpochJDN anchors the Hebrew calendar on the Julian Day Number scale;
// see Dershowitz and Reingold (R.D. epoch -1373429 = JDN 347997 - 1721426).
const hebrewEpochJDN = 347997

// HebrewToJDN converts a Hebrew date (GEDCOM month numbering) to a JDN.
func HebrewToJDN(y, m, d int) int {
	days := hebrewElapsedDays(y)
	for i := 1; i < m; i++ {
		days += hebrewMonthDays(y, i)
	}
	return hebrewEpochJDN + days + d - 1
}

// --- French Republican calendar ---------------------------------------------
//
// Year I began on 22 September 1792. For the years the calendar was in use
// (I to XIV) the leap ("sextile") years were III, VII and XI; afterwards the
// Romme rule is applied. Months 1-12 have 30 days, month 13 holds the five
// or six complementary days.

const frenchEpochJDN = 2375840 // 1 Vendémiaire I = 22 September 1792

func frenchLeap(y int) bool {
	if y <= 14 {
		return y == 3 || y == 7 || y == 11
	}
	return y%4 == 0 && (y%100 != 0 || y%400 == 0) && y%4000 != 0
}

func frenchYearStart(y int) int {
	jdn := frenchEpochJDN
	if y >= 1 {
		for i := 1; i < y; i++ {
			jdn += 365
			if frenchLeap(i) {
				jdn++
			}
		}
	} else {
		for i := y; i < 1; i++ {
			jdn -= 365
		}
	}
	return jdn
}

func frenchMonthDays(y, m int) int {
	if m == 13 {
		if frenchLeap(y) {
			return 6
		}
		return 5
	}
	if m >= 1 && m <= 12 {
		return 30
	}
	return 0
}

// FrenchToJDN converts a French Republican date to a JDN.
func FrenchToJDN(y, m, d int) int {
	return frenchYearStart(y) + (m-1)*30 + d - 1
}
