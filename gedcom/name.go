package gedcom

import "strings"

// Name is a personal name. The surname is delimited by slashes in GEDCOM
// ("John /Smith/ Jr."); the optional NPFX, GIVN, NICK, SPFX, SURN and NSFX
// substructures fill in parts the main value does not provide.
type Name struct {
	Full          string // the raw value, e.g. "John /Smith/ Jr."
	Given         string
	Surname       string
	Prefix        string // NPFX, e.g. "Dr."
	Suffix        string // text after the surname or NSFX, e.g. "Jr."
	Nickname      string // NICK
	SurnamePrefix string // SPFX, e.g. "van"
	Type          string // TYPE, e.g. "birth", "aka", "married"
}

func collapseSpaces(s string) string { return strings.Join(strings.Fields(s), " ") }

// ParseName splits a GEDCOM name value into given name, surname and suffix.
func ParseName(v string) Name {
	n := Name{Full: strings.TrimSpace(v)}
	first := strings.IndexByte(v, '/')
	if first < 0 {
		n.Given = collapseSpaces(v)
		return n
	}
	n.Given = collapseSpaces(v[:first])
	rest := v[first+1:]
	second := strings.IndexByte(rest, '/')
	if second < 0 {
		n.Surname = collapseSpaces(rest)
		return n
	}
	n.Surname = collapseSpaces(rest[:second])
	n.Suffix = collapseSpaces(strings.ReplaceAll(rest[second+1:], "/", " "))
	return n
}

// nameFromNode builds a Name from a NAME structure.
func nameFromNode(node *Node) Name {
	n := ParseName(node.Value)
	if n.Given == "" {
		n.Given = collapseSpaces(node.Val("GIVN"))
	}
	if n.Surname == "" {
		n.Surname = collapseSpaces(node.Val("SURN"))
	}
	if n.Suffix == "" {
		n.Suffix = collapseSpaces(node.Val("NSFX"))
	}
	n.Prefix = collapseSpaces(node.Val("NPFX"))
	n.Nickname = collapseSpaces(node.Val("NICK"))
	n.SurnamePrefix = collapseSpaces(node.Val("SPFX"))
	n.Type = strings.TrimSpace(node.Val("TYPE"))
	if n.Full == "" {
		n.Full = strings.TrimSpace(n.Given + " /" + n.Surname + "/ " + n.Suffix)
	}
	return n
}

// IsZero reports whether the name carries no text.
func (n Name) IsZero() bool { return n.Given == "" && n.Surname == "" && n.Suffix == "" }

// String returns the name in natural order without slashes, e.g.
// "John Smith Jr.".
func (n Name) String() string {
	return collapseSpaces(n.Given + " " + n.Surname + " " + n.Suffix)
}

// SurnameFirst returns the name in "Surname, Given Suffix" order, which is
// how names are listed and sorted.
func (n Name) SurnameFirst() string {
	given := collapseSpaces(n.Given + " " + n.Suffix)
	switch {
	case n.Surname == "":
		return given
	case given == "":
		return n.Surname
	}
	return n.Surname + ", " + given
}
