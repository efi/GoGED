package gedcom

import "strings"

// Association links a person to another one with ASSO, or with _ASSO as
// several programs write it below events: a godparent, a witness, a friend.
// Relation describes the associated person (To) relative to the owner of
// the record: in "1 ASSO @I2@ / 2 RELA Godparent" in the record of I1, I2
// is the godparent of I1.
type Association struct {
	From     *Individual // owner of the record; nil for family records
	Family   *Family     // the family record containing the link, if any
	Event    *Event      // the event the link is attached to, if any
	To       *Individual // the associated person
	Relation string      // RELA (GEDCOM 5.5.1) or ROLE (GEDCOM 7) as written
	Notes    []string
}

// roles are the GEDCOM 7 ROLE values.
var roles = map[string]string{
	"CHIL": "Child", "CLERGY": "Clergy", "FATH": "Father", "FRIEND": "Friend",
	"GODP": "Godparent", "HUSB": "Husband", "MOTH": "Mother", "MULTIPLE": "Multiple",
	"NGHBR": "Neighbor", "OFFICIATOR": "Officiator", "PARENT": "Parent",
	"SPOU": "Spouse", "WIFE": "Wife", "WITN": "Witness", "OTHER": "Other",
}

// Label returns the relation in readable form, e.g. "Witness of marriage"
// for "Witness_of_Marriage" or "Godparent" for the GEDCOM 7 role "GODP".
func (a *Association) Label() string {
	r := strings.TrimSpace(a.Relation)
	if l, ok := roles[strings.ToUpper(r)]; ok {
		return l
	}
	if r == "" {
		return "Associate"
	}
	if strings.Contains(r, "_") {
		r = strings.ToLower(strings.ReplaceAll(r, "_", " "))
	}
	return strings.ToUpper(r[:1]) + r[1:]
}

// Principals returns the people the association belongs to: the owner of
// the record, or the partners of the family.
func (a *Association) Principals() []*Individual {
	if a.From != nil {
		return []*Individual{a.From}
	}
	if a.Family != nil {
		return a.Family.Partners()
	}
	return nil
}

// Associations returns the links from the individual's record, including
// its events, and from the events of the families in which the individual
// is a partner.
func (ind *Individual) Associations() []*Association {
	var out []*Association
	for _, a := range ind.doc.Associations {
		if a.From == ind || (a.Family != nil && (a.Family.Husband == ind || a.Family.Wife == ind)) {
			out = append(out, a)
		}
	}
	return out
}

// AssociatedBy returns the links from other records to the individual.
func (ind *Individual) AssociatedBy() []*Association {
	var out []*Association
	for _, a := range ind.doc.Associations {
		if a.To == ind {
			out = append(out, a)
		}
	}
	return out
}

// Aliases returns the records that, according to ALIA links in either
// direction, may describe the same person.
func (ind *Individual) Aliases() []*Individual {
	var out []*Individual
	for _, c := range ind.Node.Children {
		if c.Tag == "ALIA" && c.IsPointer() {
			out = appendUnique(out, ind.doc.indis[StripXref(c.Value)])
		}
	}
	for _, o := range ind.doc.aliasedBy[ind] {
		out = appendUnique(out, o)
	}
	return out
}

// linkAssociations collects the associations and aliases of all records.
func (d *Document) linkAssociations() {
	d.aliasedBy = map[*Individual][]*Individual{}
	collect := func(n *Node, from *Individual, fam *Family, ev *Event) {
		for _, c := range n.Children {
			if c.Tag != "ASSO" && c.Tag != "_ASSO" {
				continue
			}
			if !c.IsPointer() {
				continue
			}
			to := d.indis[StripXref(c.Value)]
			if to == nil {
				if d.records[StripXref(c.Value)] == nil {
					d.warn(c.Line, "association refers to missing record %s", c.Value)
				}
				continue
			}
			rel := c.Val("RELA")
			if rel == "" {
				rel = c.Val("ROLE")
			}
			d.Associations = append(d.Associations, &Association{
				From: from, Family: fam, Event: ev, To: to, Relation: rel, Notes: d.notesOf(c),
			})
		}
	}
	for _, ind := range d.Individuals {
		collect(ind.Node, ind, nil, nil)
		for _, e := range ind.Events {
			collect(e.Node, ind, nil, e)
		}
		for _, c := range ind.Node.Children {
			if c.Tag != "ALIA" || !c.IsPointer() {
				continue
			}
			other := d.indis[StripXref(c.Value)]
			if other == nil {
				d.warn(c.Line, "%s refers to missing alias record %s", ind.ID, c.Value)
				continue
			}
			d.aliasedBy[other] = append(d.aliasedBy[other], ind)
		}
	}
	for _, f := range d.Families {
		collect(f.Node, nil, f, nil)
		for _, e := range f.Events {
			collect(e.Node, nil, f, e)
		}
	}
}
