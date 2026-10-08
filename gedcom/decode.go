package gedcom

import (
	"bytes"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

// Encoding names the character set a GEDCOM file was decoded from.
type Encoding string

// Supported encodings.
const (
	EncodingUTF8      Encoding = "UTF-8"
	EncodingUTF16LE   Encoding = "UTF-16LE"
	EncodingUTF16BE   Encoding = "UTF-16BE"
	EncodingANSEL     Encoding = "ANSEL"
	EncodingASCII     Encoding = "ASCII"
	EncodingCP1252    Encoding = "CP1252"
	EncodingLatin1    Encoding = "ISO-8859-1"
	EncodingCP437     Encoding = "CP437"
	EncodingCP850     Encoding = "CP850"
	EncodingMacintosh Encoding = "MACINTOSH"
)

var (
	charRe = regexp.MustCompile(`(?m)^[ \t]*1[ \t]+CHAR[ \t]+([^\r\n]*)`)
	versRe = regexp.MustCompile(`(?m)^[ \t]*2[ \t]+VERS[ \t]+([^\r\n]*)`)
)

// headerPrefix returns the part of data that belongs to the HEAD record (or
// the first 64 KiB, whichever is shorter). Only ASCII-compatible encodings
// are inspected here; UTF-16 is recognized earlier.
func headerPrefix(data []byte) []byte {
	if len(data) > 64<<10 {
		data = data[:64<<10]
	}
	// The header ends where the second level-0 record starts.
	for i := 1; i < len(data); i++ {
		if data[i-1] != '\n' && data[i-1] != '\r' {
			continue
		}
		j := i
		for j < len(data) && (data[j] == ' ' || data[j] == '\t') {
			j++
		}
		if j+1 < len(data) && data[j] == '0' && (data[j+1] == ' ' || data[j+1] == '\t') {
			return data[:i]
		}
	}
	return data
}

// declaredCharset returns the value of HEAD.CHAR and the GEDCOM version
// declared in HEAD.GEDC.VERS.
func declaredCharset(data []byte) (charset, version string) {
	head := headerPrefix(data)
	if m := charRe.FindSubmatch(head); m != nil {
		charset = strings.ToUpper(strings.TrimSpace(string(m[1])))
	}
	if m := versRe.FindSubmatch(head); m != nil {
		version = strings.TrimSpace(string(m[1]))
	}
	return charset, version
}

func isASCII(data []byte) bool {
	for _, c := range data {
		if c >= 0x80 {
			return false
		}
	}
	return true
}

func decodeWith(enc encoding.Encoding, data []byte) string {
	out, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		return strings.ToValidUTF8(string(data), "\uFFFD")
	}
	return string(out)
}

// Decode detects the character encoding of a GEDCOM file and converts it to
// UTF-8. Detection uses, in order: a byte order mark, the UTF-16 signature of
// the leading "0 HEAD" line, the HEAD.CHAR declaration and finally a UTF-8
// validity check. Problems such as a declaration that contradicts the data
// are returned as warnings.
func Decode(data []byte) (string, Encoding, []string) {
	var warnings []string
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		data = data[3:]
		if !utf8.Valid(data) {
			warnings = append(warnings, "file contains invalid UTF-8 sequences; they were replaced")
		}
		return strings.ToValidUTF8(string(data), "\uFFFD"), EncodingUTF8, warnings
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
		return decodeWith(unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM), data[2:]), EncodingUTF16LE, nil
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		return decodeWith(unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM), data[2:]), EncodingUTF16BE, nil
	case len(data) >= 2 && data[0] == '0' && data[1] == 0:
		return decodeWith(unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM), data), EncodingUTF16LE, nil
	case len(data) >= 2 && data[0] == 0 && data[1] == '0':
		return decodeWith(unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM), data), EncodingUTF16BE, nil
	}

	charset, version := declaredCharset(data)
	ascii := isASCII(data)
	validUTF8 := utf8.Valid(data)

	if strings.HasPrefix(version, "7") {
		// GEDCOM 7 files are always UTF-8.
		charset = "UTF-8"
	}

	// A file declared as some legacy character set but containing valid,
	// non-ASCII UTF-8 was almost certainly written as UTF-8. Random legacy
	// text is very unlikely to form valid multi-byte UTF-8 sequences.
	legacy := func(name Encoding, enc func([]byte) string) (string, Encoding, []string) {
		if ascii {
			return string(data), name, warnings
		}
		if validUTF8 {
			warnings = append(warnings, "file declares character set "+charset+" but contains UTF-8; decoded as UTF-8")
			return string(data), EncodingUTF8, warnings
		}
		return enc(data), name, warnings
	}

	switch charset {
	case "UTF-8", "UTF8", "UNICODE":
		if !validUTF8 {
			warnings = append(warnings, "file declares UTF-8 but contains invalid UTF-8; decoded as Windows-1252")
			return decodeWith(charmap.Windows1252, data), EncodingCP1252, warnings
		}
		return string(data), EncodingUTF8, warnings
	case "ANSEL":
		return legacy(EncodingANSEL, DecodeANSEL)
	case "ANSI", "WINDOWS-1252", "CP1252", "WINDOWS", "IBM WINDOWS", "IBM_WINDOWS":
		return legacy(EncodingCP1252, func(b []byte) string { return decodeWith(charmap.Windows1252, b) })
	case "ISO-8859-1", "ISO8859-1", "ISO-8859", "LATIN1", "LATIN-1":
		return legacy(EncodingLatin1, func(b []byte) string { return decodeWith(charmap.ISO8859_1, b) })
	case "IBMPC", "IBM-PC", "CP437", "DOS":
		return legacy(EncodingCP437, func(b []byte) string { return decodeWith(charmap.CodePage437, b) })
	case "CP850":
		return legacy(EncodingCP850, func(b []byte) string { return decodeWith(charmap.CodePage850, b) })
	case "MACINTOSH", "MACROMAN", "MAC":
		return legacy(EncodingMacintosh, func(b []byte) string { return decodeWith(charmap.Macintosh, b) })
	case "ASCII", "":
		if ascii {
			return string(data), EncodingASCII, warnings
		}
		if validUTF8 {
			if charset != "" {
				warnings = append(warnings, "file declares ASCII but contains UTF-8; decoded as UTF-8")
			}
			return string(data), EncodingUTF8, warnings
		}
		warnings = append(warnings, "file contains non-ASCII bytes that are not UTF-8; decoded as Windows-1252")
		return decodeWith(charmap.Windows1252, data), EncodingCP1252, warnings
	default:
		warnings = append(warnings, "unknown character set "+charset+"; guessing")
		if validUTF8 {
			return string(data), EncodingUTF8, warnings
		}
		return decodeWith(charmap.Windows1252, data), EncodingCP1252, warnings
	}
}
