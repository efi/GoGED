package search

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Field names a searchable property.
type Field string

// Searchable fields. FieldText is used for terms without a field prefix.
const (
	FieldText       Field = ""
	FieldName       Field = "name"
	FieldGiven      Field = "given"
	FieldSurname    Field = "surname"
	FieldBorn       Field = "born"
	FieldDied       Field = "died"
	FieldPlace      Field = "place"
	FieldYear       Field = "year"
	FieldAlive      Field = "alive"
	FieldSex        Field = "sex"
	FieldID         Field = "id"
	FieldOccupation Field = "occupation"
	FieldNote       Field = "note"
	FieldSource     Field = "source"
	FieldAny        Field = "any"
	FieldTag        Field = "tag"
	FieldHas        Field = "has"
	FieldSounds     Field = "sounds"
	FieldType       Field = "type" // event filters only
)

var fieldAliases = map[string]Field{
	"name": FieldName, "n": FieldName,
	"given": FieldGiven, "first": FieldGiven, "g": FieldGiven,
	"surname": FieldSurname, "sur": FieldSurname, "last": FieldSurname, "s": FieldSurname,
	"born": FieldBorn, "birth": FieldBorn, "b": FieldBorn,
	"died": FieldDied, "death": FieldDied, "d": FieldDied,
	"place": FieldPlace, "at": FieldPlace, "p": FieldPlace,
	"year": FieldYear, "y": FieldYear,
	"alive": FieldAlive, "living": FieldAlive,
	"sex": FieldSex, "gender": FieldSex,
	"id":         FieldID,
	"occupation": FieldOccupation, "occu": FieldOccupation, "job": FieldOccupation,
	"note": FieldNote, "notes": FieldNote,
	"source": FieldSource, "src": FieldSource, "sour": FieldSource,
	"any": FieldAny, "text": FieldAny, "all": FieldAny,
	"tag":    FieldTag,
	"has":    FieldHas,
	"sounds": FieldSounds, "soundex": FieldSounds,
	"type": FieldType, "t": FieldType,
}

// HasValues lists the accepted values of the has: field.
var HasValues = []string{"birth", "death", "parents", "father", "mother", "spouse", "children", "siblings", "notes", "sources", "media", "occupation"}

// Term is one condition of a query.
type Term struct {
	Field  Field
	Value  string // the value as typed (without quotes)
	Negate bool

	folded  string
	words   []string // words of folded, split like names, for name terms
	years   yearRange
	isYears bool     // Value is a year specification
	tagPath []string // for tag: terms
	tagVal  string   // folded value after '=' for tag: terms
	sex     string
	soundex string
	cologne string
}

// Query is a parsed search query: all terms must match.
type Query struct {
	Raw   string
	Terms []Term
}

// IsEmpty reports whether the query has no terms.
func (q Query) IsEmpty() bool { return len(q.Terms) == 0 }

// ParseError reports a problem with a query.
type ParseError struct{ Msg string }

func (e *ParseError) Error() string { return e.Msg }

// yearRange is an inclusive range of years; open ends use the extremes.
type yearRange struct{ lo, hi int }

func (r yearRange) intersects(lo, hi int) bool { return lo <= r.hi && hi >= r.lo }

const (
	minYear = math.MinInt32
	maxYear = math.MaxInt32
)

// parseYearRange understands "1850", "1850..1860", "1850-1860", "1850..",
// "..1860", "<1850", "<=1850", ">1850", ">=1850", "~1850" (±5 years) and
// decades such as "1850s".
func parseYearRange(s string) (yearRange, bool) {
	num := func(t string) (int, bool) {
		if t == "" || len(t) > 5 {
			return 0, false
		}
		n, err := strconv.Atoi(t)
		return n, err == nil && n >= 0
	}
	switch {
	case strings.HasPrefix(s, "<="):
		n, ok := num(s[2:])
		return yearRange{minYear, n}, ok
	case strings.HasPrefix(s, ">="):
		n, ok := num(s[2:])
		return yearRange{n, maxYear}, ok
	case strings.HasPrefix(s, "<"):
		n, ok := num(s[1:])
		return yearRange{minYear, n - 1}, ok
	case strings.HasPrefix(s, ">"):
		n, ok := num(s[1:])
		return yearRange{n + 1, maxYear}, ok
	case strings.HasPrefix(s, "~"):
		n, ok := num(s[1:])
		return yearRange{n - 5, n + 5}, ok
	case strings.HasSuffix(s, "s") && len(s) == 5:
		n, ok := num(s[:4])
		if !ok || n%10 != 0 {
			return yearRange{}, false
		}
		return yearRange{n, n + 9}, true
	}
	for _, sep := range []string{"..", "-"} {
		if a, b, found := strings.Cut(s, sep); found {
			lo, hi := minYear, maxYear
			var ok1, ok2 = true, true
			if a != "" {
				lo, ok1 = num(a)
			}
			if b != "" {
				hi, ok2 = num(b)
			}
			if !ok1 || !ok2 || (a == "" && b == "") {
				return yearRange{}, false
			}
			if lo > hi {
				lo, hi = hi, lo
			}
			return yearRange{lo, hi}, true
		}
	}
	n, ok := num(s)
	return yearRange{n, n}, ok
}

// looksLikeYear reports whether a bare term should be treated as a year
// specification rather than as text.
func looksLikeYear(s string) bool {
	if s == "" {
		return false
	}
	if _, ok := parseYearRange(s); !ok {
		return false
	}
	// Plain numbers must have three or four digits to count as years.
	if n, err := strconv.Atoi(s); err == nil {
		return n >= 100 && n <= 9999
	}
	return true
}

// tokenize splits a query into tokens, honoring double quotes.
func tokenize(s string) ([]string, error) {
	var toks []string
	var cur strings.Builder
	inQuote, have := false, false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			have = true
		case !inQuote && (r == ' ' || r == '\t' || r == '\n'):
			if have {
				toks = append(toks, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteRune(r)
			have = true
		}
	}
	if inQuote {
		return nil, &ParseError{"unterminated quote"}
	}
	if have {
		toks = append(toks, cur.String())
	}
	return toks, nil
}

// Parse parses a query string. Terms are separated by spaces and must all
// match. A term is either text, matched against names, or field:value.
// A leading '-' negates a term and a leading '~' matches names by sound.
func Parse(s string) (Query, error) {
	q := Query{Raw: s}
	toks, err := tokenize(s)
	if err != nil {
		return q, err
	}
	for _, tok := range toks {
		t, err := parseTerm(tok)
		if err != nil {
			return Query{Raw: s}, err
		}
		if t != nil {
			q.Terms = append(q.Terms, *t)
		}
	}
	return q, nil
}

func parseTerm(tok string) (*Term, error) {
	t := &Term{}
	if strings.HasPrefix(tok, "-") && len(tok) > 1 {
		t.Negate = true
		tok = tok[1:]
	}
	if strings.HasPrefix(tok, "~") && len(tok) > 1 && !looksLikeYear(tok) {
		t.Field = FieldSounds
		tok = tok[1:]
	} else if name, value, ok := strings.Cut(tok, ":"); ok && name != "" && !strings.ContainsAny(name, " \"") {
		f, known := fieldAliases[strings.ToLower(name)]
		if !known {
			return nil, &ParseError{fmt.Sprintf("unknown field %q (fields: %s)", name, strings.Join(fields(), ", "))}
		}
		t.Field = f
		tok = value
	}
	t.Value = strings.TrimSpace(tok)
	if t.Value == "" {
		if t.Field == FieldText {
			return nil, nil
		}
		return nil, &ParseError{fmt.Sprintf("missing value for %s:", t.Field)}
	}
	t.folded = Fold(t.Value)
	t.words = words(t.folded)

	switch t.Field {
	case FieldText:
		if looksLikeYear(t.Value) {
			t.years, t.isYears = parseYearRange(t.Value)
		}
	case FieldBorn, FieldDied:
		t.years, t.isYears = parseYearRange(t.Value)
	case FieldYear, FieldAlive:
		var ok bool
		if t.years, ok = parseYearRange(t.Value); !ok {
			return nil, &ParseError{fmt.Sprintf("%s: expects a year or range such as 1850 or 1800..1850", t.Field)}
		}
		t.isYears = true
	case FieldSex:
		switch t.folded {
		case "m", "male", "man":
			t.sex = "M"
		case "f", "female", "woman":
			t.sex = "F"
		case "u", "unknown", "?":
			t.sex = ""
		case "x", "intersex":
			t.sex = "X"
		default:
			return nil, &ParseError{fmt.Sprintf("sex: expects m, f, u or x, not %q", t.Value)}
		}
	case FieldID:
		t.folded = strings.Trim(t.folded, "@")
	case FieldHas:
		valid := false
		for _, v := range HasValues {
			if t.folded == v {
				valid = true
			}
		}
		if !valid {
			return nil, &ParseError{fmt.Sprintf("has: expects one of %s", strings.Join(HasValues, ", "))}
		}
	case FieldTag:
		path, val, _ := strings.Cut(t.Value, "=")
		for _, p := range strings.Split(strings.ToUpper(path), ".") {
			if p == "" {
				return nil, &ParseError{"tag: expects TAG, TAG.SUBTAG or TAG=value"}
			}
			t.tagPath = append(t.tagPath, p)
		}
		t.tagVal = Fold(val)
	case FieldSounds:
		t.soundex, t.cologne = Soundex(t.Value), Cologne(t.Value)
		if t.soundex == "" {
			return nil, &ParseError{"sounds: expects a name"}
		}
	}
	return t, nil
}

// String renders the query in canonical form, mainly for debugging.
func (q Query) String() string {
	parts := make([]string, 0, len(q.Terms))
	for _, t := range q.Terms {
		s := t.Value
		if strings.ContainsAny(s, " \t") {
			s = `"` + s + `"`
		}
		if t.Field != FieldText {
			s = string(t.Field) + ":" + s
		}
		if t.Negate {
			s = "-" + s
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

// fields returns the canonical field names usable in person queries.
func fields() []string {
	seen := map[Field]bool{FieldType: true}
	var out []string
	for _, f := range fieldAliases {
		if !seen[f] {
			seen[f] = true
			out = append(out, string(f))
		}
	}
	sort.Strings(out)
	return out
}
