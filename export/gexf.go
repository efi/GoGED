package export

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"

	"github.com/efi/goged/gedcom"
)

// GEXF 1.2, the version that Gephi, networkx and sigma.js all read.
const (
	gexfNamespace = "http://www.gexf.net/1.2draft"
	gexfSchema    = "http://www.gexf.net/1.2draft http://www.gexf.net/1.2draft/gexf.xsd"
)

type gexfAttribute struct {
	ID    string `xml:"id,attr"`
	Title string `xml:"title,attr"`
	Type  string `xml:"type,attr"`
}

type gexfAttributes struct {
	XMLName xml.Name        `xml:"attributes"`
	Class   string          `xml:"class,attr"`
	List    []gexfAttribute `xml:"attribute"`
}

var (
	gexfNodeAttributes = gexfAttributes{Class: "node", List: []gexfAttribute{
		{"sex", "sex", "string"},
		{"birth", "birth", "string"},
		{"birthyear", "birth year", "integer"},
		{"birthplace", "birth place", "string"},
		{"death", "death", "string"},
		{"deathyear", "death year", "integer"},
		{"deathplace", "death place", "string"},
	}}
	gexfEdgeAttributes = gexfAttributes{Class: "edge", List: []gexfAttribute{
		{"relation", "relation", "string"},
		{"pedigree", "pedigree", "string"},
		{"family", "family", "string"},
		{"marriageyear", "marriage year", "integer"},
	}}
)

type gexfMeta struct {
	XMLName      xml.Name `xml:"meta"`
	LastModified string   `xml:"lastmodifieddate,attr,omitempty"`
	Creator      string   `xml:"creator"`
	Description  string   `xml:"description"`
}

type gexfValue struct {
	For   string `xml:"for,attr"`
	Value string `xml:"value,attr"`
}

// gexfValues are the attribute values of a node or an edge; empty values
// are left out.
type gexfValues []gexfValue

func (v *gexfValues) add(id, value string) {
	if value != "" {
		*v = append(*v, gexfValue{id, value})
	}
}

type gexfNode struct {
	XMLName xml.Name    `xml:"node"`
	ID      string      `xml:"id,attr"`
	Label   string      `xml:"label,attr"`
	Values  []gexfValue `xml:"attvalues>attvalue"`
}

type gexfEdge struct {
	XMLName xml.Name    `xml:"edge"`
	ID      string      `xml:"id,attr"`
	Source  string      `xml:"source,attr"`
	Target  string      `xml:"target,attr"`
	Type    string      `xml:"type,attr,omitempty"`
	Label   string      `xml:"label,attr,omitempty"`
	Values  []gexfValue `xml:"attvalues>attvalue"`
}

// WriteGEXF writes the selection as a GEXF graph. People are the nodes,
// labeled with their names, with sex, birth and death as attributes. A
// directed edge leads from each parent to each child and records how the
// child is linked (pedigree: birth, adopted, ...); a mutual edge, which
// goes both ways, links the partners of a family and records the year of
// their marriage. Mutual rather than undirected edges keep the graph
// directed throughout, which networkx requires.
func WriteGEXF(w io.Writer, sel *Selection, opts Options) error {
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	start := func(name string, attrs ...string) error {
		el := xml.StartElement{Name: xml.Name{Local: name}}
		for i := 0; i+1 < len(attrs); i += 2 {
			el.Attr = append(el.Attr, xml.Attr{Name: xml.Name{Local: attrs[i]}, Value: attrs[i+1]})
		}
		return enc.EncodeToken(el)
	}
	end := func(name string) error { return enc.EncodeToken(xml.EndElement{Name: xml.Name{Local: name}}) }

	meta := gexfMeta{Creator: "goged", Description: Describe(sel, opts)}
	if !opts.Generated.IsZero() {
		meta.LastModified = opts.Generated.Format("2006-01-02")
	}
	steps := []func() error{
		func() error {
			return start("gexf", "xmlns", gexfNamespace, "xmlns:xsi", "http://www.w3.org/2001/XMLSchema-instance",
				"xsi:schemaLocation", gexfSchema, "version", "1.2")
		},
		func() error { return enc.Encode(meta) },
		func() error { return start("graph", "mode", "static", "defaultedgetype", "directed") },
		func() error { return enc.Encode(gexfNodeAttributes) },
		func() error { return enc.Encode(gexfEdgeAttributes) },
		func() error { return start("nodes") },
		func() error {
			for _, ind := range sel.People {
				if err := enc.Encode(gexfPerson(ind)); err != nil {
					return err
				}
			}
			return nil
		},
		func() error { return end("nodes") },
		func() error { return start("edges") },
		func() error {
			for i, l := range sel.Links {
				if err := enc.Encode(gexfLink(i, l)); err != nil {
					return err
				}
			}
			return nil
		},
		func() error { return end("edges") },
		func() error { return end("graph") },
		func() error { return end("gexf") },
		enc.Flush,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// Describe says what an export contains, e.g. "All 37 people of
// family.ged and their relationships".
func Describe(sel *Selection, opts Options) string {
	source := opts.Title
	if source == "" {
		source = "a GEDCOM file"
	}
	people := fmt.Sprintf("%d people", len(sel.People))
	if len(sel.People) == 1 {
		people = "1 person"
	}
	if opts.Scope == Component && opts.Root != nil {
		return fmt.Sprintf("%s of %s connected to %s (%s) and their relationships", people, source, opts.Root.DisplayName(), opts.Root.ID)
	}
	return fmt.Sprintf("All %s of %s and their relationships", people, source)
}

func gexfPerson(ind *gedcom.Individual) gexfNode {
	n := gexfNode{ID: ind.ID, Label: ind.DisplayName()}
	var v gexfValues
	v.add("sex", ind.Sex.String())
	for _, part := range []struct {
		tags []string
		id   string
	}{{gedcom.BirthTags, "birth"}, {gedcom.DeathTags, "death"}} {
		e := ind.FirstDatedEvent(part.tags...)
		if e == nil {
			continue
		}
		if !e.Date.IsZero() {
			v.add(part.id, e.Date.String())
		}
		if e.Date.IsValid() {
			v.add(part.id+"year", strconv.Itoa(e.Date.Year()))
		}
		v.add(part.id+"place", e.Place.String())
	}
	n.Values = v
	return n
}

func gexfLink(i int, l Link) gexfEdge {
	e := gexfEdge{ID: strconv.Itoa(i), Source: l.From.ID, Target: l.To.ID}
	var v gexfValues
	switch l.Kind {
	case Partners:
		e.Type, e.Label = "mutual", "partners"
		v.add("relation", "partners")
		v.add("family", l.Family.ID)
		if m := l.Family.Marriage(); m != nil && m.Date.IsValid() {
			v.add("marriageyear", strconv.Itoa(m.Date.Year()))
		}
	default:
		e.Label = map[gedcom.Sex]string{gedcom.SexMale: "father", gedcom.SexFemale: "mother"}[l.From.Sex]
		if e.Label == "" {
			e.Label = "parent"
		}
		v.add("relation", "parent")
		v.add("pedigree", string(l.Pedigree))
		v.add("family", l.Family.ID)
	}
	e.Values = v
	return e
}
