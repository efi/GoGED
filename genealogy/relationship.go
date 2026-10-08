// Package genealogy provides analyses on top of the GEDCOM model:
// relationship calculation, personal timelines and file statistics.
package genealogy

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/efi/goged/gedcom"
)

// Kind classifies a relationship.
type Kind int

// Relationship kinds.
const (
	KindNone     Kind = iota // no relationship found
	KindSelf                 // the same person
	KindBlood                // related by descent
	KindMarriage             // related through a marriage (spouse, in-law, step)
)

// Relationship describes how person B is related to person A.
type Relationship struct {
	Kind Kind
	// Description completes the sentence "B is A's ...", e.g. "father",
	// "half-sister" or "second cousin once removed".
	Description string
	// UpA and UpB count the generations from A and from B up to the
	// closest common ancestor(s) of the blood relationship.
	UpA, UpB int
	// Half is set for collateral relatives that share only one ancestor.
	Half bool
	// CommonAncestors are the closest shared ancestors.
	CommonAncestors []*gedcom.Individual
	// Via is the spouse through whom a marriage relationship runs.
	Via *gedcom.Individual
}

// String returns the description.
func (r Relationship) String() string { return r.Description }

// Relate determines how b is related to a. Blood relationships take
// precedence over relationships by marriage.
func Relate(a, b *gedcom.Individual) Relationship {
	if a == nil || b == nil {
		return Relationship{Description: "no relationship found"}
	}
	if r, ok := blood(a, b); ok {
		return r
	}
	for _, s := range a.Spouses() {
		if s == b {
			return Relationship{Kind: KindMarriage, Description: gendered(b.Sex, "husband", "wife", "spouse"), Via: s}
		}
	}
	// b is a blood relative of a's spouse.
	for _, s := range a.Spouses() {
		if r, ok := blood(s, b); ok && r.Kind == KindBlood {
			r.Kind = KindMarriage
			r.Description = inLawViaSpouse(r, s, b)
			r.Via = s
			return r
		}
	}
	// b is the spouse of a's blood relative.
	for _, s := range b.Spouses() {
		if r, ok := blood(a, s); ok && r.Kind == KindBlood {
			r.Kind = KindMarriage
			r.Description = inLawViaRelative(r, b)
			r.Via = s
			return r
		}
	}
	return Relationship{Description: "no relationship found"}
}

// ancestorDistances maps every ancestor of ind (and ind itself, at 0) to the
// shortest number of generations separating them.
func ancestorDistances(ind *gedcom.Individual) map[*gedcom.Individual]int {
	dist := map[*gedcom.Individual]int{ind: 0}
	queue := []*gedcom.Individual{ind}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, p := range cur.Parents() {
			if _, seen := dist[p]; !seen {
				dist[p] = dist[cur] + 1
				queue = append(queue, p)
			}
		}
	}
	return dist
}

// familyDistances maps every ancestral family of ind to its distance: the
// families ind is a child of are at distance 1, their partners' parental
// families at 2, and so on.
func familyDistances(ind *gedcom.Individual) map[*gedcom.Family]int {
	dist := map[*gedcom.Family]int{}
	var queue []*gedcom.Family
	for _, l := range ind.FamiliesAsChild() {
		if _, seen := dist[l.Family]; !seen {
			dist[l.Family] = 1
			queue = append(queue, l.Family)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, p := range cur.Partners() {
			for _, l := range p.FamiliesAsChild() {
				if _, seen := dist[l.Family]; !seen {
					dist[l.Family] = dist[cur] + 1
					queue = append(queue, l.Family)
				}
			}
		}
	}
	return dist
}

type candidate struct {
	upA, upB int
	id       string
}

func (c candidate) less(o candidate) bool {
	if s1, s2 := c.upA+c.upB, o.upA+o.upB; s1 != s2 {
		return s1 < s2
	}
	if d1, d2 := abs(c.upA-c.upB), abs(o.upA-o.upB); d1 != d2 {
		return d1 < d2
	}
	if c.upA != o.upA {
		return c.upA < o.upA
	}
	return c.id < o.id
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// blood computes the blood relationship of b to a, if any.
func blood(a, b *gedcom.Individual) (Relationship, bool) {
	if a == b {
		return Relationship{Kind: KindSelf, Description: "self"}, true
	}
	ancA := ancestorDistances(a)
	if d, ok := ancA[b]; ok {
		return Relationship{Kind: KindBlood, UpA: d, Description: describe(d, 0, b.Sex, false), CommonAncestors: []*gedcom.Individual{b}}, true
	}
	ancB := ancestorDistances(b)
	if d, ok := ancB[a]; ok {
		return Relationship{Kind: KindBlood, UpB: d, Description: describe(0, d, b.Sex, false), CommonAncestors: []*gedcom.Individual{a}}, true
	}

	// Closest common family (both partners shared: a full relationship).
	var bestFam *gedcom.Family
	var famCand candidate
	famB := familyDistances(b)
	for f, da := range familyDistances(a) {
		db, ok := famB[f]
		if !ok {
			continue
		}
		c := candidate{da, db, f.ID}
		if bestFam == nil || c.less(famCand) {
			bestFam, famCand = f, c
		}
	}

	// Closest common individual (possibly only one shared parent: half).
	var bestInd *gedcom.Individual
	var indCand candidate
	for p, da := range ancA {
		db, ok := ancB[p]
		if !ok {
			continue
		}
		c := candidate{da, db, p.ID}
		if bestInd == nil || c.less(indCand) {
			bestInd, indCand = p, c
		}
	}

	switch {
	case bestFam != nil && (bestInd == nil || famCand.upA+famCand.upB <= indCand.upA+indCand.upB):
		common := bestFam.Partners()
		sort.Slice(common, func(i, j int) bool { return common[i].ID < common[j].ID })
		return Relationship{
			Kind:            KindBlood,
			UpA:             famCand.upA,
			UpB:             famCand.upB,
			Description:     describe(famCand.upA, famCand.upB, b.Sex, false),
			CommonAncestors: common,
		}, true
	case bestInd != nil:
		return Relationship{
			Kind:            KindBlood,
			UpA:             indCand.upA,
			UpB:             indCand.upB,
			Half:            true,
			Description:     describe(indCand.upA, indCand.upB, b.Sex, true),
			CommonAncestors: []*gedcom.Individual{bestInd},
		}, true
	}
	return Relationship{}, false
}

func gendered(sex gedcom.Sex, male, female, neutral string) string {
	switch sex {
	case gedcom.SexMale:
		return male
	case gedcom.SexFemale:
		return female
	}
	return neutral
}

var ordinalWords = []string{"", "first", "second", "third", "fourth", "fifth", "sixth", "seventh", "eighth", "ninth", "tenth"}

// Ordinal returns "1st", "2nd", "3rd", "4th", "11th", "21st", ...
func Ordinal(n int) string {
	suffix := "th"
	switch n % 100 {
	case 11, 12, 13:
	default:
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return strconv.Itoa(n) + suffix
}

func greatPrefix(n int) string {
	switch {
	case n <= 0:
		return ""
	case n == 1:
		return "great-"
	}
	return Ordinal(n) + " great-"
}

func removed(n int) string {
	switch n {
	case 0:
		return ""
	case 1:
		return " once removed"
	case 2:
		return " twice removed"
	case 3:
		return " thrice removed"
	}
	return fmt.Sprintf(" %d times removed", n)
}

// describe names the relationship of B to A given the generations from each
// up to the common ancestor and B's sex.
func describe(upA, upB int, sex gedcom.Sex, half bool) string {
	h := ""
	if half {
		h = "half-"
	}
	switch {
	case upA == 0 && upB == 0:
		return "self"
	case upB == 0: // B is an ancestor of A
		base := gendered(sex, "father", "mother", "parent")
		switch upA {
		case 1:
			return base
		case 2:
			return "grand" + base
		}
		return greatPrefix(upA-2) + "grand" + base
	case upA == 0: // B is a descendant of A
		base := gendered(sex, "son", "daughter", "child")
		switch upB {
		case 1:
			return base
		case 2:
			return "grand" + base
		}
		return greatPrefix(upB-2) + "grand" + base
	case upA == 1 && upB == 1:
		return h + gendered(sex, "brother", "sister", "sibling")
	case upA == 1: // B descends from A's sibling
		return h + greatPrefix(upB-2) + gendered(sex, "nephew", "niece", "nephew/niece")
	case upB == 1: // B is a sibling of A's ancestor
		return h + greatPrefix(upA-2) + gendered(sex, "uncle", "aunt", "uncle/aunt")
	}
	degree := min(upA, upB) - 1
	word := Ordinal(degree)
	if degree < len(ordinalWords) {
		word = ordinalWords[degree]
	}
	prefix := ""
	if half {
		prefix = "half "
	}
	return prefix + word + " cousin" + removed(abs(upA-upB))
}

// inLawViaSpouse names b, a blood relative of a's spouse s (r describes b
// relative to s).
func inLawViaSpouse(r Relationship, s, b *gedcom.Individual) string {
	switch {
	case r.UpA == 1 && r.UpB == 0:
		return gendered(b.Sex, "father", "mother", "parent") + "-in-law"
	case r.UpA == 1 && r.UpB == 1:
		return gendered(b.Sex, "brother", "sister", "sibling") + "-in-law"
	case r.UpA == 0 && r.UpB == 1:
		return gendered(b.Sex, "stepson", "stepdaughter", "stepchild")
	}
	return gendered(s.Sex, "husband", "wife", "spouse") + "'s " + r.Description
}

// inLawViaRelative names b, the spouse of a's blood relative (r describes
// that relative relative to a).
func inLawViaRelative(r Relationship, b *gedcom.Individual) string {
	switch {
	case r.UpA == 0 && r.UpB == 1:
		return gendered(b.Sex, "son", "daughter", "child") + "-in-law"
	case r.UpA == 1 && r.UpB == 1:
		return gendered(b.Sex, "brother", "sister", "sibling") + "-in-law"
	case r.UpA == 1 && r.UpB == 0:
		return gendered(b.Sex, "stepfather", "stepmother", "step-parent")
	}
	return gendered(b.Sex, "husband", "wife", "spouse") + " of " + r.Description
}
