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

// Pedigree is the kind of link between a child and a parent, recorded with
// the PEDI tag and related conventions.
type Pedigree string

// Pedigree values. PedigreeUnknown is used when the file does not say; it is
// treated like a birth link.
const (
	PedigreeUnknown Pedigree = ""
	PedigreeBirth   Pedigree = "birth"
	PedigreeAdopted Pedigree = "adopted"
	PedigreeFoster  Pedigree = "foster"
	PedigreeSealed  Pedigree = "sealed"
	PedigreeStep    Pedigree = "step"
	PedigreeOther   Pedigree = "other"
)

// ParsePedigree normalizes a PEDI value. Besides the GEDCOM 5.5.1 and 7
// values it accepts the _FREL/_MREL values of other programs ("Natural",
// "Step", ...).
func ParsePedigree(v string) Pedigree {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "unknown":
		return PedigreeUnknown
	case "birth", "natural", "biological":
		return PedigreeBirth
	case "adopted", "adoption", "adoptive":
		return PedigreeAdopted
	case "foster":
		return PedigreeFoster
	case "sealed", "sealing":
		return PedigreeSealed
	case "step", "stepchild":
		return PedigreeStep
	}
	return PedigreeOther
}

// IsBirth reports whether the link is biological or unspecified.
func (p Pedigree) IsBirth() bool { return p == PedigreeUnknown || p == PedigreeBirth }

// Adjective describes relatives through such a link: "adoptive", "foster",
// "step", "sealed" or "non-biological"; it is empty for birth links.
func (p Pedigree) Adjective() string {
	switch p {
	case PedigreeUnknown, PedigreeBirth:
		return ""
	case PedigreeAdopted:
		return "adoptive"
	case PedigreeOther:
		return "non-biological"
	}
	return string(p)
}

// FamilyLink connects a child to a family it belongs to.
type FamilyLink struct {
	Family *Family
	// Pedigree is the PEDI value of the link, PedigreeUnknown when the
	// file does not say.
	Pedigree Pedigree
	// Husband and Wife are the kinds of link to each partner. They differ
	// from Pedigree when only one partner adopted the child (ADOP.FAMC.ADOP)
	// or when the family records them separately (CHIL._FREL/_MREL).
	Husband, Wife Pedigree
}

// IsBirth reports whether the child is a biological (or unspecified) child
// of both partners.
func (l FamilyLink) IsBirth() bool { return l.Husband.IsBirth() && l.Wife.IsBirth() }

// Of returns the kind of link to a partner of the family.
func (l FamilyLink) Of(parent *Individual) Pedigree {
	switch {
	case parent == nil:
		return PedigreeUnknown
	case parent == l.Family.Husband:
		return l.Husband
	case parent == l.Family.Wife:
		return l.Wife
	}
	return PedigreeUnknown
}

// Kind summarizes the link for display: the pedigree of the link, or of the
// partner who is not a birth parent, or PedigreeUnknown for birth links.
func (l FamilyLink) Kind() Pedigree {
	switch {
	case !l.Husband.IsBirth():
		return l.Husband
	case !l.Wife.IsBirth():
		return l.Wife
	}
	return PedigreeUnknown
}

// ParentLink is a parent of an individual with the kind of link.
type ParentLink struct {
	Parent   *Individual
	Family   *Family
	Pedigree Pedigree
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

// ParentLinks returns every parent once, with the closest kind of link:
// a parent who is both a birth parent and, through another family, an
// adoptive parent is listed as a birth parent.
func (ind *Individual) ParentLinks() []ParentLink {
	var out []ParentLink
	for _, l := range ind.childOf {
		for _, p := range l.Family.Partners() {
			pl := ParentLink{Parent: p, Family: l.Family, Pedigree: l.Of(p)}
			found := false
			for i := range out {
				if out[i].Parent == p {
					found = true
					if pedigreeRank(pl.Pedigree) < pedigreeRank(out[i].Pedigree) {
						out[i] = pl
					}
				}
			}
			if !found {
				out = append(out, pl)
			}
		}
	}
	return out
}

// pedigreeRank orders kinds of links from the closest to the most distant.
func pedigreeRank(p Pedigree) int {
	switch p {
	case PedigreeBirth:
		return 0
	case PedigreeUnknown:
		return 1
	case PedigreeAdopted:
		return 2
	case PedigreeSealed:
		return 3
	case PedigreeOther:
		return 4
	case PedigreeStep:
		return 5
	}
	return 6 // foster
}

// birthParent returns the first birth parent found through f(link), else the
// first parent found at all.
func (ind *Individual) birthParent(f func(*Family) *Individual) *Individual {
	var fallback *Individual
	for _, l := range ind.childOf {
		p := f(l.Family)
		if p == nil {
			continue
		}
		if l.Of(p).IsBirth() {
			return p
		}
		if fallback == nil {
			fallback = p
		}
	}
	return fallback
}

// Father returns the husband of a family in which the individual is a birth
// child, else of the first family the individual is a child of.
func (ind *Individual) Father() *Individual {
	return ind.birthParent(func(f *Family) *Individual { return f.Husband })
}

// Mother returns the wife of a family in which the individual is a birth
// child, else of the first family the individual is a child of.
func (ind *Individual) Mother() *Individual {
	return ind.birthParent(func(f *Family) *Individual { return f.Wife })
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

// BirthParents returns the parents with a biological or unspecified link.
func (ind *Individual) BirthParents() []*Individual {
	var out []*Individual
	for _, pl := range ind.ParentLinks() {
		if pl.Pedigree.IsBirth() {
			out = append(out, pl.Parent)
		}
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

// HalfSiblings returns children that share exactly one birth parent with
// the individual, i.e. birth children of a birth parent's other families.
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
	for _, p := range ind.BirthParents() {
		for _, f := range p.spouseIn {
			if own[f] {
				continue
			}
			for _, c := range f.Children {
				if l, _ := c.ChildLink(f); !full[c] && l.Of(p).IsBirth() {
					out = appendUnique(out, c)
				}
			}
		}
	}
	return out
}

// ChildLink returns the link of the individual to f, which the individual is
// a child of; ok is false if it is not.
func (ind *Individual) ChildLink(f *Family) (link FamilyLink, ok bool) {
	for _, l := range ind.childOf {
		if l.Family == f {
			return l, true
		}
	}
	return FamilyLink{Family: f}, false
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
