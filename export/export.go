// Package export writes a GEDCOM document, or the part of it that is
// selected, in formats that other programs read. The first format is GEXF,
// the Graph Exchange XML Format of Gephi, networkx and sigma.js.
//
// Select decides which people are exported and how they are linked, so
// that every format exports the same graph. Filters still to come, such as
// a temporal cutoff, belong in Options and Select for the same reason.
package export

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/efi/goged/gedcom"
)

// Scope selects which people are exported.
type Scope int

const (
	// All exports every person of the document.
	All Scope = iota
	// Component exports Options.Root and everybody connected to them
	// through parents, children and partners, however distantly.
	Component
)

// Options control an export.
type Options struct {
	Scope Scope
	// Root is the person whose connected component is exported.
	Root *gedcom.Individual
	// Title names the source in the exported file, e.g. its file name.
	Title string
	// Generated is the date recorded in the file; zero leaves it out.
	Generated time.Time
}

// ErrNoRoot is returned when the people connected to nobody are to be
// exported.
var ErrNoRoot = errors.New("export: no person to export the connected people of")

// LinkKind tells parent and partner links apart.
type LinkKind int

const (
	// Parent links a parent (From) to their child (To).
	Parent LinkKind = iota
	// Partners links the two partners of a family.
	Partners
)

// Link is a relationship between two people, recorded by Family.
type Link struct {
	Kind     LinkKind
	From, To *gedcom.Individual
	Family   *gedcom.Family
	// Pedigree is how the child is linked to this parent: birth, adopted,
	// foster, ...; PedigreeUnknown if the file does not say. Parent links
	// only.
	Pedigree gedcom.Pedigree
}

// Selection is the part of a document to export.
type Selection struct {
	// People are the selected people in file order.
	People []*gedcom.Individual
	// Links are the relationships between them, family by family.
	Links []Link

	has map[*gedcom.Individual]bool
}

// Has reports whether ind is selected.
func (s *Selection) Has(ind *gedcom.Individual) bool { return s.has[ind] }

// Select works out which people opts exports and how they are linked.
func Select(doc *gedcom.Document, opts Options) (*Selection, error) {
	all := links(doc)
	sel := &Selection{has: map[*gedcom.Individual]bool{}}
	switch opts.Scope {
	case Component:
		if opts.Root == nil || doc.Individual(opts.Root.ID) != opts.Root {
			return nil, ErrNoRoot
		}
		sel.has = component(opts.Root, all)
	default:
		for _, ind := range doc.Individuals {
			sel.has[ind] = true
		}
	}
	for _, ind := range doc.Individuals {
		if sel.has[ind] {
			sel.People = append(sel.People, ind)
		}
	}
	// A component contains both ends of every link it touches.
	for _, l := range all {
		if sel.has[l.From] && sel.has[l.To] {
			sel.Links = append(sel.Links, l)
		}
	}
	return sel, nil
}

// links returns all relationships of the document: the partners of each
// family, and each partner with each child of the family.
func links(doc *gedcom.Document) []Link {
	var out []Link
	for _, f := range doc.Families {
		partners := f.Partners()
		if len(partners) == 2 {
			out = append(out, Link{Kind: Partners, From: partners[0], To: partners[1], Family: f})
		}
		for _, c := range f.Children {
			l, _ := c.ChildLink(f)
			for _, p := range partners {
				if p != c {
					out = append(out, Link{Kind: Parent, From: p, To: c, Family: f, Pedigree: l.Of(p)})
				}
			}
		}
	}
	return out
}

// component returns root and everybody connected to them by links.
func component(root *gedcom.Individual, links []Link) map[*gedcom.Individual]bool {
	next := map[*gedcom.Individual][]*gedcom.Individual{}
	for _, l := range links {
		next[l.From] = append(next[l.From], l.To)
		next[l.To] = append(next[l.To], l.From)
	}
	seen := map[*gedcom.Individual]bool{root: true}
	queue := []*gedcom.Individual{root}
	for len(queue) > 0 {
		ind := queue[0]
		queue = queue[1:]
		for _, n := range next[ind] {
			if !seen[n] {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	return seen
}

// Format is a file format to export to.
type Format struct {
	Name        string // e.g. "GEXF"
	Description string // what the format is and which programs read it
	Extension   string // file name extension, e.g. ".gexf"
	Write       func(w io.Writer, sel *Selection, opts Options) error
}

// Formats lists the formats to export to.
var Formats = []Format{
	{Name: "GEXF", Description: "Graph Exchange XML Format, for Gephi, networkx and sigma.js", Extension: ".gexf", Write: WriteGEXF},
}

// WriteFile exports to the file at path. The export goes to a temporary
// file next to it first, which then replaces the file, so a failed export
// leaves an existing file as it was.
func WriteFile(path string, f Format, sel *Selection, opts Options) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // fails harmlessly once renamed
	w := bufio.NewWriter(tmp)
	err = f.Write(w, sel, opts)
	if err == nil {
		err = w.Flush()
	}
	if err == nil {
		err = tmp.Chmod(0o644)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
