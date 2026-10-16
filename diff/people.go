package diff

import (
	"fmt"
	"slices"
	"sort"

	"github.com/efi/goged/gedcom"
)

// Row is a line of a comparison of two people: what it is about and the
// value of each, "" where one of them has none.
type Row struct {
	What        string // e.g. "name", "birth", "parents", "partner"
	Left, Right string
}

// Same reports whether both have the same value.
func (r Row) Same() bool { return r.Left == r.Right }

// People compares two people side by side, such as two records that may
// describe the same person. The structures of their records are described
// as the changes of a file are, and paired by what they are, those that
// read the same first. The rows come in the order names, sex, birth and
// baptism, other events, death and burial, parents, partners, and the
// rest (notes, sources, …); change dates are left out.
func People(a, b *gedcom.Individual) []Row {
	if a == nil || b == nil {
		return nil
	}
	sa := newSide(a.Document())
	sb := sa
	if b.Document() != a.Document() {
		sb = newSide(b.Document())
	}
	left, right := describePerson(a, sa), describePerson(b, sb)

	pair := make([]int, len(right)) // the left structure of each right one, or -1
	used := make([]bool, len(left))
	for j := range pair {
		pair[j] = -1
	}
	for _, same := range []bool{true, false} {
		for j, r := range right {
			if pair[j] >= 0 {
				continue
			}
			for i, l := range left {
				if !used[i] && l.what == r.what && (!same || l.value == r.value) {
					pair[j], used[i] = i, true
					break
				}
			}
		}
	}

	// Rows keep the order of the left record within their group, those
	// only on the right after them.
	type ranked struct {
		row         Row
		group, i, j int
	}
	var rows []ranked
	for j, r := range right {
		if i := pair[j]; i >= 0 {
			rows = append(rows, ranked{Row{r.what, left[i].value, r.value}, r.group, i, j})
		} else {
			rows = append(rows, ranked{Row{r.what, "", r.value}, r.group, len(left), j})
		}
	}
	for i, l := range left {
		if !used[i] {
			rows = append(rows, ranked{Row{l.what, l.value, ""}, l.group, i, -1})
		}
	}
	sort.SliceStable(rows, func(x, y int) bool {
		a, b := rows[x], rows[y]
		if a.group != b.group {
			return a.group < b.group
		}
		if a.i != b.i {
			return a.i < b.i
		}
		return a.j < b.j
	})
	out := make([]Row, len(rows))
	for k, r := range rows {
		out[k] = r.row
	}
	return out
}

// item is a structure of a person's record, described.
type item struct {
	what, value string
	group       int // see rowGroup
}

// describePerson describes the structures of a person's record. A family
// as a partner is named by the other partner, the marriage and the
// number of children.
func describePerson(ind *gedcom.Individual, s side) []item {
	var out []item
	for _, e := range entries(ind.Node) {
		n := e.node
		what, value := describe(n, s)
		if n.Tag == "FAMS" {
			what, value = "partner", partnerIn(s.doc.Family(gedcom.StripXref(n.Value)), ind)
		}
		out = append(out, item{what, value, rowGroup(n, s)})
	}
	return out
}

// partnerIn names ind's partner in f, with the marriage and the children.
func partnerIn(f *gedcom.Family, ind *gedcom.Individual) string {
	if f == nil {
		return "?"
	}
	v := "unknown partner"
	if p := f.Partner(ind); p != nil {
		v = person(p)
	}
	if m := f.Marriage(); m != nil && m.Date.IsValid() {
		v += ", married " + m.Date.String()
	}
	switch n := len(f.Children); n {
	case 0:
	case 1:
		v += ", 1 child"
	default:
		v += fmt.Sprintf(", %d children", n)
	}
	return v
}

// rowGroup orders the rows of a comparison: names, sex, birth and
// baptism, other events, death and burial, parents, partners, the rest.
func rowGroup(n *gedcom.Node, s side) int {
	switch {
	case n.Tag == "NAME":
		return 0
	case n.Tag == "SEX":
		return 1
	case s.events[n] == nil:
	case slices.Contains(gedcom.BirthTags, n.Tag):
		return 2
	case slices.Contains(gedcom.DeathTags, n.Tag):
		return 4
	default:
		return 3
	}
	switch n.Tag {
	case "FAMC":
		return 5
	case "FAMS":
		return 6
	}
	return 7
}
