package gedcom

import "strings"

// Name is a personal name. The surname is delimited by slashes in GEDCOM
// ("John /Smith/ Jr."); the optional NPFX, GIVN, NICK, SPFX, SURN and NSFX
// substructures fill in parts the main value does not provide.
type Name struct {
	Full          string // the raw value, e.g. "John /Smith/ Jr."
	Given         string // given names without the prefix
	Surname       string // the surname as written between the slashes
	Prefix        string // NPFX, e.g. "Dr." or "Freiherr"
	Suffix        string // text after the surname or NSFX, e.g. "Jr."
	Nickname      string // NICK
	CallName      string // _RUFNAME: the given name the person is called by (GEDCOM-L)
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
	n.CallName = collapseSpaces(node.Val("_RUFNAME"))
	n.SurnamePrefix = collapseSpaces(node.Val("SPFX"))
	// The name value usually repeats the prefix ("Dr. John /Smith/"); it
	// is a title, not a given name.
	if rest, ok := cutWords(n.Given, n.Prefix); ok {
		n.Given = rest
	}
	n.Type = strings.TrimSpace(node.Val("TYPE"))
	if n.Full == "" {
		n.Full = strings.TrimSpace(n.Given + " /" + n.Surname + "/ " + n.Suffix)
	}
	return n
}

// cutWords removes prefix, a sequence of whole words, from the start of s,
// ignoring case.
func cutWords(s, prefix string) (string, bool) {
	if prefix == "" || len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return s, false
	}
	rest := s[len(prefix):]
	if rest != "" && rest[0] != ' ' {
		return s, false
	}
	return strings.TrimSpace(rest), true
}

// FullSurname returns the surname including the surname prefix, which some
// programs record only in SPFX ("van der Berg").
func (n Name) FullSurname() string {
	if n.SurnamePrefix == "" || n.Surname == "" {
		return n.Surname
	}
	if len(n.Surname) >= len(n.SurnamePrefix) && strings.EqualFold(n.Surname[:len(n.SurnamePrefix)], n.SurnamePrefix) {
		return n.Surname // "van der Berg", or "Vandenberg" for SPFX "van"
	}
	return n.SurnamePrefix + " " + n.Surname
}

// SortSurname returns the surname under which the name is sorted. A prefix
// recorded in SPFX is not part of it ("Berg" for "van der Berg"), following
// the convention that SPFX marks prefixes that are ignored when sorting;
// prefixes that are only part of the surname ("von Stradonitz" without SPFX)
// are kept.
func (n Name) SortSurname() string {
	if rest, ok := cutWords(n.Surname, n.SurnamePrefix); ok && rest != "" {
		return rest
	}
	return n.Surname
}

// IsZero reports whether the name carries no text.
func (n Name) IsZero() bool { return n.Given == "" && n.Surname == "" && n.Suffix == "" }

// String returns the name in natural order without slashes, e.g.
// "Dr. John Smith Jr.".
func (n Name) String() string {
	return collapseSpaces(n.Prefix + " " + n.Given + " " + n.FullSurname() + " " + n.Suffix)
}

// SurnameFirst returns the name in "Surname, Given Suffix" order, which is
// how names are listed and sorted. The title is left out, and a surname
// prefix recorded in SPFX moves to the end ("Berg, John van der").
func (n Name) SurnameFirst() string {
	surname := n.SortSurname()
	given := n.Given
	if surname != n.FullSurname() {
		given += " " + n.SurnamePrefix
	}
	given = collapseSpaces(given + " " + n.Suffix)
	switch {
	case surname == "":
		return given
	case given == "":
		return surname
	}
	return surname + ", " + given
}
