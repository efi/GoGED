package gedcom

import (
	"strings"
	"unicode"
)

// EventClass groups event tags by how important they usually are to a
// genealogist.
type EventClass int

// Event classes.
const (
	// ClassVital events are the core facts of a life: birth, baptism,
	// marriage, divorce, death and burial.
	ClassVital EventClass = iota
	// ClassEvent covers other dated life events (census, emigration, ...).
	ClassEvent
	// ClassAttribute covers attributes such as occupation or residence.
	ClassAttribute
)

type eventInfo struct {
	label string
	class EventClass
}

// eventTags lists the individual and family event and attribute tags
// defined by GEDCOM 5.5.1/7.0 plus a few widespread extensions.
var eventTags = map[string]eventInfo{
	// Individual events.
	"BIRT": {"Birth", ClassVital},
	"CHR":  {"Christening", ClassVital},
	"BAPM": {"Baptism", ClassVital},
	"DEAT": {"Death", ClassVital},
	"BURI": {"Burial", ClassVital},
	"CREM": {"Cremation", ClassVital},
	"ADOP": {"Adoption", ClassVital},
	"BARM": {"Bar mitzvah", ClassEvent},
	"BASM": {"Bat mitzvah", ClassEvent},
	"BLES": {"Blessing", ClassEvent},
	"CHRA": {"Adult christening", ClassEvent},
	"CONF": {"Confirmation", ClassEvent},
	"FCOM": {"First communion", ClassEvent},
	"ORDN": {"Ordination", ClassEvent},
	"NATU": {"Naturalization", ClassEvent},
	"EMIG": {"Emigration", ClassEvent},
	"IMMI": {"Immigration", ClassEvent},
	"CENS": {"Census", ClassEvent},
	"PROB": {"Probate", ClassEvent},
	"WILL": {"Will", ClassEvent},
	"GRAD": {"Graduation", ClassEvent},
	"RETI": {"Retirement", ClassEvent},
	"EVEN": {"Event", ClassEvent},
	"BAPL": {"LDS baptism", ClassEvent},
	"CONL": {"LDS confirmation", ClassEvent},
	"ENDL": {"LDS endowment", ClassEvent},
	"SLGC": {"LDS sealing to parents", ClassEvent},
	"INIL": {"LDS initiatory", ClassEvent},
	// Individual attributes.
	"CAST": {"Caste", ClassAttribute},
	"DSCR": {"Description", ClassAttribute},
	"EDUC": {"Education", ClassAttribute},
	"IDNO": {"ID number", ClassAttribute},
	"NATI": {"Nationality", ClassAttribute},
	"NCHI": {"Number of children", ClassAttribute},
	"NMR":  {"Number of marriages", ClassAttribute},
	"OCCU": {"Occupation", ClassAttribute},
	"PROP": {"Property", ClassAttribute},
	"RELI": {"Religion", ClassAttribute},
	"RESI": {"Residence", ClassAttribute},
	"SSN":  {"Social security number", ClassAttribute},
	"TITL": {"Title", ClassAttribute},
	"FACT": {"Fact", ClassAttribute},
	// Family events.
	"MARR": {"Marriage", ClassVital},
	"DIV":  {"Divorce", ClassVital},
	"ANUL": {"Annulment", ClassVital},
	"DIVF": {"Divorce filed", ClassEvent},
	"ENGA": {"Engagement", ClassEvent},
	"MARB": {"Marriage banns", ClassEvent},
	"MARC": {"Marriage contract", ClassEvent},
	"MARL": {"Marriage license", ClassEvent},
	"MARS": {"Marriage settlement", ClassEvent},
	"SLGS": {"LDS sealing to spouse", ClassEvent},
	// Common extensions.
	"_MILT":   {"Military service", ClassEvent},
	"_MILI":   {"Military service", ClassEvent},
	"_DEG":    {"Degree", ClassEvent},
	"_ELEC":   {"Elected", ClassEvent},
	"_EMPLOY": {"Employment", ClassEvent},
	"_FUN":    {"Funeral", ClassEvent},
	"_MDCL":   {"Medical", ClassAttribute},
	"_SEPR":   {"Separation", ClassEvent},
	"_STAT":   {"Status", ClassAttribute},
}

// BirthTags are the tags that establish when a person was born, in order of
// preference. DeathTags do the same for the end of life.
var (
	BirthTags = []string{"BIRT", "CHR", "BAPM"}
	DeathTags = []string{"DEAT", "BURI", "CREM"}
)

// IsEventTag reports whether tag is a known event or attribute tag.
func IsEventTag(tag string) bool {
	_, ok := eventTags[tag]
	return ok
}

// EventLabel returns a human readable label for an event tag.
func EventLabel(tag string) string {
	if info, ok := eventTags[tag]; ok {
		return info.label
	}
	return humanizeTag(tag)
}

// EventTagsByLabel returns the known tags whose label or tag starts with the
// given (case-insensitive) prefix; used to resolve "type:birth" filters.
func EventTagsByLabel(prefix string) []string {
	p := strings.ToLower(strings.TrimSpace(prefix))
	if p == "" {
		return nil
	}
	var out []string
	for tag, info := range eventTags {
		if strings.HasPrefix(strings.ToLower(tag), p) || strings.HasPrefix(strings.ToLower(info.label), p) {
			out = append(out, tag)
		}
	}
	return out
}

// humanizeTag turns an unknown tag such as "_MYEVENT" into "Myevent".
func humanizeTag(tag string) string {
	t := strings.Trim(tag, "_")
	if t == "" {
		return tag
	}
	r := []rune(strings.ToLower(strings.ReplaceAll(t, "_", " ")))
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// Place is the location of an event. GEDCOM places list jurisdictions from
// smallest to largest, separated by commas.
type Place struct {
	Name string
}

// Parts returns the comma-separated jurisdictions with blanks removed.
func (p Place) Parts() []string {
	var out []string
	for _, s := range strings.Split(p.Name, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// String returns the place with normalized separators.
func (p Place) String() string { return strings.Join(p.Parts(), ", ") }

// Short returns the most specific jurisdiction.
func (p Place) Short() string {
	if parts := p.Parts(); len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// Event is an individual or family event or attribute.
type Event struct {
	Tag     string
	Type    string // TYPE substructure, e.g. the kind of an EVEN or FACT
	Value   string // line value, e.g. the occupation for OCCU
	Date    Date
	Place   Place
	Age     string // AGE of the principal at the event
	Cause   string // CAUS
	Address string // ADDR, flattened to one line
	Notes   []string
	Node    *Node

	Individual *Individual // owner of an individual event
	Family     *Family     // owner of a family event
}

// Class returns how important the event is considered to be.
func (e *Event) Class() EventClass {
	if info, ok := eventTags[e.Tag]; ok {
		return info.class
	}
	return ClassEvent
}

// IsVital reports whether the event is one of the core vital events.
func (e *Event) IsVital() bool { return e.Class() == ClassVital }

// Label returns a human readable name for the event. Generic EVEN and FACT
// structures use their TYPE.
func (e *Event) Label() string {
	if (e.Tag == "EVEN" || e.Tag == "FACT" || e.Tag == "IDNO") && e.Type != "" {
		return e.Type
	}
	return EventLabel(e.Tag)
}

// Detail returns the descriptive value of the event: the attribute value
// (e.g. the occupation) or cause, excluding the "Y" flag used to assert
// that an event happened without further details.
func (e *Event) Detail() string {
	v := strings.TrimSpace(strings.ReplaceAll(e.Value, "\n", " "))
	if strings.EqualFold(v, "Y") {
		v = ""
	}
	if v == "" && e.Type != "" && e.Label() != e.Type {
		v = e.Type
	}
	if e.Cause != "" {
		if v != "" {
			v += "; "
		}
		v += "cause: " + e.Cause
	}
	return v
}

// Principals returns the people the event belongs to.
func (e *Event) Principals() []*Individual {
	if e.Individual != nil {
		return []*Individual{e.Individual}
	}
	if e.Family != nil {
		return e.Family.Partners()
	}
	return nil
}

// isEventNode decides whether a child of an INDI or FAM record describes an
// event. Unknown tags count when they carry a date or place.
func isEventNode(n *Node) bool {
	if IsEventTag(n.Tag) {
		return true
	}
	if !strings.HasPrefix(n.Tag, "_") {
		return false
	}
	return n.First("DATE") != nil || n.First("PLAC") != nil
}

func newEvent(doc *Document, n *Node) *Event {
	e := &Event{
		Tag:   n.Tag,
		Value: strings.TrimSpace(n.Value),
		Type:  strings.TrimSpace(n.Val("TYPE")),
		Date:  ParseDate(n.Val("DATE")),
		Place: Place{Name: strings.TrimSpace(n.Val("PLAC"))},
		Age:   strings.TrimSpace(n.Val("AGE")),
		Cause: strings.TrimSpace(n.Val("CAUS")),
		Node:  n,
	}
	if a := n.First("ADDR"); a != nil {
		parts := strings.Split(a.Value, "\n")
		for _, tag := range []string{"CITY", "STAE", "POST", "CTRY"} {
			parts = append(parts, a.Val(tag))
		}
		var keep []string
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				keep = append(keep, p)
			}
		}
		e.Address = strings.Join(keep, ", ")
	}
	e.Notes = doc.notesOf(n)
	return e
}
