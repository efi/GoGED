package gedcom

import (
	"sort"
	"testing"
)

func TestParseDate(t *testing.T) {
	g := func(y, m, d int) SimpleDate { return SimpleDate{Calendar: Gregorian, Year: y, Month: m, Day: d} }
	tests := []struct {
		in    string
		mod   Modifier
		start SimpleDate
		end   SimpleDate
		str   string
	}{
		{"12 MAR 1850", DateExact, g(1850, 3, 12), SimpleDate{}, "12 Mar 1850"},
		{"MAR 1850", DateExact, g(1850, 3, 0), SimpleDate{}, "Mar 1850"},
		{"1850", DateExact, g(1850, 0, 0), SimpleDate{}, "1850"},
		{"  12 mar 1850 ", DateExact, g(1850, 3, 12), SimpleDate{}, "12 Mar 1850"},
		{"12 March 1850", DateExact, g(1850, 3, 12), SimpleDate{}, "12 Mar 1850"},
		{"March 12, 1850", DateExact, g(1850, 3, 12), SimpleDate{}, "12 Mar 1850"},
		{"12 SEPT 1850", DateExact, g(1850, 9, 12), SimpleDate{}, "12 Sep 1850"},
		{"1850-03-12", DateExact, g(1850, 3, 12), SimpleDate{}, "12 Mar 1850"},
		{"1850-03", DateExact, g(1850, 3, 0), SimpleDate{}, "Mar 1850"},
		{"ABT 1850", DateAbout, g(1850, 0, 0), SimpleDate{}, "about 1850"},
		{"Abt. 1850", DateAbout, g(1850, 0, 0), SimpleDate{}, "about 1850"},
		{"c.1850", DateAbout, g(1850, 0, 0), SimpleDate{}, "about 1850"},
		{"circa 1850", DateAbout, g(1850, 0, 0), SimpleDate{}, "about 1850"},
		{"CAL 1850", DateCalculated, g(1850, 0, 0), SimpleDate{}, "calculated 1850"},
		{"EST 1850", DateEstimated, g(1850, 0, 0), SimpleDate{}, "estimated 1850"},
		{"BEF 1850", DateBefore, g(1850, 0, 0), SimpleDate{}, "before 1850"},
		{"before 3 jan 1850", DateBefore, g(1850, 1, 3), SimpleDate{}, "before 3 Jan 1850"},
		{"AFT 1850", DateAfter, g(1850, 0, 0), SimpleDate{}, "after 1850"},
		{"BET 1850 AND 1855", DateBetween, g(1850, 0, 0), g(1855, 0, 0), "between 1850 and 1855"},
		{"between MAR 1850 and 4 APR 1851", DateBetween, g(1850, 3, 0), g(1851, 4, 4), "between Mar 1850 and 4 Apr 1851"},
		{"BET 1850 - 1855", DateBetween, g(1850, 0, 0), g(1855, 0, 0), "between 1850 and 1855"},
		{"FROM 1850", DateFrom, g(1850, 0, 0), SimpleDate{}, "from 1850"},
		{"TO 1850", DateTo, SimpleDate{}, g(1850, 0, 0), "to 1850"},
		{"FROM 1850 TO 1855", DateFromTo, g(1850, 0, 0), g(1855, 0, 0), "from 1850 to 1855"},
		{"INT 12 MAR 1850 (twelfth of March)", DateInterpreted, g(1850, 3, 12), SimpleDate{}, "12 Mar 1850 (twelfth of March)"},
		{"12 FEB 1750/51", DateExact, SimpleDate{Year: 1750, Month: 2, Day: 12, DualYear: 1751}, SimpleDate{}, "12 Feb 1750/51"},
		{"1799/00", DateExact, SimpleDate{Year: 1799, DualYear: 1800}, SimpleDate{}, "1799/1800"},
		{"1699/1700", DateExact, SimpleDate{Year: 1699, DualYear: 1700}, SimpleDate{}, "1699/1700"},
		{"44 B.C.", DateExact, SimpleDate{Year: 44, BC: true}, SimpleDate{}, "44 BC"},
		{"15 MAR 44 BCE", DateExact, SimpleDate{Year: 44, Month: 3, Day: 15, BC: true}, SimpleDate{}, "15 Mar 44 BC"},
		{"@#DJULIAN@ 1 JAN 1700", DateExact, SimpleDate{Calendar: Julian, Year: 1700, Month: 1, Day: 1}, SimpleDate{}, "1 Jan 1700 (Julian)"},
		{"JULIAN 1 JAN 1700", DateExact, SimpleDate{Calendar: Julian, Year: 1700, Month: 1, Day: 1}, SimpleDate{}, "1 Jan 1700 (Julian)"},
		{"@#DGREGORIAN@ 1700", DateExact, g(1700, 0, 0), SimpleDate{}, "1700"},
		{"@#DHEBREW@ 1 TSH 5785", DateExact, SimpleDate{Calendar: Hebrew, Year: 5785, Month: 1, Day: 1}, SimpleDate{}, "1 Tishri 5785 (Hebrew)"},
		{"15 NSN 5783", DateExact, SimpleDate{Calendar: Hebrew, Year: 5783, Month: 8, Day: 15}, SimpleDate{}, "15 Nisan 5783 (Hebrew)"},
		{"@#DFRENCH R@ 18 BRUM 8", DateExact, SimpleDate{Calendar: FrenchRepublican, Year: 8, Month: 2, Day: 18}, SimpleDate{}, "18 Brumaire an VIII"},
		{"FRENCH_R 1 VEND 1", DateExact, SimpleDate{Calendar: FrenchRepublican, Year: 1, Month: 1, Day: 1}, SimpleDate{}, "1 Vendémiaire an I"},
		{"ABT @#DJULIAN@ 1650", DateAbout, SimpleDate{Calendar: Julian, Year: 1650}, SimpleDate{}, "about 1650 (Julian)"},
		{"(stillborn)", DatePhrase, SimpleDate{}, SimpleDate{}, "stillborn"},
	}
	for _, tt := range tests {
		d := ParseDate(tt.in)
		if d.Raw != tt.in {
			t.Errorf("%q: Raw = %q", tt.in, d.Raw)
		}
		if d.Modifier != tt.mod {
			t.Errorf("%q: modifier = %v, want %v", tt.in, d.Modifier, tt.mod)
			continue
		}
		if d.Start != tt.start || d.End != tt.end {
			t.Errorf("%q: start/end = %+v / %+v, want %+v / %+v", tt.in, d.Start, d.End, tt.start, tt.end)
		}
		if s := d.String(); s != tt.str {
			t.Errorf("%q: String = %q, want %q", tt.in, s, tt.str)
		}
	}
}

func TestParseDateInvalid(t *testing.T) {
	bad := []string{
		"unknown", "32 JAN 1850", "29 FEB 1900", "0 JAN 1850", "JAN", "12 1850", "BET 1850",
		"BET 1850 AND", "FROM x TO 1850", "TO", "12 13 1850", "1850/", "1850/123456",
		"1850-13-01", "18x0", "@#DJULIAN@ 1 VEND 8", "@#DHEBREW@ 1 JAN 5785", "30 ADS 5785",
		"1 2 3 4", "ABT", "1850-xx", "0", "-5", "12 MAR 1850 JAN",
	}
	for _, in := range bad {
		d := ParseDate(in)
		if d.Modifier != DateInvalid {
			t.Errorf("ParseDate(%q) = %+v, want invalid", in, d)
		}
		if d.IsValid() {
			t.Errorf("ParseDate(%q).IsValid() = true", in)
		}
		if d.String() != in {
			t.Errorf("invalid date String() = %q, want raw %q", d.String(), in)
		}
	}
	if d := ParseDate("   "); !d.IsZero() || d.IsValid() {
		t.Error("blank date should be zero and invalid")
	}
	if d := ParseDate("29 FEB 1600"); d.Modifier != DateExact {
		t.Error("1600 is a leap year")
	}
	if d := ParseDate("@#DJULIAN@ 29 FEB 1700"); d.Modifier != DateExact {
		t.Error("1700 is a Julian leap year")
	}
	if d := ParseDate("@#DUNKNOWN@ 31 JAN 1200"); d.Modifier != DateExact || d.IsValid() {
		t.Errorf("unknown calendar should parse but not be placeable: %+v", d)
	}
}

func TestCalendarConversions(t *testing.T) {
	tests := []struct {
		name       string
		jdn        int
		gy, gm, gd int
	}{
		{"gregorian epoch J2000", GregorianToJDN(2000, 1, 1), 2000, 1, 1},
		{"julian reform", JulianToJDN(1582, 10, 5), 1582, 10, 15},
		{"julian 1700", JulianToJDN(1700, 1, 1), 1700, 1, 11},
		{"julian after leap day", JulianToJDN(1700, 3, 1), 1700, 3, 12},
		{"hebrew new year 5784", HebrewToJDN(5784, 1, 1), 2023, 9, 16},
		{"hebrew new year 5785", HebrewToJDN(5785, 1, 1), 2024, 10, 3},
		{"passover 5783", HebrewToJDN(5783, 8, 15), 2023, 4, 6},
		{"hebrew leap adar II", HebrewToJDN(5784, 7, 14), 2024, 3, 24}, // Purim 5784
		{"french year I", FrenchToJDN(1, 1, 1), 1792, 9, 22},
		{"18 brumaire", FrenchToJDN(8, 2, 18), 1799, 11, 9},
		{"french year IV", FrenchToJDN(4, 1, 1), 1795, 9, 23},
		{"french year XII", FrenchToJDN(12, 1, 1), 1803, 9, 24},
		{"french sextile day", FrenchToJDN(3, 13, 6), 1795, 9, 22},
	}
	for _, tt := range tests {
		y, m, d := JDNToGregorian(tt.jdn)
		if y != tt.gy || m != tt.gm || d != tt.gd {
			t.Errorf("%s: got %d-%02d-%02d, want %d-%02d-%02d", tt.name, y, m, d, tt.gy, tt.gm, tt.gd)
		}
	}
	if GregorianToJDN(2000, 1, 1) != 2451545 {
		t.Error("JDN of 2000-01-01 must be 2451545")
	}
	// Round trip over a wide range, including BC years.
	for jdn := GregorianToJDN(-500, 1, 1); jdn < GregorianToJDN(2100, 1, 1); jdn += 997 {
		y, m, d := JDNToGregorian(jdn)
		if GregorianToJDN(y, m, d) != jdn {
			t.Fatalf("round trip failed for %d", jdn)
		}
	}
	// Hebrew year lengths are always one of the six legal values.
	legal := map[int]bool{353: true, 354: true, 355: true, 383: true, 384: true, 385: true}
	for y := 5600; y < 5900; y++ {
		if n := hebrewYearDays(y); !legal[n] {
			t.Fatalf("Hebrew year %d has %d days", y, n)
		}
		total := 0
		for m := 1; m <= 13; m++ {
			total += hebrewMonthDays(y, m)
		}
		if total != hebrewYearDays(y) {
			t.Fatalf("Hebrew year %d: months sum to %d, year has %d days", y, total, hebrewYearDays(y))
		}
	}
	for c, want := range map[Calendar]string{Gregorian: "Gregorian", Julian: "Julian", Hebrew: "Hebrew", FrenchRepublican: "French Republican", UnknownCalendar: "unknown"} {
		if c.String() != want {
			t.Errorf("%d.String() = %q", c, c.String())
		}
	}
}

func TestDateSpanAndYear(t *testing.T) {
	tests := []struct {
		in       string
		year     int
		lo, hi   int
		shortYr  string
		approx   bool
		validKey bool
	}{
		{"12 MAR 1850", 1850, 1850, 1850, "1850", false, true},
		{"1850", 1850, 1850, 1850, "1850", false, true},
		{"ABT 1850", 1850, 1850, 1850, "c.1850", true, true},
		{"BEF 1850", 1850, 1850, 1850, "bef.1850", true, true},
		{"AFT 1850", 1850, 1850, 1850, "aft.1850", true, true},
		{"BET 1850 AND 1860", 1850, 1850, 1860, "c.1850", true, true},
		{"BET 1860 AND 1850", 1860, 1850, 1860, "c.1860", true, true},
		{"FROM 1850 TO 1860", 1850, 1850, 1860, "1850", false, true},
		{"TO 1860", 1860, 1860, 1860, "1860", false, true},
		{"@#DJULIAN@ 1700", 1700, 1700, 1701, "1700", false, true},
		{"@#DHEBREW@ 5785", 2024, 2024, 2025, "2024", false, true},
		{"44 BC", -44, -44, -44, "44BC", false, true},
		{"(phrase)", 0, 0, 0, "", true, false},
		{"garbage", 0, 0, 0, "", true, false},
	}
	for _, tt := range tests {
		d := ParseDate(tt.in)
		if got := d.Year(); got != tt.year {
			t.Errorf("%q: Year = %d, want %d", tt.in, got, tt.year)
		}
		lo, hi, ok := d.YearRange()
		if ok != tt.validKey || (ok && (lo != tt.lo || hi != tt.hi)) {
			t.Errorf("%q: YearRange = %d,%d,%v want %d,%d,%v", tt.in, lo, hi, ok, tt.lo, tt.hi, tt.validKey)
		}
		if got := d.ShortYear(); got != tt.shortYr {
			t.Errorf("%q: ShortYear = %q, want %q", tt.in, got, tt.shortYr)
		}
		if got := d.IsApproximate(); got != tt.approx {
			t.Errorf("%q: IsApproximate = %v", tt.in, got)
		}
		if _, ok := d.Key(); ok != tt.validKey {
			t.Errorf("%q: Key ok = %v", tt.in, ok)
		}
	}

	// Month and year spans.
	first, last, _ := ParseDate("FEB 1900").Span()
	if last-first != 27 {
		t.Errorf("Feb 1900 has %d days", last-first+1)
	}
	first, last, _ = ParseDate("1904").Span()
	if last-first != 365 {
		t.Errorf("1904 has %d days", last-first+1)
	}
	first, last, _ = ParseDate("@#DFRENCH R@ 3").Span()
	if last-first != 365 {
		t.Errorf("French year III has %d days", last-first+1)
	}
	first, last, _ = ParseDate("@#DHEBREW@ ADS 5784").Span()
	if last-first != 28 {
		t.Errorf("Adar II 5784 has %d days", last-first+1)
	}
}

func TestDateOrdering(t *testing.T) {
	in := []string{
		"AFT 1850", "1851", "BEF 1850", "12 MAR 1850", "MAR 1850", "1850", "(phrase)",
		"@#DJULIAN@ 1 MAR 1850", "1849", "BET 1849 AND 1852", "@#DFRENCH R@ 1 VEND 1",
	}
	dates := make([]Date, len(in))
	for i, s := range in {
		dates[i] = ParseDate(s)
	}
	sort.SliceStable(dates, func(i, j int) bool { return dates[i].Compare(dates[j]) < 0 })
	var got []string
	for _, d := range dates {
		got = append(got, d.Raw)
	}
	want := []string{
		"@#DFRENCH R@ 1 VEND 1", "1849", "BET 1849 AND 1852", "BEF 1850", "1850", "MAR 1850",
		"12 MAR 1850", "@#DJULIAN@ 1 MAR 1850", "AFT 1850", "1851", "(phrase)",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %q\nwant    %q", got, want)
		}
	}
	if ParseDate("x").Compare(ParseDate("y")) != 0 {
		t.Error("two invalid dates compare equal")
	}
	if ParseDate("1850").Compare(ParseDate("1850")) != 0 {
		t.Error("equal dates")
	}
}

func TestDualDateUsesNewStyleYear(t *testing.T) {
	a := ParseDate("12 FEB 1750/51")
	b := ParseDate("12 FEB 1751")
	if a.Compare(b) != 0 {
		t.Errorf("1750/51 should sort as 1751")
	}
	if a.Year() != 1751 {
		t.Errorf("Year = %d", a.Year())
	}
}

func TestAgeBetween(t *testing.T) {
	tests := []struct {
		birth, at string
		want      string
		ok        bool
	}{
		{"12 MAR 1850", "11 MAR 1900", "49", true},
		{"12 MAR 1850", "12 MAR 1900", "50", true},
		{"29 FEB 1852", "28 FEB 1853", "0", true},
		{"1850", "1900", "~50", true},
		{"ABT 12 MAR 1850", "12 MAR 1900", "~50", true},
		{"MAR 1850", "12 FEB 1900", "~49", true},
		{"BET 1840 AND 1860", "1900", "~50", true},
		{"BEF 1850", "1900", "", false},
		{"1850", "AFT 1900", "", false},
		{"1850", "FROM 1870 TO 1880", "", false},
		{"1900", "1850", "", false},
		{"", "1900", "", false},
		{"1850", "(phrase)", "", false},
		{"1850", "1850", "~0", true},
	}
	for _, tt := range tests {
		age, ok := AgeBetween(ParseDate(tt.birth), ParseDate(tt.at))
		if ok != tt.ok || (ok && age.String() != tt.want) {
			t.Errorf("AgeBetween(%q, %q) = %v, %v; want %q, %v", tt.birth, tt.at, age, ok, tt.want, tt.ok)
		}
	}
}

func TestRoman(t *testing.T) {
	tests := map[int]string{1: "I", 4: "IV", 8: "VIII", 14: "XIV", 79: "LXXIX", 1994: "MCMXCIV", 0: "0", 4000: "4000"}
	for n, want := range tests {
		if got := roman(n); got != want {
			t.Errorf("roman(%d) = %q", n, got)
		}
	}
}

func TestFloorDivMod(t *testing.T) {
	tests := []struct{ a, b, q, m int }{{7, 2, 3, 1}, {-7, 2, -4, 1}, {7, -2, -4, -1}, {-8, 2, -4, 0}}
	for _, tt := range tests {
		if floorDiv(tt.a, tt.b) != tt.q || floorMod(tt.a, tt.b) != tt.m {
			t.Errorf("floorDiv/Mod(%d,%d) = %d,%d", tt.a, tt.b, floorDiv(tt.a, tt.b), floorMod(tt.a, tt.b))
		}
	}
}
