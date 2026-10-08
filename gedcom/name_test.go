package gedcom

import "testing"

func TestParseName(t *testing.T) {
	tests := []struct {
		in                     string
		given, surname, suffix string
		natural, surnameFirst  string
	}{
		{"John /Smith/", "John", "Smith", "", "John Smith", "Smith, John"},
		{"John William /Smith/ Jr.", "John William", "Smith", "Jr.", "John William Smith Jr.", "Smith, John William Jr."},
		{"/Smith/", "", "Smith", "", "Smith", "Smith"},
		{"John", "John", "", "", "John", "John"},
		{"John /Smith", "John", "Smith", "", "John Smith", "Smith, John"},
		{"  John   /van  der Berg/  ", "John", "van der Berg", "", "John van der Berg", "van der Berg, John"},
		{"John //", "John", "", "", "John", "John"},
		{"/Smith/ John", "", "Smith", "John", "Smith John", "Smith, John"},
		{"", "", "", "", "", ""},
		{"A /B/ C /D/", "A", "B", "C D", "A B C D", "B, A C D"},
	}
	for _, tt := range tests {
		n := ParseName(tt.in)
		if n.Given != tt.given || n.Surname != tt.surname || n.Suffix != tt.suffix {
			t.Errorf("ParseName(%q) = %+v", tt.in, n)
		}
		if n.String() != tt.natural {
			t.Errorf("ParseName(%q).String() = %q, want %q", tt.in, n.String(), tt.natural)
		}
		if n.SurnameFirst() != tt.surnameFirst {
			t.Errorf("ParseName(%q).SurnameFirst() = %q, want %q", tt.in, n.SurnameFirst(), tt.surnameFirst)
		}
	}
	if !ParseName("").IsZero() || ParseName("x").IsZero() {
		t.Error("IsZero")
	}
}

func TestNameSubstructures(t *testing.T) {
	doc := mustParse(t, `0 HEAD
0 @I1@ INDI
1 NAME
2 GIVN Maria
2 SURN Garcia
2 NSFX III
2 NPFX Dr.
2 NICK Mia
2 SPFX de
2 TYPE birth
0 @I2@ INDI
1 NAME Ann /Lee/
2 GIVN Annie
2 SURN Leigh
0 TRLR
`)
	n := doc.Individual("I1").Name()
	if n.Given != "Maria" || n.Surname != "Garcia" || n.Suffix != "III" || n.Prefix != "Dr." || n.Nickname != "Mia" || n.SurnamePrefix != "de" || n.Type != "birth" {
		t.Errorf("name = %+v", n)
	}
	if n.Full != "Maria /Garcia/ III" {
		t.Errorf("Full = %q", n.Full)
	}
	// The NAME value wins over substructures when both are present.
	n2 := doc.Individual("I2").Name()
	if n2.Given != "Ann" || n2.Surname != "Lee" {
		t.Errorf("name = %+v", n2)
	}
	if (&Individual{}).Name().String() != "" {
		t.Error("no names")
	}
}
