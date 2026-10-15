package search

import (
	"fmt"
	"slices"
	"sync"

	"github.com/efi/goged/gedcom"
)

// ancestor:I1 finds the ancestors of I1 and descendant:I1 its descendants,
// along birth links, as relationships are counted: adoptive, foster and
// step parents are not ancestors.

// birthLinks holds the birth parents and birth children of every entry,
// by their places in the index, built when a query first needs them.
type birthLinks struct {
	once              sync.Once
	parents, children [][]int32
}

func (ix *Index) birthLinks() (parents, children [][]int32) {
	l := &ix.links
	l.once.Do(func() {
		pos := make(map[*gedcom.Individual]int32, len(ix.entries))
		for _, e := range ix.entries {
			pos[e.ind] = e.pos
		}
		l.parents = make([][]int32, len(ix.entries))
		l.children = make([][]int32, len(ix.entries))
		for _, e := range ix.entries {
			for _, p := range e.ind.BirthParents() {
				if i, ok := pos[p]; ok {
					l.parents[e.pos] = append(l.parents[e.pos], i)
					l.children[i] = append(l.children[i], e.pos)
				}
			}
		}
	})
	return l.parents, l.children
}

// resolve finds the people that the ancestor: and descendant: terms of q
// stand for, in a copy of the terms. It reports an ID nobody has; such a
// term matches nobody.
func (ix *Index) resolve(q *Query) error {
	var err error
	cloned := false
	for i := range q.Terms {
		if f := q.Terms[i].Field; f != FieldAncestor && f != FieldDescendant {
			continue
		}
		if !cloned {
			q.Terms = slices.Clone(q.Terms)
			cloned = true
		}
		t := &q.Terms[i]
		root := ix.find(t.kinID)
		if root < 0 {
			t.kin = make([]bool, len(ix.entries))
			if err == nil {
				err = fmt.Errorf("%s: nobody has the ID %s", t.Field, t.kinID)
			}
			continue
		}
		parents, children := ix.birthLinks()
		next := children
		if t.Field == FieldAncestor {
			next = parents
		}
		t.kin = relatives(next, root, t.gens)
	}
	return err
}

// find returns the place in the index of the person with an ID, which may
// differ in case, or -1.
func (ix *Index) find(id string) int32 {
	if ind := ix.doc.Individual(id); ind != nil {
		for _, e := range ix.entries {
			if e.ind == ind {
				return e.pos
			}
		}
	}
	folded := Fold(id)
	for _, e := range ix.entries {
		if e.id == folded {
			return e.pos
		}
	}
	return -1
}

// relatives marks the people reached from root by following next, the
// parents or the children, whose generation lies in gens: 0 is root, 1 the
// parents or children. Somebody who is in several generations, as after a
// marriage between cousins, counts if any of them does. A file in which
// somebody is their own ancestor is followed only for as many generations
// as there are people: no other file has more.
func relatives(next [][]int32, root int32, gens yearRange) []bool {
	in := make([]bool, len(next))
	reached := make([]int32, len(next)) // the last generation + 1 reached in
	level := []int32{root}
	for g := 0; len(level) > 0 && g <= gens.hi && g <= len(next); g++ {
		if g >= gens.lo {
			// From here on everybody reached counts, so whoever counts
			// already need not be followed again.
			fresh := level[:0]
			for _, i := range level {
				if !in[i] {
					in[i] = true
					fresh = append(fresh, i)
				}
			}
			level = fresh
		}
		var following []int32
		for _, i := range level {
			for _, r := range next[i] {
				if reached[r] != int32(g+2) {
					reached[r] = int32(g + 2)
					following = append(following, r)
				}
			}
		}
		level = following
	}
	return in
}
