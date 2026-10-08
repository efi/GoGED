package gedcom

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseLine(t *testing.T) {
	tests := []struct {
		in   string
		want Line
	}{
		{"0 HEAD", Line{Level: 0, Tag: "HEAD"}},
		{"0 @I1@ INDI", Line{Level: 0, Xref: "@I1@", Tag: "INDI"}},
		{"1 NAME John /Smith/", Line{Level: 1, Tag: "NAME", Value: "John /Smith/"}},
		{"1 FAMS @F1@", Line{Level: 1, Tag: "FAMS", Value: "@F1@"}},
		{"2 DATE @#DJULIAN@ 1 JAN 1700", Line{Level: 2, Tag: "DATE", Value: "@#DJULIAN@ 1 JAN 1700"}},
		{"1 EMAIL john@@example.com", Line{Level: 1, Tag: "EMAIL", Value: "john@example.com"}},
		{"  2 PLAC Leeds", Line{Level: 2, Tag: "PLAC", Value: "Leeds"}},
		{"\t3 PAGE p. 4", Line{Level: 3, Tag: "PAGE", Value: "p. 4"}},
		{"1 _MILT", Line{Level: 1, Tag: "_MILT"}},
		{"1 birt Y", Line{Level: 1, Tag: "BIRT", Value: "Y"}},
		{"2 CONC  leading space kept", Line{Level: 2, Tag: "CONC", Value: " leading space kept"}},
		{"2 CONC trailing space kept ", Line{Level: 2, Tag: "CONC", Value: "trailing space kept "}},
		{"1 NOTE", Line{Level: 1, Tag: "NOTE"}},
		{"1 NOTE ", Line{Level: 1, Tag: "NOTE", Value: ""}},
		{"10 TAG x", Line{Level: 10, Tag: "TAG", Value: "x"}},
		{"0  @I2@  INDI", Line{Level: 0, Xref: "@I2@", Tag: "INDI"}},
		{"\uFEFF0 HEAD", Line{Level: 0, Tag: "HEAD"}},
	}
	for _, tt := range tests {
		got, err := ParseLine(tt.in, 7)
		if err != nil {
			t.Errorf("ParseLine(%q) error: %v", tt.in, err)
			continue
		}
		tt.want.Num = 7
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseLine(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestParseLineErrors(t *testing.T) {
	bad := []string{
		"",
		"   ",
		"HEAD",
		"x 1 NAME",
		"1",
		"1NAME",
		"123 NAME",
		"0 @I1@",
		"0 @I1 INDI",
		"0 @@ INDI",
		"1 NA-ME x",
		"1 NÄME x",
	}
	for _, in := range bad {
		_, err := ParseLine(in, 3)
		if err == nil {
			t.Errorf("ParseLine(%q): expected error", in)
			continue
		}
		var se *SyntaxError
		if !errors.As(err, &se) || se.Line != 3 {
			t.Errorf("ParseLine(%q): error %v is not a *SyntaxError for line 3", in, err)
		}
	}
}

func TestIsPointer(t *testing.T) {
	tests := map[string]bool{
		"@I1@":          true,
		"@F_23@":        true,
		"@VOID@":        true,
		"@#DJULIAN@":    false,
		"@@":            false,
		"@I1":           false,
		"I1@":           false,
		"@I 1@":         false,
		"@I1@ extra":    false,
		"":              false,
		"@@I1@":         false,
		"mail@host.com": false,
	}
	for in, want := range tests {
		if got := IsPointer(in); got != want {
			t.Errorf("IsPointer(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestStripXref(t *testing.T) {
	tests := map[string]string{"@I1@": "I1", "I1": "I1", " @F2@ ": "F2", "@": "@", "": ""}
	for in, want := range tests {
		if got := StripXref(in); got != want {
			t.Errorf("StripXref(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestForEachLine(t *testing.T) {
	type numbered struct {
		n int
		s string
	}
	tests := []struct {
		in   string
		want []numbered
	}{
		{"a\nb\nc", []numbered{{1, "a"}, {2, "b"}, {3, "c"}}},
		{"a\r\nb\r\n", []numbered{{1, "a"}, {2, "b"}}},
		{"a\rb\rc\r", []numbered{{1, "a"}, {2, "b"}, {3, "c"}}},
		{"a\n\nb", []numbered{{1, "a"}, {2, ""}, {3, "b"}}},
		{"a\r\n\r\nb", []numbered{{1, "a"}, {2, ""}, {3, "b"}}},
		{"a\n\rb", []numbered{{1, "a"}, {2, ""}, {3, "b"}}},
		{"", nil},
	}
	for _, tt := range tests {
		var got []numbered
		forEachLine(tt.in, func(n int, s string) { got = append(got, numbered{n, s}) })
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("forEachLine(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
