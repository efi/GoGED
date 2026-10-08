// Package search implements the query language used to find individuals
// and events in a GEDCOM document.
package search

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// special lists letters that do not decompose into a base letter plus
// combining marks but have a conventional ASCII transliteration.
var special = map[rune]string{
	'ß': "ss", 'ẞ': "ss",
	'æ': "ae", 'Æ': "ae",
	'œ': "oe", 'Œ': "oe",
	'ø': "o", 'Ø': "o",
	'ł': "l", 'Ł': "l",
	'đ': "d", 'Đ': "d",
	'ð': "d", 'Ð': "d",
	'þ': "th", 'Þ': "th",
	'ı': "i",
	'ħ': "h", 'Ħ': "h",
	'ŀ': "l", 'Ŀ': "l",
	'ŧ': "t", 'Ŧ': "t",
	'ʼ': "'", '’': "'", '‘': "'",
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// foldCache memoizes the folded form of non-ASCII runes.
var foldCache sync.Map // rune -> string

// foldRune returns the folded form of a single non-ASCII rune.
func foldRune(r rune) string {
	if v, ok := foldCache.Load(r); ok {
		return v.(string)
	}
	var b strings.Builder
	if t, ok := special[r]; ok {
		b.WriteString(t)
	} else {
		for _, d := range norm.NFKD.String(string(r)) {
			if unicode.Is(unicode.Mn, d) {
				continue
			}
			if t, ok := special[d]; ok {
				b.WriteString(t)
				continue
			}
			b.WriteRune(unicode.ToLower(d))
		}
	}
	out := b.String()
	foldCache.Store(r, out)
	return out
}

// Fold normalizes text for matching: it lower-cases, strips diacritics
// ("Müller" -> "muller"), expands compatibility characters such as
// ligatures ("ﬁ" -> "fi") and transliterates letters such as ß, æ and ø.
func Fold(s string) string {
	if isASCII(s) {
		return strings.ToLower(s)
	}
	// Precomposed input is folded rune by rune; decomposed input (base
	// letters followed by combining marks) is composed first.
	if !norm.NFC.IsNormalString(s) {
		s = norm.NFC.String(s)
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r < utf8.RuneSelf:
			if 'A' <= r && r <= 'Z' {
				r += 'a' - 'A'
			}
			b.WriteByte(byte(r))
		case unicode.Is(unicode.Mn, r):
			// Stray combining marks.
		default:
			b.WriteString(foldRune(r))
		}
	}
	return b.String()
}

// words splits folded text into words on anything that is not a letter or
// digit (apostrophes are dropped so "O'Brien" yields "obrien").
func words(s string) []string {
	s = strings.ReplaceAll(s, "'", "")
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// Soundex returns the American Soundex code of a name, e.g. "R163" for
// "Robert". Non-letters are ignored; the result is empty if s contains no
// letters.
func Soundex(s string) string {
	codes := [26]byte{
		'0', '1', '2', '3', '0', '1', '2', 'h', '0', '2', '2', '4', '5',
		'5', '0', '1', '2', '6', '2', '3', '0', '1', 'h', '2', '0', '2',
	}
	var out []byte
	var last byte
	for _, r := range Fold(s) {
		if r < 'a' || r > 'z' {
			continue
		}
		c := codes[r-'a']
		if out == nil {
			out = append(out, byte(unicode.ToUpper(r)))
			last = c
			continue
		}
		switch c {
		case 'h': // h and w do not separate letters with the same code
			continue
		case '0': // vowels separate letters with the same code
			last = '0'
			continue
		}
		if c != last {
			out = append(out, c)
			if len(out) == 4 {
				break
			}
		}
		last = c
	}
	if out == nil {
		return ""
	}
	for len(out) < 4 {
		out = append(out, '0')
	}
	return string(out[:4])
}
