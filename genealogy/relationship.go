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
	KindAdoptive             // related through adoption, fostering or another non-birth link
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
	// Pedigree is the most distant kind of parent link (adopted, foster,
	// ...) on the way to the common ancestors of a KindAdoptive
	// relationship.
	Pedigree gedcom.Pedigree
}

// String returns the description.
func (r Relationship) String() string { return r.Description }

// Relate determines how b is related to a. Blood relationships, which
// follow birth (or unspecified) parent links only, take precedence over
// marriage and over relationships through adoption or fostering.
func Relate(a, b *gedcom.Individual) Relationship {
	if a == nil || b == nil {
		return Relationship{Description: "no relationship found"}
	}
	if r, ok := blood(a, b, true); ok {
		return r
	}
	for _, s := range a.Spouses() {
		if s == b {
			return Relationship{Kind: KindMarriage, Description: gendered(b.Sex, "husband", "wife", "spouse"), Via: s}
		}
	}
	if r, ok := blood(a, b, false); ok {
		return r
	}
	// b is a relative of a's spouse.
	for _, s := range a.Spouses() {
		if r, ok := kin(s, b); ok {
			r.Kind = KindMarriage
			r.Description = inLawViaSpouse(r, s, b)
			r.Via = s
			return r
		}
	}
	// b is the spouse of a's relative.
	for _, s := range b.Spouses() {
		if r, ok := kin(a, s); ok {
			r.Kind = KindMarriage
			r.Description = inLawViaRelative(r, b)
			r.Via = s
			return r
		}
	}
	return Relationship{Description: "no relationship found"}
}

// kin returns the blood or adoptive relationship of b to a, excluding a
// being b.
func kin(a, b *gedcom.Individual) (Relationship, bool) {
	if a == b {
		return Relationship{}, false
	}
	if r, ok := blood(a, b, true); ok {
		return r, true
	}
	return blood(a, b, false)
}

// reach is the distance to an ancestor (or ancestral family) together with
// the most distant kind of parent link on the way; PedigreeUnknown means
// birth links only.
type reach struct {
	dist int
	kind gedcom.Pedigree
}

// linkRank orders kinds of parent links from the closest to the most
// distant.
func linkRank(p gedcom.Pedigree) int {
	switch p {
	case gedcom.PedigreeUnknown, gedcom.PedigreeBirth:
		return 0
	case gedcom.PedigreeAdopted:
		return 1
	case gedcom.PedigreeSealed:
		return 2
	case gedcom.PedigreeOther:
		return 3
	case gedcom.PedigreeStep:
		return 4
	}
	return 5 // foster
}

// farther returns the more distant of two kinds of links.
func farther(a, b gedcom.Pedigree) gedcom.Pedigree {
	if linkRank(b) > linkRank(a) {
		a = b
	}
	if linkRank(a) == 0 {
		return gedcom.PedigreeUnknown
	}
	return a
}

// ancestorDistances maps every ancestor of ind (and ind itself, at 0) to the
// shortest number of generations separating them. With birthOnly, adoptive
// and other non-birth parents are not followed; otherwise the closest kind
// of link among the shortest paths is recorded.
func ancestorDistances(ind *gedcom.Individual, birthOnly bool) map[*gedcom.Individual]reach {
	dist := map[*gedcom.Individual]reach{ind: {}}
	level := []*gedcom.Individual{ind}
	for d := 1; len(level) > 0; d++ {
		var next []*gedcom.Individual
		for _, cur := range level {
			for _, pl := range cur.ParentLinks() {
				if birthOnly && !pl.Pedigree.IsBirth() {
					continue
				}
				k := farther(dist[cur].kind, pl.Pedigree)
				r, seen := dist[pl.Parent]
				switch {
				case !seen:
					dist[pl.Parent] = reach{d, k}
					next = append(next, pl.Parent)
				case r.dist == d && linkRank(k) < linkRank(r.kind):
					dist[pl.Parent] = reach{d, k}
				}
			}
		}
		level = next
	}
	return dist
}

// familyDistances maps every ancestral family of ind to its distance: the
// families ind is a child of are at distance 1, their partners' parental
// families at 2, and so on. With birthOnly, only families in which the
// child is a birth child of both partners are followed.
func familyDistances(ind *gedcom.Individual, birthOnly bool) map[*gedcom.Family]reach {
	dist := map[*gedcom.Family]reach{}
	var level []*gedcom.Family
	visit := func(child *gedcom.Individual, d int, kind gedcom.Pedigree, next *[]*gedcom.Family) {
		for _, l := range child.FamiliesAsChild() {
			k := kind
			for _, p := range l.Family.Partners() {
				k = farther(k, l.Of(p))
			}
			if birthOnly && k != gedcom.PedigreeUnknown {
				continue
			}
			r, seen := dist[l.Family]
			switch {
			case !seen:
				dist[l.Family] = reach{d, k}
				*next = append(*next, l.Family)
			case r.dist == d && linkRank(k) < linkRank(r.kind):
				dist[l.Family] = reach{d, k}
			}
		}
	}
	visit(ind, 1, gedcom.PedigreeUnknown, &level)
	for d := 2; len(level) > 0; d++ {
		var next []*gedcom.Family
		for _, f := range level {
			for _, p := range f.Partners() {
				visit(p, d, dist[f].kind, &next)
			}
		}
		level = next
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

// blood computes the relationship of b to a by descent. With birthOnly it
// follows birth links only and returns a KindBlood relationship; otherwise
// it also follows adoptive and other links and returns a KindAdoptive one
// if such a link is involved.
func blood(a, b *gedcom.Individual, birthOnly bool) (Relationship, bool) {
	if a == b {
		return Relationship{Kind: KindSelf, Description: "self"}, true
	}
	result := func(r Relationship, kind gedcom.Pedigree) (Relationship, bool) {
		r.Kind = KindBlood
		if kind != gedcom.PedigreeUnknown {
			r.Kind, r.Pedigree = KindAdoptive, kind
			r.Description = qualify(r.Description, kind)
		}
		return r, true
	}
	ancA := ancestorDistances(a, birthOnly)
	if r, ok := ancA[b]; ok {
		return result(Relationship{UpA: r.dist, Description: describe(r.dist, 0, b.Sex, false), CommonAncestors: []*gedcom.Individual{b}}, r.kind)
	}
	ancB := ancestorDistances(b, birthOnly)
	if r, ok := ancB[a]; ok {
		return result(Relationship{UpB: r.dist, Description: describe(0, r.dist, b.Sex, false), CommonAncestors: []*gedcom.Individual{a}}, r.kind)
	}

	// Closest common family (both partners shared: a full relationship).
	var bestFam *gedcom.Family
	var famCand candidate
	var famKind gedcom.Pedigree
	famB := familyDistances(b, birthOnly)
	for f, ra := range familyDistances(a, birthOnly) {
		rb, ok := famB[f]
		if !ok {
			continue
		}
		c := candidate{ra.dist, rb.dist, f.ID}
		if bestFam == nil || c.less(famCand) {
			bestFam, famCand, famKind = f, c, farther(ra.kind, rb.kind)
		}
	}

	// Closest common individual (possibly only one shared parent: half).
	var bestInd *gedcom.Individual
	var indCand candidate
	var indKind gedcom.Pedigree
	for p, ra := range ancA {
		rb, ok := ancB[p]
		if !ok {
			continue
		}
		c := candidate{ra.dist, rb.dist, p.ID}
		if bestInd == nil || c.less(indCand) {
			bestInd, indCand, indKind = p, c, farther(ra.kind, rb.kind)
		}
	}

	switch {
	case bestFam != nil && (bestInd == nil || famCand.upA+famCand.upB <= indCand.upA+indCand.upB):
		common := bestFam.Partners()
		sort.Slice(common, func(i, j int) bool { return common[i].ID < common[j].ID })
		return result(Relationship{
			UpA:             famCand.upA,
			UpB:             famCand.upB,
			Description:     describe(famCand.upA, famCand.upB, b.Sex, false),
			CommonAncestors: common,
		}, famKind)
	case bestInd != nil:
		return result(Relationship{
			UpA:             indCand.upA,
			UpB:             indCand.upB,
			Half:            true,
			Description:     describe(indCand.upA, indCand.upB, b.Sex, true),
			CommonAncestors: []*gedcom.Individual{bestInd},
		}, indKind)
	}
	return Relationship{}, false
}

// qualify prefixes a description with the kind of non-birth link, e.g.
// "adoptive father", "foster sister" or "stepbrother".
func qualify(desc string, kind gedcom.Pedigree) string {
	switch kind {
	case gedcom.PedigreeUnknown, gedcom.PedigreeBirth:
		return desc
	case gedcom.PedigreeStep:
		switch desc {
		case "father", "mother", "parent", "son", "daughter", "child", "brother", "sister", "sibling":
			return "step" + desc
		}
		return "step-" + desc
	case gedcom.PedigreeSealed:
		return desc + " (sealed)"
	}
	return kind.Adjective() + " " + desc
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
