package tui

import (
	"sort"
	"strings"
	"sync"

	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
	"github.com/efi/goged/search"
)

// Shared is a document with what is worked out from it alone: the search
// indexes, the places, the places on the map and where events count
// there, the statistics and the birth surnames. None of it changes once
// it is built, so the models showing one document can share it, as the
// clients of goged's server do. Each part is built when a model first
// needs it, and only once; Shared is safe for concurrent use.
type Shared struct {
	doc   *gedcom.Document
	index *search.Index

	events   func() *search.EventIndex
	places   func() *placeData
	located  func() *locatedData
	stats    func() genealogy.Stats
	surnames func() *surnameData
}

// placeData is the place tree with the folded name of every place and the
// place of every event.
type placeData struct {
	root   *genealogy.PlaceNode
	folded map[*genealogy.PlaceNode]string
	nodeOf map[*gedcom.Event]*genealogy.PlaceNode
}

// locatedData are the places with coordinates, the map's markers, the
// most events first; the marker each event counts at, that of its place
// or of the nearest place around it with coordinates; and how many places
// with events have no coordinates.
type locatedData struct {
	places   []*genealogy.PlaceNode
	markerOf map[*gedcom.Event]int
	unknown  int
}

// surnameData are the birth surnames, most people first, and the birth
// surname of everybody who has one.
type surnameData struct {
	names     []surnameEntry
	surnameOf map[*gedcom.Individual]string
}

// NewShared prepares doc for the models that show it. The search index is
// built at once, as every model starts with the search; the rest as it is
// needed.
func NewShared(doc *gedcom.Document) *Shared {
	sh := &Shared{doc: doc, index: search.NewIndex(doc)}
	sh.events = sync.OnceValue(func() *search.EventIndex { return search.NewEventIndex(doc.Events()) })
	sh.places = sync.OnceValue(sh.buildPlaces)
	sh.located = sync.OnceValue(sh.buildLocated)
	sh.stats = sync.OnceValue(func() genealogy.Stats { return genealogy.Compute(doc) })
	sh.surnames = sync.OnceValue(sh.buildSurnames)
	return sh
}

// Doc returns the document.
func (sh *Shared) Doc() *gedcom.Document { return sh.doc }

func (sh *Shared) buildPlaces() *placeData {
	p := &placeData{
		root:   genealogy.Places(sh.doc),
		folded: map[*genealogy.PlaceNode]string{},
		nodeOf: map[*gedcom.Event]*genealogy.PlaceNode{},
	}
	p.root.Walk(func(n *genealogy.PlaceNode) bool {
		p.folded[n] = search.Fold(n.Name)
		for _, e := range n.Events {
			p.nodeOf[e] = n
		}
		return true
	})
	return p
}

func (sh *Shared) buildLocated() *locatedData {
	root := sh.places().root
	l := &locatedData{places: root.Located(), markerOf: map[*gedcom.Event]int{}}
	sort.SliceStable(l.places, func(i, j int) bool {
		a, b := l.places[i], l.places[j]
		if a.Count() != b.Count() {
			return a.Count() > b.Count()
		}
		return a.Full < b.Full
	})
	index := map[*genealogy.PlaceNode]int{}
	for i, p := range l.places {
		index[p] = i
	}
	var walk func(n *genealogy.PlaceNode, at int)
	walk = func(n *genealogy.PlaceNode, at int) {
		if i, ok := index[n]; ok {
			at = i
		}
		if len(n.Events) > 0 && !n.HasCoords {
			l.unknown++
		}
		if at >= 0 {
			for _, e := range n.Events {
				l.markerOf[e] = at
			}
		}
		for _, c := range n.Children {
			walk(c, at)
		}
	}
	walk(root, -1)
	return l
}

func (sh *Shared) buildSurnames() *surnameData {
	s := &surnameData{surnameOf: map[*gedcom.Individual]string{}}
	count := map[string]int{}
	for _, ind := range sh.doc.Individuals {
		if name := strings.TrimSpace(ind.BirthName().FullSurname()); name != "" {
			s.surnameOf[ind] = name
			count[name]++
		}
	}
	for name, n := range count {
		s.names = append(s.names, surnameEntry{name, n})
	}
	sort.Slice(s.names, func(i, j int) bool {
		a, b := s.names[i], s.names[j]
		if a.people != b.people {
			return a.people > b.people
		}
		return search.Fold(a.name) < search.Fold(b.name)
	})
	return s
}
