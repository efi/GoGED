package gedcom

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func header(charset string) string {
	h := "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n"
	if charset != "" {
		h += "1 CHAR " + charset + "\n"
	}
	return h
}

func encodeUTF16(s string, bigEndian, bom bool) []byte {
	var out []byte
	units := utf16.Encode([]rune(s))
	if bom {
		units = append([]uint16{0xFEFF}, units...)
	}
	for _, u := range units {
		if bigEndian {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	return out
}

func TestDecode(t *testing.T) {
	body := "0 @I1@ INDI\n1 NAME José /Müller/\n0 TRLR\n"
	tests := []struct {
		name     string
		data     []byte
		wantEnc  Encoding
		wantText string // substring expected in the decoded text
		warnings int
	}{
		{"utf8 declared", []byte(header("UTF-8") + body), EncodingUTF8, "José /Müller/", 0},
		{"utf8 bom", append([]byte{0xEF, 0xBB, 0xBF}, []byte(header("UTF-8")+body)...), EncodingUTF8, "José", 0},
		{"utf8 undeclared", []byte(header("") + body), EncodingUTF8, "Müller", 0},
		{"utf8 declared ansel", []byte(header("ANSEL") + body), EncodingUTF8, "Müller", 1},
		{"utf8 declared ascii", []byte(header("ASCII") + body), EncodingUTF8, "Müller", 1},
		{"utf8 declared ansi", []byte(header("ANSI") + body), EncodingUTF8, "Müller", 1},
		{"pure ascii", []byte(header("ASCII") + "0 @I1@ INDI\n1 NAME Jo /Smith/\n"), EncodingASCII, "Jo /Smith/", 0},
		{"cp1252", []byte(header("ANSI") + "0 @I1@ INDI\n1 NAME Jos\xe9 /M\xfcller/ \x80\n"), EncodingCP1252, "José /Müller/ €", 0},
		{"latin1", []byte(header("ISO-8859-1") + "1 NAME Jos\xe9\n"), EncodingLatin1, "José", 0},
		{"ibmpc", []byte(header("IBMPC") + "1 NAME M\x81ller\n"), EncodingCP437, "Müller", 0},
		{"macintosh", []byte(header("MACINTOSH") + "1 NAME M\x9fller\n"), EncodingMacintosh, "Müller", 0},
		{"ansel", []byte(header("ANSEL") + "1 NAME Jos\xe2e /M\xe8uller/ \xa1\xb2d\xc7\n"), EncodingANSEL, "José /Müller/ Łødß", 0},
		{"invalid utf8 declared utf8", []byte(header("UTF-8") + "1 NAME Jos\xe9\n"), EncodingCP1252, "José", 1},
		{"undeclared latin bytes", []byte(header("") + "1 NAME Jos\xe9\n"), EncodingCP1252, "José", 1},
		{"unknown charset", []byte(header("KLINGON") + body), EncodingUTF8, "Müller", 1},
		{"gedcom 7 always utf8", []byte("0 HEAD\n1 GEDC\n2 VERS 7.0\n" + body), EncodingUTF8, "Müller", 0},
		{"utf16le bom", encodeUTF16(header("UNICODE")+body, false, true), EncodingUTF16LE, "José /Müller/", 0},
		{"utf16be bom", encodeUTF16(header("UNICODE")+body, true, true), EncodingUTF16BE, "José /Müller/", 0},
		{"utf16le no bom", encodeUTF16(header("UNICODE")+body, false, false), EncodingUTF16LE, "José /Müller/", 0},
		{"utf16be no bom", encodeUTF16(header("UNICODE")+body, true, false), EncodingUTF16BE, "José /Müller/", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, enc, warnings := Decode(tt.data)
			if enc != tt.wantEnc {
				t.Errorf("encoding = %s, want %s", enc, tt.wantEnc)
			}
			if !strings.Contains(text, tt.wantText) {
				t.Errorf("decoded text %q does not contain %q", text, tt.wantText)
			}
			if len(warnings) != tt.warnings {
				t.Errorf("warnings = %q, want %d", warnings, tt.warnings)
			}
		})
	}
}

func TestDecodeOnlyInspectsHeader(t *testing.T) {
	// A CHAR line in a later record must not override the header.
	data := []byte("0 HEAD\n1 CHAR ASCII\n0 @I1@ INDI\n1 CHAR ANSEL\n1 NAME x\n")
	_, enc, _ := Decode(data)
	if enc != EncodingASCII {
		t.Errorf("encoding = %s, want ASCII", enc)
	}
}

func TestDecodeANSEL(t *testing.T) {
	tests := []struct {
		in   []byte
		want string
	}{
		{[]byte("plain"), "plain"},
		{[]byte("Fran\xf0cois"), "François"},
		{[]byte("\xeaAngstr\xe8om"), "Ångström"},
		{[]byte("Dvo\xe9r\xe2ak"), "Dvořák"},
		{[]byte("\xe9S\xe9cedrova"), "Ščedrova"},
		{[]byte("\xa5sir \xb6uvre"), "Æsir œuvre"},
		{[]byte("Stra\xcfe"), "Straße"},
		{[]byte("\xc3 2026 \xb9"), "© 2026 £"},
		{[]byte("ng\xe4u\xe2y\xe3en"), "ngũýên"},
		{[]byte("x\xe8\ny\xe8"), "x\ny"},
		{[]byte{0x80}, "\uFFFD"},
		{[]byte("a\x88b\x89c"), "abc"},
		{[]byte("\xe3\xe2a"), "ấ"}, // a + circumflex + acute = ấ
	}
	for _, tt := range tests {
		if got := DecodeANSEL(tt.in); got != tt.want {
			t.Errorf("DecodeANSEL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
