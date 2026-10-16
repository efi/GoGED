// Package diff compares two versions of a GEDCOM file. Records are paired
// by their identifiers, so it tells what an editor changed in a file; it
// is not a comparison of two different files about the same people. The
// file's header, people, families and other records (sources, notes,
// submitters …) are reported as added, removed or changed, with what
// changed in them. Change dates (CHAN) are left out, as editors update
// them with every edit.
package diff

import (
	"fmt"
	"slices"
	"strings"

	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/search"
)

// Kind tells whether something was added, removed or changed.
type Kind int

// Kinds of change.
const (
	Added Kind = iota + 1
	Removed
	Changed
)

// Symbol returns +, - or ~.
func (k Kind) Symbol() string { return [...]string{"", "+", "-", "~"}[k] }

// Detail is one difference: within a record, or in the header.
type Detail struct {
	Kind Kind
	What string // e.g. "name", "birth", "program"
	Old  string // the old value, if any
	New  string // the new value, if any
}

// String describes the difference: "birth: 1817 → 2 Feb 1817",
// "residence added: 1881, Leeds", "note removed: …", "birth: details
// changed".
func (d Detail) String() string {
	switch {
	case d.Kind == Added && d.New == "":
		return d.What + " added"
	case d.Kind == Added:
		return d.What + " added: " + d.New
	case d.Kind == Removed && d.Old == "":
		return d.What + " removed"
	case d.Kind == Removed:
		return d.What + " removed: " + d.Old
	case d.Old == d.New:
		return d.What + ": details changed"
	}
	return d.What + ": " + d.Old + " → " + d.New
}

// Record is a record that was added, removed or changed.
type Record struct {
	Kind  Kind
	ID    string // without @
	Tag   string // INDI, FAM, SOUR …
	Title string // the person, the partners, a source's title …
	// Person or Family is the record in the new version, or in the old one
	// if it was removed.
	Person  *gedcom.Individual
	Family  *gedcom.Family
	Details []Detail // what changed, for Changed
}

// Noun names the kind of record: "person", "family", "source" …
func (r Record) Noun() string {
	if n, ok := recordNouns[r.Tag]; ok {
		return n
	}
	return strings.ToLower(strings.TrimPrefix(r.Tag, "_"))
}

var recordNouns = map[string]string{
	"INDI": "person", "FAM": "family", "SOUR": "source", "REPO": "repository", "NOTE": "note", "SNOTE": "note",
	"OBJE": "media", "SUBM": "submitter", "SUBN": "submission", "_LOC": "place",
}

// Result is what changed from one version to the other.
type Result struct {
	Header   []Detail // the file's header: program, date, GEDCOM version …
	People   []Record // sorted by name
	Families []Record // sorted by partners
	Others   []Record // sources, notes, submitters …, by kind and title
}

// Empty reports whether the versions are the same.
func (r Result) Empty() bool {
	return len(r.Header)+len(r.People)+len(r.Families)+len(r.Others) == 0
}

// Summary counts the changes: "people: 2 changed, 1 added · header: 1
// change", or "no differences".
func (r Result) Summary() string {
	var parts []string
	count := func(name string, rs []Record) {
		var n [4]int
		for _, rec := range rs {
			n[rec.Kind]++
		}
		var p []string
		for _, k := range []Kind{Changed, Added, Removed} {
			if n[k] > 0 {
				p = append(p, fmt.Sprintf("%d %s", n[k], [...]string{"", "added", "removed", "changed"}[k]))
			}
		}
		if len(p) > 0 {
			parts = append(parts, name+": "+strings.Join(p, ", "))
		}
	}
	count("people", r.People)
	count("families", r.Families)
	count("other records", r.Others)
	if n := len(r.Header); n > 0 {
		parts = append(parts, fmt.Sprintf("header: %d %s", n, map[bool]string{true: "change", false: "changes"}[n == 1]))
	}
	if len(parts) == 0 {
		return "no differences"
	}
	return strings.Join(parts, " · ")
}

// side is one version of the file, with its events by their node.
type side struct {
	doc    *gedcom.Document
	events map[*gedcom.Node]*gedcom.Event
}

func newSide(doc *gedcom.Document) side {
	s := side{doc, map[*gedcom.Node]*gedcom.Event{}}
	for _, e := range doc.Events() {
		s.events[e.Node] = e
	}
	return s
}

// Compare tells what changed from old to cur.
func Compare(old, cur *gedcom.Document) Result {
	so, sn := newSide(old), newSide(cur)
	var r Result
	if old.Header != nil || cur.Header != nil {
		r.Header = details(old.Header, cur.Header, so, sn, describeHeader)
	}

	byID := func(doc *gedcom.Document) (map[string]*gedcom.Node, []string) {
		m := map[string]*gedcom.Node{}
		var ids []string
		for _, rec := range doc.Records {
			if rec.Xref == "" || rec.Tag == "HEAD" || rec.Tag == "TRLR" {
				continue
			}
			id := gedcom.StripXref(rec.Xref)
			if _, dup := m[id]; !dup && doc.Record(id) == rec {
				m[id] = rec
				ids = append(ids, id)
			}
		}
		return m, ids
	}
	oldRecs, oldIDs := byID(old)
	newRecs, ids := byID(cur)
	for _, id := range oldIDs {
		if newRecs[id] == nil {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		o, n := oldRecs[id], newRecs[id]
		if o != nil && n != nil && o.Tag != n.Tag {
			// The identifier now names another kind of record.
			r.add(record(Removed, id, o, so))
			r.add(record(Added, id, n, sn))
			continue
		}
		switch {
		case o == nil:
			r.add(record(Added, id, n, sn))
		case n == nil:
			r.add(record(Removed, id, o, so))
		default:
			if ds := details(o, n, so, sn, describe); len(ds) > 0 {
				rec := record(Changed, id, n, sn)
				rec.Details = ds
				r.add(rec)
			}
		}
	}
	byTitle := func(a, b Record) int {
		if c := strings.Compare(a.Tag, b.Tag); c != 0 && a.Tag != "INDI" {
			return c
		}
		if c := strings.Compare(search.Fold(a.sortKey()), search.Fold(b.sortKey())); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	}
	slices.SortStableFunc(r.People, byTitle)
	slices.SortStableFunc(r.Families, byTitle)
	slices.SortStableFunc(r.Others, func(a, b Record) int {
		// Submitters first: they belong to the file's metadata.
		if (a.Tag == "SUBM") != (b.Tag == "SUBM") {
			if a.Tag == "SUBM" {
				return -1
			}
			return 1
		}
		return byTitle(a, b)
	})
	return r
}

func (r *Result) add(rec Record) {
	switch rec.Tag {
	case "INDI":
		r.People = append(r.People, rec)
	case "FAM":
		r.Families = append(r.Families, rec)
	default:
		r.Others = append(r.Others, rec)
	}
}

func (r Record) sortKey() string {
	if r.Person != nil {
		return r.Person.SortName()
	}
	return r.Title
}

// record describes a record of one version.
func record(kind Kind, id string, n *gedcom.Node, s side) Record {
	rec := Record{Kind: kind, ID: id, Tag: n.Tag}
	switch n.Tag {
	case "INDI":
		rec.Person = s.doc.Individual(id)
		rec.Title = person(rec.Person)
	case "FAM":
		rec.Family = s.doc.Family(id)
		rec.Title = partners(rec.Family)
	case "SOUR":
		rec.Title = first(n.Val("TITL"), n.Val("ABBR"), n.Value)
	case "OBJE":
		rec.Title = first(n.Val("TITL"), pathValue(n, "FILE", "TITL"), n.Val("FILE"))
	case "REPO", "SUBM", "_LOC":
		rec.Title = n.Val("NAME")
	default:
		rec.Title = n.Value
	}
	rec.Title = shorten(rec.Title)
	return rec
}

// person names a person with their lifespan.
func person(ind *gedcom.Individual) string {
	if ind == nil {
		return "?"
	}
	s := ind.DisplayName()
	if span := ind.Lifespan(); span != "" {
		s += " (" + span + ")"
	}
	return s
}

// partners names the partners of a family.
func partners(f *gedcom.Family) string {
	if f == nil {
		return "?"
	}
	var names []string
	for _, p := range f.Partners() {
		names = append(names, p.DisplayName())
	}
	if len(names) == 0 {
		return "no partners"
	}
	return strings.Join(names, " & ")
}

func pathValue(n *gedcom.Node, tags ...string) string {
	if p := n.Path(tags...); p != nil {
		return p.Value
	}
	return ""
}

func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// shorten cuts long values for a line of their own.
func shorten(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 60 {
		return string(r[:59]) + "…"
	}
	return s
}

// entry is a structure directly in a record (or the header), with the
// text of all it holds.
type entry struct {
	node *gedcom.Node
	text string
}

func entries(rec *gedcom.Node) []entry {
	if rec == nil {
		return nil
	}
	var out []entry
	for _, c := range rec.Children {
		if c.Tag != "CHAN" {
			out = append(out, entry{c, text(c)})
		}
	}
	return out
}

// text is a structure written out, without change dates.
func text(n *gedcom.Node) string {
	var b strings.Builder
	n.Walk(func(c *gedcom.Node) bool {
		if c.Tag == "CHAN" {
			return false
		}
		fmt.Fprintf(&b, "%d %s %s\n", c.Level-n.Level, c.Tag, c.Value)
		return true
	})
	return b.String()
}

// details pairs the structures of two versions of a record: the same
// structures first, then those with the same tag in their order. The rest
// was added or removed.
func details(o, n *gedcom.Node, so, sn side, desc func(*gedcom.Node, side) (string, string)) []Detail {
	oe, ne := entries(o), entries(n)
	pair := make([]int, len(ne)) // the old structure of each new one, or -1
	used := make([]bool, len(oe))
	for j := range pair {
		pair[j] = -1
	}
	for _, same := range []bool{true, false} {
		for j, e := range ne {
			if pair[j] >= 0 {
				continue
			}
			for i, f := range oe {
				if !used[i] && f.node.Tag == e.node.Tag && (!same || f.text == e.text) {
					pair[j], used[i] = i, true
					break
				}
			}
		}
	}
	var out []Detail
	for j, e := range ne {
		what, val := desc(e.node, sn)
		switch i := pair[j]; {
		case i < 0:
			out = append(out, Detail{Kind: Added, What: what, New: val})
		case oe[i].text != e.text:
			_, old := desc(oe[i].node, so)
			out = append(out, Detail{Kind: Changed, What: what, Old: old, New: val})
		}
	}
	for i, f := range oe {
		if !used[i] {
			what, val := desc(f.node, so)
			out = append(out, Detail{Kind: Removed, What: what, Old: val})
		}
	}
	return out
}

// describe names a structure of a record and gives its value in short.
func describe(n *gedcom.Node, s side) (what, value string) {
	ref := gedcom.StripXref(n.Value)
	switch n.Tag {
	case "NAME":
		return "name", shorten(strings.ReplaceAll(n.Value, "/", ""))
	case "SEX":
		return "sex", gedcom.ParseSex(n.Value).String()
	case "FAMC":
		v := partners(s.doc.Family(ref))
		if p := strings.TrimSpace(n.Val("PEDI")); p != "" {
			v += " (" + strings.ToLower(p) + ")"
		}
		return "parents", v
	case "FAMS":
		return "family", partners(s.doc.Family(ref))
	case "HUSB", "WIFE", "CHIL":
		return map[string]string{"HUSB": "husband", "WIFE": "wife", "CHIL": "child"}[n.Tag], person(s.doc.Individual(ref))
	case "SOUR":
		if n.IsPointer() {
			if src := s.doc.Source(ref); src != nil {
				return "source", shorten(first(src.Title, n.Value))
			}
		}
		return "source", shorten(n.Value)
	case "NOTE", "SNOTE":
		if n.IsPointer() {
			if rec := s.doc.Record(ref); rec != nil {
				return "note", shorten(rec.Value)
			}
		}
		return "note", shorten(n.Value)
	case "OBJE":
		return "media", shorten(first(n.Val("TITL"), n.Val("FILE"), n.Value))
	case "ALIA", "ASSO":
		// A link to a person names them, and an association its relation.
		if ind := s.doc.Individual(ref); n.IsPointer() && ind != nil {
			v := person(ind)
			if rel := first(n.Val("RELA"), n.Val("ROLE")); rel != "" && n.Tag == "ASSO" {
				v += " · " + rel
			}
			return labels[n.Tag], shorten(v)
		}
	}
	if e := s.events[n]; e != nil {
		var parts []string
		for _, p := range []string{e.Detail(), e.Date.String(), e.Place.String()} {
			if p != "" {
				parts = append(parts, p)
			}
		}
		return strings.ToLower(e.Label()), shorten(strings.Join(parts, ", "))
	}
	if label, ok := labels[n.Tag]; ok {
		return label, shorten(values(n))
	}
	return strings.ToLower(strings.TrimPrefix(n.Tag, "_")), shorten(values(n))
}

// labels name structures of records other than events.
var labels = map[string]string{
	"TITL": "title", "AUTH": "author", "PUBL": "publication", "ABBR": "abbreviation", "TEXT": "text",
	"FILE": "file", "FORM": "format", "REPO": "repository", "ADDR": "address", "PHON": "phone",
	"EMAIL": "e-mail", "WWW": "web page", "LANG": "language", "ALIA": "alias", "ASSO": "association",
	"RESN": "restriction", "REFN": "reference number", "RIN": "record number", "UID": "unique ID", "_UID": "unique ID",
	"MAP": "coordinates", "LATI": "latitude", "LONG": "longitude", "_GOV": "GOV identifier",
}

// describeHeader names a structure of the header.
func describeHeader(n *gedcom.Node, _ side) (what, value string) {
	label, ok := headerLabels[n.Tag]
	if !ok {
		label = strings.ToLower(strings.TrimPrefix(n.Tag, "_"))
	}
	return label, shorten(values(n))
}

var headerLabels = map[string]string{
	"SOUR": "program", "DATE": "date of the file", "GEDC": "GEDCOM version", "CHAR": "character set",
	"FILE": "file name", "LANG": "language", "SUBM": "submitter", "SUBN": "submission", "PLAC": "place format",
	"NOTE": "note", "DEST": "destination", "COPR": "copyright",
}

// values joins the values in a structure: "GENJ · 3.0 · GenJ".
func values(n *gedcom.Node) string {
	var vs []string
	n.Walk(func(c *gedcom.Node) bool {
		if v := strings.TrimSpace(c.Value); v != "" {
			vs = append(vs, v)
		}
		return true
	})
	return strings.Join(vs, " · ")
}
