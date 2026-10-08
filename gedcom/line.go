// Package gedcom parses GEDCOM genealogy files (versions 5.5, 5.5.1 and 7.0)
// into a generic node tree and a typed model of individuals, families,
// events, names, dates and places.
//
// The parser is deliberately lenient: real-world GEDCOM files are frequently
// malformed, so recoverable problems are reported as warnings on the
// Document instead of aborting the parse.
package gedcom

import (
	"fmt"
	"strings"
)

// Line is a single logical GEDCOM line as it appears in the file.
type Line struct {
	Num   int    // 1-based line number in the source file
	Level int    // hierarchical level number
	Xref  string // cross-reference identifier including the '@' delimiters, e.g. "@I1@"
	Tag   string // upper-cased tag, e.g. "INDI" or "_MILT"
	Value string // line value with "@@" escapes resolved (pointers are kept verbatim)
}

// SyntaxError describes a line that could not be parsed.
type SyntaxError struct {
	Line int
	Msg  string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
}

// ParseLine parses a single GEDCOM line. The line number is only used for
// error reporting. Leading whitespace is ignored, as recommended by the
// GEDCOM 5.5.1 specification; the value keeps any trailing whitespace since
// it may be significant when lines are joined with CONC.
func ParseLine(s string, num int) (Line, error) {
	l := Line{Num: num}
	s = strings.TrimLeft(s, " \t\ufeff")
	if s == "" {
		return l, &SyntaxError{num, "empty line"}
	}

	// Level number.
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return l, &SyntaxError{num, "missing level number"}
	}
	if i > 2 {
		return l, &SyntaxError{num, "level number too large"}
	}
	for _, c := range s[:i] {
		l.Level = l.Level*10 + int(c-'0')
	}
	rest := s[i:]
	if rest == "" || !isDelim(rest[0]) {
		return l, &SyntaxError{num, "missing delimiter after level number"}
	}
	rest = trimDelims(rest)

	// Optional cross-reference identifier.
	if strings.HasPrefix(rest, "@") {
		end := strings.IndexAny(rest, " \t")
		if end < 0 {
			return l, &SyntaxError{num, "missing tag after cross-reference identifier"}
		}
		xref := rest[:end]
		if len(xref) < 3 || !strings.HasSuffix(xref, "@") {
			return l, &SyntaxError{num, fmt.Sprintf("malformed cross-reference identifier %q", xref)}
		}
		l.Xref = xref
		rest = trimDelims(rest[end:])
	}

	// Tag.
	end := strings.IndexAny(rest, " \t")
	tag := rest
	if end >= 0 {
		tag = rest[:end]
	}
	if tag == "" {
		return l, &SyntaxError{num, "missing tag"}
	}
	for _, c := range tag {
		if !(c == '_' || c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z') {
			return l, &SyntaxError{num, fmt.Sprintf("invalid character %q in tag", c)}
		}
	}
	l.Tag = strings.ToUpper(tag)

	// Value: everything after the single delimiter following the tag.
	if end >= 0 {
		l.Value = rest[end+1:]
		if !IsPointer(l.Value) {
			l.Value = strings.ReplaceAll(l.Value, "@@", "@")
		}
	}
	return l, nil
}

func isDelim(c byte) bool { return c == ' ' || c == '\t' }

func trimDelims(s string) string { return strings.TrimLeft(s, " \t") }

// IsPointer reports whether a line value is a cross-reference pointer such
// as "@I1@". Escape sequences like "@#DJULIAN@" are not pointers.
func IsPointer(v string) bool {
	if len(v) < 3 || v[0] != '@' || v[len(v)-1] != '@' || v[1] == '#' || v[1] == '@' {
		return false
	}
	return !strings.ContainsAny(v[1:len(v)-1], "@ \t")
}

// StripXref removes the '@' delimiters from a cross-reference identifier.
// "@I1@" becomes "I1"; identifiers without delimiters are returned unchanged.
func StripXref(x string) string {
	x = strings.TrimSpace(x)
	if len(x) >= 2 && x[0] == '@' && x[len(x)-1] == '@' {
		return x[1 : len(x)-1]
	}
	return x
}

// forEachLine calls fn for every line in text, accepting LF, CRLF and bare
// CR line terminators. Line numbers start at 1.
func forEachLine(text string, fn func(num int, line string)) {
	num := 1
	for len(text) > 0 {
		i := strings.IndexAny(text, "\r\n")
		if i < 0 {
			fn(num, text)
			return
		}
		fn(num, text[:i])
		if text[i] == '\r' && i+1 < len(text) && text[i+1] == '\n' {
			i++
		}
		text = text[i+1:]
		num++
	}
}
