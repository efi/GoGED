package main

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/efi/goged/chart"
	"github.com/efi/goged/diff"
	"github.com/efi/goged/gedcom"
	"github.com/efi/goged/genealogy"
	"github.com/efi/goged/search"
)

// The -json output: what the batch flags print, as data. Names of fields
// are in snake case; empty values are left out.

// writeJSON writes v indented, without escaping <, > and &.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// jsonRef names a person.
type jsonRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func refJSON(ind *gedcom.Individual) *jsonRef {
	if ind == nil {
		return nil
	}
	return &jsonRef{ind.ID, ind.DisplayName()}
}

// jsonPerson describes a person.
type jsonPerson struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Given    string     `json:"given,omitempty"`
	Surname  string     `json:"surname,omitempty"`
	Sex      string     `json:"sex"`
	Lifespan string     `json:"lifespan,omitempty"`
	Birth    *jsonEvent `json:"birth,omitempty"`
	Death    *jsonEvent `json:"death,omitempty"`
}

func personJSON(ind *gedcom.Individual) jsonPerson {
	n := ind.Name()
	return jsonPerson{
		ID: ind.ID, Name: ind.DisplayName(), Given: n.Given, Surname: n.FullSurname(), Sex: ind.Sex.String(),
		Lifespan: ind.Lifespan(),
		Birth:    eventJSON(ind.FirstDatedEvent(gedcom.BirthTags...)),
		Death:    eventJSON(ind.FirstDatedEvent(gedcom.DeathTags...)),
	}
}

// jsonEvent describes an event: its tag, label, date (as shown and as
// written in the file, with the year it falls in), place and details.
type jsonEvent struct {
	Type       string `json:"type"`
	Label      string `json:"label"`
	Date       string `json:"date,omitempty"`
	DateGEDCOM string `json:"date_gedcom,omitempty"`
	Year       int    `json:"year,omitempty"`
	Place      string `json:"place,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

func eventJSON(e *gedcom.Event) *jsonEvent {
	if e == nil {
		return nil
	}
	j := &jsonEvent{Type: e.Tag, Label: e.Label(), Place: e.Place.String(), Detail: e.Detail()}
	if !e.Date.IsZero() {
		j.Date, j.DateGEDCOM = e.Date.String(), strings.TrimSpace(e.Date.Raw)
		j.Year = e.Date.Year()
	}
	return j
}

// -q

type jsonResult struct {
	jsonPerson
	Score       int    `json:"score"`
	MatchedName string `json:"matched_name,omitempty"`
}

func queryJSON(results []search.Result) []jsonResult {
	out := []jsonResult{}
	for _, r := range results {
		j := jsonResult{jsonPerson: personJSON(r.Individual), Score: r.Score}
		if ind := r.Individual; r.NameIndex > 0 && r.NameIndex < len(ind.Names) {
			j.MatchedName = ind.Names[r.NameIndex].String()
		}
		out = append(out, j)
	}
	return out
}

// -timeline

type jsonTimeline struct {
	Person jsonPerson          `json:"person"`
	Events []jsonTimelineEntry `json:"events"`
}

type jsonTimelineEntry struct {
	jsonEvent
	Title       string     `json:"title"`
	Relation    string     `json:"relation,omitempty"`
	Relative    *jsonRef   `json:"relative,omitempty"`
	Spouse      *jsonRef   `json:"spouse,omitempty"`
	Age         string     `json:"age,omitempty"`
	Restriction string     `json:"restriction,omitempty"`
	Facts       []jsonFact `json:"facts,omitempty"`
}

// jsonFact is a detail of an event, such as the godparents of a baptism.
// An event may have several with the same label.
type jsonFact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

func timelineJSON(ind *gedcom.Individual) jsonTimeline {
	t := jsonTimeline{Person: personJSON(ind), Events: []jsonTimelineEntry{}}
	for _, e := range genealogy.Timeline(ind, genealogy.TimelineOptions{Relatives: true}) {
		j := jsonTimelineEntry{jsonEvent: *eventJSON(e.Event), Title: e.Title(), Relation: e.Relation,
			Relative: refJSON(e.Relative), Spouse: refJSON(e.Spouse), Restriction: e.Event.Restriction}
		if e.HasAge {
			j.Age = e.Age.String()
		}
		if e.Own() {
			for _, f := range e.Event.Facts() {
				j.Facts = append(j.Facts, jsonFact{f.Label, f.Value})
			}
		}
		t.Events = append(t.Events, j)
	}
	return t
}

// -relate

type jsonRelationship struct {
	A               jsonRef   `json:"a"`
	B               jsonRef   `json:"b"`
	Kind            string    `json:"kind"`
	Description     string    `json:"description,omitempty"`
	Sentence        string    `json:"sentence"`
	UpA             int       `json:"generations_up_a,omitempty"`
	UpB             int       `json:"generations_up_b,omitempty"`
	Half            bool      `json:"half,omitempty"`
	CommonAncestors []jsonRef `json:"common_ancestors,omitempty"`
	Via             *jsonRef  `json:"via,omitempty"`
	Pedigree        string    `json:"pedigree,omitempty"`
}

func relationshipJSON(a, b *gedcom.Individual) jsonRelationship {
	r := genealogy.Relate(a, b)
	j := jsonRelationship{
		A: *refJSON(a), B: *refJSON(b), Description: r.Description, UpA: r.UpA, UpB: r.UpB, Half: r.Half,
		Via: refJSON(r.Via), Pedigree: string(r.Pedigree),
		Kind:     [...]string{"none", "self", "blood", "marriage", "adoptive"}[r.Kind],
		Sentence: relationshipLines(a, b, r)[0],
	}
	for _, c := range r.CommonAncestors {
		j.CommonAncestors = append(j.CommonAncestors, *refJSON(c))
	}
	return j
}

// -events

type jsonEventPeople struct {
	jsonEvent
	People []jsonRef `json:"people"`
}

func eventsJSON(events []*gedcom.Event) []jsonEventPeople {
	out := []jsonEventPeople{}
	for _, e := range events {
		j := jsonEventPeople{jsonEvent: *eventJSON(e), People: []jsonRef{}}
		for _, p := range e.Principals() {
			j.People = append(j.People, *refJSON(p))
		}
		out = append(out, j)
	}
	return out
}

// -places

type jsonPlace struct {
	Name      string      `json:"name"`
	Full      string      `json:"full"`
	Events    int         `json:"events"`
	People    int         `json:"people"`
	Latitude  *float64    `json:"latitude,omitempty"`
	Longitude *float64    `json:"longitude,omitempty"`
	GOV       string      `json:"gov,omitempty"`
	Aliases   []string    `json:"aliases,omitempty"`
	Places    []jsonPlace `json:"places,omitempty"` // the places within it
}

func placesJSON(n *genealogy.PlaceNode) []jsonPlace {
	out := []jsonPlace{}
	for _, c := range n.Children {
		j := jsonPlace{Name: c.Name, Full: c.Full, Events: c.Count(), People: c.People(), GOV: c.GOV, Aliases: c.Aliases}
		if c.HasCoords {
			lat, lon := c.Lat, c.Lon
			j.Latitude, j.Longitude = &lat, &lon
		}
		if len(c.Children) > 0 {
			j.Places = placesJSON(c)
		}
		out = append(out, j)
	}
	return out
}

// -stats

type jsonCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type jsonValueCount struct {
	Value int `json:"value"`
	Count int `json:"count"`
}

type jsonStats struct {
	File struct {
		Name      string `json:"name"`
		Encoding  string `json:"encoding"`
		Version   string `json:"gedcom_version,omitempty"`
		CreatedBy string `json:"created_by,omitempty"`
	} `json:"file"`
	Individuals     int         `json:"individuals"`
	Males           int         `json:"males"`
	Females         int         `json:"females"`
	OtherSex        int         `json:"other_sex"`
	Families        int         `json:"families"`
	Events          int         `json:"events"`
	Places          int         `json:"places"`
	Sources         int         `json:"sources"`
	EarliestYear    int         `json:"earliest_year,omitempty"`
	LatestYear      int         `json:"latest_year,omitempty"`
	Generations     int         `json:"generations"`
	Lifespans       int         `json:"lifespans"`
	AverageLifespan float64     `json:"average_lifespan,omitempty"`
	LongestLived    *jsonRef    `json:"longest_lived,omitempty"`
	LongestAge      string      `json:"longest_age,omitempty"`
	Surnames        []jsonCount `json:"surnames"`
	GivenNames      []jsonCount `json:"given_names"`
	Collapse        struct {
		Percent           float64 `json:"percent"`
		AncestorPositions float64 `json:"ancestor_positions"`
		DistinctAncestors int     `json:"distinct_ancestors"`
		PeopleWithRepeats int     `json:"people_with_repeats"`
	} `json:"pedigree_collapse"`
	Distributions map[string][]jsonValueCount `json:"distributions"`
	Restricted    int                         `json:"restricted"`
	Warnings      []jsonWarning               `json:"warnings"`
}

type jsonWarning struct {
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

func statsJSON(doc *gedcom.Document, name string) jsonStats {
	s := genealogy.Compute(doc)
	var j jsonStats
	j.File.Name, j.File.Encoding, j.File.Version, j.File.CreatedBy = name, string(doc.Encoding), doc.Version, doc.SourceSoftware()
	j.Individuals, j.Males, j.Females, j.OtherSex = s.Individuals, s.Males, s.Females, s.OtherSex
	j.Families, j.Events, j.Places, j.Sources = s.Families, s.Events, s.Places, s.Sources
	j.EarliestYear, j.LatestYear, j.Generations = s.EarliestYear, s.LatestYear, s.Generations
	j.Lifespans, j.AverageLifespan = s.Lifespans, s.AverageLifespan
	if s.LongestLived != nil {
		j.LongestLived, j.LongestAge = refJSON(s.LongestLived), s.LongestAge.String()
	}
	counts := func(ncs []genealogy.NameCount) []jsonCount {
		out := []jsonCount{}
		for _, nc := range ncs {
			out = append(out, jsonCount{nc.Name, nc.Count})
		}
		return out
	}
	j.Surnames, j.GivenNames = counts(s.Surnames), counts(s.GivenNames)
	j.Collapse.Percent, j.Collapse.AncestorPositions = s.PedigreeCollapse(), s.AncestorPositions
	j.Collapse.DistinctAncestors, j.Collapse.PeopleWithRepeats = s.DistinctAncestors, s.Collapsed
	j.Distributions = map[string][]jsonValueCount{}
	for name, d := range map[string]genealogy.Distribution{
		"birth_years": s.BirthYears, "death_ages": s.DeathAges, "marriage_ages": s.MarriageAges, "family_sizes": s.FamilySizes,
	} {
		vs := []jsonValueCount{}
		for _, vc := range d {
			vs = append(vs, jsonValueCount{vc.Value, vc.Count})
		}
		j.Distributions[name] = vs
	}
	j.Restricted = doc.Redactions
	j.Warnings = []jsonWarning{}
	for _, w := range doc.Warnings {
		j.Warnings = append(j.Warnings, jsonWarning{w.Line, w.Msg})
	}
	return j
}

// -tree, -desc, -hourglass: a person with their parents or their families
// and children, as far as the generations go.

type jsonTree struct {
	jsonPerson
	Father *jsonTree `json:"father,omitempty"`
	Mother *jsonTree `json:"mother,omitempty"`
	// MoreAncestors: there are parents beyond the generations shown.
	// AncestorsElsewhere: the person appears more than once, and their
	// ancestors are given at another place.
	MoreAncestors      bool         `json:"more_ancestors,omitempty"`
	AncestorsElsewhere bool         `json:"ancestors_elsewhere,omitempty"`
	Families           []jsonFamily `json:"families,omitempty"`
	// MoreDescendants: there are children beyond the generations shown.
	// DescendantsElsewhere: the person appears more than once among the
	// descendants, and theirs are given at the first place.
	MoreDescendants      bool   `json:"more_descendants,omitempty"`
	DescendantsElsewhere bool   `json:"descendants_elsewhere,omitempty"`
	Link                 string `json:"link,omitempty"` // a child's link to the family other than birth: adopted …
}

type jsonFamily struct {
	ID       string      `json:"id"`
	Partner  *jsonPerson `json:"partner"`
	Marriage *jsonEvent  `json:"marriage,omitempty"`
	Children []jsonTree  `json:"children"`
}

// ancestorsJSON gives the pedigree of node i of c, as the chart shows it.
func ancestorsJSON(c *chart.Chart, i int) *jsonTree {
	n := c.Nodes[i]
	t := &jsonTree{jsonPerson: personJSON(n.Ind), MoreAncestors: n.More}
	for _, d := range n.Down {
		dn := c.Nodes[d]
		switch {
		case dn.Left:
		case dn.Kind == chart.KindDuplicate:
			t.AncestorsElsewhere = true
		case strings.HasSuffix(dn.Path, "F"):
			t.Father = ancestorsJSON(c, d)
		default:
			t.Mother = ancestorsJSON(c, d)
		}
	}
	return t
}

// descendantsJSON adds the families of t's person and their children, as
// the descendant chart shows them.
func descendantsJSON(t *jsonTree, ind *gedcom.Individual, gen, gens int, seen map[*gedcom.Individual]bool) {
	families := ind.FamiliesAsSpouse()
	if gen >= gens-1 {
		for _, f := range families {
			t.MoreDescendants = t.MoreDescendants || len(f.Children) > 0
		}
		return
	}
	t.Families = []jsonFamily{}
	for _, f := range families {
		fam := jsonFamily{ID: f.ID, Children: []jsonTree{}}
		if p := f.Partner(ind); p != nil {
			pj := personJSON(p)
			fam.Partner = &pj
		}
		if m := f.Marriage(); m != nil {
			fam.Marriage = eventJSON(m)
		}
		for _, child := range f.Children {
			ct := jsonTree{jsonPerson: personJSON(child)}
			if l, _ := child.ChildLink(f); !l.IsBirth() {
				ct.Link = string(l.Kind())
			}
			if seen[child] {
				ct.DescendantsElsewhere = true
			} else {
				seen[child] = true
				descendantsJSON(&ct, child, gen+1, gens, seen)
			}
			fam.Children = append(fam.Children, ct)
		}
		t.Families = append(t.Families, fam)
	}
}

// treeJSON gives the chart of root as data: ancestors for a pedigree,
// descendants for a descendant chart, both for an hourglass.
func treeJSON(root *gedcom.Individual, opts chart.Options, ancestors, descendants bool) *jsonTree {
	if opts.Generations < 1 {
		opts.Generations = 4
	}
	t := &jsonTree{jsonPerson: personJSON(root)}
	if ancestors {
		t = ancestorsJSON(chart.Pedigree(root, opts), 0)
	}
	if descendants {
		descendantsJSON(t, root, 0, opts.Generations, map[*gedcom.Individual]bool{root: true})
	}
	return t
}

// -lifespans

type jsonLifespans struct {
	Person      *jsonRef   `json:"person"`
	Generations int        `json:"generations"`
	People      []jsonLife `json:"people"`
}

// jsonLife is a row of the lifespans. The years are those drawn: an
// approximate date spans several (about 1795 is 1793 to 1797), and the
// first and last record stand in for a birth or death not recorded.
type jsonLife struct {
	jsonPerson
	Relation    string        `json:"relation,omitempty"`
	Generation  int           `json:"generation"` // above the person, negative below
	Partner     bool          `json:"partner,omitempty"`
	BornYears   *jsonYears    `json:"born_years,omitempty"`
	DiedYears   *jsonYears    `json:"died_years,omitempty"`
	FirstRecord int           `json:"first_record,omitempty"`
	LastRecord  int           `json:"last_record,omitempty"`
	Marriages   []int         `json:"marriages,omitempty"`
	Together    *jsonTogether `json:"together,omitempty"`
}

type jsonYears struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// jsonTogether is how long somebody lived at the same time as the person,
// for a partner how long they were married, and for the person the age
// they reached.
type jsonTogether struct {
	Years       int  `json:"years"`
	Married     bool `json:"married,omitempty"`
	Approximate bool `json:"approximate,omitempty"`
	AtLeast     bool `json:"at_least,omitempty"`
}

func lifespansJSON(c *chart.Chart, gens int) jsonLifespans {
	j := jsonLifespans{Person: refJSON(c.Nodes[0].Ind), Generations: max(1, gens), People: []jsonLife{}}
	years := func(y chart.Years) *jsonYears {
		if !y.OK {
			return nil
		}
		return &jsonYears{y.From, y.To}
	}
	for _, l := range c.Lives {
		jl := jsonLife{jsonPerson: personJSON(l.Ind), Relation: l.Relation, Generation: l.Gen, Partner: l.Partner,
			BornYears: years(l.Born), DiedYears: years(l.Died), Marriages: l.Marriages}
		if !l.Born.OK && l.Records.OK {
			jl.FirstRecord = l.Records.From
		}
		if !l.Died.OK && l.Records.OK {
			jl.LastRecord = l.Records.To
		}
		if t := l.Together; t.OK && !(t.AtLeast && t.Years == 0) {
			jl.Together = &jsonTogether{t.Years, t.Married, t.Approx, t.AtLeast}
		}
		j.People = append(j.People, jl)
	}
	return j
}

// -diff

type jsonDiff struct {
	Summary  string           `json:"summary"`
	Header   []jsonDiffDetail `json:"header"`
	People   []jsonDiffRecord `json:"people"`
	Families []jsonDiffRecord `json:"families"`
	Others   []jsonDiffRecord `json:"other_records"`
}

type jsonDiffDetail struct {
	Kind string `json:"kind"`
	What string `json:"what"`
	Old  string `json:"old,omitempty"`
	New  string `json:"new,omitempty"`
	Text string `json:"text"`
}

type jsonDiffRecord struct {
	Kind    string           `json:"kind"`
	ID      string           `json:"id"`
	Type    string           `json:"type"`
	Noun    string           `json:"noun"`
	Title   string           `json:"title,omitempty"`
	Details []jsonDiffDetail `json:"details,omitempty"`
}

var diffKinds = [...]string{"", "added", "removed", "changed"}

func diffJSON(r diff.Result) jsonDiff {
	details := func(ds []diff.Detail) []jsonDiffDetail {
		out := []jsonDiffDetail{}
		for _, d := range ds {
			out = append(out, jsonDiffDetail{diffKinds[d.Kind], d.What, d.Old, d.New, d.String()})
		}
		return out
	}
	records := func(rs []diff.Record) []jsonDiffRecord {
		out := []jsonDiffRecord{}
		for _, rec := range rs {
			j := jsonDiffRecord{Kind: diffKinds[rec.Kind], ID: rec.ID, Type: rec.Tag, Noun: rec.Noun(), Title: rec.Title}
			if len(rec.Details) > 0 {
				j.Details = details(rec.Details)
			}
			out = append(out, j)
		}
		return out
	}
	return jsonDiff{r.Summary(), details(r.Header), records(r.People), records(r.Families), records(r.Others)}
}
