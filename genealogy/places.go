package genealogy

import (
	"sort"
	"strings"

	"github.com/efi/goged/gedcom"
)

// PlaceNode is a jurisdiction in the place hierarchy. GEDCOM places list
// jurisdictions from the smallest to the largest ("Leeds, Yorkshire,
// England"); the hierarchy turns them around so that England contains
// Yorkshire, which contains Leeds.
type PlaceNode struct {
	Name     string // this jurisdiction, e.g. "Leeds"
	Full     string // the place from here up, e.g. "Leeds, Yorkshire, England"
	Depth    int    // 0 for the root, 1 for top-level jurisdictions
	Parent   *PlaceNode
	Children []*PlaceNode // sorted by name
	// Events that took place exactly here (not in sub-places).
	Events []*gedcom.Event
	// Lat and Lon locate the place if HasCoords is set; they come from the
	// first event here whose place has coordinates.
	Lat, Lon  float64
	HasCoords bool
	// Location is the place record (_LOC) of the events here, if any, and
	// GOV the identifier of the place in the GOV gazetteer.
	Location *gedcom.Location
	GOV      string
	// Aliases are other ways in which the place is written in the file.
	// Events that refer to the same place record are grouped under the
	// place as written for the most recent of them.
	Aliases []string

	count  int // events here and in all sub-places
	people int // distinct people involved in those events
}

// Count returns the number of events here and in all sub-places.
func (n *PlaceNode) Count() int { return n.count }

// People returns the number of distinct people with events here or in a
// sub-place.
func (n *PlaceNode) People() int { return n.people }

// AllEvents returns the events here and in all sub-places in
// chronological order; undated events come last.
func (n *PlaceNode) AllEvents() []*gedcom.Event {
	var out []*gedcom.Event
	n.Walk(func(p *PlaceNode) bool {
		out = append(out, p.Events...)
		return true
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.Compare(out[j].Date) < 0 })
	return out
}

// Walk visits n and its sub-places depth first in name order. Returning
// false from fn skips the sub-places of that node.
func (n *PlaceNode) Walk(fn func(*PlaceNode) bool) {
	if !fn(n) {
		return
	}
	for _, c := range n.Children {
		c.Walk(fn)
	}
}

// Located returns n and all its sub-places that have coordinates.
func (n *PlaceNode) Located() []*PlaceNode {
	var out []*PlaceNode
	n.Walk(func(p *PlaceNode) bool {
		if p.HasCoords {
			out = append(out, p)
		}
		return true
	})
	return out
}

// Path returns the names from the top-level jurisdiction down to n.
func (n *PlaceNode) Path() []string {
	var out []string
	for p := n; p != nil && p.Depth > 0; p = p.Parent {
		out = append([]string{p.Name}, out...)
	}
	return out
}

// Places builds the place hierarchy of all events in the document. The
// returned root has no name; its children are the top-level jurisdictions.
// Jurisdiction names are matched case-insensitively; the first spelling
// seen is kept. Places that refer to the same place record (_LOC) are
// grouped, see PlaceNode.Aliases.
func Places(doc *gedcom.Document) *PlaceNode {
	root := &PlaceNode{}
	index := map[*PlaceNode]map[string]*PlaceNode{}
	people := map[*PlaceNode]map[*gedcom.Individual]bool{}

	child := func(parent *PlaceNode, name string) *PlaceNode {
		key := strings.ToLower(name)
		if index[parent] == nil {
			index[parent] = map[string]*PlaceNode{}
		}
		if c := index[parent][key]; c != nil {
			return c
		}
		full := name
		if parent.Full != "" {
			full += ", " + parent.Full
		}
		c := &PlaceNode{Name: name, Full: full, Depth: parent.Depth + 1, Parent: parent}
		index[parent][key] = c
		parent.Children = append(parent.Children, c)
		return c
	}

	events := doc.Events()
	canonical := canonicalPlaces(events)
	for _, e := range events {
		parts := e.Place.Parts()
		if len(parts) == 0 {
			continue
		}
		written := e.Place.String()
		if c, ok := canonical[e.Place.Location]; ok {
			parts = c.Parts()
		}
		node := root
		for i := len(parts) - 1; i >= 0; i-- {
			node = child(node, parts[i])
		}
		node.Events = append(node.Events, e)
		if !strings.EqualFold(written, node.Full) && !containsFold(node.Aliases, written) {
			node.Aliases = append(node.Aliases, written)
		}
		if node.Location == nil {
			node.Location = e.Place.Location
		}
		if node.GOV == "" {
			node.GOV = e.Place.GOV
		}
		if !node.HasCoords && e.Place.HasCoords {
			node.Lat, node.Lon, node.HasCoords = e.Place.Lat, e.Place.Lon, true
		}
		for p := node; p != nil; p = p.Parent {
			p.count++
			if people[p] == nil {
				people[p] = map[*gedcom.Individual]bool{}
			}
			for _, ind := range e.Principals() {
				people[p][ind] = true
			}
		}
	}

	root.Walk(func(n *PlaceNode) bool {
		n.people = len(people[n])
		// Names are unique ignoring case, so this order is total.
		sort.Slice(n.Children, func(i, j int) bool {
			return strings.ToLower(n.Children[i].Name) < strings.ToLower(n.Children[j].Name)
		})
		return true
	})
	return root
}

// canonicalPlaces picks, for every place record, the place as written for
// the most recent dated event referring to it, or for the first event if
// none is dated.
func canonicalPlaces(events []*gedcom.Event) map[*gedcom.Location]gedcom.Place {
	out := map[*gedcom.Location]gedcom.Place{}
	dates := map[*gedcom.Location]gedcom.Date{}
	for _, e := range events {
		l := e.Place.Location
		if l == nil || len(e.Place.Parts()) == 0 {
			continue
		}
		_, seen := out[l]
		later := e.Date.IsValid() && (!dates[l].IsValid() || e.Date.Compare(dates[l]) > 0)
		if !seen || later {
			out[l], dates[l] = e.Place, e.Date
		}
	}
	return out
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
