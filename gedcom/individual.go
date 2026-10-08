package gedcom

import (
	"strings"
)

// Sex of an individual as recorded in the SEX tag.
type Sex string

// Sex values. SexIntersex ("X") was introduced in GEDCOM 7.
const (
	SexUnknown  Sex = ""
	SexMale     Sex = "M"
	SexFemale   Sex = "F"
	SexIntersex Sex = "X"
)

func parseSex(v string) Sex {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "M", "MALE":
		return SexMale
	case "F", "FEMALE":
		return SexFemale
	case "X":
		return SexIntersex
	}
	return SexUnknown
}

// String returns a lower-case description of the sex.
func (s Sex) String() string {
	switch s {
	case SexMale:
		return "male"
	case SexFemale:
		return "female"
	case SexIntersex:
		return "intersex"
	}
	return "unknown"
}

// FamilyLink connects a child to a family it belongs to. Pedigree records
// the PEDI value ("birth", "adopted", "foster", "sealed", ...), which is
// empty when the file does not say.
type FamilyLink struct {
	Family   *Family
	Pedigree string
}

// IsBirth reports whether the link is a biological one (or unspecified).
func (l FamilyLink) IsBirth() bool {
	p := strings.ToLower(l.Pedigree)
	return p == "" || p == "birth"
}

// Individual is an INDI record.
type Individual struct {
	ID     string // identifier without '@', e.g. "I1"
	Node   *Node
	Names  []Name
	Sex    Sex
	Events []*Event // events and attributes in file order

	childOf  []FamilyLink
	spouseIn []*Family
	doc      *Document
}

// Document returns the document the individual belongs to.
func (ind *Individual) Document() *Document { return ind.doc }

// Name returns the primary (first) name.
func (ind *Individual) Name() Name {
	if len(ind.Names) == 0 {
		return Name{}
	}
	return ind.Names[0]
}

// DisplayName returns the primary name in natural order, or a placeholder.
func (ind *Individual) DisplayName() string {
	if s := ind.Name().String(); s != "" {
		return s
	}
	return "(unnamed)"
}

// SortName returns the primary name in "Surname, Given" order.
func (ind *Individual) SortName() string {
	if s := ind.Name().SurnameFirst(); s != "" {
		return s
	}
	return "(unnamed)"
}

// FirstEvent returns the first event with one of the tags, trying the tags
// in order.
func (ind *Individual) FirstEvent(tags ...string) *Event {
	for _, t := range tags {
		for _, e := range ind.Events {
			if e.Tag == t {
				return e
			}
		}
	}
	return nil
}

// FirstDatedEvent is like FirstEvent but prefers events with a usable date:
// it returns the first event (in tag order) that has a valid date, falling
// back to the first event at all.
func (ind *Individual) FirstDatedEvent(tags ...string) *Event {
	for _, t := range tags {
		for _, e := range ind.Events {
			if e.Tag == t && e.Date.IsValid() {
				return e
			}
		}
	}
	return ind.FirstEvent(tags...)
}

// EventsWithTag returns all events with the tag.
func (ind *Individual) EventsWithTag(tag string) []*Event {
	var out []*Event
	for _, e := range ind.Events {
		if e.Tag == tag {
			out = append(out, e)
		}
	}
	return out
}

// Birth returns the BIRT event, or nil.
func (ind *Individual) Birth() *Event { return ind.FirstEvent("BIRT") }

// Death returns the DEAT event, or nil.
func (ind *Individual) Death() *Event { return ind.FirstEvent("DEAT") }

// BirthDate returns the best available date for the start of life: the
// birth date, else the christening or baptism date.
func (ind *Individual) BirthDate() Date {
	if e := ind.FirstDatedEvent(BirthTags...); e != nil {
		return e.Date
	}
	return Date{}
}

// DeathDate returns the best available date for the end of life: the death
// date, else the burial or cremation date.
func (ind *Individual) DeathDate() Date {
	if e := ind.FirstDatedEvent(DeathTags...); e != nil {
		return e.Date
	}
	return Date{}
}

// IsDeceased reports whether a death, burial or cremation is recorded.
func (ind *Individual) IsDeceased() bool { return ind.FirstEvent(DeathTags...) != nil }

// Lifespan formats birth and death years, e.g. "1820–1895", "c.1820–",
// "–1895" or "1820–?" when a death is recorded without a date.
func (ind *Individual) Lifespan() string {
	b := ind.BirthDate().ShortYear()
	d := ind.DeathDate().ShortYear()
	if d == "" && ind.IsDeceased() && b != "" {
		d = "?"
	}
	if b == "" && d == "" {
		return ""
	}
	return b + "–" + d
}

// FamiliesAsChild returns the families in which the individual is a child.
func (ind *Individual) FamiliesAsChild() []FamilyLink { return ind.childOf }

// FamiliesAsSpouse returns the families in which the individual is a
// partner, in the order given by the file.
func (ind *Individual) FamiliesAsSpouse() []*Family { return ind.spouseIn }

// parentFamily returns the family of the individual's biological parents if
// known, else the first family the individual is a child of.
func (ind *Individual) parentFamily() *Family {
	for _, l := range ind.childOf {
		if l.IsBirth() {
			return l.Family
		}
	}
	if len(ind.childOf) > 0 {
		return ind.childOf[0].Family
	}
	return nil
}

// Father returns the husband of the primary parent family.
func (ind *Individual) Father() *Individual {
	if f := ind.parentFamily(); f != nil {
		return f.Husband
	}
	return nil
}

// Mother returns the wife of the primary parent family.
func (ind *Individual) Mother() *Individual {
	if f := ind.parentFamily(); f != nil {
		return f.Wife
	}
	return nil
}

// Parents returns the partners of all families the individual is a child
// of, including adoptive and foster parents.
func (ind *Individual) Parents() []*Individual {
	var out []*Individual
	for _, l := range ind.childOf {
		out = appendUnique(out, l.Family.Partners()...)
	}
	return out
}

// Spouses returns the partners of the individual in family order.
func (ind *Individual) Spouses() []*Individual {
	var out []*Individual
	for _, f := range ind.spouseIn {
		if p := f.Partner(ind); p != nil {
			out = appendUnique(out, p)
		}
	}
	return out
}

// Children returns the children of all families of the individual.
func (ind *Individual) Children() []*Individual {
	var out []*Individual
	for _, f := range ind.spouseIn {
		out = appendUnique(out, f.Children...)
	}
	return out
}

// Siblings returns the other children of the families the individual is a
// child of (full siblings, and adoptive siblings where recorded).
func (ind *Individual) Siblings() []*Individual {
	var out []*Individual
	for _, l := range ind.childOf {
		for _, c := range l.Family.Children {
			if c != ind {
				out = appendUnique(out, c)
			}
		}
	}
	return out
}

// HalfSiblings returns children that share exactly one parent with the
// individual, i.e. children of a parent's other families.
func (ind *Individual) HalfSiblings() []*Individual {
	full := map[*Individual]bool{ind: true}
	own := map[*Family]bool{}
	for _, l := range ind.childOf {
		own[l.Family] = true
		for _, c := range l.Family.Children {
			full[c] = true
		}
	}
	var out []*Individual
	for _, p := range ind.Parents() {
		for _, f := range p.spouseIn {
			if own[f] {
				continue
			}
			for _, c := range f.Children {
				if !full[c] {
					out = appendUnique(out, c)
				}
			}
		}
	}
	return out
}

// Notes returns the text of all notes attached directly to the record.
func (ind *Individual) Notes() []string { return ind.doc.notesOf(ind.Node) }

// Citations returns all source citations anywhere in the record.
func (ind *Individual) Citations() []Citation { return ind.doc.citationsIn(ind.Node) }

func appendUnique(list []*Individual, add ...*Individual) []*Individual {
outer:
	for _, a := range add {
		if a == nil {
			continue
		}
		for _, x := range list {
			if x == a {
				continue outer
			}
		}
		list = append(list, a)
	}
	return list
}

// Family is a FAM record.
type Family struct {
	ID       string
	Node     *Node
	Husband  *Individual
	Wife     *Individual
	Children []*Individual
	Events   []*Event
	doc      *Document
}

// Partners returns the husband and wife, skipping unknown ones.
func (f *Family) Partners() []*Individual {
	var out []*Individual
	if f.Husband != nil {
		out = append(out, f.Husband)
	}
	if f.Wife != nil && f.Wife != f.Husband {
		out = append(out, f.Wife)
	}
	return out
}

// Partner returns the other partner of the family, or nil.
func (f *Family) Partner(of *Individual) *Individual {
	switch of {
	case f.Husband:
		return f.Wife
	case f.Wife:
		return f.Husband
	}
	return nil
}

// FirstEvent returns the first family event with one of the tags.
func (f *Family) FirstEvent(tags ...string) *Event {
	for _, t := range tags {
		for _, e := range f.Events {
			if e.Tag == t {
				return e
			}
		}
	}
	return nil
}

// Marriage returns the MARR event, or nil.
func (f *Family) Marriage() *Event { return f.FirstEvent("MARR") }

// Notes returns the text of the notes attached to the family record.
func (f *Family) Notes() []string { return f.doc.notesOf(f.Node) }

// Title describes the family by its partners, e.g. "John Smith & Mary Jones".
func (f *Family) Title() string {
	var names []string
	for _, p := range []*Individual{f.Husband, f.Wife} {
		if p != nil {
			names = append(names, p.DisplayName())
		} else {
			names = append(names, "?")
		}
	}
	return names[0] + " & " + names[1]
}
