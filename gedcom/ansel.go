package gedcom

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// anselSpacing maps the ANSEL (ANSI/NISO Z39.47) spacing graphic characters,
// including the GEDCOM-specific extensions, to Unicode.
var anselSpacing = map[byte]rune{
	0xA1: 'Ł', 0xA2: 'Ø', 0xA3: 'Đ', 0xA4: 'Þ', 0xA5: 'Æ', 0xA6: 'Œ',
	0xA7: 'ʹ', 0xA8: '·', 0xA9: '♭', 0xAA: '®', 0xAB: '±', 0xAC: 'Ơ',
	0xAD: 'Ư', 0xAE: 'ʼ', 0xB0: 'ʻ', 0xB1: 'ł', 0xB2: 'ø', 0xB3: 'đ',
	0xB4: 'þ', 0xB5: 'æ', 0xB6: 'œ', 0xB7: 'ʺ', 0xB8: 'ı', 0xB9: '£',
	0xBA: 'ð', 0xBC: 'ơ', 0xBD: 'ư', 0xBE: '□', 0xBF: '■', 0xC0: '°',
	0xC1: 'ℓ', 0xC2: '℗', 0xC3: '©', 0xC4: '♯', 0xC5: '¿', 0xC6: '¡',
	0xC7: 'ß', 0xC8: '€', 0xCD: 'e', 0xCE: 'o', 0xCF: 'ß',
}

// anselCombining maps ANSEL combining diacritics to Unicode combining marks.
// In ANSEL the diacritic precedes the base character; in Unicode it follows.
var anselCombining = map[byte]rune{
	0xE0: '\u0309', // hook above
	0xE1: '\u0300', // grave
	0xE2: '\u0301', // acute
	0xE3: '\u0302', // circumflex
	0xE4: '\u0303', // tilde
	0xE5: '\u0304', // macron
	0xE6: '\u0306', // breve
	0xE7: '\u0307', // dot above
	0xE8: '\u0308', // diaeresis
	0xE9: '\u030C', // caron
	0xEA: '\u030A', // ring above
	0xEB: '\uFE20', // ligature left half
	0xEC: '\uFE21', // ligature right half
	0xED: '\u0315', // comma above right
	0xEE: '\u030B', // double acute
	0xEF: '\u0310', // candrabindu
	0xF0: '\u0327', // cedilla
	0xF1: '\u0328', // ogonek
	0xF2: '\u0323', // dot below
	0xF3: '\u0324', // diaeresis below
	0xF4: '\u0325', // ring below
	0xF5: '\u0333', // double low line
	0xF6: '\u0332', // low line
	0xF7: '\u0326', // comma below
	0xF8: '\u031C', // left half ring below
	0xF9: '\u032E', // breve below
	0xFA: '\uFE22', // double tilde left half
	0xFB: '\uFE23', // double tilde right half
	0xFE: '\u0313', // comma above
}

// DecodeANSEL converts ANSEL-encoded bytes to a UTF-8 string in Unicode
// normalization form C. Bytes without an ANSEL mapping become U+FFFD.
func DecodeANSEL(data []byte) string {
	var b strings.Builder
	b.Grow(len(data))
	var pending []rune // combining marks waiting for their base character
	flush := func() {
		for _, r := range pending {
			b.WriteRune(r)
		}
		pending = pending[:0]
	}
	// A diacritic without a following base character is malformed; it is
	// dropped rather than attached to an unrelated character.
	drop := func() { pending = pending[:0] }
	for _, c := range data {
		switch {
		case c < 0x80:
			if c == '\r' || c == '\n' {
				// Diacritics never span lines.
				drop()
				b.WriteByte(c)
				continue
			}
			b.WriteByte(c)
			flush()
		case anselCombining[c] != 0:
			pending = append(pending, anselCombining[c])
		case anselSpacing[c] != 0:
			b.WriteRune(anselSpacing[c])
			flush()
		case c == 0x88 || c == 0x89 || c == 0x8D || c == 0x8E:
			// Non-sorting markers and joiners carry no visible text.
		default:
			b.WriteRune('\uFFFD')
			flush()
		}
	}
	drop()
	return norm.NFC.String(b.String())
}
