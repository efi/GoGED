package gedcom

import (
	"fmt"
	"sort"
	"strings"
)

// Source is a SOUR record.
type Source struct {
	ID          string
	Node        *Node
	Title       string
	Author      string
	Publication string
}

// Citation is a reference to a source from within a record.
type Citation struct {
	Source *Source // nil for inline sources and unresolved pointers
	Text   string  // title of the source or the inline source text
	Page   string  // PAGE: where in the source the information was found
	Path   string  // tag path of the citation, e.g. "INDI.BIRT.SOUR"
}

// Document is a parsed GEDCOM file.
type Document struct {
	Header      *Node
	Records     []*Node
	Individuals []*Individual // in file order
	Families    []*Family     // in file order
	Sources     []*Source
	Warnings    []Warning
	Encoding    Encoding
	Version     string // HEAD.GEDC.VERS

	records map[string]*Node
	indis   map[string]*Individual
	fams    map[string]*Family
	sources map[string]*Source
}

// normalizeID accepts "I1" or "@I1@" and returns "I1".
func normalizeID(id string) string { return StripXref(id) }

// Individual returns the individual with the given identifier ("I1" or
// "@I1@"), or nil.
func (d *Document) Individual(id string) *Individual { return d.indis[normalizeID(id)] }

// Family returns the family with the given identifier, or nil.
func (d *Document) Family(id string) *Family { return d.fams[normalizeID(id)] }

// Source returns the source with the given identifier, or nil.
func (d *Document) Source(id string) *Source { return d.sources[normalizeID(id)] }

// Record returns any level-0 record by identifier, or nil.
func (d *Document) Record(id string) *Node { return d.records[normalizeID(id)] }

// SourceSoftware returns the name of the program that wrote the file.
func (d *Document) SourceSoftware() string {
	if d.Header == nil {
		return ""
	}
	s := d.Header.First("SOUR")
	if s == nil {
		return ""
	}
	if name := s.Val("NAME"); name != "" {
		return name
	}
	return strings.TrimSpace(s.Value)
}

// Events returns the events of all individuals and families.
func (d *Document) Events() []*Event {
	var out []*Event
	for _, ind := range d.Individuals {
		out = append(out, ind.Events...)
	}
	for _, f := range d.Families {
		out = append(out, f.Events...)
	}
	return out
}

func (d *Document) warn(line int, format string, args ...any) {
	d.Warnings = append(d.Warnings, Warning{line, fmt.Sprintf(format, args...)})
}

// resolveNote returns the text of a NOTE or SNOTE node, following pointers
// to shared note records.
func (d *Document) resolveNote(n *Node) string {
	if n.IsPointer() {
		if r := d.records[StripXref(n.Value)]; r != nil {
			return strings.TrimSpace(r.Value)
		}
		return ""
	}
	return strings.TrimSpace(n.Value)
}

// notesOf returns the texts of the NOTE and SNOTE children of n.
func (d *Document) notesOf(n *Node) []string {
	var out []string
	for _, c := range n.Children {
		if c.Tag != "NOTE" && c.Tag != "SNOTE" {
			continue
		}
		if t := d.resolveNote(c); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// citationsIn collects all SOUR citations below n.
func (d *Document) citationsIn(n *Node) []Citation {
	var out []Citation
	n.Walk(func(c *Node) bool {
		if c == n || c.Tag != "SOUR" {
			return true
		}
		cit := Citation{Page: strings.TrimSpace(c.Val("PAGE")), Path: c.TagPath()}
		if c.IsPointer() {
			cit.Source = d.sources[StripXref(c.Value)]
			if cit.Source != nil {
				cit.Text = cit.Source.Title
			} else {
				cit.Text = c.Value
			}
		} else {
			cit.Text = strings.TrimSpace(c.Value)
			if cit.Text == "" {
				cit.Text = c.Val("TITL")
			}
		}
		out = append(out, cit)
		return false
	})
	return out
}

// newDocument indexes the records and builds the typed model.
func newDocument(records []*Node, warnings []Warning) *Document {
	d := &Document{
		Records:  records,
		Warnings: warnings,
		records:  map[string]*Node{},
		indis:    map[string]*Individual{},
		fams:     map[string]*Family{},
		sources:  map[string]*Source{},
	}
	for _, r := range records {
		if r.Tag == "HEAD" && d.Header == nil {
			d.Header = r
			d.Version = d.Header.Path("GEDC", "VERS").valueOrEmpty()
		}
		if r.Xref == "" {
			switch r.Tag {
			case "INDI", "FAM", "SOUR", "REPO", "OBJE", "SNOTE", "SUBM":
				d.warn(r.Line, "%s record without cross-reference identifier ignored", r.Tag)
			}
			continue
		}
		id := StripXref(r.Xref)
		if _, dup := d.records[id]; dup {
			d.warn(r.Line, "duplicate identifier %s; record ignored", r.Xref)
			continue
		}
		d.records[id] = r
		switch r.Tag {
		case "INDI":
			ind := &Individual{ID: id, Node: r, doc: d}
			d.Individuals = append(d.Individuals, ind)
			d.indis[id] = ind
		case "FAM":
			f := &Family{ID: id, Node: r, doc: d}
			d.Families = append(d.Families, f)
			d.fams[id] = f
		case "SOUR":
			s := &Source{
				ID:          id,
				Node:        r,
				Title:       strings.TrimSpace(strings.ReplaceAll(r.Val("TITL"), "\n", " ")),
				Author:      strings.TrimSpace(r.Val("AUTH")),
				Publication: strings.TrimSpace(r.Val("PUBL")),
			}
			if s.Title == "" {
				s.Title = r.Val("ABBR")
			}
			if s.Title == "" {
				s.Title = strings.TrimSpace(r.Value)
			}
			if s.Title == "" {
				s.Title = "Source " + id
			}
			d.Sources = append(d.Sources, s)
			d.sources[id] = s
		}
	}

	for _, ind := range d.Individuals {
		d.buildIndividual(ind)
	}
	for _, f := range d.Families {
		d.buildFamily(f)
	}
	d.linkFamilies()
	return d
}

func (n *Node) valueOrEmpty() string {
	if n == nil {
		return ""
	}
	return strings.TrimSpace(n.Value)
}

func (d *Document) buildIndividual(ind *Individual) {
	for _, c := range ind.Node.Children {
		switch {
		case c.Tag == "NAME":
			ind.Names = append(ind.Names, nameFromNode(c))
		case c.Tag == "SEX":
			ind.Sex = parseSex(c.Value)
		case isEventNode(c):
			e := newEvent(d, c)
			e.Individual = ind
			ind.Events = append(ind.Events, e)
		}
	}
}

func (d *Document) buildFamily(f *Family) {
	for _, c := range f.Node.Children {
		switch c.Tag {
		case "HUSB", "WIFE", "CHIL":
			if !c.IsPointer() {
				continue
			}
			id := StripXref(c.Value)
			if id == "VOID" {
				continue
			}
			ind := d.indis[id]
			if ind == nil {
				d.warn(c.Line, "family %s refers to missing individual %s", f.ID, c.Value)
				continue
			}
			switch c.Tag {
			case "HUSB":
				if f.Husband == nil {
					f.Husband = ind
				}
			case "WIFE":
				if f.Wife == nil {
					f.Wife = ind
				}
			case "CHIL":
				if !containsInd(f.Children, ind) {
					f.Children = append(f.Children, ind)
				}
			}
		default:
			if isEventNode(c) {
				e := newEvent(d, c)
				e.Family = f
				f.Events = append(f.Events, e)
			}
		}
	}
}

func containsInd(list []*Individual, ind *Individual) bool {
	for _, x := range list {
		if x == ind {
			return true
		}
	}
	return false
}

// linkFamilies connects individuals and families in both directions. Links
// that are only recorded on one side are repaired and reported.
func (d *Document) linkFamilies() {
	for _, ind := range d.Individuals {
		for _, c := range ind.Node.Children {
			if (c.Tag != "FAMC" && c.Tag != "FAMS") || !c.IsPointer() {
				continue
			}
			id := StripXref(c.Value)
			if id == "VOID" {
				continue
			}
			f := d.fams[id]
			if f == nil {
				d.warn(c.Line, "individual %s refers to missing family %s", ind.ID, c.Value)
				continue
			}
			if c.Tag == "FAMC" {
				if !hasChildLink(ind, f) {
					ind.childOf = append(ind.childOf, FamilyLink{Family: f, Pedigree: ParsePedigree(c.Val("PEDI"))})
				}
				if !containsInd(f.Children, ind) {
					d.warn(c.Line, "%s lists family %s as parents, but the family does not list %s as a child", ind.ID, f.ID, ind.ID)
					f.Children = append(f.Children, ind)
				}
				continue
			}
			if !containsFam(ind.spouseIn, f) {
				ind.spouseIn = append(ind.spouseIn, f)
			}
			if f.Husband != ind && f.Wife != ind {
				d.warn(c.Line, "%s lists family %s as spouse, but the family does not list %s as a partner", ind.ID, f.ID, ind.ID)
				switch {
				case ind.Sex == SexFemale && f.Wife == nil:
					f.Wife = ind
				case ind.Sex != SexFemale && f.Husband == nil:
					f.Husband = ind
				case f.Wife == nil:
					f.Wife = ind
				}
			}
		}
	}

	// Links recorded only on the family side.
	for _, f := range d.Families {
		for _, p := range f.Partners() {
			if !containsFam(p.spouseIn, f) {
				p.spouseIn = append(p.spouseIn, f)
			}
		}
		for _, c := range f.Children {
			if !hasChildLink(c, f) {
				c.childOf = append(c.childOf, FamilyLink{Family: f})
			}
		}
	}

	d.resolvePedigrees()

	// Detect people who are their own ancestors; such loops would otherwise
	// confuse tree rendering and relationship calculations.
	d.checkAncestryLoops()
}

// resolvePedigrees works out the kind of link between children and each
// partner of their families. The PEDI value applies to both partners unless
// the family records them separately (CHIL._FREL and _MREL, written by
// several programs) or an adoption event names the adopting partner
// (ADOP.FAMC.ADOP HUSB or WIFE). The partner who did not adopt is then
// linked as in the child's other families, e.g. as a birth parent.
func (d *Document) resolvePedigrees() {
	const notAdopter Pedigree = "\x00"
	for _, ind := range d.Individuals {
		adopters := map[*Family]string{}
		for _, e := range ind.Events {
			if e.Tag != "ADOP" {
				continue
			}
			if famc := e.Node.First("FAMC"); famc != nil && famc.IsPointer() {
				if f := d.fams[StripXref(famc.Value)]; f != nil {
					adopters[f] = strings.ToUpper(strings.TrimSpace(famc.Val("ADOP")))
				}
			}
		}
		for i := range ind.childOf {
			l := &ind.childOf[i]
			l.Husband, l.Wife = l.Pedigree, l.Pedigree
			for _, c := range l.Family.Node.Children {
				if c.Tag == "CHIL" && StripXref(c.Value) == ind.ID {
					if v := c.Val("_FREL"); v != "" {
						l.Husband = ParsePedigree(v)
					}
					if v := c.Val("_MREL"); v != "" {
						l.Wife = ParsePedigree(v)
					}
					break
				}
			}
			who, ok := adopters[l.Family]
			if !ok {
				continue
			}
			husband, wife := who != "WIFE", who != "HUSB"
			if husband {
				l.Husband = PedigreeAdopted
			} else if l.Husband == PedigreeAdopted {
				l.Husband = notAdopter
			}
			if wife {
				l.Wife = PedigreeAdopted
			} else if l.Wife == PedigreeAdopted {
				l.Wife = notAdopter
			}
		}
		// A partner who did not adopt keeps the closest link recorded in
		// another family, else an unspecified one.
		resolve := func(p *Individual) Pedigree {
			best := PedigreeOther
			found := false
			for _, l := range ind.childOf {
				if l.Family.Husband != p && l.Family.Wife != p {
					continue
				}
				if k := l.Of(p); k != notAdopter && (!found || pedigreeRank(k) < pedigreeRank(best)) {
					best, found = k, true
				}
			}
			if !found {
				return PedigreeUnknown
			}
			return best
		}
		for i := range ind.childOf {
			l := &ind.childOf[i]
			if l.Husband == notAdopter {
				l.Husband = resolve(l.Family.Husband)
			}
			if l.Wife == notAdopter {
				l.Wife = resolve(l.Family.Wife)
			}
		}
	}
}

func hasChildLink(ind *Individual, f *Family) bool {
	for _, l := range ind.childOf {
		if l.Family == f {
			return true
		}
	}
	return false
}

func containsFam(list []*Family, f *Family) bool {
	for _, x := range list {
		if x == f {
			return true
		}
	}
	return false
}

func (d *Document) checkAncestryLoops() {
	const (
		unvisited = iota
		active
		done
	)
	state := make(map[*Individual]int, len(d.Individuals))
	var reported []string
	var visit func(ind *Individual) bool
	visit = func(ind *Individual) bool {
		switch state[ind] {
		case active:
			return true
		case done:
			return false
		}
		state[ind] = active
		loop := false
		for _, p := range ind.Parents() {
			if visit(p) {
				loop = true
			}
		}
		state[ind] = done
		if loop && len(reported) < 20 {
			reported = append(reported, ind.ID)
		}
		return false
	}
	for _, ind := range d.Individuals {
		visit(ind)
	}
	if len(reported) > 0 {
		sort.Strings(reported)
		d.warn(0, "ancestry loop detected: %s appear(s) among their own ancestors", strings.Join(reported, ", "))
	}
}
